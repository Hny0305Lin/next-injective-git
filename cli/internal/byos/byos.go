// Package byos wires the storage-neutral successor suite into Git flows:
// verified manifest publication under revision CAS, and verified manifest/pack
// reads. It never initializes Kubo, gateways or replication, and never holds
// cloud secret values (only credential references resolved into providers).
package byos

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain/successor"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore/s3store"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
)

// Chain is the successor-suite seam implemented by chain.SuccessorRegistry in
// production and by test fakes locally.
type Chain interface {
	ChainIDDecimal() string
	DirectoryHex() string
	ResolveRepository(ctx context.Context, owner, repo string) (successor.RepositoryView, bool, error)
	ListRefNames(ctx context.Context, repoID [32]byte) ([]string, error)
	GetRef(ctx context.Context, repoID [32]byte, refName string) (successor.RefState, error)
	UpdateRef(ctx context.Context, repoID [32]byte, refName, commitSHA string, commitment successor.Commitment, expectedRevision uint64, expectedDigest [32]byte, force bool) (string, error)
	DeleteRef(ctx context.Context, repoID [32]byte, refName string) (string, error)
}

// GitRepo is the verified Git seam (gitio.Repo satisfies it).
type GitRepo interface {
	ResolveRef(ref string) string
	ScanSecrets(rev string) []string
	PackFullHistory(ctx context.Context, tip, dir string) (*packstore.File, error)
	IndexVerified(ctx context.Context, file *packstore.File, o packstore.Object, tip string) error
}

// Progress reports bilingual user-visible progress.
type Progress func(english, chinese string, args ...any)

// StoreFactory constructs the verified cloud writer and reader table for a
// repository view. Production uses the repo-bound storage profiles; tests
// inject in-memory stores.
type StoreFactory interface {
	Writer(view successor.RepositoryView) (packstore.Writer, string, string, string, error)
	Readers() packstore.Readers
	ProviderName() string
}

// ProfileStoreFactory binds chainId+Directory+repoId to explicit profiles and
// builds the production AWS/R2 writer and readers. Credential values are
// resolved inside the SDK provider from the referenced environment names.
type ProfileStoreFactory struct {
	Profiles storageconfig.Config
	Chain    Chain
}

func (f ProfileStoreFactory) Writer(view successor.RepositoryView) (packstore.Writer, string, string, string, error) {
	writerProfile, readerProfile, err := f.Profiles.Select(f.Chain.ChainIDDecimal(), f.Chain.DirectoryHex(), fmt.Sprintf("%#x", view.RepoID))
	if err != nil {
		return nil, "", "", "", i18n.Errorf(
			"no storage profile bound to this repository (chainId/directory/repoId); run igit storage add",
			"未找到绑定此仓库的 storage 配置（chainId/directory/repoId）；请运行 igit storage add")
	}
	writer, err := s3store.NewWriter(writerProfile)
	if err != nil {
		return nil, "", "", "", err
	}
	base := writerProfile.PublicReadBase
	if base == "" {
		base = readerProfile.PublicReadBase
	}
	if base == "" {
		return nil, "", "", "", i18n.Errorf(
			"no publicReadBase configured for the bootstrap locator",
			"未配置 publicReadBase，无法生成 bootstrap locator")
	}
	return writer, base, writerProfile.Prefix, writerProfile.Provider, nil
}

func (f ProfileStoreFactory) Readers() packstore.Readers {
	public := map[string]packstore.Reader{}
	authenticated := map[string]packstore.Reader{}
	for name, profile := range f.Profiles.Profiles {
		store, err := s3store.NewReader(profile)
		if err != nil {
			continue
		}
		public[profile.Provider] = store
		authenticated[name] = store
	}
	return packstore.Readers{Public: public, Authenticated: authenticated}
}

func (f ProfileStoreFactory) ProviderName() string {
	for _, profile := range f.Profiles.Profiles {
		return profile.Provider
	}
	return ""
}

// Service executes BYOS publish/read flows for one helper session.
type Service struct {
	chain    Chain
	git      GitRepo
	stores   StoreFactory
	progress Progress
	provider string
	ingested map[string]bool
}

func NewService(chain Chain, git GitRepo, stores StoreFactory, progress Progress) (*Service, error) {
	if chain == nil {
		return nil, errors.New("byos: chain is nil")
	}
	if stores == nil {
		return nil, errors.New("byos: store factory is nil")
	}
	if progress == nil {
		progress = func(string, string, ...any) {}
	}
	service := &Service{chain: chain, git: git, stores: stores, progress: progress, ingested: map[string]bool{}}
	service.provider = stores.ProviderName()
	return service, nil
}

// RefListing is one advertised ref: the commit OID is read from the verified
// manifest (the successor suite does not store it on-chain).
type RefListing struct {
	RefName   string
	CommitOID string
	Revision  uint64
}

// ListRefs resolves the repository and advertises every live ref with the
// commit OID learned from its verified manifest.
func (s *Service) ListRefs(ctx context.Context, owner, repo string) ([]RefListing, string, error) {
	view, _, err := s.chain.ResolveRepository(ctx, owner, repo)
	if err != nil {
		return nil, "", err
	}
	names, err := s.chain.ListRefNames(ctx, view.RepoID)
	if err != nil {
		return nil, "", err
	}
	listings := make([]RefListing, 0, len(names))
	for _, name := range names {
		state, err := s.chain.GetRef(ctx, view.RepoID, name)
		if err != nil {
			return nil, "", err
		}
		manifest, err := s.readManifest(ctx, view, name, state)
		if err != nil {
			return nil, "", err
		}
		listings = append(listings, RefListing{RefName: name, CommitOID: manifest.Commit.OID, Revision: state.Revision})
	}
	return listings, view.DefaultBranch, nil
}

// FetchRef downloads, verifies and ingests every pack of one ref. wantOID, when
// non-empty, must equal the manifest-bound commit.
func (s *Service) FetchRef(ctx context.Context, owner, repo, refName, wantOID, tmpDir string) error {
	view, _, err := s.chain.ResolveRepository(ctx, owner, repo)
	if err != nil {
		return err
	}
	state, err := s.chain.GetRef(ctx, view.RepoID, refName)
	if err != nil {
		return err
	}
	manifest, err := s.readManifest(ctx, view, refName, state)
	if err != nil {
		return err
	}
	if wantOID != "" && manifest.Commit.OID != wantOID {
		return i18n.Errorf("ref %s moved to another commit while fetching", "获取期间 ref %s 已更新到另一个提交", refName)
	}
	for _, entry := range manifest.Packs {
		if s.ingested[entry.SHA256] {
			continue
		}
		size, err := packmanifest.Size(entry.Size, packmanifest.MaxPackBytes)
		if err != nil {
			return err
		}
		object := packstore.Object{Kind: "packs", SHA256: entry.SHA256, Size: size}
		s.progress("downloading verified pack %s (%d bytes)", "正在下载已验证 pack %s（%d 字节）", entry.SHA256[:12], size)
		file, err := packstore.ReadVerified(ctx, s.stores.Readers(), entry.Locations, object, tmpDir)
		if err != nil {
			return i18n.Errorf("verified pack download failed: %w", "已验证 pack 下载失败：%w", err)
		}
		s.progress("ingesting verified pack into git", "正在将已验证 pack 导入 git")
		if err := s.git.IndexVerified(ctx, file, object, manifest.Commit.OID); err != nil {
			_ = file.Close()
			return i18n.Errorf("git rejected verified pack: %w", "git 拒绝了已验证 pack：%w", err)
		}
		_ = file.Close()
		s.ingested[entry.SHA256] = true
	}
	return nil
}

// PushRef publishes a self-contained full-history pack plus manifest and CASes
// the ref commitment. CAS conflicts keep the uploaded objects for a directed
// retry; nothing is force-replaced or deleted.
func (s *Service) PushRef(ctx context.Context, owner, repo, srcRef, dstRef string, force bool, tmpDir string) error {
	view, _, err := s.chain.ResolveRepository(ctx, owner, repo)
	if err != nil {
		return err
	}
	if srcRef == "" {
		s.progress("deleting %s on chain (tombstone)", "正在从链上删除 %s（tombstone）", dstRef)
		_, err := s.chain.DeleteRef(ctx, view.RepoID, dstRef)
		return err
	}
	tip := s.git.ResolveRef(srcRef)
	if tip == "" {
		return i18n.Errorf("cannot resolve local ref %s", "无法解析本地 ref %s", srcRef)
	}
	for _, match := range s.git.ScanSecrets(tip) {
		s.progress("warning: possible credential in %s", "警告：%s 中可能包含凭据", match)
	}
	expectedRevision, expectedDigest, err := s.currentExpectation(ctx, view.RepoID, dstRef)
	if err != nil {
		return err
	}
	writer, publicBase, prefix, provider, err := s.stores.Writer(view)
	if err != nil {
		return err
	}
	s.progress("packing self-contained full history for %s", "正在为 %s 打包自包含完整历史", dstRef)
	packFile, err := s.git.PackFullHistory(ctx, tip, tmpDir)
	if err != nil {
		return err
	}
	defer func() { _ = packFile.Close() }()
	manifest, err := s.buildManifest(view, dstRef, tip, packFile.Source(), provider, publicBase, prefix)
	if err != nil {
		return err
	}
	s.progress("uploading pack and manifest with verification", "正在上传并验证 pack 与 manifest")
	prepared, err := packstore.Prepare(ctx, writer, manifest, []packstore.Source{packFile.Source()}, publicBase, prefix, tmpDir)
	if err != nil {
		return i18n.Errorf("verified upload failed: %w", "已验证上传失败：%w", err)
	}
	commitment := successor.Commitment{
		ManifestDigest:   digestFromHex(prepared.Commitment.SHA256),
		ManifestSize:     mustSize(prepared.Commitment.Size),
		BootstrapLocator: prepared.Commitment.BootstrapLocator,
	}
	s.progress("broadcasting CAS ref update (revision %d)", "正在广播 CAS ref 更新（revision %d）", expectedRevision+1)
	_, err = s.chain.UpdateRef(ctx, view.RepoID, dstRef, tip, commitment, expectedRevision, expectedDigest, force)
	var mismatch *successor.CommitmentMismatchError
	if errors.As(err, &mismatch) {
		if expectedRevision == 0 && mismatch.ActualDigest == [32]byte{} && mismatch.ActualRevision > 0 {
			// The ref was deleted before (tombstone) and cannot be read back
			// through getRef; the chain just told us the monotonic revision a
			// recreate must carry. Retry exactly once with that expectation.
			s.progress("recreating %s from tombstone revision %d", "正在基于 tombstone revision %d 重建 %s", mismatch.ActualRevision, dstRef)
			_, err = s.chain.UpdateRef(ctx, view.RepoID, dstRef, tip, commitment, mismatch.ActualRevision, [32]byte{}, force)
		}
		if err != nil {
			var retryMismatch *successor.CommitmentMismatchError
			if errors.As(err, &retryMismatch) {
				return i18n.Errorf(
					"ref %s changed on chain (push was based on revision %d, chain is %d); uploaded objects are retained - fetch the ref and push again: %w",
					"ref %s 已在链上被更新（推送基于 revision %d，链上是 %d）；已上传对象保留，请重新获取 ref 后再推送：%w",
					dstRef, retryMismatch.ExpectedRevision, retryMismatch.ActualRevision, retryMismatch)
			}
			return err
		}
		return nil
	}
	return err
}

func (s *Service) currentExpectation(ctx context.Context, repoID [32]byte, refName string) (uint64, [32]byte, error) {
	state, err := s.chain.GetRef(ctx, repoID, refName)
	if errors.Is(err, successor.ErrRefNotFound) {
		return 0, [32]byte{}, nil
	}
	if err != nil {
		return 0, [32]byte{}, err
	}
	return state.Revision, state.Commitment.ManifestDigest, nil
}

// readManifest fetches the manifest through the independent reader with a
// bounded length, checks the on-chain digest/size and parses it against the
// ref's binding context.
func (s *Service) readManifest(ctx context.Context, view successor.RepositoryView, refName string, state successor.RefState) (packmanifest.PackManifest, error) {
	commitment := packmanifest.ManifestCommitment{
		SHA256:           fmt.Sprintf("%x", state.Commitment.ManifestDigest),
		Size:             strconv.FormatUint(state.Commitment.ManifestSize, 10),
		BootstrapLocator: state.Commitment.BootstrapLocator,
	}
	body, err := s.stores.Readers().Open(ctx,
		packmanifest.PackLocation{Provider: s.provider, URL: commitment.BootstrapLocator},
		packstore.Object{Kind: "manifests", SHA256: commitment.SHA256, Size: int64(state.Commitment.ManifestSize)})
	if err != nil {
		return packmanifest.PackManifest{}, i18n.Errorf(
			"manifest read failed; configure public read (publicReadBase/CORS) or an independent reader: %w",
			"manifest 读取失败；请配置公开读取（publicReadBase/CORS）或独立 reader：%w", err)
	}
	defer func() { _ = body.Close() }()
	b, err := io.ReadAll(io.LimitReader(body, packmanifest.MaxManifestBytes+1))
	if err != nil {
		return packmanifest.PackManifest{}, err
	}
	base := packmanifest.Context{
		ChainID:        s.chain.ChainIDDecimal(),
		SuiteDirectory: s.chain.DirectoryHex(),
		RepoID:         fmt.Sprintf("%#x", view.RepoID),
		RefName:        refName,
	}
	manifest, err := packmanifest.ParseRefManifest(b, base, commitment)
	if err != nil {
		return packmanifest.PackManifest{}, i18n.Errorf("manifest verification failed against the on-chain commitment", "manifest 与链上承诺核对失败")
	}
	return manifest, nil
}

func (s *Service) buildManifest(view successor.RepositoryView, refName, tip string, pack packstore.Source, provider, publicBase, prefix string) (packmanifest.PackManifest, error) {
	packKey, err := packmanifest.Key(prefix, "packs", pack.SHA256)
	if err != nil {
		return packmanifest.PackManifest{}, err
	}
	m := packmanifest.PackManifest{
		Schema: "igit.pack-manifest", SchemaVersion: 1,
		Context: packmanifest.Context{
			ChainID:        s.chain.ChainIDDecimal(),
			SuiteDirectory: s.chain.DirectoryHex(),
			RepoID:         fmt.Sprintf("%#x", view.RepoID),
			RefName:        refName,
			Commit:         packmanifest.Commit{Algorithm: "sha1", OID: tip},
		},
		Packs: []packmanifest.PackEntry{{
			Sequence: 0, SHA256: pack.SHA256, Size: strconv.FormatInt(pack.Size, 10),
			Format: "git-pack", PackVersion: 2, Thin: false, DependsOn: []string{},
			Locations: []packmanifest.PackLocation{{Provider: provider, URL: publicBase + "/" + packKey}},
		}},
	}
	if err := m.Validate(); err != nil {
		return packmanifest.PackManifest{}, err
	}
	return m, nil
}

func digestFromHex(s string) [32]byte {
	var out [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil || len(raw) != 32 {
		panic("byos: invalid digest " + s)
	}
	copy(out[:], raw)
	return out
}

func mustSize(decimal string) uint64 {
	n, err := strconv.ParseUint(decimal, 10, 64)
	if err != nil {
		panic("byos: invalid size " + decimal)
	}
	return n
}

package successor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/ethereum/go-ethereum/common"
)

const fixtureBase = "https://public.example.com"

// memStore is a fake cloud: conditional create, full read-back and anonymous
// reads over stable https URLs. It never deletes or overwrites.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemStore() *memStore { return &memStore{objects: make(map[string][]byte)} }

func (m *memStore) Capabilities() packstore.Capabilities {
	return packstore.Capabilities{
		Provider:     "cloudflare-r2",
		MaxSinglePut: packmanifest.MaxPackBytes,
		MaxObject:    packmanifest.MaxPackBytes,
		RawReadback:  true,
	}
}

func (m *memStore) PutIfAbsent(_ context.Context, o packstore.Object, s packstore.Source) (packstore.Receipt, error) {
	if err := o.Validate(); err != nil {
		return packstore.Receipt{}, err
	}
	prefix := "igit-fixture"
	key, err := packmanifest.Key(prefix, o.Kind, o.SHA256)
	if err != nil {
		return packstore.Receipt{}, err
	}
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return packstore.Receipt{}, packstore.Fail(packstore.Missing, "memstore-source")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.objects[key]; ok {
		if sha256Hex(existing) != o.SHA256 || int64(len(existing)) != o.Size {
			return packstore.Receipt{}, packstore.Fail(packstore.Conflict, "memstore-conditional-put")
		}
		return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Reused: true, Verified: true}, nil
	}
	if sha256Hex(body) != o.SHA256 || int64(len(body)) != o.Size {
		return packstore.Receipt{}, packstore.Fail(packstore.Integrity, "memstore-source")
	}
	m.objects[key] = body
	return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Verified: true}, nil
}

func (m *memStore) VerifyStoredBytes(_ context.Context, r packstore.Receipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.objects[r.Key]
	if !ok {
		return packstore.Fail(packstore.Missing, "memstore-readback")
	}
	if sha256Hex(body) != r.Object.SHA256 || int64(len(body)) != r.Object.Size {
		return packstore.Fail(packstore.Integrity, "memstore-readback")
	}
	return nil
}

func (m *memStore) ObjectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

// Open is the anonymous reader used by the cold-clone side.
func (m *memStore) Open(_ context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	if l.Provider != "cloudflare-r2" || l.Reader != "" {
		return nil, packstore.Fail(packstore.Auth, "memstore-location")
	}
	if !strings.HasPrefix(l.URL, fixtureBase+"/") {
		return nil, packstore.Fail(packstore.Auth, "memstore-base")
	}
	key := strings.TrimPrefix(l.URL, fixtureBase+"/")
	m.mu.Lock()
	body, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return nil, packstore.Fail(packstore.Missing, "memstore-open")
	}
	return io.NopCloser(strings.NewReader(string(body))), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func digestFromHex(s string) [32]byte {
	var out [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil || len(raw) != 32 {
		panic("invalid digest fixture: " + s)
	}
	copy(out[:], raw)
	return out
}

func mustSize(decimal string) uint64 {
	n, err := strconv.ParseUint(decimal, 10, 64)
	if err != nil {
		panic("invalid size fixture: " + decimal)
	}
	return n
}

func localGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-config"),
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// buildFixtureRepo creates a real two-commit repo and returns its git dir and tip.
func buildFixtureRepo(t *testing.T) (gitDir, tip string) {
	t.Helper()
	dir := t.TempDir()
	localGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	for _, content := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(dir, "history.txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		localGit(t, dir, "add", ".")
		localGit(t, dir, "commit", "-m", content)
	}
	return filepath.Join(dir, ".git"), localGit(t, dir, "rev-parse", "HEAD")
}

func fixtureManifest(t *testing.T, chain *FakeChain, repoID [32]byte, refName, tip string, pack packstore.Source) packmanifest.PackManifest {
	t.Helper()
	packKey, err := packmanifest.Key("igit-fixture", "packs", pack.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return packmanifest.PackManifest{
		Schema: "igit.pack-manifest", SchemaVersion: 1,
		Context: packmanifest.Context{
			ChainID:        "1439",
			SuiteDirectory: fmt.Sprintf("%#x", chain.directory),
			RepoID:         fmt.Sprintf("%#x", repoID),
			RefName:        refName,
			Commit:         packmanifest.Commit{Algorithm: "sha1", OID: tip},
		},
		Packs: []packmanifest.PackEntry{{
			Sequence: 0, SHA256: pack.SHA256, Size: strconv.FormatInt(pack.Size, 10),
			Format: "git-pack", PackVersion: 2, Thin: false, DependsOn: []string{},
			Locations: []packmanifest.PackLocation{{Provider: "cloudflare-r2", URL: fixtureBase + "/" + packKey}},
		}},
	}
}

// publish runs the verified prepare flow and returns the proposed commitment.
func publish(t *testing.T, m packmanifest.PackManifest, packFile *packstore.File, cloud *memStore) packmanifest.ManifestCommitment {
	t.Helper()
	prepared, err := packstore.Prepare(context.Background(), cloud, m,
		[]packstore.Source{packFile.Source()}, fixtureBase, "igit-fixture", t.TempDir())
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prepared.Commitment == nil {
		t.Fatal("prepare returned no commitment")
	}
	return *prepared.Commitment
}

func coldClone(t *testing.T, chain *FakeChain, client *Client, cloud *memStore, repoID [32]byte, refName, wantTip string) {
	t.Helper()
	ctx := context.Background()
	state, err := client.GetRef(ctx, repoID, refName)
	if err != nil {
		t.Fatalf("getRef: %v", err)
	}
	// Bounded manifest fetch with digest/size pre-checks before parsing.
	body, err := cloud.Open(ctx, packmanifest.PackLocation{Provider: "cloudflare-r2", URL: state.Commitment.BootstrapLocator},
		packstore.Object{Kind: "manifests", SHA256: fmt.Sprintf("%x", state.Commitment.ManifestDigest), Size: int64(state.Commitment.ManifestSize)})
	if err != nil {
		t.Fatalf("manifest fetch: %v", err)
	}
	defer body.Close()
	manifestBytes, err := io.ReadAll(io.LimitReader(body, packmanifest.MaxManifestBytes+1))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(manifestBytes)) != int64(state.Commitment.ManifestSize) ||
		sha256Hex(manifestBytes) != fmt.Sprintf("%x", state.Commitment.ManifestDigest) {
		t.Fatalf("manifest precheck failed: size %d digest mismatch", len(manifestBytes))
	}
	commitment := packmanifest.ManifestCommitment{
		SHA256:           fmt.Sprintf("%x", state.Commitment.ManifestDigest),
		Size:             strconv.FormatUint(state.Commitment.ManifestSize, 10),
		BootstrapLocator: state.Commitment.BootstrapLocator,
	}
	expectedContext := packmanifest.Context{
		ChainID: "1439", SuiteDirectory: fmt.Sprintf("%#x", chain.directory),
		RepoID: fmt.Sprintf("%#x", repoID), RefName: refName,
		Commit: packmanifest.Commit{Algorithm: "sha1", OID: wantTip},
	}
	parsed, err := packmanifest.Parse(manifestBytes, expectedContext, commitment)
	if err != nil {
		t.Fatalf("manifest parse: %v", err)
	}
	bare := t.TempDir()
	localGit(t, bare, "init", "--bare", "--object-format=sha1")
	for _, entry := range parsed.Packs {
		size, err := packmanifest.Size(entry.Size, packmanifest.MaxPackBytes)
		if err != nil {
			t.Fatal(err)
		}
		file, err := packstore.ReadVerified(ctx, cloud, entry.Locations,
			packstore.Object{Kind: "packs", SHA256: entry.SHA256, Size: size}, t.TempDir())
		if err != nil {
			t.Fatalf("pack read: %v", err)
		}
		t.Cleanup(func() { _ = file.Close() })
		if err := (&gitio.Repo{GitDir: bare}).IndexVerified(ctx, file,
			packstore.Object{Kind: "packs", SHA256: entry.SHA256, Size: size}, wantTip); err != nil {
			t.Fatalf("index verified: %v", err)
		}
	}
	localGit(t, bare, "update-ref", refName, wantTip)
	localGit(t, bare, "fsck", "--strict")
}

func TestVerticalPublishCasAndColdClone(t *testing.T) {
	ctx := context.Background()
	chain := NewFakeChain(common.HexToAddress("0xa11ce000000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, err := chain.CreateRepository("vertical")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(chain)
	if err != nil {
		t.Fatal(err)
	}
	cloud := newMemStore()
	gitDir, tip := buildFixtureRepo(t)

	packFile, err := (&gitio.Repo{GitDir: gitDir}).PackFullHistory(ctx, tip, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packFile.Close() })

	manifest := fixtureManifest(t, chain, repoID, "refs/heads/main", tip, packFile.Source())
	commitment := publish(t, manifest, packFile, cloud)

	onchain := Commitment{
		ManifestDigest:   digestFromHex(commitment.SHA256),
		ManifestSize:     mustSize(commitment.Size),
		BootstrapLocator: commitment.BootstrapLocator,
	}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", tip, onchain, 0, [32]byte{}, false); err != nil {
		t.Fatalf("publish updateRef: %v", err)
	}
	state, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil || state.Commitment != onchain || state.Revision != 1 {
		t.Fatalf("published state: %v %+v", err, state)
	}
	coldClone(t, chain, client, cloud, repoID, "refs/heads/main", tip)
}

func TestVerticalCasConflictRetainsObjectsAndRecovers(t *testing.T) {
	ctx := context.Background()
	chain := NewFakeChain(common.HexToAddress("0xa11ce000000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, _ := chain.CreateRepository("conflict")
	client, _ := NewClient(chain)
	cloud := newMemStore()
	gitDir, tip := buildFixtureRepo(t)

	packFile, err := (&gitio.Repo{GitDir: gitDir}).PackFullHistory(ctx, tip, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packFile.Close() })
	manifest := fixtureManifest(t, chain, repoID, "refs/heads/main", tip, packFile.Source())
	commitment := publish(t, manifest, packFile, cloud)
	onchain := Commitment{
		ManifestDigest: digestFromHex(commitment.SHA256), ManifestSize: mustSize(commitment.Size),
		BootstrapLocator: commitment.BootstrapLocator,
	}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", tip, onchain, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}

	// A concurrent maintainer wins the next revision while we upload a second
	// manifest bound to the same commit.
	other := testCommitment(2)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", tip, other, 1, onchain.ManifestDigest, false); err != nil {
		t.Fatal(err)
	}

	// Upload succeeded before the conflict: objects are retained, not deleted.
	if cloud.ObjectCount() < 2 {
		t.Fatalf("objects after upload: %d", cloud.ObjectCount())
	}
	_, err = client.UpdateRef(ctx, repoID, "refs/heads/main", tip, onchain, 1, onchain.ManifestDigest, false)
	var mismatch *CommitmentMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("conflict error = %v, want CommitmentMismatch", err)
	}
	if mismatch.ActualRevision != 2 {
		t.Fatalf("actual revision = %d, want 2", mismatch.ActualRevision)
	}
	// Directed recovery: re-read state, republish the same commitment under CAS.
	fresh, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", tip, onchain, fresh.Revision, fresh.Commitment.ManifestDigest, false); err != nil {
		t.Fatalf("recovered publish: %v", err)
	}
	final, _ := client.GetRef(ctx, repoID, "refs/heads/main")
	if final.Revision != 3 || final.Commitment != onchain {
		t.Fatalf("final state: %+v", final)
	}
	coldClone(t, chain, client, cloud, repoID, "refs/heads/main", tip)
}

func TestVerticalForkRequiresFreshContextBoundManifest(t *testing.T) {
	ctx := context.Background()
	chain := NewFakeChain(common.HexToAddress("0xa11ce000000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, _ := chain.CreateRepository("fork-source")
	client, _ := NewClient(chain)
	cloud := newMemStore()
	gitDir, tip := buildFixtureRepo(t)
	packFile, err := (&gitio.Repo{GitDir: gitDir}).PackFullHistory(ctx, tip, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packFile.Close() })

	sourceManifest := fixtureManifest(t, chain, repoID, "refs/heads/main", tip, packFile.Source())
	commitment := publish(t, sourceManifest, packFile, cloud)
	onchain := Commitment{
		ManifestDigest: digestFromHex(commitment.SHA256), ManifestSize: mustSize(commitment.Size),
		BootstrapLocator: commitment.BootstrapLocator,
	}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", tip, onchain, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}

	forkID, err := chain.ForkRepository(repoID, "fork-target")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := chain.Refs(forkID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("fork carried commitments: %v", refs)
	}

	// Copying the source commitment to the fork fails client-side context
	// validation before any chain write: the manifest is bound to the source repo.
	sourceBytes := manifestBytesFor(t, cloud, commitment.BootstrapLocator)
	forkContext := packmanifest.Context{
		ChainID: "1439", SuiteDirectory: fmt.Sprintf("%#x", chain.directory),
		RepoID: fmt.Sprintf("%#x", forkID), RefName: "refs/heads/main",
		Commit: packmanifest.Commit{Algorithm: "sha1", OID: tip},
	}
	if _, err := packmanifest.Parse(sourceBytes, forkContext, commitment); err == nil {
		t.Fatal("source manifest accepted under fork context")
	}

	// The correct flow: regenerate a manifest bound to the fork repo/ref.
	forkManifest := fixtureManifest(t, chain, forkID, "refs/heads/main", tip, packFile.Source())
	forkCommitment := publish(t, forkManifest, packFile, cloud)
	forkOnchain := Commitment{
		ManifestDigest: digestFromHex(forkCommitment.SHA256), ManifestSize: mustSize(forkCommitment.Size),
		BootstrapLocator: forkCommitment.BootstrapLocator,
	}
	if _, err := client.UpdateRef(ctx, forkID, "refs/heads/main", tip, forkOnchain, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	coldClone(t, chain, client, cloud, forkID, "refs/heads/main", tip)
}

func manifestBytesFor(t *testing.T, cloud *memStore, locator string) []byte {
	t.Helper()
	body, err := cloud.Open(context.Background(), packmanifest.PackLocation{Provider: "cloudflare-r2", URL: locator}, packstore.Object{})
	if err != nil {
		t.Fatalf("open source manifest: %v", err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

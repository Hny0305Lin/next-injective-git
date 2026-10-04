package byos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain/successor"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/ethereum/go-ethereum/common"
)

const fixtureBase = "https://public.example.com"

// ---- fake chain adapter over successor.FakeChain ----

type fakeChain struct {
	inner *successor.FakeChain
	repos map[[32]byte]successor.RepositoryView
	names map[[32]byte][]string

	// interceptUpdate, when armed, advances the ref once right before the next
	// UpdateRef is applied, simulating a concurrent maintainer winning the race.
	interceptUpdate func()
}

func newFakeChain(t *testing.T) *fakeChain {
	t.Helper()
	inner := successor.NewFakeChain(common.HexToAddress("0xa11ce00000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	return &fakeChain{inner: inner, repos: map[[32]byte]successor.RepositoryView{}, names: map[[32]byte][]string{}}
}

func (f *fakeChain) create(t *testing.T, name string) [32]byte {
	t.Helper()
	id, err := f.inner.CreateRepository(name)
	if err != nil {
		t.Fatal(err)
	}
	owner := common.HexToAddress("0xa11ce00000000000000000000000000000000001")
	f.repos[id] = successor.RepositoryView{RepoID: id, OwnerHex: owner.Hex(), Name: name, DefaultBranch: "main"}
	return id
}

func (f *fakeChain) ChainIDDecimal() string { return "1439" }
func (f *fakeChain) DirectoryHex() string {
	return strings.ToLower(common.HexToAddress("0x4444000000000000000000000000000000000444").Hex())
}

func (f *fakeChain) ResolveRepository(ctx context.Context, owner, repo string) (successor.RepositoryView, bool, error) {
	for _, view := range f.repos {
		if strings.EqualFold(view.Name, repo) {
			return view, true, nil
		}
	}
	return successor.RepositoryView{}, false, errors.New("repository not found")
}

func (f *fakeChain) ListRefNames(ctx context.Context, repoID [32]byte) ([]string, error) {
	refs, err := f.inner.Refs(repoID)
	if err != nil {
		return nil, err
	}
	var names []string
	for name := range refs {
		names = append(names, name)
	}
	sortStrings(names)
	return names, nil
}

func (f *fakeChain) GetRef(ctx context.Context, repoID [32]byte, refName string) (successor.RefState, error) {
	return f.inner.GetRefPublic(repoID, refName)
}

func (f *fakeChain) UpdateRef(ctx context.Context, repoID [32]byte, refName, commitSHA string, commitment successor.Commitment, expectedRevision uint64, expectedDigest [32]byte, force bool) (string, error) {
	if hook := f.interceptUpdate; hook != nil {
		f.interceptUpdate = nil
		hook()
	}
	client, err := successor.NewClient(f.inner)
	if err != nil {
		return "", err
	}
	return client.UpdateRef(ctx, repoID, refName, commitSHA, commitment, expectedRevision, expectedDigest, force)
}

func (f *fakeChain) DeleteRef(ctx context.Context, repoID [32]byte, refName string) (string, error) {
	client, err := successor.NewClient(f.inner)
	if err != nil {
		return "", err
	}
	return client.DeleteRef(ctx, repoID, refName)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---- in-memory verified cloud factory ----

type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) Capabilities() packstore.Capabilities {
	return packstore.Capabilities{Provider: "cloudflare-r2", MaxSinglePut: packmanifest.MaxPackBytes, MaxObject: packmanifest.MaxPackBytes, RawReadback: true}
}

func (m *memStore) PutIfAbsent(_ context.Context, o packstore.Object, s packstore.Source) (packstore.Receipt, error) {
	if err := o.Validate(); err != nil {
		return packstore.Receipt{}, err
	}
	key, err := packmanifest.Key("igit-fixture", o.Kind, o.SHA256)
	if err != nil {
		return packstore.Receipt{}, err
	}
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return packstore.Receipt{}, packstore.Fail(packstore.Missing, "mem-source")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.objects[key]; ok {
		if fmt.Sprintf("%x", sum256(existing)) != o.SHA256 || int64(len(existing)) != o.Size {
			return packstore.Receipt{}, packstore.Fail(packstore.Conflict, "mem-conditional-put")
		}
		return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Reused: true, Verified: true}, nil
	}
	if fmt.Sprintf("%x", sum256(body)) != o.SHA256 || int64(len(body)) != o.Size {
		return packstore.Receipt{}, packstore.Fail(packstore.Integrity, "mem-source")
	}
	m.objects[key] = body
	return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Verified: true}, nil
}

func (m *memStore) VerifyStoredBytes(_ context.Context, r packstore.Receipt) error { return nil }

func (m *memStore) Open(_ context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	if !strings.HasPrefix(l.URL, fixtureBase+"/") {
		return nil, packstore.Fail(packstore.Auth, "mem-base")
	}
	key := strings.TrimPrefix(l.URL, fixtureBase+"/")
	m.mu.Lock()
	body, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return nil, packstore.Fail(packstore.Missing, "mem-open")
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

type memFactory struct{ store *memStore }

func (memFactory) Writer(successor.RepositoryView) (packstore.Writer, string, string, string, error) {
	return nil, "", "", "", errors.New("mem factory writer must be wrapped")
}
func (memFactory) Readers() packstore.Readers { return packstore.Readers{} }
func (memFactory) ProviderName() string       { return "cloudflare-r2" }

// rwFactory returns the mem store for both writer and reader roles.
type rwFactory struct{ store *memStore }

func (f rwFactory) Writer(successor.RepositoryView) (packstore.Writer, string, string, string, error) {
	return f.store, fixtureBase, "igit-fixture", "cloudflare-r2", nil
}
func (f rwFactory) Readers() packstore.Readers {
	return packstore.Readers{Public: map[string]packstore.Reader{"cloudflare-r2": f.store}}
}
func (f rwFactory) ProviderName() string { return "cloudflare-r2" }

func sum256(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

// ---- real git fixture ----

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

func buildRepo(t *testing.T, commits ...string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	localGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	if len(commits) == 0 {
		commits = []string{"first"}
	}
	for _, msg := range commits {
		if err := os.WriteFile(filepath.Join(dir, "history.txt"), []byte(msg), 0o600); err != nil {
			t.Fatal(err)
		}
		localGit(t, dir, "add", ".")
		localGit(t, dir, "commit", "-m", msg)
	}
	return filepath.Join(dir, ".git"), localGit(t, dir, "rev-parse", "HEAD")
}

func newService(t *testing.T, chain *fakeChain, store *memStore, git *gitio.Repo) *Service {
	t.Helper()
	service, err := NewService(chain, git, rwFactory{store}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestPushListFetchRoundTrip(t *testing.T) {
	ctx := context.Background()
	chain := newFakeChain(t)
	chain.create(t, "demo")
	store := newMemStore()
	gitDir, tip := buildRepo(t, "first", "second")
	service := newService(t, chain, store, &gitio.Repo{GitDir: gitDir})

	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatalf("push: %v", err)
	}
	listings, defaultBranch, err := service.ListRefs(ctx, "inj1owner", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 1 || listings[0].CommitOID != tip || listings[0].Revision != 1 || defaultBranch != "main" {
		t.Fatalf("listings: %+v branch %q", listings, defaultBranch)
	}

	// cold fetch into a bare repository
	bare := t.TempDir()
	localGit(t, bare, "init", "--bare", "--object-format=sha1")
	fetchService := newService(t, chain, store, &gitio.Repo{GitDir: bare})
	if err := fetchService.FetchRef(ctx, "inj1owner", "demo", "refs/heads/main", tip, t.TempDir()); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	localGit(t, bare, "update-ref", "refs/heads/main", tip)
	if got := localGit(t, bare, "show", tip+":history.txt"); got != "second" {
		t.Fatalf("fetched content = %q", got)
	}
	localGit(t, bare, "fsck", "--strict")
}

func TestPushUpdateBumpsRevisionWithFreshCAS(t *testing.T) {
	ctx := context.Background()
	chain := newFakeChain(t)
	chain.create(t, "demo")
	store := newMemStore()
	gitDir, tip1 := buildRepo(t, "first")
	service := newService(t, chain, store, &gitio.Repo{GitDir: gitDir})
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// new commit on the same repo
	localGit(t, filepath.Dir(gitDir), "commit", "--allow-empty", "-m", "second")
	tip2 := localGit(t, filepath.Dir(gitDir), "rev-parse", "HEAD")
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatalf("second push: %v", err)
	}
	state, err := successorClient(chain).GetRef(ctx, chain.firstRepoID(t), "refs/heads/main")
	if err != nil || state.Revision != 2 {
		t.Fatalf("state after update: %v %+v", err, state)
	}
	if state.Commitment.ManifestSize == 0 || state.Commitment.BootstrapLocator == "" {
		t.Fatalf("commitment incomplete: %+v", state.Commitment)
	}
	_ = tip1
	_ = tip2
}

func TestPushConflictRetainsObjectsAndDirectedRetry(t *testing.T) {
	ctx := context.Background()
	chain := newFakeChain(t)
	repoID := chain.create(t, "demo")
	store := newMemStore()
	gitDir, _ := buildRepo(t, "first")
	service := newService(t, chain, store, &gitio.Repo{GitDir: gitDir})
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	objectsAfterFirst := store.count()

	// A concurrent maintainer lands between our fresh read and our transaction.
	chain.interceptUpdate = func() { concurrentAdvance(t, chain, repoID) }
	localGit(t, filepath.Dir(gitDir), "commit", "--allow-empty", "-m", "second")
	err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir())
	var mismatch *successor.CommitmentMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("stale push error = %v, want CommitmentMismatch with retained objects", err)
	}
	if store.count() <= objectsAfterFirst {
		t.Fatalf("objects after conflicted upload: %d (want > %d)", store.count(), objectsAfterFirst)
	}
	// Directed retry with fresh state succeeds and continues the revision.
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	state, _ := successorClient(chain).GetRef(ctx, repoID, "refs/heads/main")
	if state.Revision != 3 {
		t.Fatalf("revision after retry = %d, want 3", state.Revision)
	}
}

func TestDeleteTombstonesAndRelistIsEmpty(t *testing.T) {
	ctx := context.Background()
	chain := newFakeChain(t)
	chain.create(t, "demo")
	store := newMemStore()
	gitDir, _ := buildRepo(t, "first")
	service := newService(t, chain, store, &gitio.Repo{GitDir: gitDir})
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := service.PushRef(ctx, "inj1owner", "demo", "", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	listings, _, err := service.ListRefs(ctx, "inj1owner", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 0 {
		t.Fatalf("refs after delete: %+v", listings)
	}
}

func TestFetchRejectsMovedRefAndCorruptManifest(t *testing.T) {
	ctx := context.Background()
	chain := newFakeChain(t)
	repoID := chain.create(t, "demo")
	store := newMemStore()
	gitDir, tip := buildRepo(t, "first")
	service := newService(t, chain, store, &gitio.Repo{GitDir: gitDir})
	if err := service.PushRef(ctx, "inj1owner", "demo", "refs/heads/main", "refs/heads/main", false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	bare := t.TempDir()
	localGit(t, bare, "init", "--bare", "--object-format=sha1")
	fetcher := newService(t, chain, store, &gitio.Repo{GitDir: bare})
	wrongOID := strings.Repeat("f", 40)
	if err := fetcher.FetchRef(ctx, "inj1owner", "demo", "refs/heads/main", wrongOID, t.TempDir()); err == nil {
		t.Fatal("fetch accepted a moved/mismatched OID")
	}
	// tamper the manifest bytes in the fake cloud: chain digest no longer matches
	store.mu.Lock()
	for key, body := range store.objects {
		if strings.HasSuffix(key, ".json") {
			tampered := append([]byte{}, body...)
			tampered[len(tampered)-2] = 'x'
			store.objects[key] = tampered
		}
	}
	store.mu.Unlock()
	if err := fetcher.FetchRef(ctx, "inj1owner", "demo", "refs/heads/main", tip, t.TempDir()); err == nil {
		t.Fatal("fetch accepted a tampered manifest")
	}
	_ = repoID
}

// helpers

func successorClient(chain *fakeChain) *successor.Client {
	client, err := successor.NewClient(chain.inner)
	if err != nil {
		panic(err)
	}
	return client
}

func (f *fakeChain) firstRepoID(t *testing.T) [32]byte {
	t.Helper()
	for id := range f.repos {
		return id
	}
	t.Fatal("no repository")
	return [32]byte{}
}

func concurrentAdvance(t *testing.T, chain *fakeChain, repoID [32]byte) successor.RefState {
	t.Helper()
	client := successorClient(chain)
	ctx := context.Background()
	state, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	digest := [32]byte{}
	for i := range digest {
		digest[i] = byte(i + 7)
	}
	commitment := successor.Commitment{ManifestDigest: digest, ManifestSize: 900, BootstrapLocator: fixtureBase + "/manifests/sha256/" + strings.Repeat("ab", 32) + ".json"}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), commitment, state.Revision, state.Commitment.ManifestDigest, false); err != nil {
		t.Fatal(err)
	}
	next, _ := client.GetRef(ctx, repoID, "refs/heads/main")
	return next
}

func testCommitmentFromDigest(digest [32]byte) successor.Commitment {
	return successor.Commitment{ManifestDigest: digest, ManifestSize: 778, BootstrapLocator: fixtureBase + "/m.json"}
}

func successorCommitment(t *testing.T, chain *fakeChain, repoID [32]byte) [32]byte {
	t.Helper()
	state, err := successorClient(chain).GetRef(context.Background(), repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	return state.Commitment.ManifestDigest
}

func sha256Hex(b []byte) string {
	sum := sha256Of(b)
	return fmt.Sprintf("%x", sum)
}
func sha256Of(b []byte) [32]byte {
	return sha256.Sum256(b)
}

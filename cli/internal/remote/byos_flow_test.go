package remote

import (
	"bytes"
	"context"
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

	"github.com/Hny0305Lin/next-injective-git/cli/internal/byos"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain/successor"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/ethereum/go-ethereum/common"
)

const byosFixtureBase = "https://public.example.com"

type byosFakeChain struct {
	inner *successor.FakeChain
	view  successor.RepositoryView
}

func (f *byosFakeChain) ChainIDDecimal() string { return "1439" }
func (f *byosFakeChain) DirectoryHex() string   { return strings.ToLower(f.view2Hex()) }
func (f *byosFakeChain) view2Hex() string       { return "0x4444000000000000000000000000000000000444" }

func (f *byosFakeChain) ResolveRepository(ctx context.Context, owner, repo string) (successor.RepositoryView, bool, error) {
	if strings.EqualFold(repo, f.view.Name) {
		return f.view, true, nil
	}
	return successor.RepositoryView{}, false, errors.New("repository not found")
}

func (f *byosFakeChain) ListRefNames(ctx context.Context, repoID [32]byte) ([]string, error) {
	refs, err := f.inner.Refs(repoID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	return names, nil
}

func (f *byosFakeChain) GetRef(ctx context.Context, repoID [32]byte, refName string) (successor.RefState, error) {
	return f.inner.GetRefPublic(repoID, refName)
}

func (f *byosFakeChain) UpdateRef(ctx context.Context, repoID [32]byte, refName, commitSHA string, commitment successor.Commitment, expectedRevision uint64, expectedDigest [32]byte, force bool) (string, error) {
	client, err := successor.NewClient(f.inner)
	if err != nil {
		return "", err
	}
	return client.UpdateRef(ctx, repoID, refName, commitSHA, commitment, expectedRevision, expectedDigest, force)
}

func (f *byosFakeChain) DeleteRef(ctx context.Context, repoID [32]byte, refName string) (string, error) {
	client, err := successor.NewClient(f.inner)
	if err != nil {
		return "", err
	}
	return client.DeleteRef(ctx, repoID, refName)
}

type byosMemStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (m *byosMemStore) Capabilities() packstore.Capabilities {
	return packstore.Capabilities{Provider: "cloudflare-r2", MaxSinglePut: packmanifest.MaxPackBytes, MaxObject: packmanifest.MaxPackBytes, RawReadback: true}
}

func (m *byosMemStore) PutIfAbsent(_ context.Context, o packstore.Object, s packstore.Source) (packstore.Receipt, error) {
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
	if _, exists := m.objects[key]; exists {
		return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Reused: true, Verified: true}, nil
	}
	m.objects[key] = body
	return packstore.Receipt{Provider: "cloudflare-r2", Key: key, Object: o, Verified: true}, nil
}

func (m *byosMemStore) VerifyStoredBytes(context.Context, packstore.Receipt) error { return nil }

func (m *byosMemStore) Open(_ context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	if !strings.HasPrefix(l.URL, byosFixtureBase+"/") {
		return nil, packstore.Fail(packstore.Auth, "mem-base")
	}
	key := strings.TrimPrefix(l.URL, byosFixtureBase+"/")
	m.mu.Lock()
	body, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return nil, packstore.Fail(packstore.Missing, "mem-open")
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

type byosMemFactory struct{ store *byosMemStore }

func (f byosMemFactory) Writer(successor.RepositoryView) (packstore.Writer, string, string, string, error) {
	return f.store, byosFixtureBase, "igit-fixture", "cloudflare-r2", nil
}
func (f byosMemFactory) Readers() packstore.Readers {
	return packstore.Readers{Public: map[string]packstore.Reader{"cloudflare-r2": f.store}}
}
func (f byosMemFactory) ProviderName() string { return "cloudflare-r2" }

func byosLocalGit(t *testing.T, dir string, args ...string) string {
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

// TestHelperByosConversation drives the full remote-helper protocol over the
// BYOS path with a real local repository: push, list, cold fetch.
func TestHelperByosConversation(t *testing.T) {
	dir := t.TempDir()
	byosLocalGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "history.txt"), []byte("hello byos"), 0o600); err != nil {
		t.Fatal(err)
	}
	byosLocalGit(t, dir, "add", ".")
	byosLocalGit(t, dir, "commit", "-m", "first")
	tip := byosLocalGit(t, dir, "rev-parse", "HEAD")

	inner := successor.NewFakeChain(common.HexToAddress("0xa11ce00000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, err := inner.CreateRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	chain := &byosFakeChain{inner: inner, view: successor.RepositoryView{RepoID: repoID, OwnerHex: "0xa11ce00000000000000000000000000000000001", Name: "demo", DefaultBranch: "main"}}
	store := &byosMemStore{objects: map[string][]byte{}}
	service, err := byos.NewService(chain, &gitio.Repo{GitDir: filepath.Join(dir, ".git")}, byosMemFactory{store}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	h := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil, strings.NewReader(""), &out, io.Discard)
	h.SetByosStorage(service)

	// 1) push
	out.Reset()
	h2 := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil,
		strings.NewReader("list for-push\npush refs/heads/main:refs/heads/main\n\n"), &out, io.Discard)
	h2.SetByosStorage(service)
	if err := h2.Run(); err != nil {
		t.Fatalf("push conversation: %v", err)
	}
	if !strings.Contains(out.String(), "ok refs/heads/main") {
		t.Fatalf("push output: %q", out.String())
	}

	// 2) list shows the advertised OID (learned from the verified manifest)
	out.Reset()
	h3 := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil,
		strings.NewReader("list\n"), &out, io.Discard)
	h3.SetByosStorage(service)
	if err := h3.Run(); err != nil {
		t.Fatalf("list conversation: %v", err)
	}
	if !strings.Contains(out.String(), tip+" refs/heads/main") || !strings.Contains(out.String(), "@refs/heads/main HEAD") {
		t.Fatalf("list output: %q", out.String())
	}

	// 3) cold fetch into a bare repo through the helper protocol
	bare := t.TempDir()
	byosLocalGit(t, bare, "init", "--bare", "--object-format=sha1")
	coldService, err := byos.NewService(chain, &gitio.Repo{GitDir: bare}, byosMemFactory{store}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	h4 := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil,
		strings.NewReader(fmt.Sprintf("fetch %s refs/heads/main\n\n", tip)), &out, io.Discard)
	h4.SetByosStorage(coldService)
	if err := h4.Run(); err != nil {
		t.Fatalf("fetch conversation: %v", err)
	}
	byosLocalGit(t, bare, "update-ref", "refs/heads/main", tip)
	if got := byosLocalGit(t, bare, "show", tip+":history.txt"); got != "hello byos" {
		t.Fatalf("cold-fetch content: %q", got)
	}
	byosLocalGit(t, bare, "fsck", "--strict")
}

// TestHelperByosIncrementalConversation drives push, an incremental push, list
// and a cold fetch of a schema-2 chain through the remote-helper protocol.
func TestHelperByosIncrementalConversation(t *testing.T) {
	dir := t.TempDir()
	byosLocalGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "history.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	byosLocalGit(t, dir, "add", ".")
	byosLocalGit(t, dir, "commit", "-m", "first")

	inner := successor.NewFakeChain(common.HexToAddress("0xa11ce00000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, err := inner.CreateRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	chain := &byosFakeChain{inner: inner, view: successor.RepositoryView{RepoID: repoID, OwnerHex: "0xa11ce00000000000000000000000000000000001", Name: "demo", DefaultBranch: "main"}}
	store := &byosMemStore{objects: map[string][]byte{}}
	service, err := byos.NewService(chain, &gitio.Repo{GitDir: filepath.Join(dir, ".git")}, byosMemFactory{store}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	runHelper := func(input string) string {
		out.Reset()
		h := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil, strings.NewReader(input), &out, io.Discard)
		h.SetByosStorage(service)
		if err := h.Run(); err != nil {
			t.Fatalf("conversation %q: %v", input, err)
		}
		return out.String()
	}

	// 1) initial push
	if got := runHelper("list for-push\npush refs/heads/main:refs/heads/main\n\n"); !strings.Contains(got, "ok refs/heads/main") {
		t.Fatalf("push output: %q", got)
	}
	objectsAfterFull := len(storeSnapshot(store))

	// 2) second commit → incremental push through the same protocol
	if err := os.WriteFile(filepath.Join(dir, "history.txt"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	byosLocalGit(t, dir, "commit", "-am", "second")
	tip2 := byosLocalGit(t, dir, "rev-parse", "HEAD")
	if got := runHelper("list for-push\npush refs/heads/main:refs/heads/main\n\n"); !strings.Contains(got, "ok refs/heads/main") {
		t.Fatalf("incremental push output: %q", got)
	}
	after := storeSnapshot(store)
	if len(after) != objectsAfterFull+2 {
		t.Fatalf("objects after incremental push = %d (full push left %d); only the new pack and manifest may be uploaded", len(after), objectsAfterFull)
	}

	// 3) list advertises the incremental tip
	if got := runHelper("list\n"); !strings.Contains(got, tip2+" refs/heads/main") {
		t.Fatalf("list output: %q", got)
	}

	// 4) cold fetch of the chain into a bare repository
	bare := t.TempDir()
	byosLocalGit(t, bare, "init", "--bare", "--object-format=sha1")
	coldService, err := byos.NewService(chain, &gitio.Repo{GitDir: bare}, byosMemFactory{store}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	h4 := NewHelper(RepoURL{Owner: "inj1owner", Repo: "demo"}, nil, nil, nil, nil, nil,
		strings.NewReader(fmt.Sprintf("fetch %s refs/heads/main\n\n", tip2)), &out, io.Discard)
	h4.SetByosStorage(coldService)
	if err := h4.Run(); err != nil {
		t.Fatalf("cold fetch conversation: %v", err)
	}
	byosLocalGit(t, bare, "update-ref", "refs/heads/main", tip2)
	if got := byosLocalGit(t, bare, "show", tip2+":history.txt"); got != "two" {
		t.Fatalf("cold-fetch content: %q", got)
	}
	byosLocalGit(t, bare, "fsck", "--strict")
}

func storeSnapshot(m *byosMemStore) map[string][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := make(map[string][]byte, len(m.objects))
	for key, body := range m.objects {
		snapshot[key] = body
	}
	return snapshot
}

package remote

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/replication"
)

type fakeChain struct {
	updateErr error
	updates   int
}

func (f *fakeChain) ListRefs(string, string) ([]chain.RefInfo, error)            { return nil, nil }
func (f *fakeChain) RepoInfo(string, string) (*chain.RepoInfo, error)            { return nil, nil }
func (f *fakeChain) ResolveRef(string, string, string) (string, []string, error) { return "", nil, nil }
func (f *fakeChain) DeleteRef(string, string, string) error                      { return nil }
func (f *fakeChain) UpdateRef(string, string, string, string, []string, string, bool) error {
	f.updates++
	return f.updateErr
}

type fakeIPFS struct{ add, swarm, gc, get int }

func (f *fakeIPFS) AddTemporary(string, io.Reader) (string, error) { f.add++; return "bafy-test", nil }
func (f *fakeIPFS) GetFromGateways(string) (io.ReadCloser, error) {
	f.get++
	return io.NopCloser(strings.NewReader("pack")), nil
}
func (f *fakeIPFS) SwarmConnect(string) error { f.swarm++; return nil }
func (f *fakeIPFS) GC() error                 { f.gc++; return nil }

type fakeGit struct{}

func (fakeGit) ResolveRef(string) string { return "0123456789abcdef0123456789abcdef01234567" }
func (fakeGit) PackObjects(string, []string) ([]byte, error) {
	return []byte{'P', 'A', 'C', 'K', 0, 0, 0, 2, 0, 0, 0, 1}, nil
}
func (fakeGit) IndexPack(io.Reader) error   { return nil }
func (fakeGit) ScanSecrets(string) []string { return nil }

type fakeAuthorizer struct {
	confirmErr error
	authorized int
}

func (f *fakeAuthorizer) Authorize(replication.Request) (replication.Confirmer, error) {
	f.authorized++
	return fakeConfirmer{f.confirmErr}, nil
}

type fakeConfirmer struct{ err error }

func (f fakeConfirmer) Confirm(replication.Request) (replication.Response, error) {
	return replication.Response{CID: "bafy-test", Pinned: f.err == nil}, f.err
}

func newPushHelper(c *fakeChain, i *fakeIPFS, r replication.Authorizer) *Helper {
	return NewHelper(RepoURL{Owner: "inj1owner", Repo: "repo"}, c, i, r, []string{"/dns4/us.example/tcp/4001/p2p/peer"}, fakeGit{}, strings.NewReader(""), io.Discard, io.Discard)
}

func TestPushGCOnlyAfterUSPinAndChainUpdate(t *testing.T) {
	c, i, r := &fakeChain{}, &fakeIPFS{}, &fakeAuthorizer{}
	if err := newPushHelper(c, i, r).pushOne(pushSpec{src: "main", dst: "refs/heads/main"}); err != nil {
		t.Fatal(err)
	}
	if c.updates != 1 || i.add != 1 || i.swarm != 1 || i.gc != 1 {
		t.Fatalf("updates=%d add=%d swarm=%d gc=%d", c.updates, i.add, i.swarm, i.gc)
	}
}

func TestPushDoesNotUpdateOrGCAfterUSPinFailure(t *testing.T) {
	c, i, r := &fakeChain{}, &fakeIPFS{}, &fakeAuthorizer{confirmErr: errors.New("pin failed")}
	if err := newPushHelper(c, i, r).pushOne(pushSpec{src: "main", dst: "refs/heads/main"}); err == nil {
		t.Fatal("expected replication failure")
	}
	if c.updates != 0 || i.gc != 0 {
		t.Fatalf("updates=%d gc=%d", c.updates, i.gc)
	}
}

func TestPushDoesNotGCAfterChainFailure(t *testing.T) {
	c, i, r := &fakeChain{updateErr: errors.New("tx rejected")}, &fakeIPFS{}, &fakeAuthorizer{}
	if err := newPushHelper(c, i, r).pushOne(pushSpec{src: "main", dst: "refs/heads/main"}); err == nil {
		t.Fatal("expected chain failure")
	}
	if c.updates != 1 || i.gc != 0 {
		t.Fatalf("updates=%d gc=%d", c.updates, i.gc)
	}
}

func TestFetchUsesGatewayClient(t *testing.T) {
	c, i := &fakeChain{}, &fakeIPFS{}
	h := newPushHelper(c, i, &fakeAuthorizer{})
	body, err := h.fetchPack("ipfs://bafy-test")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != "pack" || i.get != 1 {
		t.Fatalf("got=%q get=%d", bytes.TrimSpace(got), i.get)
	}
}

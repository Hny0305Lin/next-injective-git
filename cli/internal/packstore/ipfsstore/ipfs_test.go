package ipfsstore

import (
	"bytes"
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"io"
	"strings"
	"testing"
)

var _ Client = (*ipfs.Client)(nil)
var _ packstore.Writer = Store{}

type fakeIPFS struct {
	bytes     []byte
	requested []string
}

func (f *fakeIPFS) AddTemporaryContext(_ context.Context, _ string, r io.Reader) (string, error) {
	b, e := io.ReadAll(r)
	f.bytes = b
	return "b" + strings.Repeat("a", 58), e
}
func (f *fakeIPFS) GetFromGatewaysContext(_ context.Context, cid string) (io.ReadCloser, error) {
	f.requested = append(f.requested, cid)
	return io.NopCloser(bytes.NewReader(f.bytes)), nil
}
func TestAdapterAndLegacyOrder(t *testing.T) {
	ctx := context.Background()
	f := &fakeIPFS{}
	s := Store{f}
	file, e := packstore.Spool(ctx, t.TempDir(), io.NopCloser(strings.NewReader("fixture")), 1024)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	src := file.Source()
	o := packstore.Object{Kind: "packs", SHA256: src.SHA256, Size: src.Size}
	r, e := s.PutIfAbsent(ctx, o, src)
	if e != nil || !r.Verified {
		t.Fatal(r, e)
	}
	f.bytes = []byte("tampered")
	if e = s.VerifyStoredBytes(ctx, r); e == nil {
		t.Fatal("corruption accepted")
	}
	f.requested = nil
	uris := []string{"ipfs://b" + strings.Repeat("a", 58), "ipfs://b" + strings.Repeat("b", 58)}
	for _, uri := range uris {
		body, e := s.OpenLegacy(ctx, uri)
		if e != nil {
			t.Fatal(e)
		}
		body.Close()
	}
	if len(f.requested) != 2 || f.requested[0] != strings.TrimPrefix(uris[0], "ipfs://") || f.requested[1] != strings.TrimPrefix(uris[1], "ipfs://") {
		t.Fatal("legacy reordered")
	}
	if _, e = s.Open(ctx, packmanifest.PackLocation{Provider: "aws-s3", URL: "https://storage.example.com/a"}, o); e == nil {
		t.Fatal("wrong provider")
	}
}

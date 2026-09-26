package s3store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectedRecoveryAndMultipartRestart(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, bytes.Repeat([]byte("x"), int(SinglePutLimit+1)))
	path := filepath.Join(t.TempDir(), "receipt.json")
	writes := 0
	checkpoint := func(r packstore.Receipt) error { writes++; return s.SaveReceipt(path, r) }
	f.completeStatuses = []int{500}
	prior, e := s.PutRecoverable(context.Background(), o, src, checkpoint)
	if e == nil || prior.UploadID == "" || len(prior.CompletedParts) != 3 || writes < 5 {
		t.Fatal(prior, e, writes)
	}
	loaded, e := s.LoadReceipt(path)
	if e != nil || loaded.UploadID != prior.UploadID {
		t.Fatal(loaded, e)
	}
	raw, _ := os.ReadFile(path)
	for _, secret := range []string{"FAKE_SECRET", "FAKE_SESSION", "Authorization", "X-Amz-Signature", src.Path} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("journal leak", secret)
		}
	}
	recovered, e := s.Recover(context.Background(), loaded, src, checkpoint)
	if e != nil || !recovered.Verified || len(recovered.RetainedUploadIDs) != 1 || f.created != 2 || f.aborts != 0 {
		t.Fatal(recovered, e)
	}
	count := len(f.requests)
	_, e = s.Recover(context.Background(), recovered, src, checkpoint)
	if e != nil || len(f.requests) != count+1 {
		t.Fatal("recovery did not check final object first", e)
	}
}
func TestCheckpointFailureStopsUpload(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, []byte("data"))
	_, e := s.PutRecoverable(context.Background(), o, src, func(packstore.Receipt) error { return errors.New("SECRET") })
	code(t, e, packstore.Uncertain)
	if len(f.requests) != 0 {
		t.Fatal("IO after failed journal")
	}
}
func TestPrepareManifestFailureRetainsPack(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, bytes.Repeat([]byte("a"), 64))
	key, _ := s.key(o)
	m := packmanifest.PackManifest{Schema: "igit.pack-manifest", SchemaVersion: 1, Context: packmanifest.Context{ChainID: "1776", SuiteDirectory: "0x" + strings.Repeat("1", 40), RepoID: "0x" + strings.Repeat("2", 64), RefName: "refs/heads/main", Commit: packmanifest.Commit{Algorithm: "sha1", OID: strings.Repeat("3", 40)}}, Packs: []packmanifest.PackEntry{{Sequence: 0, SHA256: o.SHA256, Size: "64", Format: "git-pack", PackVersion: 2, DependsOn: []string{}, Locations: []packmanifest.PackLocation{{Provider: "aws-s3", URL: "https://public.example.com/" + key}}}}}
	f.putStatuses = []int{200, 403}
	r, e := packstore.Prepare(context.Background(), s, m, []packstore.Source{src}, "https://public.example.com", "fixture", t.TempDir())
	if e == nil || r.Commitment != nil || len(r.Receipts) != 2 || !r.Receipts[0].Verified || len(f.objects) != 1 {
		t.Fatal(r, e)
	}
	r, e = packstore.Prepare(context.Background(), s, m, []packstore.Source{src}, "https://public.example.com", "fixture", t.TempDir())
	if e != nil || r.Commitment == nil || !r.Receipts[0].Reused || len(f.objects) != 2 || f.aborts != 0 {
		t.Fatal(r, e)
	}
	// The boundary exposes no ref write; even after a caller's future CAS failure
	// the returned receipts and objects remain available for directed recovery.
	b, e := json.Marshal(r)
	if e != nil || bytes.Contains(b, []byte("FAKE_SECRET")) {
		t.Fatal("receipt serialization")
	}
}

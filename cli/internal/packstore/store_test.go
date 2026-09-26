package packstore

import (
	"bytes"
	"context"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

type fakeReader struct {
	bodies []string
	calls  int
	closed int
}
type closer struct {
	io.Reader
	close func()
}

func (c closer) Close() error { c.close(); return nil }
func (f *fakeReader) Open(context.Context, packmanifest.PackLocation, Object) (io.ReadCloser, error) {
	b := f.bodies[f.calls]
	f.calls++
	return closer{bytes.NewBufferString(b), func() { f.closed++ }}, nil
}
func TestVerifiedFilesAndFallback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b := []byte("expected bytes")
	o := Object{Kind: "packs", SHA256: packmanifest.Digest(b), Size: int64(len(b))}
	locations := []packmanifest.PackLocation{{Provider: "aws-s3", URL: "https://storage.example.com/one"}, {Provider: "cloudflare-r2", URL: "https://storage.example.com/two"}}
	f := &fakeReader{bodies: []string{"corrupted", string(b)}}
	file, e := ReadVerified(ctx, f, locations, o, dir)
	if e != nil {
		t.Fatal(e)
	}
	if f.calls != 2 || f.closed != 2 {
		t.Fatal("fallback/response not closed", f)
	}
	r, e := file.Open()
	if e != nil {
		t.Fatal("file not reopened on Windows", e)
	}
	r.Close()
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("temp file leak")
	}
	for _, body := range []string{string(b[:len(b)-1]), string(b) + "x", "different bytes"} {
		f = &fakeReader{bodies: []string{body}}
		file, e = ReadVerified(ctx, f, locations[:1], o, dir)
		if e == nil || file != nil {
			t.Fatal("unverified bytes escaped")
		}
		files, _ := os.ReadDir(dir)
		if len(files) != 0 || f.closed != 1 {
			t.Fatal("failure leak")
		}
	}
}

type blockingBody struct {
	done chan struct{}
	once sync.Once
}

func (b *blockingBody) Read([]byte) (int, error) { <-b.done; return 0, errors.New("closed") }
func (b *blockingBody) Close() error             { b.once.Do(func() { close(b.done) }); return nil }
func TestCancellationClosesBlockedBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	b := &blockingBody{done: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, e := Spool(ctx, dir, b, 1024); done <- e }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, &Error{Code: Canceled}) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not close body")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("temp leaked")
	}
}
func TestSourceTamperingAndGenerationBounds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, e := Generate(ctx, dir, 32, func(w io.Writer) error { _, e := w.Write(bytes.Repeat([]byte{1}, 33)); return e })
	if e == nil || f != nil {
		t.Fatal("size limit")
	}
	f, e = Generate(ctx, dir, 32, func(w io.Writer) error { _, e := w.Write([]byte("original")); return e })
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s := f.Source()
	if e = os.WriteFile(s.Path, []byte("modified"), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := CheckSource(ctx, s, Object{Kind: "packs", SHA256: s.SHA256, Size: s.Size})
	if e == nil || r != nil {
		t.Fatal("source mutation accepted")
	}
}

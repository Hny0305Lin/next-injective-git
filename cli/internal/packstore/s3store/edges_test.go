package s3store

import (
	"context"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
	"github.com/aws/aws-sdk-go-v2/aws"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReaderCannotWriteAndMissingMapping(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	s.writable = false
	src, o := source(t, []byte("fixture"))
	_, e := s.PutIfAbsent(context.Background(), o, src)
	code(t, e, packstore.Auth)
	if len(f.requests) != 0 {
		t.Fatal("reader wrote")
	}
	readers := packstore.Readers{Public: map[string]packstore.Reader{"aws-s3": s}}
	_, e = readers.Open(context.Background(), packmanifest.PackLocation{Provider: "aws-s3", Reader: "missing"}, o)
	code(t, e, packstore.Auth)
}
func TestTimeoutAndEarlyBodyClose(t *testing.T) {
	s, _ := fakeStore(t, "aws-s3")
	src, o := source(t, []byte("fixture"))
	attempts := 0
	blocked := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		<-r.Context().Done()
		return nil, errors.New("SECRET signed-url")
	})}
	creds := s.client.Options().Credentials
	st, e := newStore(s.profile, creds, blocked, blocked)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, e = st.PutIfAbsent(ctx, o, src)
	code(t, e, packstore.Canceled)
	if attempts != 1 {
		t.Fatal("retried canceled request")
	}
	e = packstore.Check(context.Background(), &earlyBody{}, o)
	code(t, e, packstore.Integrity)
}

type earlyBody struct{}

func (*earlyBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestSourceMismatchDoesNotWrite(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, []byte("fixture"))
	os.WriteFile(src.Path, []byte("mutated"), 0600)
	_, e := s.PutIfAbsent(context.Background(), o, src)
	code(t, e, packstore.Integrity)
	if len(f.requests) != 0 {
		t.Fatal("mutated source uploaded")
	}
}
func TestMultipartCheckpointFailureRetainsID(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, []byte(strings.Repeat("x", int(SinglePutLimit+1))))
	calls := 0
	r, e := s.PutRecoverable(context.Background(), o, src, func(packstore.Receipt) error {
		calls++
		if calls >= 2 {
			return errors.New("disk full")
		}
		return nil
	})
	code(t, e, packstore.Uncertain)
	if r.UploadID == "" || len(f.requests) != 1 || f.aborts != 0 {
		t.Fatal("lost session or IO after checkpoint failure", r)
	}
}

func TestMissingCredentialsNeverDiscoversOrRequests(t *testing.T) {
	s, _ := fakeStore(t, "aws-s3")
	calls := 0
	transport := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected network") })}
	creds := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) { return aws.Credentials{}, storageconfig.ErrConfig })
	store, e := newStore(s.profile, creds, transport, transport)
	if e != nil {
		t.Fatal(e)
	}
	src, o := source(t, []byte("fixture"))
	_, e = store.PutIfAbsent(context.Background(), o, src)
	code(t, e, packstore.Auth)
	if calls != 0 {
		t.Fatal("network request despite missing credentials")
	}
}

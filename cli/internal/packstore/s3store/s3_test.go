package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
	"github.com/aws/aws-sdk-go-v2/aws"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeCloud struct {
	mu               sync.Mutex
	objects          map[string][]byte
	parts            map[string]map[int][]byte
	requests         []string
	headers          []http.Header
	putStatuses      []int
	getStatuses      []int
	completeStatuses []int
	created          int
	aborts           int
	getTransform     func([]byte) []byte
	cors             string
	encoding         string
}

func (f *fakeCloud) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	f.headers = append(f.headers, r.Header.Clone())
	if r.Method == "DELETE" {
		f.aborts++
		http.Error(w, "forbidden", 500)
		return
	}
	fail := func(code int) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(code)
		fmt.Fprint(w, "<Error><Code>Failure</Code><Message>DO-NOT-LEAK-SECRET https://example.com/?X-Amz-Signature=bad</Message></Error>")
	}
	key := r.URL.Path
	q := r.URL.Query()
	if r.Method == "POST" && q.Has("uploads") {
		f.created++
		id := fmt.Sprintf("session-%d", f.created)
		f.parts[id] = map[int][]byte{}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", id)
		return
	}
	if r.Method == "PUT" && q.Get("uploadId") != "" {
		var n int
		fmt.Sscan(q.Get("partNumber"), &n)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			fail(500)
			return
		}
		f.parts[q.Get("uploadId")][n] = b
		w.Header().Set("ETag", fmt.Sprintf("\"part-%d\"", n))
		return
	}
	if r.Method == "POST" && q.Get("uploadId") != "" {
		if r.Header.Get("If-None-Match") != "*" {
			fail(400)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		if len(f.completeStatuses) > 0 {
			code := f.completeStatuses[0]
			f.completeStatuses = f.completeStatuses[1:]
			if code != 200 {
				fail(code)
				return
			}
		}
		if _, ok := f.objects[key]; ok {
			fail(412)
			return
		}
		p := f.parts[q.Get("uploadId")]
		var b []byte
		for i := 1; i <= len(p); i++ {
			b = append(b, p[i]...)
		}
		f.objects[key] = b
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, "<CompleteMultipartUploadResult><ETag>not-a-sha256</ETag></CompleteMultipartUploadResult>")
		return
	}
	if r.Method == "PUT" {
		if r.Header.Get("If-None-Match") != "*" {
			fail(400)
			return
		}
		if len(f.putStatuses) > 0 {
			code := f.putStatuses[0]
			f.putStatuses = f.putStatuses[1:]
			if code != 200 {
				fail(code)
				return
			}
		}
		if _, ok := f.objects[key]; ok {
			fail(412)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			fail(500)
			return
		}
		f.objects[key] = b
		w.Header().Set("ETag", "not-a-sha256")
		return
	}
	if r.Method == "GET" {
		if len(f.getStatuses) > 0 {
			code := f.getStatuses[0]
			f.getStatuses = f.getStatuses[1:]
			if code != 200 {
				fail(code)
				return
			}
		}
		b, ok := f.objects[key]
		if !ok {
			fail(404)
			return
		}
		if f.getTransform != nil {
			b = f.getTransform(append([]byte{}, b...))
		}
		if f.cors != "" {
			w.Header().Set("Access-Control-Allow-Origin", f.cors)
		}
		if f.encoding != "" {
			w.Header().Set("Content-Encoding", f.encoding)
		}
		w.Header().Set("ETag", "apparently-matching")
		w.Header().Set("X-Amz-Meta-Sha256", packmanifest.Digest(b))
		w.Write(b)
		return
	}
	fail(400)
}
func fakeStore(t *testing.T, provider string) (*Store, *fakeCloud) {
	t.Helper()
	f := &fakeCloud{objects: map[string][]byte{}, parts: map[string]map[int][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	p := storageconfig.Profile{Provider: provider, Bucket: "fixture-bucket", Region: "us-east-1", Prefix: "fixture", PublicReadBase: "https://public.example.com", CredentialRef: &storageconfig.CredentialRef{Kind: "env", AccessKeyEnv: "TEST_WRITER_ID", SecretKeyEnv: "TEST_WRITER_SECRET"}}
	if provider == "cloudflare-r2" {
		p.Region = "auto"
		p.AccountID = strings.Repeat("a", 32)
	}
	endpoint, _ := p.Endpoint()
	target, _ := url.Parse(server.URL)
	allowed := strings.TrimPrefix(endpoint, "https://")
	transport := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != allowed && r.URL.Host != "public.example.com" {
			t.Errorf("unexpected network host %s", r.URL.Host)
			return nil, errors.New("unexpected host")
		}
		if r.URL.Host == allowed && !strings.Contains(r.Header.Get("Authorization"), "Credential=FAKE_WRITER/") {
			t.Error("missing explicit fake credentials")
		}
		if r.URL.Host == "public.example.com" && (r.Header.Get("Authorization") != "" || r.Header.Get("X-Amz-Security-Token") != "") {
			t.Error("writer credential leaked to public host")
		}
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		clone.URL = &u
		return server.Client().Transport.RoundTrip(clone)
	})
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("no redirect") }}
	creds := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "FAKE_WRITER", SecretAccessKey: "FAKE_SECRET_NEVER_LOG", SessionToken: "FAKE_SESSION"}, nil
	})
	s, err := newStore(p, creds, client, client)
	if err != nil {
		t.Fatal(err)
	}
	s.pause = func(context.Context, int) error { return nil }
	return s, f
}
func source(t *testing.T, b []byte) (packstore.Source, packstore.Object) {
	t.Helper()
	p := t.TempDir() + "/source.pack"
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	s := packstore.Source{Path: p, SHA256: packmanifest.Digest(b), Size: int64(len(b))}
	return s, packstore.Object{Kind: "packs", SHA256: s.SHA256, Size: s.Size}
}
func code(t *testing.T, err error, want packstore.Code) {
	t.Helper()
	var e *packstore.Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("got %v, want %s", err, want)
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "Signature") {
		t.Fatal("provider error leaked")
	}
}
func TestProvidersConditionalReadback(t *testing.T) {
	for _, provider := range []string{"aws-s3", "cloudflare-r2"} {
		t.Run(provider, func(t *testing.T) {
			s, f := fakeStore(t, provider)
			src, o := source(t, bytes.Repeat([]byte("pack"), 100))
			r, e := s.PutIfAbsent(context.Background(), o, src)
			if e != nil || !r.Verified || r.Reused {
				t.Fatal(r, e)
			}
			r, e = s.PutIfAbsent(context.Background(), o, src)
			if e != nil || !r.Verified || !r.Reused {
				t.Fatal(r, e)
			}
			f.objects["/fixture-bucket/"+r.Key] = []byte("different bytes")
			_, e = s.PutIfAbsent(context.Background(), o, src)
			code(t, e, packstore.Integrity)
			delete(f.objects, "/fixture-bucket/"+r.Key)
			code(t, s.VerifyStoredBytes(context.Background(), r), packstore.Missing)
			if f.aborts != 0 {
				t.Fatal("automatic deletion")
			}
			for _, h := range f.headers {
				if !strings.Contains(h.Get("Authorization"), "Credential=FAKE_WRITER/") {
					t.Fatal("signing")
				}
			}
			if provider == "cloudflare-r2" {
				for _, h := range f.headers {
					if !strings.Contains(h.Get("Authorization"), "/auto/s3/aws4_request") {
						t.Fatal("R2 signing region")
					}
				}
			}
		})
	}
}
func TestProviderFailures(t *testing.T) {
	for _, provider := range []string{"aws-s3", "cloudflare-r2"} {
		t.Run(provider, func(t *testing.T) {
			for _, statuses := range [][]int{{409, 200}, {429, 200}, {500, 200}, {503, 503, 503}, {403}, {401}} {
				t.Run(fmt.Sprint(statuses), func(t *testing.T) {
					s, f := fakeStore(t, provider)
					src, o := source(t, []byte("fixture"))
					f.putStatuses = append([]int{}, statuses...)
					_, e := s.PutIfAbsent(context.Background(), o, src)
					if statuses[0] == 401 || statuses[0] == 403 {
						code(t, e, packstore.Auth)
					} else if len(statuses) == 3 {
						code(t, e, packstore.Uncertain)
					} else if e != nil {
						t.Fatal(e)
					}
					if len(f.requests) > 4 {
						t.Fatal("unbounded retries")
					}
				})
			}
			for _, mutate := range []func([]byte) []byte{func(b []byte) []byte { return b[:len(b)-1] }, func(b []byte) []byte { return append(b, 'x') }, func(b []byte) []byte { b[0] ^= 1; return b }} {
				s, f := fakeStore(t, provider)
				src, o := source(t, []byte("fixture"))
				f.getTransform = mutate
				_, e := s.PutIfAbsent(context.Background(), o, src)
				code(t, e, packstore.Integrity)
			}
			s, f := fakeStore(t, provider)
			src, o := source(t, []byte("fixture"))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, e := s.PutIfAbsent(ctx, o, src)
			code(t, e, packstore.Canceled)
			if len(f.requests) != 0 {
				t.Fatal("request after cancel")
			}
		})
	}
}
func TestAWSMultipartCompletion(t *testing.T) {
	for _, scenario := range []string{"success", "409-rebuild", "412-reuse", "412-corrupt", "500-uncertain"} {
		t.Run(scenario, func(t *testing.T) {
			s, f := fakeStore(t, "aws-s3")
			src, o := source(t, bytes.Repeat([]byte("m"), int(SinglePutLimit+1)))
			key, _ := s.key(o)
			switch scenario {
			case "409-rebuild":
				f.completeStatuses = []int{409, 200}
			case "412-reuse":
				f.objects["/fixture-bucket/"+key] = bytes.Repeat([]byte("m"), int(o.Size))
			case "412-corrupt":
				f.objects["/fixture-bucket/"+key] = []byte("bad")
			case "500-uncertain":
				f.completeStatuses = []int{500}
			}
			r, e := s.PutIfAbsent(context.Background(), o, src)
			switch scenario {
			case "412-corrupt":
				code(t, e, packstore.Integrity)
			case "500-uncertain":
				code(t, e, packstore.Uncertain)
			default:
				if e != nil || !r.Verified {
					t.Fatal(r, e)
				}
			}
			if scenario == "409-rebuild" && (f.created != 2 || len(r.RetainedUploadIDs) != 1) {
				t.Fatal("did not rebuild/retain session", r)
			}
			if strings.HasPrefix(scenario, "412-") && r.UploadID == "" {
				t.Fatal("lost incomplete upload")
			}
			if scenario == "500-uncertain" && r.UploadID == "" {
				t.Fatal("lost uncertain upload")
			}
			if f.aborts != 0 {
				t.Fatal("implicit abort")
			}
		})
	}
}
func TestR2RejectsMultipartBeforeIO(t *testing.T) {
	s, f := fakeStore(t, "cloudflare-r2")
	_, e := s.PutIfAbsent(context.Background(), packstore.Object{Kind: "packs", SHA256: strings.Repeat("a", 64), Size: SinglePutLimit + 1}, packstore.Source{})
	code(t, e, packstore.Limit)
	if len(f.requests) != 0 || s.Capabilities().ConditionalMultipart {
		t.Fatal("R2 unsafe multipart")
	}
}
func TestPublicReadAndCORS(t *testing.T) {
	s, f := fakeStore(t, "cloudflare-r2")
	src, o := source(t, []byte("public bytes"))
	f.objects["/public.pack"] = []byte("public bytes")
	for _, cors := range []string{"", "https://wrong.example.com", "*", "https://app.example.com"} {
		f.cors = cors
		d, e := s.DiagnosePublic(context.Background(), "https://public.example.com/public.pack", "https://app.example.com", o)
		if e != nil || !d.Verified || d.CORS != (cors == "*" || cors == "https://app.example.com") {
			t.Fatal(d, e)
		}
	}
	_ = src
	f.encoding = "gzip"
	_, e := s.DiagnosePublic(context.Background(), "https://public.example.com/public.pack", "https://app.example.com", o)
	code(t, e, packstore.Integrity)
}
func TestAmbiguousPutRecovery(t *testing.T) {
	s, f := fakeStore(t, "aws-s3")
	src, o := source(t, []byte("uploaded before timeout"))
	key, _ := s.key(o)
	f.objects["/fixture-bucket/"+key] = []byte("uploaded before timeout")
	f.putStatuses = []int{503, 503, 503}
	r, e := s.PutIfAbsent(context.Background(), o, src)
	if e != nil || !r.Verified || !r.Reused {
		t.Fatal(r, e)
	}
}

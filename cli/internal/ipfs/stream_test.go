package ipfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamingTemporaryUpload(t *testing.T) {
	data := bytes.Repeat([]byte("pack-data"), 10000)
	cid := "b" + strings.Repeat("a", 58)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/add" {
			if r.URL.Query().Get("pin") != "false" {
				t.Error("pin enabled")
			}
			mr, e := r.MultipartReader()
			if e != nil {
				t.Error(e)
				return
			}
			part, e := mr.NextPart()
			if e != nil {
				t.Error(e)
				return
			}
			b, e := io.ReadAll(part)
			if e != nil || !bytes.Equal(b, data) {
				t.Error("bytes changed")
			}
			fmt.Fprintf(w, `{"Hash":%q}`, cid)
			return
		}
		if r.Method != "GET" {
			t.Error("not read-only")
		}
		w.Write(data)
	}))
	defer server.Close()
	c := NewWithGateways(server.URL, []string{server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, e := c.AddTemporaryContext(ctx, "fixture.pack", bytes.NewReader(data))
	if e != nil || got != cid {
		t.Fatal(got, e)
	}
	body, e := c.GetFromGatewaysContext(ctx, cid)
	if e != nil {
		t.Fatal(e)
	}
	defer body.Close()
	b, e := io.ReadAll(body)
	if e != nil || !bytes.Equal(b, data) {
		t.Fatal("read mismatch")
	}
}
func TestStreamingUploadEarlyRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	c := NewWithGateways(server.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := c.AddTemporaryContext(ctx, "test", bytes.NewReader(bytes.Repeat([]byte("x"), 2<<20))); e == nil {
		t.Fatal("early rejection accepted")
	}
}

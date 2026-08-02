package ipfs

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddTemporaryUsesPinFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pin") != "false" || r.URL.Path != "/api/v0/add" {
			t.Fatalf("temporary add used %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"Hash":"bafy-test"}`)
	}))
	defer server.Close()
	cid, err := NewWithGateways(server.URL, nil).AddTemporary("pack", bytes.NewReader([]byte("pack")))
	if err != nil || cid != "bafy-test" { t.Fatalf("cid=%q err=%v", cid, err) }
}

func TestGCUsesRepoGC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/repo/gc" {
			t.Fatalf("unexpected GC request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, "{}\n")
	}))
	defer server.Close()
	if err := NewWithGateways(server.URL, nil).GC(); err != nil { t.Fatal(err) }
}

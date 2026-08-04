package ipfs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// TestGatewayFallbackAcceptance is the deterministic, repository-local drill
// for the production HK -> US -> public gateway order. It deliberately uses an
// unavailable local Kubo API so the test also proves Clone/Fetch never depends
// on a local daemon.
func TestGatewayFallbackAcceptance(t *testing.T) {
	t.Run("HK outage selects US", func(t *testing.T) {
		var mu sync.Mutex
		var requests []string
		record := func(name string, r *http.Request) {
			mu.Lock()
			requests = append(requests, name+" "+r.URL.Path)
			mu.Unlock()
		}

		hk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("hk", r)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer hk.Close()

		us := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("us", r)
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.URL.Path == "/ipfs/bafy-hk-outage" {
				_, _ = io.WriteString(w, "us-pack")
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer us.Close()

		public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("public", r)
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer public.Close()

		selected, health := SelectGateways(context.Background(), []config.Gateway{
			{Name: "hk", URL: hk.URL},
			{Name: "us", URL: us.URL},
		})
		if len(health) != 2 || health[0].Err == nil || health[1].Err != nil {
			t.Fatalf("unexpected health results: %#v", health)
		}
		if len(selected) != 1 || selected[0].Name != "us" {
			t.Fatalf("selected gateways = %#v, want only us", selected)
		}

		body, err := NewWithGateways("http://127.0.0.1:1", []string{us.URL, public.URL}).GetFromGateways("bafy-hk-outage")
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "us-pack" {
			t.Fatalf("content = %q, want us-pack", got)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, request := range requests {
			if request == "public /ipfs/bafy-hk-outage" {
				t.Fatalf("public gateway was used while US served the CID: %v", requests)
			}
		}
	})

	t.Run("CID miss reaches public fallback", func(t *testing.T) {
		var mu sync.Mutex
		var requests []string
		record := func(name string, r *http.Request) {
			mu.Lock()
			requests = append(requests, name+" "+r.URL.Path)
			mu.Unlock()
		}
		miss := func(name string) *httptest.Server {
			return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record(name, r)
				if r.URL.Path == "/healthz" {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
		}
		hk := miss("hk")
		defer hk.Close()
		us := miss("us")
		defer us.Close()
		public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("public", r)
			if r.URL.Path == "/ipfs/bafy-cid-miss" {
				_, _ = io.WriteString(w, "public-pack")
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer public.Close()

		body, err := NewWithGateways("http://127.0.0.1:1", []string{hk.URL, us.URL, public.URL}).GetFromGateways("bafy-cid-miss")
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "public-pack" {
			t.Fatalf("content = %q, want public-pack", got)
		}
		mu.Lock()
		defer mu.Unlock()
		want := []string{
			"hk /ipfs/bafy-cid-miss",
			"us /ipfs/bafy-cid-miss",
			"public /ipfs/bafy-cid-miss",
		}
		if len(requests) != len(want) {
			t.Fatalf("requests = %v, want %v", requests, want)
		}
		for i := range want {
			if requests[i] != want[i] {
				t.Fatalf("requests = %v, want %v", requests, want)
			}
		}
	})
}

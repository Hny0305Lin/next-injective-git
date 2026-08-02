package ipfs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestSelectGatewaysSkipsUnhealthy(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected probe path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthy.Close()

	selected, health := SelectGateways(context.Background(), []config.Gateway{
		{Name: "down", URL: unhealthy.URL},
		{Name: "up", URL: healthy.URL},
	})
	if len(health) != 2 || health[0].Err == nil || health[1].Err != nil {
		t.Fatalf("unexpected health: %#v", health)
	}
	if len(selected) != 1 || selected[0].Name != "up" {
		t.Fatalf("selected = %#v, want only healthy gateway", selected)
	}
}

func TestGetFromGatewaysFallsBackAcrossGateways(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ipfs/bafy-test" {
			t.Fatalf("unexpected content path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("packfile"))
	}))
	defer second.Close()

	client := NewWithGateways("http://127.0.0.1:1", []string{first.URL, second.URL})
	body, err := client.GetFromGateways("bafy-test")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "packfile" {
		t.Fatalf("content = %q, want packfile", got)
	}
}

func TestSelectGatewaysRetainsOrderWhenAllProbesFail(t *testing.T) {
	gateways := []config.Gateway{{Name: "first", URL: "http://127.0.0.1:1"}, {Name: "second", URL: "http://127.0.0.1:2"}}
	selected, health := SelectGateways(context.Background(), gateways)
	if len(health) != 2 || health[0].Err == nil || health[1].Err == nil {
		t.Fatalf("unexpected health: %#v", health)
	}
	if len(selected) != 2 || selected[0].Name != "first" || selected[1].Name != "second" {
		t.Fatalf("selected = %#v, want original order", selected)
	}
}

func TestGetFromGatewaysNeverCallsLocalKubo(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("gateway pack"))
	}))
	defer gateway.Close()
	client := NewWithGateways("http://127.0.0.1:1", []string{gateway.URL})
	body, err := client.GetFromGateways("bafy-test")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != "gateway pack" {
		t.Fatalf("got %q", got)
	}
}

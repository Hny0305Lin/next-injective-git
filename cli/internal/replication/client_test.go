package replication

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfirmSendsScopedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected request: %s %q", r.Method, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cid":"bafy-test","pinned":true,"expires_at":123}`))
	}))
	defer server.Close()
	_, err := New(server.URL, "token").Confirm(Request{CID: "bafy-test", Owner: "inj1owner", Repo: "repo", Ref: "refs/heads/main", PackSHA256: "abc", Size: 1, ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConfirmRejectsUnconfirmedPin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"cid":"bafy-test","pinned":false}`))
	}))
	defer server.Close()
	if _, err := New(server.URL, "token").Confirm(Request{CID: "bafy-test"}); err == nil {
		t.Fatal("expected unconfirmed pin error")
	}
}

func TestConfirmRetriesTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary outage", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cid":"bafy-test","pinned":true}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "token").Confirm(Request{CID: "bafy-test"}); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("request attempts = %d, want 2", got)
	}
}

func TestConfirmDoesNotRetryPermanentClientError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := New(server.URL, "token").Confirm(Request{CID: "bafy-test"}); err == nil {
		t.Fatal("expected unauthorized error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("request attempts = %d, want 1", got)
	}
}

func TestAuthorizeExchangesIdentityForScopedTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/upload-authorizations" || r.Header.Get("Authorization") != "Bearer identity" {
			t.Fatalf("unexpected authorization request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"authorization":"scoped-ticket"}`))
	}))
	defer server.Close()
	scoped, err := New(server.URL+"/v1/replications", "identity").Authorize(Request{CID: "bafy-test", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	client, ok := scoped.(*Client)
	if !ok || client.token != "scoped-ticket" {
		t.Fatalf("scoped ticket = %#v", scoped)
	}
}

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthorizeBindsTicketPayload(t *testing.T) {
	s := &service{
		secret:   []byte("01234567890123456789012345678901"),
		maxBytes: 1024,
	}
	req := replicationRequest{
		CID: "bafy-test", Owner: "inj1owner", Repo: "repo", Ref: "refs/heads/main",
		PackSHA256: strings.Repeat("a", 64), Size: 4,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	ticket, err := s.sign(claims{
		Kind: "replication", Subject: "alice", CID: req.CID, Owner: req.Owner, Repo: req.Repo,
		Ref: req.Ref, PackSHA256: req.PackSHA256, Size: req.Size,
		ExpiresAt: time.Now().Add(time.Minute).Unix(), JTI: "jti",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.authorize("Bearer "+ticket, req); err != nil {
		t.Fatal(err)
	}
	req.Ref = "refs/heads/other"
	if _, err := s.authorize("Bearer "+ticket, req); err == nil {
		t.Fatal("expected ticket payload binding failure")
	}
}

func TestAllowEnforcesPerRepositoryByteBudget(t *testing.T) {
	s := &service{
		maxBytes:    100,
		ratePerMin:  100,
		bytesPerMin: 10,
		window:      map[string]*rateWindow{},
	}
	if !s.allow("alice", "inj1owner", "repo", 6) {
		t.Fatal("first request should fit the byte budget")
	}
	if s.allow("alice", "inj1owner", "repo", 5) {
		t.Fatal("second request should exceed the repository byte budget")
	}
	if !s.allow("alice", "inj1owner", "other", 5) {
		t.Fatal("a different repository should have an independent budget")
	}
}

func TestReplicationHTTPFlowIsScopedIdempotentAndFailClosed(t *testing.T) {
	const pack = "test pack bytes"
	hash := sha256Hex(pack)
	state := filepath.Join(t.TempDir(), "issued.tsv")
	pinned := map[string]bool{}
	removed := map[string]bool{}
	kubo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/pin/add":
			cid := r.URL.Query().Get("arg")
			pinned[cid] = true
			w.WriteHeader(http.StatusOK)
		case "/api/v0/cat":
			if !pinned[r.URL.Query().Get("arg")] {
				http.Error(w, "missing pin", http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, pack)
		case "/api/v0/pin/rm":
			cid := r.URL.Query().Get("arg")
			removed[cid] = true
			delete(pinned, cid)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer kubo.Close()

	s := &service{
		secret: []byte("01234567890123456789012345678901"), kuboAPI: kubo.URL,
		stateFile: state, audit: log.New(io.Discard, "", 0), maxBytes: 1024,
		ratePerMin: 1, bytesPerMin: 1024, http: kubo.Client(),
		used: map[string]string{}, window: map[string]*rateWindow{},
	}
	identity, err := s.sign(claims{Kind: "identity", Subject: "alice", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := replicationRequest{CID: "bafy-test", Owner: "inj1owner", Repo: "repo", Ref: "refs/heads/main", PackSHA256: hash, Size: int64(len(pack))}

	issue := httptest.NewRecorder()
	issueRequest := httptest.NewRequest(http.MethodPost, "/v1/upload-authorizations", strings.NewReader(jsonBody(req)))
	issueRequest.Header.Set("Authorization", "Bearer "+identity)
	s.issueAuthorization(issue, issueRequest)
	if issue.Code != http.StatusCreated {
		t.Fatalf("issue status=%d body=%s", issue.Code, issue.Body.String())
	}
	var issued struct {
		Authorization string `json:"authorization"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil || issued.Authorization == "" {
		t.Fatalf("issued ticket = %q, err=%v", issued.Authorization, err)
	}

	replicate := func(ticket string, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/replications", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+ticket)
		s.replicate(rr, r)
		return rr
	}
	first := replicate(issued.Authorization, jsonBody(req))
	if first.Code != http.StatusCreated || !pinned[req.CID] {
		t.Fatalf("first replication status=%d body=%s pinned=%v", first.Code, first.Body.String(), pinned[req.CID])
	}
	// The per-minute quota is one request. A retry with the same JTI must still
	// succeed and must not call Kubo a second time.
	second := replicate(issued.Authorization, jsonBody(req))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"idempotent":true`) {
		t.Fatalf("idempotent retry status=%d body=%s", second.Code, second.Body.String())
	}

	// A process restart must preserve the consumed JTI, otherwise the same
	// scoped ticket could trigger another Kubo pin after a service restart.
	restarted := &service{
		secret: []byte("01234567890123456789012345678901"), kuboAPI: kubo.URL,
		stateFile: state, audit: log.New(io.Discard, "", 0), maxBytes: 1024,
		ratePerMin: 1, bytesPerMin: 1024, http: kubo.Client(),
		used: map[string]string{}, window: map[string]*rateWindow{},
	}
	if err := restarted.loadState(); err != nil {
		t.Fatal(err)
	}
	restartedRetry := httptest.NewRecorder()
	restartedRequest := httptest.NewRequest(http.MethodPost, "/v1/replications", strings.NewReader(jsonBody(req)))
	restartedRequest.Header.Set("Authorization", "Bearer "+issued.Authorization)
	restarted.replicate(restartedRetry, restartedRequest)
	if restartedRetry.Code != http.StatusOK || !strings.Contains(restartedRetry.Body.String(), `"idempotent":true`) {
		t.Fatalf("restart retry status=%d body=%s", restartedRetry.Code, restartedRetry.Body.String())
	}

	mutated := req
	mutated.Ref = "refs/heads/other"
	conflict := replicate(issued.Authorization, jsonBody(mutated))
	if conflict.Code != http.StatusUnauthorized {
		t.Fatalf("mutated request status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	if len(removed) != 0 {
		t.Fatalf("unexpected pin removal on successful flow: %#v", removed)
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestReplicationHashFailureRemovesPinAndDoesNotConsumeJTI(t *testing.T) {
	const pack = "actual bytes"
	state := filepath.Join(t.TempDir(), "issued.tsv")
	pinned := false
	removed := false
	kubo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/pin/add":
			pinned = true
			w.WriteHeader(http.StatusOK)
		case "/api/v0/cat":
			_, _ = io.WriteString(w, pack)
		case "/api/v0/pin/rm":
			removed = true
			pinned = false
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer kubo.Close()
	s := &service{
		secret: []byte("01234567890123456789012345678901"), kuboAPI: kubo.URL,
		stateFile: state, audit: log.New(io.Discard, "", 0), maxBytes: 1024,
		ratePerMin: 2, bytesPerMin: 1024, http: kubo.Client(),
		used: map[string]string{}, window: map[string]*rateWindow{},
	}
	req := replicationRequest{CID: "bafy-bad", Owner: "inj1owner", Repo: "repo", Ref: "refs/heads/main", PackSHA256: strings.Repeat("0", 64), Size: int64(len(pack))}
	ticket, err := s.sign(claims{Kind: "replication", Subject: "alice", CID: req.CID, Owner: req.Owner, Repo: req.Repo, Ref: req.Ref, PackSHA256: req.PackSHA256, Size: req.Size, ExpiresAt: time.Now().Add(time.Minute).Unix(), JTI: "jti-bad"})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/replications", strings.NewReader(jsonBody(req)))
	r.Header.Set("Authorization", "Bearer "+ticket)
	s.replicate(rr, r)
	if rr.Code != http.StatusBadGateway || pinned || !removed {
		t.Fatalf("hash failure status=%d pinned=%v removed=%v body=%s", rr.Code, pinned, removed, rr.Body.String())
	}
	if _, ok := s.used["jti-bad"]; ok {
		t.Fatal("failed replication must not consume JTI")
	}
}

func sha256Hex(value string) string {
	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", h[:])
}

func jsonBody(value any) string {
	b, _ := json.Marshal(value)
	return string(b)
}

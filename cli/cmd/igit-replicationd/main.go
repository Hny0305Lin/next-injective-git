// igit-replicationd is the US-only controlled replication/Pin service.
// It exposes no generic IPFS API and never submits update_ref transactions.
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type claims struct {
	Kind       string `json:"kind,omitempty"`
	Subject    string `json:"sub"`
	CID        string `json:"cid"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Ref        string `json:"ref"`
	PackSHA256 string `json:"pack_sha256"`
	Size       int64  `json:"size"`
	ExpiresAt  int64  `json:"exp"`
	JTI        string `json:"jti"`
}

type replicationRequest struct {
	CID        string `json:"cid"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Ref        string `json:"ref"`
	PackSHA256 string `json:"pack_sha256"`
	Size       int64  `json:"size"`
	ExpiresAt  int64  `json:"expires_at"`
}

type service struct {
	secret      []byte
	kuboAPI     string
	stateFile   string
	audit       *log.Logger
	maxBytes    int64
	ratePerMin  int
	bytesPerMin int64
	http        *http.Client
	mu          sync.Mutex
	used        map[string]string // jti -> cid; identical retries are idempotent
	window      map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	count int
	bytes int64
}

const maxReplicationRequestBody = 64 << 10

func main() {
	secret := []byte(os.Getenv("IGIT_REPLICATION_JWT_HMAC"))
	if len(secret) < 32 {
		log.Fatal("IGIT_REPLICATION_JWT_HMAC must be at least 32 bytes")
	}
	state := env("IGIT_REPLICATION_STATE", "/var/lib/igit-replication/issued.tsv")
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		log.Fatal(err)
	}
	s := &service{
		secret: secret, kuboAPI: strings.TrimRight(env("KUBO_API", "http://127.0.0.1:5001"), "/"),
		stateFile: state, audit: log.New(os.Stdout, "audit ", 0), maxBytes: envInt64("IGIT_REPLICATION_MAX_BYTES", 2<<30),
		ratePerMin:  int(envInt64("IGIT_REPLICATION_RATE_PER_MINUTE", 12)),
		bytesPerMin: envInt64("IGIT_REPLICATION_BYTES_PER_MINUTE", 4<<30),
		http:        &http.Client{Timeout: 12 * time.Minute},
		used:        map[string]string{}, window: map[string]*rateWindow{},
	}
	if err := s.loadState(); err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/upload-authorizations", s.issueAuthorization)
	mux.HandleFunc("/v1/replications", s.replicate)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// Replication requests contain only a small JSON binding; the pack bytes
	// stay in Kubo. Keep the control-plane parser independent of the pack-size
	// quota so an attacker cannot force the service to accept multi-gigabyte
	// request bodies before authentication.
	server := &http.Server{Addr: env("LISTEN_ADDR", "127.0.0.1:8088"), Handler: maxBody(mux, maxReplicationRequestBody), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("igit replication service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

// issueAuthorization turns a user identity token into a single, scoped
// replication ticket after the client has learned its temporary CID.
func (s *service) issueAuthorization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req replicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	identity, err := s.identity(r.Header.Get("Authorization"))
	if err != nil {
		s.auditf("authorization_denied", req, "", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if req.Size <= 0 || req.Size > s.maxBytes || req.CID == "" || req.Owner == "" || req.Repo == "" || req.Ref == "" || len(req.PackSHA256) != 64 {
		http.Error(w, "invalid upload authorization binding", http.StatusBadRequest)
		return
	}
	if !s.allow(identity.Subject, req.Owner, req.Repo, req.Size) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	expires := time.Now().Add(15 * time.Minute).Unix()
	if req.ExpiresAt > 0 && req.ExpiresAt < expires {
		expires = req.ExpiresAt
	}
	if expires <= time.Now().Unix() {
		http.Error(w, "requested expiration is invalid", http.StatusBadRequest)
		return
	}
	cl := claims{Kind: "replication", Subject: identity.Subject, CID: req.CID, Owner: req.Owner, Repo: req.Repo, Ref: req.Ref, PackSHA256: req.PackSHA256, Size: req.Size, ExpiresAt: expires, JTI: randomID()}
	token, err := s.sign(cl)
	if err != nil {
		http.Error(w, "cannot issue authorization", http.StatusInternalServerError)
		return
	}
	s.auditf("authorization_issued", req, identity.Subject, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"authorization": token, "expires_at": expires})
}

func (s *service) replicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req replicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	cl, err := s.authorize(r.Header.Get("Authorization"), req)
	if err != nil {
		s.auditf("denied", req, "", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	// The authorization endpoint accounts for the upload quota. A replication
	// request may be retried after a lost response, so counting it again here
	// would make an otherwise valid JTI retry fail with 429.
	s.mu.Lock()
	if existing, ok := s.used[cl.JTI]; ok {
		s.mu.Unlock()
		if existing == req.CID {
			writeJSON(w, http.StatusOK, map[string]any{"cid": req.CID, "pinned": true, "expires_at": cl.ExpiresAt, "idempotent": true})
			return
		}
		http.Error(w, "authorization already used", http.StatusConflict)
		return
	}
	// Serialize the state transition: pin success is recorded before the reply,
	// so a network retry cannot create another authorization use.
	defer s.mu.Unlock()
	// A concurrent request with the same JTI may have completed while this
	// request waited for the state lock. Re-check before touching Kubo.
	if existing, ok := s.used[cl.JTI]; ok {
		if existing == req.CID {
			writeJSON(w, http.StatusOK, map[string]any{"cid": req.CID, "pinned": true, "expires_at": cl.ExpiresAt, "idempotent": true})
			return
		}
		http.Error(w, "authorization already used", http.StatusConflict)
		return
	}
	if err := s.kuboPost("/api/v0/pin/add?arg="+url.QueryEscape(req.CID)+"&recursive=true", nil, nil); err != nil {
		s.auditf("pin_failed", req, cl.Subject, err)
		http.Error(w, "US pin failed", http.StatusBadGateway)
		return
	}
	hash, err := s.kuboSHA256(req.CID)
	if err != nil || !hmac.Equal([]byte(hash), []byte(req.PackSHA256)) {
		_ = s.kuboPost("/api/v0/pin/rm?arg="+url.QueryEscape(req.CID), nil, nil)
		if err == nil {
			err = errors.New("pack SHA-256 mismatch")
		}
		s.auditf("hash_failed", req, cl.Subject, err)
		http.Error(w, "pack verification failed", http.StatusBadGateway)
		return
	}
	if err := s.record(cl, req); err != nil {
		s.auditf("state_failed", req, cl.Subject, err)
		http.Error(w, "pin recorded but state write failed", http.StatusInternalServerError)
		return
	}
	s.auditf("pinned", req, cl.Subject, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"cid": req.CID, "pinned": true, "expires_at": cl.ExpiresAt})
}

func (s *service) authorize(header string, req replicationRequest) (claims, error) {
	cl, err := s.parseToken(header)
	if err != nil {
		return claims{}, err
	}
	if cl.Kind != "replication" || cl.JTI == "" {
		return claims{}, errors.New("invalid replication authorization")
	}
	if req.ExpiresAt > cl.ExpiresAt {
		return claims{}, errors.New("authorization expired")
	}
	if req.CID != cl.CID || req.Owner != cl.Owner || req.Repo != cl.Repo || req.Ref != cl.Ref || req.PackSHA256 != cl.PackSHA256 || req.Size != cl.Size {
		return claims{}, errors.New("authorization does not match replication request")
	}
	if req.Size <= 0 || req.Size > s.maxBytes {
		return claims{}, errors.New("pack exceeds upload limit")
	}
	return cl, nil
}

func (s *service) identity(header string) (claims, error) {
	cl, err := s.parseToken(header)
	if err != nil {
		return claims{}, err
	}
	if cl.Kind != "identity" {
		return claims{}, errors.New("identity authorization required")
	}
	return cl, nil
}

func (s *service) parseToken(header string) (claims, error) {
	if !strings.HasPrefix(header, "Bearer ") {
		return claims{}, errors.New("missing bearer authorization")
	}
	parts := strings.Split(strings.TrimPrefix(header, "Bearer "), ".")
	if len(parts) != 3 {
		return claims{}, errors.New("invalid authorization")
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return claims{}, errors.New("invalid authorization signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, errors.New("invalid authorization claims")
	}
	var cl claims
	if json.Unmarshal(raw, &cl) != nil || cl.Subject == "" || cl.ExpiresAt <= time.Now().Unix() {
		return claims{}, errors.New("invalid authorization claims")
	}
	return cl, nil
}

func (s *service) sign(cl claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	raw, err := json.Marshal(cl)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(header + "." + payload))
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func randomID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *service) allow(subject, owner, repo string, size int64) bool {
	if size <= 0 || size > s.maxBytes {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := subject + "|" + owner + "/" + repo
	now := time.Now()
	w := s.window[key]
	if w == nil || now.Sub(w.start) >= time.Minute {
		s.window[key] = &rateWindow{start: now, count: 1, bytes: size}
		return true
	}
	if w.count >= s.ratePerMin || (s.bytesPerMin > 0 && w.bytes > s.bytesPerMin-size) {
		return false
	}
	w.count++
	w.bytes += size
	return true
}

func (s *service) kuboPost(path string, body io.Reader, out io.Writer) error {
	req, err := http.NewRequest(http.MethodPost, s.kuboAPI+path, body)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		_, _ = io.Copy(out, resp.Body)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Kubo API HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *service) kuboSHA256(cid string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, s.kuboAPI+"/api/v0/cat?arg="+url.QueryEscape(cid), nil)
	if err != nil {
		return "", err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Kubo cat HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(resp.Body, s.maxBytes+1)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *service) loadState() error {
	data, err := os.ReadFile(s.stateFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		p := strings.Split(line, "\t")
		if len(p) >= 2 {
			s.used[p[0]] = p[1]
		}
	}
	return nil
}
func (s *service) record(cl claims, req replicationRequest) error {
	f, err := os.OpenFile(s.stateFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = fmt.Fprintf(f, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%d\n", cl.JTI, req.CID, cl.ExpiresAt, cl.Subject, req.Owner, req.Repo, req.Ref, req.Size); err != nil {
		return err
	}
	s.used[cl.JTI] = req.CID
	return nil
}
func (s *service) auditf(event string, req replicationRequest, subject string, err error) {
	entry := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339), "event": event, "subject": subject, "cid": req.CID, "owner": req.Owner, "repo": req.Repo, "ref": req.Ref, "size": req.Size}
	if err != nil {
		entry["error"] = err.Error()
	}
	b, _ := json.Marshal(entry)
	s.audit.Println(string(b))
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func maxBody(next http.Handler, n int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, n)
		next.ServeHTTP(w, r)
	})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt64(k string, d int64) int64 {
	v, err := strconv.ParseInt(env(k, strconv.FormatInt(d, 10)), 10, 64)
	if err != nil || v <= 0 {
		return d
	}
	return v
}

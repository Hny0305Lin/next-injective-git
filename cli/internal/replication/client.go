// Package replication talks to the controlled US Pin service. It never talks
// to a remote Kubo API.
package replication

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Request binds a temporary CID to exactly one intended on-chain ref update.
type Request struct {
	CID        string `json:"cid"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Ref        string `json:"ref"`
	PackSHA256 string `json:"pack_sha256"`
	Size       int64  `json:"size"`
	ExpiresAt  int64  `json:"expires_at"`
}

// Response is returned only when US Kubo has completed its recursive pin.
type Response struct {
	CID       string `json:"cid"`
	Pinned    bool   `json:"pinned"`
	ExpiresAt int64  `json:"expires_at"`
}

// Client is the push-only controlled replication client.
type Client struct {
	endpoint         string
	identityEndpoint string
	token            string
	http             *http.Client
}

const (
	// Confirm is safe to retry because the server binds the ticket JTI and
	// returns an idempotent success after the first durable Pin.
	maxConfirmAttempts = 3
	confirmRetryDelay  = 250 * time.Millisecond
)

// Confirmer is a scoped ticket that can request one US replication.
type Confirmer interface {
	Confirm(Request) (Response, error)
}

// Authorizer exchanges the user identity token for a scoped ticket.
type Authorizer interface {
	Authorize(Request) (Confirmer, error)
}

func New(endpoint, token string) *Client {
	return NewDynamic(endpoint, "", token)
}

// NewDynamic creates a replication client that can refresh its identity token
// from the public igit authorization API. A non-expired explicit token remains
// a backwards-compatible override for offline/private deployments.
func NewDynamic(endpoint, identityEndpoint, token string) *Client {
	return &Client{
		endpoint:         strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		identityEndpoint: strings.TrimRight(strings.TrimSpace(identityEndpoint), "/"),
		token:            strings.TrimSpace(token),
		http:             &http.Client{Timeout: 12 * time.Minute},
	}
}

// Authorize exchanges an identity token after the CID is known for a
// short-lived ticket whose claims bind this exact replication request.
func (c *Client) Authorize(reqBody Request) (Confirmer, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("upload replication endpoint is not configured")
	}
	if err := c.refreshIdentityToken(); err != nil {
		return nil, err
	}
	if c.token == "" {
		return nil, fmt.Errorf("upload identity authorization is missing; sign in to the upload authorization service before pushing")
	}
	endpoint := strings.TrimSuffix(c.endpoint, "/replications") + "/upload-authorizations"
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload authorization request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("upload authorization request: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Authorization string `json:"authorization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Authorization == "" {
		return nil, fmt.Errorf("invalid upload authorization response")
	}
	return New(c.endpoint, out.Authorization), nil
}

func (c *Client) refreshIdentityToken() error {
	if tokenFresh(c.token, 30*time.Second) {
		return nil
	}
	if c.identityEndpoint == "" {
		return fmt.Errorf("upload identity authorization expired and no authorization endpoint is configured")
	}
	req, err := http.NewRequest(http.MethodPost, c.identityEndpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("refresh upload identity authorization: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("refresh upload identity authorization: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Authorization string `json:"authorization"`
		ExpiresAt     int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || strings.TrimSpace(out.Authorization) == "" {
		return fmt.Errorf("invalid upload identity authorization response")
	}
	c.token = strings.TrimSpace(out.Authorization)
	if !tokenFresh(c.token, 5*time.Second) {
		return fmt.Errorf("upload identity authorization API returned an expired token")
	}
	return nil
}

// tokenFresh only reads the untrusted JWT expiration hint. Signature
// verification remains the replication server's responsibility. Opaque legacy
// tokens have no client-readable expiration and are therefore used as-is.
func tokenFresh(token string, margin time.Duration) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.ExpiresAt == 0 {
		return true
	}
	return claims.ExpiresAt > time.Now().Add(margin).Unix()
}

// Confirm requests a US fetch and pin. Authorization is a short-lived scoped
// bearer token validated by the service; it cannot authorize a chain update.
func (c *Client) Confirm(reqBody Request) (Response, error) {
	if c.endpoint == "" {
		return Response{}, fmt.Errorf("upload replication endpoint is not configured")
	}
	if c.token == "" {
		return Response{}, fmt.Errorf("upload authorization is missing; obtain a short-lived CID-bound upload token before pushing")
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Response{}, err
	}
	var lastErr error
	for attempt := 1; attempt <= maxConfirmAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			return Response{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("US replication request: %w", err)
			if attempt < maxConfirmAttempts {
				time.Sleep(confirmRetryDelay)
				continue
			}
			return Response{}, lastErr
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read US replication response: %w", readErr)
			if attempt < maxConfirmAttempts {
				time.Sleep(confirmRetryDelay)
				continue
			}
			return Response{}, lastErr
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			lastErr = fmt.Errorf("US replication request: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if retryableConfirmStatus(resp.StatusCode) && attempt < maxConfirmAttempts {
				time.Sleep(confirmRetryDelay)
				continue
			}
			return Response{}, lastErr
		}

		var out Response
		if err := json.Unmarshal(body, &out); err != nil {
			return Response{}, fmt.Errorf("decode US replication response: %w", err)
		}
		if !out.Pinned || out.CID != reqBody.CID {
			return Response{}, fmt.Errorf("US replication did not confirm pin for %s", reqBody.CID)
		}
		return out, nil
	}
	return Response{}, lastErr
}

func retryableConfirmStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

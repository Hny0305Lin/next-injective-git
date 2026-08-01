// Package ipfs is a minimal Kubo (go-ipfs) HTTP RPC client using only the
// standard library, plus a public-gateway fallback for downloads.
package ipfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Kubo node's RPC API (/api/v0) and, optionally,
// falls back to a public gateway for reads.
type Client struct {
	apiURL      string
	gatewayURLs []string
	http        *http.Client
}

// New creates a client. gatewayURL may be empty to disable the fallback.
func New(apiURL, gatewayURL string) *Client {
	return NewWithGateways(apiURL, []string{gatewayURL})
}

// NewWithGateways creates a client with ordered read-only gateway fallbacks.
func NewWithGateways(apiURL string, gatewayURLs []string) *Client {
	var normalized []string
	seen := make(map[string]bool)
	for _, gatewayURL := range gatewayURLs {
		gatewayURL = strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
		if gatewayURL == "" || seen[gatewayURL] {
			continue
		}
		seen[gatewayURL] = true
		normalized = append(normalized, gatewayURL)
	}
	return &Client{
		apiURL:      strings.TrimRight(apiURL, "/"),
		gatewayURLs: normalized,
		http:        &http.Client{Timeout: 10 * time.Minute},
	}
}

type addResponse struct {
	Hash string `json:"Hash"`
	Name string `json:"Name"`
	Size string `json:"Size"`
}

// Add uploads a blob to IPFS (pinned, CIDv1) and returns its CID.
func (c *Client) Add(name string, r io.Reader) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	endpoint := c.apiURL + "/api/v0/add?pin=true&cid-version=1"
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ipfs add: %w (is Kubo running at %s?)", err, c.apiURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ipfs add: HTTP %d: %s", resp.StatusCode, msg)
	}
	var ar addResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", fmt.Errorf("ipfs add: decode response: %w", err)
	}
	if ar.Hash == "" {
		return "", fmt.Errorf("ipfs add: empty CID in response")
	}
	return ar.Hash, nil
}

// Cat downloads a blob by CID, trying the local node first and then the
// public gateway.
func (c *Client) Cat(cid string) (io.ReadCloser, error) {
	endpoint := c.apiURL + "/api/v0/cat?arg=" + url.QueryEscape(cid)
	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}
	localFailure := "unknown error"
	if err != nil {
		localFailure = err.Error()
	} else if resp != nil {
		localFailure = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if resp != nil {
		resp.Body.Close()
	}

	if len(c.gatewayURLs) == 0 {
		return nil, fmt.Errorf("ipfs cat %s: %s", cid, localFailure)
	}
	// Gateway fallback (read-only, plain GET). Try every selected endpoint so a
	// mid-transfer outage never turns into a clone failure when the other region
	// is healthy.
	var gatewayErrors []string
	for _, gatewayURL := range c.gatewayURLs {
		gwResp, gwErr := c.http.Get(gatewayURL + "/ipfs/" + url.PathEscape(cid))
		if gwErr != nil {
			gatewayErrors = append(gatewayErrors, gatewayURL+": "+gwErr.Error())
			continue
		}
		if gwResp.StatusCode == http.StatusOK {
			return gwResp.Body, nil
		}
		gwResp.Body.Close()
		gatewayErrors = append(gatewayErrors, fmt.Sprintf("%s: HTTP %d", gatewayURL, gwResp.StatusCode))
	}
	return nil, fmt.Errorf("ipfs cat %s: node and all gateways failed: %s / %s", cid, localFailure, strings.Join(gatewayErrors, "; "))
}

// SwarmConnect asks the local Kubo node to open a direct connection to the
// given peer multiaddr, so Bitswap can exchange blocks without waiting on DHT
// discovery. Best effort: callers typically ignore the error (a missing daemon
// or unreachable peer just means the gateway fallback is used instead).
func (c *Client) SwarmConnect(multiaddr string) error {
	endpoint := c.apiURL + "/api/v0/swarm/connect?arg=" + url.QueryEscape(multiaddr)
	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	// short timeout: this is an optimization, never block a git op for long
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("swarm connect: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("swarm connect: HTTP %d: %s", resp.StatusCode, msg)
	}
	return nil
}

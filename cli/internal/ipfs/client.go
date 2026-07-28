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
	apiURL     string
	gatewayURL string
	http       *http.Client
}

// New creates a client. gatewayURL may be empty to disable the fallback.
func New(apiURL, gatewayURL string) *Client {
	return &Client{
		apiURL:     strings.TrimRight(apiURL, "/"),
		gatewayURL: strings.TrimRight(gatewayURL, "/"),
		http:       &http.Client{Timeout: 10 * time.Minute},
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
	if resp != nil {
		resp.Body.Close()
	}

	if c.gatewayURL == "" {
		if err != nil {
			return nil, fmt.Errorf("ipfs cat %s: %w", cid, err)
		}
		return nil, fmt.Errorf("ipfs cat %s: HTTP %d", cid, resp.StatusCode)
	}
	// gateway fallback (read-only, plain GET)
	gwResp, gwErr := c.http.Get(c.gatewayURL + "/ipfs/" + url.PathEscape(cid))
	if gwErr != nil {
		return nil, fmt.Errorf("ipfs cat %s: node and gateway both failed: %v / %v", cid, err, gwErr)
	}
	if gwResp.StatusCode != http.StatusOK {
		gwResp.Body.Close()
		return nil, fmt.Errorf("ipfs cat %s: gateway HTTP %d", cid, gwResp.StatusCode)
	}
	return gwResp.Body, nil
}

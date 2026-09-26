package ipfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

// AddTemporaryContext is the streaming storage-boundary entry point. Like the
// legacy upload it does not pin; it makes no durable replication claim.
func (c *Client) AddTemporaryContext(ctx context.Context, name string, r io.Reader) (string, error) {
	if c.apiURL == "" {
		return "", fmt.Errorf("local Kubo is not configured")
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	done := make(chan error, 1)
	go func() {
		part, e := mw.CreateFormFile("file", name)
		if e == nil {
			_, e = io.CopyBuffer(part, r, make([]byte, 64<<10))
		}
		if e == nil {
			e = mw.Close()
		}
		_ = pw.CloseWithError(e)
		done <- e
	}()
	defer pr.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/v0/add?pin=false&cid-version=1", pr)
	if err != nil {
		return "", fmt.Errorf("ipfs request invalid")
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	// Unblock producer even if the endpoint rejects before consuming the body.
	_ = pr.Close()
	producerErr := <-done
	if err != nil {
		return "", fmt.Errorf("ipfs upload failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || producerErr != nil {
		return "", fmt.Errorf("ipfs upload failed")
	}
	var result addResponse
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil || result.Hash == "" {
		return "", fmt.Errorf("invalid ipfs response")
	}
	return result.Hash, nil
}
func (c *Client) GetFromGatewaysContext(ctx context.Context, cid string) (io.ReadCloser, error) {
	for _, base := range c.gatewayURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ipfs/"+url.PathEscape(cid), nil)
		if err != nil {
			continue
		}
		res, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if res.StatusCode == 200 {
			return res.Body, nil
		}
		res.Body.Close()
	}
	return nil, fmt.Errorf("ipfs gateways unavailable")
}

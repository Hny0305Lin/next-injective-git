package archivev1

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxArchiveResponseBytes = 16 << 20

type Client struct {
	LCD      string
	Contract string
	Height   uint64
	HTTP     *http.Client
}

func (c Client) SmartQuery(ctx context.Context, query json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(query) {
		return nil, errors.New("V1 smart query is not valid JSON")
	}
	endpoint, err := c.endpoint()
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(query)
	requestURL := endpoint + "/cosmwasm/wasm/v1/contract/" + url.PathEscape(c.Contract) + "/smart/" + url.PathEscape(encoded)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-cosmos-block-height", strconv.FormatUint(c.Height, 10))
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("query archived CosmWasm V1 state: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxArchiveResponseBytes {
		return nil, fmt.Errorf("archived V1 query response exceeds %d bytes", maxArchiveResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("archived V1 query HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode archived V1 query response: %w", err)
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return nil, errors.New("archived V1 query response has no data")
	}
	return append(json.RawMessage(nil), envelope.Data...), nil
}

func (c Client) endpoint() (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(c.LCD), "/")
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Fragment != "" {
		return "", errors.New("archive LCD endpoint must be an absolute http(s) URL without a fragment")
	}
	if strings.TrimSpace(c.Contract) == "" || strings.ContainsAny(c.Contract, " /?#") {
		return "", errors.New("archive CosmWasm contract address is invalid")
	}
	if c.Height == 0 {
		return "", errors.New("archive query height must be positive")
	}
	return endpoint, nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

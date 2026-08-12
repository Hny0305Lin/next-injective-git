package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultEVMRPCTimeout   = 30 * time.Second
	defaultReceiptInterval = 500 * time.Millisecond
	maxRPCAttempts         = 3
	initialRPCBackoff      = 100 * time.Millisecond
	maxRPCResponseBytes    = 8 << 20
)

// RPCError is a JSON-RPC error returned by an EVM node. Data is retained as a
// hex string when the node includes revert data so callers can decode custom
// Solidity errors instead of showing only a generic RPC failure.
type RPCError struct {
	Method  string
	Code    int64
	Message string
	Data    string
}

// rpcHTTPError keeps the status code separate from its human-readable body so
// Call can retry only gateway/rate-limit failures. Solidity/RPC errors are
// deliberately not retried because repeating a deterministic revert cannot
// change the outcome and would make user-visible failures slower.
type rpcHTTPError struct {
	Method string
	Status int
	Body   string
}

func (e *rpcHTTPError) Error() string {
	return fmt.Sprintf("EVM RPC %s HTTP %d: %s", e.Method, e.Status, e.Body)
}

type rpcTransportError struct {
	Method string
	Err    error
}

func (e *rpcTransportError) Error() string {
	return fmt.Sprintf("EVM RPC %s transport: %v", e.Method, e.Err)
}

func (e *rpcTransportError) Unwrap() error { return e.Err }

func (e *RPCError) Error() string {
	if e == nil {
		return "<nil>"
	}
	base := fmt.Sprintf("EVM RPC %s failed (code %d): %s", e.Method, e.Code, e.Message)
	if e.Data != "" {
		base += ": " + e.Data
	}
	return base
}

type rpcErrorBody struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcErrorBody   `json:"error,omitempty"`
}

// EVMRPC is a small, dependency-free JSON-RPC transport. It is deliberately
// independent of signing so it can be used for read-only calls and tests with
// httptest servers.
type EVMRPC struct {
	endpoint string
	client   *http.Client

	mu       sync.Mutex
	nextID   uint64
	interval time.Duration
}

// NewEVMRPC creates a transport with a bounded HTTP timeout and conservative
// receipt polling interval.
func NewEVMRPC(endpoint string) *EVMRPC {
	return &EVMRPC{
		endpoint: strings.TrimSpace(endpoint),
		client:   &http.Client{Timeout: defaultEVMRPCTimeout},
		interval: defaultReceiptInterval,
	}
}

// NewEVMRPCWithClient is intended for tests and callers that need a custom
// transport. A nil client falls back to the default bounded client.
func NewEVMRPCWithClient(endpoint string, client *http.Client) *EVMRPC {
	if client == nil {
		client = &http.Client{Timeout: defaultEVMRPCTimeout}
	}
	return &EVMRPC{
		endpoint: strings.TrimSpace(endpoint),
		client:   client,
		interval: defaultReceiptInterval,
	}
}

// SetReceiptInterval adjusts polling cadence. Values <= 0 restore the default.
// It is primarily useful for deterministic tests.
func (r *EVMRPC) SetReceiptInterval(interval time.Duration) {
	if interval <= 0 {
		interval = defaultReceiptInterval
	}
	r.mu.Lock()
	r.interval = interval
	r.mu.Unlock()
}

func (r *EVMRPC) nextRequestID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}

// Call performs one JSON-RPC request and decodes its result into out. The
// method preserves node error data for custom revert decoding.
func (r *EVMRPC) Call(ctx context.Context, method string, params any, out any) error {
	if strings.TrimSpace(r.endpoint) == "" {
		return fmt.Errorf("EVM RPC endpoint is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      r.nextRequestID(),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("encode EVM RPC %s: %w", method, err)
	}
	for attempt := 0; attempt < maxRPCAttempts; attempt++ {
		err := r.callOnce(ctx, method, body, out)
		if err == nil {
			return nil
		}
		if !retryableRPCError(err) || attempt == maxRPCAttempts-1 {
			return err
		}
		if err := waitRPCBackoff(ctx, attempt); err != nil {
			return fmt.Errorf("retry EVM RPC %s: %w", method, err)
		}
	}
	return fmt.Errorf("EVM RPC %s failed after %d attempts", method, maxRPCAttempts)
}

func (r *EVMRPC) callOnce(ctx context.Context, method string, body []byte, out any) error {
	reqBody := bytes.NewReader(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, reqBody)
	if err != nil {
		return fmt.Errorf("create EVM RPC %s request: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(request)
	if err != nil {
		return &rpcTransportError{Method: method, Err: err}
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRPCResponseBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return &rpcTransportError{Method: method, Err: fmt.Errorf("read response: %w", readErr)}
	}
	if closeErr != nil {
		return &rpcTransportError{Method: method, Err: fmt.Errorf("close response: %w", closeErr)}
	}
	if len(responseBody) > maxRPCResponseBytes {
		return fmt.Errorf("EVM RPC %s response exceeds %d bytes", method, maxRPCResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &rpcHTTPError{Method: method, Status: resp.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	var decoded rpcResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return fmt.Errorf("decode EVM RPC %s response: %w", method, err)
	}
	if decoded.Error != nil {
		return &RPCError{
			Method:  method,
			Code:    decoded.Error.Code,
			Message: decoded.Error.Message,
			Data:    rpcErrorData(decoded.Error.Data),
		}
	}
	if len(decoded.Result) == 0 || bytes.Equal(bytes.TrimSpace(decoded.Result), []byte("null")) {
		return nil
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(decoded.Result, out); err != nil {
		return fmt.Errorf("decode EVM RPC %s result: %w", method, err)
	}
	return nil
}

func retryableRPCError(err error) bool {
	var httpErr *rpcHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status == http.StatusRequestTimeout ||
			httpErr.Status == http.StatusTooManyRequests ||
			httpErr.Status >= 500
	}
	var transportErr *rpcTransportError
	if errors.As(err, &transportErr) {
		return !errors.Is(transportErr.Err, context.Canceled) &&
			!errors.Is(transportErr.Err, context.DeadlineExceeded)
	}
	return false
}

func waitRPCBackoff(ctx context.Context, attempt int) error {
	delay := initialRPCBackoff * time.Duration(1<<attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func rpcErrorData(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"data", "originalError", "error"} {
			if nested, ok := object[key]; ok {
				if found := rpcErrorData(nested); found != "" {
					return found
				}
			}
		}
	}
	return strings.TrimSpace(string(raw))
}

// EVMCall is the subset of an EVM transaction accepted by eth_call and
// eth_estimateGas.
type EVMCall struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Gas   string `json:"gas,omitempty"`
	Value string `json:"value,omitempty"`
	Data  string `json:"data,omitempty"`
}

type EVMReceipt struct {
	TransactionHash string   `json:"transactionHash"`
	BlockNumber     string   `json:"blockNumber"`
	BlockHash       string   `json:"blockHash"`
	To              string   `json:"to"`
	Status          string   `json:"status"`
	GasUsed         string   `json:"gasUsed"`
	Logs            []EVMLog `json:"logs"`
}

type EVMLog struct {
	Address         string   `json:"address"`
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	BlockNumber     string   `json:"blockNumber"`
	BlockHash       string   `json:"blockHash"`
	TransactionHash string   `json:"transactionHash"`
	Removed         bool     `json:"removed"`
}

type EVMBlock struct {
	Number string `json:"number"`
	Hash   string `json:"hash"`
}

// TransactionRevertedError indicates a mined transaction with status 0x0.
type TransactionRevertedError struct {
	Hash   string
	Status string
}

func (e *TransactionRevertedError) Error() string {
	status := e.Status
	if status == "" {
		status = "0x0"
	}
	return fmt.Sprintf("EVM transaction %s reverted (status %s)", e.Hash, status)
}

func (r *EVMRPC) ChainID(ctx context.Context) (uint64, error) {
	var raw string
	if err := r.Call(ctx, "eth_chainId", []any{}, &raw); err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("eth_chainId returned null result")
	}
	return parseHexUint(raw)
}

// BlockNumber returns the latest block height. Callers that drain a mutable
// index across multiple eth_call requests can pin every page to this height so
// swap-pop removals in a later block cannot make entries move behind a cursor.
func (r *EVMRPC) BlockNumber(ctx context.Context) (uint64, error) {
	var raw string
	if err := r.Call(ctx, "eth_blockNumber", []any{}, &raw); err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("eth_blockNumber returned null result")
	}
	return parseHexUint(raw)
}

// BlockByNumber resolves a block tag to its canonical number and hash. State
// exporters can compare this hash with a transaction receipt before and after
// paginated reads to detect a reorg at the pinned height.
func (r *EVMRPC) BlockByNumber(ctx context.Context, blockTag string) (*EVMBlock, error) {
	if strings.TrimSpace(blockTag) == "" {
		blockTag = "latest"
	}
	var block *EVMBlock
	if err := r.Call(ctx, "eth_getBlockByNumber", []any{blockTag, false}, &block); err != nil {
		return nil, err
	}
	if block == nil || strings.TrimSpace(block.Number) == "" || strings.TrimSpace(block.Hash) == "" {
		return nil, fmt.Errorf("eth_getBlockByNumber returned an incomplete block")
	}
	return block, nil
}

func (r *EVMRPC) CallContract(ctx context.Context, to, data string) ([]byte, error) {
	return r.CallContractAt(ctx, to, data, "latest")
}

// CallContractAt evaluates calldata against a specific block tag. An empty
// tag preserves the ordinary latest-state behavior for non-paginated reads.
func (r *EVMRPC) CallContractAt(ctx context.Context, to, data, blockTag string) ([]byte, error) {
	if strings.TrimSpace(blockTag) == "" {
		blockTag = "latest"
	}
	var raw string
	if err := r.Call(ctx, "eth_call", []any{EVMCall{To: to, Data: normalizeHex(data)}, blockTag}, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("eth_call returned null result")
	}
	return decodeHexBytes(raw)
}

func (r *EVMRPC) EstimateGas(ctx context.Context, call EVMCall) (uint64, error) {
	var raw string
	if err := r.Call(ctx, "eth_estimateGas", []any{call}, &raw); err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("eth_estimateGas returned null result")
	}
	return parseHexUint(raw)
}

func (r *EVMRPC) TransactionCount(ctx context.Context, address, blockTag string) (uint64, error) {
	if strings.TrimSpace(blockTag) == "" {
		blockTag = "pending"
	}
	var raw string
	if err := r.Call(ctx, "eth_getTransactionCount", []any{address, blockTag}, &raw); err != nil {
		return 0, err
	}
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("eth_getTransactionCount returned null result")
	}
	return parseHexUint(raw)
}

// GasPrice returns the node's current legacy gas price as a normalized hex
// quantity. The signer uses it for a replay-protected legacy transaction;
// Injective's EVM RPC accepts this form and it keeps the signer independent of
// fee-market-specific extensions.
func (r *EVMRPC) GasPrice(ctx context.Context) (string, error) {
	var raw string
	if err := r.Call(ctx, "eth_gasPrice", []any{}, &raw); err != nil {
		return "", err
	}
	if _, err := parseHexUint(raw); err != nil {
		return "", err
	}
	return normalizeHex(raw), nil
}

func (r *EVMRPC) SendRawTransaction(ctx context.Context, rawTx string) (string, error) {
	var hash string
	if err := r.Call(ctx, "eth_sendRawTransaction", []any{normalizeHex(rawTx)}, &hash); err != nil {
		return "", err
	}
	if strings.TrimSpace(hash) == "" {
		return "", fmt.Errorf("eth_sendRawTransaction returned an empty transaction hash")
	}
	return hash, nil
}

// TransactionReceipt returns nil when a node reports a pending transaction.
func (r *EVMRPC) TransactionReceipt(ctx context.Context, hash string) (*EVMReceipt, error) {
	var receipt *EVMReceipt
	if err := r.Call(ctx, "eth_getTransactionReceipt", []any{hash}, &receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

// WaitReceipt polls until a receipt is mined, the context is canceled, or the
// mined transaction reports status 0x0.
func (r *EVMRPC) WaitReceipt(ctx context.Context, hash string) (*EVMReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		receipt, err := r.TransactionReceipt(ctx, hash)
		if err != nil {
			return nil, err
		}
		if receipt != nil {
			if strings.EqualFold(strings.TrimSpace(receipt.Status), "0x0") || strings.TrimSpace(receipt.Status) == "0" {
				return receipt, &TransactionRevertedError{Hash: hash, Status: receipt.Status}
			}
			return receipt, nil
		}
		r.mu.Lock()
		interval := r.interval
		r.mu.Unlock()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, fmt.Errorf("wait for EVM transaction %s receipt: %w", hash, ctx.Err())
		case <-timer.C:
		}
	}
}

func parseHexUint(raw string) (uint64, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if value == "" {
		return 0, nil
	}
	if len(value) > 16 {
		return 0, fmt.Errorf("hex quantity %q overflows uint64", raw)
	}
	parsed, err := hex.DecodeString(normalizeOddHex(value))
	if err != nil {
		return 0, fmt.Errorf("invalid hex quantity %q: %w", raw, err)
	}
	var out uint64
	for _, b := range parsed {
		out = (out << 8) | uint64(b)
	}
	return out, nil
}

func decodeHexBytes(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if value == "" {
		return nil, nil
	}
	if len(value)%2 != 0 {
		return nil, fmt.Errorf("hex data %q has odd length", raw)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid hex data: %w", err)
	}
	return decoded, nil
}

func normalizeHex(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0x"
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		return "0x" + value[2:]
	}
	return "0x" + value
}

func normalizeOddHex(value string) string {
	if len(value)%2 == 1 {
		return "0" + value
	}
	return value
}

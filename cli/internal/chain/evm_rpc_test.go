package chain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEVMRPCMethodsAndReceiptPolling(t *testing.T) {
	var receiptCalls atomic.Int32
	var methods []string
	var callTags []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		result := any(nil)
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_blockNumber":
			result = "0x1234"
		case "eth_call":
			var params []json.RawMessage
			if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) != 2 {
				t.Errorf("decode eth_call params: %v", err)
			} else {
				var blockTag string
				if err := json.Unmarshal(params[1], &blockTag); err != nil {
					t.Errorf("decode eth_call block tag: %v", err)
				} else {
					callTags = append(callTags, blockTag)
				}
			}
			result = "0x1234"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_getTransactionCount":
			result = "0x3"
		case "eth_getBalance":
			result = "0xde0b6b3a7640000"
		case "eth_sendRawTransaction":
			result = "0xabc123"
		case "eth_getTransactionReceipt":
			if receiptCalls.Add(1) == 1 {
				result = nil
			} else {
				result = map[string]string{
					"transactionHash": "0xabc123",
					"blockNumber":     "0x10",
					"status":          "0x1",
					"gasUsed":         "0x5208",
				}
			}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	rpc := NewEVMRPC(server.URL)
	rpc.SetReceiptInterval(time.Millisecond)

	chainID, err := rpc.ChainID(context.Background())
	if err != nil || chainID != 0x7a69 {
		t.Fatalf("chain ID = %d, err=%v", chainID, err)
	}
	blockNumber, err := rpc.BlockNumber(context.Background())
	if err != nil || blockNumber != 0x1234 {
		t.Fatalf("block number = %d, err=%v", blockNumber, err)
	}
	result, err := rpc.CallContract(context.Background(), "0x1111111111111111111111111111111111111111", "1234")
	if err != nil || string(result) != string([]byte{0x12, 0x34}) {
		t.Fatalf("eth_call result = %x, err=%v", result, err)
	}
	pinnedResult, err := rpc.CallContractAt(context.Background(), "0x1111111111111111111111111111111111111111", "1234", "0x1234")
	if err != nil || string(pinnedResult) != string([]byte{0x12, 0x34}) {
		t.Fatalf("pinned eth_call result = %x, err=%v", pinnedResult, err)
	}
	gas, err := rpc.EstimateGas(context.Background(), EVMCall{To: "0x1111111111111111111111111111111111111111", Data: "0x1234"})
	if err != nil || gas != 0x5208 {
		t.Fatalf("gas = %d, err=%v", gas, err)
	}
	nonce, err := rpc.TransactionCount(context.Background(), "0x1111111111111111111111111111111111111111", "pending")
	if err != nil || nonce != 3 {
		t.Fatalf("nonce = %d, err=%v", nonce, err)
	}
	balance, err := rpc.Balance(context.Background(), "0x1111111111111111111111111111111111111111", "latest")
	if err != nil || balance.String() != "1000000000000000000" {
		t.Fatalf("balance = %v, err=%v", balance, err)
	}
	hash, err := rpc.SendRawTransaction(context.Background(), "1234")
	if err != nil || hash != "0xabc123" {
		t.Fatalf("hash = %q, err=%v", hash, err)
	}
	receipt, err := rpc.WaitReceipt(context.Background(), hash)
	if err != nil || receipt == nil || receipt.Status != "0x1" {
		t.Fatalf("receipt = %#v, err=%v", receipt, err)
	}
	if len(methods) != 10 || strings.Join(methods, ",") != "eth_chainId,eth_blockNumber,eth_call,eth_call,eth_estimateGas,eth_getTransactionCount,eth_getBalance,eth_sendRawTransaction,eth_getTransactionReceipt,eth_getTransactionReceipt" {
		t.Fatalf("RPC methods = %v", methods)
	}
	if strings.Join(callTags, ",") != "latest,0x1234" {
		t.Fatalf("eth_call block tags = %v, want [latest 0x1234]", callTags)
	}
}

func TestEVMRPCPreservesErrorData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]any{
				"code":    -32000,
				"message": "execution reverted",
				"data":    "0xdeadbeef",
			},
		})
	}))
	defer server.Close()
	var out string
	err := NewEVMRPC(server.URL).Call(context.Background(), "eth_call", []any{}, &out)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want *RPCError", err)
	}
	if rpcErr.Data != "0xdeadbeef" || rpcErr.Method != "eth_call" {
		t.Fatalf("RPC error = %#v", rpcErr)
	}
}

func TestEVMRPCRetriesTransientHTTPFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) < maxRPCAttempts {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporarily unavailable"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  "0x7a69",
		})
	}))
	defer server.Close()
	var result string
	if err := NewEVMRPC(server.URL).Call(context.Background(), "eth_chainId", []any{}, &result); err != nil {
		t.Fatalf("transient RPC call failed: %v", err)
	}
	if result != "0x7a69" || calls.Load() != maxRPCAttempts {
		t.Fatalf("result=%q calls=%d, want result 0x7a69 after %d attempts", result, calls.Load(), maxRPCAttempts)
	}
}

func TestEVMRPCDoesNotRetryDeterministicRPCError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]any{
				"code":    3,
				"message": "execution reverted",
				"data":    "0xdeadbeef",
			},
		})
	}))
	defer server.Close()
	var result string
	err := NewEVMRPC(server.URL).Call(context.Background(), "eth_call", []any{}, &result)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("error=%v calls=%d, want one deterministic RPC failure", err, calls.Load())
	}
}

func TestEVMRPCRetryBackoffHonorsContext(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts.Add(1)
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := NewEVMRPC(server.URL).Call(ctx, "eth_chainId", []any{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempt count = %d, want no retry after cancellation", got)
	}
}

func TestEVMRPCWaitReceiptReportsRevert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]string{
				"transactionHash": "0xbad",
				"status":          "0x0",
			},
		})
	}))
	defer server.Close()
	receipt, err := NewEVMRPC(server.URL).WaitReceipt(context.Background(), "0xbad")
	var reverted *TransactionRevertedError
	if !errors.As(err, &reverted) || receipt == nil {
		t.Fatalf("receipt=%#v err=%v, want mined revert", receipt, err)
	}
}

func TestEVMRPCWaitReceiptHonorsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": nil})
	}))
	defer server.Close()
	rpc := NewEVMRPC(server.URL)
	rpc.SetReceiptInterval(time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := rpc.WaitReceipt(ctx, "0xpending")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %v, want context deadline", err)
	}
}

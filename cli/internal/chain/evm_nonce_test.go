package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
)

func TestEVMNonceManagerReservesDistinctConcurrentNonces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Method != "eth_getTransactionCount" {
			t.Errorf("method = %q, want eth_getTransactionCount", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  "0x4",
		})
	}))
	defer server.Close()

	manager := newEVMNonceManager()
	rpc := NewEVMRPC(server.URL)
	const count = 16
	nonces := make([]uint64, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			nonce, reservation, err := manager.reserve(
				context.Background(), rpc, 1439,
				"0x1111111111111111111111111111111111111111",
			)
			errs[index] = err
			nonces[index] = nonce
			if err == nil {
				reservation.Commit()
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("reservation %d failed: %v", i, err)
		}
	}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	for i, nonce := range nonces {
		want := uint64(4 + i)
		if nonce != want {
			t.Fatalf("sorted nonce[%d] = %d, want %d (all nonces: %v)", i, nonce, want, nonces)
		}
	}
}

func TestEVMNonceReservationReleaseReusesNonce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  "0x9",
		})
	}))
	defer server.Close()
	manager := newEVMNonceManager()
	rpc := NewEVMRPC(server.URL)

	first, reservation, err := manager.reserve(context.Background(), rpc, 1439, "0x1111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if first != 9 {
		t.Fatalf("first nonce = %d, want 9", first)
	}
	reservation.Release()

	second, reservation, err := manager.reserve(context.Background(), rpc, 1439, "0x1111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	reservation.Commit()
	if second != first {
		t.Fatalf("released nonce = %d, want reuse of %d", second, first)
	}
}

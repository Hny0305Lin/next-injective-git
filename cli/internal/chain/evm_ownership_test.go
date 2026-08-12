package chain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestEVMOwnershipTransferWritesUseStableRepoID(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	target := "0x2222222222222222222222222222222222222222"
	var repoID [32]byte
	for i := range repoID {
		repoID[i] = byte(i + 1)
	}
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var calldata []string
	server := ownershipRPCServer(t, func(request rpcRequest) (any, any) {
		switch request.Method {
		case "eth_estimateGas":
			var params []map[string]any
			if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil {
				t.Errorf("decode estimate params: %v", err)
			} else if len(params) == 1 {
				if data, ok := params[0]["data"].(string); ok {
					calldata = append(calldata, data)
				}
			}
			return "0x5208", nil
		case "eth_chainId", "eth_getTransactionCount", "eth_gasPrice":
			if request.Method == "eth_chainId" {
				return "0x7a69", nil
			}
			if request.Method == "eth_getTransactionCount" {
				return "0x0", nil
			}
			return "0x3b9aca00", nil
		case "eth_sendRawTransaction":
			return "0xhash", nil
		case "eth_getTransactionReceipt":
			return map[string]string{"transactionHash": "0xhash", "status": "0x1"}, nil
		default:
			return nil, errors.New("unexpected RPC method " + request.Method)
		}
	})
	defer server.Close()

	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	repo := &ResolvedRepo{
		RepoID:      repoID,
		Backend:     BackendEVM,
		Requested:   RepoLocator{Owner: owner, Name: "demo"},
		Canonical:   RepoLocator{Owner: owner, Name: "demo"},
		IsCanonical: true,
	}
	targetUser, err := userAddressFromEVM(target)
	if err != nil {
		t.Fatal(err)
	}
	actions := []struct {
		name string
		call func() error
	}{
		{name: "begin", call: func() error { return backend.BeginOwnershipTransfer(repo, targetUser) }},
		{name: "cancel", call: func() error { return backend.CancelOwnershipTransfer(repo) }},
		{name: "reject", call: func() error { return backend.RejectOwnershipTransfer(repo) }},
		{name: "expire", call: func() error { return backend.ExpireOwnershipTransfer(repo) }},
		{name: "accept", call: func() error { return backend.AcceptOwnership(repo) }},
	}
	for _, action := range actions {
		if err := action.call(); err != nil {
			t.Fatalf("%s ownership transfer failed: %v", action.name, err)
		}
	}
	wantSelectors := []string{
		selectorFor("beginOwnershipTransfer(bytes32,address)"),
		selectorFor("cancelOwnershipTransfer(bytes32)"),
		selectorFor("rejectOwnershipTransfer(bytes32)"),
		selectorFor("expireOwnershipTransfer(bytes32)"),
		selectorFor("acceptOwnership(bytes32)"),
	}
	if len(calldata) != len(wantSelectors) {
		t.Fatalf("estimate calldata count = %d, want %d", len(calldata), len(wantSelectors))
	}
	for i, data := range calldata {
		raw, err := decodeHexBytes(data)
		if err != nil {
			t.Fatalf("decode calldata %d: %v", i, err)
		}
		if len(raw) < 4+32 || hex.EncodeToString(raw[:4]) != wantSelectors[i] {
			t.Fatalf("calldata %d selector/length = %x, want selector %s", i, raw, wantSelectors[i])
		}
		if !equalBytes(raw[4:4+32], repoID[:]) {
			t.Fatalf("calldata %d repo id = %x, want %x", i, raw[4:36], repoID)
		}
		if i == 0 {
			wantTarget, _ := parseEVMAddress(target)
			if !equalBytes(raw[4+32+12:4+32+32], wantTarget) {
				t.Fatalf("begin target = %x, want %x", raw[4+32+12:4+32+32], wantTarget)
			}
		}
	}
}

func TestEVMOwnershipTransferAliasWriteFailsBeforeRPC(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		t.Errorf("unexpected RPC request %s", req.Method)
	}))
	defer server.Close()
	var repoID [32]byte
	repoID[31] = 9
	backend := NewEVMRegistryV2WithDependencies(
		testEVMConfig(server.URL),
		NewEVMRPC(server.URL),
		&fakeEVMSigner{address: "0x1111111111111111111111111111111111111111", raw: "0x1234"},
	)
	repo := &ResolvedRepo{
		RepoID:      repoID,
		Backend:     BackendEVM,
		Requested:   RepoLocator{Owner: "0x1111111111111111111111111111111111111111", Name: "old"},
		Canonical:   RepoLocator{Owner: "0x2222222222222222222222222222222222222222", Name: "new"},
		IsCanonical: false,
	}
	err := backend.CancelOwnershipTransfer(repo)
	var moved *RepoMovedError
	if !errors.As(err, &moved) {
		t.Fatalf("error = %v, want RepoMovedError", err)
	}
	if got := moved.CanonicalURL(); !strings.HasPrefix(got, "igit://") {
		t.Fatalf("canonical URL = %q", got)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("RPC calls = %d, want none", got)
	}
}

func TestEVMOwnershipTransferRevertDoesNotFallbackToV1(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var legacyCalls atomic.Int32
	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "eth_chainId":
			response["result"] = "0x7a69"
		case "eth_getTransactionCount":
			response["result"] = "0x0"
		case "eth_estimateGas":
			response["error"] = map[string]any{
				"code": 3, "message": "execution reverted",
				"data": customErrorData("TransferNotPending(bytes32)", make([]byte, 32)),
			}
		default:
			t.Errorf("unexpected RPC method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer evmServer.Close()
	lcdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		legacyCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer lcdServer.Close()

	cfg := testEVMConfig(evmServer.URL)
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.LCDEndpoint = lcdServer.URL
	backend := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(evmServer.URL), &fakeEVMSigner{address: owner, raw: "0x1234"})
	var repoID [32]byte
	repoID[31] = 1
	err := backend.CancelOwnershipTransfer(&ResolvedRepo{RepoID: repoID, Backend: BackendEVM, IsCanonical: true})
	if err == nil || !strings.Contains(err.Error(), "ownership transfer is not pending") {
		t.Fatalf("error = %v, want decoded TransferNotPending error", err)
	}
	if got := legacyCalls.Load(); got != 0 {
		t.Fatalf("legacy calls = %d, want no V1 fallback", got)
	}
}

func TestDecodeEVMPendingOwnershipTransferIncludesExpiry(t *testing.T) {
	var target [20]byte
	for i := range target {
		target[i] = byte(i + 1)
	}
	result := make([]byte, 4*32)
	copy(result[12:32], target[:])
	putABIWord(result[32:64], 11)
	putABIWord(result[64:96], 22)
	putABIWord(result[96:128], 33)
	transfer, err := decodeEVMPendingOwnershipTransfer(result)
	if err != nil {
		t.Fatal(err)
	}
	if transfer == nil || transfer.ProposedAt != 11 || transfer.ExecuteAfter != 22 || transfer.ExpiresAt != 33 {
		t.Fatalf("pending transfer = %#v", transfer)
	}
	wantOwner, err := userAddressFromEVM("0x" + hex.EncodeToString(target[:]))
	if err != nil {
		t.Fatal(err)
	}
	if transfer.NewOwner != wantOwner {
		t.Fatalf("new owner = %q, want %q", transfer.NewOwner, wantOwner)
	}
}

func TestEVMPendingOwnershipTransferUsesStableRepoID(t *testing.T) {
	var repoID [32]byte
	repoID[0] = 0xa5
	repoID[31] = 0x5a
	target := "0x2222222222222222222222222222222222222222"
	result := make([]byte, 4*32)
	targetBytes, _ := parseEVMAddress(target)
	copy(result[12:32], targetBytes)
	putABIWord(result[32:64], 11)
	putABIWord(result[64:96], 22)
	putABIWord(result[96:128], 33)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		if request.Method != "eth_call" {
			t.Errorf("method = %s, want eth_call", request.Method)
		}
		var params []json.RawMessage
		if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) == 0 {
			t.Errorf("decode eth_call params: %v", err)
		} else {
			var call map[string]any
			if err := json.Unmarshal(params[0], &call); err != nil {
				t.Errorf("decode eth_call object: %v", err)
			}
			data, _ := call["data"].(string)
			raw, err := decodeHexBytes(data)
			if err != nil {
				t.Errorf("decode pending calldata: %v", err)
			} else if len(raw) != 4+32 || hex.EncodeToString(raw[:4]) != selectorFor("pendingOwnershipTransfer(bytes32)") || !equalBytes(raw[4:], repoID[:]) {
				t.Errorf("pending calldata = %x", raw)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2(testEVMConfig(server.URL))
	transfer, err := backend.PendingOwnershipTransfer(&ResolvedRepo{
		RepoID: repoID, Backend: BackendEVM, IsCanonical: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if transfer == nil || transfer.ProposedAt != 11 || transfer.ExecuteAfter != 22 || transfer.ExpiresAt != 33 {
		t.Fatalf("pending transfer = %#v", transfer)
	}
}

func TestDecodeEVMPendingOwnershipTransferZeroIsNil(t *testing.T) {
	transfer, err := decodeEVMPendingOwnershipTransfer(make([]byte, 4*32))
	if err != nil {
		t.Fatal(err)
	}
	if transfer != nil {
		t.Fatalf("pending transfer = %#v, want nil", transfer)
	}
}

func TestEVMPendingOwnershipTransferRejectsUnknownBackend(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		t.Errorf("unexpected RPC request %s", req.Method)
	}))
	defer server.Close()
	backend := NewEVMRegistryV2(testEVMConfig(server.URL))
	_, err := backend.PendingOwnershipTransfer(&ResolvedRepo{Backend: BackendKind("unknown")})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("error = %v, want unknown backend rejection", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("RPC calls = %d, want none", got)
	}
}

func ownershipRPCServer(t *testing.T, handler func(rpcRequest) (result any, rpcErr any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		result, rpcErr := handler(request)
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if rpcErr != nil {
			response["error"] = rpcErr
		} else {
			response["result"] = result
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
}

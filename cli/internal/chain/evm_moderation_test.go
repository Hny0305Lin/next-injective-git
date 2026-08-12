package chain

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

const testModerationModuleAddress = "0x6666666666666666666666666666666666666666"

func TestEVMModerationStatusUsesStableRepoIDAndReceiptPipeline(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0], repoID[31] = 0xa5, 0x5a
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	var estimate EVMCall

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "eth_call":
			to, data, _ := testEthCallTargetDataAndTag(t, request)
			if !strings.EqualFold(to, "0x2222222222222222222222222222222222222222") || data[2:10] != selectorFor("resolveRepo(address,string)") {
				t.Errorf("resolve call target/data = %s/%s", to, data)
			}
			result = "0x" + hex.EncodeToString(resolved)
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x0"
		case "eth_estimateGas":
			var params []EVMCall
			_ = json.Unmarshal(mustJSON(request.Params), &params)
			estimate = params[0]
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x1"
		case "eth_sendRawTransaction":
			result = "0xhash"
		case "eth_getTransactionReceipt":
			result = map[string]string{"transactionHash": "0xhash", "status": "0x1"}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	cfg := testEVMConfig(server.URL)
	cfg.EVMModerationModuleAddress = testModerationModuleAddress
	registry := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	moderation := NewEVMModerationModuleWithDependencies(cfg, registry, registry.rpc, signer)
	if err := moderation.SetModerationStatus(owner, "demo", "frozen", "decision"); err != nil {
		t.Fatal(err)
	}
	wantMethods := "eth_call,eth_chainId,eth_getTransactionCount,eth_estimateGas,eth_gasPrice,eth_sendRawTransaction,eth_getTransactionReceipt"
	if got := strings.Join(methods, ","); got != wantMethods {
		t.Fatalf("RPC methods = %s, want %s", got, wantMethods)
	}
	if !strings.EqualFold(estimate.To, testModerationModuleAddress) {
		t.Fatalf("estimate target = %s, want moderation module", estimate.To)
	}
	raw, err := decodeHexBytes(estimate.Data)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(raw[:4]) != selectorFor("setModerationStatus(bytes32,uint8,string)") || !equalBytes(raw[4:36], repoID[:]) {
		t.Fatalf("moderation calldata does not bind stable repo ID: %x", raw)
	}
	if status, err := readABIUint(raw[4:], 32); err != nil || status != 1 {
		t.Fatalf("status = %d, err=%v", status, err)
	}
}

func TestEVMModerationReceiptFailureIsReturned(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x0"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x1"
		case "eth_sendRawTransaction":
			result = "0xhash"
		case "eth_getTransactionReceipt":
			result = map[string]string{"transactionHash": "0xhash", "status": "0x0"}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	cfg := testEVMConfig(server.URL)
	cfg.EVMModerationModuleAddress = testModerationModuleAddress
	registry := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	moderation := NewEVMModerationModuleWithDependencies(cfg, registry, registry.rpc, signer)
	err := moderation.ResolveModerationReport(7, "delisted", "decision")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "revert") {
		t.Fatalf("receipt error = %v, want reverted receipt", err)
	}
}

func TestEVMModerationReportDecodeUsesSelectedModuleOnly(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	reporter := "0x3333333333333333333333333333333333333333"
	result := testModerationReportResult(7, owner, "demo", reporter, "report", 3, 0, true, "resolved", true, "appeal", 10, 20)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		calls.Add(1)
		to, data, tag := testEthCallTargetDataAndTag(t, request)
		if !strings.EqualFold(to, testModerationModuleAddress) || data[2:10] != selectorFor("getReport(uint256)") || tag != "latest" {
			t.Errorf("report call = %s/%s/%s", to, data, tag)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result)})
	}))
	defer server.Close()

	cfg := testEVMConfig(server.URL)
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.EVMModerationModuleAddress = testModerationModuleAddress
	moderation := NewEVMModerationModule(cfg)
	report, err := moderation.ModerationReport(7)
	if err != nil {
		t.Fatal(err)
	}
	if report.ID != 7 || !strings.HasPrefix(report.Owner, "inj1") || !strings.HasPrefix(report.Reporter, "inj1") || report.Repo != "demo" || report.Status != "appeal_resolved" || report.Resolution != "active" || report.ResolutionHash == nil || *report.ResolutionHash != "resolved" || report.AppealHash == nil || *report.AppealHash != "appeal" {
		t.Fatalf("decoded report = %#v", report)
	}
	if calls.Load() != 1 {
		t.Fatalf("report ID query made %d calls, want one selected-module call", calls.Load())
	}
}

func TestEVMModerationLegacyLocatorReadNeverWritesV1(t *testing.T) {
	owner := config.DefaultContractAddress
	var legacyQueries atomic.Int32
	var legacyWrites atomic.Int32
	var transactionRPCs atomic.Int32

	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if request.Method != "eth_call" {
			transactionRPCs.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID,
			"error": map[string]any{"code": 3, "message": "execution reverted", "data": locatorNotFoundData(owner, "legacy")},
		})
	}))
	defer evmServer.Close()

	lcdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			legacyWrites.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		legacyQueries.Add(1)
		encoded, _ := url.PathUnescape(path.Base(req.URL.Path))
		message, _ := base64.StdEncoding.DecodeString(encoded)
		var query map[string]json.RawMessage
		_ = json.Unmarshal(message, &query)
		if query["repo_info"] == nil {
			t.Fatalf("unexpected legacy query: %s", message)
		}
		payload := map[string]any{"owner": owner, "name": "legacy", "description": "old", "default_branch": "main", "created_at": 1, "updated_at": 2, "moderation_status": "active"}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
	}))
	defer lcdServer.Close()

	cfg := testEVMConfig(evmServer.URL)
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.LCDEndpoint = lcdServer.URL
	cfg.EVMModerationModuleAddress = testModerationModuleAddress
	moderation := NewEVMModerationModule(cfg)
	if err := moderation.SubmitModerationReport(owner, "legacy", "report"); !errors.Is(err, ErrLegacyWriteFallbackDisabled) {
		t.Fatalf("report error = %v, want ErrLegacyWriteFallbackDisabled", err)
	}
	if err := moderation.SetModerationStatus(owner, "legacy", "frozen", "decision"); !errors.Is(err, ErrLegacyWriteFallbackDisabled) {
		t.Fatalf("status error = %v, want ErrLegacyWriteFallbackDisabled", err)
	}
	if legacyQueries.Load() != 2 || legacyWrites.Load() != 0 || transactionRPCs.Load() != 0 {
		t.Fatalf("legacy queries/writes/transaction RPCs = %d/%d/%d, want 2/0/0", legacyQueries.Load(), legacyWrites.Load(), transactionRPCs.Load())
	}
}

func testModerationReportResult(
	id uint64, owner, repo, reporter, reason string, status, resolution uint64,
	hasResolution bool, resolutionHash string, hasAppeal bool, appealHash string,
	createdAt, updatedAt uint64,
) []byte {
	const headSize = 14 * 32
	repoTail := encodeABIString(repo)
	reasonTail := encodeABIString(reason)
	resolutionTail := encodeABIString(resolutionHash)
	appealTail := encodeABIString(appealHash)
	tuple := make([]byte, headSize)
	putABIWord(tuple[0:32], id)
	ownerRaw, _ := parseEVMAddress(owner)
	copy(tuple[64+12:96], ownerRaw)
	putABIWord(tuple[96:128], headSize)
	reporterRaw, _ := parseEVMAddress(reporter)
	copy(tuple[128+12:160], reporterRaw)
	putABIWord(tuple[160:192], uint64(headSize+len(repoTail)))
	putABIWord(tuple[192:224], status)
	putABIWord(tuple[224:256], resolution)
	if hasResolution {
		tuple[256+31] = 1
	}
	putABIWord(tuple[288:320], uint64(headSize+len(repoTail)+len(reasonTail)))
	putABIWord(tuple[320:352], uint64(headSize+len(repoTail)+len(reasonTail)+len(resolutionTail)))
	if hasAppeal {
		tuple[352+31] = 1
	}
	putABIWord(tuple[384:416], createdAt)
	putABIWord(tuple[416:448], updatedAt)
	tuple = append(tuple, repoTail...)
	tuple = append(tuple, reasonTail...)
	tuple = append(tuple, resolutionTail...)
	tuple = append(tuple, appealTail...)
	result := make([]byte, 32)
	putABIWord(result, 32)
	return append(result, tuple...)
}

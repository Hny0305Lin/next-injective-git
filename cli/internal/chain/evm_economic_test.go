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

const testEconomicModuleAddress = "0x5555555555555555555555555555555555555555"

func TestEVMEconomicSponsorTargetsModuleWithExactValueAndReceiptPipeline(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0], repoID[31] = 0xa5, 0x5a
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	var estimate EVMCall

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "eth_call":
			to, data, blockTag := testEthCallTargetDataAndTag(t, request)
			if !strings.EqualFold(to, "0x2222222222222222222222222222222222222222") {
				t.Errorf("resolve target = %s", to)
			}
			if blockTag != "latest" || data[2:10] != selectorFor("resolveRepo(address,string)") {
				t.Errorf("resolve call = %s at %s", data, blockTag)
			}
			result = "0x" + hex.EncodeToString(resolved)
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			var params []EVMCall
			if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) != 1 {
				t.Fatalf("decode estimate params: %v", err)
			}
			estimate = params[0]
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x3b9aca00"
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
	cfg.EVMEconomicModuleAddress = testEconomicModuleAddress
	registry := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	economic := NewEVMEconomicModuleWithDependencies(cfg, registry, registry.rpc, signer)
	const baseUnits = "125000000000000000inj"
	if err := economic.Sponsor(owner, "demo", "great work", baseUnits); err != nil {
		t.Fatal(err)
	}

	wantMethods := "eth_call,eth_chainId,eth_getTransactionCount,eth_estimateGas,eth_gasPrice,eth_sendRawTransaction,eth_getTransactionReceipt"
	if got := strings.Join(methods, ","); got != wantMethods {
		t.Fatalf("RPC methods = %s, want %s", got, wantMethods)
	}
	// Compare quantities numerically because the expected decimal base units
	// are converted to a canonical hexadecimal transaction value.
	wantValue, _ := evmINJValue(baseUnits)
	if !strings.EqualFold(estimate.To, testEconomicModuleAddress) || estimate.Value != wantValue {
		t.Fatalf("estimate call = %#v, want module/value %s/%s", estimate, testEconomicModuleAddress, wantValue)
	}
	txs := signer.transactions()
	if len(txs) != 1 || txs[0].Value != wantValue || !strings.EqualFold(txs[0].To, testEconomicModuleAddress) {
		t.Fatalf("signed transactions = %#v", txs)
	}
	raw, err := decodeHexBytes(txs[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(raw[:4]) != selectorFor("sponsor(bytes32,string)") || !equalBytes(raw[4:36], repoID[:]) {
		t.Fatalf("sponsor calldata does not bind stable repo ID: %x", raw)
	}
	messageOffset, err := readABIOffset(raw[4:], 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	if message, err := readABIString(raw[4:], messageOffset); err != nil || message != "great work" {
		t.Fatalf("message = %q, err=%v", message, err)
	}
}

func TestEVMEconomicSplitsEncodeArraysAndReadsStayAtOneBlock(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	recipient := "0x4444444444444444444444444444444444444444"
	var repoID [32]byte
	repoID[0] = 0xab
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var estimateData string
	var readBlockTags []string

	revenueResult := make([]byte, 32+32+64)
	putABIWord(revenueResult[:32], 32)
	putABIWord(revenueResult[32:64], 1)
	recipientBytes, _ := parseEVMAddress(recipient)
	copy(revenueResult[64+12:96], recipientBytes)
	putABIWord(revenueResult[96:128], 1250)
	totalResult := make([]byte, 32)
	putABIWord(totalResult, 500_000_000_000_000_000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var result any
		switch request.Method {
		case "eth_blockNumber":
			result = testSnapshotBlockTag
		case "eth_call":
			to, data, blockTag := testEthCallTargetDataAndTag(t, request)
			readBlockTags = append(readBlockTags, blockTag)
			if strings.EqualFold(to, testEconomicModuleAddress) {
				switch data[2:10] {
				case selectorFor("revenueSplits(bytes32)"):
					result = "0x" + hex.EncodeToString(revenueResult)
				case selectorFor("sponsorTotal(bytes32)"):
					result = "0x" + hex.EncodeToString(totalResult)
				default:
					t.Errorf("unexpected economic selector %s", data[2:10])
				}
			} else {
				result = "0x" + hex.EncodeToString(resolved)
			}
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x0"
		case "eth_estimateGas":
			var params []EVMCall
			_ = json.Unmarshal(mustJSON(request.Params), &params)
			estimateData = params[0].Data
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
	cfg.EVMEconomicModuleAddress = testEconomicModuleAddress
	registry := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	economic := NewEVMEconomicModuleWithDependencies(cfg, registry, registry.rpc, signer)
	if err := economic.SetRevenueSplits(owner, "demo", []SplitEntry{{Address: recipient, Bps: 1250}}); err != nil {
		t.Fatal(err)
	}
	raw, err := decodeHexBytes(estimateData)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(raw[:4]) != selectorFor("setRevenueSplits(bytes32,address[],uint16[])") {
		t.Fatalf("split selector = %s", hex.EncodeToString(raw[:4]))
	}
	recipientsOffset, err := readABIOffset(raw[4:], 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	bpsOffset, err := readABIOffset(raw[4:], 64, 0)
	if err != nil {
		t.Fatal(err)
	}
	recipients, err := readABIAddressArray(raw[4:], recipientsOffset)
	if err != nil {
		t.Fatal(err)
	}
	bps, err := readABIUintArray(raw[4:], bpsOffset)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || len(bps) != 1 || bps[0] != 1250 {
		t.Fatalf("decoded splits = %v / %v", recipients, bps)
	}

	readBlockTags = nil
	splits, err := economic.RevenueSplits(owner, "demo")
	if err != nil || len(splits) != 1 || splits[0].Bps != 1250 {
		t.Fatalf("revenue splits = %#v, err=%v", splits, err)
	}
	totals, err := economic.SponsorTotals(owner, "demo")
	if err != nil || len(totals) != 1 || totals[0].Amount != "500000000000000000" {
		t.Fatalf("sponsor totals = %#v, err=%v", totals, err)
	}
	if len(readBlockTags) != 4 {
		t.Fatalf("economic read calls = %d, want resolve+module for each query", len(readBlockTags))
	}
	t.Logf("economic read block tags: %#v", readBlockTags)
	for _, tag := range readBlockTags {
		if tag != testSnapshotBlockTag {
			t.Fatalf("economic read block tag = %q, want %s", tag, testSnapshotBlockTag)
		}
	}
}

func TestEVMEconomicLegacyReadFallbackNeverWritesV1(t *testing.T) {
	owner := config.DefaultContractAddress
	var legacyQueries atomic.Int32
	var legacyWrites atomic.Int32
	var transactionRPCs atomic.Int32

	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
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
		var payload any
		switch {
		case query["repo_info"] != nil:
			payload = map[string]any{"owner": owner, "name": "legacy", "description": "old", "default_branch": "main", "created_at": 1, "updated_at": 2, "moderation_status": "active"}
		case query["revenue_splits"] != nil:
			payload = map[string]any{"splits": []any{map[string]any{"address": owner, "bps": 500}}}
		default:
			t.Fatalf("unexpected legacy query: %s", message)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
	}))
	defer lcdServer.Close()

	cfg := testEVMConfig(evmServer.URL)
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.LCDEndpoint = lcdServer.URL
	cfg.EVMEconomicModuleAddress = testEconomicModuleAddress
	registry := NewEVMRegistryV2(cfg)
	economic := NewEVMEconomicModuleWithDependencies(cfg, registry, registry.rpc, registry.signer)

	splits, err := economic.RevenueSplits(owner, "legacy")
	if err != nil || len(splits) != 1 || splits[0].Bps != 500 {
		t.Fatalf("legacy splits = %#v, err=%v", splits, err)
	}
	if err := economic.Sponsor(owner, "legacy", "no double write", "1inj"); !errors.Is(err, ErrLegacyWriteFallbackDisabled) {
		t.Fatalf("sponsor error = %v, want ErrLegacyWriteFallbackDisabled", err)
	}
	if err := economic.SetRevenueSplits(owner, "legacy", nil); !errors.Is(err, ErrLegacyWriteFallbackDisabled) {
		t.Fatalf("set splits error = %v, want ErrLegacyWriteFallbackDisabled", err)
	}
	if legacyQueries.Load() != 4 {
		t.Fatalf("legacy queries = %d, want repo+splits+repo+repo", legacyQueries.Load())
	}
	if legacyWrites.Load() != 0 || transactionRPCs.Load() != 0 {
		t.Fatalf("legacy writes / transaction RPCs = %d / %d, want 0 / 0", legacyWrites.Load(), transactionRPCs.Load())
	}
}

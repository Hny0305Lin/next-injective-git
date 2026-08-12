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

const testBadgeModuleAddress = "0x3333333333333333333333333333333333333333"

type testBadgeFixture struct {
	id        uint64
	repoID    [32]byte
	recipient string
	reason    string
	awardedBy string
	awardedAt uint64
}

func TestEVMBadgeAwardTargetsModuleAndUsesRegistryTransactionPipeline(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	recipient := "0x4444444444444444444444444444444444444444"
	var repoID [32]byte
	repoID[0] = 0xa5
	repoID[31] = 0x5a
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	var callTargets []string
	var estimateTarget string

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
			callTargets = append(callTargets, to)
			if blockTag != "latest" {
				t.Errorf("resolve block tag = %q, want latest", blockTag)
			}
			if data[2:10] != selectorFor("resolveRepo(address,string)") {
				t.Errorf("resolve selector = %s", data[2:10])
			}
			result = "0x" + hex.EncodeToString(resolved)
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			estimateTarget = testEVMCallTarget(t, request)
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
	cfg.EVMBadgeModuleAddress = testBadgeModuleAddress
	registry := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	badges := NewEVMBadgeModuleWithDependencies(cfg, registry, registry.rpc, signer)
	if err := badges.AwardBadge(owner, "demo", recipient, "fixed CI"); err != nil {
		t.Fatal(err)
	}

	wantMethods := "eth_call,eth_chainId,eth_getTransactionCount,eth_estimateGas,eth_gasPrice,eth_sendRawTransaction,eth_getTransactionReceipt"
	if got := strings.Join(methods, ","); got != wantMethods {
		t.Fatalf("RPC methods = %s, want %s", got, wantMethods)
	}
	if len(callTargets) != 1 || !strings.EqualFold(callTargets[0], cfg.EVMContractAddress) {
		t.Fatalf("resolve targets = %v, want core registry %s", callTargets, cfg.EVMContractAddress)
	}
	if !strings.EqualFold(estimateTarget, testBadgeModuleAddress) {
		t.Fatalf("estimate target = %q, want badge module %s", estimateTarget, testBadgeModuleAddress)
	}
	txs := signer.transactions()
	if len(txs) != 1 {
		t.Fatalf("signed transactions = %d, want 1", len(txs))
	}
	tx := txs[0]
	if tx.ChainID != 31337 || tx.Nonce != 4 || tx.GasLimit != 39_400 || tx.GasPrice != "0x3b9aca00" {
		t.Fatalf("signed transaction pipeline fields = %#v", tx)
	}
	if !strings.EqualFold(tx.To, testBadgeModuleAddress) {
		t.Fatalf("signed transaction target = %s, want module %s", tx.To, testBadgeModuleAddress)
	}
	raw, err := decodeHexBytes(tx.Data)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(raw[:4]); got != selectorFor("awardBadge(bytes32,address,string)") {
		t.Fatalf("award selector = %s", got)
	}
	if !equalBytes(raw[4:36], repoID[:]) {
		t.Fatal("award calldata does not contain the resolved stable repo ID")
	}
	recipientBytes, _ := parseEVMAddress(recipient)
	if !equalBytes(raw[4+32+12:4+64], recipientBytes) {
		t.Fatal("award calldata does not contain the recipient")
	}
	reasonOffset, err := readABIOffset(raw[4:], 64, 0)
	if err != nil {
		t.Fatal(err)
	}
	if reason, err := readABIString(raw[4:], reasonOffset); err != nil || reason != "fixed CI" {
		t.Fatalf("award reason = %q, err=%v", reason, err)
	}
}

func TestEVMBadgeRecipientPagesUseOneBlockAndCanonicalStableRepoLookup(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	recipient := "0x4444444444444444444444444444444444444444"
	var repoID [32]byte
	repoID[0] = 0x42
	firstPage := testBadgePageResult(64, true, []testBadgeFixture{{
		id: 1, repoID: repoID, recipient: recipient, reason: "first", awardedBy: owner, awardedAt: 10,
	}})
	secondPage := testBadgePageResult(65, false, []testBadgeFixture{{
		id: 2, repoID: repoID, recipient: recipient, reason: "second", awardedBy: owner, awardedAt: 11,
	}})
	repoResult := testRepoResult(owner, "renamed", "description", "main", 1, 2, false)
	coreAddress := "0x2222222222222222222222222222222222222222"
	var moduleCalls atomic.Int32
	var coreCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		to, data, blockTag := testEthCallTargetDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("badge read block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		var result []byte
		switch {
		case strings.EqualFold(to, testBadgeModuleAddress):
			moduleCalls.Add(1)
			if data[2:10] != selectorFor("listBadgesByRecipientPage(address,uint256,uint256)") {
				t.Errorf("module selector = %s", data[2:10])
			}
			if limit := abiUintFromCalldata(data, 2); limit != evmQueryPageSize {
				t.Errorf("badge page limit = %d, want %d", limit, evmQueryPageSize)
			}
			switch cursor := abiUintFromCalldata(data, 1); cursor {
			case 0:
				result = firstPage
			case 64:
				result = secondPage
			default:
				t.Errorf("unexpected badge cursor %d", cursor)
			}
		case strings.EqualFold(to, coreAddress):
			coreCalls.Add(1)
			if data[2:10] != selectorFor("getRepoById(bytes32)") {
				t.Errorf("core selector = %s", data[2:10])
			}
			result = repoResult
		default:
			t.Errorf("unexpected eth_call target %s", to)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	cfg := testEVMConfig(server.URL)
	cfg.EVMBadgeModuleAddress = testBadgeModuleAddress
	badges, err := NewEVMBadgeModule(cfg).BadgesByRecipient(recipient)
	if err != nil {
		t.Fatal(err)
	}
	if len(badges) != 2 || badges[0].Reason != "first" || badges[1].Reason != "second" {
		t.Fatalf("badges = %#v", badges)
	}
	wantOwner, _ := userAddressFromEVM(owner)
	for _, badge := range badges {
		if badge.RepoOwner != wantOwner || badge.RepoName != "renamed" || badge.RepoID != "0x"+hex.EncodeToString(repoID[:]) {
			t.Fatalf("badge canonical repository metadata = %#v", badge)
		}
	}
	if moduleCalls.Load() != 2 || coreCalls.Load() != 1 {
		t.Fatalf("module/core calls = %d/%d, want 2/1 (deduplicated repo lookup)", moduleCalls.Load(), coreCalls.Load())
	}
}

func TestEVMBadgeRejectsStalledCursorAndMalformedABI(t *testing.T) {
	recipient := "0x4444444444444444444444444444444444444444"
	for _, tc := range []struct {
		name   string
		result []byte
		want   string
	}{
		{name: "stalled cursor", result: testBadgePageResult(0, true, nil), want: "cursor did not advance"},
		{name: "truncated", result: make([]byte, 31), want: "out of bounds"},
		{name: "invalid boolean", result: func() []byte {
			result := testBadgePageResult(0, false, nil)
			result[63] = 2
			return result
		}(), want: "invalid hasMore flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var request rpcRequest
				_ = json.NewDecoder(req.Body).Decode(&request)
				if respondWithTestBlockNumber(w, request) {
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(tc.result),
				})
			}))
			defer server.Close()
			cfg := testEVMConfig(server.URL)
			cfg.EVMBadgeModuleAddress = testBadgeModuleAddress
			_, err := NewEVMBadgeModule(cfg).BadgesByRecipient(recipient)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestEVMBadgeRepoReadFallsBackToV1ButAwardNeverWritesV1(t *testing.T) {
	owner := "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	recipient, err := userAddressFromEVM("0x4444444444444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
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
			"error": map[string]any{
				"code": 3, "message": "execution reverted", "data": locatorNotFoundData(owner, "legacy"),
			},
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
		encoded := path.Base(req.URL.Path)
		encoded, _ = url.PathUnescape(encoded)
		message, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			t.Fatalf("decode legacy query: %v", decodeErr)
		}
		var query map[string]json.RawMessage
		if err := json.Unmarshal(message, &query); err != nil {
			t.Fatalf("decode legacy query JSON: %v", err)
		}
		var payload any
		switch {
		case query["repo_info"] != nil:
			payload = map[string]any{
				"owner": owner, "name": "legacy", "description": "old", "default_branch": "main",
				"created_at": 1, "updated_at": 2, "moderation_status": "active",
			}
		case query["badges_by_repo"] != nil:
			payload = map[string]any{"badges": []any{map[string]any{
				"id": 7, "repo_owner": owner, "repo_name": "legacy", "recipient": recipient,
				"reason": "legacy contribution", "awarded_at": 3,
			}}}
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
	cfg.EVMBadgeModuleAddress = testBadgeModuleAddress
	registry := NewEVMRegistryV2(cfg)
	backend := NewEVMBadgeModuleWithDependencies(cfg, registry, registry.rpc, registry.signer)

	badges, err := backend.BadgesByRepo(owner, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(badges) != 1 || badges[0].ID != 7 || badges[0].Reason != "legacy contribution" {
		t.Fatalf("legacy badges = %#v", badges)
	}
	err = backend.AwardBadge(owner, "legacy", recipient, "must not write V1")
	if !errors.Is(err, ErrLegacyWriteFallbackDisabled) {
		t.Fatalf("award error = %v, want ErrLegacyWriteFallbackDisabled", err)
	}
	if legacyQueries.Load() != 3 {
		t.Fatalf("legacy queries = %d, want repo+badges+repo", legacyQueries.Load())
	}
	if legacyWrites.Load() != 0 || transactionRPCs.Load() != 0 {
		t.Fatalf("legacy writes / transaction RPCs = %d / %d, want 0 / 0", legacyWrites.Load(), transactionRPCs.Load())
	}
}

func testBadgePageResult(nextCursor uint64, hasMore bool, badges []testBadgeFixture) []byte {
	arrayHead := make([]byte, 32+len(badges)*32)
	putABIWord(arrayHead[:32], uint64(len(badges)))
	var arrayTail []byte
	for i, badge := range badges {
		tuple := testBadgeTuple(badge)
		putABIWord(arrayHead[32+i*32:64+i*32], uint64(len(arrayHead)-32+len(arrayTail)))
		arrayTail = append(arrayTail, tuple...)
	}
	array := append(arrayHead, arrayTail...)
	result := make([]byte, 96)
	putABIWord(result[:32], nextCursor)
	if hasMore {
		result[63] = 1
	}
	putABIWord(result[64:96], 96)
	return append(result, array...)
}

func testBadgeTuple(badge testBadgeFixture) []byte {
	result := make([]byte, 192)
	putABIWord(result[:32], badge.id)
	copy(result[32:64], badge.repoID[:])
	recipient, _ := parseEVMAddress(badge.recipient)
	copy(result[64+12:96], recipient)
	putABIWord(result[96:128], 192)
	awardedBy, _ := parseEVMAddress(badge.awardedBy)
	copy(result[128+12:160], awardedBy)
	putABIWord(result[160:192], badge.awardedAt)
	return append(result, encodeABIString(badge.reason)...)
}

func testEthCallTargetDataAndTag(t *testing.T, request rpcRequest) (string, string, string) {
	t.Helper()
	data, blockTag := testEthCallDataAndTag(t, request)
	var params []json.RawMessage
	if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil {
		t.Fatalf("decode eth_call params: %v", err)
	}
	var call EVMCall
	if err := json.Unmarshal(params[0], &call); err != nil {
		t.Fatalf("decode eth_call target: %v", err)
	}
	return call.To, data, blockTag
}

func testEVMCallTarget(t *testing.T, request rpcRequest) string {
	t.Helper()
	var params []json.RawMessage
	if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) == 0 {
		t.Fatalf("decode %s params: %v", request.Method, err)
	}
	var call EVMCall
	if err := json.Unmarshal(params[0], &call); err != nil {
		t.Fatalf("decode %s call: %v", request.Method, err)
	}
	return call.To
}

package chain

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

type fakeEVMSigner struct {
	address string
	raw     string
	txs     []EVMTransaction
	mu      sync.Mutex
}

const testSnapshotBlockTag = "0x1234"

func respondWithTestBlockNumber(w http.ResponseWriter, request rpcRequest) bool {
	if request.Method != "eth_blockNumber" {
		return false
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  testSnapshotBlockTag,
	})
	return true
}

func testEthCallDataAndTag(t *testing.T, request rpcRequest) (string, string) {
	t.Helper()
	if request.Method != "eth_call" {
		t.Fatalf("RPC method = %s, want eth_call", request.Method)
	}
	var params []json.RawMessage
	if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil {
		t.Fatalf("decode eth_call params: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("eth_call params = %#v, want call object and block tag", params)
	}
	var call map[string]any
	if err := json.Unmarshal(params[0], &call); err != nil {
		t.Fatalf("decode eth_call object: %v", err)
	}
	var blockTag string
	if err := json.Unmarshal(params[1], &blockTag); err != nil {
		t.Fatalf("decode eth_call block tag: %v", err)
	}
	data, _ := call["data"].(string)
	return data, blockTag
}

func (f *fakeEVMSigner) OwnerAddress() (string, error) { return f.address, nil }
func (f *fakeEVMSigner) CreateKey(string) error        { return nil }
func (f *fakeEVMSigner) SignTransaction(_ context.Context, tx EVMTransaction) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txs = append(f.txs, tx)
	return f.raw, nil
}

func (f *fakeEVMSigner) transactions() []EVMTransaction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]EVMTransaction(nil), f.txs...)
}

func TestEVMRegistryReadOperationsDecodeABIResults(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	repoResult := testRepoResult(owner, "demo", "description", "main", 11, 12, false)
	refResult := testRefResult("abcdef0123456789abcdef0123456789abcdef01", []string{"ipfs://one"}, 13, owner)
	var repoID [32]byte
	repoID[0] = 0x01
	resolvedResult := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 11, 12, false)
	listPageResult := testRefPageResult(
		1,
		false,
		[]string{"refs/heads/main"},
		[]testRefFixture{{commit: "abcdef0123456789abcdef0123456789abcdef01", packs: []string{"ipfs://one"}, updated: 13, by: owner}},
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		calls.Add(1)
		data, blockTag := testEthCallDataAndTag(t, request)
		selector := ""
		if len(data) >= 10 {
			selector = data[2:10]
		}
		result := "0x" + hex.EncodeToString(repoResult)
		switch selector {
		case selectorFor("resolveRepo(address,string)"):
			if blockTag != testSnapshotBlockTag {
				t.Errorf("paginated resolve block tag = %q, want %q", blockTag, testSnapshotBlockTag)
			}
			result = "0x" + hex.EncodeToString(resolvedResult)
		case selectorFor("listRefsPageById(bytes32,uint256,uint256)"):
			if blockTag != testSnapshotBlockTag {
				t.Errorf("ref page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
			}
			result = "0x" + hex.EncodeToString(listPageResult)
		case selectorFor("resolveRef(address,string,string)"):
			result = "0x" + hex.EncodeToString(refResult)
		case selectorFor("getRepo(address,string)"):
			result = "0x" + hex.EncodeToString(repoResult)
		default:
			t.Errorf("unexpected calldata selector %s", selector)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	cfg := testEVMConfig(server.URL)
	backend := NewEVMRegistryV2(cfg)

	info, err := backend.RepoInfo(owner, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if info.Owner == "" || info.Name != "demo" || info.Description != "description" || info.DefaultBranch != "main" || info.CreatedAt != 11 || info.UpdatedAt != 12 {
		t.Fatalf("repo info = %#v", info)
	}
	refs, err := backend.ListRefs(owner, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].RefName != "refs/heads/main" || refs[0].CommitSha != "abcdef0123456789abcdef0123456789abcdef01" || len(refs[0].PackURIs) != 1 {
		t.Fatalf("refs = %#v", refs)
	}
	sha, packs, err := backend.ResolveRef(owner, "demo", "refs/heads/main")
	if err != nil || sha != "abcdef0123456789abcdef0123456789abcdef01" || len(packs) != 1 {
		t.Fatalf("resolve = %s %#v err=%v", sha, packs, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("eth_call count = %d, want 4", calls.Load())
	}
}

func TestDecodeEVMRepoModerationStatus(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	for code, want := range map[uint64]string{0: "active", 1: "frozen", 2: "delisted"} {
		info, err := decodeEVMRepoInfo(
			testRepoResultWithStatus(owner, "demo", "description", "main", 11, 12, code),
		)
		if err != nil {
			t.Fatalf("status %d: %v", code, err)
		}
		if info.ModerationStatus != want {
			t.Fatalf("status %d decoded as %q, want %q", code, info.ModerationStatus, want)
		}
	}
	if _, err := decodeEVMRepoInfo(
		testRepoResultWithStatus(owner, "demo", "description", "main", 11, 12, 3),
	); err == nil || !strings.Contains(err.Error(), "invalid moderation status 3") {
		t.Fatalf("invalid moderation status error = %v", err)
	}
}

func TestEVMRegistryDrainsStableIDPagesWithoutV1Fallback(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0] = 0x42
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	firstRefs := testRefPageResult(
		64,
		true,
		[]string{"refs/heads/main", "refs/heads/release"},
		[]testRefFixture{
			{commit: "abcdef0123456789abcdef0123456789abcdef01", packs: []string{"ipfs://one"}, updated: 3, by: owner},
			{commit: "bcdef0123456789abcdef0123456789abcdef012", packs: []string{"ipfs://two"}, updated: 4, by: owner},
		},
	)
	secondRefs := testRefPageResult(
		65,
		false,
		[]string{"refs/tags/v1"},
		[]testRefFixture{{commit: "cdef0123456789abcdef0123456789abcdef0123", packs: []string{"ipfs://three"}, updated: 5, by: owner}},
	)
	firstCollaborators := testCollaboratorPageResult(
		64,
		true,
		[]string{"0x2222222222222222222222222222222222222222"},
		[]uint64{1},
	)
	secondCollaborators := testCollaboratorPageResult(
		65,
		false,
		[]string{"0x3333333333333333333333333333333333333333"},
		[]uint64{2},
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		data, blockTag := testEthCallDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("stable-ID page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		selector := ""
		if len(data) >= 10 {
			selector = data[2:10]
		}
		var result []byte
		switch selector {
		case selectorFor("resolveRepo(address,string)"):
			result = resolved
		case selectorFor("listRefsPageById(bytes32,uint256,uint256)"):
			cursor := abiUintFromCalldata(data, 1)
			limit := abiUintFromCalldata(data, 2)
			if limit != evmQueryPageSize {
				t.Errorf("ref page limit = %d, want %d", limit, evmQueryPageSize)
			}
			if cursor == 0 {
				result = firstRefs
			} else if cursor == 64 {
				result = secondRefs
			} else {
				t.Errorf("unexpected ref cursor %d", cursor)
			}
		case selectorFor("listCollaboratorsPageById(bytes32,uint256,uint256)"):
			cursor := abiUintFromCalldata(data, 1)
			limit := abiUintFromCalldata(data, 2)
			if limit != evmQueryPageSize {
				t.Errorf("collaborator page limit = %d, want %d", limit, evmQueryPageSize)
			}
			if cursor == 0 {
				result = firstCollaborators
			} else if cursor == 64 {
				result = secondCollaborators
			} else {
				t.Errorf("unexpected collaborator cursor %d", cursor)
			}
		default:
			t.Errorf("unexpected selector %s", selector)
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result)})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2(testEVMConfig(server.URL))
	refs, err := backend.ListRefs(owner, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 || refs[2].RefName != "refs/tags/v1" {
		t.Fatalf("refs = %#v", refs)
	}
	collaborators, err := backend.ListCollaborators(owner, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(collaborators) != 2 || collaborators[0].Role != "maintainer" || collaborators[1].Role != "reader" {
		t.Fatalf("collaborators = %#v", collaborators)
	}
	if calls.Load() != 6 {
		t.Fatalf("eth_call count = %d, want 6 (two resolves and four pages)", calls.Load())
	}
}

func TestEVMRegistryListReposDrainsBoundedOwnerPages(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var firstID, secondID, thirdID [32]byte
	firstID[0], secondID[0], thirdID[0] = 0x11, 0x22, 0x33
	firstPage := testRepoPageResult(64, true, []testRepoFixture{
		{id: firstID, owner: owner, name: "alpha", description: "first", branch: "main", created: 1, updated: 2},
		{id: secondID, owner: owner, name: "beta", description: "second", branch: "develop", created: 3, updated: 4},
	})
	secondPage := testRepoPageResult(65, false, []testRepoFixture{
		{id: thirdID, owner: owner, name: "gamma", description: "third", branch: "main", created: 5, updated: 6, frozen: true},
	})
	var cursors []uint64
	var blockCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			blockCalls.Add(1)
			return
		}
		data, blockTag := testEthCallDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("repository page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		if len(data) < 10 || data[2:10] != selectorFor("listReposPage(address,uint256,uint256)") {
			t.Errorf("unexpected listRepos calldata %q", data)
		}
		cursor := abiUintFromCalldata(data, 1)
		limit := abiUintFromCalldata(data, 2)
		if limit != evmQueryPageSize {
			t.Errorf("repository page limit = %d, want %d", limit, evmQueryPageSize)
		}
		cursors = append(cursors, cursor)
		var result []byte
		switch cursor {
		case 0:
			result = firstPage
		case 64:
			result = secondPage
		default:
			t.Errorf("unexpected repository cursor %d", cursor)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	repositories, err := NewEVMRegistryV2(testEVMConfig(server.URL)).ListRepos(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 3 {
		t.Fatalf("repositories = %#v, want 3 entries", repositories)
	}
	if repositories[0].Name != "alpha" || repositories[1].DefaultBranch != "develop" || repositories[2].ModerationStatus != "frozen" {
		t.Fatalf("repositories = %#v", repositories)
	}
	if len(cursors) != 2 || cursors[0] != 0 || cursors[1] != 64 {
		t.Fatalf("repository cursors = %v, want [0 64]", cursors)
	}
	if blockCalls.Load() != 1 {
		t.Fatalf("eth_blockNumber calls = %d, want 1", blockCalls.Load())
	}
}

func TestEVMRegistryListReposRejectsStalledCursor(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	stalled := testRepoPageResult(0, true, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		_, blockTag := testEthCallDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("repository page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(stalled),
		})
	}))
	defer server.Close()

	_, err := NewEVMRegistryV2(testEVMConfig(server.URL)).ListRepos(owner)
	if err == nil || !strings.Contains(err.Error(), "cursor did not advance") {
		t.Fatalf("error = %v, want stalled repository cursor failure", err)
	}
}

func TestEVMRegistryRejectsStalledPageCursor(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0] = 0x99
	resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	stalled := testRefPageResult(0, true, []string{}, []testRefFixture{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		data, blockTag := testEthCallDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("ref page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		selector := data[2:10]
		result := resolved
		if selector == selectorFor("listRefsPageById(bytes32,uint256,uint256)") {
			result = stalled
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result)})
	}))
	defer server.Close()

	_, err := NewEVMRegistryV2(testEVMConfig(server.URL)).ListRefs(owner, "demo")
	if err == nil || !strings.Contains(err.Error(), "cursor did not advance") {
		t.Fatalf("error = %v, want stalled cursor failure", err)
	}
}

func TestEVMRegistryResolveRepoReturnsStableIdentityAndCanonicalLocator(t *testing.T) {
	requestedOwner := "0x1111111111111111111111111111111111111111"
	currentOwner := "0x2222222222222222222222222222222222222222"
	var repoID [32]byte
	repoID[0] = 0xaa
	repoID[31] = 0x55
	result := testResolvedRepoResult(
		repoID,
		false,
		currentOwner,
		"demo",
		"description",
		"main",
		11,
		12,
		false,
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var params []map[string]any
		_ = json.Unmarshal(mustJSON(request.Params), &params)
		data, _ := params[0]["data"].(string)
		if len(data) < 10 || data[2:10] != selectorFor("resolveRepo(address,string)") {
			t.Errorf("resolve selector = %q", data)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2(testEVMConfig(server.URL))
	resolved, err := backend.ResolveRepo(requestedOwner, "demo")
	if err != nil {
		t.Fatal(err)
	}
	wantCurrentOwner, err := userAddressFromEVM(currentOwner)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RepoID != repoID || resolved.Backend != BackendEVM || resolved.IsCanonical {
		t.Fatalf("resolved identity = %#v", resolved)
	}
	if resolved.Requested != (RepoLocator{Owner: requestedOwner, Name: "demo"}) {
		t.Fatalf("requested locator = %#v", resolved.Requested)
	}
	if resolved.Canonical != (RepoLocator{Owner: wantCurrentOwner, Name: "demo"}) {
		t.Fatalf("canonical locator = %#v", resolved.Canonical)
	}
	if resolved.Info.Owner != wantCurrentOwner || resolved.Info.Description != "description" {
		t.Fatalf("resolved repo info = %#v", resolved.Info)
	}
	if got := resolved.CanonicalURL(); got != "igit://"+wantCurrentOwner+"/demo" {
		t.Fatalf("canonical URL = %q", got)
	}
}

func TestEVMRegistryCollaboratorReadUsesV2AndConvertsAddresses(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	collaborators := []string{
		"0x2222222222222222222222222222222222222222",
		"0x3333333333333333333333333333333333333333",
	}
	var repoID [32]byte
	repoID[0] = 0x02
	resolvedResult := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
	result := testCollaboratorPageResult(2, false, collaborators, []uint64{1, 2})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		if respondWithTestBlockNumber(w, request) {
			return
		}
		data, blockTag := testEthCallDataAndTag(t, request)
		if blockTag != testSnapshotBlockTag {
			t.Errorf("collaborator page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
		}
		calls.Add(1)
		selector := data[2:10]
		response := result
		switch selector {
		case selectorFor("resolveRepo(address,string)"):
			response = resolvedResult
		case selectorFor("listCollaboratorsPageById(bytes32,uint256,uint256)"):
			// response is the page fixture.
		default:
			t.Errorf("selector = %q", data)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(response)})
	}))
	defer server.Close()

	cfg := testEVMConfig(server.URL)
	backend := NewEVMRegistryV2(cfg)
	ownerUser, err := userAddressFromEVM(owner)
	if err != nil {
		t.Fatal(err)
	}
	got, err := backend.ListCollaborators(ownerUser, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("eth_call count = %d, want 2", calls.Load())
	}
	if len(got) != 2 {
		t.Fatalf("collaborators = %#v, want 2 entries", got)
	}
	for i, role := range []string{"maintainer", "reader"} {
		wantAddress, err := userAddressFromEVM(collaborators[i])
		if err != nil {
			t.Fatal(err)
		}
		if got[i].Address != wantAddress || got[i].Role != role {
			t.Fatalf("collaborator[%d] = %#v, want address=%s role=%s", i, got[i], wantAddress, role)
		}
	}
}

func TestEVMRegistryCollaboratorReadRejectsMalformedResult(t *testing.T) {
	cases := []struct {
		name   string
		result []byte
		want   string
	}{
		{
			name: "mismatched arrays",
			result: func() []byte {
				return testCollaboratorPageResult(1, false,
					[]string{"0x2222222222222222222222222222222222222222"}, []uint64{})
			}(),
			want: "roles",
		},
		{
			name: "unknown role",
			result: testCollaboratorPageResult(1, false,
				[]string{"0x2222222222222222222222222222222222222222"}, []uint64{0}),
			want: "invalid collaborator role value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner := "0x1111111111111111111111111111111111111111"
			var repoID [32]byte
			repoID[0] = 0x03
			resolved := testResolvedRepoResult(repoID, true, owner, "demo", "description", "main", 1, 2, false)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var request rpcRequest
				_ = json.NewDecoder(req.Body).Decode(&request)
				if respondWithTestBlockNumber(w, request) {
					return
				}
				data, blockTag := testEthCallDataAndTag(t, request)
				if blockTag != testSnapshotBlockTag {
					t.Errorf("collaborator page block tag = %q, want %q", blockTag, testSnapshotBlockTag)
				}
				result := tc.result
				if data[2:10] == selectorFor("resolveRepo(address,string)") {
					result = resolved
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result":  "0x" + hex.EncodeToString(result),
				})
			}))
			defer server.Close()
			backend := NewEVMRegistryV2(testEVMConfig(server.URL))
			if _, err := backend.ListCollaborators("0x1111111111111111111111111111111111111111", "demo"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestEVMRegistryCollaboratorWriteUsesV2CalldataAndRoleValues(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	collaborator := "0x2222222222222222222222222222222222222222"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
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
	backend := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	ownerUser, err := userAddressFromEVM(owner)
	if err != nil {
		t.Fatal(err)
	}
	collaboratorUser, err := userAddressFromEVM(collaborator)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.SetCollaborator(ownerUser, "demo", collaboratorUser, "reader"); err != nil {
		t.Fatal(err)
	}
	if err := backend.SetCollaborator(ownerUser, "demo", collaboratorUser, ""); err != nil {
		t.Fatal(err)
	}
	txs := signer.transactions()
	if len(txs) != 2 {
		t.Fatalf("signed transactions = %d, want 2", len(txs))
	}
	for i, wantRole := range []uint64{2, 0} {
		raw, err := decodeHexBytes(txs[i].Data)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(raw[:4]); got != selectorFor("setCollaborator(address,string,address,uint8)") {
			t.Fatalf("tx[%d] selector = %s", i, got)
		}
		args := raw[4:]
		ownerBytes, _ := parseEVMAddress(owner)
		collaboratorBytes, _ := parseEVMAddress(collaborator)
		if !equalBytes(args[12:32], ownerBytes) {
			t.Fatalf("tx[%d] owner argument does not match signer", i)
		}
		if !equalBytes(args[64+12:96], collaboratorBytes) {
			t.Fatalf("tx[%d] collaborator argument does not match", i)
		}
		if got := readTestWordUint(args[96:128]); got != wantRole {
			t.Fatalf("tx[%d] role = %d, want %d", i, got, wantRole)
		}
	}
}

func TestEVMRegistryUpdateRepoInfoUsesExplicitPatchFlags(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x0"
		case "eth_estimateGas":
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

	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	empty := ""
	branch := "release"
	if err := backend.UpdateRepoInfo("demo", &empty, nil); err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateRepoInfo("demo", nil, &branch); err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateRepoInfo("demo", nil, nil); err != nil {
		t.Fatal(err)
	}

	txs := signer.transactions()
	if len(txs) != 3 {
		t.Fatalf("signed transactions = %d, want 3", len(txs))
	}
	wantFlags := [][2]uint64{{1, 0}, {0, 1}, {0, 0}}
	wantValues := [][2]string{{"", ""}, {"", "release"}, {"", ""}}
	for i, tx := range txs {
		raw, err := decodeHexBytes(tx.Data)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) < 4+160 || hex.EncodeToString(raw[:4]) != selectorFor("updateRepoInfo(string,bool,string,bool,string)") {
			t.Fatalf("transaction %d has unexpected selector or length", i)
		}
		args := raw[4:]
		if got := readTestWordUint(args[32:64]); got != wantFlags[i][0] {
			t.Fatalf("transaction %d description flag = %d, want %d", i, got, wantFlags[i][0])
		}
		if got := readTestWordUint(args[96:128]); got != wantFlags[i][1] {
			t.Fatalf("transaction %d branch flag = %d, want %d", i, got, wantFlags[i][1])
		}
		for field, headOffset := range []int{64, 128} {
			offset := int(readTestWordUint(args[headOffset : headOffset+32]))
			got, err := readABIString(args, offset)
			if err != nil {
				t.Fatalf("transaction %d field %d: %v", i, field, err)
			}
			if got != wantValues[i][field] {
				t.Fatalf("transaction %d field %d = %q, want %q", i, field, got, wantValues[i][field])
			}
		}
	}
}

func TestEVMRegistryCollaboratorWriteRejectsInvalidInputsBeforeRPC(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		t.Errorf("unexpected RPC request for invalid collaborator input")
	}))
	defer server.Close()
	backend := NewEVMRegistryV2WithDependencies(
		testEVMConfig(server.URL),
		NewEVMRPC(server.URL),
		&fakeEVMSigner{address: "0x1111111111111111111111111111111111111111", raw: "0x1234"},
	)
	cases := []struct {
		name         string
		owner        string
		collaborator string
		role         string
		want         string
	}{
		{
			name:         "invalid owner",
			owner:        "not-an-address",
			collaborator: "0x2222222222222222222222222222222222222222",
			role:         "reader",
			want:         "bech32",
		},
		{
			name:         "invalid collaborator",
			owner:        "0x1111111111111111111111111111111111111111",
			collaborator: "not-an-address",
			role:         "reader",
			want:         "invalid collaborator address",
		},
		{
			name:         "invalid role",
			owner:        "0x1111111111111111111111111111111111111111",
			collaborator: "0x2222222222222222222222222222222222222222",
			role:         "admin",
			want:         "invalid collaborator role",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := backend.SetCollaborator(tc.owner, "demo", tc.collaborator, tc.role)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("RPC calls = %d, want none for invalid inputs", got)
	}
}

func TestEVMRegistryCollaboratorWriteDoesNotFallbackToLegacy(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var legacyCalls atomic.Int32
	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "eth_chainId":
			response["result"] = "0x7a69"
		case "eth_getTransactionCount":
			response["result"] = "0x0"
		case "eth_estimateGas":
			response["error"] = map[string]any{
				"code":    3,
				"message": "execution reverted",
				"data":    locatorNotFoundData(owner, "demo"),
			}
		default:
			t.Errorf("unexpected method after collaborator revert: %s", request.Method)
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
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	backend := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(evmServer.URL), signer)
	err := backend.SetCollaborator(owner, "demo", "0x2222222222222222222222222222222222222222", "reader")
	if err == nil || !strings.Contains(err.Error(), "repository locator not found") {
		t.Fatalf("error = %v, want decoded V2 locator-not-found error", err)
	}
	if got := legacyCalls.Load(); got != 0 {
		t.Fatalf("legacy calls = %d, want no V1 fallback on write", got)
	}
	if got := len(signer.transactions()); got != 0 {
		t.Fatalf("signed transactions = %d, want none after estimate revert", got)
	}
}

func TestEVMRegistryRepoMovedIsTypedAndNeverFallsBackToLegacy(t *testing.T) {
	requestedOwner := "0x1111111111111111111111111111111111111111"
	currentOwner := "0x3333333333333333333333333333333333333333"
	var repoID [32]byte
	repoID[31] = 7
	var legacyCalls atomic.Int32

	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "eth_chainId":
			response["result"] = "0x7a69"
		case "eth_getTransactionCount":
			response["result"] = "0x0"
		case "eth_estimateGas":
			response["error"] = map[string]any{
				"code":    3,
				"message": "execution reverted",
				"data":    repoMovedData(repoID, currentOwner, "demo"),
			}
		default:
			t.Errorf("unexpected method after RepoMoved: %s", request.Method)
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
	signer := &fakeEVMSigner{address: requestedOwner, raw: "0x1234"}
	backend := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(evmServer.URL), signer)
	err := backend.UpdateRef(
		requestedOwner,
		"demo",
		"refs/heads/main",
		"0123456789abcdef0123456789abcdef01234567",
		[]string{"ipfs://pack"},
		"",
		true,
	)
	var moved *RepoMovedError
	if !errors.As(err, &moved) {
		t.Fatalf("error = %T %v, want RepoMovedError", err, err)
	}
	wantOwner, convErr := userAddressFromEVM(currentOwner)
	if convErr != nil {
		t.Fatal(convErr)
	}
	if moved.RepoID != repoID || moved.CurrentOwner != wantOwner || moved.Name != "demo" {
		t.Fatalf("moved error = %#v", moved)
	}
	if got := moved.CanonicalURL(); got != "igit://"+wantOwner+"/demo" {
		t.Fatalf("canonical URL = %q", got)
	}
	if got := legacyCalls.Load(); got != 0 {
		t.Fatalf("legacy calls = %d, want no fallback for RepoMoved", got)
	}
	if got := len(signer.transactions()); got != 0 {
		t.Fatalf("signed transactions = %d, want none after estimate revert", got)
	}
}

func TestEVMRegistryFallsBackToLegacyReadsWhenV2RepoIsMissing(t *testing.T) {
	owner := "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	collaborator, err := userAddressFromEVM("0x2222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	const repo = "legacy"
	const refName = "refs/heads/main"
	legacySHA := "0123456789abcdef0123456789abcdef01234567"

	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		if respondWithTestBlockNumber(w, request) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": map[string]any{
				"code":    3,
				"message": "execution reverted",
				"data":    locatorNotFoundData(owner, repo),
			},
		})
	}))
	defer evmServer.Close()

	lcdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		encoded := path.Base(req.URL.Path)
		encoded, _ = url.PathUnescape(encoded)
		message, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("decode legacy query: %v", err)
		}
		var query map[string]json.RawMessage
		if err := json.Unmarshal(message, &query); err != nil {
			t.Fatalf("decode legacy query JSON: %v", err)
		}
		var payload any
		switch {
		case query["repo_info"] != nil:
			payload = map[string]any{"owner": owner, "name": repo, "description": "legacy repo", "default_branch": "main", "created_at": 1, "updated_at": 2, "moderation_status": "active"}
		case query["list_refs"] != nil:
			payload = map[string]any{"refs": []any{map[string]any{"ref_name": refName, "commit_sha": legacySHA, "pack_uris": []string{"ipfs://legacy"}, "updated_at": 2, "updated_by": owner}}}
		case query["resolve_ref"] != nil:
			payload = map[string]any{"ref_name": refName, "commit_sha": legacySHA, "pack_uris": []string{"ipfs://legacy"}}
		case query["list_collaborators"] != nil:
			var params struct {
				Owner string `json:"owner"`
			}
			if err := json.Unmarshal(query["list_collaborators"], &params); err != nil {
				t.Fatalf("decode collaborator query: %v", err)
			}
			if params.Owner != owner {
				t.Fatalf("legacy collaborator owner = %q, want canonical %q", params.Owner, owner)
			}
			payload = map[string]any{"collaborators": []any{map[string]any{"address": collaborator, "role": "reader"}}}
		case query["resolve_username"] != nil:
			payload = map[string]any{"owner": owner}
		default:
			t.Fatalf("unexpected legacy query: %s", message)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
	}))
	defer lcdServer.Close()

	cfg := testEVMConfig(evmServer.URL)
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v2"
	cfg.LCDEndpoint = lcdServer.URL
	cfg.ContractAddress = config.DefaultContractAddress
	backend := NewEVMRegistryV2(cfg)

	resolvedRepo, err := backend.ResolveRepo(owner, repo)
	if err != nil || resolvedRepo.Backend != BackendCosmWasm || !resolvedRepo.IsCanonical || !resolvedRepo.WriteDisabled {
		t.Fatalf("legacy resolved repo = %#v, err=%v", resolvedRepo, err)
	}
	info, err := backend.RepoInfo(owner, repo)
	if err != nil || info.Name != repo {
		t.Fatalf("legacy repo info = %#v, err=%v", info, err)
	}
	refs, err := backend.ListRefs(owner, repo)
	if err != nil || len(refs) != 1 || refs[0].CommitSha != legacySHA {
		t.Fatalf("legacy refs = %#v, err=%v", refs, err)
	}
	sha, packs, err := backend.ResolveRef(owner, repo, refName)
	if err != nil || sha != legacySHA || len(packs) != 1 {
		t.Fatalf("legacy ref = %s %#v, err=%v", sha, packs, err)
	}
	ownerEVM, err := normalizeEVMAddress(owner)
	if err != nil {
		t.Fatal(err)
	}
	collaborators, err := backend.ListCollaborators(ownerEVM, repo)
	if err != nil || len(collaborators) != 1 || collaborators[0].Address != collaborator || collaborators[0].Role != "reader" {
		t.Fatalf("legacy collaborators = %#v, err=%v", collaborators, err)
	}
	resolved, err := backend.ResolveUsername("alice")
	if err != nil || resolved != owner {
		t.Fatalf("legacy username = %q, err=%v", resolved, err)
	}
}

func TestExplicitEVMSelectionDoesNotEnableLegacyReadFallback(t *testing.T) {
	owner := "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	var legacyCalls atomic.Int32
	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID,
			"error": map[string]any{"code": 3, "message": "execution reverted", "data": locatorNotFoundData(owner, "legacy")},
		})
	}))
	defer evmServer.Close()
	lcdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		legacyCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer lcdServer.Close()

	cfg := testEVMConfig(evmServer.URL)
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.LCDEndpoint = lcdServer.URL
	backend := NewEVMRegistryV2(cfg)
	if _, err := backend.ResolveRepo(owner, "legacy"); err == nil || !strings.Contains(err.Error(), "repository locator not found") {
		t.Fatalf("explicit EVM resolve error = %v, want V2 locator error", err)
	}
	if got := legacyCalls.Load(); got != 0 {
		t.Fatalf("legacy fallback calls = %d, want none for explicit EVM", got)
	}
}

func TestEVMRegistryDoesNotFallbackOnNonNotFoundRPCError(t *testing.T) {
	owner := "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	var legacyCalls atomic.Int32
	evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": map[string]any{
				"code":    -32000,
				"message": "upstream unavailable",
			},
		})
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
	backend := NewEVMRegistryV2(cfg)
	if _, err := backend.RepoInfo(owner, "unavailable"); err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("error = %v, want original EVM RPC error", err)
	}
	if got := legacyCalls.Load(); got != 0 {
		t.Fatalf("legacy fallback calls = %d, want none for non-RepoNotFound error", got)
	}
}

func TestEVMRegistryWriteUsesChainIDNonceEstimateSendAndReceipt(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
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
	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	if err := backend.CreateRepo("demo", "description", "main"); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 6 || strings.Join(methods, ",") != "eth_chainId,eth_getTransactionCount,eth_estimateGas,eth_gasPrice,eth_sendRawTransaction,eth_getTransactionReceipt" {
		t.Fatalf("methods = %v", methods)
	}
	if len(signer.txs) != 1 {
		t.Fatalf("signed transactions = %d", len(signer.txs))
	}
	tx := signer.txs[0]
	if tx.ChainID != 0x7a69 || tx.Nonce != 4 || tx.To != testEVMConfig(server.URL).EVMContractAddress || tx.GasLimit != 39_400 || tx.GasPrice != "0x3b9aca00" {
		t.Fatalf("signed tx = %#v", tx)
	}
	if got := tx.Data[2:10]; got != selectorFor("createRepo(string,string,string)") {
		t.Fatalf("signed calldata selector = %s", got)
	}
}

func TestEVMRegistryRejectsGasAdjustmentOverflow(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			result = fmt.Sprintf("0x%x", uint64(math.MaxUint64))
		default:
			t.Errorf("unexpected method after gas overflow: %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	err := backend.CreateRepo("demo", "description", "main")
	if err == nil || !strings.Contains(err.Error(), "overflows gas adjustment") {
		t.Fatalf("error = %v, want gas adjustment overflow", err)
	}
	if got := strings.Join(methods, ","); got != "eth_chainId,eth_getTransactionCount,eth_estimateGas" {
		t.Fatalf("RPC methods = %s, want no gas price or broadcast", got)
	}
	if got := len(signer.transactions()); got != 0 {
		t.Fatalf("signed transactions = %d, want no signing after gas overflow", got)
	}
}

func TestEVMRegistryRejectsUnexpectedChainIDBeforeSigning(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		if request.Method != "eth_chainId" {
			t.Errorf("unexpected RPC method after chain ID mismatch: %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  "0x1",
		})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	err := backend.CreateRepo("demo", "description", "main")
	if err == nil || !strings.Contains(err.Error(), "EVM chain ID mismatch") {
		t.Fatalf("error = %v, want chain ID mismatch", err)
	}
	if got := strings.Join(methods, ","); got != "eth_chainId" {
		t.Fatalf("RPC methods = %s, want only eth_chainId", got)
	}
	if got := len(signer.transactions()); got != 0 {
		t.Fatalf("signed transactions = %d, want no signing after mismatch", got)
	}
}

func TestEVMRegistryUpdateAndDeleteRefUseV2WritePath(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
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

	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	if err := backend.UpdateRef(owner, "demo", "refs/heads/main", "abcdef0123456789abcdef0123456789abcdef01", []string{"ipfs://pack"}, "", true); err != nil {
		t.Fatalf("UpdateRef failed: %v", err)
	}
	if err := backend.DeleteRef(owner, "demo", "refs/heads/main"); err != nil {
		t.Fatalf("DeleteRef failed: %v", err)
	}
	txs := signer.transactions()
	if len(txs) != 2 {
		t.Fatalf("signed transactions = %d, want 2", len(txs))
	}
	if got := txs[0].Data[2:10]; got != selectorFor("updateRef(address,string,string,string,string[],string,bool)") {
		t.Fatalf("UpdateRef calldata selector = %s", got)
	}
	if got := txs[1].Data[2:10]; got != selectorFor("deleteRef(address,string,string)") {
		t.Fatalf("DeleteRef calldata selector = %s", got)
	}
}

func TestEVMRegistryConcurrentWritesReserveDistinctNonces(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
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

	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	var wg sync.WaitGroup
	var errs [2]error
	names := []string{"demo-a", "demo-b"}
	for i := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = backend.CreateRepo(names[index], "description", "main")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent write %d failed: %v", i, err)
		}
	}
	txs := signer.transactions()
	if len(txs) != 2 {
		t.Fatalf("signed transactions = %d, want 2", len(txs))
	}
	nonces := []uint64{txs[0].Nonce, txs[1].Nonce}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	if nonces[0] != 4 || nonces[1] != 5 {
		t.Fatalf("reserved nonces = %v, want [4 5]", nonces)
	}
}

func TestEVMRegistrySendFailureInvalidatesNonceCache(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var nonceCalls atomic.Int32
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var result any
		var rpcErr any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			nonceCalls.Add(1)
			result = "0x4"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_sendRawTransaction":
			if sendCalls.Add(1) == 1 {
				rpcErr = map[string]any{"code": -32000, "message": "temporary broadcast failure"}
			} else {
				result = "0xhash"
			}
		case "eth_getTransactionReceipt":
			result = map[string]string{"transactionHash": "0xhash", "status": "0x1"}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}
		if rpcErr != nil {
			response["error"] = rpcErr
			delete(response, "result")
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	if err := backend.CreateRepo("first", "description", "main"); err == nil {
		t.Fatal("first broadcast unexpectedly succeeded")
	}
	if err := backend.CreateRepo("second", "description", "main"); err != nil {
		t.Fatal(err)
	}
	if got := nonceCalls.Load(); got != 2 {
		t.Fatalf("pending nonce calls = %d, want re-sync after send failure", got)
	}
	txs := signer.transactions()
	if len(txs) != 2 || txs[0].Nonce != 4 || txs[1].Nonce != 4 {
		t.Fatalf("signed nonces = %#v, want both retries to use nonce 4", txs)
	}
}

func TestEVMRegistryReceiptFailureInvalidatesNonceCache(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var nonceCalls atomic.Int32
	var receiptCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			nonceCalls.Add(1)
			result = "0x4"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_sendRawTransaction":
			result = "0xhash"
		case "eth_getTransactionReceipt":
			if receiptCalls.Add(1) == 1 {
				result = map[string]string{"transactionHash": "0xhash", "status": "0x0"}
			} else {
				result = map[string]string{"transactionHash": "0xhash", "status": "0x1"}
			}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	backend := NewEVMRegistryV2WithDependencies(testEVMConfig(server.URL), NewEVMRPC(server.URL), signer)
	firstErr := backend.CreateRepo("first", "description", "main")
	if firstErr == nil {
		t.Fatal("first receipt unexpectedly succeeded")
	}
	var reverted *TransactionRevertedError
	if !errors.As(firstErr, &reverted) {
		t.Fatalf("first error = %v, want TransactionRevertedError", firstErr)
	}
	if err := backend.CreateRepo("second", "description", "main"); err != nil {
		t.Fatal(err)
	}
	if got := nonceCalls.Load(); got != 2 {
		t.Fatalf("pending nonce calls = %d, want re-sync after receipt failure", got)
	}
	txs := signer.transactions()
	if len(txs) != 2 || txs[0].Nonce != 4 || txs[1].Nonce != 4 {
		t.Fatalf("signed nonces = %#v, want both retries to use nonce 4", txs)
	}
}

func TestEVMRegistryWriteBlocksWithoutSigner(t *testing.T) {
	backend := NewEVMRegistryV2WithDependencies(testEVMConfig("http://127.0.0.1:1"), nil, nil)
	err := backend.CreateRepo("demo", "", "main")
	if !errors.Is(err, ErrEVMSignerUnavailable) {
		t.Fatalf("error = %v, want ErrEVMSignerUnavailable", err)
	}
}

func TestEVMRegistryResynchronizesNonceAfterBroadcastFailure(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	signer := &fakeEVMSigner{address: owner, raw: "0x1234"}
	var sendAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(req.Body).Decode(&request)
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			// The failed broadcast did not make it into the node's pending set;
			// the next operation must be allowed to reuse this nonce.
			result = "0x4"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_sendRawTransaction":
			sendAttempts++
			if sendAttempts == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"error":   map[string]any{"code": -32000, "message": "temporary broadcast failure"},
				})
				return
			}
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
	backend := NewEVMRegistryV2WithDependencies(cfg, NewEVMRPC(server.URL), signer)
	backend.nonceManager = newEVMNonceManager()
	if err := backend.CreateRepo("first", "", "main"); err == nil {
		t.Fatal("first broadcast unexpectedly succeeded")
	}
	if err := backend.CreateRepo("second", "", "main"); err != nil {
		t.Fatal(err)
	}
	if len(signer.txs) != 2 || signer.txs[0].Nonce != 4 || signer.txs[1].Nonce != 4 {
		t.Fatalf("signed transactions = %#v, want both attempts to use resynchronized nonce 4", signer.txs)
	}
}

func TestEVMRegistryDecodesContractRevert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		errorData := customErrorData("RepoNotFound(bytes32)", make([]byte, 32))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error":   map[string]any{"code": 3, "message": "execution reverted", "data": errorData},
		})
	}))
	defer server.Close()
	backend := NewEVMRegistryV2(testEVMConfig(server.URL))
	_, err := backend.RepoInfo("0x1111111111111111111111111111111111111111", "missing")
	if err == nil || !strings.Contains(err.Error(), "repository not found") {
		t.Fatalf("error = %v, want decoded RepoNotFound", err)
	}
}

type testRefFixture struct {
	commit  string
	packs   []string
	updated uint64
	by      string
}

type testRepoFixture struct {
	id          [32]byte
	owner       string
	name        string
	description string
	branch      string
	created     uint64
	updated     uint64
	frozen      bool
}

func testEVMConfig(endpoint string) config.Config {
	cfg := config.Defaults()
	cfg.Network = "test"
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMContractAddress = cfg.ContractAddress
	cfg.EVMRPC = endpoint
	cfg.EVMChainID = 31337
	cfg.Node = endpoint
	return cfg
}

func selectorFor(signature string) string {
	digest := keccak256([]byte(signature))
	return hex.EncodeToString(digest[:4])
}

func customErrorData(signature string, payload []byte) string {
	digest := keccak256([]byte(signature))
	return "0x" + hex.EncodeToString(append(digest[:4], payload...))
}

func locatorNotFoundData(owner, repo string) string {
	data, err := encodeABICall(
		"LocatorNotFound(address,string)",
		abiAddressValue(owner),
		abiStringValue(repo),
	)
	if err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(data)
}

func repoMovedData(repoID [32]byte, owner, repo string) string {
	data, err := encodeABICall(
		"RepoMoved(bytes32,address,string)",
		abiBytes32Value(repoID),
		abiAddressValue(owner),
		abiStringValue(repo),
	)
	if err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(data)
}

func testRepoResult(owner, name, description, branch string, created, updated uint64, frozen bool) []byte {
	status := uint64(0)
	if frozen {
		status = 1
	}
	return testRepoResultWithStatus(owner, name, description, branch, created, updated, status)
}

func testRepoResultWithStatus(owner, name, description, branch string, created, updated, status uint64) []byte {
	base := make([]byte, 32+256)
	putABIWord(base[:32], 32)
	ownerBytes, _ := parseEVMAddress(owner)
	copy(base[32+12:32+32], ownerBytes)
	putABIWord(base[32+32:32+64], 256)
	nameTail := encodeABIString(name)
	putABIWord(base[32+64:32+96], uint64(256+len(nameTail)))
	descriptionTail := encodeABIString(description)
	putABIWord(base[32+96:32+128], uint64(256+len(nameTail)+len(descriptionTail)))
	branchTail := encodeABIString(branch)
	putABIWord(base[32+128:32+160], created)
	putABIWord(base[32+160:32+192], updated)
	putABIWord(base[32+192:32+224], status)
	base[32+224+31] = 1
	return append(append(append(base, nameTail...), descriptionTail...), branchTail...)
}

func testResolvedRepoResult(
	repoID [32]byte,
	canonical bool,
	owner, name, description, branch string,
	created, updated uint64,
	frozen bool,
) []byte {
	repoResult := testRepoResult(owner, name, description, branch, created, updated, frozen)
	result := make([]byte, 96)
	copy(result[:32], repoID[:])
	if canonical {
		result[63] = 1
	}
	putABIWord(result[64:96], 96)
	return append(result, repoResult[32:]...)
}

func testRepoPageResult(nextCursor uint64, hasMore bool, repos []testRepoFixture) []byte {
	ids := make([]byte, 32+len(repos)*32)
	putABIWord(ids[:32], uint64(len(repos)))
	repositoriesHead := make([]byte, 32+len(repos)*32)
	putABIWord(repositoriesHead[:32], uint64(len(repos)))
	var repositoriesTail []byte
	for i, repo := range repos {
		copy(ids[32+i*32:64+i*32], repo.id[:])
		tuple := testRepoResult(
			repo.owner,
			repo.name,
			repo.description,
			repo.branch,
			repo.created,
			repo.updated,
			repo.frozen,
		)[32:]
		putABIWord(
			repositoriesHead[32+i*32:64+i*32],
			uint64(len(repositoriesHead)-32+len(repositoriesTail)),
		)
		repositoriesTail = append(repositoriesTail, tuple...)
	}
	repositories := append(repositoriesHead, repositoriesTail...)
	result := make([]byte, 128)
	putABIWord(result[:32], nextCursor)
	if hasMore {
		result[63] = 1
	}
	putABIWord(result[64:96], 128)
	putABIWord(result[96:128], uint64(128+len(ids)))
	result = append(result, ids...)
	return append(result, repositories...)
}

func testRefResult(commit string, packs []string, updated uint64, by string) []byte {
	tuple := testRefTuple(commit, packs, updated, by)
	result := make([]byte, 32)
	putABIWord(result, 32)
	return append(result, tuple...)
}

func testRefTuple(commit string, packs []string, updated uint64, by string) []byte {
	commitTail := encodeABIString(commit)
	packTail := encodeABIStringArray(packs)
	result := make([]byte, 160)
	putABIWord(result[:32], 160)
	putABIWord(result[32:64], uint64(160+len(commitTail)))
	putABIWord(result[64:96], updated)
	byBytes, _ := parseEVMAddress(by)
	copy(result[96+12:128], byBytes)
	result[128+31] = 1
	return append(append(result, commitTail...), packTail...)
}

func testListRefsResult(names []string, refs []testRefFixture) []byte {
	namesTail := encodeABIStringArray(names)
	valuesHead := make([]byte, 32+len(refs)*32)
	putABIWord(valuesHead[:32], uint64(len(refs)))
	var valuesTail []byte
	for i, ref := range refs {
		tuple := testRefTuple(ref.commit, ref.packs, ref.updated, ref.by)
		putABIWord(valuesHead[32+i*32:64+i*32], uint64(len(valuesHead)-32+len(valuesTail)))
		valuesTail = append(valuesTail, tuple...)
	}
	valuesTail = append(valuesHead, valuesTail...)
	result := make([]byte, 64)
	putABIWord(result[:32], 64)
	putABIWord(result[32:64], uint64(64+len(namesTail)))
	result = append(result, namesTail...)
	result = append(result, valuesTail...)
	return result
}

func testRefPageResult(nextCursor uint64, hasMore bool, names []string, refs []testRefFixture) []byte {
	namesTail := encodeABIStringArray(names)
	valuesHead := make([]byte, 32+len(refs)*32)
	putABIWord(valuesHead[:32], uint64(len(refs)))
	var valuesTail []byte
	for i, ref := range refs {
		tuple := testRefTuple(ref.commit, ref.packs, ref.updated, ref.by)
		putABIWord(valuesHead[32+i*32:64+i*32], uint64(len(valuesHead)-32+len(valuesTail)))
		valuesTail = append(valuesTail, tuple...)
	}
	values := append(valuesHead, valuesTail...)
	result := make([]byte, 128)
	putABIWord(result[:32], nextCursor)
	if hasMore {
		result[63] = 1
	}
	putABIWord(result[64:96], 128)
	putABIWord(result[96:128], uint64(128+len(namesTail)))
	result = append(result, namesTail...)
	return append(result, values...)
}

func testCollaboratorsResult(addresses []string, roles []uint64) []byte {
	addressTail := make([]byte, 32+len(addresses)*32)
	putABIWord(addressTail[:32], uint64(len(addresses)))
	for i, address := range addresses {
		raw, err := parseEVMAddress(address)
		if err != nil {
			panic(err)
		}
		copy(addressTail[32+i*32+12:32+(i+1)*32], raw)
	}
	roleTail := make([]byte, 32+len(roles)*32)
	putABIWord(roleTail[:32], uint64(len(roles)))
	for i, role := range roles {
		putABIWord(roleTail[32+i*32:64+i*32], role)
	}
	result := make([]byte, 64)
	putABIWord(result[:32], 64)
	putABIWord(result[32:64], uint64(64+len(addressTail)))
	result = append(result, addressTail...)
	result = append(result, roleTail...)
	return result
}

func testCollaboratorPageResult(nextCursor uint64, hasMore bool, addresses []string, roles []uint64) []byte {
	addressTail := make([]byte, 32+len(addresses)*32)
	putABIWord(addressTail[:32], uint64(len(addresses)))
	for i, address := range addresses {
		raw, err := parseEVMAddress(address)
		if err != nil {
			panic(err)
		}
		copy(addressTail[32+i*32+12:32+(i+1)*32], raw)
	}
	roleTail := make([]byte, 32+len(roles)*32)
	putABIWord(roleTail[:32], uint64(len(roles)))
	for i, role := range roles {
		putABIWord(roleTail[32+i*32:64+i*32], role)
	}
	result := make([]byte, 128)
	putABIWord(result[:32], nextCursor)
	if hasMore {
		result[63] = 1
	}
	putABIWord(result[64:96], 128)
	putABIWord(result[96:128], uint64(128+len(addressTail)))
	result = append(result, addressTail...)
	return append(result, roleTail...)
}

func abiUintFromCalldata(data string, argument int) uint64 {
	raw, err := decodeHexBytes(data)
	if err != nil {
		panic(err)
	}
	value, err := readABIUint(raw[4:], argument*32)
	if err != nil {
		panic(err)
	}
	return value
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

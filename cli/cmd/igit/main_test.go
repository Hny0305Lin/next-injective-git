package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// TestVersionLinkerInjection covers both the development default and the
// release linker override used by the tag workflow.
func TestVersionLinkerInjection(t *testing.T) {
	want := os.Getenv("IGIT_EXPECTED_VERSION")
	if want == "" {
		want = "dev"
	}
	if version != want {
		t.Fatalf("version = %q, want %q", version, want)
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	content := []byte("release artifact\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	got, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
}

func TestVisibleReposHidesModeratedRepositoriesByDefault(t *testing.T) {
	repos := []chain.RepoInfo{
		{Name: "active", ModerationStatus: "active"},
		{Name: "legacy", ModerationStatus: "frozen"},
		{Name: "removed", ModerationStatus: "delisted"},
	}
	visible := visibleRepos(repos, false)
	if len(visible) != 1 || visible[0].Name != "active" {
		t.Fatalf("visible repos = %#v", visible)
	}
	if all := visibleRepos(repos, true); len(all) != len(repos) {
		t.Fatalf("--all returned %d repos, want %d", len(all), len(repos))
	}
}

func TestLegacyClientRejectsExplicitEVMBackend(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	_, err := legacyClient(cfg)
	if !errors.Is(err, chain.ErrEVMUnsupportedFeature) {
		t.Fatalf("legacyClient error = %v, want ErrEVMUnsupportedFeature", err)
	}
}

func TestPublicConfigViewHidesBackendAndTransportDetails(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMRPC = "https://rpc.example.invalid"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.InjectivedBin = "injectived"
	cfg.KeyringBackend = "test"
	cfg.KeyName = "dev"

	view := publicConfigView(cfg)
	if view["network"] != cfg.Network || view["key_name"] != "dev" {
		t.Fatalf("public config view = %#v, want network and key name", view)
	}
	for _, hidden := range []string{"contract_backend", "contract_version", "evm_rpc", "evm_contract_address", "injectived_bin", "keyring_backend"} {
		if _, ok := view[hidden]; ok {
			t.Fatalf("public config view exposes %q: %#v", hidden, view)
		}
	}
}

func TestSetConfigNetworkCannotRetainPreviousProfileTransport(t *testing.T) {
	cfg := config.Defaults()
	cfg.EVMRPC = "https://stale-testnet-rpc.invalid"
	cfg.EVMChainID = 1439
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	if err := setConfigField(&cfg, "network", "injective-mainnet"); err != nil {
		t.Fatal(err)
	}
	if cfg.Network != "injective-mainnet" || cfg.ChainID != "injective-1" || cfg.EVMChainID != 1776 {
		t.Fatalf("network identity = %#v", cfg)
	}
	if cfg.EVMRPC != "https://k8s.json-rpc.injective.network" || cfg.EVMContractAddress != "" || cfg.ContractAddress != "" {
		t.Fatalf("network switch retained stale profile values: %#v", cfg)
	}
}

func TestCmdReposSelectsEVMRegistryWithoutSigner(t *testing.T) {
	const owner = "0x1111111111111111111111111111111111111111"
	// listReposPage returns four head words followed by empty repo ID and
	// repository-array tails.
	page := make([]byte, 192)
	testCmdTransferWord(page[64:96], 128)
	testCmdTransferWord(page[96:128], 160)
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      uint64          `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		if request.Method == "eth_blockNumber" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID, "result": "0x1234",
			})
			return
		}
		if request.Method != "eth_call" {
			t.Errorf("method = %s, want eth_call", request.Method)
		}
		var params []json.RawMessage
		if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 2 {
			t.Errorf("decode eth_call parameters: %v", err)
			return
		}
		var blockTag string
		if err := json.Unmarshal(params[1], &blockTag); err != nil || blockTag != "0x1234" {
			t.Errorf("eth_call block tag = %q, err=%v, want 0x1234", blockTag, err)
			return
		}
		var call map[string]any
		if err := json.Unmarshal(params[0], &call); err != nil {
			t.Errorf("decode eth_call object: %v", err)
			return
		}
		calldata, _ := call["data"].(string)
		raw, err := hex.DecodeString(strings.TrimPrefix(calldata, "0x"))
		if err != nil || len(raw) < 4 {
			t.Errorf("decode calldata %q: %v", calldata, err)
			return
		}
		want := ethcrypto.Keccak256([]byte("listReposPage(address,uint256,uint256)"))[:4]
		if !equalCmdTransferBytes(raw[:4], want) {
			t.Errorf("listRepos selector = %x, want %x", raw[:4], want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(page),
		})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	// An explicit owner makes this a signer-free read path.
	if err := cmdRepos(cfg, []string{owner}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "eth_blockNumber,eth_call" {
		t.Fatalf("RPC methods = %v, want block snapshot then one eth_call", methods)
	}
}

func TestCmdCollabListSelectsEVMRegistryWithoutSigner(t *testing.T) {
	var methods []string
	var selectors []string
	const owner = "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0] = 0x7a
	resolved := testCmdTransferResolvedRepoResult(repoID, owner, "demo")
	// listCollaboratorsPageById returns four head words followed by each
	// dynamic array tail. Both arrays are empty in this read-only CLI fixture.
	page := make([]byte, 192)
	testCmdTransferWord(page[64:96], 128)
	testCmdTransferWord(page[96:128], 160)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      uint64          `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		if request.Method == "eth_blockNumber" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID, "result": "0x1234",
			})
			return
		}
		if request.Method != "eth_call" {
			t.Errorf("method = %s, want eth_call", request.Method)
		}
		var params []json.RawMessage
		if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 2 {
			t.Errorf("decode eth_call parameters: %v", err)
			return
		}
		var blockTag string
		if err := json.Unmarshal(params[1], &blockTag); err != nil || blockTag != "0x1234" {
			t.Errorf("eth_call block tag = %q, err=%v, want 0x1234", blockTag, err)
			return
		}
		var call map[string]any
		if err := json.Unmarshal(params[0], &call); err != nil {
			t.Errorf("decode eth_call object: %v", err)
			return
		}
		calldata, _ := call["data"].(string)
		raw, err := hex.DecodeString(strings.TrimPrefix(calldata, "0x"))
		if err != nil || len(raw) < 4 {
			t.Errorf("decode calldata %q: %v", calldata, err)
			return
		}
		selector := hex.EncodeToString(raw[:4])
		selectors = append(selectors, selector)
		var result []byte
		switch selector {
		case hex.EncodeToString(ethcrypto.Keccak256([]byte("resolveRepo(address,string)"))[:4]):
			result = resolved
		case hex.EncodeToString(ethcrypto.Keccak256([]byte("listCollaboratorsPageById(bytes32,uint256,uint256)"))[:4]):
			result = page
		default:
			t.Errorf("unexpected calldata selector %s", selector)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	// Deliberately leave KeyName empty: listing is read-only and must not invoke
	// either the Cosmos keyring or the EVM keystore.
	if err := cmdCollab(cfg, []string{"list", owner, "demo"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(methods, ","); got != "eth_blockNumber,eth_call,eth_call" {
		t.Fatalf("RPC methods = %v, want block snapshot then two read-only eth_call requests", methods)
	}
	if got := strings.Join(selectors, ","); got != strings.Join([]string{
		hex.EncodeToString(ethcrypto.Keccak256([]byte("resolveRepo(address,string)"))[:4]),
		hex.EncodeToString(ethcrypto.Keccak256([]byte("listCollaboratorsPageById(bytes32,uint256,uint256)"))[:4]),
	}, ",") {
		t.Fatalf("selectors = %s, want resolve then stable-ID collaborator page", got)
	}
}

func TestCmdCollabRejectsInvalidRoleBeforeBackendCall(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = "http://127.0.0.1:1"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	cfg.KeyName = "dev"
	err := cmdCollab(cfg, []string{"add", "demo", "0x3333333333333333333333333333333333333333", "admin"})
	if err == nil || !strings.Contains(err.Error(), "admin") || !strings.Contains(err.Error(), "maintainer") {
		t.Fatalf("error = %v, want invalid-role details", err)
	}
}

func TestCmdCollabRejectsExtraAddArguments(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = "http://127.0.0.1:1"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	cfg.KeyName = "dev"
	err := cmdCollab(cfg, []string{"add", "demo", "0x3333333333333333333333333333333333333333", "reader", "unexpected"})
	if err == nil || !strings.Contains(err.Error(), "collab add") {
		t.Fatalf("error = %v, want usage details", err)
	}
}

func TestCmdCollabAddAndRemoveUseEVMRegistry(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "collaborator-test-password")
	var calldata []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string `json:"jsonrpc"`
			ID      uint64 `json:"id"`
			Method  string `json:"method"`
			Params  any    `json:"params"`
		}
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
			encoded, _ := json.Marshal(request.Params)
			var params []map[string]any
			if err := json.Unmarshal(encoded, &params); err != nil || len(params) != 1 {
				t.Errorf("decode estimate params: params=%#v err=%v", params, err)
			} else if data, ok := params[0]["data"].(string); !ok {
				t.Errorf("estimate data = %#v", params[0]["data"])
			} else {
				calldata = append(calldata, data)
			}
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

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	cfg.EVMKeystoreDir = filepath.Join(t.TempDir(), "keys")
	cfg.KeyName = "dev"
	if err := chain.NewEVMKeystoreSigner(cfg).CreateKey(cfg.KeyName); err != nil {
		t.Fatal(err)
	}

	collaborator := "0x3333333333333333333333333333333333333333"
	if err := cmdCollab(cfg, []string{"add", "demo", collaborator, "reader"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdCollab(cfg, []string{"remove", "demo", collaborator}); err != nil {
		t.Fatal(err)
	}
	if len(calldata) != 2 {
		t.Fatalf("collaborator estimate calldata count = %d, want 2", len(calldata))
	}
	digest := ethcrypto.Keccak256([]byte("setCollaborator(address,string,address,uint8)"))
	wantSelector := hex.EncodeToString(digest[:4])
	for i, wantRole := range []byte{2, 0} {
		raw, err := hex.DecodeString(strings.TrimPrefix(calldata[i], "0x"))
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) < 4+128 || hex.EncodeToString(raw[:4]) != wantSelector {
			t.Fatalf("calldata[%d] has unexpected selector or length", i)
		}
		roleWord := raw[4+96 : 4+128]
		invalidPadding := false
		for _, value := range roleWord[:31] {
			invalidPadding = invalidPadding || value != 0
		}
		if invalidPadding || roleWord[31] != wantRole {
			t.Fatalf("calldata[%d] role word = %x, want %d", i, roleWord, wantRole)
		}
	}
}

func TestCmdRepoEditUsesEVMRegistryAndPreservesEmptyDescription(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "metadata-test-password")
	var calldata string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string `json:"jsonrpc"`
			ID      uint64 `json:"id"`
			Method  string `json:"method"`
			Params  any    `json:"params"`
		}
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
			encoded, _ := json.Marshal(request.Params)
			var params []map[string]any
			if err := json.Unmarshal(encoded, &params); err != nil || len(params) != 1 {
				t.Errorf("decode estimate params: params=%#v err=%v", params, err)
			} else {
				calldata, _ = params[0]["data"].(string)
			}
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

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	cfg.EVMKeystoreDir = filepath.Join(t.TempDir(), "keys")
	cfg.KeyName = "dev"
	if err := chain.NewEVMKeystoreSigner(cfg).CreateKey(cfg.KeyName); err != nil {
		t.Fatal(err)
	}
	if err := cmdRepo(cfg, []string{"edit", "demo", "description", ""}); err != nil {
		t.Fatal(err)
	}

	raw, err := hex.DecodeString(strings.TrimPrefix(calldata, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	digest := ethcrypto.Keccak256([]byte("updateRepoInfo(string,bool,string,bool,string)"))
	if len(raw) < 4+160 || !strings.EqualFold(hex.EncodeToString(raw[:4]), hex.EncodeToString(digest[:4])) {
		t.Fatalf("metadata calldata has unexpected selector or length: %s", calldata)
	}
	args := raw[4:]
	if args[63] != 1 || args[127] != 0 {
		t.Fatalf("patch flags = description:%d branch:%d, want 1/0", args[63], args[127])
	}
	descriptionOffset := int(args[92])<<24 | int(args[93])<<16 | int(args[94])<<8 | int(args[95])
	if descriptionOffset+32 > len(args) {
		t.Fatalf("description offset %d is out of bounds", descriptionOffset)
	}
	for _, value := range args[descriptionOffset : descriptionOffset+32] {
		if value != 0 {
			t.Fatalf("empty description ABI length word = %x", args[descriptionOffset:descriptionOffset+32])
		}
	}
}

func TestCmdRepoEditRejectsExtraBranchArgumentsBeforeBackendSelection(t *testing.T) {
	cfg := config.Defaults()
	err := cmdRepo(cfg, []string{"edit", "demo", "branch", "main", "unexpected"})
	if err == nil || !strings.Contains(err.Error(), "repo edit") {
		t.Fatalf("error = %v, want branch usage error", err)
	}
}

func TestCmdBadgeAwardUsesEVMModuleWithoutLegacyWrite(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "badge-test-password")
	var repoID [32]byte
	repoID[0] = 0xa5
	repoID[31] = 0x5a
	ownerEVM := "0x1111111111111111111111111111111111111111"
	recipient := "0x3333333333333333333333333333333333333333"
	resolved := testCmdTransferResolvedRepoResult(repoID, ownerEVM, "demo")
	coreAddress := "0x2222222222222222222222222222222222222222"
	moduleAddress := "0x4444444444444444444444444444444444444444"
	var ethCallTarget string
	var estimateTarget string
	var estimateData string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      uint64          `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "eth_call":
			var params []json.RawMessage
			_ = json.Unmarshal(request.Params, &params)
			var call map[string]any
			_ = json.Unmarshal(params[0], &call)
			ethCallTarget, _ = call["to"].(string)
			data, _ := call["data"].(string)
			wantSelector := hex.EncodeToString(ethcrypto.Keccak256([]byte("resolveRepo(address,string)"))[:4])
			if len(data) < 10 || data[2:10] != wantSelector {
				t.Errorf("resolve calldata = %s", data)
			}
			result = "0x" + hex.EncodeToString(resolved)
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x0"
		case "eth_estimateGas":
			var params []map[string]any
			_ = json.Unmarshal(request.Params, &params)
			estimateTarget, _ = params[0]["to"].(string)
			estimateData, _ = params[0]["data"].(string)
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

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = coreAddress
	cfg.EVMBadgeModuleAddress = moduleAddress
	cfg.EVMChainID = 31337
	cfg.EVMKeystoreDir = filepath.Join(t.TempDir(), "keys")
	cfg.KeyName = "dev"
	if err := chain.NewEVMKeystoreSigner(cfg).CreateKey(cfg.KeyName); err != nil {
		t.Fatal(err)
	}

	// The fixture repository must belong to the actual test signer because the
	// command obtains its owner through the signer abstraction.
	signerAddress, err := chain.NewEVMKeystoreSigner(cfg).OwnerAddress()
	if err != nil {
		t.Fatal(err)
	}
	signerHex, err := chain.NormalizeEVMAddress(signerAddress)
	if err != nil {
		t.Fatal(err)
	}
	resolved = testCmdTransferResolvedRepoResult(repoID, signerHex, "demo")

	if err := cmdBadge(cfg, []string{"award", "demo", recipient, "fixed", "CI"}); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(ethCallTarget, coreAddress) {
		t.Fatalf("resolve target = %q, want core %s", ethCallTarget, coreAddress)
	}
	if !strings.EqualFold(estimateTarget, moduleAddress) {
		t.Fatalf("estimate target = %q, want badge module %s", estimateTarget, moduleAddress)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(estimateData, "0x"))
	if err != nil || len(raw) < 4+96 {
		t.Fatalf("decode award calldata %q: %v", estimateData, err)
	}
	wantSelector := hex.EncodeToString(ethcrypto.Keccak256([]byte("awardBadge(bytes32,address,string)"))[:4])
	if hex.EncodeToString(raw[:4]) != wantSelector || !equalCmdTransferBytes(raw[4:36], repoID[:]) {
		t.Fatalf("award calldata does not target the stable repository: %x", raw)
	}
}

func TestCmdBadgeV2FailsClosedWithoutReviewedModuleAddress(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMRPC = "http://127.0.0.1:1"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMBadgeModuleAddress = ""
	cfg.EVMChainID = 31337
	cfg.KeyName = "dev"
	err := cmdBadge(cfg, []string{"list", "0x3333333333333333333333333333333333333333"})
	if err == nil || !strings.Contains(err.Error(), "badge module address") {
		t.Fatalf("error = %v, want missing badge module address", err)
	}
}

func TestShortAddressHandlesShortAndLongValues(t *testing.T) {
	if got := shortAddress("short"); got != "short" {
		t.Fatalf("short address = %q", got)
	}
	if got := shortAddress("123456789012345"); got != "123456789012…" {
		t.Fatalf("long address = %q", got)
	}
}

func TestCmdKeyNewResolvesFreshEVMKeyAddress(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-only-password")
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMKeystoreDir = filepath.Join(t.TempDir(), "keystore")
	cfg.KeyName = ""

	if err := cmdKey(cfg, []string{"new", "dev"}); err != nil {
		t.Fatalf("cmdKey new = %v", err)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.KeyName != "dev" {
		t.Fatalf("saved key name = %q, want dev", saved.KeyName)
	}
	signer := chain.NewEVMKeystoreSigner(saved)
	address, err := signer.OwnerAddress()
	if err != nil {
		t.Fatalf("resolve fresh EVM address = %v", err)
	}
	if !strings.HasPrefix(address, "inj1") {
		t.Fatalf("address = %q, want inj1 bech32", address)
	}
}

func TestCmdTransferShowUsesEVMRegistryResolvedRepoID(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var repoID [32]byte
	repoID[0] = 0xa5
	repoID[31] = 0x5a
	resolved := testCmdTransferResolvedRepoResult(repoID, owner, "demo")
	var selectors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      uint64          `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		if request.Method != "eth_call" {
			t.Errorf("method = %s, want eth_call", request.Method)
		}
		var params []json.RawMessage
		if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 2 {
			t.Errorf("decode eth_call params: %v", err)
		}
		var call map[string]any
		if len(params) > 0 {
			if err := json.Unmarshal(params[0], &call); err != nil {
				t.Errorf("decode eth_call object: %v", err)
			}
		}
		data, _ := call["data"].(string)
		raw, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
		if err != nil || len(raw) < 4 {
			t.Errorf("decode calldata %q: %v", data, err)
			return
		}
		selector := hex.EncodeToString(raw[:4])
		selectors = append(selectors, selector)
		var result []byte
		switch selector {
		case hex.EncodeToString(ethcrypto.Keccak256([]byte("resolveRepo(address,string)"))[:4]):
			result = resolved
		case hex.EncodeToString(ethcrypto.Keccak256([]byte("pendingOwnershipTransfer(bytes32)"))[:4]):
			if len(raw) != 4+32 || !equalCmdTransferBytes(raw[4:], repoID[:]) {
				t.Errorf("pending call did not use resolved repo ID: %x", raw)
			}
			result = make([]byte, 4*32)
		default:
			t.Errorf("unexpected calldata selector %s", selector)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": "0x" + hex.EncodeToString(result),
		})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	cfg.EVMRPC = server.URL
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMChainID = 31337
	if err := cmdTransfer(cfg, []string{"show", owner, "demo"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(selectors, ","); got != strings.Join([]string{
		hex.EncodeToString(ethcrypto.Keccak256([]byte("resolveRepo(address,string)"))[:4]),
		hex.EncodeToString(ethcrypto.Keccak256([]byte("pendingOwnershipTransfer(bytes32)"))[:4]),
	}, ",") {
		t.Fatalf("selectors = %s", got)
	}
}

func testCmdTransferResolvedRepoResult(repoID [32]byte, owner, name string) []byte {
	ownerBytes, err := hex.DecodeString(strings.TrimPrefix(owner, "0x"))
	if err != nil || len(ownerBytes) != 20 {
		panic("invalid EVM owner fixture")
	}
	nameTail := testCmdTransferABIString(name)
	descriptionTail := testCmdTransferABIString("description")
	branchTail := testCmdTransferABIString("main")
	tuple := make([]byte, 256)
	copy(tuple[12:32], ownerBytes)
	testCmdTransferWord(tuple[32:64], 256)
	testCmdTransferWord(tuple[64:96], uint64(256+len(nameTail)))
	testCmdTransferWord(tuple[96:128], uint64(256+len(nameTail)+len(descriptionTail)))
	testCmdTransferWord(tuple[128:160], 1)
	testCmdTransferWord(tuple[160:192], 2)
	tuple[255] = 1
	tuple = append(tuple, nameTail...)
	tuple = append(tuple, descriptionTail...)
	tuple = append(tuple, branchTail...)

	result := make([]byte, 96)
	copy(result[:32], repoID[:])
	result[63] = 1
	testCmdTransferWord(result[64:96], 96)
	return append(result, tuple...)
}

func testCmdTransferABIString(value string) []byte {
	padded := ((len(value) + 31) / 32) * 32
	result := make([]byte, 32+padded)
	testCmdTransferWord(result[:32], uint64(len(value)))
	copy(result[32:], value)
	return result
}

func testCmdTransferWord(word []byte, value uint64) {
	for i := 0; i < 8; i++ {
		word[len(word)-1-i] = byte(value)
		value >>= 8
	}
}

func equalCmdTransferBytes(left, right []byte) bool {
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

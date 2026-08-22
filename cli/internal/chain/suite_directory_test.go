package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	testSuiteDirectory = "0x1000000000000000000000000000000000000001"
	testCoordinator    = "0x2000000000000000000000000000000000000002"
)

type suiteRPCFixture struct {
	chainID              uint64
	state                uint8
	blockNumber          uint64
	directory            common.Address
	coordinator          common.Address
	snapshotRoot         common.Hash
	coordinatorHash      common.Hash
	coordinatorDirectory common.Address
	coordinatorSnapshot  common.Hash
	addresses            map[common.Hash]common.Address
	codes                map[common.Address][]byte
	committedHashes      map[common.Hash]common.Hash
	boundDirectories     map[common.Address]common.Address
	boundCoordinators    map[common.Address]common.Address
	boundIDs             map[common.Address]common.Hash
	blockTags            []string
	moduleRuntime        func(common.Address, gethabi.Method, []byte) ([]any, bool, error)
	rpcRuntime           func(rpcRequest) (any, bool, error)
}

func newSuiteRPCFixture(t *testing.T) *suiteRPCFixture {
	t.Helper()
	fixture := &suiteRPCFixture{
		chainID:           1439,
		state:             1,
		blockNumber:       0x456,
		directory:         common.HexToAddress(testSuiteDirectory),
		coordinator:       common.HexToAddress(testCoordinator),
		snapshotRoot:      crypto.Keccak256Hash([]byte("snapshot")),
		addresses:         make(map[common.Hash]common.Address),
		codes:             make(map[common.Address][]byte),
		committedHashes:   make(map[common.Hash]common.Hash),
		boundDirectories:  make(map[common.Address]common.Address),
		boundCoordinators: make(map[common.Address]common.Address),
		boundIDs:          make(map[common.Address]common.Hash),
	}
	fixture.codes[fixture.directory] = []byte{0x60, 0x00, 0x60, 0x01}
	fixture.codes[fixture.coordinator] = []byte{0x60, 0x02}
	fixture.coordinatorHash = crypto.Keccak256Hash(fixture.codes[fixture.coordinator])
	fixture.coordinatorDirectory = fixture.directory
	fixture.coordinatorSnapshot = fixture.snapshotRoot
	for index, required := range RequiredSuiteModules {
		address := common.BigToAddress(big.NewInt(int64(0x300 + index)))
		code := []byte{0x60, byte(index + 3), 0x60, byte(index + 4)}
		fixture.addresses[required.ID] = address
		fixture.codes[address] = code
		fixture.committedHashes[required.ID] = crypto.Keccak256Hash(code)
		fixture.boundDirectories[address] = fixture.directory
		fixture.boundCoordinators[address] = fixture.coordinator
		fixture.boundIDs[address] = required.ID
	}
	return fixture
}

func (f *suiteRPCFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var result any
		var err error
		switch request.Method {
		case "eth_chainId":
			result = fmt.Sprintf("0x%x", f.chainID)
		case "eth_blockNumber":
			result = fmt.Sprintf("0x%x", f.blockNumber)
		case "eth_getCode":
			result, err = f.codeResult(request.Params)
		case "eth_call":
			result, err = f.callResult(request.Params)
		default:
			if f.rpcRuntime != nil {
				var handled bool
				result, handled, err = f.rpcRuntime(request)
				if !handled && err == nil {
					err = fmt.Errorf("unexpected RPC method %s", request.Method)
				}
			} else {
				err = fmt.Errorf("unexpected RPC method %s", request.Method)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			t.Errorf("serve %s: %v", request.Method, err)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"error": map[string]any{"code": -32602, "message": err.Error()},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": result,
		})
	}))
}

func (f *suiteRPCFixture) codeResult(params any) (string, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(mustJSON(params), &values); err != nil || len(values) != 2 {
		return "", fmt.Errorf("decode eth_getCode params: %w", err)
	}
	var addressText, blockTag string
	if err := json.Unmarshal(values[0], &addressText); err != nil {
		return "", err
	}
	if err := json.Unmarshal(values[1], &blockTag); err != nil {
		return "", err
	}
	f.blockTags = append(f.blockTags, blockTag)
	return "0x" + hex.EncodeToString(f.codes[common.HexToAddress(addressText)]), nil
}

func (f *suiteRPCFixture) callResult(params any) (string, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(mustJSON(params), &values); err != nil || len(values) != 2 {
		return "", fmt.Errorf("decode eth_call params: %w", err)
	}
	var call struct {
		To   string `json:"to"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(values[0], &call); err != nil {
		return "", err
	}
	var blockTag string
	if err := json.Unmarshal(values[1], &blockTag); err != nil {
		return "", err
	}
	f.blockTags = append(f.blockTags, blockTag)
	data, err := hex.DecodeString(strings.TrimPrefix(call.Data, "0x"))
	if err != nil || len(data) < 4 {
		return "", fmt.Errorf("decode calldata: %w", err)
	}
	address := common.HexToAddress(call.To)
	if address == f.directory {
		return f.directoryCall(data)
	}
	if address == f.coordinator {
		return f.coordinatorCall(data)
	}
	return f.moduleCall(address, data)
}

func (f *suiteRPCFixture) directoryCall(data []byte) (string, error) {
	contractABI, err := loadSuiteABI(suiteABIDirectory)
	if err != nil {
		return "", err
	}
	method, err := contractABI.MethodById(data[:4])
	if err != nil {
		return "", err
	}
	var value any
	switch method.Name {
	case "suiteVersion":
		value = uint64(SupportedSuiteVersion)
	case "configuredChainId":
		value = new(big.Int).SetUint64(f.chainID)
	case "state":
		value = f.state
	case "snapshotRoot":
		value = [32]byte(f.snapshotRoot)
	case "bootstrapCoordinator":
		value = f.coordinator
	case "bootstrapCoordinatorCodeHash":
		value = [32]byte(f.coordinatorHash)
	case "moduleAddress", "moduleCodeHash":
		arguments, unpackErr := method.Inputs.Unpack(data[4:])
		if unpackErr != nil || len(arguments) != 1 {
			return "", fmt.Errorf("unpack %s: %w", method.Name, unpackErr)
		}
		rawID := arguments[0].([32]byte)
		id := common.BytesToHash(rawID[:])
		if method.Name == "moduleAddress" {
			value = f.addresses[id]
		} else {
			value = [32]byte(f.committedHashes[id])
		}
	default:
		return "", fmt.Errorf("unexpected directory method %s", method.Name)
	}
	encoded, err := method.Outputs.Pack(value)
	return "0x" + hex.EncodeToString(encoded), err
}

func (f *suiteRPCFixture) coordinatorCall(data []byte) (string, error) {
	contractABI, err := loadSuiteABI(suiteABICoordinator)
	if err != nil {
		return "", err
	}
	method, err := contractABI.MethodById(data[:4])
	if err != nil {
		return "", err
	}
	var value any
	switch method.Name {
	case "suiteDirectory":
		value = f.coordinatorDirectory
	case "snapshotRoot":
		value = [32]byte(f.coordinatorSnapshot)
	default:
		return "", fmt.Errorf("unexpected coordinator method %s", method.Name)
	}
	encoded, err := method.Outputs.Pack(value)
	return "0x" + hex.EncodeToString(encoded), err
}

func (f *suiteRPCFixture) moduleCall(address common.Address, data []byte) (string, error) {
	var contractABI gethabi.ABI
	var err error
	for _, required := range RequiredSuiteModules {
		if f.addresses[required.ID] == address {
			contractABI, err = loadSuiteABI(required.ABI)
			break
		}
	}
	if err != nil {
		return "", err
	}
	method, err := contractABI.MethodById(data[:4])
	if err != nil {
		return "", err
	}
	var value any
	switch method.Name {
	case "suiteDirectory":
		value = f.boundDirectories[address]
	case "bootstrapCoordinator":
		value = f.boundCoordinators[address]
	case "moduleId":
		value = [32]byte(f.boundIDs[address])
	default:
		if f.moduleRuntime == nil {
			return "", fmt.Errorf("unexpected module method %s", method.Name)
		}
		values, handled, runtimeErr := f.moduleRuntime(address, *method, data)
		if runtimeErr != nil {
			return "", runtimeErr
		}
		if !handled {
			return "", fmt.Errorf("unexpected module method %s", method.Name)
		}
		encoded, packErr := method.Outputs.Pack(values...)
		return "0x" + hex.EncodeToString(encoded), packErr
	}
	encoded, err := method.Outputs.Pack(value)
	return "0x" + hex.EncodeToString(encoded), err
}

func TestVerifySuiteValidatesPinnedTrustChain(t *testing.T) {
	fixture := newSuiteRPCFixture(t)
	server := fixture.serve(t)
	defer server.Close()

	info, err := VerifySuite(context.Background(), NewEVMRPC(server.URL), testSuiteDirectory, fixture.chainID)
	if err != nil {
		t.Fatalf("VerifySuite: %v", err)
	}
	if info.Version != SupportedSuiteVersion || info.State != "active" || len(info.Modules) != len(RequiredSuiteModules) {
		t.Fatalf("unexpected suite info: %#v", info)
	}
	if info.ModuleAddress("core") != fixture.addresses[RequiredSuiteModules[0].ID].Hex() {
		t.Fatalf("core address = %s", info.ModuleAddress("core"))
	}
	wantTag := fmt.Sprintf("0x%x", fixture.blockNumber)
	for _, tag := range fixture.blockTags {
		if tag != wantTag {
			t.Fatalf("suite read used block tag %q, want %q", tag, wantTag)
		}
	}
}

func TestVerifySuiteFailsClosedOnTampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*suiteRPCFixture)
		check  string
		module string
	}{
		{
			name: "inactive directory", check: "directory state",
			mutate: func(f *suiteRPCFixture) { f.state = 0 },
		},
		{
			name: "coordinator runtime hash", check: "coordinator code hash",
			mutate: func(f *suiteRPCFixture) { f.coordinatorHash = crypto.Keccak256Hash([]byte("wrong")) },
		},
		{
			name: "coordinator directory", check: "coordinator directory binding",
			mutate: func(f *suiteRPCFixture) {
				f.coordinatorDirectory = common.HexToAddress("0x9999999999999999999999999999999999999999")
			},
		},
		{
			name: "coordinator snapshot", check: "coordinator snapshot binding",
			mutate: func(f *suiteRPCFixture) { f.coordinatorSnapshot = crypto.Keccak256Hash([]byte("other")) },
		},
		{
			name: "module runtime hash", check: "module code hash", module: "moderation",
			mutate: func(f *suiteRPCFixture) {
				f.committedHashes[RequiredSuiteModules[2].ID] = crypto.Keccak256Hash([]byte("wrong"))
			},
		},
		{
			name: "directory binding", check: "directory binding", module: "economic",
			mutate: func(f *suiteRPCFixture) {
				address := f.addresses[RequiredSuiteModules[3].ID]
				f.boundDirectories[address] = common.HexToAddress("0x9999999999999999999999999999999999999999")
			},
		},
		{
			name: "coordinator binding", check: "coordinator binding", module: "username",
			mutate: func(f *suiteRPCFixture) {
				address := f.addresses[RequiredSuiteModules[4].ID]
				f.boundCoordinators[address] = common.HexToAddress("0x9999999999999999999999999999999999999999")
			},
		},
		{
			name: "module id binding", check: "module ID binding", module: "badge",
			mutate: func(f *suiteRPCFixture) {
				address := f.addresses[RequiredSuiteModules[5].ID]
				f.boundIDs[address] = crypto.Keccak256Hash([]byte("wrong-id"))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSuiteRPCFixture(t)
			test.mutate(fixture)
			server := fixture.serve(t)
			defer server.Close()
			_, err := VerifySuite(context.Background(), NewEVMRPC(server.URL), testSuiteDirectory, fixture.chainID)
			verificationErr, ok := err.(*SuiteVerificationError)
			if !ok || verificationErr.Check != test.check || verificationErr.Module != test.module {
				t.Fatalf("error = %#v, want check=%q module=%q", err, test.check, test.module)
			}
		})
	}
}

func TestVerifySuiteRejectsProfileChainMismatchBeforeContractReads(t *testing.T) {
	fixture := newSuiteRPCFixture(t)
	server := fixture.serve(t)
	defer server.Close()
	_, err := VerifySuite(context.Background(), NewEVMRPC(server.URL), testSuiteDirectory, fixture.chainID+1)
	verificationErr, ok := err.(*SuiteVerificationError)
	if !ok || verificationErr.Check != "profile chain ID" {
		t.Fatalf("error = %#v, want profile chain ID verification error", err)
	}
}

func TestEmbeddedSuiteABIsMatchSolidityArtifacts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	artifactDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "contracts", "evm-v2", "abi")
	embedded := map[string]string{
		"SuiteDirectory.json":       suiteDirectoryABIJSON,
		"BootstrapCoordinator.json": bootstrapCoordinatorABIJSON,
		"RepositoryCore.json":       repositoryCoreABIJSON,
		"RecoveryModule.json":       recoveryModuleABIJSON,
		"ModerationModule.json":     moderationModuleABIJSON,
		"EconomicModule.json":       economicModuleABIJSON,
		"UsernameModule.json":       usernameModuleABIJSON,
		"BadgeModule.json":          badgeModuleABIJSON,
		"ReleaseModule.json":        releaseModuleABIJSON,
	}
	for name, source := range embedded {
		t.Run(name, func(t *testing.T) {
			artifact, err := os.ReadFile(filepath.Join(artifactDir, name))
			if err != nil {
				t.Fatalf("read Solidity ABI: %v", err)
			}
			if strings.TrimSpace(source) != strings.TrimSpace(string(artifact)) {
				t.Fatalf("embedded ABI differs from contracts/evm-v2/abi/%s; run the ABI sync gate", name)
			}
		})
	}
}

package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const SupportedSuiteVersion uint64 = 3

type SuiteModuleID struct {
	Name string
	ID   common.Hash
	ABI  suiteABIName
}

var RequiredSuiteModules = []SuiteModuleID{
	{Name: "core", ID: crypto.Keccak256Hash([]byte("igit.module.repository-core")), ABI: suiteABICore},
	{Name: "recovery", ID: crypto.Keccak256Hash([]byte("igit.module.recovery")), ABI: suiteABIRecovery},
	{Name: "moderation", ID: crypto.Keccak256Hash([]byte("igit.module.moderation")), ABI: suiteABIModeration},
	{Name: "economic", ID: crypto.Keccak256Hash([]byte("igit.module.economic")), ABI: suiteABIEconomic},
	{Name: "username", ID: crypto.Keccak256Hash([]byte("igit.module.username")), ABI: suiteABIUsername},
	{Name: "badge", ID: crypto.Keccak256Hash([]byte("igit.module.badge")), ABI: suiteABIBadge},
	{Name: "release", ID: crypto.Keccak256Hash([]byte("igit.module.release")), ABI: suiteABIRelease},
}

type SuiteModuleInfo struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Address  string `json:"address"`
	CodeHash string `json:"code_hash"`
}

type SuiteInfo struct {
	Directory            string            `json:"directory"`
	Version              uint64            `json:"version"`
	ChainID              uint64            `json:"chain_id"`
	State                string            `json:"state"`
	SnapshotRoot         string            `json:"snapshot_root"`
	BootstrapCoordinator string            `json:"bootstrap_coordinator"`
	BlockTag             string            `json:"block_tag"`
	Modules              []SuiteModuleInfo `json:"modules"`
}

func (s *SuiteInfo) ModuleAddress(name string) string {
	if s == nil {
		return ""
	}
	for _, module := range s.Modules {
		if module.Name == name {
			return module.Address
		}
	}
	return ""
}

type SuiteVerificationError struct {
	Check  string
	Module string
	Err    error
}

func (e *SuiteVerificationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	prefix := "suite verification failed"
	if e.Module != "" {
		prefix += " for module " + e.Module
	}
	if e.Check != "" {
		prefix += " (" + e.Check + ")"
	}
	return prefix + ": " + e.Err.Error()
}

func (e *SuiteVerificationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func suiteVerificationError(check, module string, err error) error {
	return &SuiteVerificationError{Check: check, Module: module, Err: err}
}

// VerifySuite pins all reads to one block and validates the complete trust
// chain rooted at a profile's single SuiteDirectory address.
func VerifySuite(ctx context.Context, rpc *EVMRPC, directoryAddress string, expectedChainID uint64) (*SuiteInfo, error) {
	if rpc == nil {
		return nil, suiteVerificationError("rpc", "", errors.New("EVM RPC transport is nil"))
	}
	directory, err := normalizeEVMAddress(directoryAddress)
	if err != nil {
		return nil, suiteVerificationError("directory address", "", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	chainID, err := rpc.ChainID(ctx)
	if err != nil {
		return nil, suiteVerificationError("RPC chain ID", "", err)
	}
	if expectedChainID != 0 && chainID != expectedChainID {
		return nil, suiteVerificationError(
			"profile chain ID", "", fmt.Errorf("RPC reported %d, profile requires %d", chainID, expectedChainID),
		)
	}
	blockNumber, err := rpc.BlockNumber(ctx)
	if err != nil {
		return nil, suiteVerificationError("snapshot block", "", err)
	}
	blockTag := fmt.Sprintf("0x%x", blockNumber)
	directoryCode, err := rpc.CodeAt(ctx, directory, blockTag)
	if err != nil {
		return nil, suiteVerificationError("directory code", "", err)
	}
	if len(directoryCode) == 0 {
		return nil, suiteVerificationError("directory code", "", errors.New("address has no runtime bytecode"))
	}

	directoryABI, err := loadSuiteABI(suiteABIDirectory)
	if err != nil {
		return nil, suiteVerificationError("directory ABI", "", err)
	}
	version, err := suiteUint64Call(ctx, rpc, directory, blockTag, directoryABI, "suiteVersion")
	if err != nil {
		return nil, suiteVerificationError("suite version", "", err)
	}
	if version != SupportedSuiteVersion {
		return nil, suiteVerificationError(
			"suite version", "", fmt.Errorf("got %d, supported version is %d", version, SupportedSuiteVersion),
		)
	}
	configuredChainID, err := suiteBigIntCall(ctx, rpc, directory, blockTag, directoryABI, "configuredChainId")
	if err != nil {
		return nil, suiteVerificationError("configured chain ID", "", err)
	}
	if !configuredChainID.IsUint64() || configuredChainID.Uint64() != chainID {
		return nil, suiteVerificationError(
			"configured chain ID", "", fmt.Errorf("directory reports %s, RPC reports %d", configuredChainID, chainID),
		)
	}
	state, err := suiteUint8Call(ctx, rpc, directory, blockTag, directoryABI, "state")
	if err != nil {
		return nil, suiteVerificationError("directory state", "", err)
	}
	if state != 1 {
		return nil, suiteVerificationError("directory state", "", fmt.Errorf("directory is not active (state %d)", state))
	}
	snapshotRoot, err := suiteHashCall(ctx, rpc, directory, blockTag, directoryABI, "snapshotRoot")
	if err != nil {
		return nil, suiteVerificationError("snapshot root", "", err)
	}
	if snapshotRoot == (common.Hash{}) {
		return nil, suiteVerificationError("snapshot root", "", errors.New("directory returned zero root"))
	}
	coordinator, err := suiteAddressCall(ctx, rpc, directory, blockTag, directoryABI, "bootstrapCoordinator")
	if err != nil {
		return nil, suiteVerificationError("bootstrap coordinator", "", err)
	}
	if coordinator == (common.Address{}) {
		return nil, suiteVerificationError("bootstrap coordinator", "", errors.New("directory returned zero address"))
	}
	coordinatorCode, err := rpc.CodeAt(ctx, coordinator.Hex(), blockTag)
	if err != nil {
		return nil, suiteVerificationError("coordinator code", "", err)
	}
	if len(coordinatorCode) == 0 {
		return nil, suiteVerificationError("coordinator code", "", errors.New("address has no runtime bytecode"))
	}
	expectedCoordinatorHash, err := suiteHashCall(
		ctx, rpc, directory, blockTag, directoryABI, "bootstrapCoordinatorCodeHash",
	)
	if err != nil {
		return nil, suiteVerificationError("coordinator code hash", "", err)
	}
	actualCoordinatorHash := crypto.Keccak256Hash(coordinatorCode)
	if actualCoordinatorHash != expectedCoordinatorHash {
		return nil, suiteVerificationError(
			"coordinator code hash", "",
			fmt.Errorf("directory commits %s, runtime hashes to %s", expectedCoordinatorHash.Hex(), actualCoordinatorHash.Hex()),
		)
	}
	coordinatorABI, err := loadSuiteABI(suiteABICoordinator)
	if err != nil {
		return nil, suiteVerificationError("coordinator ABI", "", err)
	}
	boundDirectory, err := suiteAddressCall(ctx, rpc, coordinator.Hex(), blockTag, coordinatorABI, "suiteDirectory")
	if err != nil {
		return nil, suiteVerificationError("coordinator directory binding", "", err)
	}
	if boundDirectory != common.HexToAddress(directory) {
		return nil, suiteVerificationError(
			"coordinator directory binding", "", fmt.Errorf("coordinator reports %s, want %s", boundDirectory.Hex(), directory),
		)
	}
	boundSnapshotRoot, err := suiteHashCall(ctx, rpc, coordinator.Hex(), blockTag, coordinatorABI, "snapshotRoot")
	if err != nil {
		return nil, suiteVerificationError("coordinator snapshot binding", "", err)
	}
	if boundSnapshotRoot != snapshotRoot {
		return nil, suiteVerificationError(
			"coordinator snapshot binding", "", fmt.Errorf("coordinator reports %s, want %s", boundSnapshotRoot.Hex(), snapshotRoot.Hex()),
		)
	}

	info := &SuiteInfo{
		Directory: directory, Version: version, ChainID: chainID, State: "active",
		SnapshotRoot: snapshotRoot.Hex(), BootstrapCoordinator: coordinator.Hex(), BlockTag: blockTag,
		Modules: make([]SuiteModuleInfo, 0, len(RequiredSuiteModules)),
	}
	for _, required := range RequiredSuiteModules {
		address, err := suiteAddressCall(
			ctx, rpc, directory, blockTag, directoryABI, "moduleAddress", required.ID,
		)
		if err != nil {
			return nil, suiteVerificationError("module address", required.Name, err)
		}
		if address == (common.Address{}) {
			return nil, suiteVerificationError("module address", required.Name, errors.New("directory returned zero address"))
		}
		expectedHash, err := suiteHashCall(
			ctx, rpc, directory, blockTag, directoryABI, "moduleCodeHash", required.ID,
		)
		if err != nil {
			return nil, suiteVerificationError("module code hash", required.Name, err)
		}
		code, err := rpc.CodeAt(ctx, address.Hex(), blockTag)
		if err != nil {
			return nil, suiteVerificationError("module code", required.Name, err)
		}
		if len(code) == 0 {
			return nil, suiteVerificationError("module code", required.Name, errors.New("address has no runtime bytecode"))
		}
		actualHash := crypto.Keccak256Hash(code)
		if actualHash != expectedHash {
			return nil, suiteVerificationError(
				"module code hash", required.Name,
				fmt.Errorf("directory commits %s, runtime hashes to %s", expectedHash.Hex(), actualHash.Hex()),
			)
		}
		moduleABI, err := loadSuiteABI(required.ABI)
		if err != nil {
			return nil, suiteVerificationError("module ABI", required.Name, err)
		}
		boundDirectory, err := suiteAddressCall(ctx, rpc, address.Hex(), blockTag, moduleABI, "suiteDirectory")
		if err != nil {
			return nil, suiteVerificationError("directory binding", required.Name, err)
		}
		boundCoordinator, err := suiteAddressCall(
			ctx, rpc, address.Hex(), blockTag, moduleABI, "bootstrapCoordinator",
		)
		if err != nil {
			return nil, suiteVerificationError("coordinator binding", required.Name, err)
		}
		boundID, err := suiteHashCall(ctx, rpc, address.Hex(), blockTag, moduleABI, "moduleId")
		if err != nil {
			return nil, suiteVerificationError("module ID binding", required.Name, err)
		}
		if !bytes.Equal(boundDirectory.Bytes(), common.HexToAddress(directory).Bytes()) {
			return nil, suiteVerificationError(
				"directory binding", required.Name, fmt.Errorf("module reports %s, want %s", boundDirectory.Hex(), directory),
			)
		}
		if boundCoordinator != coordinator {
			return nil, suiteVerificationError(
				"coordinator binding", required.Name,
				fmt.Errorf("module reports %s, want %s", boundCoordinator.Hex(), coordinator.Hex()),
			)
		}
		if boundID != required.ID {
			return nil, suiteVerificationError(
				"module ID binding", required.Name, fmt.Errorf("module reports %s, want %s", boundID.Hex(), required.ID.Hex()),
			)
		}
		info.Modules = append(info.Modules, SuiteModuleInfo{
			Name: required.Name, ID: required.ID.Hex(), Address: address.Hex(), CodeHash: expectedHash.Hex(),
		})
	}
	return info, nil
}

func suiteCall(
	ctx context.Context,
	rpc *EVMRPC,
	address, blockTag string,
	contractABI gethabi.ABI,
	method string,
	arguments ...any,
) ([]any, error) {
	data, err := contractABI.Pack(method, arguments...)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", method, err)
	}
	result, err := rpc.CallContractAt(ctx, address, "0x"+hex.EncodeToString(data), blockTag)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	values, err := contractABI.Unpack(method, result)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", method, err)
	}
	return values, nil
}

func suiteSingleValue(
	ctx context.Context,
	rpc *EVMRPC,
	address, blockTag string,
	contractABI gethabi.ABI,
	method string,
	arguments ...any,
) (any, error) {
	values, err := suiteCall(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("%s returned %d values, want 1", method, len(values))
	}
	return values[0], nil
}

func suiteAddressCall(ctx context.Context, rpc *EVMRPC, address, blockTag string, contractABI gethabi.ABI, method string, arguments ...any) (common.Address, error) {
	value, err := suiteSingleValue(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return common.Address{}, err
	}
	result, ok := value.(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("%s returned %T, want address", method, value)
	}
	return result, nil
}

func suiteHashCall(ctx context.Context, rpc *EVMRPC, address, blockTag string, contractABI gethabi.ABI, method string, arguments ...any) (common.Hash, error) {
	value, err := suiteSingleValue(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return common.Hash{}, err
	}
	switch result := value.(type) {
	case [32]byte:
		return common.BytesToHash(result[:]), nil
	case common.Hash:
		return result, nil
	default:
		return common.Hash{}, fmt.Errorf("%s returned %T, want bytes32", method, value)
	}
}

func suiteUint64Call(ctx context.Context, rpc *EVMRPC, address, blockTag string, contractABI gethabi.ABI, method string, arguments ...any) (uint64, error) {
	value, err := suiteSingleValue(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return 0, err
	}
	result, ok := value.(uint64)
	if !ok {
		return 0, fmt.Errorf("%s returned %T, want uint64", method, value)
	}
	return result, nil
}

func suiteUint8Call(ctx context.Context, rpc *EVMRPC, address, blockTag string, contractABI gethabi.ABI, method string, arguments ...any) (uint8, error) {
	value, err := suiteSingleValue(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return 0, err
	}
	result, ok := value.(uint8)
	if !ok {
		return 0, fmt.Errorf("%s returned %T, want uint8", method, value)
	}
	return result, nil
}

func suiteBigIntCall(ctx context.Context, rpc *EVMRPC, address, blockTag string, contractABI gethabi.ABI, method string, arguments ...any) (*big.Int, error) {
	value, err := suiteSingleValue(ctx, rpc, address, blockTag, contractABI, method, arguments...)
	if err != nil {
		return nil, err
	}
	result, ok := value.(*big.Int)
	if !ok || result == nil {
		return nil, fmt.Errorf("%s returned %T, want uint256", method, value)
	}
	return result, nil
}

func FormatSuiteInfo(info *SuiteInfo) string {
	if info == nil {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "directory=%s\nversion=%d\nchain_id=%d\nstate=%s\nsnapshot_root=%s\nbootstrap_coordinator=%s\nblock_tag=%s\n",
		info.Directory, info.Version, info.ChainID, info.State, info.SnapshotRoot, info.BootstrapCoordinator, info.BlockTag)
	for _, module := range info.Modules {
		fmt.Fprintf(&builder, "module.%s.address=%s\nmodule.%s.code_hash=%s\n", module.Name, module.Address, module.Name, module.CodeHash)
	}
	return builder.String()
}

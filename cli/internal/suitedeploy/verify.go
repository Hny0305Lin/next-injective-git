package suitedeploy

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func (deployment *deploymentContext) verifyBindings(blockNumber, expectedBlockHash string) (*DirectoryBindingEvidence, error) {
	if _, err := parseHexQuantity(blockNumber); err != nil {
		return nil, fmt.Errorf("invalid binding verification block %q", blockNumber)
	}
	before, err := deployment.rpc.BlockByNumber(deployment.ctx, blockNumber)
	if err != nil {
		return nil, err
	}
	if !equalHash(before.Hash, expectedBlockHash) || !strings.EqualFold(before.Number, blockNumber) {
		return nil, fmt.Errorf("binding verification block does not match final configuration receipt")
	}
	directoryAddress := deployment.addresses["SuiteDirectory"]
	coordinatorAddress := deployment.addresses["BootstrapCoordinator"]
	directoryName := "SuiteDirectory"
	coordinatorName := "BootstrapCoordinator"

	suiteVersion, err := deployment.callUint64(directoryName, directoryAddress, "suiteVersion", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	state, err := deployment.callUint8(directoryName, directoryAddress, "state", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	configuredChainID, err := deployment.callUint(directoryName, directoryAddress, "configuredChainId", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	snapshotRoot, err := deployment.callBytes32(directoryName, directoryAddress, "snapshotRoot", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	bootstrapAuthority, err := deployment.callAddress(directoryName, directoryAddress, "bootstrapAuthority", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	configuredCoordinator, err := deployment.callAddress(directoryName, directoryAddress, "bootstrapCoordinator", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	configuredCoordinatorCodeHash, err := deployment.callBytes32(directoryName, directoryAddress, "bootstrapCoordinatorCodeHash", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	registeredModuleCount, err := deployment.callUint(directoryName, directoryAddress, "registeredModuleCount", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	coordinatorDirectory, err := deployment.callAddress(coordinatorName, coordinatorAddress, "suiteDirectory", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	coordinatorSnapshot, err := deployment.callBytes32(coordinatorName, coordinatorAddress, "snapshotRoot", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	coordinatorOperator, err := deployment.callAddress(coordinatorName, coordinatorAddress, "operator", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	coordinatorActivated, err := deployment.callBool(coordinatorName, coordinatorAddress, "activated", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	nextModule, err := deployment.callUint(coordinatorName, coordinatorAddress, "nextModuleIndex", nil, blockNumber)
	if err != nil {
		return nil, err
	}

	if suiteVersion != 3 || state != 0 || configuredChainID.Cmp(new(big.Int).SetUint64(deployment.options.ChainID)) != 0 || snapshotRoot != deployment.snapshot {
		return nil, fmt.Errorf("directory chain/version/state/snapshot binding is inconsistent")
	}
	if bootstrapAuthority != (common.Address{}) || configuredCoordinator != coordinatorAddress ||
		configuredCoordinatorCodeHash != deployment.codeHash[coordinatorName] {
		return nil, fmt.Errorf("directory coordinator binding is inconsistent")
	}
	if registeredModuleCount.Cmp(big.NewInt(7)) != 0 {
		return nil, fmt.Errorf("directory registered module count is %s, want 7", registeredModuleCount.String())
	}
	if coordinatorDirectory != directoryAddress || coordinatorSnapshot != deployment.snapshot ||
		coordinatorOperator != deployment.operator || coordinatorActivated || nextModule.Sign() != 0 {
		return nil, fmt.Errorf("coordinator immutable/bootstrap state binding is inconsistent")
	}
	coordinatorCode, err := deployment.rpc.CodeAt(deployment.ctx, lowerAddress(coordinatorAddress), blockNumber)
	if err != nil {
		return nil, err
	}
	observedCoordinatorHash := ethcrypto.Keccak256Hash(coordinatorCode)
	if configuredCoordinatorCodeHash != [32]byte(observedCoordinatorHash) {
		return nil, fmt.Errorf("coordinator code hash differs at binding verification block")
	}
	directoryCode, err := deployment.rpc.CodeAt(deployment.ctx, lowerAddress(directoryAddress), blockNumber)
	if err != nil {
		return nil, err
	}
	if [32]byte(ethcrypto.Keccak256Hash(directoryCode)) != deployment.codeHash[directoryName] {
		return nil, fmt.Errorf("directory code hash changed before binding verification")
	}

	binding := &DirectoryBindingEvidence{
		BlockNumber: blockNumber, BlockHash: normalizeHash(expectedBlockHash), SuiteVersion: suiteVersion,
		State: state, Active: state == 1, ConfiguredChainID: configuredChainID.Uint64(),
		SnapshotRoot: hashHex(snapshotRoot), BootstrapAuthority: lowerAddress(bootstrapAuthority),
		BootstrapCoordinator: lowerAddress(configuredCoordinator), CoordinatorCodeHash: hashHex(configuredCoordinatorCodeHash),
		RegisteredModuleCount: registeredModuleCount.Uint64(), CoordinatorDirectory: lowerAddress(coordinatorDirectory),
		CoordinatorSnapshotRoot: hashHex(coordinatorSnapshot), CoordinatorOperator: lowerAddress(coordinatorOperator),
		CoordinatorActivated: coordinatorActivated, CoordinatorNextModule: nextModule.Uint64(),
	}
	for _, contractName := range contractOrder[2:] {
		moduleID, _ := parseBytes32(moduleIDHex(contractName))
		moduleAddress, err := deployment.callAddress(directoryName, directoryAddress, "moduleAddress", []any{moduleID}, blockNumber)
		if err != nil {
			return nil, err
		}
		directoryCodeHash, err := deployment.callBytes32(directoryName, directoryAddress, "moduleCodeHash", []any{moduleID}, blockNumber)
		if err != nil {
			return nil, err
		}
		directoryVerified, err := deployment.callBool(directoryName, directoryAddress, "verifyModule", []any{moduleID}, blockNumber)
		if err != nil {
			return nil, err
		}
		moduleDirectory, err := deployment.callAddress(contractName, moduleAddress, "suiteDirectory", nil, blockNumber)
		if err != nil {
			return nil, err
		}
		moduleCoordinator, err := deployment.callAddress(contractName, moduleAddress, "bootstrapCoordinator", nil, blockNumber)
		if err != nil {
			return nil, err
		}
		observedModuleID, err := deployment.callBytes32(contractName, moduleAddress, "moduleId", nil, blockNumber)
		if err != nil {
			return nil, err
		}
		bootstrapFinalized, err := deployment.callBool(contractName, moduleAddress, "bootstrapFinalized", nil, blockNumber)
		if err != nil {
			return nil, err
		}
		code, err := deployment.rpc.CodeAt(deployment.ctx, lowerAddress(moduleAddress), blockNumber)
		if err != nil {
			return nil, err
		}
		observedHash := ethcrypto.Keccak256Hash(code)
		if moduleAddress != deployment.addresses[contractName] || directoryCodeHash != deployment.codeHash[contractName] ||
			directoryCodeHash != [32]byte(observedHash) || !directoryVerified || moduleDirectory != directoryAddress ||
			moduleCoordinator != coordinatorAddress || observedModuleID != moduleID || bootstrapFinalized {
			return nil, fmt.Errorf("module binding for %s is inconsistent", contractName)
		}
		authorityRole, authorityAddress, err := deployment.verifyModuleAuthority(contractName, moduleAddress, blockNumber)
		if err != nil {
			return nil, err
		}
		binding.Modules = append(binding.Modules, ModuleBindingEvidence{
			ModuleID: hashHex(moduleID), ContractName: contractName, Address: lowerAddress(moduleAddress),
			DirectoryCodeHash: hashHex(directoryCodeHash), ObservedCodeHash: strings.ToLower(observedHash.Hex()),
			DirectoryVerified: directoryVerified, ModuleDirectory: lowerAddress(moduleDirectory),
			ModuleCoordinator: lowerAddress(moduleCoordinator), ObservedModuleID: hashHex(observedModuleID),
			BootstrapFinalized: bootstrapFinalized, AuthorityRole: authorityRole, AuthorityAddress: authorityAddress,
		})
	}
	moderationCommittee, err := deployment.callAddress("ModerationModule", deployment.addresses["ModerationModule"], "committee", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	economicTreasury, err := deployment.callAddress("EconomicModule", deployment.addresses["EconomicModule"], "treasury", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	economicFee, err := deployment.callUint64("EconomicModule", deployment.addresses["EconomicModule"], "platformFeeBps", nil, blockNumber)
	if err != nil {
		return nil, err
	}
	if moderationCommittee != deployment.operator || economicTreasury != deployment.operator || economicFee != uint64(deployment.options.PlatformFeeBPS) {
		return nil, fmt.Errorf("mutable initial governance settings do not match deployment parameters")
	}
	binding.InitialGovernance = InitialGovernanceBinding{
		ModerationAdmin: lowerAddress(deployment.operator), ModerationCommittee: lowerAddress(moderationCommittee),
		EconomicAdmin: lowerAddress(deployment.operator), EconomicTreasury: lowerAddress(economicTreasury),
		EconomicPlatformFeeBPS: uint16(economicFee), UsernamePolicyAdmin: lowerAddress(deployment.operator),
		ReleaseAuthority: lowerAddress(deployment.operator),
	}
	after, err := deployment.rpc.BlockByNumber(deployment.ctx, blockNumber)
	if err != nil {
		return nil, err
	}
	if !equalHash(after.Hash, expectedBlockHash) || !equalHash(after.Hash, before.Hash) || !strings.EqualFold(after.Number, blockNumber) {
		return nil, fmt.Errorf("binding verification block changed during fixed-block reads")
	}
	return binding, nil
}

func (deployment *deploymentContext) verifyModuleAuthority(contractName string, address common.Address, blockNumber string) (string, string, error) {
	var role, method string
	switch contractName {
	case "ModerationModule", "EconomicModule":
		role, method = "admin", "admin"
	case "UsernameModule":
		role, method = "username_policy_admin", "policyAdmin"
	case "ReleaseModule":
		role, method = "release_authority", "releaseAuthority"
	default:
		return "", "", nil
	}
	value, err := deployment.callAddress(contractName, address, method, nil, blockNumber)
	if err != nil {
		return "", "", err
	}
	if value != deployment.operator {
		return "", "", fmt.Errorf("%s %s does not match deployment operator", contractName, role)
	}
	return role, lowerAddress(value), nil
}

func (deployment *deploymentContext) call(contractName string, address common.Address, method string, args []any, block string) ([]any, error) {
	artifact := deployment.artifacts.byName[contractName]
	if artifact == nil {
		return nil, fmt.Errorf("missing ABI for %s", contractName)
	}
	calldata, err := artifact.parsedABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("encode %s.%s: %w", contractName, method, err)
	}
	output, err := deployment.rpc.CallContractAt(deployment.ctx, lowerAddress(address), "0x"+fmt.Sprintf("%x", calldata), block)
	if err != nil {
		return nil, fmt.Errorf("call %s.%s at %s: %w", contractName, method, block, err)
	}
	definition, ok := artifact.parsedABI.Methods[method]
	if !ok {
		return nil, fmt.Errorf("ABI method %s.%s is missing", contractName, method)
	}
	values, err := definition.Outputs.Unpack(output)
	if err != nil {
		return nil, fmt.Errorf("decode %s.%s: %w", contractName, method, err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("%s.%s returned %d values, want 1", contractName, method, len(values))
	}
	return values, nil
}

func (deployment *deploymentContext) callAddress(contract string, address common.Address, method string, args []any, block string) (common.Address, error) {
	values, err := deployment.call(contract, address, method, args, block)
	if err != nil {
		return common.Address{}, err
	}
	value, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("%s.%s returned %T, want address", contract, method, values[0])
	}
	return value, nil
}

func (deployment *deploymentContext) callBytes32(contract string, address common.Address, method string, args []any, block string) ([32]byte, error) {
	values, err := deployment.call(contract, address, method, args, block)
	if err != nil {
		return [32]byte{}, err
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return [32]byte{}, fmt.Errorf("%s.%s returned %T, want bytes32", contract, method, values[0])
	}
	return value, nil
}

func (deployment *deploymentContext) callBool(contract string, address common.Address, method string, args []any, block string) (bool, error) {
	values, err := deployment.call(contract, address, method, args, block)
	if err != nil {
		return false, err
	}
	value, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("%s.%s returned %T, want bool", contract, method, values[0])
	}
	return value, nil
}

func (deployment *deploymentContext) callUint(contract string, address common.Address, method string, args []any, block string) (*big.Int, error) {
	values, err := deployment.call(contract, address, method, args, block)
	if err != nil {
		return nil, err
	}
	switch value := values[0].(type) {
	case *big.Int:
		return value, nil
	case uint64:
		return new(big.Int).SetUint64(value), nil
	case uint8:
		return new(big.Int).SetUint64(uint64(value)), nil
	case uint16:
		return new(big.Int).SetUint64(uint64(value)), nil
	case uint32:
		return new(big.Int).SetUint64(uint64(value)), nil
	default:
		return nil, fmt.Errorf("%s.%s returned %T, want uint", contract, method, values[0])
	}
}

func (deployment *deploymentContext) callUint64(contract string, address common.Address, method string, args []any, block string) (uint64, error) {
	value, err := deployment.callUint(contract, address, method, args, block)
	if err != nil {
		return 0, err
	}
	if !value.IsUint64() {
		return 0, fmt.Errorf("%s.%s overflows uint64", contract, method)
	}
	return value.Uint64(), nil
}

func (deployment *deploymentContext) callUint8(contract string, address common.Address, method string, args []any, block string) (uint8, error) {
	value, err := deployment.callUint64(contract, address, method, args, block)
	if err != nil {
		return 0, err
	}
	if value > 255 {
		return 0, fmt.Errorf("%s.%s overflows uint8", contract, method)
	}
	return uint8(value), nil
}

package suitedeploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/common"
)

type recoveryContext struct {
	*deploymentContext
	source    HistoricalTransactionSource
	input     *RecoveryInput
	nextInput int
	lastNonce uint64
	hasNonce  bool
	firstTime string
	lastTime  string
}

func Recover(ctx context.Context, options Options, rpc rpcReader, source HistoricalTransactionSource, input *RecoveryInput, outputPath string) (_ *Manifest, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rpc == nil || source == nil || input == nil {
		return nil, fmt.Errorf("historical deployment recovery requires RPC, transaction source, and recovery input")
	}
	artifacts, operator, snapshot, manifest, err := prepare(options)
	if err != nil {
		return nil, err
	}
	writer, err := newEvidenceWriter(outputPath)
	if err != nil {
		return nil, err
	}
	now := options.Clock
	if now == nil {
		now = time.Now
	}
	manifest.EvidenceMode = evidenceModeHistoricalRecovery
	manifest.Recovery = &RecoveryEvidence{Source: source.Description(), RecoveredAt: now().UTC().Format(time.RFC3339Nano)}
	deployer := &deploymentContext{
		ctx: ctx, options: options, artifacts: artifacts, rpc: rpc,
		manifest: manifest, writer: writer, operator: operator, snapshot: snapshot,
		addresses: make(map[string]common.Address), codeHash: make(map[string][32]byte),
	}
	recovery := &recoveryContext{deploymentContext: deployer, source: source, input: input}
	defer func() {
		if closeErr := writer.close(); returnedErr == nil && closeErr != nil {
			returnedErr = fmt.Errorf("close deployment evidence: %w", closeErr)
		}
	}()
	if err := writer.persist(manifest); err != nil {
		return manifest, err
	}
	if chainID, err := rpc.ChainID(ctx); err != nil {
		return manifest, deployer.fail("verify_chain", err)
	} else if chainID != options.ChainID {
		return manifest, deployer.fail("verify_chain", fmt.Errorf("RPC chain ID %d does not match requested chain ID %d", chainID, options.ChainID))
	}

	directory := contractSpec{
		name: "SuiteDirectory",
		args: []any{operator, new(big.Int).SetUint64(options.ChainID), snapshot},
		argument: []ConstructorArgument{
			{Name: "authority", Type: "address", Value: lowerAddress(operator)},
			{Name: "chainId", Type: "uint256", Value: strconv.FormatUint(options.ChainID, 10)},
			{Name: "snapshotRoot_", Type: "bytes32", Value: hashHex(snapshot)},
		},
	}
	if err := recovery.recoverContract("deploy_SuiteDirectory", directory); err != nil {
		return manifest, err
	}
	directoryAddress := deployer.addresses["SuiteDirectory"]

	coordinator := contractSpec{
		name: "BootstrapCoordinator",
		args: []any{directoryAddress, operator},
		argument: []ConstructorArgument{
			{Name: "directory", Type: "address", Value: lowerAddress(directoryAddress)},
			{Name: "operator_", Type: "address", Value: lowerAddress(operator)},
		},
	}
	if err := recovery.recoverContract("deploy_BootstrapCoordinator", coordinator); err != nil {
		return manifest, err
	}
	coordinatorAddress := deployer.addresses["BootstrapCoordinator"]
	if err := recovery.recoverConfiguration("bind_bootstrap_coordinator", "", "SuiteDirectory", "setBootstrapCoordinator", []any{coordinatorAddress, deployer.codeHash["BootstrapCoordinator"]}); err != nil {
		return manifest, err
	}

	moduleSpecs := []contractSpec{
		{name: "RepositoryCore", args: []any{directoryAddress, coordinatorAddress}},
		{name: "RecoveryModule", args: []any{directoryAddress, coordinatorAddress}},
		{name: "ModerationModule", args: []any{directoryAddress, coordinatorAddress, operator, operator}},
		{name: "EconomicModule", args: []any{directoryAddress, coordinatorAddress, operator, operator, options.PlatformFeeBPS}},
		{name: "UsernameModule", args: []any{directoryAddress, coordinatorAddress, operator}},
		{name: "BadgeModule", args: []any{directoryAddress, coordinatorAddress}},
		{name: "ReleaseModule", args: []any{directoryAddress, coordinatorAddress, operator}},
	}
	for index := range moduleSpecs {
		spec := &moduleSpecs[index]
		spec.moduleID = moduleIDHex(spec.name)
		spec.argument = moduleConstructorEvidence(spec.name, directoryAddress, coordinatorAddress, operator, options.PlatformFeeBPS)
		if err := recovery.recoverContract("deploy_"+spec.name, *spec); err != nil {
			return manifest, err
		}
		moduleID, _ := parseBytes32(spec.moduleID)
		if err := recovery.recoverConfiguration("register_"+strings.ToLower(spec.name), spec.moduleID, "BootstrapCoordinator", "registerModule", []any{moduleID, deployer.addresses[spec.name], deployer.codeHash[spec.name]}); err != nil {
			return manifest, err
		}
	}
	if recovery.nextInput != len(input.Transactions) {
		return manifest, deployer.fail("validate_recovery_input", fmt.Errorf("unused recovery transactions: consumed %d of %d", recovery.nextInput, len(input.Transactions)))
	}

	lastReceipt := manifest.ConfigurationTransactions[len(manifest.ConfigurationTransactions)-1].Receipt
	binding, err := deployer.verifyBindings(lastReceipt.BlockNumber, lastReceipt.BlockHash)
	if err != nil {
		return manifest, deployer.fail("verify_directory_bindings", err)
	}
	manifest.DirectoryBindingVerification = binding
	manifest.Status = "bootstrapping"
	manifest.CreatedAt = recovery.firstTime
	manifest.UpdatedAt = recovery.lastTime
	manifest.Recovery.TransactionsValidated = uint64(recovery.nextInput)
	if err := writer.persist(manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (recovery *recoveryContext) nextTransaction(purpose string) (*HistoricalTransaction, error) {
	if recovery.nextInput >= len(recovery.input.Transactions) {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("missing recovery transaction for %s", purpose))
	}
	reference := recovery.input.Transactions[recovery.nextInput]
	if reference.Purpose != purpose {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("recovery transaction %d purpose %q, want %q", recovery.nextInput, reference.Purpose, purpose))
	}
	transaction, err := recovery.source.Transaction(recovery.ctx, reference.TransactionHash)
	if err != nil {
		return nil, recovery.fail("recover_"+purpose, err)
	}
	if transaction == nil || !equalHash(transaction.Hash, reference.TransactionHash) {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("historical transaction hash does not match %s", reference.TransactionHash))
	}
	if !transaction.Successful || transaction.BlockNumber == 0 {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("historical transaction %s is not a successful mined transaction", reference.TransactionHash))
	}
	from, err := chain.NormalizeEVMAddress(transaction.From)
	if err != nil || from != lowerAddress(recovery.operator) {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("historical transaction sender %q does not match operator %s", transaction.From, lowerAddress(recovery.operator)))
	}
	if recovery.hasNonce && transaction.Nonce != recovery.lastNonce+1 {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("historical transaction nonce %d does not follow %d", transaction.Nonce, recovery.lastNonce))
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, transaction.Timestamp)
	if err != nil {
		return nil, recovery.fail("recover_"+purpose, fmt.Errorf("invalid historical transaction timestamp %q", transaction.Timestamp))
	}
	timestamp := parsedTime.UTC().Format(time.RFC3339Nano)
	if recovery.firstTime == "" {
		recovery.firstTime = timestamp
	}
	recovery.lastTime = timestamp
	recovery.lastNonce = transaction.Nonce
	recovery.hasNonce = true
	recovery.nextInput++
	recovery.manifest.Recovery.TransactionsValidated = uint64(recovery.nextInput)
	return transaction, nil
}

func (recovery *recoveryContext) recoverContract(purpose string, spec contractSpec) error {
	transaction, err := recovery.nextTransaction(purpose)
	if err != nil {
		return err
	}
	artifact := recovery.artifacts.byName[spec.name]
	constructor, err := artifact.parsedABI.Constructor.Inputs.Pack(spec.args...)
	if err != nil {
		return recovery.fail("encode_"+spec.name+"_constructor", err)
	}
	initcode := append(append([]byte(nil), artifact.creation...), constructor...)
	observedInput, err := decodeBytecode("historical transaction input", strings.ToLower(transaction.Input))
	if err != nil || !bytes.Equal(observedInput, initcode) {
		return recovery.fail("validate_"+spec.name+"_initcode", fmt.Errorf("historical deployment input does not match checked artifact and constructor arguments"))
	}
	if strings.TrimSpace(transaction.To) != "" {
		return recovery.fail("validate_"+spec.name+"_target", fmt.Errorf("historical deployment unexpectedly targets %q", transaction.To))
	}
	addressText, err := chain.NormalizeEVMAddress(transaction.ContractAddress)
	if err != nil {
		return recovery.fail("validate_"+spec.name+"_address", err)
	}
	address := common.HexToAddress(addressText)
	if address == (common.Address{}) {
		return recovery.fail("validate_"+spec.name+"_address", fmt.Errorf("historical deployment returned zero contract address"))
	}
	receipt, err := recovery.receipt(transaction, "", addressText)
	if err != nil {
		return recovery.fail("validate_"+spec.name+"_receipt", err)
	}
	initcodeDigest := sha256.Sum256(initcode)
	recovery.nextOrder++
	evidence := &ContractEvidence{
		TransactionOrder: recovery.nextOrder, ContractName: spec.name, ModuleID: spec.moduleID,
		SourceName: artifact.SourceName, SourceSHA256: artifact.SourceSHA256,
		CreationBytecodeSHA256: artifact.CreationBytecodeSHA256, RuntimeTemplateSHA256: artifact.RuntimeTemplateSHA256,
		ConstructorArguments: spec.argument, ConstructorArgsABI: "0x" + hex.EncodeToString(constructor),
		InitcodeSHA256: hex.EncodeToString(initcodeDigest[:]), Address: lowerAddress(address),
		TransactionHash: normalizeHash(transaction.Hash), Receipt: receipt,
	}
	recovery.manifest.Contracts = append(recovery.manifest.Contracts, evidence)
	runtime, runtimeHash, err := recovery.verifyRuntime(spec.name, address, receipt)
	if err != nil {
		return recovery.fail("verify_"+spec.name+"_runtime", err)
	}
	evidence.Runtime = runtime
	recovery.addresses[spec.name] = address
	recovery.codeHash[spec.name] = runtimeHash
	recovery.manifest.UpdatedAt = recovery.lastTime
	return recovery.writer.persist(recovery.manifest)
}

func (recovery *recoveryContext) recoverConfiguration(purpose, moduleID, targetName, method string, args []any) error {
	transaction, err := recovery.nextTransaction(purpose)
	if err != nil {
		return err
	}
	artifact := recovery.artifacts.byName[targetName]
	calldata, err := artifact.parsedABI.Pack(method, args...)
	if err != nil {
		return recovery.fail("encode_"+purpose, err)
	}
	observedInput, err := decodeBytecode("historical transaction input", strings.ToLower(transaction.Input))
	if err != nil || !bytes.Equal(observedInput, calldata) {
		return recovery.fail("validate_"+purpose+"_calldata", fmt.Errorf("historical configuration calldata does not match %s.%s", targetName, method))
	}
	target := lowerAddress(recovery.addresses[targetName])
	observedTarget, err := chain.NormalizeEVMAddress(transaction.To)
	if err != nil || observedTarget != target {
		return recovery.fail("validate_"+purpose+"_target", fmt.Errorf("historical configuration target %q does not match %s", transaction.To, target))
	}
	if strings.TrimSpace(transaction.ContractAddress) != "" {
		return recovery.fail("validate_"+purpose+"_receipt", fmt.Errorf("historical configuration unexpectedly created contract %q", transaction.ContractAddress))
	}
	receipt, err := recovery.receipt(transaction, target, "")
	if err != nil {
		return recovery.fail("validate_"+purpose+"_receipt", err)
	}
	digest := sha256.Sum256(calldata)
	recovery.nextOrder++
	evidence := &ConfigurationTransaction{
		TransactionOrder: recovery.nextOrder, Purpose: purpose, ModuleID: moduleID,
		Target: target, Calldata: "0x" + hex.EncodeToString(calldata), CalldataSHA256: hex.EncodeToString(digest[:]),
		TransactionHash: normalizeHash(transaction.Hash), Receipt: receipt,
	}
	recovery.manifest.ConfigurationTransactions = append(recovery.manifest.ConfigurationTransactions, evidence)
	recovery.manifest.UpdatedAt = recovery.lastTime
	return recovery.writer.persist(recovery.manifest)
}

func (recovery *recoveryContext) receipt(transaction *HistoricalTransaction, target, contractAddress string) (*chain.EVMReceipt, error) {
	blockTag := fmt.Sprintf("0x%x", transaction.BlockNumber)
	block, err := recovery.rpc.BlockByNumber(recovery.ctx, blockTag)
	if err != nil {
		return nil, fmt.Errorf("read historical block %s: %w", blockTag, err)
	}
	if block == nil || strings.ToLower(strings.TrimSpace(block.Number)) != blockTag || !validHash(block.Hash) {
		return nil, fmt.Errorf("historical block %s is missing or invalid", blockTag)
	}
	gasUsed, err := historicalQuantity(transaction.GasUsed)
	if err != nil {
		return nil, fmt.Errorf("invalid historical gas used %q: %w", transaction.GasUsed, err)
	}
	return &chain.EVMReceipt{
		TransactionHash: normalizeHash(transaction.Hash), BlockNumber: blockTag,
		BlockHash: normalizeHash(block.Hash), To: target, ContractAddress: contractAddress,
		Status: "0x1", GasUsed: gasUsed, Logs: []chain.EVMLog{},
	}, nil
}

func historicalQuantity(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	base := 10
	if strings.HasPrefix(trimmed, "0x") || strings.HasPrefix(trimmed, "0X") {
		base = 16
		trimmed = trimmed[2:]
	}
	parsed, ok := new(big.Int).SetString(trimmed, base)
	if !ok || parsed.Sign() < 0 {
		return "", fmt.Errorf("not a non-negative integer")
	}
	return "0x" + parsed.Text(16), nil
}

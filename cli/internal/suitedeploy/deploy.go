package suitedeploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

type transactor interface {
	Deploy(context.Context, []byte, string) (*chain.EVMTransactionResult, error)
	Send(context.Context, string, []byte, string) (*chain.EVMTransactionResult, error)
}

type rpcReader interface {
	ChainID(context.Context) (uint64, error)
	CodeAt(context.Context, string, string) ([]byte, error)
	BlockByNumber(context.Context, string) (*chain.EVMBlock, error)
	CallContractAt(context.Context, string, string, string) ([]byte, error)
}

type evidenceWriter struct {
	file *os.File
}

func newEvidenceWriter(path string) (*evidenceWriter, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return nil, fmt.Errorf("deployment evidence output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create deployment evidence directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("deployment evidence already exists and will not be overwritten: %s", path)
		}
		return nil, fmt.Errorf("create deployment evidence: %w", err)
	}
	return &evidenceWriter{file: file}, nil
}

func (writer *evidenceWriter) close() error {
	if writer == nil || writer.file == nil {
		return nil
	}
	return writer.file.Close()
}

func (writer *evidenceWriter) persist(manifest *Manifest) error {
	data, err := manifest.MarshalCanonical()
	if err != nil {
		return fmt.Errorf("encode deployment evidence: %w", err)
	}
	if err := writer.file.Truncate(0); err != nil {
		return fmt.Errorf("truncate deployment evidence: %w", err)
	}
	if _, err := writer.file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek deployment evidence: %w", err)
	}
	if _, err := writer.file.Write(data); err != nil {
		return fmt.Errorf("write deployment evidence: %w", err)
	}
	if err := writer.file.Sync(); err != nil {
		return fmt.Errorf("sync deployment evidence: %w", err)
	}
	return nil
}

type deploymentContext struct {
	ctx       context.Context
	options   Options
	artifacts *artifactSet
	rpc       rpcReader
	tx        transactor
	manifest  *Manifest
	writer    *evidenceWriter
	operator  common.Address
	snapshot  [32]byte
	addresses map[string]common.Address
	codeHash  map[string][32]byte
	nextOrder uint64
}

type contractSpec struct {
	name     string
	moduleID string
	args     []any
	argument []ConstructorArgument
}

var moduleLabels = map[string]string{
	"RepositoryCore":   "igit.module.repository-core",
	"RecoveryModule":   "igit.module.recovery",
	"ModerationModule": "igit.module.moderation",
	"EconomicModule":   "igit.module.economic",
	"UsernameModule":   "igit.module.username",
	"BadgeModule":      "igit.module.badge",
	"ReleaseModule":    "igit.module.release",
}

// Deploy creates the immutable suite and writes exclusive deployment
// evidence. It intentionally stops in Bootstrapping state; snapshot import and
// activation are separate, reviewed operations.
func Deploy(ctx context.Context, options Options, rpc rpcReader, tx transactor, outputPath string) (_ *Manifest, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rpc == nil || tx == nil {
		return nil, fmt.Errorf("suite deployment requires RPC and transactor dependencies")
	}
	artifacts, operator, snapshot, manifest, err := prepare(options)
	if err != nil {
		return nil, err
	}
	writer, err := newEvidenceWriter(outputPath)
	if err != nil {
		return nil, err
	}
	deployer := &deploymentContext{
		ctx: ctx, options: options, artifacts: artifacts, rpc: rpc, tx: tx,
		manifest: manifest, writer: writer, operator: operator, snapshot: snapshot,
		addresses: make(map[string]common.Address), codeHash: make(map[string][32]byte),
	}
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
	if err := deployer.deployContract(directory); err != nil {
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
	if err := deployer.deployContract(coordinator); err != nil {
		return manifest, err
	}
	coordinatorAddress := deployer.addresses["BootstrapCoordinator"]
	if err := deployer.configure("bind_bootstrap_coordinator", "", "SuiteDirectory", "setBootstrapCoordinator", []any{coordinatorAddress, deployer.codeHash["BootstrapCoordinator"]}); err != nil {
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
		if err := deployer.deployContract(*spec); err != nil {
			return manifest, err
		}
		moduleID, _ := parseBytes32(spec.moduleID)
		if err := deployer.configure("register_"+strings.ToLower(spec.name), spec.moduleID, "BootstrapCoordinator", "registerModule", []any{moduleID, deployer.addresses[spec.name], deployer.codeHash[spec.name]}); err != nil {
			return manifest, err
		}
	}

	lastReceipt := manifest.ConfigurationTransactions[len(manifest.ConfigurationTransactions)-1].Receipt
	binding, err := deployer.verifyBindings(lastReceipt.BlockNumber, lastReceipt.BlockHash)
	if err != nil {
		return manifest, deployer.fail("verify_directory_bindings", err)
	}
	manifest.DirectoryBindingVerification = binding
	manifest.Status = "bootstrapping"
	manifest.UpdatedAt = deployer.now()
	if err := writer.persist(manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func prepare(options Options) (*artifactSet, common.Address, [32]byte, *Manifest, error) {
	artifacts, err := loadArtifactSet(options.ArtifactDirectory)
	if err != nil {
		return nil, common.Address{}, [32]byte{}, nil, err
	}
	if options.ChainID == 0 {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("deployment chain ID must be positive")
	}
	if strings.TrimSpace(options.RPCEndpoint) == "" {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("deployment RPC endpoint is required")
	}
	if options.PlatformFeeBPS > 500 {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("platform fee %d exceeds contract maximum 500 bps", options.PlatformFeeBPS)
	}
	if err := validateSourceCommit(options.SourceCommit); err != nil {
		return nil, common.Address{}, [32]byte{}, nil, err
	}
	normalizedOperator, err := chain.NormalizeEVMAddress(options.Operator)
	if err != nil {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("normalize bootstrap operator: %w", err)
	}
	operator := common.HexToAddress(normalizedOperator)
	if operator == (common.Address{}) {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("bootstrap operator cannot be zero")
	}
	snapshot, err := parseBytes32(options.SnapshotRoot)
	if err != nil {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("parse snapshot root: %w", err)
	}
	if snapshot == ([32]byte{}) {
		return nil, common.Address{}, [32]byte{}, nil, fmt.Errorf("snapshot root cannot be zero")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC().Format(time.RFC3339Nano)
	inspection := artifacts.inspection()
	operatorHex := lowerAddress(operator)
	manifest := &Manifest{
		Schema: manifestSchema, Status: "preparing", CreatedAt: now, UpdatedAt: now,
		Compiler:     requiredCompilerEvidence(),
		Source:       SourceEvidence{Commit: options.SourceCommit, ArtifactSetSHA256: artifacts.digest, Contracts: inspection.Contracts},
		Chain:        ChainEvidence{Network: strings.TrimSpace(options.Network), ChainID: options.ChainID, RPCEndpoint: evidenceEndpoint(options.RPCEndpoint), BlockExplorer: options.BlockExplorer},
		SnapshotRoot: hashHex(snapshot), PlatformFeeBPS: options.PlatformFeeBPS,
		Governance: GovernanceEvidence{
			BootstrapOperator: operatorHex, Admin: operatorHex, Treasury: operatorHex,
			Committee: operatorHex, UsernamePolicy: operatorHex, ReleaseAuthority: operatorHex,
			ProductionReady: false,
			Warning:         "testnet bootstrap uses one rotated encrypted-keystore EOA for operator, admin, treasury, committee, username policy, and release authority; production requires redesigned multisig/timelock governance",
		},
		BlockscoutVerification: BlockscoutVerification{
			Status: "not_attempted", Explorer: options.BlockExplorer, Contracts: map[string]string{},
			Note: "source verification is independent of deployment success and must be recorded by a separate evidence-producing command",
		},
	}
	return artifacts, operator, snapshot, manifest, nil
}

func (deployment *deploymentContext) now() string {
	clock := deployment.options.Clock
	if clock == nil {
		clock = time.Now
	}
	return clock().UTC().Format(time.RFC3339Nano)
}

func (deployment *deploymentContext) fail(stage string, cause error) error {
	deployment.manifest.Status = "failed"
	deployment.manifest.FailureStage = stage
	deployment.manifest.Failure = cause.Error()
	deployment.manifest.UpdatedAt = deployment.now()
	if persistErr := deployment.writer.persist(deployment.manifest); persistErr != nil {
		return fmt.Errorf("%s: %v (also failed to persist evidence: %v)", stage, cause, persistErr)
	}
	return fmt.Errorf("%s: %w", stage, cause)
}

func (deployment *deploymentContext) deployContract(spec contractSpec) error {
	artifact := deployment.artifacts.byName[spec.name]
	constructor, err := artifact.parsedABI.Constructor.Inputs.Pack(spec.args...)
	if err != nil {
		return deployment.fail("encode_"+spec.name+"_constructor", err)
	}
	initcode := append(append([]byte(nil), artifact.creation...), constructor...)
	if len(initcode) > maximumInitcodeBytes {
		return deployment.fail("encode_"+spec.name+"_constructor", fmt.Errorf("constructor produces %d-byte initcode, exceeding EIP-3860 maximum %d", len(initcode), maximumInitcodeBytes))
	}
	initcodeDigest := sha256.Sum256(initcode)
	deployment.nextOrder++
	evidence := &ContractEvidence{
		TransactionOrder: deployment.nextOrder, ContractName: spec.name, ModuleID: spec.moduleID,
		SourceName: artifact.SourceName, SourceSHA256: artifact.SourceSHA256,
		CreationBytecodeSHA256: artifact.CreationBytecodeSHA256, RuntimeTemplateSHA256: artifact.RuntimeTemplateSHA256,
		ConstructorArguments: spec.argument, ConstructorArgsABI: "0x" + hex.EncodeToString(constructor),
		InitcodeSHA256: hex.EncodeToString(initcodeDigest[:]),
	}
	deployment.manifest.Contracts = append(deployment.manifest.Contracts, evidence)
	deployment.manifest.UpdatedAt = deployment.now()
	if err := deployment.writer.persist(deployment.manifest); err != nil {
		return err
	}
	result, txErr := deployment.tx.Deploy(deployment.ctx, initcode, "0x0")
	if result != nil {
		evidence.TransactionHash = normalizeHash(result.Hash)
		evidence.Receipt = result.Receipt
		deployment.manifest.UpdatedAt = deployment.now()
		if err := deployment.writer.persist(deployment.manifest); err != nil {
			return err
		}
	}
	if txErr != nil {
		return deployment.fail("deploy_"+spec.name, txErr)
	}
	if result == nil || result.Receipt == nil {
		return deployment.fail("deploy_"+spec.name, fmt.Errorf("transactor returned no confirmed receipt"))
	}
	address, err := validateDeploymentReceipt(result, evidence.TransactionHash)
	if err != nil {
		return deployment.fail("validate_"+spec.name+"_receipt", err)
	}
	evidence.Address = lowerAddress(address)
	runtime, runtimeHash, err := deployment.verifyRuntime(spec.name, address, result.Receipt)
	if err != nil {
		return deployment.fail("verify_"+spec.name+"_runtime", err)
	}
	evidence.Runtime = runtime
	deployment.addresses[spec.name] = address
	deployment.codeHash[spec.name] = runtimeHash
	deployment.manifest.UpdatedAt = deployment.now()
	return deployment.writer.persist(deployment.manifest)
}

func (deployment *deploymentContext) configure(purpose, moduleID, targetName, method string, args []any) error {
	artifact := deployment.artifacts.byName[targetName]
	calldata, err := artifact.parsedABI.Pack(method, args...)
	if err != nil {
		return deployment.fail("encode_"+purpose, err)
	}
	digest := sha256.Sum256(calldata)
	deployment.nextOrder++
	evidence := &ConfigurationTransaction{
		TransactionOrder: deployment.nextOrder, Purpose: purpose, ModuleID: moduleID,
		Target: lowerAddress(deployment.addresses[targetName]), Calldata: "0x" + hex.EncodeToString(calldata),
		CalldataSHA256: hex.EncodeToString(digest[:]),
	}
	deployment.manifest.ConfigurationTransactions = append(deployment.manifest.ConfigurationTransactions, evidence)
	deployment.manifest.UpdatedAt = deployment.now()
	if err := deployment.writer.persist(deployment.manifest); err != nil {
		return err
	}
	result, txErr := deployment.tx.Send(deployment.ctx, evidence.Target, calldata, "0x0")
	if result != nil {
		evidence.TransactionHash = normalizeHash(result.Hash)
		evidence.Receipt = result.Receipt
		deployment.manifest.UpdatedAt = deployment.now()
		if err := deployment.writer.persist(deployment.manifest); err != nil {
			return err
		}
	}
	if txErr != nil {
		return deployment.fail(purpose, txErr)
	}
	if result == nil || result.Receipt == nil {
		return deployment.fail(purpose, fmt.Errorf("transactor returned no confirmed receipt"))
	}
	if _, err := validateCallReceipt(result, evidence.TransactionHash, evidence.Target); err != nil {
		return deployment.fail("validate_"+purpose+"_receipt", err)
	}
	block, err := deployment.rpc.BlockByNumber(deployment.ctx, result.Receipt.BlockNumber)
	if err != nil {
		return deployment.fail("pin_"+purpose+"_block", err)
	}
	if !equalHash(block.Hash, result.Receipt.BlockHash) || !strings.EqualFold(block.Number, result.Receipt.BlockNumber) {
		return deployment.fail("pin_"+purpose+"_block", fmt.Errorf("receipt block changed or returned inconsistent evidence"))
	}
	return nil
}

func (deployment *deploymentContext) verifyRuntime(name string, address common.Address, receipt *chain.EVMReceipt) (*RuntimeEvidence, [32]byte, error) {
	if _, err := validateReceiptBlock(receipt); err != nil {
		return nil, [32]byte{}, err
	}
	block, err := deployment.rpc.BlockByNumber(deployment.ctx, receipt.BlockNumber)
	if err != nil {
		return nil, [32]byte{}, err
	}
	if !equalHash(block.Hash, receipt.BlockHash) || !strings.EqualFold(block.Number, receipt.BlockNumber) {
		return nil, [32]byte{}, fmt.Errorf("receipt block changed before runtime verification")
	}
	observed, err := deployment.rpc.CodeAt(deployment.ctx, lowerAddress(address), receipt.BlockNumber)
	if err != nil {
		return nil, [32]byte{}, err
	}
	artifact := deployment.artifacts.byName[name]
	immutableValues, err := compareRuntimeTemplate(artifact.runtime, observed, artifact.ImmutableReferences)
	if err != nil {
		return nil, [32]byte{}, err
	}
	blockAfter, err := deployment.rpc.BlockByNumber(deployment.ctx, receipt.BlockNumber)
	if err != nil {
		return nil, [32]byte{}, err
	}
	if !equalHash(blockAfter.Hash, receipt.BlockHash) || !equalHash(blockAfter.Hash, block.Hash) {
		return nil, [32]byte{}, fmt.Errorf("receipt block changed during runtime verification")
	}
	shaDigest := sha256.Sum256(observed)
	keccakDigest := ethcrypto.Keccak256Hash(observed)
	var codeHash [32]byte
	copy(codeHash[:], keccakDigest[:])
	return &RuntimeEvidence{
		BlockNumber: receipt.BlockNumber, BlockHash: normalizeHash(receipt.BlockHash), ByteLength: len(observed),
		SHA256: hex.EncodeToString(shaDigest[:]), CodeHashKeccak256: strings.ToLower(keccakDigest.Hex()),
		TemplateSHA256: artifact.RuntimeTemplateSHA256, ImmutableValuesByID: immutableValues,
		TemplateMatchVerified: true,
	}, codeHash, nil
}

func compareRuntimeTemplate(template, observed []byte, references map[string][]ImmutableReference) (map[string]string, error) {
	if len(observed) != len(template) {
		return nil, fmt.Errorf("runtime length %d does not match %d-byte template", len(observed), len(template))
	}
	mutable := make([]bool, len(template))
	values := make(map[string]string, len(references))
	keys := make([]string, 0, len(references))
	for key := range references {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var expected []byte
		for _, reference := range references[key] {
			start, end := int(reference.Start), int(reference.Start+reference.Length)
			value := observed[start:end]
			if expected == nil {
				expected = append([]byte(nil), value...)
			} else if !bytes.Equal(expected, value) {
				return nil, fmt.Errorf("immutable %s has inconsistent values across runtime references", key)
			}
			for index := start; index < end; index++ {
				mutable[index] = true
			}
		}
		values[key] = "0x" + hex.EncodeToString(expected)
	}
	for index := range template {
		if !mutable[index] && template[index] != observed[index] {
			return nil, fmt.Errorf("runtime differs from compiler template outside immutable references at byte %d", index)
		}
	}
	return values, nil
}

func validateDeploymentReceipt(result *chain.EVMTransactionResult, expectedHash string) (common.Address, error) {
	if _, err := validateCallReceipt(result, expectedHash, ""); err != nil {
		return common.Address{}, err
	}
	normalized, err := chain.NormalizeEVMAddress(result.Receipt.ContractAddress)
	if err != nil {
		return common.Address{}, fmt.Errorf("invalid receipt contractAddress: %w", err)
	}
	address := common.HexToAddress(normalized)
	if address == (common.Address{}) {
		return common.Address{}, fmt.Errorf("receipt contractAddress is zero")
	}
	if strings.TrimSpace(result.Receipt.To) != "" && !strings.EqualFold(result.Receipt.To, "0x0000000000000000000000000000000000000000") {
		return common.Address{}, fmt.Errorf("contract creation receipt unexpectedly has destination %q", result.Receipt.To)
	}
	return address, nil
}

func validateCallReceipt(result *chain.EVMTransactionResult, expectedHash, expectedTarget string) (*chain.EVMReceipt, error) {
	if result == nil || result.Receipt == nil {
		return nil, fmt.Errorf("missing transaction receipt")
	}
	receipt := result.Receipt
	if !equalHash(result.Hash, expectedHash) || !equalHash(receipt.TransactionHash, expectedHash) {
		return nil, fmt.Errorf("transaction and receipt hashes do not match")
	}
	status := strings.ToLower(strings.TrimSpace(receipt.Status))
	if status != "0x1" && status != "1" {
		return nil, fmt.Errorf("transaction receipt status is %q, want success", receipt.Status)
	}
	if _, err := validateReceiptBlock(receipt); err != nil {
		return nil, err
	}
	if expectedTarget != "" {
		normalized, err := chain.NormalizeEVMAddress(receipt.To)
		if err != nil || normalized != strings.ToLower(expectedTarget) {
			return nil, fmt.Errorf("receipt destination %q does not match %s", receipt.To, expectedTarget)
		}
	}
	return receipt, nil
}

func validateReceiptBlock(receipt *chain.EVMReceipt) (uint64, error) {
	if receipt == nil {
		return 0, fmt.Errorf("missing receipt")
	}
	blockNumber, err := parseHexQuantity(receipt.BlockNumber)
	if err != nil || blockNumber == 0 {
		return 0, fmt.Errorf("invalid receipt block number %q", receipt.BlockNumber)
	}
	if !validHash(receipt.BlockHash) {
		return 0, fmt.Errorf("invalid receipt block hash %q", receipt.BlockHash)
	}
	return blockNumber, nil
}

func validateSourceCommit(value string) error {
	value = strings.TrimSpace(value)
	if (len(value) != 40 && len(value) != 64) || value != strings.ToLower(value) {
		return fmt.Errorf("source commit must be a 40- or 64-character lower-case Git object ID")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("decode source commit: %w", err)
	}
	allZero := true
	for _, b := range decoded {
		allZero = allZero && b == 0
	}
	if allZero {
		return fmt.Errorf("source commit cannot be zero")
	}
	return nil
}

func evidenceEndpoint(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "redacted-invalid-endpoint"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func parseBytes32(value string) ([32]byte, error) {
	var result [32]byte
	if !strings.HasPrefix(value, "0x") || value != strings.ToLower(value) || len(value) != 66 {
		return result, fmt.Errorf("value must be 0x followed by 64 lower-case hex characters")
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil {
		return result, err
	}
	copy(result[:], decoded)
	return result, nil
}

func parseHexQuantity(value string) (uint64, error) {
	value = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(value), "0x"), "0X")
	if value == "" || len(value) > 16 {
		return 0, fmt.Errorf("invalid hex quantity")
	}
	return strconv.ParseUint(value, 16, 64)
}

func validHash(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil {
		return false
	}
	for _, b := range decoded {
		if b != 0 {
			return true
		}
	}
	return false
}

func equalHash(left, right string) bool {
	return validHash(left) && validHash(right) && strings.EqualFold(left, right)
}

func normalizeHash(value string) string {
	if validHash(value) {
		return strings.ToLower(value)
	}
	return strings.TrimSpace(value)
}

func hashHex(value [32]byte) string { return "0x" + hex.EncodeToString(value[:]) }

func lowerAddress(value common.Address) string { return strings.ToLower(value.Hex()) }

func moduleIDHex(contractName string) string {
	return strings.ToLower(ethcrypto.Keccak256Hash([]byte(moduleLabels[contractName])).Hex())
}

func moduleConstructorEvidence(name string, directory, coordinator, operator common.Address, fee uint16) []ConstructorArgument {
	args := []ConstructorArgument{
		{Name: "directory", Type: "address", Value: lowerAddress(directory)},
		{Name: "coordinator", Type: "address", Value: lowerAddress(coordinator)},
	}
	switch name {
	case "ModerationModule":
		args = append(args,
			ConstructorArgument{Name: "admin_", Type: "address", Value: lowerAddress(operator)},
			ConstructorArgument{Name: "committee_", Type: "address", Value: lowerAddress(operator)},
		)
	case "EconomicModule":
		args = append(args,
			ConstructorArgument{Name: "admin_", Type: "address", Value: lowerAddress(operator)},
			ConstructorArgument{Name: "treasury_", Type: "address", Value: lowerAddress(operator)},
			ConstructorArgument{Name: "platformFeeBps_", Type: "uint16", Value: strconv.FormatUint(uint64(fee), 10)},
		)
	case "UsernameModule":
		args = append(args, ConstructorArgument{Name: "policyAdmin_", Type: "address", Value: lowerAddress(operator)})
	case "ReleaseModule":
		args = append(args, ConstructorArgument{Name: "releaseAuthority_", Type: "address", Value: lowerAddress(operator)})
	}
	return args
}

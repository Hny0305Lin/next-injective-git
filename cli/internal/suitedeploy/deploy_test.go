package suitedeploy

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func testArtifactDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "contracts", "evm-v2", "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectArtifactsUsesLockedSolcArtifacts(t *testing.T) {
	inspection, err := InspectArtifacts(testArtifactDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Compiler.Version != "0.8.24" || !inspection.Compiler.ViaIR ||
		!inspection.Compiler.Optimizer.Enabled || inspection.Compiler.Optimizer.Runs != 1 {
		t.Fatalf("unexpected compiler evidence: %#v", inspection.Compiler)
	}
	if len(inspection.Contracts) != 9 || len(inspection.ArtifactSetSHA256) != 64 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if inspection.Contracts[0].ContractName != "SuiteDirectory" || inspection.Contracts[8].ContractName != "ReleaseModule" {
		t.Fatalf("artifact order = %s ... %s", inspection.Contracts[0].ContractName, inspection.Contracts[8].ContractName)
	}
}

func TestCompareRuntimeTemplateRejectsNonImmutableDifference(t *testing.T) {
	template := make([]byte, 96)
	for index := range template {
		template[index] = byte(index)
	}
	references := map[string][]ImmutableReference{
		"1": {{Start: 32, Length: 32}},
	}
	observed := append([]byte(nil), template...)
	for index := 32; index < 64; index++ {
		observed[index] = 0xab
	}
	values, err := compareRuntimeTemplate(template, observed, references)
	if err != nil {
		t.Fatal(err)
	}
	if values["1"] != "0x"+strings.Repeat("ab", 32) {
		t.Fatalf("immutable value = %q", values["1"])
	}
	observed[3] ^= 0xff
	if _, err := compareRuntimeTemplate(template, observed, references); err == nil || !strings.Contains(err.Error(), "outside immutable") {
		t.Fatalf("non-immutable mismatch error = %v", err)
	}
}

func TestDeployFullSuiteWritesNoClobberEvidence(t *testing.T) {
	artifacts, err := loadArtifactSet(testArtifactDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := parseBytes32("0x" + strings.Repeat("11", 32))
	operator := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	fixture := newDeployFixture(artifacts, snapshot, operator)
	path := filepath.Join(t.TempDir(), "deployment.json")
	clock := func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }
	manifest, err := Deploy(context.Background(), Options{
		ArtifactDirectory: testArtifactDirectory(t), SourceCommit: testCommit,
		Network: "injective-testnet", ChainID: 1439, RPCEndpoint: "http://fixture.invalid",
		BlockExplorer: "https://fixture.blockscout.invalid", SnapshotRoot: hashHex(snapshot),
		Operator: lowerAddress(operator), PlatformFeeBPS: 300, Clock: clock,
	}, fixture, fixture, path)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "bootstrapping" || manifest.DirectoryBindingVerification == nil || manifest.DirectoryBindingVerification.Active {
		t.Fatalf("unexpected deployment status: %#v", manifest.DirectoryBindingVerification)
	}
	if len(manifest.Contracts) != 9 || len(manifest.ConfigurationTransactions) != 8 || fixture.transactionCount != 17 {
		t.Fatalf("contracts=%d configuration=%d transactions=%d", len(manifest.Contracts), len(manifest.ConfigurationTransactions), fixture.transactionCount)
	}
	for index, name := range contractOrder {
		contract := manifest.Contracts[index]
		if contract.ContractName != name || contract.Runtime == nil || !contract.Runtime.TemplateMatchVerified || contract.TransactionHash == "" {
			t.Fatalf("contract %d evidence = %#v", index, contract)
		}
		if index > 0 && contract.TransactionOrder <= manifest.Contracts[index-1].TransactionOrder {
			t.Fatalf("contract deployment order did not advance")
		}
	}
	if manifest.ConfigurationTransactions[0].Purpose != "bind_bootstrap_coordinator" {
		t.Fatalf("first configuration = %q", manifest.ConfigurationTransactions[0].Purpose)
	}
	if manifest.BlockscoutVerification.Status != "not_attempted" || manifest.Governance.ProductionReady {
		t.Fatalf("unsafe governance/verification evidence: %#v %#v", manifest.Governance, manifest.BlockscoutVerification)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"directory_binding_verification"`) || !strings.Contains(string(data), `"template_match_verified": true`) {
		t.Fatalf("evidence file is incomplete: %s", data)
	}
	before := fixture.transactionCount
	if _, err := Deploy(context.Background(), Options{
		ArtifactDirectory: testArtifactDirectory(t), SourceCommit: testCommit,
		Network: "injective-testnet", ChainID: 1439, RPCEndpoint: "http://fixture.invalid",
		SnapshotRoot: hashHex(snapshot), Operator: lowerAddress(operator), PlatformFeeBPS: 300,
	}, fixture, fixture, path); err == nil || !strings.Contains(err.Error(), "will not be overwritten") {
		t.Fatalf("no-clobber error = %v", err)
	}
	if fixture.transactionCount != before {
		t.Fatalf("no-clobber check happened after broadcast: %d -> %d", before, fixture.transactionCount)
	}
}

func TestDeployPersistsBroadcastHashOnUnconfirmedReceipt(t *testing.T) {
	artifacts, err := loadArtifactSet(testArtifactDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := parseBytes32("0x" + strings.Repeat("22", 32))
	operator := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	fixture := newDeployFixture(artifacts, snapshot, operator)
	fixture.failFirstDeploy = true
	path := filepath.Join(t.TempDir(), "deployment.json")
	manifest, err := Deploy(context.Background(), Options{
		ArtifactDirectory: testArtifactDirectory(t), SourceCommit: testCommit,
		Network: "injective-testnet", ChainID: 1439, RPCEndpoint: "http://fixture.invalid",
		SnapshotRoot: hashHex(snapshot), Operator: lowerAddress(operator), PlatformFeeBPS: 300,
	}, fixture, fixture, path)
	if err == nil || !strings.Contains(err.Error(), "receipt was not confirmed") {
		t.Fatalf("deployment error = %v", err)
	}
	if manifest == nil || manifest.Status != "failed" || len(manifest.Contracts) != 1 || manifest.Contracts[0].TransactionHash == "" {
		t.Fatalf("uncertain-state manifest = %#v", manifest)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), manifest.Contracts[0].TransactionHash) || !strings.Contains(string(data), `"status": "failed"`) {
		t.Fatalf("uncertain tx evidence was not persisted: %s", data)
	}
}

func TestRecoverExistingDeploymentRevalidatesHistoricalTransactions(t *testing.T) {
	artifacts, err := loadArtifactSet(testArtifactDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := parseBytes32("0x" + strings.Repeat("11", 32))
	operator := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	fixture := newDeployFixture(artifacts, snapshot, operator)
	options := Options{
		ArtifactDirectory: testArtifactDirectory(t), SourceCommit: testCommit,
		Network: "injective-testnet", ChainID: 1439, RPCEndpoint: "http://fixture.invalid",
		BlockExplorer: "https://fixture.blockscout.invalid", SnapshotRoot: hashHex(snapshot),
		Operator: lowerAddress(operator), PlatformFeeBPS: 300,
		Clock: func() time.Time { return time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC) },
	}
	live, err := Deploy(context.Background(), options, fixture, fixture, filepath.Join(t.TempDir(), "live.json"))
	if err != nil {
		t.Fatal(err)
	}

	references := make([]RecoveryTransactionReference, 17)
	transactions := make(map[string]*HistoricalTransaction, 17)
	for _, contract := range live.Contracts {
		constructor, err := decodeBytecode("constructor args", contract.ConstructorArgsABI)
		if err != nil {
			t.Fatal(err)
		}
		input := append(append([]byte(nil), artifacts.byName[contract.ContractName].creation...), constructor...)
		purpose := "deploy_" + contract.ContractName
		references[contract.TransactionOrder-1] = RecoveryTransactionReference{Purpose: purpose, TransactionHash: contract.TransactionHash}
		transactions[contract.TransactionHash] = historicalFromReceipt(contract.TransactionHash, lowerAddress(operator), "", contract.Address, "0x"+hex.EncodeToString(input), contract.TransactionOrder, contract.Receipt)
	}
	for _, configuration := range live.ConfigurationTransactions {
		references[configuration.TransactionOrder-1] = RecoveryTransactionReference{Purpose: configuration.Purpose, TransactionHash: configuration.TransactionHash}
		transactions[configuration.TransactionHash] = historicalFromReceipt(configuration.TransactionHash, lowerAddress(operator), configuration.Target, "", configuration.Calldata, configuration.TransactionOrder, configuration.Receipt)
	}
	input := &RecoveryInput{Schema: recoveryInputSchema, Transactions: references}
	recovered, err := Recover(context.Background(), options, fixture, &historicalFixture{transactions: transactions}, input, filepath.Join(t.TempDir(), "recovered.json"))
	if err != nil {
		t.Fatal(err)
	}
	if recovered.EvidenceMode != evidenceModeHistoricalRecovery || recovered.Recovery == nil || recovered.Recovery.TransactionsValidated != 17 {
		t.Fatalf("recovery metadata = %#v", recovered.Recovery)
	}
	if recovered.Status != "bootstrapping" || recovered.DirectoryBindingVerification == nil || recovered.DirectoryBindingVerification.Active {
		t.Fatalf("recovered deployment status = %#v", recovered.DirectoryBindingVerification)
	}
	if len(recovered.Contracts) != 9 || len(recovered.ConfigurationTransactions) != 8 {
		t.Fatalf("contracts=%d configurations=%d", len(recovered.Contracts), len(recovered.ConfigurationTransactions))
	}
	for index := range recovered.Contracts {
		if recovered.Contracts[index].Address != live.Contracts[index].Address || recovered.Contracts[index].Runtime.CodeHashKeccak256 != live.Contracts[index].Runtime.CodeHashKeccak256 {
			t.Fatalf("recovered contract %d mismatch", index)
		}
	}
}

type historicalFixture struct {
	transactions map[string]*HistoricalTransaction
}

func (fixture *historicalFixture) Description() string { return "https://fixture.blockscout.invalid" }

func (fixture *historicalFixture) Transaction(_ context.Context, hash string) (*HistoricalTransaction, error) {
	transaction := fixture.transactions[normalizeHash(hash)]
	if transaction == nil {
		return nil, fmt.Errorf("missing historical transaction %s", hash)
	}
	copy := *transaction
	return &copy, nil
}

func historicalFromReceipt(hash, from, to, contractAddress, input string, order uint64, receipt *chain.EVMReceipt) *HistoricalTransaction {
	blockNumber, _ := parseHexQuantity(receipt.BlockNumber)
	return &HistoricalTransaction{
		Hash: hash, From: from, To: to, ContractAddress: contractAddress, Input: input,
		BlockNumber: blockNumber, GasUsed: receipt.GasUsed, Nonce: 100 + order,
		Timestamp: time.Date(2026, 8, 19, 18, 0, int(order), 0, time.UTC).Format(time.RFC3339Nano), Successful: true,
	}
}

type deployFixture struct {
	artifacts        *artifactSet
	snapshot         [32]byte
	operator         common.Address
	addresses        map[string]common.Address
	contractByAddr   map[string]string
	codeByAddr       map[string][]byte
	blocks           map[string]*chain.EVMBlock
	deployments      int
	transactionCount int
	failFirstDeploy  bool
}

func newDeployFixture(artifacts *artifactSet, snapshot [32]byte, operator common.Address) *deployFixture {
	return &deployFixture{
		artifacts: artifacts, snapshot: snapshot, operator: operator,
		addresses: make(map[string]common.Address), contractByAddr: make(map[string]string),
		codeByAddr: make(map[string][]byte), blocks: make(map[string]*chain.EVMBlock),
	}
}

func (fixture *deployFixture) ChainID(context.Context) (uint64, error) { return 1439, nil }

func (fixture *deployFixture) Deploy(_ context.Context, _ []byte, _ string) (*chain.EVMTransactionResult, error) {
	name := contractOrder[fixture.deployments]
	fixture.deployments++
	fixture.transactionCount++
	address := common.BigToAddress(big.NewInt(int64(0x100 + fixture.deployments)))
	fixture.addresses[name] = address
	fixture.contractByAddr[lowerAddress(address)] = name
	runtime := append([]byte(nil), fixture.artifacts.byName[name].runtime...)
	for groupIndex, references := range fixture.artifacts.byName[name].ImmutableReferences {
		value := ethcrypto.Keccak256([]byte(name + ":" + groupIndex))
		for _, reference := range references {
			copy(runtime[int(reference.Start):int(reference.Start+reference.Length)], value[:reference.Length])
		}
	}
	fixture.codeByAddr[lowerAddress(address)] = runtime
	result := fixture.result(address, "")
	if fixture.failFirstDeploy && fixture.deployments == 1 {
		result.Receipt = nil
		return result, &chain.EVMReceiptUnconfirmedError{Hash: result.Hash, Err: context.DeadlineExceeded}
	}
	return result, nil
}

func (fixture *deployFixture) Send(_ context.Context, target string, _ []byte, _ string) (*chain.EVMTransactionResult, error) {
	fixture.transactionCount++
	return fixture.result(common.Address{}, strings.ToLower(target)), nil
}

func (fixture *deployFixture) result(contract common.Address, target string) *chain.EVMTransactionResult {
	number := fmt.Sprintf("0x%x", fixture.transactionCount)
	txHash := fmt.Sprintf("0x%064x", fixture.transactionCount)
	blockHash := fmt.Sprintf("0x%064x", 0x1000+fixture.transactionCount)
	fixture.blocks[number] = &chain.EVMBlock{Number: number, Hash: blockHash}
	receipt := &chain.EVMReceipt{
		TransactionHash: txHash, BlockNumber: number, BlockHash: blockHash,
		To: target, Status: "0x1", GasUsed: "0x5208",
	}
	if contract != (common.Address{}) {
		receipt.ContractAddress = lowerAddress(contract)
	}
	return &chain.EVMTransactionResult{Hash: txHash, Receipt: receipt}
}

func (fixture *deployFixture) BlockByNumber(_ context.Context, block string) (*chain.EVMBlock, error) {
	value := fixture.blocks[block]
	if value == nil {
		return nil, fmt.Errorf("missing fixture block %s", block)
	}
	copy := *value
	return &copy, nil
}

func (fixture *deployFixture) CodeAt(_ context.Context, address, _ string) ([]byte, error) {
	code := fixture.codeByAddr[strings.ToLower(address)]
	if len(code) == 0 {
		return nil, fmt.Errorf("missing fixture code for %s", address)
	}
	return append([]byte(nil), code...), nil
}

func (fixture *deployFixture) CallContractAt(_ context.Context, address, calldata, _ string) ([]byte, error) {
	name := fixture.contractByAddr[strings.ToLower(address)]
	artifact := fixture.artifacts.byName[name]
	raw, err := hex.DecodeString(strings.TrimPrefix(calldata, "0x"))
	if err != nil || len(raw) < 4 {
		return nil, fmt.Errorf("invalid fixture calldata")
	}
	var methodName string
	for candidate, method := range artifact.parsedABI.Methods {
		if string(method.ID) == string(raw[:4]) {
			methodName = candidate
			break
		}
	}
	if methodName == "" {
		return nil, fmt.Errorf("unknown %s selector %x", name, raw[:4])
	}
	method := artifact.parsedABI.Methods[methodName]
	value, err := fixture.callValue(name, methodName, raw[4:])
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(value)
}

func (fixture *deployFixture) callValue(contractName, method string, arguments []byte) (any, error) {
	directory := fixture.addresses["SuiteDirectory"]
	coordinator := fixture.addresses["BootstrapCoordinator"]
	switch contractName {
	case "SuiteDirectory":
		switch method {
		case "suiteVersion":
			return uint64(3), nil
		case "state":
			return uint8(0), nil
		case "configuredChainId":
			return big.NewInt(1439), nil
		case "snapshotRoot":
			return fixture.snapshot, nil
		case "bootstrapAuthority":
			return common.Address{}, nil
		case "bootstrapCoordinator":
			return coordinator, nil
		case "bootstrapCoordinatorCodeHash":
			return fixture.codeHash("BootstrapCoordinator"), nil
		case "registeredModuleCount":
			return big.NewInt(7), nil
		case "verifyModule":
			return true, nil
		case "moduleAddress", "moduleCodeHash":
			if len(arguments) < 32 {
				return nil, fmt.Errorf("missing module ID")
			}
			var id [32]byte
			copy(id[:], arguments[:32])
			name := fixture.moduleName(id)
			if method == "moduleAddress" {
				return fixture.addresses[name], nil
			}
			return fixture.codeHash(name), nil
		}
	case "BootstrapCoordinator":
		switch method {
		case "suiteDirectory":
			return directory, nil
		case "snapshotRoot":
			return fixture.snapshot, nil
		case "operator":
			return fixture.operator, nil
		case "activated":
			return false, nil
		case "nextModuleIndex":
			return big.NewInt(0), nil
		}
	default:
		switch method {
		case "suiteDirectory":
			return directory, nil
		case "bootstrapCoordinator":
			return coordinator, nil
		case "moduleId":
			id, _ := parseBytes32(moduleIDHex(contractName))
			return id, nil
		case "bootstrapFinalized":
			return false, nil
		case "admin", "policyAdmin", "releaseAuthority", "committee", "treasury":
			return fixture.operator, nil
		case "platformFeeBps":
			return uint16(300), nil
		}
	}
	return nil, fmt.Errorf("unhandled fixture call %s.%s", contractName, method)
}

func (fixture *deployFixture) codeHash(name string) [32]byte {
	hash := ethcrypto.Keccak256Hash(fixture.codeByAddr[lowerAddress(fixture.addresses[name])])
	return [32]byte(hash)
}

func (fixture *deployFixture) moduleName(id [32]byte) string {
	for _, name := range contractOrder[2:] {
		expected, _ := parseBytes32(moduleIDHex(name))
		if expected == id {
			return name
		}
	}
	return ""
}

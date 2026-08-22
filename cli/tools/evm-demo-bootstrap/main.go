// Command evm-demo-bootstrap runs a reviewed, state-aware bootstrap manifest
// for a standalone EVM demo Suite. It never invents import records or evidence.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitemigration"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type artifact struct {
	ABI json.RawMessage `json:"abi"`
}

type progress struct {
	Started       bool
	Finalized     bool
	ExpectedCount uint64
	ExpectedBatch uint64
	ImportedCount uint64
	NextSequence  uint64
	ExpectedRoot  common.Hash
	RollingRoot   common.Hash
}

type coordinatorState struct {
	NextModule uint64
	Evidence   common.Hash
	Activated  bool
	Ready      bool
}

type journalEntry struct {
	Order          int    `json:"order"`
	Phase          string `json:"phase"`
	Function       string `json:"function"`
	Hash           string `json:"hash,omitempty"`
	Status         string `json:"status"`
	BlockNumber    string `json:"block_number,omitempty"`
	AlreadyApplied bool   `json:"already_applied,omitempty"`
}

type journal struct {
	Schema       string         `json:"schema"`
	Status       string         `json:"status"`
	Directory    string         `json:"directory"`
	Coordinator  string         `json:"coordinator"`
	PlanSHA256   string         `json:"plan_sha256"`
	SnapshotRoot string         `json:"snapshot_root"`
	Transactions []journalEntry `json:"transactions"`
}

func main() {
	var planPath, manifestPath, evidencePath, keyName, directory, rpcURL, artifacts string
	var chainID uint64
	flag.StringVar(&planPath, "plan", "", "validated bootstrap plan JSON")
	flag.StringVar(&manifestPath, "manifest", "", "validated unsigned calldata manifest JSON")
	flag.StringVar(&evidencePath, "evidence", "", "exclusive broadcast journal output")
	flag.StringVar(&keyName, "key", "", "encrypted EVM key name")
	flag.StringVar(&directory, "directory", "", "expected SuiteDirectory address")
	flag.StringVar(&rpcURL, "rpc", "", "EVM JSON-RPC endpoint (defaults to config)")
	flag.StringVar(&artifacts, "artifacts", "contracts/evm-v2/artifacts", "artifact directory")
	flag.Uint64Var(&chainID, "chain-id", 1439, "expected EVM chain ID")
	flag.Parse()
	if planPath == "" || manifestPath == "" || evidencePath == "" || keyName == "" || directory == "" {
		fatal("-plan, -manifest, -evidence, -key, and -directory are required")
	}

	plan, err := suitemigration.ReadPlan(planPath)
	if err != nil {
		fatal(fmt.Sprintf("read plan: %v", err))
	}
	manifest, err := suitemigration.ReadCalldataManifest(manifestPath, plan)
	if err != nil {
		fatal(fmt.Sprintf("read calldata manifest: %v", err))
	}
	if !manifest.CalldataReady || manifest.Signed || manifest.Broadcast {
		fatal("manifest must be calldata-ready, unsigned, and not broadcast")
	}
	planSHA, err := suitemigration.PlanSHA256(plan)
	if err != nil {
		fatal(fmt.Sprintf("hash plan: %v", err))
	}
	if manifest.PlanSHA256 != planSHA {
		fatal(fmt.Sprintf("manifest plan hash %s does not match plan %s", manifest.PlanSHA256, planSHA))
	}
	directory = common.HexToAddress(directory).Hex()
	if !strings.EqualFold(plan.Target.Directory, directory) || plan.Target.ChainID != chainID {
		fatal("plan target does not match explicit directory or chain ID")
	}
	if len(manifest.Transactions) != 16 {
		fatal(fmt.Sprintf("demo manifest must contain exactly 16 transactions, got %d", len(manifest.Transactions)))
	}
	for _, module := range plan.Modules {
		if module.ExpectedCount != 0 || module.ExpectedBatches != 0 || len(module.Batches) != 0 {
			fatal("demo runner refuses a non-empty import plan")
		}
	}
	for index, item := range manifest.Transactions {
		if item.Order != index || !strings.EqualFold(item.To, plan.Target.Coordinator) {
			fatal(fmt.Sprintf("manifest transaction %d is not canonical", index))
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Sprintf("load config: %v", err))
	}
	if rpcURL == "" {
		rpcURL = cfg.EffectiveEVMRPC()
	}
	if rpcURL == "" {
		fatal("EVM RPC endpoint is not configured")
	}
	cfg.KeyName = keyName
	cfg.EVMChainID = chainID
	cfg.EVMSuiteDirectoryAddress = directory
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	rpc := chain.NewEVMRPC(rpcURL)
	coordABI, err := loadABI(artifacts, "BootstrapCoordinator")
	if err != nil {
		fatal(err.Error())
	}
	dirABI, err := loadABI(artifacts, "SuiteDirectory")
	if err != nil {
		fatal(err.Error())
	}
	coordinator := common.HexToAddress(plan.Target.Coordinator).Hex()
	if err := verifyTarget(ctx, rpc, dirABI, coordABI, directory, coordinator, common.HexToHash(plan.Snapshot.Root)); err != nil {
		fatal(err.Error())
	}
	if _, err := os.Stat(evidencePath); err == nil {
		fatal(fmt.Sprintf("evidence already exists: %s", evidencePath))
	} else if !errors.Is(err, os.ErrNotExist) {
		fatal(fmt.Sprintf("inspect evidence path: %v", err))
	}
	j := journal{Schema: "igit.evm-suite.demo-bootstrap-broadcast.v2", Status: "broadcasting", Directory: strings.ToLower(directory), Coordinator: strings.ToLower(coordinator), PlanSHA256: planSHA, SnapshotRoot: strings.ToLower(plan.Snapshot.Root)}
	writeJournal(evidencePath, j)

	signer := chain.NewEVMKeystoreSigner(cfg)
	operator, err := signer.OwnerAddress()
	if err != nil {
		fatal(fmt.Sprintf("resolve operator: %v", err))
	}
	fmt.Printf("operator=%s\ncoordinator=%s\ndirectory=%s\n", operator, coordinator, directory)
	tx := chain.NewEVMTransactor(cfg, rpc, signer)
	// State confirmation below is the source of truth when receipt indexing lags.
	tx.SetReceiptTimeout(12 * time.Second)

	for index, item := range manifest.Transactions {
		state, stateErr := readCoordinatorState(ctx, rpc, coordABI, coordinator)
		if stateErr != nil {
			fatal(fmt.Sprintf("read coordinator state before %d: %v", index, stateErr))
		}
		applied, applyErr := transactionApplied(item, plan, state, ctx, rpc, coordABI, coordinator)
		if applyErr != nil {
			fatal(fmt.Sprintf("check transaction %d (%s): %v", index, item.Phase, applyErr))
		}
		entry := journalEntry{Order: index, Phase: item.Phase, Function: item.Function}
		if applied {
			entry.Status = "already_applied"
			entry.AlreadyApplied = true
			j.Transactions = append(j.Transactions, entry)
			writeJournal(evidencePath, j)
			fmt.Printf("%02d %s already applied\n", index, item.Phase)
			continue
		}

		data, decodeErr := decodeHex(item.Data)
		if decodeErr != nil {
			fatal(fmt.Sprintf("decode transaction %d: %v", index, decodeErr))
		}
		result, sendErr := tx.Send(ctx, coordinator, data, item.Value)
		if sendErr != nil {
			var uncertain *chain.EVMReceiptUnconfirmedError
			if !errors.As(sendErr, &uncertain) {
				fatal(fmt.Sprintf("broadcast transaction %d (%s): %v", index, item.Phase, sendErr))
			}
			fmt.Printf("%02d %s broadcast %s; receipt delayed, checking state\n", index, item.Phase, uncertain.Hash)
			entry.Hash, entry.Status = uncertain.Hash, "state_pending"
			if err := waitUntilApplied(ctx, rpc, coordABI, coordinator, item, plan); err != nil {
				fatal(fmt.Sprintf("transaction %d (%s) uncertain and state not applied: %v", index, item.Phase, err))
			}
			entry.Status = "confirmed_by_state"
		} else {
			if result == nil || strings.TrimSpace(result.Hash) == "" {
				fatal(fmt.Sprintf("transaction %d (%s) returned no hash", index, item.Phase))
			}
			entry.Hash = result.Hash
			entry.Status = "receipt_confirmed"
			if result.Receipt != nil {
				entry.BlockNumber = result.Receipt.BlockNumber
			}
			if err := waitUntilApplied(ctx, rpc, coordABI, coordinator, item, plan); err != nil {
				fatal(fmt.Sprintf("transaction %d (%s) receipt succeeded but state did not apply: %v", index, item.Phase, err))
			}
		}
		j.Transactions = append(j.Transactions, entry)
		writeJournal(evidencePath, j)
		fmt.Printf("%02d %s %s\n", index, item.Phase, entry.Hash)
	}

	finalState, err := readCoordinatorState(ctx, rpc, coordABI, coordinator)
	if err != nil {
		fatal(fmt.Sprintf("read final coordinator state: %v", err))
	}
	directoryState, err := readUint8(ctx, rpc, dirABI, directory, "state")
	if err != nil {
		fatal(fmt.Sprintf("read final directory state: %v", err))
	}
	// readyForActivation() is intentionally false after activation because the
	// contract includes !activated in that predicate. Activated + all modules
	// finalized + Directory.Active is the correct post-activation invariant.
	if !finalState.Activated || finalState.NextModule != suitemigration.RequiredModuleCount || directoryState != 1 {
		fatal(fmt.Sprintf("activation verification failed: next=%d ready=%v activated=%v directory_state=%d", finalState.NextModule, finalState.Ready, finalState.Activated, directoryState))
	}
	j.Status = "broadcasted_and_verified"
	writeJournal(evidencePath, j)
	fmt.Printf("demo bootstrap complete: directory=%s coordinator=%s\n", directory, coordinator)
}

func verifyTarget(ctx context.Context, rpc *chain.EVMRPC, dirABI, coordABI *abi.ABI, directory, coordinator string, root common.Hash) error {
	state, err := readUint8(ctx, rpc, dirABI, directory, "state")
	if err != nil {
		return fmt.Errorf("directory.state: %w", err)
	}
	if state != 0 && state != 1 {
		return fmt.Errorf("directory must be Bootstrapping or Active (state 0/1), got %d", state)
	}
	registered, err := readUint256(ctx, rpc, dirABI, directory, "registeredModuleCount")
	if err != nil || registered != suitemigration.RequiredModuleCount {
		return fmt.Errorf("directory registered module count must be 7, got %d (%v)", registered, err)
	}
	bound, err := readAddress(ctx, rpc, dirABI, directory, "bootstrapCoordinator")
	if err != nil || !strings.EqualFold(bound.Hex(), coordinator) {
		return fmt.Errorf("directory coordinator binding is %s, want %s (%v)", bound.Hex(), coordinator, err)
	}
	boundDir, err := readAddress(ctx, rpc, coordABI, coordinator, "suiteDirectory")
	if err != nil || !strings.EqualFold(boundDir.Hex(), directory) {
		return fmt.Errorf("coordinator directory binding is %s, want %s (%v)", boundDir.Hex(), directory, err)
	}
	boundRoot, err := readHash(ctx, rpc, coordABI, coordinator, "snapshotRoot")
	if err != nil || boundRoot != root {
		return fmt.Errorf("coordinator snapshot root is %s, want %s (%v)", boundRoot.Hex(), root.Hex(), err)
	}
	dirRoot, err := readHash(ctx, rpc, dirABI, directory, "snapshotRoot")
	if err != nil || dirRoot != root {
		return fmt.Errorf("directory snapshot root is %s, want %s (%v)", dirRoot.Hex(), root.Hex(), err)
	}
	if state == 1 {
		activated, activatedErr := readBool(ctx, rpc, coordABI, coordinator, "activated")
		if activatedErr != nil || !activated {
			return fmt.Errorf("Directory is Active but coordinator.activated is %v (%v)", activated, activatedErr)
		}
	}
	return nil
}

func transactionApplied(item suitemigration.ManifestTransaction, plan *suitemigration.Plan, state coordinatorState, ctx context.Context, rpc *chain.EVMRPC, coordABI *abi.ABI, coordinator string) (bool, error) {
	switch item.Phase {
	case "begin_module":
		module, err := manifestModule(item, plan)
		if err != nil {
			return false, err
		}
		if state.NextModule > uint64(module.Order) {
			return true, nil
		}
		if state.NextModule != uint64(module.Order) {
			return false, fmt.Errorf("next module index is %d, manifest expects %d", state.NextModule, module.Order)
		}
		p, err := readProgress(ctx, rpc, coordABI, coordinator, common.HexToHash(module.ID))
		if err != nil {
			return false, err
		}
		return p.Started, nil
	case "finalize_module":
		module, err := manifestModule(item, plan)
		if err != nil {
			return false, err
		}
		if state.NextModule > uint64(module.Order) {
			return true, nil
		}
		if state.NextModule != uint64(module.Order) {
			return false, fmt.Errorf("next module index is %d, manifest expects finalize %d", state.NextModule, module.Order)
		}
		p, err := readProgress(ctx, rpc, coordABI, coordinator, common.HexToHash(module.ID))
		if err != nil {
			return false, err
		}
		return p.Finalized, nil
	case "attest_username_escrow":
		expected := common.HexToHash(plan.UsernameEscrowEvidenceHash)
		if state.Evidence == (common.Hash{}) {
			return false, nil
		}
		if state.Evidence != expected {
			return false, fmt.Errorf("username evidence is %s, want %s", state.Evidence.Hex(), expected.Hex())
		}
		return true, nil
	case "activate_suite":
		return state.Activated, nil
	default:
		return false, fmt.Errorf("unsupported manifest phase %q", item.Phase)
	}
}

func waitUntilApplied(ctx context.Context, rpc *chain.EVMRPC, coordABI *abi.ABI, coordinator string, item suitemigration.ManifestTransaction, plan *suitemigration.Plan) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		state, err := readCoordinatorState(ctx, rpc, coordABI, coordinator)
		if err == nil {
			applied, checkErr := transactionApplied(item, plan, state, ctx, rpc, coordABI, coordinator)
			if checkErr == nil && applied {
				return nil
			}
		}
		time.Sleep(750 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s state application", item.Phase)
}

func manifestModule(item suitemigration.ManifestTransaction, plan *suitemigration.Plan) (*suitemigration.ModulePlan, error) {
	if item.ModuleOrder == nil || *item.ModuleOrder < 0 || *item.ModuleOrder >= len(plan.Modules) {
		return nil, fmt.Errorf("invalid module order")
	}
	module := &plan.Modules[*item.ModuleOrder]
	if !strings.EqualFold(module.ID, item.ModuleID) {
		return nil, fmt.Errorf("manifest module ID %s does not match plan %s", item.ModuleID, module.ID)
	}
	return module, nil
}

func readCoordinatorState(ctx context.Context, rpc *chain.EVMRPC, coordABI *abi.ABI, coordinator string) (coordinatorState, error) {
	next, err := readUint256(ctx, rpc, coordABI, coordinator, "nextModuleIndex")
	if err != nil {
		return coordinatorState{}, err
	}
	evidence, err := readHash(ctx, rpc, coordABI, coordinator, "usernameEscrowEvidenceHash")
	if err != nil {
		return coordinatorState{}, err
	}
	activated, err := readBool(ctx, rpc, coordABI, coordinator, "activated")
	if err != nil {
		return coordinatorState{}, err
	}
	ready, err := readBool(ctx, rpc, coordABI, coordinator, "readyForActivation")
	if err != nil {
		return coordinatorState{}, err
	}
	return coordinatorState{NextModule: next, Evidence: evidence, Activated: activated, Ready: ready}, nil
}

func readProgress(ctx context.Context, rpc *chain.EVMRPC, coordABI *abi.ABI, coordinator string, id common.Hash) (progress, error) {
	values, err := call(ctx, rpc, coordABI, coordinator, "moduleProgress", id)
	if err != nil {
		return progress{}, err
	}
	if len(values) != 8 {
		return progress{}, fmt.Errorf("moduleProgress returned %d values", len(values))
	}
	started, ok1 := values[0].(bool)
	finalized, ok2 := values[1].(bool)
	expectedCount, ok3 := uintValue(values[2])
	expectedBatch, ok4 := uintValue(values[3])
	importedCount, ok5 := uintValue(values[4])
	nextSequence, ok6 := uintValue(values[5])
	expectedRoot, ok7 := hashValue(values[6])
	rollingRoot, ok8 := hashValue(values[7])
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7 && ok8) {
		return progress{}, fmt.Errorf("moduleProgress returned unexpected ABI types")
	}
	return progress{Started: started, Finalized: finalized, ExpectedCount: expectedCount, ExpectedBatch: expectedBatch, ImportedCount: importedCount, NextSequence: nextSequence, ExpectedRoot: expectedRoot, RollingRoot: rollingRoot}, nil
}

func readUint256(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) (uint64, error) {
	values, err := call(ctx, rpc, contractABI, address, method, args...)
	if err != nil || len(values) != 1 {
		return 0, err
	}
	value, ok := uintValue(values[0])
	if !ok {
		return 0, fmt.Errorf("%s returned unexpected ABI type", method)
	}
	return value, nil
}

func readUint8(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) (uint8, error) {
	values, err := call(ctx, rpc, contractABI, address, method, args...)
	if err != nil || len(values) != 1 {
		return 0, err
	}
	if value, ok := values[0].(uint8); ok {
		return value, nil
	}
	if value, ok := uintValue(values[0]); ok {
		return uint8(value), nil
	}
	return 0, fmt.Errorf("%s returned unexpected ABI type", method)
}

func readBool(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) (bool, error) {
	values, err := call(ctx, rpc, contractABI, address, method, args...)
	if err != nil || len(values) != 1 {
		return false, err
	}
	value, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("%s returned unexpected ABI type", method)
	}
	return value, nil
}

func readAddress(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) (common.Address, error) {
	values, err := call(ctx, rpc, contractABI, address, method, args...)
	if err != nil || len(values) != 1 {
		return common.Address{}, err
	}
	value, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("%s returned unexpected ABI type", method)
	}
	return value, nil
}

func readHash(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) (common.Hash, error) {
	values, err := call(ctx, rpc, contractABI, address, method, args...)
	if err != nil || len(values) != 1 {
		return common.Hash{}, err
	}
	value, ok := hashValue(values[0])
	if !ok {
		return common.Hash{}, fmt.Errorf("%s returned unexpected ABI type", method)
	}
	return value, nil
}

func call(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) ([]any, error) {
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", method, err)
	}
	raw, err := rpc.CallContract(ctx, address, "0x"+hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}
	values, err := contractABI.Unpack(method, raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", method, err)
	}
	return values, nil
}

func uintValue(value any) (uint64, bool) {
	parsed, ok := value.(*big.Int)
	if !ok || parsed == nil || !parsed.IsUint64() {
		return 0, false
	}
	return parsed.Uint64(), true
}

func hashValue(value any) (common.Hash, bool) {
	switch typed := value.(type) {
	case common.Hash:
		return typed, true
	case [32]byte:
		return common.BytesToHash(typed[:]), true
	default:
		return common.Hash{}, false
	}
}

func decodeHex(value string) ([]byte, error) {
	value = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(value), "0x"), "0X")
	return hex.DecodeString(value)
}

func writeJournal(path string, value journal) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(fmt.Sprintf("encode journal: %v", err))
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fatal(fmt.Sprintf("create journal directory: %v", err))
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		fatal(fmt.Sprintf("write journal: %v", err))
	}
}

func loadABI(artifactsDir, name string) (*abi.ABI, error) {
	path := filepath.Join(artifactsDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	var art artifact
	if err := json.Unmarshal(data, &art); err != nil {
		return nil, fmt.Errorf("unmarshal artifact: %w", err)
	}
	parsed, err := abi.JSON(strings.NewReader(string(art.ABI)))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}
	return &parsed, nil
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

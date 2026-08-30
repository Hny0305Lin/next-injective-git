// Command evm-activate-suite finalizes all modules and activates an empty EVM
// suite without importing a snapshot. Use this for clean deployments that start
// from zero state.
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
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type artifact struct {
	ABI json.RawMessage `json:"abi"`
}

var moduleNames = []struct {
	name     string
	fullName string
}{
	{"core", "igit.module.repository-core"},
	{"recovery", "igit.module.recovery"},
	{"moderation", "igit.module.moderation"},
	{"economic", "igit.module.economic"},
	{"username", "igit.module.username"},
	{"badge", "igit.module.badge"},
	{"release", "igit.module.release"},
}

var snapshotRoot = common.HexToHash("0x5e2eaab50320b54d85e45bdc9f59ce94ada216d440fac3086fb30da7f98a1cea")

func main() {
	var coordinator, directory, keyName, rpcURL, artifacts, usernameEscrowEvidence string
	var chainID uint64
	var dryRun, skipFinalize bool
	flag.StringVar(&coordinator, "coordinator", "", "BootstrapCoordinator address")
	flag.StringVar(&directory, "directory", "", "SuiteDirectory address")
	flag.StringVar(&keyName, "key", "", "keystore key name (e.g., 'igit-dev')")
	flag.StringVar(&rpcURL, "rpc", "", "EVM JSON-RPC endpoint")
	flag.Uint64Var(&chainID, "chain-id", 1439, "EVM chain ID")
	flag.StringVar(&artifacts, "artifacts", "contracts/evm-v2/artifacts", "artifact directory")
	flag.StringVar(&usernameEscrowEvidence, "username-escrow-evidence-hash", "", "non-zero bytes32 evidence hash required before finalizing the username module")
	flag.BoolVar(&dryRun, "dry-run", false, "check readiness without sending transaction")
	flag.BoolVar(&skipFinalize, "skip-finalize", false, "skip finalize step (if already done)")
	flag.Parse()

	if strings.TrimSpace(coordinator) == "" {
		fatal("-coordinator is required")
	}
	if strings.TrimSpace(directory) == "" {
		fatal("-directory is required")
	}
	if !dryRun && strings.TrimSpace(keyName) == "" {
		fatal("-key is required (unless -dry-run)")
	}

	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Sprintf("load config: %v", err))
	}
	if rpcURL == "" {
		rpcURL = cfg.EffectiveEVMRPC()
	}
	if rpcURL == "" {
		fatal("no EVM RPC endpoint configured")
	}

	cfg.KeyName = keyName
	cfg.EVMChainID = chainID

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	rpc := chain.NewEVMRPC(rpcURL)

	coordABI, err := loadABI(artifacts, "BootstrapCoordinator")
	if err != nil {
		fatal(fmt.Sprintf("load BootstrapCoordinator ABI: %v", err))
	}

	// Check current state
	activatedData, err := coordABI.Pack("activated")
	if err != nil {
		fatal(fmt.Sprintf("pack activated: %v", err))
	}
	activatedResult, err := rpc.CallContract(ctx, coordinator, "0x"+hex.EncodeToString(activatedData))
	if err != nil {
		fatal(fmt.Sprintf("call activated: %v", err))
	}
	activatedValues, err := coordABI.Unpack("activated", activatedResult)
	if err != nil || len(activatedValues) != 1 {
		fatal(fmt.Sprintf("unpack activated: %v", err))
	}
	activated, ok := activatedValues[0].(bool)
	if !ok {
		fatal("activated has unexpected type")
	}
	fmt.Printf("coordinator.activated=%v\n", activated)
	if activated {
		fmt.Println("✅ Suite is already activated; nothing to do.")
		return
	}

	// Check readyForActivation
	readyData, err := coordABI.Pack("readyForActivation")
	if err != nil {
		fatal(fmt.Sprintf("pack readyForActivation: %v", err))
	}
	readyResult, err := rpc.CallContract(ctx, coordinator, "0x"+hex.EncodeToString(readyData))
	if err != nil {
		fatal(fmt.Sprintf("call readyForActivation: %v", err))
	}
	readyValues, err := coordABI.Unpack("readyForActivation", readyResult)
	if err != nil || len(readyValues) != 1 {
		fatal(fmt.Sprintf("unpack readyForActivation: %v", err))
	}
	ready, ok := readyValues[0].(bool)
	if !ok {
		fatal("readyForActivation has unexpected type")
	}
	fmt.Printf("coordinator.readyForActivation=%v\n", ready)

	nextModuleIndex, err := callUint64(ctx, rpc, coordABI, coordinator, "nextModuleIndex")
	if err != nil {
		fatal(fmt.Sprintf("call nextModuleIndex: %v", err))
	}
	if nextModuleIndex > uint64(len(moduleNames)) {
		fatal(fmt.Sprintf("coordinator nextModuleIndex=%d exceeds required module count", nextModuleIndex))
	}
	fmt.Printf("coordinator.nextModuleIndex=%d\n", nextModuleIndex)

	configuredEvidence, err := callHash(ctx, rpc, coordABI, coordinator, "usernameEscrowEvidenceHash")
	if err != nil {
		fatal(fmt.Sprintf("call usernameEscrowEvidenceHash: %v", err))
	}
	requestedEvidence, err := parseRequiredHash(usernameEscrowEvidence)
	if err != nil {
		fatal(err.Error())
	}
	if configuredEvidence != (common.Hash{}) && requestedEvidence != (common.Hash{}) && configuredEvidence != requestedEvidence {
		fatal(fmt.Sprintf("configured username escrow evidence %s differs from requested %s", configuredEvidence.Hex(), requestedEvidence.Hex()))
	}
	if !ready && nextModuleIndex <= 4 && configuredEvidence == (common.Hash{}) && requestedEvidence == (common.Hash{}) {
		fatal("-username-escrow-evidence-hash is required before processing an unfinalized username module")
	}

	if dryRun {
		if ready {
			fmt.Println("✅ Dry-run: suite is ready for activation. Re-run without -dry-run to activate.")
		} else {
			fmt.Println("⚠️  Dry-run: suite is NOT ready. Modules need to be finalized first.")
			fmt.Printf("    Resume will start at module index %d.\n", nextModuleIndex)
		}
		return
	}

	// Load operator key and create transactor
	signer := chain.NewEVMKeystoreSigner(cfg)
	operatorAddr, err := signer.OwnerAddress()
	if err != nil {
		fatal(fmt.Sprintf("get operator address: %v", err))
	}
	fmt.Printf("operator=%s\n", operatorAddr)
	operatorEVM, err := chain.NormalizeEVMAddress(operatorAddr)
	if err != nil {
		fatal(fmt.Sprintf("normalize operator address: %v", err))
	}
	configuredOperator, err := callAddress(ctx, rpc, coordABI, coordinator, "operator")
	if err != nil {
		fatal(fmt.Sprintf("call operator: %v", err))
	}
	if !strings.EqualFold(configuredOperator.Hex(), operatorEVM) {
		fatal(fmt.Sprintf("signer %s is not coordinator operator %s", operatorEVM, configuredOperator.Hex()))
	}

	transactor := chain.NewEVMTransactor(cfg, rpc, signer)
	transactor.SetReceiptTimeout(20 * time.Second)

	// Step 1: Finalize all modules if not ready
	if !ready && !skipFinalize {
		fmt.Println("\n=== Step 1: Finalizing all modules ===")
		fmt.Printf("Snapshot root: %s\n", snapshotRoot.Hex())

		for i := int(nextModuleIndex); i < len(moduleNames); i++ {
			mod := moduleNames[i]
			fmt.Printf("\n[%d/%d] Processing %s module...\n", i+1, len(moduleNames), mod.name)

			// Compute module ID
			moduleID := computeModuleID(mod.fullName)
			fmt.Printf("  Module ID: %s\n", hex.EncodeToString(moduleID[:]))

			// Compute expected root for this module
			expectedRoot := computeExpectedRoot(moduleID, snapshotRoot)
			fmt.Printf("  Expected root: %s\n", expectedRoot.Hex())
			started, finalized, progressRoot, err := moduleProgress(ctx, rpc, coordABI, coordinator, moduleID)
			if err != nil {
				fatal(fmt.Sprintf("call moduleProgress(%s): %v", mod.name, err))
			}
			if finalized {
				fatal(fmt.Sprintf("module %s is finalized but coordinator nextModuleIndex still points to it", mod.name))
			}
			if started && progressRoot != expectedRoot {
				fatal(fmt.Sprintf("module %s started with expected root %s, want %s", mod.name, progressRoot.Hex(), expectedRoot.Hex()))
			}

			// Step 1.1: Begin next module with expected root
			// For zero-state import: expectedCount=0, expectedBatches=0
			if !started {
				beginCalldata, err := coordABI.Pack("beginNextModule", moduleID, big.NewInt(0), big.NewInt(0), expectedRoot)
				if err != nil {
					fatal(fmt.Sprintf("pack beginNextModule(%s): %v", mod.name, err))
				}

				fmt.Printf("  Sending beginNextModule...\n")
				beginResult, err := sendAndConfirmState(ctx, transactor, coordinator, beginCalldata, func() (bool, error) {
					progressStarted, _, observedRoot, progressErr := moduleProgress(ctx, rpc, coordABI, coordinator, moduleID)
					return progressStarted && observedRoot == expectedRoot, progressErr
				})
				if err != nil {
					fatal(fmt.Sprintf("send beginNextModule(%s) transaction: %v", mod.name, err))
				}
				printTransactionConfirmation("Begin", beginResult)
			} else {
				fmt.Println("  Resuming already-started module")
			}

			if mod.name == "username" && configuredEvidence == (common.Hash{}) {
				attestCalldata, packErr := coordABI.Pack("attestUsernameEscrowReleased", requestedEvidence)
				if packErr != nil {
					fatal(fmt.Sprintf("pack attestUsernameEscrowReleased: %v", packErr))
				}
				fmt.Println("  Sending attestUsernameEscrowReleased...")
				attestResult, sendErr := sendAndConfirmState(ctx, transactor, coordinator, attestCalldata, func() (bool, error) {
					observedEvidence, evidenceErr := callHash(ctx, rpc, coordABI, coordinator, "usernameEscrowEvidenceHash")
					return observedEvidence == requestedEvidence, evidenceErr
				})
				if sendErr != nil {
					fatal(fmt.Sprintf("send attestUsernameEscrowReleased transaction: %v", sendErr))
				}
				configuredEvidence = requestedEvidence
				printTransactionConfirmation("Username escrow attestation", attestResult)
			}

			// Step 1.2: Finalize module immediately
			finalizeCalldata, err := coordABI.Pack("finalizeCurrentModule", moduleID)
			if err != nil {
				fatal(fmt.Sprintf("pack finalizeCurrentModule(%s): %v", mod.name, err))
			}

			fmt.Printf("  Sending finalizeCurrentModule...\n")
			finalizeResult, err := sendAndConfirmState(ctx, transactor, coordinator, finalizeCalldata, func() (bool, error) {
				observedIndex, indexErr := callUint64(ctx, rpc, coordABI, coordinator, "nextModuleIndex")
				return observedIndex > uint64(i), indexErr
			})
			if err != nil {
				fatal(fmt.Sprintf("send finalizeCurrentModule(%s) transaction: %v", mod.name, err))
			}
			printTransactionConfirmation(fmt.Sprintf("%s finalize", mod.name), finalizeResult)
		}

		fmt.Println("\n✅ All modules finalized!")
	}
	if !ready {
		ready, err = callBool(ctx, rpc, coordABI, coordinator, "readyForActivation")
		if err != nil {
			fatal(fmt.Sprintf("recheck readyForActivation: %v", err))
		}
	}
	if !ready {
		fatal("suite is not ready for activation after module processing")
	}

	// Step 2: Activate suite
	fmt.Println("\n=== Step 2: Activating suite ===")

	calldata, err := coordABI.Pack("activateSuite")
	if err != nil {
		fatal(fmt.Sprintf("pack activateSuite: %v", err))
	}

	result, err := sendAndConfirmState(ctx, transactor, coordinator, calldata, func() (bool, error) {
		return callBool(ctx, rpc, coordABI, coordinator, "activated")
	})
	if err != nil {
		fatal(fmt.Sprintf("send activateSuite transaction: %v", err))
	}
	printTransactionConfirmation("Suite activation", result)
	fmt.Printf("\n🎉 Suite activated successfully!\n")
	fmt.Printf("\nYou can now configure igit CLI:\n")
	fmt.Printf("  igit config set evm.suite_directory %s\n", directory)
}

func computeModuleID(name string) [32]byte {
	hash := crypto.Keccak256Hash([]byte(name))
	return hash
}

func computeExpectedRoot(moduleID common.Hash, snapshotRoot common.Hash) common.Hash {
	encoded := append(append([]byte{}, moduleID[:]...), snapshotRoot[:]...)
	return crypto.Keccak256Hash(encoded)
}

func parseRequiredHash(value string) (common.Hash, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return common.Hash{}, nil
	}
	raw := strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if len(raw) != 64 {
		return common.Hash{}, fmt.Errorf("invalid -username-escrow-evidence-hash %q", value)
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid -username-escrow-evidence-hash %q", value)
	}
	hash := common.BytesToHash(decoded)
	if hash == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("-username-escrow-evidence-hash must be non-zero")
	}
	return hash, nil
}

func callValues(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string, args ...any) ([]any, error) {
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	result, err := rpc.CallContract(ctx, address, "0x"+hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}
	return contractABI.Unpack(method, result)
}

func callBool(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string) (bool, error) {
	values, err := callValues(ctx, rpc, contractABI, address, method)
	if err != nil {
		return false, err
	}
	if len(values) != 1 {
		return false, fmt.Errorf("%s returned %d values", method, len(values))
	}
	value, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("%s returned unexpected type", method)
	}
	return value, nil
}

func callUint64(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string) (uint64, error) {
	values, err := callValues(ctx, rpc, contractABI, address, method)
	if err != nil {
		return 0, err
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("%s returned %d values", method, len(values))
	}
	value, ok := values[0].(*big.Int)
	if !ok || !value.IsUint64() {
		return 0, fmt.Errorf("%s returned unexpected value", method)
	}
	return value.Uint64(), nil
}

func callHash(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string) (common.Hash, error) {
	values, err := callValues(ctx, rpc, contractABI, address, method)
	if err != nil {
		return common.Hash{}, err
	}
	if len(values) != 1 {
		return common.Hash{}, fmt.Errorf("%s returned %d values", method, len(values))
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return common.Hash{}, fmt.Errorf("%s returned unexpected type", method)
	}
	return common.Hash(value), nil
}

func callAddress(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address, method string) (common.Address, error) {
	values, err := callValues(ctx, rpc, contractABI, address, method)
	if err != nil {
		return common.Address{}, err
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("%s returned %d values", method, len(values))
	}
	value, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("%s returned unexpected type", method)
	}
	return value, nil
}

func moduleProgress(ctx context.Context, rpc *chain.EVMRPC, contractABI *abi.ABI, address string, moduleID common.Hash) (bool, bool, common.Hash, error) {
	values, err := callValues(ctx, rpc, contractABI, address, "moduleProgress", moduleID)
	if err != nil {
		return false, false, common.Hash{}, err
	}
	if len(values) != 8 {
		return false, false, common.Hash{}, fmt.Errorf("moduleProgress returned %d values", len(values))
	}
	started, startedOK := values[0].(bool)
	finalized, finalizedOK := values[1].(bool)
	expectedRoot, rootOK := values[6].([32]byte)
	if !startedOK || !finalizedOK || !rootOK {
		return false, false, common.Hash{}, fmt.Errorf("moduleProgress returned unexpected types")
	}
	return started, finalized, common.Hash(expectedRoot), nil
}

func sendAndConfirmState(
	ctx context.Context,
	transactor *chain.EVMTransactor,
	target string,
	calldata []byte,
	verify func() (bool, error),
) (*chain.EVMTransactionResult, error) {
	result, err := transactor.Send(ctx, target, calldata, "0x0")
	if err == nil {
		return result, nil
	}
	var uncertain *chain.EVMReceiptUnconfirmedError
	if !errors.As(err, &uncertain) || result == nil {
		return result, err
	}
	fmt.Printf("  Receipt unavailable for %s; confirming contract state...\n", result.Hash)
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastVerifyErr error
	for {
		confirmed, verifyErr := verify()
		if verifyErr == nil && confirmed {
			return result, nil
		}
		if verifyErr != nil {
			lastVerifyErr = verifyErr
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("%w; state confirmation stopped: %v", err, ctx.Err())
		case <-deadline.C:
			if lastVerifyErr != nil {
				return result, fmt.Errorf("%w; state confirmation failed: %v", err, lastVerifyErr)
			}
			return result, err
		case <-ticker.C:
		}
	}
}

func printTransactionConfirmation(label string, result *chain.EVMTransactionResult) {
	fmt.Printf("  %s tx: %s\n", label, result.Hash)
	if result.Receipt != nil {
		fmt.Printf("  ✅ %s confirmed (block %s, gas %s)\n", label, result.Receipt.BlockNumber, result.Receipt.GasUsed)
		return
	}
	fmt.Printf("  ✅ %s confirmed from contract state\n", label)
}

func loadABI(artifactDir, contractName string) (*abi.ABI, error) {
	path := filepath.Join(artifactDir, contractName+".json")
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

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "fatal: %s\n", msg)
	os.Exit(1)
}

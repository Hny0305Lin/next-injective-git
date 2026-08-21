// Command evm-activate-suite finalizes all modules and activates an empty EVM
// suite without importing a snapshot. Use this for clean deployments that start
// from zero state.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
	var coordinator, directory, keyName, rpcURL, artifacts string
	var chainID uint64
	var dryRun, skipFinalize bool
	flag.StringVar(&coordinator, "coordinator", "", "BootstrapCoordinator address")
	flag.StringVar(&directory, "directory", "", "SuiteDirectory address")
	flag.StringVar(&keyName, "key", "", "keystore key name (e.g., 'igit-dev')")
	flag.StringVar(&rpcURL, "rpc", "", "EVM JSON-RPC endpoint")
	flag.Uint64Var(&chainID, "chain-id", 1439, "EVM chain ID")
	flag.StringVar(&artifacts, "artifacts", "contracts/evm-v2/artifacts", "artifact directory")
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

	if dryRun {
		if ready {
			fmt.Println("✅ Dry-run: suite is ready for activation. Re-run without -dry-run to activate.")
		} else {
			fmt.Println("⚠️  Dry-run: suite is NOT ready. Modules need to be finalized first.")
			fmt.Println("    Re-run without -dry-run to finalize and activate.")
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

	transactor := chain.NewEVMTransactor(cfg, rpc, signer)
	transactor.SetReceiptTimeout(5 * time.Minute) // Increase timeout for testnet

	// Step 1: Finalize all modules if not ready
	if !ready && !skipFinalize {
		fmt.Println("\n=== Step 1: Finalizing all modules ===")
		fmt.Printf("Snapshot root: %s\n", snapshotRoot.Hex())

		for i, mod := range moduleNames {
			fmt.Printf("\n[%d/%d] Processing %s module...\n", i+1, len(moduleNames), mod.name)

			// Compute module ID
			moduleID := computeModuleID(mod.fullName)
			fmt.Printf("  Module ID: %s\n", hex.EncodeToString(moduleID[:]))

			// Compute expected root for this module
			expectedRoot := computeExpectedRoot(moduleID, snapshotRoot, i)
			fmt.Printf("  Expected root: %s\n", expectedRoot.Hex())

			// Step 1.1: Begin next module with expected root
			// For zero-state import: expectedCount=0, expectedBatches=0
			beginCalldata, err := coordABI.Pack("beginNextModule", moduleID, big.NewInt(0), big.NewInt(0), expectedRoot)
			if err != nil {
				fatal(fmt.Sprintf("pack beginNextModule(%s): %v", mod.name, err))
			}

			fmt.Printf("  Sending beginNextModule...\n")
			beginResult, err := transactor.Send(ctx, coordinator, beginCalldata, "0x0")
			if err != nil {
				fatal(fmt.Sprintf("send beginNextModule(%s) transaction: %v", mod.name, err))
			}
			fmt.Printf("  Begin tx: %s\n", beginResult.Hash)

			if beginResult.Receipt == nil {
				fatal(fmt.Sprintf("beginNextModule(%s) receipt is nil", mod.name))
			}
			if beginResult.Receipt.Status != "0x1" {
				fatal(fmt.Sprintf("beginNextModule(%s) reverted: %s", mod.name, beginResult.Hash))
			}
			fmt.Printf("  ✅ Begin successful (block %s)\n", beginResult.Receipt.BlockNumber)

			// Step 1.2: Finalize module immediately
			finalizeCalldata, err := coordABI.Pack("finalizeCurrentModule", moduleID)
			if err != nil {
				fatal(fmt.Sprintf("pack finalizeCurrentModule(%s): %v", mod.name, err))
			}

			fmt.Printf("  Sending finalizeCurrentModule...\n")
			finalizeResult, err := transactor.Send(ctx, coordinator, finalizeCalldata, "0x0")
			if err != nil {
				fatal(fmt.Sprintf("send finalizeCurrentModule(%s) transaction: %v", mod.name, err))
			}
			fmt.Printf("  Finalize tx: %s\n", finalizeResult.Hash)

			if finalizeResult.Receipt == nil {
				fatal(fmt.Sprintf("finalizeCurrentModule(%s) receipt is nil", mod.name))
			}
			if finalizeResult.Receipt.Status != "0x1" {
				fatal(fmt.Sprintf("finalizeCurrentModule(%s) reverted: %s", mod.name, finalizeResult.Hash))
			}
			fmt.Printf("  ✅ %s finalized (block %s, gas %s)\n", mod.name, finalizeResult.Receipt.BlockNumber, finalizeResult.Receipt.GasUsed)
		}

		fmt.Println("\n✅ All modules finalized!")
	}

	// Step 2: Activate suite
	fmt.Println("\n=== Step 2: Activating suite ===")

	calldata, err := coordABI.Pack("activateSuite")
	if err != nil {
		fatal(fmt.Sprintf("pack activateSuite: %v", err))
	}

	result, err := transactor.Send(ctx, coordinator, calldata, "0x0")
	if err != nil {
		fatal(fmt.Sprintf("send activateSuite transaction: %v", err))
	}
	fmt.Printf("Transaction sent: %s\n", result.Hash)
	fmt.Printf("Waiting for receipt...\n")

	if result.Receipt == nil {
		fatal("activateSuite receipt is nil")
	}
	if result.Receipt.Status != "0x1" {
		fatal(fmt.Sprintf("transaction reverted: %s", result.Hash))
	}
	fmt.Printf("\n🎉 Suite activated successfully!\n")
	fmt.Printf("Block: %s\n", result.Receipt.BlockNumber)
	fmt.Printf("Gas used: %s\n", result.Receipt.GasUsed)
	fmt.Printf("\nYou can now configure igit CLI:\n")
	fmt.Printf("  igit config set evm.suite_directory %s\n", directory)
}

func computeModuleID(name string) [32]byte {
	hash := crypto.Keccak256Hash([]byte(name))
	return hash
}

func computeExpectedRoot(moduleID common.Hash, snapshotRoot common.Hash, order int) common.Hash {
	// For zero-state import, expectedRoot is computed as a rolling hash
	// First module: keccak256(abi.encode(moduleID, snapshotRoot))
	// Subsequent: keccak256(abi.encode(moduleID, previousExpectedRoot))

	var prevRoot common.Hash
	if order == 0 {
		prevRoot = snapshotRoot
	} else {
		// Recompute all previous expected roots
		prevRoot = snapshotRoot
		for i := 0; i < order; i++ {
			prevModuleID := computeModuleID(moduleNames[i].fullName)
			encoded := append(prevModuleID[:], prevRoot[:]...)
			prevRoot = crypto.Keccak256Hash(encoded)
		}
	}

	encoded := append(moduleID[:], prevRoot[:]...)
	return crypto.Keccak256Hash(encoded)
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

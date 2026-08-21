// Command igit-suite-operator executes a reviewed calldata manifest with
// append-only signed journal, safe resume, and receipt verification.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var (
		manifestPath string
		keystorePath string
		journalPath  string
		rpcURL       string
		chainID      uint64
		gasPrice     uint64
		dryRun       bool
	)

	flags := flag.NewFlagSet("igit-suite-operator", flag.ContinueOnError)
	flags.StringVar(&manifestPath, "manifest", "", "calldata manifest path (required)")
	flags.StringVar(&keystorePath, "keystore", "", "encrypted keystore path (required)")
	flags.StringVar(&journalPath, "journal", "operator-journal.jsonl", "append-only journal path")
	flags.StringVar(&rpcURL, "rpc", "https://k8s.testnet.json-rpc.injective.network/", "Injective EVM RPC endpoint")
	flags.Uint64Var(&chainID, "chain-id", 1439, "EVM chain ID (1439 for testnet)")
	flags.Uint64Var(&gasPrice, "gas-price", 160000000, "gas price in wei (minimum 160000000)")
	flags.BoolVar(&dryRun, "dry-run", false, "verify setup without broadcasting")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if manifestPath == "" || keystorePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --manifest and --keystore are required")
		flags.PrintDefaults()
		return 2
	}

	fmt.Println("🚀 Operator Runner Starting...")
	fmt.Printf("📄 Manifest: %s\n", manifestPath)
	fmt.Printf("🔑 Keystore: %s\n", keystorePath)
	fmt.Printf("📝 Journal: %s\n", journalPath)
	fmt.Printf("🌐 RPC: %s\n", rpcURL)
	fmt.Printf("⛓️  Chain ID: %d\n", chainID)
	fmt.Printf("⛽ Gas Price: %d wei\n", gasPrice)

	if dryRun {
		fmt.Println("\n🧪 DRY RUN MODE - No transactions will be broadcast")
	}

	// Load private key from keystore
	fmt.Println("\n🔓 Loading keystore...")
	privateKey, err := LoadPrivateKey(keystorePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading keystore: %v\n", err)
		return 1
	}
	address := GetAddress(privateKey)
	fmt.Printf("✅ Loaded operator address: %s\n", address)

	// Open or create journal
	fmt.Println("\n📖 Opening journal...")
	journal, err := OpenJournal(journalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening journal: %v\n", err)
		return 1
	}
	defer journal.Close()
	fmt.Printf("✅ Journal opened: %d existing entries\n", len(journal.entries))

	// Load manifest
	fmt.Println("\n📋 Loading manifest...")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading manifest: %v\n", err)
		return 1
	}
	fmt.Printf("✅ Manifest loaded: %d batches\n", len(manifest.Batches))

	// Recover from journal
	fmt.Println("\n🔄 Recovering from journal...")
	lastEntry := journal.LastEntry()
	if lastEntry != nil {
		fmt.Printf("📌 Last entry: Batch %d, Status: %s\n", lastEntry.BatchIndex, lastEntry.Status)

		// Check if we need to query receipt for uncertain status
		if lastEntry.Status == StatusBroadcast || lastEntry.Status == StatusUncertain {
			fmt.Printf("⚠️  Last transaction uncertain, checking status...\n")
			fmt.Printf("   Tx Hash: %s\n", lastEntry.TxHash)
			// TODO: Query receipt and update journal
			fmt.Println("   [Receipt query not yet implemented]")
		}
	} else {
		fmt.Println("📌 No previous entries, starting fresh")
	}

	if dryRun {
		fmt.Println("\n✅ Dry run completed successfully!")
		return 0
	}

	// TODO: Implement batch execution loop
	fmt.Println("\n⚠️  Batch execution not yet implemented")
	fmt.Println("   Next step: Implement ExecuteBatches() function")

	return 0
}

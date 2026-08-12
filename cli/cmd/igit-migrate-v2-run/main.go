// Command igit-migrate-v2-run is the explicit, resumable administrator path
// for broadcasting a reviewed V1-to-V2 transaction manifest. Normal igit users
// never invoke it. The exact plan SHA-256 must be supplied as confirmation.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

type commandServices struct {
	readPlan     func(string) (*migration.Plan, error)
	readManifest func(string) (*migration.TransactionManifest, error)
	loadConfig   func() (config.Config, error)
	inspect      func(string, *migration.Plan, *migration.TransactionManifest) (*migration.ReceiptJournalStatus, error)
	execute      func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithServices(ctx, args, stdout, stderr, commandServices{
		readPlan:     migration.ReadPlan,
		readManifest: migration.ReadTransactionManifest,
		loadConfig:   config.Load,
		inspect:      migration.InspectReceiptJournal,
		execute:      executeWithEVMDependencies,
	})
}

func runWithServices(ctx context.Context, args []string, stdout, stderr io.Writer, services commandServices) int {
	var planPath string
	var manifestPath string
	var journalDirectory string
	var network string
	var rpcOverride string
	var keyName string
	var confirmation string
	var acknowledgeCoreOnly bool
	var receiptTimeout time.Duration
	var maxGasLimit uint64
	var checkOnly bool
	var statusOnly bool
	flags := flag.NewFlagSet("igit-migrate-v2-run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&planPath, "plan", "", "validated V1-to-V2 import plan JSON")
	flags.StringVar(&manifestPath, "manifest", "", "canonical unsigned transaction manifest JSON")
	flags.StringVar(&journalDirectory, "journal", "", "dedicated append-only receipt journal directory")
	flags.StringVar(&network, "network", "injective-testnet", "named Injective network profile")
	flags.StringVar(&rpcOverride, "rpc", "", "operator-only EVM JSON-RPC override")
	flags.StringVar(&keyName, "key", "", "encrypted EVM administrator key name (defaults to igit config)")
	flags.StringVar(&confirmation, "confirm-plan-sha256", "", "exact lowercase plan SHA-256 required before broadcast")
	flags.BoolVar(&acknowledgeCoreOnly, "acknowledge-core-only-import", false, "confirm that deferred V1 extension sections remain unmigrated")
	flags.DurationVar(&receiptTimeout, "receipt-timeout", migration.DefaultImportReceiptTimeout, "maximum wait for each transaction receipt")
	flags.Uint64Var(&maxGasLimit, "max-gas", migration.DefaultImportMaxGasLimit, "hard gas limit ceiling for each import transaction")
	flags.BoolVar(&checkOnly, "check", false, "validate and summarize artifacts without loading a key or contacting RPC")
	flags.BoolVar(&statusOnly, "status", false, "validate and summarize an existing journal without loading a key or contacting RPC")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	if checkOnly && statusOnly {
		fmt.Fprintln(stderr, "--check and --status are mutually exclusive")
		return 2
	}
	if strings.TrimSpace(planPath) == "" || strings.TrimSpace(manifestPath) == "" || (!checkOnly && strings.TrimSpace(journalDirectory) == "") {
		fmt.Fprintln(stderr, "--plan and --manifest are required; execution also requires --journal")
		return 2
	}
	if !checkOnly && !statusOnly && (receiptTimeout <= 0 || maxGasLimit == 0) {
		fmt.Fprintln(stderr, "--receipt-timeout and --max-gas must be positive")
		return 2
	}

	plan, err := services.readPlan(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read V2 import plan: %v\n", err)
		return 1
	}
	if plan == nil {
		fmt.Fprintln(stderr, "read V2 import plan: decoded plan is nil")
		return 1
	}
	manifest, err := services.readManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "read V2 transaction manifest: %v\n", err)
		return 1
	}
	if err := migration.VerifyTransactionManifest(plan, manifest); err != nil {
		fmt.Fprintf(stderr, "verify V2 transaction manifest: %v\n", err)
		return 1
	}
	planDigest, err := migration.PlanSHA256(plan)
	if err != nil {
		fmt.Fprintf(stderr, "hash V2 import plan: %v\n", err)
		return 1
	}
	manifestDigest, err := migration.TransactionManifestSHA256(manifest)
	if err != nil {
		fmt.Fprintf(stderr, "hash V2 transaction manifest: %v\n", err)
		return 1
	}
	if checkOnly {
		fmt.Fprintf(
			stdout,
			"plan sha256: %s\nmanifest sha256: %s\nimport scope: %s\ndeferred sections: %s\ntarget chain id: %d\nregistry: %s\ncontroller: %s\ntransactions: %d\ncalldata ready: true\nsigned: false\nbroadcast: false\n",
			planDigest,
			manifestDigest,
			plan.ImportScope,
			strings.Join(plan.DeferredSections, ","),
			plan.Target.ChainID,
			plan.Target.Contract,
			plan.Target.Controller,
			len(manifest.Transactions),
		)
		return 0
	}
	if statusOnly {
		status, err := services.inspect(journalDirectory, plan, manifest)
		if err != nil {
			fmt.Fprintf(stderr, "inspect V2 import receipt journal: %v\n", err)
			return 1
		}
		if status == nil {
			fmt.Fprintln(stderr, "inspect V2 import receipt journal: inspector returned nil status")
			return 1
		}
		fmt.Fprintf(
			stdout,
			"receipt journal: %s\noffline status: true\nchain evidence revalidated: false\nplan sha256: %s\nmanifest sha256: %s\nimport scope: %s\ndeferred sections: %s\nsigner (journal metadata): %s\ntransactions: %d\nprepared: %d\nbroadcast: %d\nmined: %d\nreverted: %d\nfailed order: %d\nreverted receipt: %s\nnext order: %d\ncomplete: %t\nfinalize transaction: %s\n",
			journalDirectory,
			status.Header.PlanSHA256,
			status.Header.ManifestSHA256,
			status.Header.ImportScope,
			strings.Join(status.Header.DeferredSections, ","),
			status.Header.Signer,
			status.Header.TransactionCount,
			status.PreparedTransactions,
			status.BroadcastTransactions,
			status.MinedTransactions,
			status.RevertedTransactions,
			status.FailedOrder,
			status.RevertedTransactionHash,
			status.NextOrder,
			status.Complete,
			status.FinalizeTransactionHash,
		)
		return 0
	}
	if confirmation != planDigest {
		fmt.Fprintf(
			stderr,
			"broadcast confirmation is missing or does not match; review the artifacts, then pass --confirm-plan-sha256 %s\n",
			planDigest,
		)
		return 2
	}
	if len(plan.DeferredSections) != 0 && !acknowledgeCoreOnly {
		fmt.Fprintf(
			stderr,
			"core-only import requires --acknowledge-core-only-import; deferred sections: %s\n",
			strings.Join(plan.DeferredSections, ","),
		)
		return 2
	}

	profile, ok := config.NetworkProfileFor(network)
	if !ok {
		fmt.Fprintf(stderr, "unknown network profile %q\n", strings.TrimSpace(network))
		return 2
	}
	if profile.EVMChainID != plan.Target.ChainID {
		fmt.Fprintf(
			stderr,
			"network profile %s uses EVM chain ID %d, but the plan targets %d\n",
			profile.Name,
			profile.EVMChainID,
			plan.Target.ChainID,
		)
		return 1
	}
	endpoint := strings.TrimSpace(rpcOverride)
	if endpoint == "" {
		endpoint = profile.EVMRPC
	}
	endpoint, err = validateRPCEndpoint(endpoint)
	if err != nil {
		fmt.Fprintf(stderr, "validate EVM RPC endpoint: %v\n", err)
		return 2
	}
	cfg, err := services.loadConfig()
	if err != nil {
		fmt.Fprintf(stderr, "load igit keystore configuration: %v\n", err)
		return 1
	}
	if strings.TrimSpace(keyName) != "" {
		cfg.KeyName = strings.TrimSpace(keyName)
	}
	if strings.TrimSpace(cfg.KeyName) == "" {
		fmt.Fprintln(stderr, "an encrypted EVM administrator key is required; pass --key or configure key_name")
		return 2
	}
	cfg.Network = profile.Name
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMRPC = endpoint
	cfg.EVMChainID = plan.Target.ChainID
	cfg.EVMContractAddress = plan.Target.Contract
	cfg.ContractAddress = ""

	result, err := services.execute(ctx, plan, manifest, journalDirectory, cfg, migration.ImportExecutionOptions{
		ReceiptTimeout: receiptTimeout,
		MaxGasLimit:    maxGasLimit,
	})
	if err != nil {
		fmt.Fprintf(stderr, "execute V2 import: %v\n", err)
		fmt.Fprintf(stderr, "inspect the receipt journal path before deterministic resume: %s\n", journalDirectory)
		return 1
	}
	if result == nil {
		fmt.Fprintln(stderr, "execute V2 import: runner returned nil result")
		return 1
	}
	if result.TransactionCount != len(manifest.Transactions) || !isCanonicalHash32(result.FinalizeTransactionHash) {
		fmt.Fprintln(stderr, "execute V2 import: runner returned incomplete finalization evidence")
		return 1
	}
	fmt.Fprintf(
		stdout,
		"receipt journal: %s\nnetwork: %s\ntransactions: %d\nprepared this run: %d\nbroadcast records this run: %d\nmined this run: %d\nrevalidated mined: %d\nfinalize transaction: %s\n",
		journalDirectory,
		profile.Name,
		result.TransactionCount,
		result.PreparedThisRun,
		result.BroadcastRecordedThisRun,
		result.MinedThisRun,
		result.RevalidatedMined,
		result.FinalizeTransactionHash,
	)
	return 0
}

func isCanonicalHash32(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil || len(decoded) != 32 {
		return false
	}
	for _, byteValue := range decoded {
		if byteValue != 0 {
			return true
		}
	}
	return false
}

func executeWithEVMDependencies(
	ctx context.Context,
	plan *migration.Plan,
	manifest *migration.TransactionManifest,
	journalDirectory string,
	cfg config.Config,
	options migration.ImportExecutionOptions,
) (*migration.ImportExecutionResult, error) {
	rpc := chain.NewEVMRPC(cfg.EffectiveEVMRPC())
	signer := chain.NewEVMKeystoreSigner(cfg)
	return migration.ExecuteImport(ctx, plan, manifest, journalDirectory, rpc, signer, options)
}

func validateRPCEndpoint(raw string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if endpoint == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("endpoint must be an absolute http(s) URL")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("endpoint must not contain a URL fragment")
	}
	return endpoint, nil
}

// Command igit-deploy-suite is the explicit administrator path for deploying
// the immutable EVM v3 suite. It never imports a snapshot or activates a
// directory; those operations require separately reviewed migration evidence.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitedeploy"
)

type commandServices struct {
	loadConfig func() (config.Config, error)
	inspect    func(string) (*suitedeploy.ArtifactInspection, error)
	verifyGit  func(string, string) error
	deploy     func(context.Context, suitedeploy.Options, *chain.EVMRPC, *chain.EVMTransactor, string) (*suitedeploy.Manifest, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithServices(ctx, args, stdout, stderr, commandServices{
		loadConfig: config.Load,
		inspect:    suitedeploy.InspectArtifacts,
		verifyGit:  verifyCheckedInSuiteSources,
		deploy: func(ctx context.Context, options suitedeploy.Options, rpc *chain.EVMRPC, transactor *chain.EVMTransactor, output string) (*suitedeploy.Manifest, error) {
			return suitedeploy.Deploy(ctx, options, rpc, transactor, output)
		},
	})
}

func runWithServices(ctx context.Context, args []string, stdout, stderr io.Writer, services commandServices) int {
	var artifacts string
	var output string
	var network string
	var rpcOverride string
	var keyName string
	var sourceCommit string
	var confirmation string
	var snapshotRoot string
	var platformFeeBPS uint
	var checkOnly bool
	flags := flag.NewFlagSet("igit-deploy-suite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&artifacts, "artifacts", "", "checked-in solc 0.8.24 artifact directory (required)")
	flags.StringVar(&output, "output", "", "exclusive deployment.json evidence path")
	flags.StringVar(&network, "network", "injective-testnet", "named Injective network profile")
	flags.StringVar(&rpcOverride, "rpc", "", "operator-only EVM JSON-RPC override")
	flags.StringVar(&keyName, "key", "", "rotated encrypted EVM bootstrap key name (defaults to config)")
	flags.StringVar(&sourceCommit, "source-commit", "", "reviewed source Git commit recorded in evidence")
	flags.StringVar(&confirmation, "confirm-source-commit", "", "exact source commit required before any broadcast")
	flags.StringVar(&snapshotRoot, "snapshot-root", "", "reviewed 0x-prefixed snapshot root")
	flags.UintVar(&platformFeeBPS, "platform-fee-bps", 300, "initial platform fee in basis points (maximum 500)")
	flags.BoolVar(&checkOnly, "check", false, "validate and hash checked-in artifacts without config, key, or RPC")
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
	if strings.TrimSpace(artifacts) == "" {
		fmt.Fprintln(stderr, "--artifacts is required and must resolve to the repository contracts/evm-v2/artifacts directory")
		return 2
	}
	inspection, err := services.inspect(artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "inspect checked-in suite artifacts: %v\n", err)
		return 1
	}
	if checkOnly {
		data, err := json.MarshalIndent(inspection, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "encode artifact inspection: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	sourceCommit = strings.TrimSpace(sourceCommit)
	if strings.TrimSpace(output) == "" || sourceCommit == "" || strings.TrimSpace(snapshotRoot) == "" {
		fmt.Fprintln(stderr, "deployment requires --output, --source-commit, and --snapshot-root")
		return 2
	}
	if confirmation != sourceCommit {
		fmt.Fprintf(stderr, "broadcast confirmation is missing or does not match; review artifacts, then pass --confirm-source-commit %s\n", sourceCommit)
		return 2
	}
	if platformFeeBPS > 500 {
		fmt.Fprintln(stderr, "--platform-fee-bps cannot exceed 500")
		return 2
	}
	profile, ok := config.NetworkProfileFor(network)
	if !ok {
		fmt.Fprintf(stderr, "unknown network profile %q\n", strings.TrimSpace(network))
		return 2
	}
	if profile.Name != "injective-testnet" {
		fmt.Fprintln(stderr, "single-EOA suite deployment is restricted to injective-testnet; production requires multisig/timelock governance")
		return 2
	}
	if services.verifyGit == nil {
		fmt.Fprintln(stderr, "checked-in source verifier is unavailable")
		return 1
	}
	if err := services.verifyGit(artifacts, sourceCommit); err != nil {
		fmt.Fprintf(stderr, "verify checked-in suite source: %v\n", err)
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
		fmt.Fprintf(stderr, "load encrypted EVM key configuration: %v\n", err)
		return 1
	}
	if strings.TrimSpace(keyName) != "" {
		cfg.KeyName = strings.TrimSpace(keyName)
	}
	if strings.TrimSpace(cfg.KeyName) == "" {
		fmt.Fprintln(stderr, "a rotated encrypted EVM key is required; pass --key or configure key_name")
		return 2
	}
	cfg.Network = profile.Name
	cfg.EVMRPC = endpoint
	cfg.EVMChainID = profile.EVMChainID
	signer := chain.NewEVMKeystoreSigner(cfg)
	operator, err := signer.OwnerAddress()
	if err != nil {
		fmt.Fprintf(stderr, "resolve encrypted EVM bootstrap key: %v\n", err)
		return 1
	}
	rpc := chain.NewEVMRPC(endpoint)
	transactor := chain.NewEVMTransactor(cfg, rpc, signer)
	manifest, err := services.deploy(ctx, suitedeploy.Options{
		ArtifactDirectory: filepath.Clean(artifacts), SourceCommit: sourceCommit,
		Network: profile.Name, ChainID: profile.EVMChainID, RPCEndpoint: endpoint,
		BlockExplorer: profile.EVMExplorer, SnapshotRoot: snapshotRoot, Operator: operator,
		PlatformFeeBPS: uint16(platformFeeBPS),
	}, rpc, transactor, filepath.Clean(output))
	if err != nil {
		fmt.Fprintf(stderr, "deploy immutable EVM suite: %v\n", err)
		fmt.Fprintf(stderr, "inspect the no-clobber evidence file before any retry: %s\n", output)
		return 1
	}
	if manifest == nil || manifest.DirectoryBindingVerification == nil || manifest.DirectoryBindingVerification.Active {
		fmt.Fprintln(stderr, "deployment returned invalid bootstrapping evidence")
		return 1
	}
	var directory string
	for _, contract := range manifest.Contracts {
		if contract.ContractName == "SuiteDirectory" {
			directory = contract.Address
			break
		}
	}
	fmt.Fprintf(stdout, "deployment evidence: %s\nnetwork: %s\nchain id: %d\nsuite directory: %s\ncontracts: %d\nconfiguration transactions: %d\ndirectory state: bootstrapping\nblockscout verification: %s\n",
		output, profile.Name, profile.EVMChainID, directory, len(manifest.Contracts), len(manifest.ConfigurationTransactions), manifest.BlockscoutVerification.Status)
	return 0
}

func verifyCheckedInSuiteSources(artifactDirectory, sourceCommit string) error {
	repositoryRootRaw, err := gitOutput("", "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("resolve Git repository root: %w", err)
	}
	repositoryRoot := filepath.Clean(strings.TrimSpace(string(repositoryRootRaw)))
	expectedArtifacts, err := filepath.EvalSymlinks(filepath.Join(repositoryRoot, "contracts", "evm-v2", "artifacts"))
	if err != nil {
		return fmt.Errorf("resolve repository artifact directory: %w", err)
	}
	actualArtifacts, err := filepath.Abs(filepath.Clean(artifactDirectory))
	if err != nil {
		return fmt.Errorf("resolve requested artifact directory: %w", err)
	}
	actualArtifacts, err = filepath.EvalSymlinks(actualArtifacts)
	if err != nil {
		return fmt.Errorf("resolve requested artifact directory: %w", err)
	}
	if actualArtifacts != expectedArtifacts {
		return fmt.Errorf("artifact directory %s is not the checked-in repository directory %s", actualArtifacts, expectedArtifacts)
	}
	headRaw, err := gitOutput(repositoryRoot, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("resolve HEAD commit: %w", err)
	}
	head := strings.ToLower(strings.TrimSpace(string(headRaw)))
	if head != strings.ToLower(strings.TrimSpace(sourceCommit)) {
		return fmt.Errorf("source commit %s does not equal checked-out HEAD %s", sourceCommit, head)
	}
	paths := []string{
		"contracts/evm-v2/src",
		"contracts/evm-v2/artifacts",
		"contracts/evm-v2/package.json",
		"contracts/evm-v2/package-lock.json",
		"scripts/evm-suite-solc-check.mjs",
	}
	status, err := gitOutput(repositoryRoot, append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)...)
	if err != nil {
		return fmt.Errorf("inspect suite source worktree: %w", err)
	}
	if strings.TrimSpace(string(status)) != "" {
		return fmt.Errorf("suite sources/artifacts are not clean at HEAD:\n%s", strings.TrimSpace(string(status)))
	}
	tracked := append([]string{"ls-files", "--error-unmatch", "--"}, paths...)
	if _, err := gitOutput(repositoryRoot, tracked...); err != nil {
		return fmt.Errorf("suite source evidence includes untracked paths: %w", err)
	}
	return nil
}

func gitOutput(directory string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	if directory != "" {
		command.Dir = directory
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
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

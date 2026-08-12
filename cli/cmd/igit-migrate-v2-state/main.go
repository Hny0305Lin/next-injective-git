// Command igit-migrate-v2-state exports receipt-pinned EVM V2 state and
// verifies it against the exact offline V1 import plan. It is read-only: the
// command never loads a signer, signs calldata, or broadcasts a transaction.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

type commandServices struct {
	readPlan    func(string) (*migration.Plan, error)
	exportState func(context.Context, *migration.Plan, string, config.Config) (*migration.ImportedState, error)
	marshal     func(*migration.ImportedState) ([]byte, error)
	writeOutput func(string, []byte) error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithServices(args, stdout, stderr, commandServices{
		readPlan:    migration.ReadPlan,
		exportState: exportStateFromRPC,
		marshal:     migration.MarshalImportedState,
		writeOutput: writeExclusiveArtifact,
	})
}

func runWithServices(args []string, stdout, stderr io.Writer, services commandServices) int {
	var planPath string
	var finalizeTx string
	var output string
	var network string
	var rpcOverride string
	flags := flag.NewFlagSet("igit-migrate-v2-state", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&planPath, "plan", "", "validated V1-to-V2 import plan JSON")
	flags.StringVar(&finalizeTx, "finalize-tx", "", "mined finalizeImport transaction hash")
	flags.StringVar(&output, "output", "", "new receipt-pinned imported-state JSON artifact")
	flags.StringVar(&network, "network", "injective-testnet", "named Injective network profile")
	flags.StringVar(&rpcOverride, "rpc", "", "operator-only EVM JSON-RPC override")
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
	if strings.TrimSpace(planPath) == "" || strings.TrimSpace(finalizeTx) == "" || strings.TrimSpace(output) == "" {
		fmt.Fprintln(stderr, "--plan, --finalize-tx, and --output are required")
		return 2
	}
	if pathsEqual(planPath, output) {
		fmt.Fprintln(stderr, "output must not overwrite the import plan")
		return 2
	}
	if err := requireNewOutput(output); err != nil {
		fmt.Fprintf(stderr, "validate imported-state output: %v\n", err)
		return 1
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
		endpoint = strings.TrimSpace(profile.EVMRPC)
	}
	endpoint, err = validateRPCEndpoint(endpoint)
	if err != nil {
		fmt.Fprintf(stderr, "validate EVM RPC endpoint: %v\n", err)
		return 2
	}
	cfg := config.ApplyNetworkProfile(config.Config{
		Network:            profile.Name,
		ContractBackend:    "evm",
		ContractVersion:    "v2",
		EVMRPC:             endpoint,
		EVMContractAddress: plan.Target.Contract,
		EVMChainID:         plan.Target.ChainID,
	})
	// The exporter is deliberately EVM-only even if the selected profile has a
	// legacy V1 contract. Fixed-block reads must never fall back to CosmWasm.
	cfg.ContractAddress = ""

	state, err := services.exportState(context.Background(), plan, finalizeTx, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "export finalized V2 state: %v\n", err)
		return 1
	}
	if state == nil {
		fmt.Fprintln(stderr, "export finalized V2 state: exporter returned nil state")
		return 1
	}
	raw, err := services.marshal(state)
	if err != nil {
		fmt.Fprintf(stderr, "encode finalized V2 state: %v\n", err)
		return 1
	}
	if err := services.writeOutput(output, raw); err != nil {
		fmt.Fprintf(stderr, "write finalized V2 state: %v\n", err)
		return 1
	}

	refCount, collaboratorCount := stateCounts(state)
	fmt.Fprintf(
		stdout,
		"imported state: %s\nnetwork: %s\nimport scope: %s\ndeferred sections: %s\nblock tag: %s\nblock hash: %s\nrepositories: %d\nrefs: %d\ncollaborators: %d\nstate verification: pass\nread-only: true\nsigned: false\nbroadcast: false\n",
		output,
		profile.Name,
		state.ImportScope,
		strings.Join(state.DeferredSections, ","),
		state.BlockTag,
		state.Finalization.BlockHash,
		len(state.Repositories),
		refCount,
		collaboratorCount,
	)
	return 0
}

func exportStateFromRPC(
	ctx context.Context,
	plan *migration.Plan,
	finalizeTx string,
	cfg config.Config,
) (*migration.ImportedState, error) {
	rpc := chain.NewEVMRPC(cfg.EffectiveEVMRPC())
	reader := chain.NewEVMRegistryV2WithDependencies(cfg, rpc, nil)
	return migration.ExportImportedState(ctx, plan, finalizeTx, reader)
}

func validateRPCEndpoint(raw string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(raw), "/")
	if endpoint == "" {
		return "", errors.New("endpoint is empty")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("endpoint must be an absolute http(s) URL")
	}
	if parsed.Fragment != "" {
		return "", errors.New("endpoint must not contain a URL fragment")
	}
	return endpoint, nil
}

func stateCounts(state *migration.ImportedState) (int, int) {
	if state == nil {
		return 0, 0
	}
	refs := 0
	collaborators := 0
	for _, repository := range state.Repositories {
		refs += len(repository.Refs)
		collaborators += len(repository.Collaborators)
	}
	return refs, collaborators
}

func requireNewOutput(output string) error {
	_, err := os.Lstat(output)
	if err == nil {
		return fmt.Errorf("%s already exists; migration evidence is never overwritten", output)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeExclusiveArtifact publishes a fully written and synced temporary file
// with an atomic, no-clobber hard link. The temporary file is created in the
// destination directory so the link cannot cross filesystems.
func writeExclusiveArtifact(output string, raw []byte) error {
	if err := requireNewOutput(output); err != nil {
		return err
	}
	directory := filepath.Dir(output)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".igit-v2-state-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary output: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Link(temporaryName, output); err != nil {
		return fmt.Errorf("publish output without overwrite: %w", err)
	}
	return nil
}

func pathsEqual(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftPath, leftErr := filepath.Abs(filepath.Clean(left))
	rightPath, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftPath, rightPath)
	}
	return leftPath == rightPath
}

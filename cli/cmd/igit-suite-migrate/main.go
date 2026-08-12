// Command igit-suite-migrate creates and verifies immutable, unsigned EVM
// Suite bootstrap evidence. It has no RPC, key loading, signing, or broadcast
// path.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitemigration"
)

type commandServices struct {
	verifySnapshot  func(string, string) error
	buildPlan       func(string, suitemigration.BuildOptions) (*suitemigration.Plan, error)
	buildManifest   func(*suitemigration.Plan) (*suitemigration.CalldataManifest, error)
	readPlan        func(string) (*suitemigration.Plan, error)
	readManifest    func(string, *suitemigration.Plan) (*suitemigration.CalldataManifest, error)
	marshalPlan     func(*suitemigration.Plan) ([]byte, error)
	marshalManifest func(*suitemigration.Plan, *suitemigration.CalldataManifest) ([]byte, error)
	publish         func(string, []byte, string, []byte) error
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	return runWithServices(args, stdout, stderr, commandServices{
		verifySnapshot:  suitemigration.VerifySnapshotSHA256File,
		buildPlan:       suitemigration.BuildPlanFile,
		buildManifest:   suitemigration.BuildCalldataManifest,
		readPlan:        suitemigration.ReadPlan,
		readManifest:    suitemigration.ReadCalldataManifest,
		marshalPlan:     suitemigration.MarshalPlan,
		marshalManifest: suitemigration.MarshalCalldataManifest,
		publish:         publishArtifacts,
	})
}

func runWithServices(args []string, stdout, stderr io.Writer, services commandServices) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr, services)
	case "verify":
		return runVerify(args[1:], stdout, stderr, services)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func runBuild(args []string, stdout, stderr io.Writer, services commandServices) int {
	var snapshotPath, hashPath, directory, coordinator, planOutput, manifestOutput string
	var chainID uint64
	var batchSize int
	flags := flag.NewFlagSet("igit-suite-migrate build", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&snapshotPath, "snapshot", "", "complete canonical V1 suite snapshot JSON")
	flags.StringVar(&hashPath, "hash-file", "", "snapshot SHA-256 sidecar (default <snapshot>.sha256)")
	flags.Uint64Var(&chainID, "target-chain-id", 0, "target EVM chain ID")
	flags.StringVar(&directory, "directory", "", "deployed SuiteDirectory address")
	flags.StringVar(&coordinator, "coordinator", "", "deployed BootstrapCoordinator address")
	flags.IntVar(&batchSize, "batch-size", suitemigration.DefaultBatchSize, "maximum records per import batch")
	flags.StringVar(&planOutput, "plan-output", "suite-bootstrap-plan.json", "new immutable suite bootstrap plan")
	flags.StringVar(&manifestOutput, "manifest-output", "suite-bootstrap-calldata.json", "new immutable unsigned calldata manifest")
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
	if strings.TrimSpace(snapshotPath) == "" || chainID == 0 || strings.TrimSpace(directory) == "" || strings.TrimSpace(coordinator) == "" {
		fmt.Fprintln(stderr, "--snapshot, --target-chain-id, --directory, and --coordinator are required")
		return 2
	}
	if hashPath == "" {
		hashPath = snapshotPath + ".sha256"
	}
	if err := validateOutputPaths(snapshotPath, hashPath, planOutput, manifestOutput); err != nil {
		fmt.Fprintf(stderr, "validate outputs: %v\n", err)
		return 1
	}
	if err := services.verifySnapshot(snapshotPath, hashPath); err != nil {
		fmt.Fprintf(stderr, "verify snapshot evidence: %v\n", err)
		return 1
	}
	plan, err := services.buildPlan(snapshotPath, suitemigration.BuildOptions{TargetChainID: chainID, Directory: strings.ToLower(strings.TrimSpace(directory)), Coordinator: strings.ToLower(strings.TrimSpace(coordinator)), BatchSize: batchSize})
	if err != nil {
		fmt.Fprintf(stderr, "build suite bootstrap plan: %v\n", err)
		return 1
	}
	manifest, err := services.buildManifest(plan)
	if err != nil {
		fmt.Fprintf(stderr, "build suite calldata manifest: %v\n", err)
		return 1
	}
	planRaw, err := services.marshalPlan(plan)
	if err != nil {
		fmt.Fprintf(stderr, "encode suite bootstrap plan: %v\n", err)
		return 1
	}
	manifestRaw, err := services.marshalManifest(plan, manifest)
	if err != nil {
		fmt.Fprintf(stderr, "encode suite calldata manifest: %v\n", err)
		return 1
	}
	if err := services.publish(planOutput, planRaw, manifestOutput, manifestRaw); err != nil {
		fmt.Fprintf(stderr, "publish suite migration evidence: %v\n", err)
		return 1
	}
	planSHA, err := suitemigration.PlanSHA256(plan)
	if err != nil {
		fmt.Fprintf(stderr, "hash suite plan: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "suite bootstrap plan: %s\nplan sha256: %s\nsnapshot root: %s\ncalldata manifest: %s\nmodules: %d\nbatches: %d\ntransactions: %d\nsigned: false\nbroadcast: false\n", planOutput, planSHA, plan.Snapshot.Root, manifestOutput, len(plan.Modules), plan.Summary.BatchCount, len(manifest.Transactions))
	return 0
}

func runVerify(args []string, stdout, stderr io.Writer, services commandServices) int {
	var planPath, manifestPath string
	flags := flag.NewFlagSet("igit-suite-migrate verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&planPath, "plan", "", "suite bootstrap plan JSON")
	flags.StringVar(&manifestPath, "manifest", "", "unsigned Coordinator calldata manifest JSON")
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
	if strings.TrimSpace(planPath) == "" || strings.TrimSpace(manifestPath) == "" {
		fmt.Fprintln(stderr, "--plan and --manifest are required")
		return 2
	}
	plan, err := services.readPlan(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify suite plan: %v\n", err)
		return 1
	}
	manifest, err := services.readManifest(manifestPath, plan)
	if err != nil {
		fmt.Fprintf(stderr, "verify suite calldata manifest: %v\n", err)
		return 1
	}
	planSHA, err := suitemigration.PlanSHA256(plan)
	if err != nil {
		fmt.Fprintf(stderr, "hash suite plan: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "suite migration evidence: pass\nplan sha256: %s\nsnapshot root: %s\nmodules: %d\nbatches: %d\ntransactions: %d\nsigned: false\nbroadcast: false\n", planSHA, plan.Snapshot.Root, len(plan.Modules), plan.Summary.BatchCount, len(manifest.Transactions))
	return 0
}

func usage(output io.Writer) {
	fmt.Fprintln(output, "usage: igit-suite-migrate <build|verify> [options]")
}

func validateOutputPaths(snapshot, hashFile, plan, manifest string) error {
	if strings.TrimSpace(plan) == "" || strings.TrimSpace(manifest) == "" {
		return errors.New("plan and manifest output paths are required")
	}
	paths := []string{snapshot, hashFile, plan, manifest}
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if pathsEqual(paths[i], paths[j]) {
				return fmt.Errorf("paths must be distinct: %s", paths[i])
			}
		}
	}
	for _, path := range []string{plan, manifest} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists; migration evidence is never overwritten", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
	}
	return nil
}

func pathsEqual(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	a, aErr := filepath.Abs(filepath.Clean(left))
	b, bErr := filepath.Abs(filepath.Clean(right))
	if aErr != nil || bErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func publishArtifacts(planPath string, planRaw []byte, manifestPath string, manifestRaw []byte) error {
	if err := writeExclusive(planPath, planRaw); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	if err := writeExclusive(manifestPath, manifestRaw); err != nil {
		return fmt.Errorf("plan %s was published and must be retained; write manifest: %w", planPath, err)
	}
	return nil
}

func writeExclusive(path string, raw []byte) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s already exists; migration evidence is never overwritten", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".igit-suite-migrate-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("artifact published but open output directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("artifact published but sync output directory: %w", err)
	}
	return nil
}

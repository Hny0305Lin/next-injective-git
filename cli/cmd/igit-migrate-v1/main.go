// Command igit-migrate-v1 builds an offline, deterministic V1-to-V2 import
// plan. It never contacts RPC, signs, or broadcasts a transaction.
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

	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var options migration.Options
	var output string
	var manifestOutput string
	var verifyStatePath string
	flags := flag.NewFlagSet("igit-migrate-v1", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.SnapshotPath, "snapshot", "", "canonical V1 snapshot JSON")
	flags.StringVar(&options.HashPath, "hash-file", "", "snapshot SHA-256 sidecar (default <snapshot>.sha256)")
	flags.Uint64Var(&options.TargetChainID, "target-chain-id", 0, "target EVM chain ID")
	flags.StringVar(&options.TargetContract, "target-contract", "", "target RepoRegistryV2 contract address")
	flags.StringVar(&options.ControllerContract, "controller-contract", "", "optional RepoRegistryV2ImportController address")
	flags.IntVar(&options.BatchSize, "batch-size", migration.DefaultBatchSize, "maximum refs or collaborators per batch")
	flags.StringVar(&output, "output", "v1-import-plan.json", "output plan JSON")
	flags.StringVar(&manifestOutput, "manifest-output", "", "optional unsigned ABI transaction manifest JSON")
	flags.StringVar(&verifyStatePath, "verify-state", "", "optional finalized V2 state export to compare with the plan")
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
	if anyPathEqual(output, options.SnapshotPath, options.HashPath, verifyStatePath) {
		fmt.Fprintln(stderr, "output must not overwrite the snapshot, hash, or verified-state file")
		return 2
	}
	if manifestOutput != "" && anyPathEqual(manifestOutput, options.SnapshotPath, options.HashPath, verifyStatePath, output) {
		fmt.Fprintln(stderr, "manifest output must differ from the plan, snapshot, hash, and verified-state files")
		return 2
	}
	if err := validateNewOutputs(output, manifestOutput); err != nil {
		fmt.Fprintf(stderr, "validate import evidence outputs: %v\n", err)
		return 1
	}
	plan, err := migration.BuildPlan(options)
	if err != nil {
		fmt.Fprintf(stderr, "build V1 import plan: %v\n", err)
		return 1
	}
	planDigest, err := migration.PlanSHA256(plan)
	if err != nil {
		fmt.Fprintf(stderr, "hash V1 import plan: %v\n", err)
		return 1
	}
	if verifyStatePath != "" {
		state, err := migration.ReadImportedState(verifyStatePath)
		if err != nil {
			fmt.Fprintf(stderr, "read finalized V2 state: %v\n", err)
			return 1
		}
		if err := migration.VerifyImportedState(plan, state); err != nil {
			fmt.Fprintf(stderr, "verify finalized V2 state: %v\n", err)
			return 1
		}
	}
	raw, err := migration.MarshalPlan(plan)
	if err != nil {
		fmt.Fprintf(stderr, "encode V1 import plan: %v\n", err)
		return 1
	}
	var manifest *migration.TransactionManifest
	var manifestRaw []byte
	if manifestOutput != "" {
		manifest, err = migration.BuildTransactionManifest(plan)
		if err != nil {
			fmt.Fprintf(stderr, "build V2 transaction manifest: %v\n", err)
			return 1
		}
		manifestRaw, err = migration.MarshalTransactionManifest(manifest)
		if err != nil {
			fmt.Fprintf(stderr, "encode V2 transaction manifest: %v\n", err)
			return 1
		}
	}
	if err := publishImportArtifacts(output, raw, manifestOutput, manifestRaw); err != nil {
		fmt.Fprintf(stderr, "publish import evidence: %v\n", err)
		return 1
	}
	if verifyStatePath != "" {
		fmt.Fprintf(stdout, "state verification: pass\nverified state: %s\n", verifyStatePath)
	}
	fmt.Fprintf(
		stdout,
		"import plan: %s\nplan sha256: %s\nrepositories: %d\nrefs: %d\ncollaborators: %d\nbatches: %d\ndeferred sections: %s\n",
		output,
		planDigest,
		plan.Summary.RepositoryCount,
		plan.Summary.RefCount,
		plan.Summary.CollaboratorCount,
		plan.Summary.BatchCount,
		strings.Join(plan.DeferredSections, ","),
	)
	if manifestOutput != "" {
		fmt.Fprintf(
			stdout,
			"transaction manifest: %s\ncalldata transactions: %d\nsigned: false\nbroadcast: false\n",
			manifestOutput,
			manifest.Summary.TransactionCount,
		)
	}
	return 0
}

func anyPathEqual(path string, candidates ...string) bool {
	for _, candidate := range candidates {
		if pathsEqual(path, candidate) {
			return true
		}
	}
	return false
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

func validateNewOutputs(outputs ...string) error {
	seen := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if strings.TrimSpace(output) == "" {
			continue
		}
		for _, previous := range seen {
			if pathsEqual(output, previous) {
				return fmt.Errorf("output paths must be distinct: %s", output)
			}
		}
		if err := requireNewOutput(output); err != nil {
			return err
		}
		seen = append(seen, output)
	}
	return nil
}

func requireNewOutput(output string) error {
	info, err := os.Lstat(output)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s already exists as a symlink; migration evidence is never overwritten", output)
		}
		return fmt.Errorf("%s already exists; migration evidence is never overwritten", output)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", output, err)
	}
	return nil
}

// PublishedOutputError means the final no-clobber link exists but the
// containing directory could not be synced. The artifact is therefore
// published and must be preserved for inspection before retrying.
type PublishedOutputError struct {
	Path string
	Err  error
}

func (err *PublishedOutputError) Error() string {
	if err == nil {
		return "migration evidence was published but directory durability was not confirmed"
	}
	return fmt.Sprintf("migration evidence %s was published but directory durability was not confirmed: %v", err.Path, err.Err)
}

func (err *PublishedOutputError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// PartialPublicationError records that the plan was published before the
// manifest failed. Both files are immutable; callers must retain the plan and
// inspect this error rather than deleting or replacing either artifact.
type PartialPublicationError struct {
	PublishedPath string
	FailedPath    string
	Err           error
}

func (err *PartialPublicationError) Error() string {
	if err == nil {
		return "migration evidence publication was partial"
	}
	return fmt.Sprintf("partial migration evidence publication: %s was published; %s was not published: %v", err.PublishedPath, err.FailedPath, err.Err)
}

func (err *PartialPublicationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func publishImportArtifacts(planPath string, planRaw []byte, manifestPath string, manifestRaw []byte) error {
	if err := writeSecureOutput(planPath, planRaw); err != nil {
		return fmt.Errorf("write import plan: %w", err)
	}
	if strings.TrimSpace(manifestPath) == "" {
		return nil
	}
	if err := writeSecureOutput(manifestPath, manifestRaw); err != nil {
		var published *PublishedOutputError
		if errors.As(err, &published) {
			return fmt.Errorf("plan and manifest were published, but manifest directory durability was not confirmed: %w", err)
		}
		return &PartialPublicationError{PublishedPath: planPath, FailedPath: manifestPath, Err: err}
	}
	return nil
}

func writeSecureOutput(output string, raw []byte) error {
	if err := requireNewOutput(output); err != nil {
		return err
	}
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".v1-import-plan-*")
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
	if err := syncOutputDirectory(dir); err != nil {
		return &PublishedOutputError{Path: output, Err: err}
	}
	return nil
}

func syncOutputDirectory(directory string) error {
	if syncOutputDirectoryHook != nil {
		return syncOutputDirectoryHook(directory)
	}
	if runtime.GOOS == "windows" {
		// Windows has no portable directory-handle Sync contract. The temporary
		// file was synced before the atomic hard link; ACL/flush requirements are
		// documented in the release runbook.
		return nil
	}
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

// syncOutputDirectoryHook is kept narrow and package-local so tests can cover
// the post-publication durability boundary without weakening production
// publication semantics.
var syncOutputDirectoryHook func(string) error

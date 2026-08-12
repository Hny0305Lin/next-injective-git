package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

func TestRunExecutesOnlyExactConfirmedPlanAndManifest(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	digest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	var gotConfig config.Config
	var gotOptions migration.ImportExecutionOptions
	services := commandMigrationServices(plan, manifest)
	services.execute = func(
		_ context.Context,
		gotPlan *migration.Plan,
		gotManifest *migration.TransactionManifest,
		journal string,
		cfg config.Config,
		options migration.ImportExecutionOptions,
	) (*migration.ImportExecutionResult, error) {
		if gotPlan != plan || gotManifest != manifest || journal != "journal-dir" {
			t.Fatalf("runner inputs = %#v %#v %q", gotPlan, gotManifest, journal)
		}
		gotConfig = cfg
		gotOptions = options
		return &migration.ImportExecutionResult{
			TransactionCount: len(manifest.Transactions), PreparedThisRun: 2,
			BroadcastRecordedThisRun: 2, MinedThisRun: 2,
			FinalizeTransactionHash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}, nil
	}

	var stdout strings.Builder
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json",
		"--manifest", "transactions.json",
		"--journal", "journal-dir",
		"--confirm-plan-sha256", digest,
		"--acknowledge-core-only-import",
		"--rpc", "http://127.0.0.1:8545/",
		"--key", "migration-admin",
		"--receipt-timeout", "2m",
		"--max-gas", "9000000",
	}, &stdout, &stderr, services)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if gotConfig.Network != "injective-testnet" || gotConfig.EVMRPC != "http://127.0.0.1:8545" ||
		gotConfig.EVMChainID != 1439 || gotConfig.EVMContractAddress != plan.Target.Contract ||
		gotConfig.ContractAddress != "" || gotConfig.KeyName != "migration-admin" ||
		gotConfig.ContractBackend != "evm" || gotConfig.ContractVersion != "v2" {
		t.Fatalf("runner config = %#v", gotConfig)
	}
	if gotOptions.ReceiptTimeout != 2*time.Minute || gotOptions.MaxGasLimit != 9_000_000 {
		t.Fatalf("runner options = %#v", gotOptions)
	}
	for _, expected := range []string{
		"receipt journal: journal-dir", "network: injective-testnet",
		"prepared this run: 2", "broadcast records this run: 2", "mined this run: 2",
		"finalize transaction: 0xaaaaaaaa",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunRequiresExactPlanHashBeforeConfigOrExecution(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	digest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	services := commandMigrationServices(plan, manifest)
	services.loadConfig = func() (config.Config, error) {
		t.Fatal("config loaded before explicit broadcast confirmation")
		return config.Config{}, nil
	}
	services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
		t.Fatal("runner executed before explicit broadcast confirmation")
		return nil, nil
	}
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "journal",
	}, &strings.Builder{}, &stderr, services)
	if code != 2 || !strings.Contains(stderr.String(), "--confirm-plan-sha256 "+digest) {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
}

func TestRunCheckValidatesArtifactsWithoutJournalKeyOrRPC(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	services := commandMigrationServices(plan, manifest)
	services.loadConfig = func() (config.Config, error) {
		t.Fatal("check mode loaded key configuration")
		return config.Config{}, nil
	}
	services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
		t.Fatal("check mode executed the broadcaster")
		return nil, nil
	}
	var stdout strings.Builder
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--check",
	}, &stdout, &stderr, services)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	for _, expected := range []string{
		"plan sha256: ", "manifest sha256: ", "import scope: repo-ref-collaborator-core",
		"deferred sections: repo_extensions", "target chain id: 1439",
		"calldata ready: true", "signed: false", "broadcast: false",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("check output missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunStatusInspectsJournalWithoutKeyOrRPC(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	planDigest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := migration.TransactionManifestSHA256(manifest)
	if err != nil {
		t.Fatal(err)
	}
	services := commandMigrationServices(plan, manifest)
	services.loadConfig = func() (config.Config, error) {
		t.Fatal("status mode loaded key configuration")
		return config.Config{}, nil
	}
	services.inspect = func(directory string, gotPlan *migration.Plan, gotManifest *migration.TransactionManifest) (*migration.ReceiptJournalStatus, error) {
		if directory != "journal" || gotPlan != plan || gotManifest != manifest {
			t.Fatalf("status inputs = %q %#v %#v", directory, gotPlan, gotManifest)
		}
		return &migration.ReceiptJournalStatus{
			Header: migration.ReceiptJournalHeader{
				PlanSHA256: planDigest, ManifestSHA256: manifestDigest,
				ImportScope: plan.ImportScope, DeferredSections: plan.DeferredSections,
				Signer:           "0x3333333333333333333333333333333333333333",
				TransactionCount: len(manifest.Transactions),
			},
			PreparedTransactions: 2, BroadcastTransactions: 1, MinedTransactions: 1,
			FailedOrder: -1, NextOrder: 1,
		}, nil
	}
	services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
		t.Fatal("status mode executed the broadcaster")
		return nil, nil
	}
	var stdout strings.Builder
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "journal", "--status",
	}, &stdout, &stderr, services)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	for _, expected := range []string{
		"offline status: true", "chain evidence revalidated: false",
		"signer (journal metadata): 0x3333333333333333333333333333333333333333",
		"prepared: 2", "broadcast: 1", "mined: 1", "reverted: 0",
		"failed order: -1", "next order: 1", "complete: false", "import scope: repo-ref-collaborator-core",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("status output missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunRequiresExplicitCoreOnlyAcknowledgement(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	digest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	services := commandMigrationServices(plan, manifest)
	services.loadConfig = func() (config.Config, error) {
		t.Fatal("config loaded before deferred sections were acknowledged")
		return config.Config{}, nil
	}
	services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
		t.Fatal("runner executed before deferred sections were acknowledged")
		return nil, nil
	}
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "journal",
		"--confirm-plan-sha256", digest,
	}, &strings.Builder{}, &stderr, services)
	if code != 2 || !strings.Contains(stderr.String(), "--acknowledge-core-only-import") || !strings.Contains(stderr.String(), "repo_extensions") {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
}

func TestRunRejectsTamperedManifestBeforeConfirmation(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	manifest.Transactions[0].Data = "0x1234"
	services := commandMigrationServices(plan, manifest)
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "journal",
	}, &strings.Builder{}, &stderr, services)
	if code != 1 || !strings.Contains(stderr.String(), "does not exactly match") {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
}

func TestRunRejectsConfigurationBeforeExecution(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	digest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "journal",
		"--confirm-plan-sha256", digest,
		"--acknowledge-core-only-import",
	}
	tests := []struct {
		name      string
		args      []string
		configure func(*commandServices)
		wantCode  int
		want      string
	}{
		{name: "unknown network", args: append(append([]string(nil), base...), "--network", "unknown"), wantCode: 2, want: "unknown network"},
		{name: "chain mismatch", args: append(append([]string(nil), base...), "--network", "injective-mainnet"), wantCode: 1, want: "plan targets"},
		{name: "invalid RPC", args: append(append([]string(nil), base...), "--rpc", "localhost:8545"), wantCode: 2, want: "absolute http(s) URL"},
		{name: "zero timeout", args: append(append([]string(nil), base...), "--receipt-timeout", "0s"), wantCode: 2, want: "must be positive"},
		{name: "zero max gas", args: append(append([]string(nil), base...), "--max-gas", "0"), wantCode: 2, want: "must be positive"},
		{
			name: "missing key", args: base, wantCode: 2, want: "administrator key is required",
			configure: func(services *commandServices) {
				services.loadConfig = func() (config.Config, error) { return config.Config{}, nil }
			},
		},
		{
			name: "config read", args: base, wantCode: 1, want: "load igit keystore",
			configure: func(services *commandServices) {
				services.loadConfig = func() (config.Config, error) { return config.Config{}, errors.New("config denied") }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executed := false
			services := commandMigrationServices(plan, manifest)
			if tc.configure != nil {
				tc.configure(&services)
			}
			services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
				executed = true
				return nil, nil
			}
			var stderr strings.Builder
			code := runWithServices(context.Background(), tc.args, &strings.Builder{}, &stderr, services)
			if code != tc.wantCode || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("exit code = %d, stderr=%q, want %d/%q", code, stderr.String(), tc.wantCode, tc.want)
			}
			if executed {
				t.Fatal("invalid configuration reached the broadcaster")
			}
		})
	}
}

func TestRunExecutionFailureReportsDurableResumePath(t *testing.T) {
	plan, manifest := commandMigrationFixture(t)
	digest, err := migration.PlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	services := commandMigrationServices(plan, manifest)
	services.execute = func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
		return nil, errors.New("receipt reverted")
	}
	var stderr strings.Builder
	code := runWithServices(context.Background(), []string{
		"--plan", "plan.json", "--manifest", "manifest.json", "--journal", "resume-journal",
		"--confirm-plan-sha256", digest,
		"--acknowledge-core-only-import",
	}, &strings.Builder{}, &stderr, services)
	if code != 1 || !strings.Contains(stderr.String(), "receipt reverted") || !strings.Contains(stderr.String(), "resume-journal") {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
}

func TestRunHelpDoesNotReadMigrationArtifacts(t *testing.T) {
	var stderr strings.Builder
	if code := runWithServices(context.Background(), []string{"--help"}, &strings.Builder{}, &stderr, commandServices{}); code != 0 {
		t.Fatalf("help exit code = %d", code)
	}
	if !strings.Contains(stderr.String(), "confirm-plan-sha256") {
		t.Fatalf("help output = %s", stderr.String())
	}
}

func commandMigrationServices(plan *migration.Plan, manifest *migration.TransactionManifest) commandServices {
	return commandServices{
		readPlan:     func(string) (*migration.Plan, error) { return plan, nil },
		readManifest: func(string) (*migration.TransactionManifest, error) { return manifest, nil },
		loadConfig: func() (config.Config, error) {
			return config.Config{KeyName: "configured"}, nil
		},
		inspect: func(string, *migration.Plan, *migration.TransactionManifest) (*migration.ReceiptJournalStatus, error) {
			return &migration.ReceiptJournalStatus{}, nil
		},
		execute: func(context.Context, *migration.Plan, *migration.TransactionManifest, string, config.Config, migration.ImportExecutionOptions) (*migration.ImportExecutionResult, error) {
			return &migration.ImportExecutionResult{}, nil
		},
	}
}

func commandMigrationFixture(t *testing.T) (*migration.Plan, *migration.TransactionManifest) {
	t.Helper()
	directory := t.TempDir()
	snapshotPath := filepath.Join(directory, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	snapshot := migration.Snapshot{
		Schema: migration.SnapshotSchema,
		Source: migration.SnapshotSource{
			ChainID: "injective-888", Contract: "0x9999999999999999999999999999999999999999", Height: "123",
		},
		Repositories: []migration.SnapshotRepo{{
			Owner: "0x3333333333333333333333333333333333333333", Name: "demo",
			Description: "migration", DefaultBranch: "main", CreatedAt: 1, UpdatedAt: 2,
			ModerationStatus: "active",
		}},
		Refs: []migration.SnapshotRefGroup{{
			Owner: "0x3333333333333333333333333333333333333333", Repo: "demo",
			Refs: []migration.SnapshotRef{{
				RefName: "refs/heads/main", CommitSHA: strings.Repeat("a", 40),
				PackURIs: []string{"ipfs://bafy-demo"}, UpdatedAt: 2,
				UpdatedBy: "0x3333333333333333333333333333333333333333",
			}},
		}},
		Collaborators: []migration.SnapshotCollabGroup{{
			Owner: "0x3333333333333333333333333333333333333333", Repo: "demo",
			Collaborators: []migration.SnapshotCollaborator{},
		}},
		RepoExtensions: []migration.SnapshotExtension{{
			Owner: "0x3333333333333333333333333333333333333333", Repo: "demo",
		}},
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(snapshotPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	record := fmt.Sprintf("%s  %s\n", hex.EncodeToString(digest[:]), filepath.Base(snapshotPath))
	if err := os.WriteFile(hashPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := migration.BuildPlan(migration.Options{
		SnapshotPath: snapshotPath, HashPath: hashPath, TargetChainID: 1439,
		TargetContract:     "0x1111111111111111111111111111111111111111",
		ControllerContract: "0x2222222222222222222222222222222222222222",
		BatchSize:          2,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := migration.BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	return plan, manifest
}

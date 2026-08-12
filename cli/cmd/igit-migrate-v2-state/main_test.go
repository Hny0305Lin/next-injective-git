package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

const testFinalizeTx = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRunExportsReceiptPinnedStateAndPublishesOneArtifact(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "plan.json")
	output := filepath.Join(t.TempDir(), "evidence", "imported-state.json")
	plan := commandTestPlan()
	state := commandTestState()
	var gotConfig config.Config
	services := commandTestServices(plan)
	services.exportState = func(_ context.Context, gotPlan *migration.Plan, finalizeTx string, cfg config.Config) (*migration.ImportedState, error) {
		if gotPlan != plan {
			t.Fatal("exporter did not receive the strictly read plan")
		}
		if finalizeTx != testFinalizeTx {
			t.Fatalf("finalize tx = %q", finalizeTx)
		}
		gotConfig = cfg
		return state, nil
	}
	services.marshal = migration.MarshalImportedState
	services.writeOutput = writeExclusiveArtifact

	var stdout strings.Builder
	var stderr strings.Builder
	code := runWithServices([]string{
		"--plan", planPath,
		"--finalize-tx", testFinalizeTx,
		"--output", output,
		"--rpc", "http://127.0.0.1:8545/",
	}, &stdout, &stderr, services)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if gotConfig.Network != "injective-testnet" || gotConfig.EVMChainID != 1439 {
		t.Fatalf("network config = %#v", gotConfig)
	}
	if gotConfig.EVMRPC != "http://127.0.0.1:8545" {
		t.Fatalf("RPC override = %q", gotConfig.EVMRPC)
	}
	if gotConfig.EVMContractAddress != plan.Target.Contract || gotConfig.ContractAddress != "" {
		t.Fatalf("contract config = %#v", gotConfig)
	}
	if gotConfig.ContractBackend != "evm" || gotConfig.ContractVersion != "v2" {
		t.Fatalf("backend config = %#v", gotConfig)
	}

	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := migration.ReadImportedState(output)
	if err != nil {
		t.Fatalf("read published evidence: %v\n%s", err, raw)
	}
	if decoded.BlockTag != state.BlockTag || len(decoded.Repositories) != 1 {
		t.Fatalf("published state = %#v", decoded)
	}
	for _, expected := range []string{
		"network: injective-testnet",
		"import scope: repo-ref-collaborator-core",
		"deferred sections: repo_extensions",
		"block tag: 0x2a",
		"repositories: 1",
		"refs: 2",
		"collaborators: 1",
		"state verification: pass",
		"read-only: true",
		"signed: false",
		"broadcast: false",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, stdout.String())
		}
	}
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(output), ".igit-v2-state-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary artifacts remain: %v", temporary)
	}
}

func TestRunExporterFailureCreatesNoOutputOrDirectory(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "not-created", "state.json")
	services := commandTestServices(commandTestPlan())
	services.exportState = func(context.Context, *migration.Plan, string, config.Config) (*migration.ImportedState, error) {
		return nil, errors.New("receipt event mismatch")
	}
	services.writeOutput = func(string, []byte) error {
		t.Fatal("output writer called after failed verification")
		return nil
	}

	var stderr strings.Builder
	code := runWithServices(requiredArgs(filepath.Join(root, "plan.json"), output), &strings.Builder{}, &stderr, services)
	if code != 1 || !strings.Contains(stderr.String(), "receipt event mismatch") {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed export created output: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(output)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed export created output directory: %v", err)
	}
}

func TestRunEncodingFailureCreatesNoOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "state.json")
	services := commandTestServices(commandTestPlan())
	services.marshal = func(*migration.ImportedState) ([]byte, error) {
		return nil, errors.New("encode failed")
	}
	services.writeOutput = func(string, []byte) error {
		t.Fatal("output writer called after encoding failure")
		return nil
	}

	var stderr strings.Builder
	code := runWithServices(requiredArgs("plan.json", output), &strings.Builder{}, &stderr, services)
	if code != 1 || !strings.Contains(stderr.String(), "encode failed") {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("encoding failure created output: %v", err)
	}
}

func TestRunRejectsInvalidInputsBeforeExport(t *testing.T) {
	tests := []struct {
		name      string
		args      func(string) []string
		plan      *migration.Plan
		wantCode  int
		wantError string
		precreate bool
	}{
		{name: "required flags", args: func(string) []string { return nil }, plan: commandTestPlan(), wantCode: 2, wantError: "are required"},
		{name: "positional argument", args: func(output string) []string { return append(requiredArgs("plan.json", output), "extra") }, plan: commandTestPlan(), wantCode: 2, wantError: "positional"},
		{name: "same plan and output", args: func(output string) []string { return requiredArgs(output, output) }, plan: commandTestPlan(), wantCode: 2, wantError: "must not overwrite"},
		{name: "existing output", args: func(output string) []string { return requiredArgs("plan.json", output) }, plan: commandTestPlan(), wantCode: 1, wantError: "already exists", precreate: true},
		{name: "unknown network", args: func(output string) []string { return append(requiredArgs("plan.json", output), "--network", "unknown") }, plan: commandTestPlan(), wantCode: 2, wantError: "unknown network"},
		{name: "profile chain mismatch", args: func(output string) []string {
			return append(requiredArgs("plan.json", output), "--network", "injective-mainnet")
		}, plan: commandTestPlan(), wantCode: 1, wantError: "but the plan targets"},
		{name: "invalid RPC", args: func(output string) []string {
			return append(requiredArgs("plan.json", output), "--rpc", "localhost:8545")
		}, plan: commandTestPlan(), wantCode: 2, wantError: "absolute http(s) URL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "state.json")
			if tc.precreate {
				if err := os.WriteFile(output, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			exported := false
			services := commandTestServices(tc.plan)
			services.exportState = func(context.Context, *migration.Plan, string, config.Config) (*migration.ImportedState, error) {
				exported = true
				return commandTestState(), nil
			}
			var stderr strings.Builder
			code := runWithServices(tc.args(output), &strings.Builder{}, &stderr, services)
			if code != tc.wantCode || !strings.Contains(stderr.String(), tc.wantError) {
				t.Fatalf("exit code = %d, stderr=%q, want code=%d error=%q", code, stderr.String(), tc.wantCode, tc.wantError)
			}
			if exported {
				t.Fatal("invalid input reached the RPC exporter")
			}
			if tc.precreate {
				raw, err := os.ReadFile(output)
				if err != nil || string(raw) != "keep" {
					t.Fatalf("existing evidence changed: %q, %v", raw, err)
				}
			}
		})
	}
}

func TestWriteExclusiveArtifactDoesNotOverwriteEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "state.json")
	if err := writeExclusiveArtifact(output, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusiveArtifact(output, []byte("second\n")); err == nil || !strings.Contains(err.Error(), "never overwritten") {
		t.Fatalf("overwrite error = %v", err)
	}
	raw, err := os.ReadFile(output)
	if err != nil || string(raw) != "first\n" {
		t.Fatalf("published artifact = %q, err=%v", raw, err)
	}
}

func TestRunHelpDoesNotRequireServices(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	if code := runWithServices([]string{"--help"}, &stdout, &stderr, commandServices{}); code != 0 {
		t.Fatalf("help exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-finalize-tx") {
		t.Fatalf("help output = %s", stderr.String())
	}
}

func commandTestServices(plan *migration.Plan) commandServices {
	return commandServices{
		readPlan: func(string) (*migration.Plan, error) { return plan, nil },
		exportState: func(context.Context, *migration.Plan, string, config.Config) (*migration.ImportedState, error) {
			return commandTestState(), nil
		},
		marshal: migration.MarshalImportedState,
		writeOutput: func(string, []byte) error {
			return nil
		},
	}
}

func commandTestPlan() *migration.Plan {
	return &migration.Plan{Target: migration.PlanTarget{
		ChainID:  1439,
		Contract: "0x1111111111111111111111111111111111111111",
	}}
}

func commandTestState() *migration.ImportedState {
	return &migration.ImportedState{
		Schema:           migration.ImportedStateSchema,
		ImportScope:      migration.CoreImportScope,
		DeferredSections: []string{"repo_extensions"},
		BlockTag:         "0x2a",
		Finalization: migration.ImportedFinalization{
			TransactionHash: testFinalizeTx,
			BlockHash:       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Repositories: []migration.ImportedRepository{{
			RepoID: "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			Refs: []migration.PlanRef{
				{RefName: "refs/heads/main"},
				{RefName: "refs/tags/v1"},
			},
			Collaborators: []migration.PlanCollaborator{{Address: "0x2222222222222222222222222222222222222222", Role: "reader"}},
		}},
	}
}

func requiredArgs(planPath, output string) []string {
	return []string{
		"--plan", planPath,
		"--finalize-tx", testFinalizeTx,
		"--output", output,
	}
}

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/migration"
)

func TestWriteSecureOutputCreatesParentsAndCommitsCompleteBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "manifest.json")
	want := []byte("{\n  \"broadcast\": false\n}\n")
	if err := writeSecureOutput(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("output = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("temporary output leaked: %#v", entries)
	}
}

func TestWriteSecureOutputNeverOverwritesExistingEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeSecureOutput(path, []byte("replacement\n"))
	if err == nil || !strings.Contains(err.Error(), "never overwritten") {
		t.Fatalf("overwrite error = %v", err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil || string(raw) != "reviewed\n" {
		t.Fatalf("existing evidence = %q, err=%v", raw, readErr)
	}
}

func TestValidateNewOutputsRejectsExistingManifestBeforePlanPublication(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateNewOutputs(planPath, manifestPath); err == nil || !strings.Contains(err.Error(), "never overwritten") {
		t.Fatalf("validation error = %v", err)
	}
	if _, err := os.Lstat(planPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("validation published plan: %v", err)
	}
}

func TestValidateNewOutputsRejectsSymlinkAndDirectory(t *testing.T) {
	dir := t.TempDir()
	directoryPath := filepath.Join(dir, "directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateNewOutputs(directoryPath); err == nil || !strings.Contains(err.Error(), "never overwritten") {
		t.Fatalf("directory validation error = %v", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("unprivileged Windows symlink creation is not portable")
	}
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := validateNewOutputs(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink validation error = %v", err)
	}
}

func TestPublishImportArtifactsReportsImmutablePartialPublication(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	// Simulate a second publisher winning the manifest name after the upfront
	// pair validation but before this process links its manifest.
	if err := os.WriteFile(manifestPath, []byte("racing publisher\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := publishImportArtifacts(planPath, []byte("plan\n"), manifestPath, []byte("manifest\n"))
	var partial *PartialPublicationError
	if !errors.As(err, &partial) {
		t.Fatalf("publication error = %T %v, want PartialPublicationError", err, err)
	}
	if partial.PublishedPath != planPath || partial.FailedPath != manifestPath {
		t.Fatalf("partial publication = %#v", partial)
	}
	planRaw, planErr := os.ReadFile(planPath)
	manifestRaw, manifestErr := os.ReadFile(manifestPath)
	if planErr != nil || string(planRaw) != "plan\n" {
		t.Fatalf("published plan = %q, err=%v", planRaw, planErr)
	}
	if manifestErr != nil || string(manifestRaw) != "racing publisher\n" {
		t.Fatalf("racing manifest changed = %q, err=%v", manifestRaw, manifestErr)
	}
}

func TestWriteSecureOutputReportsPublishedButUnsyncedEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	syncFailure := errors.New("directory sync failed")
	syncOutputDirectoryHook = func(string) error { return syncFailure }
	t.Cleanup(func() { syncOutputDirectoryHook = nil })
	err := writeSecureOutput(path, []byte("plan\n"))
	var published *PublishedOutputError
	if !errors.As(err, &published) || !errors.Is(err, syncFailure) {
		t.Fatalf("write error = %T %v, want PublishedOutputError", err, err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil || string(raw) != "plan\n" {
		t.Fatalf("published evidence = %q, err=%v", raw, readErr)
	}
}

func TestPublishImportArtifactsReportsBothPublishedWhenManifestSyncFails(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	syncCalls := 0
	syncFailure := errors.New("manifest directory sync failed")
	syncOutputDirectoryHook = func(string) error {
		syncCalls++
		if syncCalls == 2 {
			return syncFailure
		}
		return nil
	}
	t.Cleanup(func() { syncOutputDirectoryHook = nil })

	err := publishImportArtifacts(planPath, []byte("plan\n"), manifestPath, []byte("manifest\n"))
	var published *PublishedOutputError
	var partial *PartialPublicationError
	if !errors.As(err, &published) || errors.As(err, &partial) || !errors.Is(err, syncFailure) {
		t.Fatalf("publication error = %T %v, want published-unsynced and not partial", err, err)
	}
	if !strings.Contains(err.Error(), "plan and manifest were published") {
		t.Fatalf("publication error does not report both artifacts: %v", err)
	}
	for path, want := range map[string]string{planPath: "plan\n", manifestPath: "manifest\n"} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil || string(raw) != want {
			t.Fatalf("published %s = %q, err=%v", path, raw, readErr)
		}
	}
}

func TestWriteSecureOutputPublicationRaceDoesNotClobberWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, raw := range [][]byte{[]byte("first\n"), []byte("second\n")} {
		raw := raw
		go func() {
			<-start
			results <- writeSecureOutput(path, raw)
		}()
	}
	close(start)
	errA, errB := <-results, <-results
	if (errA == nil) == (errB == nil) {
		t.Fatalf("race errors = %v / %v, want exactly one winner", errA, errB)
	}
	raw, err := os.ReadFile(path)
	if err != nil || (string(raw) != "first\n" && string(raw) != "second\n") {
		t.Fatalf("published winner = %q, err=%v", raw, err)
	}
}

func TestRunVerifiesFinalizedStateBeforeWritingOutputs(t *testing.T) {
	snapshotPath, hashPath, statePath := writeCommandMigrationFixture(t, false)
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		"--snapshot", snapshotPath,
		"--hash-file", hashPath,
		"--target-chain-id", "1439",
		"--target-contract", commandTargetContract,
		"--controller-contract", commandControllerContract,
		"--verify-state", statePath,
		"--output", planPath,
		"--manifest-output", manifestPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "state verification: pass") {
		t.Fatalf("stdout does not report successful verification: %s", stdout.String())
	}
	writtenPlan, err := migration.ReadPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	planDigest, err := migration.PlanSHA256(writtenPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "plan sha256: "+planDigest) {
		t.Fatalf("stdout does not report the confirmation digest: %s", stdout.String())
	}
	for _, path := range []string{planPath, manifestPath} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("expected complete output %s: info=%v err=%v", path, info, err)
		}
	}
}

func TestRunStateMismatchWritesNoPlanOrManifest(t *testing.T) {
	snapshotPath, hashPath, statePath := writeCommandMigrationFixture(t, true)
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{
		"--snapshot", snapshotPath,
		"--hash-file", hashPath,
		"--target-chain-id", "1439",
		"--target-contract", commandTargetContract,
		"--controller-contract", commandControllerContract,
		"--verify-state", statePath,
		"--output", planPath,
		"--manifest-output", manifestPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "state verification: pass") {
		t.Fatalf("mismatched state was reported as verified: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "plan_sha256 mismatch") {
		t.Fatalf("stderr = %q, want plan hash mismatch", stderr.String())
	}
	for _, path := range []string{planPath, manifestPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("verification failure created %s: %v", path, err)
		}
	}
}

func TestRunExistingManifestWritesNoPlanAndPreservesManifest(t *testing.T) {
	snapshotPath, hashPath, _ := writeCommandMigrationFixture(t, false)
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := run([]string{
		"--snapshot", snapshotPath,
		"--hash-file", hashPath,
		"--target-chain-id", "1439",
		"--target-contract", commandTargetContract,
		"--controller-contract", commandControllerContract,
		"--output", planPath,
		"--manifest-output", manifestPath,
	}, &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "never overwritten") {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Lstat(planPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("existing manifest validation created plan: %v", err)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil || string(raw) != "reviewed\n" {
		t.Fatalf("manifest = %q, err=%v", raw, err)
	}
}

func TestRunRefusesToOverwriteVerifiedState(t *testing.T) {
	snapshotPath, hashPath, statePath := writeCommandMigrationFixture(t, false)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := run([]string{
		"--snapshot", snapshotPath,
		"--hash-file", hashPath,
		"--target-chain-id", "1439",
		"--target-contract", commandTargetContract,
		"--controller-contract", commandControllerContract,
		"--verify-state", statePath,
		"--output", statePath,
	}, &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "must not overwrite") {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("verified-state input changed after rejected output collision")
	}
}

const (
	commandSourceContract     = "0x1111111111111111111111111111111111111111"
	commandTargetContract     = "0x2222222222222222222222222222222222222222"
	commandControllerContract = "0x3333333333333333333333333333333333333333"
	commandOwner              = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	commandCollaborator       = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func writeCommandMigrationFixture(t *testing.T, mismatch bool) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	statePath := filepath.Join(dir, "imported-state.json")
	snapshot := migration.Snapshot{
		Schema: migration.SnapshotSchema,
		Source: migration.SnapshotSource{ChainID: "injective-888", Contract: commandSourceContract, Height: "42"},
		Repositories: []migration.SnapshotRepo{{
			Owner: commandOwner, Name: "legacy", Description: "history",
			DefaultBranch: "main", CreatedAt: 1, UpdatedAt: 2, ModerationStatus: "active",
		}},
		Refs: []migration.SnapshotRefGroup{{
			Owner: commandOwner,
			Repo:  "legacy",
			Refs: []migration.SnapshotRef{{
				RefName: "refs/heads/main", CommitSHA: strings.Repeat("a", 40),
				PackURIs: []string{"ipfs://bafy-test-pack"}, UpdatedAt: 2, UpdatedBy: commandOwner,
			}},
		}},
		Collaborators: []migration.SnapshotCollabGroup{{
			Owner: commandOwner,
			Repo:  "legacy",
			Collaborators: []migration.SnapshotCollaborator{{
				Address: commandCollaborator,
				Role:    "reader",
			}},
		}},
		RepoExtensions: []migration.SnapshotExtension{{Owner: commandOwner, Repo: "legacy"}},
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
	hashRecord := hex.EncodeToString(digest[:]) + "  " + filepath.Base(snapshotPath) + "\n"
	if err := os.WriteFile(hashPath, []byte(hashRecord), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := migration.BuildPlan(migration.Options{
		SnapshotPath:       snapshotPath,
		HashPath:           hashPath,
		TargetChainID:      1439,
		TargetContract:     commandTargetContract,
		ControllerContract: commandControllerContract,
		BatchSize:          migration.DefaultBatchSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	planRaw, err := migration.MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	planDigest := sha256.Sum256(planRaw)
	repository := plan.Repositories[0]
	state := &migration.ImportedState{
		Schema:           migration.ImportedStateSchema,
		ImportScope:      plan.ImportScope,
		DeferredSections: append([]string(nil), plan.DeferredSections...),
		PlanSHA256:       hex.EncodeToString(planDigest[:]),
		SnapshotSHA256:   plan.Snapshot.SHA256,
		SessionID:        "0x" + plan.Snapshot.SHA256,
		ImportCommitment: "0x" + plan.ImportCommitment,
		ChainID:          plan.Target.ChainID,
		Registry:         plan.Target.Contract,
		Controller:       plan.Target.Controller,
		BlockTag:         "0x2a",
		Finalization: migration.ImportedFinalization{
			TransactionHash: "0x" + strings.Repeat("a", 64),
			BlockHash:       "0x" + strings.Repeat("b", 64),
			Event:           "ImportPublished",
			EventEmitter:    plan.Target.Controller,
		},
		Progress: migration.ImportedProgress{
			Exists: true, Finalized: true,
			NextSequence: uint64(len(plan.Batches)), ExpectedBatches: uint64(len(plan.Batches)),
			ImportedRepositories: uint64(len(plan.Repositories)), ExpectedRepositories: uint64(len(plan.Repositories)),
			ImportedRefs: uint64(plan.Summary.RefCount), ImportedCollaborators: uint64(plan.Summary.CollaboratorCount),
		},
		Repositories: []migration.ImportedRepository{{
			RepoID: repository.RepoID, Owner: repository.Owner, Name: repository.Name,
			Description: repository.Description, DefaultBranch: repository.DefaultBranch,
			CreatedAt: repository.CreatedAt, UpdatedAt: repository.UpdatedAt,
			ModerationStatus: repository.ModerationStatus,
			Refs:             repository.Refs,
			Collaborators:    repository.Collaborators,
		}},
	}
	if mismatch {
		state.PlanSHA256 = strings.Repeat("0", 64)
	}
	stateRaw, err := migration.MarshalImportedState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	return snapshotPath, hashPath, statePath
}

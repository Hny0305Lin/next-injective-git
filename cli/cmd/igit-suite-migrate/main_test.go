package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitemigration"
)

func TestRunBuildAndVerifyOfflineEvidence(t *testing.T) {
	directory := t.TempDir()
	snapshotPath := filepath.Join(directory, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	planPath := filepath.Join(directory, "plan.json")
	manifestPath := filepath.Join(directory, "manifest.json")
	raw := commandSnapshotJSON(t)
	if err := os.WriteFile(snapshotPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	sidecar := hex.EncodeToString(digest[:]) + "  " + filepath.Base(snapshotPath) + "\n"
	if err := os.WriteFile(hashPath, []byte(sidecar), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"build", "--snapshot", snapshotPath, "--hash-file", hashPath, "--target-chain-id", "1439", "--directory", commandDirectory, "--coordinator", commandCoordinator, "--plan-output", planPath, "--manifest-output", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("build code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "signed: false\nbroadcast: false") {
		t.Fatalf("stdout=%s", stdout.String())
	}
	for _, path := range []string{planPath, manifestPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact %s info=%v err=%v", path, info, err)
		}
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"verify", "--plan", planPath, "--manifest", manifestPath}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "suite migration evidence: pass") {
		t.Fatalf("verify code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsBadSidecarAndDoesNotPublish(t *testing.T) {
	directory := t.TempDir()
	snapshotPath := filepath.Join(directory, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	planPath := filepath.Join(directory, "plan.json")
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(snapshotPath, commandSnapshotJSON(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hashPath, []byte(strings.Repeat("0", 64)+"  snapshot.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := run([]string{"build", "--snapshot", snapshotPath, "--hash-file", hashPath, "--target-chain-id", "1439", "--directory", commandDirectory, "--coordinator", commandCoordinator, "--plan-output", planPath, "--manifest-output", manifestPath}, &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "mismatch") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	for _, path := range []string{planPath, manifestPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("bad sidecar published %s: %v", path, err)
		}
	}
}

func TestRunNeverOverwritesEvidence(t *testing.T) {
	directory := t.TempDir()
	snapshotPath := filepath.Join(directory, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	planPath := filepath.Join(directory, "plan.json")
	manifestPath := filepath.Join(directory, "manifest.json")
	raw := commandSnapshotJSON(t)
	if err := os.WriteFile(snapshotPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if err := os.WriteFile(hashPath, []byte(hex.EncodeToString(digest[:])+"  snapshot.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := run([]string{"build", "--snapshot", snapshotPath, "--target-chain-id", "1439", "--directory", commandDirectory, "--coordinator", commandCoordinator, "--plan-output", planPath, "--manifest-output", manifestPath}, &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "never overwritten") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	content, _ := os.ReadFile(planPath)
	if string(content) != "reviewed\n" {
		t.Fatalf("plan overwritten: %q", content)
	}
}

const (
	commandDirectory   = "0x1000000000000000000000000000000000000001"
	commandCoordinator = "0x2000000000000000000000000000000000000002"
	commandAdmin       = "0x3000000000000000000000000000000000000003"
	commandOwner       = "0x4000000000000000000000000000000000000004"
	commandRepoID      = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func commandSnapshotJSON(t *testing.T) []byte {
	t.Helper()
	h := strings.Repeat("1", 64)
	snapshot := suitemigration.Snapshot{Schema: suitemigration.SnapshotSchema, Source: suitemigration.SnapshotSource{ChainID: "injective-888", Contract: "inj1contract", Height: 100, BlockHash: h, InventorySHA256: strings.Repeat("2", 64), TxSearchSHA256: strings.Repeat("3", 64), BlockEvidenceSHA256: strings.Repeat("4", 64), EventCommitmentSHA256: strings.Repeat("5", 64)}, ContractPolicy: suitemigration.ContractPolicy{Admin: commandAdmin, Treasury: commandAdmin, Committee: commandAdmin, ReleaseAuthority: commandAdmin, UsernamePolicyAdmin: commandAdmin}, Repositories: []suitemigration.SnapshotRepository{{ID: commandRepoID, Owner: commandOwner, Name: "demo", DefaultBranch: "main", CreatedAt: 1, UpdatedAt: 1}}, Moderation: suitemigration.SnapshotModeration{FinalStatuses: []suitemigration.SnapshotFinalStatus{{RepoID: commandRepoID, Status: "active"}}}, Username: suitemigration.SnapshotUsername{EscrowRelease: suitemigration.UsernameEscrowRelease{AllReleased: true, Contract: "inj1contract", Height: 100, BlockHash: strings.Repeat("6", 64), Denom: "inj", EscrowBalance: "0", EvidenceSHA256: strings.Repeat("7", 64)}}}
	raw, err := jsonMarshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonMarshal(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value)
	return buffer.Bytes(), err
}

package migration

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ImportedStateSchema is the offline export contract consumed after a V2
// import. The exporter/receipt runner must read all pages at one block_tag and
// write this file only after the registry reports a successful finalization.
const ImportedStateSchema = "igit.evm-v2.import-state.v2"

// ImportedState is deliberately transport-neutral. A future EVM reader can
// populate it from eth_call/logs without making this verifier depend on RPC.
type ImportedState struct {
	Schema           string               `json:"schema"`
	ImportScope      string               `json:"import_scope"`
	DeferredSections []string             `json:"deferred_sections"`
	PlanSHA256       string               `json:"plan_sha256"`
	SnapshotSHA256   string               `json:"snapshot_sha256"`
	SessionID        string               `json:"session_id"`
	ImportCommitment string               `json:"import_commitment"`
	ChainID          uint64               `json:"chain_id"`
	Registry         string               `json:"registry"`
	Controller       string               `json:"controller,omitempty"`
	BlockTag         string               `json:"block_tag"`
	Finalization     ImportedFinalization `json:"finalization"`
	Progress         ImportedProgress     `json:"import_progress"`
	Repositories     []ImportedRepository `json:"repositories"`
}

// ImportedFinalization records the mined receipt evidence that authorized the
// fixed block snapshot. It is still an input contract, not a cryptographic
// proof, until the exporter and its durable receipt journal are audited.
type ImportedFinalization struct {
	TransactionHash string `json:"transaction_hash"`
	BlockHash       string `json:"block_hash"`
	Event           string `json:"event"`
	EventEmitter    string `json:"event_emitter"`
}

type ImportedProgress struct {
	Exists                 bool   `json:"exists"`
	Active                 bool   `json:"active"`
	Finalized              bool   `json:"finalized"`
	NextSequence           uint64 `json:"next_sequence"`
	ExpectedBatches        uint64 `json:"expected_batches"`
	ImportedRepositories   uint64 `json:"imported_repositories"`
	ExpectedRepositories   uint64 `json:"expected_repositories"`
	ImportedRefs           uint64 `json:"imported_refs"`
	RemainingRefs          uint64 `json:"remaining_refs"`
	ImportedCollaborators  uint64 `json:"imported_collaborators"`
	RemainingCollaborators uint64 `json:"remaining_collaborators"`
	IncompleteRepositories uint64 `json:"incomplete_repositories"`
}

type ImportedRepository struct {
	RepoID           string             `json:"repo_id"`
	Owner            string             `json:"owner"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	DefaultBranch    string             `json:"default_branch"`
	CreatedAt        uint64             `json:"created_at"`
	UpdatedAt        uint64             `json:"updated_at"`
	ModerationStatus string             `json:"moderation_status"`
	Refs             []PlanRef          `json:"refs"`
	Collaborators    []PlanCollaborator `json:"collaborators"`
}

// ReadImportedState reads one complete JSON document and rejects trailing
// data. It performs no network or chain access.
func ReadImportedState(path string) (*ImportedState, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("imported state path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read imported state: %w", err)
	}
	var state ImportedState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("decode imported state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("imported state contains trailing JSON data")
		}
		return nil, fmt.Errorf("read imported state trailer: %w", err)
	}
	return &state, nil
}

// MarshalImportedState emits stable, newline-terminated JSON suitable for
// hashing, review, and fixture generation by a future chain-state exporter.
func MarshalImportedState(state *ImportedState) ([]byte, error) {
	if state == nil {
		return nil, errors.New("imported state is nil")
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// VerifyImportedState compares a finalized V2 state export with the exact
// offline plan. It is intentionally fail-closed and does not infer omitted
// fields or tolerate duplicate records.
func VerifyImportedState(plan *Plan, state *ImportedState) error {
	if plan == nil {
		return errors.New("import plan is nil")
	}
	if state == nil {
		return errors.New("imported state is nil")
	}
	if err := validateManifestPlan(plan); err != nil {
		return fmt.Errorf("validate import plan: %w", err)
	}
	if state.Schema != ImportedStateSchema {
		return fmt.Errorf("unsupported imported state schema %q", state.Schema)
	}
	if state.ImportScope != plan.ImportScope {
		return fmt.Errorf("imported state import_scope %q does not match plan %q", state.ImportScope, plan.ImportScope)
	}
	if !equalStrings(state.DeferredSections, plan.DeferredSections) {
		return errors.New("imported state deferred_sections do not match the plan")
	}
	if err := validateFixedBlockTag(state.BlockTag); err != nil {
		return err
	}
	expectedSessionID := "0x" + strings.ToLower(plan.Snapshot.SHA256)
	if state.SessionID != expectedSessionID {
		return fmt.Errorf("imported state session_id %q does not match expected %q", state.SessionID, expectedSessionID)
	}
	expectedCommitment := "0x" + strings.ToLower(plan.ImportCommitment)
	if state.ImportCommitment != expectedCommitment {
		return fmt.Errorf("imported state import_commitment %q does not match expected %q", state.ImportCommitment, expectedCommitment)
	}
	if err := validateImportedFinalization(plan, state.Finalization); err != nil {
		return err
	}
	if err := validateImportedProgress(plan, state.Progress); err != nil {
		return err
	}
	planDigest, err := PlanSHA256(plan)
	if err != nil {
		return fmt.Errorf("encode import plan: %w", err)
	}
	planDigestBytes, err := hex.DecodeString(planDigest)
	if err != nil {
		return fmt.Errorf("decode import plan SHA-256: %w", err)
	}
	if !equalDigest(state.PlanSHA256, planDigestBytes) {
		return fmt.Errorf("imported state plan_sha256 mismatch: expected %s", planDigest)
	}
	snapshotDigest := mustDecodeDigest(plan.Snapshot.SHA256)
	if !equalDigest(state.SnapshotSHA256, snapshotDigest) {
		return fmt.Errorf("imported state snapshot_sha256 mismatch: expected %s", plan.Snapshot.SHA256)
	}
	if state.ChainID != plan.Target.ChainID {
		return fmt.Errorf("imported state chain_id %d does not match target %d", state.ChainID, plan.Target.ChainID)
	}
	if state.Registry != plan.Target.Contract {
		return fmt.Errorf("imported state registry %q does not match target %q", state.Registry, plan.Target.Contract)
	}
	if state.Controller != plan.Target.Controller {
		return fmt.Errorf("imported state controller %q does not match target %q", state.Controller, plan.Target.Controller)
	}
	if len(state.Repositories) != len(plan.Repositories) {
		return fmt.Errorf("imported state repository count %d does not match plan %d", len(state.Repositories), len(plan.Repositories))
	}

	planned := make(map[string]PlanRepository, len(plan.Repositories))
	for _, repository := range plan.Repositories {
		planned[strings.ToLower(repository.RepoID)] = repository
	}
	seen := make(map[string]struct{}, len(state.Repositories))
	for index, actual := range state.Repositories {
		key := strings.ToLower(actual.RepoID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("imported state repository %d duplicates repo ID %s", index, actual.RepoID)
		}
		seen[key] = struct{}{}
		expected, exists := planned[key]
		if !exists {
			return fmt.Errorf("imported state contains unexpected repo ID %s", actual.RepoID)
		}
		if err := compareImportedRepository(expected, actual); err != nil {
			return fmt.Errorf("repo %s: %w", actual.RepoID, err)
		}
	}
	return nil
}

func validateImportedFinalization(plan *Plan, finalization ImportedFinalization) error {
	for _, value := range []struct {
		label string
		value string
	}{
		{label: "transaction_hash", value: finalization.TransactionHash},
		{label: "block_hash", value: finalization.BlockHash},
	} {
		if err := validateCanonicalHash32(value.label, value.value); err != nil {
			return fmt.Errorf("imported state finalization: %w", err)
		}
	}
	wantEvent := "ImportFinalized"
	wantEmitter := plan.Target.Contract
	if plan.Target.Controller != "" {
		wantEvent = "ImportPublished"
		wantEmitter = plan.Target.Controller
	}
	if finalization.Event != wantEvent {
		return fmt.Errorf("imported state finalization event %q does not match %q", finalization.Event, wantEvent)
	}
	if finalization.EventEmitter != wantEmitter {
		return fmt.Errorf("imported state finalization emitter %q does not match %q", finalization.EventEmitter, wantEmitter)
	}
	return nil
}

func validateImportedProgress(plan *Plan, progress ImportedProgress) error {
	if !progress.Exists || progress.Active || !progress.Finalized {
		return errors.New("imported state import_progress is not a finalized inactive session")
	}
	wantRepositories := uint64(len(plan.Repositories))
	wantRefs := uint64(plan.Summary.RefCount)
	wantCollaborators := uint64(plan.Summary.CollaboratorCount)
	wantBatches := uint64(len(plan.Batches))
	if progress.NextSequence != wantBatches ||
		progress.ExpectedBatches != wantBatches ||
		progress.ImportedRepositories != wantRepositories ||
		progress.ExpectedRepositories != wantRepositories ||
		progress.ImportedRefs != wantRefs ||
		progress.RemainingRefs != 0 ||
		progress.ImportedCollaborators != wantCollaborators ||
		progress.RemainingCollaborators != 0 ||
		progress.IncompleteRepositories != 0 {
		return fmt.Errorf("imported state import_progress counts do not match plan")
	}
	return nil
}

func validateCanonicalHash32(label, value string) error {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") || strings.ToLower(value) != value {
		return fmt.Errorf("%s must be a lowercase 0x-prefixed 32-byte hash", label)
	}
	raw, err := hex.DecodeString(value[2:])
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("%s must be a lowercase 0x-prefixed 32-byte hash", label)
	}
	allZero := true
	for _, byteValue := range raw {
		allZero = allZero && byteValue == 0
	}
	if allZero {
		return fmt.Errorf("%s must not be zero", label)
	}
	return nil
}

func validateFixedBlockTag(value string) error {
	if value == "" {
		return errors.New("imported state block_tag is required")
	}
	if !strings.HasPrefix(value, "0x") || len(value) == 2 {
		return errors.New("imported state block_tag must be a fixed hexadecimal block number")
	}
	digits := value[2:]
	if len(digits) > 1 && digits[0] == '0' {
		return errors.New("imported state block_tag must use canonical JSON-RPC quantity encoding")
	}
	for _, digit := range digits {
		if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') {
			return errors.New("imported state block_tag must be a lowercase hexadecimal JSON-RPC quantity")
		}
	}
	return nil
}

func compareImportedRepository(expected PlanRepository, actual ImportedRepository) error {
	if actual.RepoID != expected.RepoID ||
		actual.Owner != expected.Owner ||
		actual.Name != expected.Name ||
		actual.Description != expected.Description ||
		actual.DefaultBranch != expected.DefaultBranch ||
		actual.CreatedAt != expected.CreatedAt ||
		actual.UpdatedAt != expected.UpdatedAt ||
		actual.ModerationStatus != expected.ModerationStatus {
		return errors.New("repository identity or metadata differs from plan")
	}
	if len(actual.Refs) != len(expected.Refs) {
		return fmt.Errorf("ref count %d does not match plan %d", len(actual.Refs), len(expected.Refs))
	}
	expectedRefs := append([]PlanRef(nil), expected.Refs...)
	actualRefs := append([]PlanRef(nil), actual.Refs...)
	sort.Slice(expectedRefs, func(i, j int) bool { return expectedRefs[i].RefName < expectedRefs[j].RefName })
	sort.Slice(actualRefs, func(i, j int) bool { return actualRefs[i].RefName < actualRefs[j].RefName })
	for index := range expectedRefs {
		if expectedRefs[index].RefName != actualRefs[index].RefName ||
			expectedRefs[index].CommitSHA != actualRefs[index].CommitSHA ||
			expectedRefs[index].UpdatedAt != actualRefs[index].UpdatedAt ||
			expectedRefs[index].UpdatedBy != actualRefs[index].UpdatedBy ||
			!equalStrings(expectedRefs[index].PackURIs, actualRefs[index].PackURIs) {
			return fmt.Errorf("ref %s differs from plan", actualRefs[index].RefName)
		}
	}
	if len(actual.Collaborators) != len(expected.Collaborators) {
		return fmt.Errorf("collaborator count %d does not match plan %d", len(actual.Collaborators), len(expected.Collaborators))
	}
	expectedCollaborators := append([]PlanCollaborator(nil), expected.Collaborators...)
	actualCollaborators := append([]PlanCollaborator(nil), actual.Collaborators...)
	sort.Slice(expectedCollaborators, func(i, j int) bool { return expectedCollaborators[i].Address < expectedCollaborators[j].Address })
	sort.Slice(actualCollaborators, func(i, j int) bool { return actualCollaborators[i].Address < actualCollaborators[j].Address })
	for index := range expectedCollaborators {
		if expectedCollaborators[index] != actualCollaborators[index] {
			return fmt.Errorf("collaborator %s differs from plan", actualCollaborators[index].Address)
		}
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalDigest(value string, expected []byte) bool {
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x"))
	return err == nil && len(raw) == len(expected) && string(raw) == string(expected)
}

func mustDecodeDigest(value string) []byte {
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x"))
	if err != nil {
		return nil
	}
	return raw
}

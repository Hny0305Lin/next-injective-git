package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerifyImportedStateAcceptsExactStateInAnyEnumerationOrder(t *testing.T) {
	plan := verificationPlan(t)
	state := importedStateForPlan(t, plan)

	state.Repositories[0], state.Repositories[1] = state.Repositories[1], state.Repositories[0]
	for index := range state.Repositories {
		if state.Repositories[index].Name != "alpha" {
			continue
		}
		reverseRefs(state.Repositories[index].Refs)
		reverseCollaborators(state.Repositories[index].Collaborators)
	}
	if err := VerifyImportedState(plan, state); err != nil {
		t.Fatalf("verify exact imported state: %v", err)
	}
}

func TestMarshalAndReadImportedStateAreDeterministicAndStrict(t *testing.T) {
	state := importedStateForPlan(t, verificationPlan(t))
	first, err := MarshalImportedState(state)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalImportedState(state)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same imported state produced non-deterministic JSON")
	}
	if len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatal("imported state JSON is not newline terminated")
	}

	path := filepath.Join(t.TempDir(), "imported-state.json")
	if err := os.WriteFile(path, first, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadImportedState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, state) {
		t.Fatalf("decoded state differs:\n got: %#v\nwant: %#v", decoded, state)
	}

	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{
			name: "trailing JSON",
			raw:  append(append([]byte(nil), first...), []byte("{}\n")...),
			want: "trailing JSON data",
		},
		{
			name: "unknown field",
			raw: bytes.Replace(
				first,
				[]byte("{\n"),
				[]byte("{\n  \"unexpected\": true,\n"),
				1,
			),
			want: "unknown field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.json")
			if err := os.WriteFile(path, tc.raw, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadImportedState(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestVerifyImportedStateRejectsIdentityAndContentMismatches(t *testing.T) {
	plan := verificationPlan(t)
	tests := []struct {
		name   string
		mutate func(*ImportedState)
		want   string
	}{
		{name: "schema", mutate: func(state *ImportedState) { state.Schema = "v0" }, want: "unsupported imported state schema"},
		{name: "scope", mutate: func(state *ImportedState) { state.ImportScope = "all" }, want: "import_scope"},
		{name: "deferred sections", mutate: func(state *ImportedState) { state.DeferredSections = state.DeferredSections[:1] }, want: "deferred_sections"},
		{name: "plan hash", mutate: func(state *ImportedState) { state.PlanSHA256 = strings.Repeat("0", 64) }, want: "plan_sha256 mismatch"},
		{name: "snapshot hash", mutate: func(state *ImportedState) { state.SnapshotSHA256 = strings.Repeat("0", 64) }, want: "snapshot_sha256 mismatch"},
		{name: "chain", mutate: func(state *ImportedState) { state.ChainID++ }, want: "chain_id"},
		{name: "registry", mutate: func(state *ImportedState) { state.Registry = "0x4444444444444444444444444444444444444444" }, want: "registry"},
		{name: "controller", mutate: func(state *ImportedState) { state.Controller = "" }, want: "controller"},
		{name: "missing block tag", mutate: func(state *ImportedState) { state.BlockTag = "" }, want: "block_tag is required"},
		{name: "moving block tag", mutate: func(state *ImportedState) { state.BlockTag = "latest" }, want: "fixed hexadecimal block number"},
		{name: "uppercase block tag", mutate: func(state *ImportedState) { state.BlockTag = "0xABC" }, want: "lowercase hexadecimal"},
		{name: "noncanonical block tag", mutate: func(state *ImportedState) { state.BlockTag = "0x01" }, want: "canonical JSON-RPC quantity"},
		{name: "metadata", mutate: func(state *ImportedState) { state.Repositories[0].Description += " changed" }, want: "metadata differs"},
		{name: "ref", mutate: func(state *ImportedState) { state.Repositories[0].Refs[0].CommitSHA = strings.Repeat("f", 40) }, want: "ref "},
		{name: "collaborator", mutate: func(state *ImportedState) { state.Repositories[0].Collaborators[0].Role = "maintainer" }, want: "collaborator "},
		{name: "missing repository", mutate: func(state *ImportedState) { state.Repositories = state.Repositories[:1] }, want: "repository count"},
		{
			name: "duplicate repository",
			mutate: func(state *ImportedState) {
				state.Repositories[1] = cloneImportedRepository(state.Repositories[0])
			},
			want: "duplicates repo ID",
		},
		{
			name: "unexpected repository",
			mutate: func(state *ImportedState) {
				state.Repositories[1].RepoID = "0x" + strings.Repeat("f", 64)
			},
			want: "unexpected repo ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := importedStateForPlan(t, plan)
			tc.mutate(state)
			err := VerifyImportedState(plan, state)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestImportedStateHelpersRejectNilAndInvalidPlan(t *testing.T) {
	if _, err := MarshalImportedState(nil); err == nil {
		t.Fatal("nil imported state was accepted")
	}
	if err := VerifyImportedState(nil, &ImportedState{}); err == nil {
		t.Fatal("nil plan was accepted")
	}
	if err := VerifyImportedState(&Plan{}, nil); err == nil {
		t.Fatal("nil imported state was accepted")
	}
	if _, err := ReadImportedState(""); err == nil {
		t.Fatal("empty imported state path was accepted")
	}

	plan := verificationPlan(t)
	plan.Summary.RepositoryCount++
	err := VerifyImportedState(plan, importedStateForPlan(t, plan))
	if err == nil || !strings.Contains(err.Error(), "validate import plan") {
		t.Fatalf("error = %v, want plan validation failure", err)
	}
}

func verificationPlan(t *testing.T) *Plan {
	t.Helper()
	paths := writeSnapshotFixture(t, validSnapshot())
	plan, err := BuildPlan(Options{
		SnapshotPath:       paths.snapshot,
		HashPath:           paths.hash,
		TargetChainID:      1439,
		TargetContract:     testTargetContract,
		ControllerContract: testControllerContract,
		BatchSize:          2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func importedStateForPlan(t *testing.T, plan *Plan) *ImportedState {
	t.Helper()
	planRaw, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	planDigest := sha256.Sum256(planRaw)
	state := &ImportedState{
		Schema:           ImportedStateSchema,
		ImportScope:      plan.ImportScope,
		DeferredSections: append([]string(nil), plan.DeferredSections...),
		PlanSHA256:       hex.EncodeToString(planDigest[:]),
		SnapshotSHA256:   plan.Snapshot.SHA256,
		SessionID:        "0x" + plan.Snapshot.SHA256,
		ImportCommitment: "0x" + plan.ImportCommitment,
		ChainID:          plan.Target.ChainID,
		Registry:         plan.Target.Contract,
		Controller:       plan.Target.Controller,
		BlockTag:         "0xabc",
		Finalization: ImportedFinalization{
			TransactionHash: "0x" + strings.Repeat("a", 64),
			BlockHash:       "0x" + strings.Repeat("b", 64),
			Event:           "ImportPublished",
			EventEmitter:    plan.Target.Controller,
		},
		Progress: ImportedProgress{
			Exists: true, Finalized: true,
			NextSequence: uint64(len(plan.Batches)), ExpectedBatches: uint64(len(plan.Batches)),
			ImportedRepositories: uint64(len(plan.Repositories)), ExpectedRepositories: uint64(len(plan.Repositories)),
			ImportedRefs: uint64(plan.Summary.RefCount), ImportedCollaborators: uint64(plan.Summary.CollaboratorCount),
		},
	}
	for _, repository := range plan.Repositories {
		state.Repositories = append(state.Repositories, ImportedRepository{
			RepoID:           repository.RepoID,
			Owner:            repository.Owner,
			Name:             repository.Name,
			Description:      repository.Description,
			DefaultBranch:    repository.DefaultBranch,
			CreatedAt:        repository.CreatedAt,
			UpdatedAt:        repository.UpdatedAt,
			ModerationStatus: repository.ModerationStatus,
			Refs:             clonePlanRefs(repository.Refs),
			Collaborators:    append([]PlanCollaborator(nil), repository.Collaborators...),
		})
	}
	return state
}

func cloneImportedRepository(repository ImportedRepository) ImportedRepository {
	repository.Refs = clonePlanRefs(repository.Refs)
	repository.Collaborators = append([]PlanCollaborator(nil), repository.Collaborators...)
	return repository
}

func clonePlanRefs(refs []PlanRef) []PlanRef {
	cloned := append([]PlanRef(nil), refs...)
	for index := range cloned {
		cloned[index].PackURIs = append([]string(nil), cloned[index].PackURIs...)
	}
	return cloned
}

func reverseRefs(values []PlanRef) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseCollaborators(values []PlanCollaborator) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

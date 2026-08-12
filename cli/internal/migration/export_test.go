package migration

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

func TestReadPlanStrictlyRevalidatesOneDocument(t *testing.T) {
	plan := verificationPlan(t)
	raw, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	write := func(t *testing.T, raw []byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	decoded, err := ReadPlan(write(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, plan) {
		t.Fatalf("decoded plan differs:\n got: %#v\nwant: %#v", decoded, plan)
	}

	tampered := *plan
	tampered.Summary.RefCount++
	tamperedRaw, err := MarshalPlan(&tampered)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "trailing JSON", raw: append(append([]byte(nil), raw...), []byte("{}\n")...), want: "trailing JSON"},
		{
			name: "unknown field",
			raw:  bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"unknown\": true,\n"), 1),
			want: "unknown field",
		},
		{name: "tampered invariant", raw: tamperedRaw, want: "validate import plan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadPlan(write(t, tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExportImportedStatePinsFinalizeReceiptBlockAndVerifiesExactState(t *testing.T) {
	plan := verificationPlan(t)
	reader := newFakeImportedStateReader(t, plan)
	state, err := ExportImportedState(context.Background(), plan, reader.receipt.TransactionHash, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyImportedState(plan, state); err != nil {
		t.Fatal(err)
	}
	if state.BlockTag != "0x2a" || state.Finalization.BlockHash != reader.blockHash {
		t.Fatalf("state finalization pin = %#v", state.Finalization)
	}
	if reader.blockCalls != 2 {
		t.Fatalf("block identity checks = %d, want 2", reader.blockCalls)
	}
	for index, blockTag := range reader.stateBlockTags {
		if blockTag != "0x2a" {
			t.Fatalf("state call %d used block tag %q", index, blockTag)
		}
	}
	if len(reader.stateBlockTags) != 1+3*len(plan.Repositories) {
		t.Fatalf("state calls = %d, want %d", len(reader.stateBlockTags), 1+3*len(plan.Repositories))
	}
}

func TestExportImportedStateAcceptsDirectRegistryFinalizationEvidence(t *testing.T) {
	paths := writeSnapshotFixture(t, validSnapshot())
	plan, err := BuildPlan(Options{
		SnapshotPath: paths.snapshot, HashPath: paths.hash,
		TargetChainID: 1439, TargetContract: testTargetContract, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := newFakeImportedStateReader(t, plan)
	state, err := ExportImportedState(context.Background(), plan, reader.receipt.TransactionHash, reader)
	if err != nil {
		t.Fatal(err)
	}
	if state.Finalization.Event != "ImportFinalized" || state.Finalization.EventEmitter != plan.Target.Contract {
		t.Fatalf("direct finalization evidence = %#v", state.Finalization)
	}
}

func TestExportImportedStateRejectsReceiptProgressStateAndReorgMismatches(t *testing.T) {
	plan := verificationPlan(t)
	tests := []struct {
		name   string
		mutate func(*fakeImportedStateReader)
		want   string
	}{
		{name: "pending receipt", mutate: func(reader *fakeImportedStateReader) { reader.receipt = nil }, want: "not mined"},
		{name: "reverted receipt", mutate: func(reader *fakeImportedStateReader) { reader.receipt.Status = "0x0" }, want: "status"},
		{name: "wrong receipt target", mutate: func(reader *fakeImportedStateReader) { reader.receipt.To = testTargetContract }, want: "receipt target"},
		{name: "missing event", mutate: func(reader *fakeImportedStateReader) { reader.receipt.Logs = nil }, want: "does not contain ImportPublished"},
		{
			name: "wrong commitment topic",
			mutate: func(reader *fakeImportedStateReader) {
				reader.receipt.Logs[0].Topics[2] = "0x" + strings.Repeat("0", 64)
			},
			want: "does not contain ImportPublished",
		},
		{
			name: "log from another block",
			mutate: func(reader *fakeImportedStateReader) {
				reader.receipt.Logs[0].BlockHash = "0x" + strings.Repeat("c", 64)
			},
			want: "does not contain ImportPublished",
		},
		{name: "wrong chain", mutate: func(reader *fakeImportedStateReader) { reader.chainID++ }, want: "chain ID"},
		{name: "not finalized", mutate: func(reader *fakeImportedStateReader) { reader.progress.Finalized = false }, want: "not a finalized inactive session"},
		{name: "wrong counts", mutate: func(reader *fakeImportedStateReader) { reader.progress.ImportedRefs++ }, want: "counts do not match plan"},
		{name: "receipt block reorged before export", mutate: func(reader *fakeImportedStateReader) { reader.blockHash = "0x" + strings.Repeat("c", 64) }, want: "does not match finalize receipt"},
		{name: "receipt block reorged during export", mutate: func(reader *fakeImportedStateReader) { reader.secondBlockHash = "0x" + strings.Repeat("c", 64) }, want: "changed during export"},
		{
			name: "repository state mismatch",
			mutate: func(reader *fakeImportedStateReader) {
				for _, repository := range reader.repositories {
					repository.Description += " changed"
					break
				}
			},
			want: "metadata differs from plan",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := newFakeImportedStateReader(t, plan)
			tc.mutate(reader)
			_, err := ExportImportedState(context.Background(), plan, fakeFinalizeTx, reader)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

const (
	fakeFinalizeTx = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fakeBlockHash  = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeImportedStateReader struct {
	chainID         uint64
	receipt         *chain.EVMReceipt
	blockHash       string
	secondBlockHash string
	blockCalls      int
	progress        *chain.EVMImportProgress
	repositories    map[[32]byte]*chain.RepoInfo
	refs            map[[32]byte][]chain.RefInfo
	collaborators   map[[32]byte][]chain.CollaboratorInfo
	stateBlockTags  []string
}

func newFakeImportedStateReader(t *testing.T, plan *Plan) *fakeImportedStateReader {
	t.Helper()
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	finalize := manifest.Transactions[len(manifest.Transactions)-1]
	topics := []string{finalize.ExpectedTopic, "0x" + plan.Snapshot.SHA256}
	data := "0x"
	if plan.Target.Controller != "" {
		topics = append(topics, "0x"+plan.ImportCommitment)
	} else {
		topics = append(topics, "0x"+plan.Snapshot.SHA256)
		arguments := abi.Arguments{
			{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
			{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
		}
		encoded, err := arguments.Pack(
			big.NewInt(int64(len(plan.Repositories))),
			big.NewInt(int64(plan.Summary.RefCount)),
			big.NewInt(int64(plan.Summary.CollaboratorCount)),
			big.NewInt(int64(len(plan.Batches))),
		)
		if err != nil {
			t.Fatal(err)
		}
		data = "0x" + hex.EncodeToString(encoded)
	}
	reader := &fakeImportedStateReader{
		chainID:   plan.Target.ChainID,
		blockHash: fakeBlockHash,
		progress: &chain.EVMImportProgress{
			Exists: true, Finalized: true,
			NextSequence: uint64(len(plan.Batches)), ExpectedBatches: uint64(len(plan.Batches)),
			ImportedRepositories: uint64(len(plan.Repositories)), ExpectedRepositories: uint64(len(plan.Repositories)),
			ImportedRefs: uint64(plan.Summary.RefCount), ImportedCollaborators: uint64(plan.Summary.CollaboratorCount),
		},
		repositories:  make(map[[32]byte]*chain.RepoInfo),
		refs:          make(map[[32]byte][]chain.RefInfo),
		collaborators: make(map[[32]byte][]chain.CollaboratorInfo),
	}
	reader.receipt = &chain.EVMReceipt{
		TransactionHash: fakeFinalizeTx,
		BlockNumber:     "0x2a",
		BlockHash:       fakeBlockHash,
		To:              finalize.To,
		Status:          "0x1",
		Logs: []chain.EVMLog{{
			Address: finalize.EventEmitter, Topics: topics, Data: data,
			BlockNumber: "0x2a", BlockHash: fakeBlockHash, TransactionHash: fakeFinalizeTx,
		}},
	}
	for _, repository := range plan.Repositories {
		repoID, err := manifestBytes32("repo ID", repository.RepoID)
		if err != nil {
			t.Fatal(err)
		}
		reader.repositories[repoID] = &chain.RepoInfo{
			Owner: repository.Owner, Name: repository.Name, Description: repository.Description,
			DefaultBranch: repository.DefaultBranch, CreatedAt: repository.CreatedAt,
			UpdatedAt: repository.UpdatedAt, ModerationStatus: repository.ModerationStatus,
		}
		for _, ref := range repository.Refs {
			reader.refs[repoID] = append(reader.refs[repoID], chain.RefInfo{
				RefName: ref.RefName, CommitSha: ref.CommitSHA, PackURIs: append([]string(nil), ref.PackURIs...),
				UpdatedAt: ref.UpdatedAt, UpdatedBy: ref.UpdatedBy,
			})
		}
		for _, collaborator := range repository.Collaborators {
			reader.collaborators[repoID] = append(reader.collaborators[repoID], chain.CollaboratorInfo{
				Address: collaborator.Address,
				Role:    collaborator.Role,
			})
		}
	}
	return reader
}

func (reader *fakeImportedStateReader) ChainID(context.Context) (uint64, error) {
	return reader.chainID, nil
}

func (reader *fakeImportedStateReader) TransactionReceipt(context.Context, string) (*chain.EVMReceipt, error) {
	return reader.receipt, nil
}

func (reader *fakeImportedStateReader) BlockByNumber(_ context.Context, blockTag string) (*chain.EVMBlock, error) {
	reader.blockCalls++
	hash := reader.blockHash
	if reader.blockCalls > 1 && reader.secondBlockHash != "" {
		hash = reader.secondBlockHash
	}
	return &chain.EVMBlock{Number: blockTag, Hash: hash}, nil
}

func (reader *fakeImportedStateReader) ImportProgressAt(_ context.Context, _ [32]byte, blockTag string) (*chain.EVMImportProgress, error) {
	reader.stateBlockTags = append(reader.stateBlockTags, blockTag)
	return reader.progress, nil
}

func (reader *fakeImportedStateReader) GetRepoByIDAt(_ context.Context, repoID [32]byte, blockTag string) (*chain.RepoInfo, error) {
	reader.stateBlockTags = append(reader.stateBlockTags, blockTag)
	return reader.repositories[repoID], nil
}

func (reader *fakeImportedStateReader) ListRefsByIDAt(_ context.Context, repoID [32]byte, blockTag string) ([]chain.RefInfo, error) {
	reader.stateBlockTags = append(reader.stateBlockTags, blockTag)
	return reader.refs[repoID], nil
}

func (reader *fakeImportedStateReader) ListCollaboratorsByIDAt(_ context.Context, repoID [32]byte, blockTag string) ([]chain.CollaboratorInfo, error) {
	reader.stateBlockTags = append(reader.stateBlockTags, blockTag)
	return reader.collaborators[repoID], nil
}

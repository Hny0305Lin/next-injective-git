package migration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

// ImportedStateReader is the read-only boundary used by the post-import
// exporter. Implementations must evaluate every state call at the supplied
// numeric block tag and must not fall back to CosmWasm.
type ImportedStateReader interface {
	ChainID(context.Context) (uint64, error)
	TransactionReceipt(context.Context, string) (*chain.EVMReceipt, error)
	BlockByNumber(context.Context, string) (*chain.EVMBlock, error)
	ImportProgressAt(context.Context, [32]byte, string) (*chain.EVMImportProgress, error)
	GetRepoByIDAt(context.Context, [32]byte, string) (*chain.RepoInfo, error)
	ListRefsByIDAt(context.Context, [32]byte, string) ([]chain.RefInfo, error)
	ListCollaboratorsByIDAt(context.Context, [32]byte, string) ([]chain.CollaboratorInfo, error)
}

// ReadPlan loads one complete, strictly decoded plan and revalidates all
// commitment, ordering, ABI-bound, and count invariants before it is used by a
// chain reader.
func ReadPlan(path string) (*Plan, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("import plan path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read import plan: %w", err)
	}
	var plan Plan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return nil, fmt.Errorf("decode import plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("import plan contains trailing JSON data")
		}
		return nil, fmt.Errorf("read import plan trailer: %w", err)
	}
	if err := validateManifestPlan(&plan); err != nil {
		return nil, fmt.Errorf("validate import plan: %w", err)
	}
	return &plan, nil
}

// ExportImportedState verifies a mined finalization receipt and then reads the
// complete V2 core state at that receipt's exact block. It performs no writes,
// signing, or broadcasting. A block hash check before and after pagination
// rejects a reorg while the export is in progress.
func ExportImportedState(
	ctx context.Context,
	plan *Plan,
	finalizeTx string,
	reader ImportedStateReader,
) (*ImportedState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateManifestPlan(plan); err != nil {
		return nil, fmt.Errorf("validate import plan: %w", err)
	}
	if reader == nil {
		return nil, errors.New("imported state reader is nil")
	}
	finalizeTx = strings.ToLower(strings.TrimSpace(finalizeTx))
	if err := validateCanonicalHash32("finalize transaction hash", finalizeTx); err != nil {
		return nil, err
	}
	receipt, err := reader.TransactionReceipt(ctx, finalizeTx)
	if err != nil {
		return nil, fmt.Errorf("read finalize transaction receipt: %w", err)
	}
	if receipt == nil {
		return nil, errors.New("finalize transaction is not mined")
	}
	if strings.ToLower(strings.TrimSpace(receipt.TransactionHash)) != finalizeTx {
		return nil, fmt.Errorf("receipt transaction hash %q does not match requested %q", receipt.TransactionHash, finalizeTx)
	}
	if strings.TrimSpace(receipt.Status) != "0x1" {
		return nil, fmt.Errorf("finalize transaction receipt status %q is not 0x1", receipt.Status)
	}
	blockTag, err := canonicalReceiptBlockTag(receipt.BlockNumber)
	if err != nil {
		return nil, err
	}
	blockHash := strings.ToLower(strings.TrimSpace(receipt.BlockHash))
	if err := validateCanonicalHash32("finalize receipt block hash", blockHash); err != nil {
		return nil, err
	}
	expectedTarget := plan.Target.Contract
	if plan.Target.Controller != "" {
		expectedTarget = plan.Target.Controller
	}
	to, err := chain.NormalizeEVMAddress(receipt.To)
	if err != nil || to != expectedTarget {
		return nil, fmt.Errorf("finalize receipt target %q does not match expected %q", receipt.To, expectedTarget)
	}
	if err := validateFinalizationLog(plan, receipt.Logs, finalizeTx, blockTag, blockHash); err != nil {
		return nil, err
	}

	before, err := reader.BlockByNumber(ctx, blockTag)
	if err != nil {
		return nil, fmt.Errorf("pin finalize block: %w", err)
	}
	if err := validatePinnedBlock(before, blockTag, blockHash); err != nil {
		return nil, err
	}
	chainID, err := reader.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read EVM chain ID: %w", err)
	}
	if chainID != plan.Target.ChainID {
		return nil, fmt.Errorf("EVM chain ID %d does not match plan target %d", chainID, plan.Target.ChainID)
	}

	sessionID, err := manifestBytes32("session ID", plan.Snapshot.SHA256)
	if err != nil {
		return nil, err
	}
	progress, err := reader.ImportProgressAt(ctx, sessionID, blockTag)
	if err != nil {
		return nil, fmt.Errorf("read import progress at %s: %w", blockTag, err)
	}
	planDigest, err := PlanSHA256(plan)
	if err != nil {
		return nil, fmt.Errorf("hash import plan: %w", err)
	}
	state := &ImportedState{
		Schema:           ImportedStateSchema,
		ImportScope:      plan.ImportScope,
		DeferredSections: append([]string(nil), plan.DeferredSections...),
		PlanSHA256:       planDigest,
		SnapshotSHA256:   plan.Snapshot.SHA256,
		SessionID:        "0x" + strings.ToLower(plan.Snapshot.SHA256),
		ImportCommitment: "0x" + strings.ToLower(plan.ImportCommitment),
		ChainID:          chainID,
		Registry:         plan.Target.Contract,
		Controller:       plan.Target.Controller,
		BlockTag:         blockTag,
		Finalization: ImportedFinalization{
			TransactionHash: finalizeTx,
			BlockHash:       blockHash,
			Event:           finalizationEventName(plan),
			EventEmitter:    expectedTarget,
		},
		Progress: importedProgress(progress),
	}

	for _, planned := range plan.Repositories {
		repoID, err := manifestBytes32("repo ID", planned.RepoID)
		if err != nil {
			return nil, err
		}
		repository, err := reader.GetRepoByIDAt(ctx, repoID, blockTag)
		if err != nil {
			return nil, fmt.Errorf("read repo %s at %s: %w", planned.RepoID, blockTag, err)
		}
		if repository == nil {
			return nil, fmt.Errorf("read repo %s returned nil", planned.RepoID)
		}
		owner, err := chain.NormalizeEVMAddress(repository.Owner)
		if err != nil {
			return nil, fmt.Errorf("repo %s owner: %w", planned.RepoID, err)
		}
		refs, err := reader.ListRefsByIDAt(ctx, repoID, blockTag)
		if err != nil {
			return nil, fmt.Errorf("read refs for repo %s at %s: %w", planned.RepoID, blockTag, err)
		}
		collaborators, err := reader.ListCollaboratorsByIDAt(ctx, repoID, blockTag)
		if err != nil {
			return nil, fmt.Errorf("read collaborators for repo %s at %s: %w", planned.RepoID, blockTag, err)
		}
		imported, err := importedRepository(*repository, owner, refs, collaborators)
		if err != nil {
			return nil, fmt.Errorf("convert repo %s: %w", planned.RepoID, err)
		}
		imported.RepoID = planned.RepoID
		state.Repositories = append(state.Repositories, imported)
	}

	after, err := reader.BlockByNumber(ctx, blockTag)
	if err != nil {
		return nil, fmt.Errorf("recheck finalize block: %w", err)
	}
	if err := validatePinnedBlock(after, blockTag, blockHash); err != nil {
		return nil, fmt.Errorf("finalize block changed during export: %w", err)
	}
	if err := VerifyImportedState(plan, state); err != nil {
		return nil, fmt.Errorf("verify exported imported state: %w", err)
	}
	return state, nil
}

func canonicalReceiptBlockTag(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if err := validateFixedBlockTag(value); err != nil {
		return "", fmt.Errorf("finalize receipt block number: %w", err)
	}
	return value, nil
}

func validatePinnedBlock(block *chain.EVMBlock, blockTag, expectedHash string) error {
	if block == nil {
		return errors.New("pinned block is nil")
	}
	number, err := canonicalReceiptBlockTag(block.Number)
	if err != nil || number != blockTag {
		return fmt.Errorf("pinned block number %q does not match %s", block.Number, blockTag)
	}
	hash := strings.ToLower(strings.TrimSpace(block.Hash))
	if err := validateCanonicalHash32("pinned block hash", hash); err != nil {
		return err
	}
	if hash != expectedHash {
		return fmt.Errorf("pinned block hash %q does not match finalize receipt %q", hash, expectedHash)
	}
	return nil
}

func finalizationEventName(plan *Plan) string {
	if plan.Target.Controller != "" {
		return "ImportPublished"
	}
	return "ImportFinalized"
}

func validateFinalizationLog(plan *Plan, logs []chain.EVMLog, transactionHash, blockTag, blockHash string) error {
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		return fmt.Errorf("build finalization evidence: %w", err)
	}
	finalize := manifest.Transactions[len(manifest.Transactions)-1]
	sessionTopic := "0x" + strings.ToLower(plan.Snapshot.SHA256)
	commitmentTopic := "0x" + strings.ToLower(plan.ImportCommitment)
	for _, log := range logs {
		if log.Removed || len(log.Topics) < 2 {
			continue
		}
		logBlockTag, blockErr := canonicalReceiptBlockTag(log.BlockNumber)
		if blockErr != nil || logBlockTag != blockTag ||
			!equalHex(log.BlockHash, blockHash) || !equalHex(log.TransactionHash, transactionHash) {
			continue
		}
		emitter, err := chain.NormalizeEVMAddress(log.Address)
		if err != nil || emitter != finalize.EventEmitter {
			continue
		}
		if !equalHex(log.Topics[0], finalize.ExpectedTopic) || !equalHex(log.Topics[1], sessionTopic) {
			continue
		}
		if plan.Target.Controller != "" {
			data := strings.TrimSpace(log.Data)
			if len(log.Topics) != 3 || !equalHex(log.Topics[2], commitmentTopic) || (data != "" && data != "0x" && data != "0X") {
				continue
			}
			return nil
		}
		if len(log.Topics) != 3 || !equalHex(log.Topics[2], sessionTopic) {
			continue
		}
		if err := validateDirectFinalizationData(log.Data, plan); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("finalize receipt does not contain %s from %s", finalize.ExpectedEvent, finalize.EventEmitter)
}

func validateDirectFinalizationData(raw string, plan *Plan) error {
	data := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "0x"), "0X")
	if len(data) != 4*64 {
		return fmt.Errorf("ImportFinalized event data has %d hex digits, want %d", len(data), 4*64)
	}
	decoded, err := hex.DecodeString(data)
	if err != nil {
		return fmt.Errorf("decode ImportFinalized event data: %w", err)
	}
	arguments := abi.Arguments{
		{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
	}
	values, err := arguments.Unpack(decoded)
	if err != nil {
		return fmt.Errorf("decode ImportFinalized event counts: %w", err)
	}
	want := []uint64{uint64(len(plan.Repositories)), uint64(plan.Summary.RefCount), uint64(plan.Summary.CollaboratorCount), uint64(len(plan.Batches))}
	for index, value := range values {
		actual, ok := value.(*big.Int)
		if !ok || !actual.IsUint64() || actual.Uint64() != want[index] {
			return fmt.Errorf("ImportFinalized count %d does not match plan", index)
		}
	}
	return nil
}

func equalHex(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func importedProgress(progress *chain.EVMImportProgress) ImportedProgress {
	if progress == nil {
		return ImportedProgress{}
	}
	return ImportedProgress{
		Exists: progress.Exists, Active: progress.Active, Finalized: progress.Finalized,
		NextSequence: progress.NextSequence, ExpectedBatches: progress.ExpectedBatches,
		ImportedRepositories: progress.ImportedRepositories, ExpectedRepositories: progress.ExpectedRepositories,
		ImportedRefs: progress.ImportedRefs, RemainingRefs: progress.RemainingRefs,
		ImportedCollaborators: progress.ImportedCollaborators, RemainingCollaborators: progress.RemainingCollaborators,
		IncompleteRepositories: progress.IncompleteRepositories,
	}
}

func importedRepository(
	repository chain.RepoInfo,
	owner string,
	refs []chain.RefInfo,
	collaborators []chain.CollaboratorInfo,
) (ImportedRepository, error) {
	converted := ImportedRepository{
		Owner: owner, Name: repository.Name, Description: repository.Description,
		DefaultBranch: repository.DefaultBranch, CreatedAt: repository.CreatedAt,
		UpdatedAt: repository.UpdatedAt, ModerationStatus: repository.ModerationStatus,
	}
	for _, ref := range refs {
		updatedBy, err := chain.NormalizeEVMAddress(ref.UpdatedBy)
		if err != nil {
			return ImportedRepository{}, fmt.Errorf("ref %s updater: %w", ref.RefName, err)
		}
		converted.Refs = append(converted.Refs, PlanRef{
			RefName: ref.RefName, CommitSHA: ref.CommitSha, PackURIs: append([]string(nil), ref.PackURIs...),
			UpdatedAt: ref.UpdatedAt, UpdatedBy: updatedBy,
		})
	}
	for _, collaborator := range collaborators {
		address, err := chain.NormalizeEVMAddress(collaborator.Address)
		if err != nil {
			return ImportedRepository{}, fmt.Errorf("collaborator: %w", err)
		}
		converted.Collaborators = append(converted.Collaborators, PlanCollaborator{Address: address, Role: collaborator.Role})
	}
	return converted, nil
}

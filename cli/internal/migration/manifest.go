package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const TransactionManifestSchema = "igit.evm-v2.import-transaction-manifest.v3"

type TransactionManifest struct {
	Schema           string                `json:"schema"`
	CalldataReady    bool                  `json:"calldata_ready"`
	Signed           bool                  `json:"signed"`
	Broadcast        bool                  `json:"broadcast"`
	ImportScope      string                `json:"import_scope"`
	DeferredSections []string              `json:"deferred_sections"`
	PlanSHA256       string                `json:"plan_sha256"`
	SessionID        string                `json:"session_id"`
	ImportCommitment string                `json:"import_commitment"`
	Snapshot         PlanSnapshot          `json:"snapshot"`
	Target           PlanTarget            `json:"target"`
	Transactions     []ManifestTransaction `json:"transactions"`
	Summary          ManifestSummary       `json:"summary"`
}

type ManifestTransaction struct {
	Order         int    `json:"order"`
	Phase         string `json:"phase"`
	Sequence      *int   `json:"sequence,omitempty"`
	RepoID        string `json:"repo_id,omitempty"`
	PayloadSHA256 string `json:"payload_sha256,omitempty"`
	Function      string `json:"function"`
	ExpectedEvent string `json:"expected_event"`
	ExpectedTopic string `json:"expected_event_topic"`
	EventEmitter  string `json:"event_emitter"`
	To            string `json:"to"`
	Value         string `json:"value"`
	Data          string `json:"data"`
}

type ManifestSummary struct {
	RepositoryCount   int `json:"repository_count"`
	RefCount          int `json:"ref_count"`
	CollaboratorCount int `json:"collaborator_count"`
	BatchCount        int `json:"batch_count"`
	TransactionCount  int `json:"transaction_count"`
}

type manifestImportRef struct {
	RefName   string         `abi:"refName"`
	CommitSHA string         `abi:"commitSha"`
	PackURIs  []string       `abi:"packUris"`
	UpdatedAt uint64         `abi:"updatedAt"`
	UpdatedBy common.Address `abi:"updatedBy"`
}

type manifestImportCollaborator struct {
	Account common.Address `abi:"account"`
	Role    uint8          `abi:"role"`
}

func importRepoPayloadHash(repo PlanRepository) string {
	arguments := abi.Arguments{
		{Type: mustABIType("bytes32", nil)}, {Type: mustABIType("address", nil)},
		{Type: mustABIType("string", nil)}, {Type: mustABIType("string", nil)},
		{Type: mustABIType("string", nil)}, {Type: mustABIType("uint8", nil)},
		{Type: mustABIType("uint64", nil)}, {Type: mustABIType("uint64", nil)},
		{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
	}
	repoID, err := manifestBytes32("repo ID", repo.RepoID)
	if err != nil {
		panic(err)
	}
	encoded, err := arguments.Pack(
		repoID,
		common.HexToAddress(repo.Owner),
		repo.Name,
		repo.Description,
		repo.DefaultBranch,
		importModerationStatusCode(repo.ModerationStatus),
		repo.CreatedAt,
		repo.UpdatedAt,
		new(big.Int).SetUint64(uint64(len(repo.Refs))),
		new(big.Int).SetUint64(uint64(len(repo.Collaborators))),
	)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func importModerationStatusCode(status string) uint8 {
	switch status {
	case "active":
		return 0
	case "frozen":
		return 1
	case "delisted":
		return 2
	default:
		// Both BuildPlan and validateManifestPlan reject unsupported statuses.
		return ^uint8(0)
	}
}

func importRefsPayloadHash(refs []PlanRef) string {
	values := make([]manifestImportRef, 0, len(refs))
	for _, ref := range refs {
		values = append(values, manifestImportRef{
			RefName: ref.RefName, CommitSHA: ref.CommitSHA,
			PackURIs: append([]string(nil), ref.PackURIs...), UpdatedAt: ref.UpdatedAt,
			UpdatedBy: common.HexToAddress(ref.UpdatedBy),
		})
	}
	tuple := mustABIType("tuple[]", []abi.ArgumentMarshaling{
		{Name: "refName", Type: "string"}, {Name: "commitSha", Type: "string"},
		{Name: "packUris", Type: "string[]"}, {Name: "updatedAt", Type: "uint64"},
		{Name: "updatedBy", Type: "address"},
	})
	encoded, err := (abi.Arguments{{Type: tuple}}).Pack(values)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func importCollaboratorsPayloadHash(collaborators []PlanCollaborator) string {
	values := make([]manifestImportCollaborator, 0, len(collaborators))
	for _, collaborator := range collaborators {
		role := uint8(2)
		if collaborator.Role == "maintainer" {
			role = 1
		}
		values = append(values, manifestImportCollaborator{
			Account: common.HexToAddress(collaborator.Address), Role: role,
		})
	}
	tuple := mustABIType("tuple[]", []abi.ArgumentMarshaling{
		{Name: "account", Type: "address"}, {Name: "role", Type: "uint8"},
	})
	encoded, err := (abi.Arguments{{Type: tuple}}).Pack(values)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// ImportCommitment mirrors RepoRegistryV2ImportController's ordered rolling
// SHA-256 commitment. It is deliberately offline: the plan, controller
// calldata, and a future receipt journal can independently reproduce it.
func ImportCommitment(plan *Plan) (string, error) {
	if plan == nil {
		return "", errors.New("import plan is nil")
	}
	snapshotHash, err := manifestBytes32("snapshot SHA-256", plan.Snapshot.SHA256)
	if err != nil {
		return "", err
	}
	registry, err := normalizeNonZeroAddress(plan.Target.Contract)
	if err != nil {
		return "", fmt.Errorf("target registry: %w", err)
	}
	seedArgs := abi.Arguments{
		{Type: mustABIType("string", nil)},
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("address", nil)},
	}
	seed, err := seedArgs.Pack(BatchCommitmentDomain, snapshotHash, common.HexToAddress(registry))
	if err != nil {
		return "", fmt.Errorf("encode import commitment seed: %w", err)
	}
	current := sha256.Sum256(seed)
	stepArgs := abi.Arguments{
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("bytes32", nil)},
	}
	for index, batch := range plan.Batches {
		if batch.Sequence != index {
			return "", fmt.Errorf("batch %d sequence is %d; want %d", index, batch.Sequence, index)
		}
		repoID, err := manifestBytes32("batch repo ID", batch.RepoID)
		if err != nil {
			return "", err
		}
		payloadHash, err := manifestBytes32("batch payload SHA-256", batch.PayloadSHA256)
		if err != nil {
			return "", err
		}
		encoded, err := stepArgs.Pack(
			current,
			new(big.Int).SetUint64(uint64(batch.Sequence)),
			crypto.Keccak256Hash([]byte(batch.Kind)),
			repoID,
			payloadHash,
		)
		if err != nil {
			return "", fmt.Errorf("encode import commitment batch %d: %w", index, err)
		}
		current = sha256.Sum256(encoded)
	}
	return hex.EncodeToString(current[:]), nil
}

func manifestTarget(plan *Plan) string {
	if plan.Target.Controller != "" {
		return plan.Target.Controller
	}
	return plan.Target.Contract
}

// BuildTransactionManifest turns a validated offline plan into deterministic
// unsigned calldata. It deliberately performs no RPC, signing, nonce, gas, or
// broadcasting work; those belong to a separately reviewed execution step.
func BuildTransactionManifest(plan *Plan) (*TransactionManifest, error) {
	if err := validateManifestPlan(plan); err != nil {
		return nil, err
	}
	planDigest, err := PlanSHA256(plan)
	if err != nil {
		return nil, fmt.Errorf("encode source plan: %w", err)
	}
	sessionID, err := manifestBytes32("snapshot SHA-256", plan.Snapshot.SHA256)
	if err != nil {
		return nil, err
	}
	sourceHeight, err := strconv.ParseUint(plan.Snapshot.Height, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot height %q: %w", plan.Snapshot.Height, err)
	}

	manifest := &TransactionManifest{
		Schema:           TransactionManifestSchema,
		CalldataReady:    true,
		Signed:           false,
		Broadcast:        false,
		ImportScope:      plan.ImportScope,
		DeferredSections: append([]string(nil), plan.DeferredSections...),
		PlanSHA256:       planDigest,
		SessionID:        "0x" + hex.EncodeToString(sessionID[:]),
		ImportCommitment: "0x" + plan.ImportCommitment,
		Snapshot:         plan.Snapshot,
		Target:           plan.Target,
		Summary: ManifestSummary{
			RepositoryCount:   plan.Summary.RepositoryCount,
			RefCount:          plan.Summary.RefCount,
			CollaboratorCount: plan.Summary.CollaboratorCount,
			BatchCount:        plan.Summary.BatchCount,
		},
	}

	createData, err := encodeCreateImportSession(plan, sessionID, sourceHeight)
	if err != nil {
		return nil, err
	}
	createFunction := "createImportSession(bytes32,string,address,uint64,uint256,uint256,uint256,uint256)"
	createEvent := "ImportSessionCreated"
	createTopic := manifestEventTopic("ImportSessionCreated(bytes32,bytes32,string,address,uint64,uint256,uint256,uint256,uint256)")
	if plan.Target.Controller != "" {
		createFunction = "createImportSession(bytes32,string,address,uint64,uint256,uint256,uint256,uint256,bytes32)"
		createEvent = "ImportSessionStarted"
		createTopic = manifestEventTopic("ImportSessionStarted(bytes32,bytes32,bytes32,uint256)")
	}
	manifest.Transactions = append(manifest.Transactions, ManifestTransaction{
		Order: 0, Phase: "create_session",
		Function:      createFunction,
		ExpectedEvent: createEvent,
		ExpectedTopic: createTopic,
		EventEmitter:  manifestTarget(plan),
		To:            manifestTarget(plan),
		Value:         "0x0",
		Data:          createData,
	})

	repositories := make(map[string]PlanRepository, len(plan.Repositories))
	for _, repository := range plan.Repositories {
		repositories[strings.ToLower(repository.RepoID)] = repository
	}
	for index, batch := range plan.Batches {
		repository, ok := repositories[strings.ToLower(batch.RepoID)]
		if !ok {
			return nil, fmt.Errorf("batch %d references unknown repo ID %s", batch.Sequence, batch.RepoID)
		}
		transaction, err := encodeManifestBatch(plan, sessionID, repository, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d: %w", batch.Sequence, err)
		}
		transaction.Order = index + 1
		manifest.Transactions = append(manifest.Transactions, transaction)
	}

	finalizeData, err := encodeManifestCall(
		"finalizeImport(bytes32)",
		abi.Arguments{{Type: mustABIType("bytes32", nil)}},
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("encode finalize import: %w", err)
	}
	finalizeEvent := "ImportFinalized"
	finalizeTopic := manifestEventTopic("ImportFinalized(bytes32,bytes32,uint256,uint256,uint256,uint256)")
	if plan.Target.Controller != "" {
		finalizeEvent = "ImportPublished"
		finalizeTopic = manifestEventTopic("ImportPublished(bytes32,bytes32)")
	}
	manifest.Transactions = append(manifest.Transactions, ManifestTransaction{
		Order: len(manifest.Transactions), Phase: "finalize",
		Function:      "finalizeImport(bytes32)",
		ExpectedEvent: finalizeEvent,
		ExpectedTopic: finalizeTopic,
		EventEmitter:  manifestTarget(plan),
		To:            manifestTarget(plan),
		Value:         "0x0",
		Data:          finalizeData,
	})
	manifest.Summary.TransactionCount = len(manifest.Transactions)
	return manifest, nil
}

func validateManifestPlan(plan *Plan) error {
	if plan == nil {
		return errors.New("import plan is nil")
	}
	if plan.Schema != PlanSchema {
		return fmt.Errorf("unsupported import plan schema %q", plan.Schema)
	}
	if plan.Executable {
		return errors.New("offline import plan unexpectedly claims to be executable")
	}
	if plan.ImportDomain != ImportDomain {
		return fmt.Errorf("unsupported import identity domain %q", plan.ImportDomain)
	}
	if plan.ImportScope != CoreImportScope {
		return fmt.Errorf("unsupported import scope %q", plan.ImportScope)
	}
	if plan.BatchSize < 1 || plan.BatchSize > MaximumBatchSize {
		return fmt.Errorf("plan batch size must be between 1 and %d", MaximumBatchSize)
	}
	if plan.Target.ChainID == 0 {
		return errors.New("target chain ID must be positive")
	}
	target, err := normalizeNonZeroAddress(plan.Target.Contract)
	if err != nil {
		return fmt.Errorf("manifest target contract is not a canonical non-zero address: %w", err)
	}
	if target != plan.Target.Contract {
		return errors.New("manifest target contract is not a canonical non-zero address")
	}
	if plan.Target.Controller != "" {
		controller, err := normalizeNonZeroAddress(plan.Target.Controller)
		if err != nil {
			return fmt.Errorf("manifest controller contract is not a canonical non-zero address: %w", err)
		}
		if controller != plan.Target.Controller || controller == plan.Target.Contract {
			return errors.New("manifest controller contract is not a distinct canonical non-zero address")
		}
	}
	source, err := normalizeNonZeroAddress(plan.Snapshot.ContractEVM)
	if err != nil {
		return fmt.Errorf("manifest source contract is not a canonical non-zero address: %w", err)
	}
	if source != plan.Snapshot.ContractEVM {
		return errors.New("manifest source contract is not a canonical non-zero address")
	}
	if strings.TrimSpace(plan.Snapshot.ChainID) == "" {
		return errors.New("snapshot source chain ID is required")
	}
	if _, err := manifestBytes32("snapshot SHA-256", plan.Snapshot.SHA256); err != nil {
		return err
	}
	if _, err := strconv.ParseUint(plan.Snapshot.Height, 10, 64); err != nil {
		return fmt.Errorf("invalid snapshot height %q: %w", plan.Snapshot.Height, err)
	}
	if len(plan.Repositories) != plan.Summary.RepositoryCount ||
		len(plan.Batches) != plan.Summary.BatchCount {
		return errors.New("plan summary does not match repository or batch records")
	}
	if plan.Summary.RefCount < 0 || plan.Summary.CollaboratorCount < 0 {
		return errors.New("plan summary contains a negative count")
	}

	seen := make(map[string]struct{}, len(plan.Repositories))
	refs := 0
	collaborators := 0
	expectedBatches := make([]PlanBatch, 0, len(plan.Batches))
	for index, repository := range plan.Repositories {
		if repository.Sequence != index {
			return fmt.Errorf("repository sequence %d is not contiguous at index %d", repository.Sequence, index)
		}
		if index > 0 {
			previous := plan.Repositories[index-1]
			if previous.SourceOwner > repository.SourceOwner ||
				(previous.SourceOwner == repository.SourceOwner && previous.Name >= repository.Name) {
				return fmt.Errorf("repository %d is not in planner source_owner/name order", index)
			}
		}
		key := strings.ToLower(repository.RepoID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate plan repo ID %s", repository.RepoID)
		}
		seen[key] = struct{}{}
		if err := validateManifestRepository(repository); err != nil {
			return fmt.Errorf("repository %d (%s/%s): %w", index, repository.Owner, repository.Name, err)
		}
		expectedID, err := importedRepoID(
			plan.Target.ChainID,
			plan.Target.Contract,
			plan.Snapshot.ChainID,
			plan.Snapshot.ContractEVM,
			repository.Owner,
			repository.Name,
		)
		if err != nil {
			return fmt.Errorf("derive repo ID for %s/%s: %w", repository.Owner, repository.Name, err)
		}
		if !strings.EqualFold(expectedID, repository.RepoID) {
			return fmt.Errorf("repo ID mismatch for %s/%s: expected %s, got %s", repository.Owner, repository.Name, expectedID, repository.RepoID)
		}
		refs += len(repository.Refs)
		collaborators += len(repository.Collaborators)
		expectedBatches = append(expectedBatches, batchForRepo(repository, len(expectedBatches)))
		expectedBatches = append(
			expectedBatches,
			batchesForRefs(repository, plan.BatchSize, len(expectedBatches))...,
		)
		expectedBatches = append(
			expectedBatches,
			batchesForCollaborators(repository, plan.BatchSize, len(expectedBatches))...,
		)
	}
	if refs != plan.Summary.RefCount || collaborators != plan.Summary.CollaboratorCount {
		return errors.New("plan summary does not match ref or collaborator records")
	}
	if len(expectedBatches) != len(plan.Batches) {
		return fmt.Errorf(
			"plan batches do not completely cover repository data: got %d, want %d",
			len(plan.Batches),
			len(expectedBatches),
		)
	}
	for index := range plan.Batches {
		if err := comparePlanBatch(plan.Batches[index], expectedBatches[index]); err != nil {
			return fmt.Errorf("batch %d does not exactly cover repository data: %w", index, err)
		}
	}
	expectedCommitment, err := ImportCommitment(plan)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimPrefix(plan.ImportCommitment, "0x"), expectedCommitment) {
		return fmt.Errorf("import commitment mismatch: expected %s, got %s", expectedCommitment, plan.ImportCommitment)
	}
	return nil
}

func validateManifestRepository(repository PlanRepository) error {
	if strings.TrimSpace(repository.SourceOwner) == "" {
		return errors.New("source owner is required")
	}
	if _, err := manifestBytes32("repo ID", repository.RepoID); err != nil {
		return err
	}
	owner, err := normalizeNonZeroAddress(repository.Owner)
	if err != nil {
		return fmt.Errorf("owner is not a canonical non-zero address: %w", err)
	}
	if owner != repository.Owner {
		return errors.New("owner is not a canonical non-zero address")
	}
	if err := validateImportRepoName(repository.Name); err != nil {
		return err
	}
	if err := validateImportModerationStatus(repository.ModerationStatus); err != nil {
		return fmt.Errorf("moderation status: %w", err)
	}
	if len(repository.Refs) > EVMV2MaxRefsPerRepo {
		return fmt.Errorf("has %d refs; V2 maximum is %d", len(repository.Refs), EVMV2MaxRefsPerRepo)
	}
	if len(repository.Collaborators) > EVMV2MaxCollaboratorsPerRepo {
		return fmt.Errorf(
			"has %d collaborators; V2 maximum is %d",
			len(repository.Collaborators),
			EVMV2MaxCollaboratorsPerRepo,
		)
	}

	seenRefs := make(map[string]struct{}, len(repository.Refs))
	for index, ref := range repository.Refs {
		if err := validateImportRefName(ref.RefName); err != nil {
			return fmt.Errorf("ref %d (%s): %w", index, ref.RefName, err)
		}
		if _, exists := seenRefs[ref.RefName]; exists {
			return fmt.Errorf("duplicate ref %s", ref.RefName)
		}
		seenRefs[ref.RefName] = struct{}{}
		if !validCommitSHA(ref.CommitSHA) {
			return fmt.Errorf("ref %s has invalid commit SHA", ref.RefName)
		}
		updatedBy, err := normalizeNonZeroAddress(ref.UpdatedBy)
		if err != nil {
			return fmt.Errorf("ref %s updater is not a canonical non-zero address: %w", ref.RefName, err)
		}
		if updatedBy != ref.UpdatedBy {
			return fmt.Errorf("ref %s updater is not a canonical non-zero address", ref.RefName)
		}
		if len(ref.PackURIs) == 0 || len(ref.PackURIs) > EVMV2MaxPackURIs {
			return fmt.Errorf(
				"ref %s has %d pack URIs; V2 requires 1..%d",
				ref.RefName,
				len(ref.PackURIs),
				EVMV2MaxPackURIs,
			)
		}
		for _, uri := range ref.PackURIs {
			if err := validateImportPackURI(uri); err != nil {
				return fmt.Errorf("ref %s pack URI: %w", ref.RefName, err)
			}
		}
	}

	seenCollaborators := make(map[string]struct{}, len(repository.Collaborators))
	for index, collaborator := range repository.Collaborators {
		account, err := normalizeNonZeroAddress(collaborator.Address)
		if err != nil {
			return fmt.Errorf("collaborator %d is not a canonical non-zero address: %w", index, err)
		}
		if account != collaborator.Address {
			return fmt.Errorf("collaborator %d is not a canonical non-zero address", index)
		}
		if account == owner {
			return fmt.Errorf("collaborator %s is the repository owner", account)
		}
		if collaborator.Role != "maintainer" && collaborator.Role != "reader" {
			return fmt.Errorf("collaborator %s has invalid role %q", account, collaborator.Role)
		}
		if _, exists := seenCollaborators[account]; exists {
			return fmt.Errorf("duplicate collaborator %s", account)
		}
		seenCollaborators[account] = struct{}{}
	}
	return nil
}

func encodeCreateImportSession(plan *Plan, sessionID [32]byte, sourceHeight uint64) (string, error) {
	arguments := abi.Arguments{
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("string", nil)},
		{Type: mustABIType("address", nil)},
		{Type: mustABIType("uint64", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)},
	}
	values := []any{
		sessionID,
		plan.Snapshot.ChainID,
		common.HexToAddress(plan.Snapshot.ContractEVM),
		sourceHeight,
		big.NewInt(int64(plan.Summary.RepositoryCount)),
		big.NewInt(int64(plan.Summary.RefCount)),
		big.NewInt(int64(plan.Summary.CollaboratorCount)),
		big.NewInt(int64(plan.Summary.BatchCount)),
	}
	signature := "createImportSession(bytes32,string,address,uint64,uint256,uint256,uint256,uint256)"
	if plan.Target.Controller != "" {
		commitment, err := manifestBytes32("import commitment", plan.ImportCommitment)
		if err != nil {
			return "", err
		}
		arguments = append(arguments, abi.Argument{Type: mustABIType("bytes32", nil)})
		values = append(values, commitment)
		signature = "createImportSession(bytes32,string,address,uint64,uint256,uint256,uint256,uint256,bytes32)"
	}
	return encodeManifestCall(signature, arguments, values...)
}

func encodeManifestBatch(
	plan *Plan,
	sessionID [32]byte,
	repository PlanRepository,
	batch PlanBatch,
) (ManifestTransaction, error) {
	repoID, err := manifestBytes32("repo ID", batch.RepoID)
	if err != nil {
		return ManifestTransaction{}, err
	}
	payloadHash, err := manifestBytes32("payload SHA-256", batch.PayloadSHA256)
	if err != nil {
		return ManifestTransaction{}, err
	}
	sequence := batch.Sequence
	transaction := ManifestTransaction{
		Phase:         batch.Kind,
		Sequence:      &sequence,
		RepoID:        batch.RepoID,
		PayloadSHA256: batch.PayloadSHA256,
		EventEmitter:  plan.Target.Contract,
		To:            manifestTarget(plan),
		Value:         "0x0",
	}

	switch batch.Kind {
	case "import_repo":
		expected := batchForRepo(repository, batch.Sequence)
		if err := comparePlanBatch(batch, expected); err != nil {
			return ManifestTransaction{}, err
		}
		arguments := abi.Arguments{
			{Type: mustABIType("bytes32", nil)}, {Type: mustABIType("uint256", nil)},
			{Type: mustABIType("bytes32", nil)}, {Type: mustABIType("address", nil)},
			{Type: mustABIType("string", nil)}, {Type: mustABIType("string", nil)},
			{Type: mustABIType("string", nil)}, {Type: mustABIType("uint8", nil)},
			{Type: mustABIType("uint64", nil)},
			{Type: mustABIType("uint64", nil)}, {Type: mustABIType("uint256", nil)},
			{Type: mustABIType("uint256", nil)}, {Type: mustABIType("bytes32", nil)},
		}
		transaction.Function = "importRepo(bytes32,uint256,bytes32,address,string,string,string,uint8,uint64,uint64,uint256,uint256,bytes32)"
		transaction.ExpectedEvent = "ImportRepoApplied"
		transaction.ExpectedTopic = manifestEventTopic("ImportRepoApplied(bytes32,uint256,bytes32,bytes32,address,string,uint8,uint256,uint256)")
		transaction.Data, err = encodeManifestCall(
			transaction.Function,
			arguments,
			sessionID,
			new(big.Int).SetUint64(uint64(batch.Sequence)),
			repoID,
			common.HexToAddress(repository.Owner),
			repository.Name,
			repository.Description,
			repository.DefaultBranch,
			importModerationStatusCode(repository.ModerationStatus),
			repository.CreatedAt,
			repository.UpdatedAt,
			big.NewInt(int64(len(repository.Refs))),
			big.NewInt(int64(len(repository.Collaborators))),
			payloadHash,
		)
	case "import_refs":
		if batch.Start < 0 || batch.Count <= 0 || batch.Start > len(repository.Refs)-batch.Count {
			return ManifestTransaction{}, errors.New("ref batch bounds are invalid")
		}
		refs := repository.Refs[batch.Start : batch.Start+batch.Count]
		expected := PlanBatch{
			Sequence: batch.Sequence, RepoID: repository.RepoID, Kind: "import_refs",
			Start: batch.Start, Count: batch.Count, PayloadSHA256: importRefsPayloadHash(refs),
		}
		for _, ref := range refs {
			expected.ItemKeys = append(expected.ItemKeys, ref.RefName)
		}
		if err := comparePlanBatch(batch, expected); err != nil {
			return ManifestTransaction{}, err
		}
		values := make([]manifestImportRef, 0, len(refs))
		for _, ref := range refs {
			values = append(values, manifestImportRef{
				RefName: ref.RefName, CommitSHA: ref.CommitSHA,
				PackURIs: append([]string(nil), ref.PackURIs...), UpdatedAt: ref.UpdatedAt,
				UpdatedBy: common.HexToAddress(ref.UpdatedBy),
			})
		}
		refTuple := mustABIType("tuple[]", []abi.ArgumentMarshaling{
			{Name: "refName", Type: "string"},
			{Name: "commitSha", Type: "string"},
			{Name: "packUris", Type: "string[]"},
			{Name: "updatedAt", Type: "uint64"},
			{Name: "updatedBy", Type: "address"},
		})
		transaction.Function = "importRefs(bytes32,uint256,bytes32,(string,string,string[],uint64,address)[],bytes32)"
		transaction.ExpectedEvent = "ImportRefsApplied"
		transaction.ExpectedTopic = manifestEventTopic("ImportRefsApplied(bytes32,uint256,bytes32,bytes32,uint256)")
		transaction.Data, err = encodeManifestCall(
			transaction.Function,
			abi.Arguments{
				{Type: mustABIType("bytes32", nil)}, {Type: mustABIType("uint256", nil)},
				{Type: mustABIType("bytes32", nil)}, {Type: refTuple},
				{Type: mustABIType("bytes32", nil)},
			},
			sessionID,
			new(big.Int).SetUint64(uint64(batch.Sequence)),
			repoID,
			values,
			payloadHash,
		)
	case "import_collaborators":
		if batch.Start < 0 || batch.Count <= 0 || batch.Start > len(repository.Collaborators)-batch.Count {
			return ManifestTransaction{}, errors.New("collaborator batch bounds are invalid")
		}
		collaborators := repository.Collaborators[batch.Start : batch.Start+batch.Count]
		expected := PlanBatch{
			Sequence: batch.Sequence, RepoID: repository.RepoID, Kind: "import_collaborators",
			Start: batch.Start, Count: batch.Count, PayloadSHA256: importCollaboratorsPayloadHash(collaborators),
		}
		values := make([]manifestImportCollaborator, 0, len(collaborators))
		for _, collaborator := range collaborators {
			expected.ItemKeys = append(expected.ItemKeys, collaborator.Address)
			role := uint8(2)
			if collaborator.Role == "maintainer" {
				role = 1
			}
			values = append(values, manifestImportCollaborator{
				Account: common.HexToAddress(collaborator.Address), Role: role,
			})
		}
		if err := comparePlanBatch(batch, expected); err != nil {
			return ManifestTransaction{}, err
		}
		collaboratorTuple := mustABIType("tuple[]", []abi.ArgumentMarshaling{
			{Name: "account", Type: "address"},
			{Name: "role", Type: "uint8"},
		})
		transaction.Function = "importCollaborators(bytes32,uint256,bytes32,(address,uint8)[],bytes32)"
		transaction.ExpectedEvent = "ImportCollaboratorsApplied"
		transaction.ExpectedTopic = manifestEventTopic("ImportCollaboratorsApplied(bytes32,uint256,bytes32,bytes32,uint256)")
		transaction.Data, err = encodeManifestCall(
			transaction.Function,
			abi.Arguments{
				{Type: mustABIType("bytes32", nil)}, {Type: mustABIType("uint256", nil)},
				{Type: mustABIType("bytes32", nil)}, {Type: collaboratorTuple},
				{Type: mustABIType("bytes32", nil)},
			},
			sessionID,
			new(big.Int).SetUint64(uint64(batch.Sequence)),
			repoID,
			values,
			payloadHash,
		)
	default:
		return ManifestTransaction{}, fmt.Errorf("unsupported batch kind %q", batch.Kind)
	}
	if err != nil {
		return ManifestTransaction{}, fmt.Errorf("encode %s calldata: %w", batch.Kind, err)
	}
	return transaction, nil
}

func comparePlanBatch(actual, expected PlanBatch) error {
	if actual.Sequence != expected.Sequence ||
		!strings.EqualFold(actual.RepoID, expected.RepoID) ||
		actual.Kind != expected.Kind ||
		actual.Start != expected.Start ||
		actual.Count != expected.Count ||
		!strings.EqualFold(actual.PayloadSHA256, expected.PayloadSHA256) {
		return errors.New("batch metadata or payload hash does not match repository data")
	}
	if len(actual.ItemKeys) != len(expected.ItemKeys) {
		return errors.New("batch item keys do not match repository data")
	}
	for i := range actual.ItemKeys {
		if actual.ItemKeys[i] != expected.ItemKeys[i] {
			return errors.New("batch item keys do not match repository data")
		}
	}
	return nil
}

func encodeManifestCall(signature string, arguments abi.Arguments, values ...any) (string, error) {
	encoded, err := arguments.Pack(values...)
	if err != nil {
		return "", err
	}
	selector := crypto.Keccak256([]byte(signature))[:4]
	data := make([]byte, 0, len(selector)+len(encoded))
	data = append(data, selector...)
	data = append(data, encoded...)
	return "0x" + hex.EncodeToString(data), nil
}

func manifestEventTopic(signature string) string {
	return crypto.Keccak256Hash([]byte(signature)).Hex()
}

func mustABIType(name string, components []abi.ArgumentMarshaling) abi.Type {
	value, err := abi.NewType(name, "", components)
	if err != nil {
		panic(err)
	}
	return value
}

func manifestBytes32(label, value string) ([32]byte, error) {
	var out [32]byte
	raw := strings.TrimPrefix(value, "0x")
	if len(raw) != 64 {
		return out, fmt.Errorf("%s must contain exactly 32 bytes", label)
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return out, fmt.Errorf("invalid %s: %w", label, err)
	}
	copy(out[:], decoded)
	return out, nil
}

func MarshalTransactionManifest(manifest *TransactionManifest) ([]byte, error) {
	if manifest == nil {
		return nil, errors.New("transaction manifest is nil")
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// ReadTransactionManifest strictly decodes exactly one manifest document. A
// runner must additionally call VerifyTransactionManifest with the source plan
// before it signs or broadcasts any calldata.
func ReadTransactionManifest(path string) (*TransactionManifest, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("transaction manifest path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transaction manifest: %w", err)
	}
	var manifest TransactionManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode transaction manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("transaction manifest contains trailing JSON data")
		}
		return nil, fmt.Errorf("read transaction manifest trailer: %w", err)
	}
	return &manifest, nil
}

// VerifyTransactionManifest regenerates the complete canonical manifest from
// the validated plan and requires byte-for-byte structural equality. This
// binds every target, selector, argument, payload hash, event, and ordering
// field without maintaining a second handwritten validator.
func VerifyTransactionManifest(plan *Plan, actual *TransactionManifest) error {
	if actual == nil {
		return errors.New("transaction manifest is nil")
	}
	expected, err := BuildTransactionManifest(plan)
	if err != nil {
		return fmt.Errorf("rebuild transaction manifest: %w", err)
	}
	expectedRaw, err := MarshalTransactionManifest(expected)
	if err != nil {
		return fmt.Errorf("encode expected transaction manifest: %w", err)
	}
	actualRaw, err := MarshalTransactionManifest(actual)
	if err != nil {
		return fmt.Errorf("encode transaction manifest: %w", err)
	}
	if !bytes.Equal(actualRaw, expectedRaw) {
		return errors.New("transaction manifest does not exactly match the plan-derived canonical manifest")
	}
	return nil
}

// TransactionManifestSHA256 hashes the same canonical representation that a
// future execution journal binds in its immutable header.
func TransactionManifestSHA256(manifest *TransactionManifest) (string, error) {
	raw, err := MarshalTransactionManifest(manifest)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

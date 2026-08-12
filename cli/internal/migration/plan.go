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
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	SnapshotSchema        = "igit.cosmwasm-v1.snapshot.v1"
	PlanSchema            = "igit.evm-v2.import-plan.v4"
	ImportDomain          = "igit:v2:import"
	CoreImportScope       = "repo-ref-collaborator-core"
	BatchCommitmentDomain = "igit:v2:import:batch"
	DefaultBatchSize      = 32
	MaximumBatchSize      = 64

	// Keep the offline planner's admission rules aligned with the V2 import
	// entry points. A snapshot that passes planning must not fail only after an
	// import session has globally locked the registry.
	EVMV2MaxRepoNameLength       = 64
	EVMV2MaxRefNameLength        = 256
	EVMV2MaxPackURIs             = 128
	EVMV2MaxPackURIBytes         = 512
	EVMV2MaxRefsPerRepo          = 1024
	EVMV2MaxCollaboratorsPerRepo = 256
)

type Snapshot struct {
	Schema         string                `json:"schema"`
	Source         SnapshotSource        `json:"source"`
	Repositories   []SnapshotRepo        `json:"repositories"`
	Refs           []SnapshotRefGroup    `json:"refs"`
	Collaborators  []SnapshotCollabGroup `json:"collaborators"`
	RepoExtensions []SnapshotExtension   `json:"repo_extensions"`
}

type SnapshotSource struct {
	ChainID  string `json:"chain_id"`
	Contract string `json:"contract"`
	Height   string `json:"height"`
}

type SnapshotRepo struct {
	Owner            string  `json:"owner"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	DefaultBranch    string  `json:"default_branch"`
	CreatedAt        uint64  `json:"created_at"`
	UpdatedAt        uint64  `json:"updated_at"`
	ModerationStatus string  `json:"moderation_status"`
	ForkedFrom       *string `json:"forked_from"`
}

type SnapshotRefGroup struct {
	Owner string        `json:"owner"`
	Repo  string        `json:"repo"`
	Refs  []SnapshotRef `json:"refs"`
}

type SnapshotRef struct {
	RefName   string   `json:"ref_name"`
	CommitSHA string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
	UpdatedAt uint64   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

type SnapshotCollabGroup struct {
	Owner         string                 `json:"owner"`
	Repo          string                 `json:"repo"`
	Collaborators []SnapshotCollaborator `json:"collaborators"`
}

type SnapshotCollaborator struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

type SnapshotExtension struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type Options struct {
	SnapshotPath       string
	HashPath           string
	TargetChainID      uint64
	TargetContract     string
	ControllerContract string
	BatchSize          int
}

type Plan struct {
	Schema           string           `json:"schema"`
	Executable       bool             `json:"executable"`
	ImportScope      string           `json:"import_scope"`
	Snapshot         PlanSnapshot     `json:"snapshot"`
	Target           PlanTarget       `json:"target"`
	ImportCommitment string           `json:"import_commitment"`
	ImportDomain     string           `json:"import_domain"`
	BatchSize        int              `json:"batch_size"`
	Repositories     []PlanRepository `json:"repositories"`
	Batches          []PlanBatch      `json:"batches"`
	Summary          PlanSummary      `json:"summary"`
	DeferredSections []string         `json:"deferred_sections"`
}

type PlanSnapshot struct {
	SHA256      string `json:"sha256"`
	ChainID     string `json:"chain_id"`
	Contract    string `json:"contract"`
	ContractEVM string `json:"contract_evm"`
	Height      string `json:"height"`
}

type PlanTarget struct {
	ChainID    uint64 `json:"chain_id"`
	Contract   string `json:"contract"`
	Controller string `json:"controller,omitempty"`
}

type PlanRepository struct {
	Sequence         int                `json:"sequence"`
	RepoID           string             `json:"repo_id"`
	SourceOwner      string             `json:"source_owner"`
	Owner            string             `json:"owner"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	DefaultBranch    string             `json:"default_branch"`
	CreatedAt        uint64             `json:"created_at"`
	UpdatedAt        uint64             `json:"updated_at"`
	ModerationStatus string             `json:"moderation_status"`
	ForkedFrom       *string            `json:"forked_from"`
	Refs             []PlanRef          `json:"refs"`
	Collaborators    []PlanCollaborator `json:"collaborators"`
}

type PlanRef struct {
	RefName   string   `json:"ref_name"`
	CommitSHA string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
	UpdatedAt uint64   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

type PlanCollaborator struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

type PlanBatch struct {
	Sequence      int      `json:"sequence"`
	RepoID        string   `json:"repo_id"`
	Kind          string   `json:"kind"`
	Start         int      `json:"start"`
	Count         int      `json:"count"`
	ItemKeys      []string `json:"item_keys"`
	PayloadSHA256 string   `json:"payload_sha256"`
}

type PlanSummary struct {
	RepositoryCount   int `json:"repository_count"`
	RefCount          int `json:"ref_count"`
	CollaboratorCount int `json:"collaborator_count"`
	BatchCount        int `json:"batch_count"`
}

func BuildPlan(options Options) (*Plan, error) {
	if strings.TrimSpace(options.SnapshotPath) == "" {
		return nil, errors.New("snapshot path is required")
	}
	if strings.TrimSpace(options.HashPath) == "" {
		options.HashPath = options.SnapshotPath + ".sha256"
	}
	if options.TargetChainID == 0 {
		return nil, errors.New("target chain ID must be positive")
	}
	targetContract, err := normalizeNonZeroAddress(options.TargetContract)
	if err != nil {
		return nil, fmt.Errorf("invalid target contract address: %w", err)
	}
	controllerContract := ""
	if strings.TrimSpace(options.ControllerContract) != "" {
		controllerContract, err = normalizeNonZeroAddress(options.ControllerContract)
		if err != nil {
			return nil, fmt.Errorf("invalid controller contract address: %w", err)
		}
		if controllerContract == targetContract {
			return nil, errors.New("controller contract must differ from target registry contract")
		}
	}
	if options.BatchSize == 0 {
		options.BatchSize = DefaultBatchSize
	}
	if options.BatchSize < 1 || options.BatchSize > MaximumBatchSize {
		return nil, fmt.Errorf("batch size must be between 1 and %d", MaximumBatchSize)
	}

	raw, err := os.ReadFile(options.SnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	digest := sha256.Sum256(raw)
	digestText := hex.EncodeToString(digest[:])
	if err := verifyHashFile(options.HashPath, filepath.Base(options.SnapshotPath), digestText); err != nil {
		return nil, err
	}
	var snapshot Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("snapshot contains trailing JSON data")
	}
	if snapshot.Schema != SnapshotSchema {
		return nil, fmt.Errorf("unsupported snapshot schema %q", snapshot.Schema)
	}
	if strings.TrimSpace(snapshot.Source.ChainID) == "" || strings.TrimSpace(snapshot.Source.Contract) == "" || strings.TrimSpace(snapshot.Source.Height) == "" {
		return nil, errors.New("snapshot source chain_id, contract, and height are required")
	}
	if _, err := strconv.ParseUint(snapshot.Source.Height, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid snapshot height %q", snapshot.Source.Height)
	}
	sourceContract, err := normalizeNonZeroAddress(snapshot.Source.Contract)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot source contract: %w", err)
	}

	refGroups, err := indexRefGroups(snapshot.Refs)
	if err != nil {
		return nil, err
	}
	collabGroups, err := indexCollaboratorGroups(snapshot.Collaborators)
	if err != nil {
		return nil, err
	}
	extensionGroups, err := indexExtensionGroups(snapshot.RepoExtensions)
	if err != nil {
		return nil, err
	}

	repositories := append([]SnapshotRepo(nil), snapshot.Repositories...)
	sort.Slice(repositories, func(i, j int) bool {
		if repositories[i].Owner != repositories[j].Owner {
			return repositories[i].Owner < repositories[j].Owner
		}
		return repositories[i].Name < repositories[j].Name
	})
	plan := &Plan{
		Schema:           PlanSchema,
		Executable:       false,
		ImportScope:      CoreImportScope,
		Snapshot:         PlanSnapshot{SHA256: digestText, ChainID: snapshot.Source.ChainID, Contract: snapshot.Source.Contract, ContractEVM: sourceContract, Height: snapshot.Source.Height},
		Target:           PlanTarget{ChainID: options.TargetChainID, Contract: targetContract, Controller: controllerContract},
		ImportDomain:     ImportDomain,
		BatchSize:        options.BatchSize,
		DeferredSections: []string{"repo_extensions", "moderation_reports", "usernames", "badges_by_recipient", "releases"},
	}
	seenRepos := make(map[string]struct{}, len(repositories))
	seenRepoIDs := make(map[string]string, len(repositories))
	for sequence, repository := range repositories {
		key, err := repositoryKey(repository.Owner, repository.Name)
		if err != nil {
			return nil, fmt.Errorf("repository %d: %w", sequence, err)
		}
		if err := validateImportRepoName(repository.Name); err != nil {
			return nil, fmt.Errorf("repository %s/%s: %w", repository.Owner, repository.Name, err)
		}
		if err := validateImportModerationStatus(repository.ModerationStatus); err != nil {
			return nil, fmt.Errorf("repository %s/%s moderation status: %w", repository.Owner, repository.Name, err)
		}
		if _, exists := seenRepos[key]; exists {
			return nil, fmt.Errorf("duplicate repository %s/%s", repository.Owner, repository.Name)
		}
		seenRepos[key] = struct{}{}
		refs, ok := refGroups[key]
		if !ok {
			return nil, fmt.Errorf("repository %s/%s has no refs group", repository.Owner, repository.Name)
		}
		collaborators, ok := collabGroups[key]
		if !ok {
			return nil, fmt.Errorf("repository %s/%s has no collaborators group", repository.Owner, repository.Name)
		}
		if _, ok := extensionGroups[key]; !ok {
			return nil, fmt.Errorf("repository %s/%s has no extension group", repository.Owner, repository.Name)
		}
		owner, err := normalizeNonZeroAddress(repository.Owner)
		if err != nil {
			return nil, fmt.Errorf("repository %s/%s owner mapping: %w", repository.Owner, repository.Name, err)
		}
		repoID, err := importedRepoID(options.TargetChainID, targetContract, snapshot.Source.ChainID, sourceContract, owner, repository.Name)
		if err != nil {
			return nil, fmt.Errorf("repository %s/%s identity: %w", repository.Owner, repository.Name, err)
		}
		if previous, exists := seenRepoIDs[repoID]; exists {
			return nil, fmt.Errorf("repository ID collision between %s and %s/%s", previous, repository.Owner, repository.Name)
		}
		seenRepoIDs[repoID] = repository.Owner + "/" + repository.Name
		plannedRefs, err := convertRefs(refs, repository.Owner, repository.Name)
		if err != nil {
			return nil, err
		}
		plannedCollaborators, err := convertCollaborators(collaborators, repository.Owner, repository.Name, owner)
		if err != nil {
			return nil, err
		}
		planned := PlanRepository{
			Sequence: sequence, RepoID: repoID, SourceOwner: repository.Owner, Owner: owner,
			Name: repository.Name, Description: repository.Description, DefaultBranch: repository.DefaultBranch,
			CreatedAt: repository.CreatedAt, UpdatedAt: repository.UpdatedAt,
			ModerationStatus: repository.ModerationStatus, ForkedFrom: repository.ForkedFrom,
			Refs: plannedRefs, Collaborators: plannedCollaborators,
		}
		plan.Repositories = append(plan.Repositories, planned)
		plan.Batches = append(plan.Batches, batchForRepo(planned, len(plan.Batches)))
		plan.Batches = append(plan.Batches, batchesForRefs(planned, options.BatchSize, len(plan.Batches))...)
		plan.Batches = append(plan.Batches, batchesForCollaborators(planned, options.BatchSize, len(plan.Batches))...)
		plan.Summary.RefCount += len(plannedRefs)
		plan.Summary.CollaboratorCount += len(plannedCollaborators)
	}
	if len(refGroups) != len(seenRepos) || len(collabGroups) != len(seenRepos) || len(extensionGroups) != len(seenRepos) {
		return nil, errors.New("snapshot contains repository groups that do not reference a repository")
	}
	plan.Summary.RepositoryCount = len(plan.Repositories)
	plan.Summary.BatchCount = len(plan.Batches)
	commitment, err := ImportCommitment(plan)
	if err != nil {
		return nil, err
	}
	plan.ImportCommitment = commitment
	return plan, nil
}

func verifyHashFile(path, snapshotName, actual string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read snapshot hash file: %w", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		return errors.New("snapshot hash file must contain exactly one '<sha256>  <basename>' record")
	}
	if len(fields[0]) != 64 || strings.ToLower(fields[0]) != fields[0] {
		return errors.New("snapshot hash file contains an invalid lowercase SHA-256 digest")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return errors.New("snapshot hash file contains an invalid lowercase SHA-256 digest")
	}
	if fields[1] != snapshotName {
		return fmt.Errorf("snapshot hash file names %q, want %q", fields[1], snapshotName)
	}
	if fields[0] != actual {
		return fmt.Errorf("snapshot hash mismatch: expected %s, got %s", fields[0], actual)
	}
	return nil
}

func repositoryKey(owner, name string) (string, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return "", errors.New("repository owner and name are required")
	}
	return owner + "\x00" + name, nil
}

func indexRefGroups(groups []SnapshotRefGroup) (map[string][]SnapshotRef, error) {
	out := make(map[string][]SnapshotRef, len(groups))
	for _, group := range groups {
		key, err := repositoryKey(group.Owner, group.Repo)
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate refs group for %s/%s", group.Owner, group.Repo)
		}
		out[key] = group.Refs
	}
	return out, nil
}

func indexCollaboratorGroups(groups []SnapshotCollabGroup) (map[string][]SnapshotCollaborator, error) {
	out := make(map[string][]SnapshotCollaborator, len(groups))
	for _, group := range groups {
		key, err := repositoryKey(group.Owner, group.Repo)
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate collaborators group for %s/%s", group.Owner, group.Repo)
		}
		out[key] = group.Collaborators
	}
	return out, nil
}

func indexExtensionGroups(groups []SnapshotExtension) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		key, err := repositoryKey(group.Owner, group.Repo)
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate extension group for %s/%s", group.Owner, group.Repo)
		}
		out[key] = struct{}{}
	}
	return out, nil
}

func convertRefs(refs []SnapshotRef, owner, repo string) ([]PlanRef, error) {
	if len(refs) > EVMV2MaxRefsPerRepo {
		return nil, fmt.Errorf("repository %s/%s has %d refs; V2 maximum is %d", owner, repo, len(refs), EVMV2MaxRefsPerRepo)
	}
	values := append([]SnapshotRef(nil), refs...)
	sort.Slice(values, func(i, j int) bool { return values[i].RefName < values[j].RefName })
	seen := make(map[string]struct{}, len(values))
	out := make([]PlanRef, 0, len(values))
	for _, ref := range values {
		if err := validateImportRefName(ref.RefName); err != nil {
			return nil, fmt.Errorf("repository %s/%s ref %s: %w", owner, repo, ref.RefName, err)
		}
		if _, exists := seen[ref.RefName]; exists {
			return nil, fmt.Errorf("repository %s/%s has duplicate ref %s", owner, repo, ref.RefName)
		}
		seen[ref.RefName] = struct{}{}
		if !validCommitSHA(ref.CommitSHA) {
			return nil, fmt.Errorf("repository %s/%s ref %s has invalid commit SHA", owner, repo, ref.RefName)
		}
		updatedBy, err := normalizeNonZeroAddress(ref.UpdatedBy)
		if err != nil {
			return nil, fmt.Errorf("repository %s/%s ref %s updater mapping: %w", owner, repo, ref.RefName, err)
		}
		if len(ref.PackURIs) == 0 {
			return nil, fmt.Errorf("repository %s/%s ref %s has no pack URI", owner, repo, ref.RefName)
		}
		if len(ref.PackURIs) > EVMV2MaxPackURIs {
			return nil, fmt.Errorf("repository %s/%s ref %s has %d pack URIs; V2 maximum is %d", owner, repo, ref.RefName, len(ref.PackURIs), EVMV2MaxPackURIs)
		}
		uris := append([]string(nil), ref.PackURIs...)
		for _, uri := range uris {
			if err := validateImportPackURI(uri); err != nil {
				return nil, fmt.Errorf("repository %s/%s ref %s pack URI: %w", owner, repo, ref.RefName, err)
			}
		}
		out = append(out, PlanRef{RefName: ref.RefName, CommitSHA: ref.CommitSHA, PackURIs: uris, UpdatedAt: ref.UpdatedAt, UpdatedBy: updatedBy})
	}
	return out, nil
}

func convertCollaborators(values []SnapshotCollaborator, owner, repo, targetOwner string) ([]PlanCollaborator, error) {
	if len(values) > EVMV2MaxCollaboratorsPerRepo {
		return nil, fmt.Errorf("repository %s/%s has %d collaborators; V2 maximum is %d", owner, repo, len(values), EVMV2MaxCollaboratorsPerRepo)
	}
	copyValues := append([]SnapshotCollaborator(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].Address < copyValues[j].Address })
	seen := make(map[string]struct{}, len(copyValues))
	out := make([]PlanCollaborator, 0, len(copyValues))
	for _, collaborator := range copyValues {
		address, err := normalizeNonZeroAddress(collaborator.Address)
		if err != nil {
			return nil, fmt.Errorf("repository %s/%s collaborator mapping: %w", owner, repo, err)
		}
		if address == targetOwner {
			return nil, fmt.Errorf("repository %s/%s lists its owner as a collaborator", owner, repo)
		}
		role := strings.ToLower(strings.TrimSpace(collaborator.Role))
		if role != "maintainer" && role != "reader" {
			return nil, fmt.Errorf("repository %s/%s collaborator %s has invalid role %q", owner, repo, collaborator.Address, collaborator.Role)
		}
		if _, exists := seen[address]; exists {
			return nil, fmt.Errorf("repository %s/%s has duplicate collaborator %s after address normalization", owner, repo, address)
		}
		seen[address] = struct{}{}
		out = append(out, PlanCollaborator{Address: address, Role: role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

func validateImportRepoName(value string) error {
	chars := []byte(value)
	if len(chars) == 0 {
		return errors.New("repo name is empty")
	}
	if len(chars) > EVMV2MaxRepoNameLength {
		return fmt.Errorf("repo name is %d bytes; V2 maximum is %d", len(chars), EVMV2MaxRepoNameLength)
	}
	for _, c := range chars {
		alphaNum := (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if !alphaNum && c != '.' && c != '_' && c != '-' {
			return fmt.Errorf("repo name contains unsupported byte 0x%02x", c)
		}
	}
	return nil
}

func validateImportRefName(value string) error {
	chars := []byte(value)
	if len(chars) < len("refs/") || len(chars) > EVMV2MaxRefNameLength {
		return fmt.Errorf("ref name is %d bytes; V2 requires %d..%d bytes", len(chars), len("refs/"), EVMV2MaxRefNameLength)
	}
	if !strings.HasPrefix(value, "refs/") {
		return errors.New("ref name must start with refs/")
	}
	for i, c := range chars {
		if c <= 0x20 || c == 0x7f || c == '~' || c == '^' || c == ':' || c == '\\' {
			return fmt.Errorf("ref name contains unsupported byte 0x%02x at byte %d", c, i)
		}
		if i+1 < len(chars) && c == '.' && chars[i+1] == '.' {
			return errors.New("ref name must not contain ..")
		}
	}
	return nil
}

func validateImportPackURI(value string) error {
	chars := []byte(value)
	if len(chars) <= len("ipfs://") || len(chars) > EVMV2MaxPackURIBytes {
		return fmt.Errorf("pack URI is %d bytes; V2 requires 8..%d bytes", len(chars), EVMV2MaxPackURIBytes)
	}
	if !strings.HasPrefix(value, "ipfs://") {
		return errors.New("pack URI must start with ipfs://")
	}
	first := chars[len("ipfs://")]
	alphaNum := (first >= '0' && first <= '9') || (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
	if !alphaNum {
		return fmt.Errorf("pack URI locator must start with an ASCII alphanumeric byte, got 0x%02x", first)
	}
	for i := len("ipfs://"); i < len(chars); i++ {
		if chars[i] <= 0x20 || chars[i] == 0x7f {
			return fmt.Errorf("pack URI contains a control byte 0x%02x at byte %d", chars[i], i)
		}
	}
	return nil
}

func validateImportModerationStatus(value string) error {
	switch value {
	case "active", "delisted", "frozen":
		return nil
	default:
		return fmt.Errorf("unsupported status %q; want active, delisted, or frozen", value)
	}
}

func validCommitSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeNonZeroAddress(value string) (string, error) {
	address, err := chain.NormalizeEVMAddress(value)
	if err != nil {
		return "", err
	}
	if common.HexToAddress(address) == (common.Address{}) {
		return "", errors.New("zero address is not allowed")
	}
	return address, nil
}

func importedRepoID(targetChainID uint64, targetContract, sourceChainID, sourceContract, owner, name string) (string, error) {
	stringType, _ := abi.NewType("string", "", nil)
	uintType, _ := abi.NewType("uint256", "", nil)
	addressType, _ := abi.NewType("address", "", nil)
	arguments := abi.Arguments{
		{Type: stringType}, {Type: uintType}, {Type: addressType},
		{Type: stringType}, {Type: addressType}, {Type: addressType}, {Type: stringType},
	}
	encoded, err := arguments.Pack(
		ImportDomain,
		new(big.Int).SetUint64(targetChainID),
		common.HexToAddress(targetContract),
		sourceChainID,
		common.HexToAddress(sourceContract),
		common.HexToAddress(owner),
		name,
	)
	if err != nil {
		return "", err
	}
	return crypto.Keccak256Hash(encoded).Hex(), nil
}

func batchForRepo(repo PlanRepository, sequence int) PlanBatch {
	return PlanBatch{
		Sequence: sequence, RepoID: repo.RepoID, Kind: "import_repo", Start: 0, Count: 1,
		ItemKeys: []string{repo.Owner + "/" + repo.Name}, PayloadSHA256: importRepoPayloadHash(repo),
	}
}

func batchesForRefs(repo PlanRepository, size, sequence int) []PlanBatch {
	var batches []PlanBatch
	for start := 0; start < len(repo.Refs); start += size {
		end := start + size
		if end > len(repo.Refs) {
			end = len(repo.Refs)
		}
		keys := make([]string, 0, end-start)
		for _, ref := range repo.Refs[start:end] {
			keys = append(keys, ref.RefName)
		}
		batches = append(batches, PlanBatch{Sequence: sequence + len(batches), RepoID: repo.RepoID, Kind: "import_refs", Start: start, Count: end - start, ItemKeys: keys, PayloadSHA256: importRefsPayloadHash(repo.Refs[start:end])})
	}
	return batches
}

func batchesForCollaborators(repo PlanRepository, size, sequence int) []PlanBatch {
	var batches []PlanBatch
	for start := 0; start < len(repo.Collaborators); start += size {
		end := start + size
		if end > len(repo.Collaborators) {
			end = len(repo.Collaborators)
		}
		keys := make([]string, 0, end-start)
		for _, collaborator := range repo.Collaborators[start:end] {
			keys = append(keys, collaborator.Address)
		}
		batches = append(batches, PlanBatch{Sequence: sequence + len(batches), RepoID: repo.RepoID, Kind: "import_collaborators", Start: start, Count: end - start, ItemKeys: keys, PayloadSHA256: importCollaboratorsPayloadHash(repo.Collaborators[start:end])})
	}
	return batches
}

func MarshalPlan(plan *Plan) ([]byte, error) {
	if plan == nil {
		return nil, errors.New("plan is nil")
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// PlanSHA256 hashes the canonical, newline-terminated plan representation used
// by transaction manifests, imported-state evidence, and execution journals.
func PlanSHA256(plan *Plan) (string, error) {
	raw, err := MarshalPlan(plan)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

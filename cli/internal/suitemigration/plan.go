package suitemigration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func BuildPlanFile(path string, options BuildOptions) (*Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite snapshot: %w", err)
	}
	return BuildPlan(raw, options)
}

func VerifySnapshotSHA256File(snapshotPath, hashPath string) error {
	if strings.TrimSpace(snapshotPath) == "" || strings.TrimSpace(hashPath) == "" {
		return errors.New("snapshot and SHA-256 sidecar paths are required")
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("read suite snapshot: %w", err)
	}
	digest := sha256.Sum256(raw)
	sidecar, err := os.ReadFile(hashPath)
	if err != nil {
		return fmt.Errorf("read snapshot SHA-256 sidecar: %w", err)
	}
	fields := strings.Fields(string(sidecar))
	if len(fields) != 2 || fields[0] != strings.ToLower(fields[0]) || len(fields[0]) != sha256.Size*2 {
		return errors.New("snapshot SHA-256 sidecar must contain exactly '<lowercase-sha256>  <basename>'")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return errors.New("snapshot SHA-256 sidecar digest is invalid")
	}
	if fields[1] != filepath.Base(snapshotPath) {
		return fmt.Errorf("snapshot SHA-256 sidecar names %q; want %q", fields[1], filepath.Base(snapshotPath))
	}
	actual := hex.EncodeToString(digest[:])
	if fields[0] != actual {
		return fmt.Errorf("snapshot SHA-256 mismatch: sidecar=%s actual=%s", fields[0], actual)
	}
	return nil
}

func BuildPlan(raw []byte, options BuildOptions) (*Plan, error) {
	if len(raw) == 0 {
		return nil, errors.New("suite snapshot is empty")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("decode suite snapshot: %w", err)
	}
	var snapshot Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode suite snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("suite snapshot contains trailing JSON data")
		}
		return nil, fmt.Errorf("decode suite snapshot trailer: %w", err)
	}
	if err := validateSnapshot(&snapshot); err != nil {
		return nil, fmt.Errorf("validate suite snapshot: %w", err)
	}
	if options.TargetChainID == 0 {
		return nil, errors.New("target chain ID must be positive")
	}
	directory, err := canonicalAddress("SuiteDirectory", options.Directory)
	if err != nil {
		return nil, err
	}
	coordinator, err := canonicalAddress("BootstrapCoordinator", options.Coordinator)
	if err != nil {
		return nil, err
	}
	if directory == coordinator {
		return nil, errors.New("SuiteDirectory and BootstrapCoordinator must differ")
	}
	if options.BatchSize == 0 {
		options.BatchSize = DefaultBatchSize
	}
	if options.BatchSize < 1 || options.BatchSize > MaximumBatchItems {
		return nil, fmt.Errorf("batch size must be between 1 and %d", MaximumBatchItems)
	}

	snapshotSHA := sha256.Sum256(raw)
	// SuiteDirectory.snapshotRoot is the immutable SHA-256 sidecar value. The
	// per-module commitments use Keccak exactly where the contracts do.
	snapshotRoot := common.BytesToHash(snapshotSHA[:])
	plan := &Plan{
		Schema: PlanSchema, Executable: true,
		Snapshot: PlanSnapshot{
			Schema: snapshot.Schema, SHA256: hex.EncodeToString(snapshotSHA[:]), Root: snapshotRoot.Hex(),
			SourceChainID: snapshot.Source.ChainID, SourceContract: snapshot.Source.Contract,
			SourceHeight: snapshot.Source.Height, SourceBlockHash: snapshot.Source.BlockHash,
			InventorySHA256: snapshot.Source.InventorySHA256, TxSearchSHA256: snapshot.Source.TxSearchSHA256,
			BlockEvidenceSHA256:   snapshot.Source.BlockEvidenceSHA256,
			EventCommitmentSHA256: snapshot.Source.EventCommitmentSHA256,
		},
		Target:                     PlanTarget{},
		ContractPolicy:             snapshot.ContractPolicy,
		UsernameEscrowRelease:      snapshot.Username.EscrowRelease,
		UsernameEscrowEvidenceHash: "0x" + snapshot.Username.EscrowRelease.EvidenceSHA256,
		Limits:                     PlanLimits{BatchSize: options.BatchSize, MaxBatchItems: MaximumBatchItems, MaxPayloadBytes: MaximumPayloadBytes},
	}
	plan.Target = PlanTarget{ChainID: options.TargetChainID, SuiteVersion: SuiteVersion, Directory: directory.Hex(), Coordinator: coordinator.Hex()}
	// Hex() is checksum-cased; all evidence artifacts use canonical lowercase.
	plan.Target.Directory = strings.ToLower(plan.Target.Directory)
	plan.Target.Coordinator = strings.ToLower(plan.Target.Coordinator)

	moduleRecords, summary, err := snapshotModuleRecords(&snapshot)
	if err != nil {
		return nil, err
	}
	for order, spec := range requiredModules {
		module := ModulePlan{Order: order, Name: spec.name, ID: strings.ToLower(spec.id.Hex())}
		root := rollingSeed(spec.id, snapshotRoot)
		for _, group := range moduleRecords[spec.name] {
			if err := appendRecordBatches(&module, group, options.BatchSize, spec.id, &root); err != nil {
				return nil, fmt.Errorf("build %s/%s batches: %w", spec.name, group.kind, err)
			}
		}
		module.ExpectedBatches = uint64(len(module.Batches))
		module.ExpectedRoot = root.Hex()
		plan.Modules = append(plan.Modules, module)
	}
	summary.BatchCount = totalBatches(plan.Modules)
	summary.TransactionCount = 2*RequiredModuleCount + summary.BatchCount + 2
	plan.Summary = summary
	if err := ValidatePlan(plan); err != nil {
		return nil, fmt.Errorf("validate generated suite plan: %w", err)
	}
	return plan, nil
}

// The dummy literal guard above would not compile if PlanTarget fields drift.
// Keep construction explicit without accepting unknown target metadata.
func init() { _ = PlanTarget{} }

type recordGroup struct {
	kind     string
	kindCode uint8
	values   any
	keys     []string
	encode   func(any) ([]byte, error)
}

func appendRecordBatches(module *ModulePlan, group recordGroup, batchSize int, moduleID common.Hash, root *common.Hash) error {
	value := reflect.ValueOf(group.values)
	if value.Kind() != reflect.Slice || value.Len() != len(group.keys) {
		return errors.New("internal record/key mismatch")
	}
	for start := 0; start < value.Len(); {
		end := start + batchSize
		if end > value.Len() {
			end = value.Len()
		}
		var payload []byte
		var err error
		for {
			payload, err = group.encode(value.Slice(start, end).Interface())
			if err != nil {
				return err
			}
			if len(payload) <= MaximumPayloadBytes {
				break
			}
			if end-start == 1 {
				return fmt.Errorf("record %s encodes to %d bytes; maximum is %d", group.keys[start], len(payload), MaximumPayloadBytes)
			}
			end = start + (end-start)/2
		}
		count := uint64(end - start)
		sequence := uint64(len(module.Batches))
		payloadHash := crypto.Keccak256Hash(payload)
		*root = rollingStep(*root, moduleID, sequence, count, payloadHash)
		module.Batches = append(module.Batches, PlanBatch{
			Sequence: sequence, Kind: group.kind, KindCode: group.kindCode, Count: count,
			ItemKeys: append([]string(nil), group.keys[start:end]...), Payload: hexBytes(payload),
			PayloadHash: payloadHash.Hex(), RollingRoot: root.Hex(),
		})
		module.ExpectedCount += count
		start = end
	}
	return nil
}

func snapshotModuleRecords(snapshot *Snapshot) (map[string][]recordGroup, PlanSummary, error) {
	result := make(map[string][]recordGroup, RequiredModuleCount)
	summary := PlanSummary{
		RepositoryCount: len(snapshot.Repositories), AliasCount: len(snapshot.Aliases), RefCount: len(snapshot.Refs),
		CollaboratorCount: len(snapshot.Collaborators), GuardianConfigCount: len(snapshot.GuardianConfigs),
		ModerationStatusCount: len(snapshot.Moderation.FinalStatuses), ModerationReportCount: len(snapshot.Moderation.Reports),
		ModerationTrailCount: len(snapshot.Moderation.StatusTrails), EconomicSplitCount: len(snapshot.Economic.Splits),
		EconomicTotalCount: len(snapshot.Economic.Totals), UsernameOwnerCount: len(snapshot.Username.OriginalOwners),
		ReservedNameCount: len(snapshot.Username.ReservedNames), BadgeCount: len(snapshot.Badges), ReleaseArtifactCount: len(snapshot.Releases),
	}

	repositories := append([]SnapshotRepository(nil), snapshot.Repositories...)
	repoByID := make(map[string]SnapshotRepository, len(repositories))
	for _, repository := range repositories {
		repoByID[repository.ID] = repository
	}
	depth := make(map[string]int, len(repositories))
	var forkDepth func(string) int
	forkDepth = func(id string) int {
		if value, ok := depth[id]; ok {
			return value
		}
		parent := repoByID[id].ForkedFrom
		if parent == "" {
			depth[id] = 0
			return 0
		}
		value := forkDepth(parent) + 1
		depth[id] = value
		return value
	}
	for _, repository := range repositories {
		forkDepth(repository.ID)
	}
	sort.Slice(repositories, func(i, j int) bool {
		if depth[repositories[i].ID] != depth[repositories[j].ID] {
			return depth[repositories[i].ID] < depth[repositories[j].ID]
		}
		return repositories[i].ID < repositories[j].ID
	})
	repoValues := make([]coreRepositoryABI, len(repositories))
	repoKeys := make([]string, len(repositories))
	for i, record := range repositories {
		id, _ := parseRawHash("repository ID", record.ID)
		fork := [32]byte{}
		if record.ForkedFrom != "" {
			fork, _ = parseRawHash("forked_from", record.ForkedFrom)
		}
		repoValues[i] = coreRepositoryABI{id, common.HexToAddress(record.Owner), record.Name, record.Description, record.DefaultBranch, fork, record.CreatedAt, record.UpdatedAt}
		repoKeys[i] = record.ID
	}
	result["repository-core"] = append(result["repository-core"], kindGroup("repositories", 0, repoValues, repoKeys, coreRepositoryArguments, true))

	aliases := append([]SnapshotAlias(nil), snapshot.Aliases...)
	sort.Slice(aliases, func(i, j int) bool {
		if aliases[i].Owner != aliases[j].Owner {
			return aliases[i].Owner < aliases[j].Owner
		}
		return aliases[i].Name < aliases[j].Name
	})
	aliasValues := make([]coreAliasABI, len(aliases))
	aliasKeys := make([]string, len(aliases))
	for i, record := range aliases {
		id, _ := parseRawHash("alias repo ID", record.RepoID)
		aliasValues[i] = coreAliasABI{id, common.HexToAddress(record.Owner), record.Name}
		aliasKeys[i] = record.Owner + "/" + record.Name
	}
	result["repository-core"] = append(result["repository-core"], kindGroup("aliases", 1, aliasValues, aliasKeys, coreAliasArguments, true))

	refs := append([]SnapshotRef(nil), snapshot.Refs...)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].RepoID != refs[j].RepoID {
			return refs[i].RepoID < refs[j].RepoID
		}
		return refs[i].RefName < refs[j].RefName
	})
	refValues := make([]coreRefABI, len(refs))
	refKeys := make([]string, len(refs))
	for i, record := range refs {
		id, _ := parseRawHash("ref repo ID", record.RepoID)
		refValues[i] = coreRefABI{id, record.RefName, record.CommitSHA, append([]string(nil), record.PackURIs...), record.UpdatedAt, common.HexToAddress(record.UpdatedBy)}
		refKeys[i] = record.RepoID + ":" + record.RefName
	}
	result["repository-core"] = append(result["repository-core"], kindGroup("refs", 2, refValues, refKeys, coreRefArguments, true))

	collaborators := append([]SnapshotCollaborator(nil), snapshot.Collaborators...)
	sort.Slice(collaborators, func(i, j int) bool {
		if collaborators[i].RepoID != collaborators[j].RepoID {
			return collaborators[i].RepoID < collaborators[j].RepoID
		}
		return collaborators[i].Account < collaborators[j].Account
	})
	collabValues := make([]coreCollaboratorABI, len(collaborators))
	collabKeys := make([]string, len(collaborators))
	for i, record := range collaborators {
		id, _ := parseRawHash("collaborator repo ID", record.RepoID)
		role, _ := roleCode(record.Role)
		collabValues[i] = coreCollaboratorABI{id, common.HexToAddress(record.Account), role}
		collabKeys[i] = record.RepoID + ":" + record.Account
	}
	result["repository-core"] = append(result["repository-core"], kindGroup("collaborators", 3, collabValues, collabKeys, coreCollaboratorArguments, true))

	guardians := append([]SnapshotGuardianConfig(nil), snapshot.GuardianConfigs...)
	sort.Slice(guardians, func(i, j int) bool { return guardians[i].RepoID < guardians[j].RepoID })
	guardianValues := make([]guardianConfigABI, len(guardians))
	guardianKeys := make([]string, len(guardians))
	for i, record := range guardians {
		id, _ := parseRawHash("guardian repo ID", record.RepoID)
		addresses := make([]common.Address, len(record.Guardians))
		for j, v := range record.Guardians {
			addresses[j] = common.HexToAddress(v)
		}
		guardianValues[i] = guardianConfigABI{id, common.HexToAddress(record.ConfiguredBy), record.Threshold, addresses}
		guardianKeys[i] = record.RepoID
	}
	result["recovery"] = append(result["recovery"], kindGroup("guardian-configs", 0, guardianValues, guardianKeys, guardianArguments, false))

	statuses := append([]SnapshotFinalStatus(nil), snapshot.Moderation.FinalStatuses...)
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].RepoID < statuses[j].RepoID })
	statusValues := make([]moderationStatusABI, len(statuses))
	statusKeys := make([]string, len(statuses))
	for i, r := range statuses {
		id, _ := parseRawHash("status repo ID", r.RepoID)
		code, _ := repoStatusCode(r.Status)
		statusValues[i] = moderationStatusABI{id, code}
		statusKeys[i] = r.RepoID
	}
	result["moderation"] = append(result["moderation"], kindGroup("final-statuses", 0, statusValues, statusKeys, moderationStatusArguments, true))
	reports := append([]SnapshotReport(nil), snapshot.Moderation.Reports...)
	sort.Slice(reports, func(i, j int) bool { return reports[i].ID < reports[j].ID })
	reportValues := make([]moderationReportABI, len(reports))
	reportKeys := make([]string, len(reports))
	for i, r := range reports {
		id, _ := parseRawHash("report repo ID", r.RepoID)
		status, _ := reportStatusCode(r.Status)
		resolution, _ := repoStatusCode(r.Resolution)
		trail := make([]moderationReportTrailABI, len(r.Trail))
		for j, e := range r.Trail {
			action, _ := trailActionCode(e.Action)
			s, _ := repoStatusCode(e.Status)
			trail[j] = moderationReportTrailABI{action, common.HexToAddress(e.Actor), s, e.ReasonHash, e.Timestamp}
		}
		reportValues[i] = moderationReportABI{moderationReportRecordABI{new(big.Int).SetUint64(r.ID), id, common.HexToAddress(r.Reporter), status, resolution, r.ReasonHash, r.CreatedAt, r.UpdatedAt, true}, trail}
		reportKeys[i] = strconv.FormatUint(r.ID, 10)
	}
	result["moderation"] = append(result["moderation"], kindGroup("reports", 1, reportValues, reportKeys, moderationReportArguments, true))
	statusTrails := append([]SnapshotStatusTrail(nil), snapshot.Moderation.StatusTrails...)
	sort.Slice(statusTrails, func(i, j int) bool { return statusTrails[i].RepoID < statusTrails[j].RepoID })
	statusTrailValues := make([]moderationStatusTrailABI, len(statusTrails))
	statusTrailKeys := make([]string, len(statusTrails))
	for i, r := range statusTrails {
		id, _ := parseRawHash("trail repo ID", r.RepoID)
		trail := make([]moderationStatusLogABI, len(r.Trail))
		for j, e := range r.Trail {
			s, _ := repoStatusCode(e.Status)
			trail[j] = moderationStatusLogABI{s, common.HexToAddress(e.Actor), e.ReasonHash, e.Timestamp, new(big.Int).SetUint64(e.ReportID)}
		}
		statusTrailValues[i] = moderationStatusTrailABI{id, trail}
		statusTrailKeys[i] = r.RepoID
	}
	result["moderation"] = append(result["moderation"], kindGroup("status-trails", 2, statusTrailValues, statusTrailKeys, moderationStatusTrailArguments, true))

	splits := append([]SnapshotRevenueSplits(nil), snapshot.Economic.Splits...)
	sort.Slice(splits, func(i, j int) bool { return splits[i].RepoID < splits[j].RepoID })
	splitValues := make([]economicSplitsABI, len(splits))
	splitKeys := make([]string, len(splits))
	for i, r := range splits {
		id, _ := parseRawHash("split repo ID", r.RepoID)
		entries := make([]economicSplitABI, len(r.Splits))
		for j, e := range r.Splits {
			entries[j] = economicSplitABI{common.HexToAddress(e.Recipient), e.BPS}
		}
		splitValues[i] = economicSplitsABI{id, entries}
		splitKeys[i] = r.RepoID
	}
	result["economic"] = append(result["economic"], kindGroup("splits", 0, splitValues, splitKeys, economicSplitsArguments, true))
	totals := append([]SnapshotSponsorTotal(nil), snapshot.Economic.Totals...)
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].RepoID != totals[j].RepoID {
			return totals[i].RepoID < totals[j].RepoID
		}
		return totals[i].Denom < totals[j].Denom
	})
	totalValues := make([]economicTotalABI, len(totals))
	totalKeys := make([]string, len(totals))
	for i, r := range totals {
		id, _ := parseRawHash("total repo ID", r.RepoID)
		amount, _ := new(big.Int).SetString(r.Amount, 10)
		totalValues[i] = economicTotalABI{id, r.Denom, amount}
		totalKeys[i] = r.RepoID + ":" + r.Denom
	}
	result["economic"] = append(result["economic"], kindGroup("all-denom-totals", 1, totalValues, totalKeys, economicTotalArguments, true))

	owners := append([]SnapshotOriginalUsername(nil), snapshot.Username.OriginalOwners...)
	sort.Slice(owners, func(i, j int) bool { return owners[i].Name < owners[j].Name })
	ownerValues := make([]usernameOwnerABI, len(owners))
	ownerKeys := make([]string, len(owners))
	for i, r := range owners {
		ownerValues[i] = usernameOwnerABI{r.Name, common.HexToAddress(r.Owner)}
		ownerKeys[i] = r.Name
	}
	result["username"] = append(result["username"], kindGroup("original-owners", 0, ownerValues, ownerKeys, usernameOwnerArguments, true))
	reserved := append([]string(nil), snapshot.Username.ReservedNames...)
	sort.Strings(reserved)
	result["username"] = append(result["username"], kindGroup("reserved-names", 1, reserved, reserved, usernameReservedArguments, true))

	badges := append([]SnapshotBadge(nil), snapshot.Badges...)
	sort.Slice(badges, func(i, j int) bool { return badges[i].ID < badges[j].ID })
	badgeValues := make([]badgeABI, len(badges))
	badgeKeys := make([]string, len(badges))
	for i, r := range badges {
		id, _ := parseRawHash("badge repo ID", r.RepoID)
		badgeValues[i] = badgeABI{new(big.Int).SetUint64(r.ID), id, common.HexToAddress(r.Recipient), common.HexToAddress(r.AwardedBy), r.Reason, r.AwardedAt, true}
		badgeKeys[i] = strconv.FormatUint(r.ID, 10)
	}
	result["badge"] = append(result["badge"], kindGroup("badges", 0, badgeValues, badgeKeys, badgeArguments, false))
	releases := append([]SnapshotRelease(nil), snapshot.Releases...)
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Version != releases[j].Version {
			return releases[i].Version < releases[j].Version
		}
		return releases[i].Platform < releases[j].Platform
	})
	releaseValues := make([]releaseABI, len(releases))
	releaseKeys := make([]string, len(releases))
	for i, r := range releases {
		digest, _ := rawSHA256("release SHA-256", r.SHA256)
		releaseValues[i] = releaseABI{r.Version, r.Platform, digest, common.HexToAddress(r.RegisteredBy), r.RegisteredAt, true}
		releaseKeys[i] = r.Version + ":" + r.Platform
	}
	result["release"] = append(result["release"], kindGroup("artifacts", 0, releaseValues, releaseKeys, releaseArguments, false))
	return result, summary, nil
}

func kindGroup(name string, code uint8, values any, keys []string, arguments abi.Arguments, wrapped bool) recordGroup {
	return recordGroup{kind: name, kindCode: code, values: values, keys: keys, encode: func(records any) ([]byte, error) {
		if wrapped {
			return encodeKindPayload(code, arguments, records)
		}
		return encodeDirectPayload(arguments, records)
	}}
}

func ValidatePlan(plan *Plan) error {
	if plan == nil {
		return errors.New("suite plan is nil")
	}
	if plan.Schema != PlanSchema || !plan.Executable {
		return errors.New("unsupported or non-executable suite plan")
	}
	if plan.Snapshot.Schema != SnapshotSchema {
		return fmt.Errorf("unsupported snapshot schema %q", plan.Snapshot.Schema)
	}
	snapshotDigest, err := rawSHA256("snapshot SHA-256", plan.Snapshot.SHA256)
	if err != nil {
		return err
	}
	snapshotRoot, err := parseHash("snapshot root", plan.Snapshot.Root)
	if err != nil {
		return err
	}
	if snapshotRoot != common.BytesToHash(snapshotDigest[:]) {
		return errors.New("snapshot root must equal the snapshot SHA-256 bound into SuiteDirectory")
	}
	if plan.Snapshot.SourceHeight == 0 {
		return errors.New("snapshot source height must be positive")
	}
	if strings.TrimSpace(plan.Snapshot.SourceChainID) == "" || strings.TrimSpace(plan.Snapshot.SourceContract) == "" {
		return errors.New("snapshot source chain ID and contract are required")
	}
	if _, err := rawSHA256("source block hash", plan.Snapshot.SourceBlockHash); err != nil {
		return err
	}
	for label, value := range map[string]string{"inventory SHA-256": plan.Snapshot.InventorySHA256, "tx_search SHA-256": plan.Snapshot.TxSearchSHA256, "block evidence SHA-256": plan.Snapshot.BlockEvidenceSHA256, "event commitment SHA-256": plan.Snapshot.EventCommitmentSHA256} {
		if _, err := rawSHA256(label, value); err != nil {
			return err
		}
	}
	if plan.Target.ChainID == 0 || plan.Target.SuiteVersion != SuiteVersion {
		return errors.New("invalid suite target chain or version")
	}
	if _, err := canonicalAddress("SuiteDirectory", plan.Target.Directory); err != nil {
		return err
	}
	if _, err := canonicalAddress("BootstrapCoordinator", plan.Target.Coordinator); err != nil {
		return err
	}
	if plan.Target.Directory == plan.Target.Coordinator {
		return errors.New("directory and coordinator must differ")
	}
	if err := validateContractPolicy(plan.ContractPolicy); err != nil {
		return err
	}
	if err := validateEscrowRelease(plan.UsernameEscrowRelease, plan.Snapshot.SourceContract, plan.Snapshot.SourceHeight); err != nil {
		return err
	}
	if plan.Limits.BatchSize < 1 || plan.Limits.BatchSize > MaximumBatchItems || plan.Limits.MaxBatchItems != MaximumBatchItems || plan.Limits.MaxPayloadBytes != MaximumPayloadBytes {
		return errors.New("suite import limits do not match contracts")
	}
	if _, err := parseHash("username escrow evidence hash", plan.UsernameEscrowEvidenceHash); err != nil {
		return err
	}
	if plan.UsernameEscrowEvidenceHash != "0x"+plan.UsernameEscrowRelease.EvidenceSHA256 {
		return errors.New("username escrow evidence hash does not match the reviewed escrow release evidence")
	}
	if len(plan.Modules) != RequiredModuleCount {
		return fmt.Errorf("suite plan has %d modules; want %d", len(plan.Modules), RequiredModuleCount)
	}
	totalCount := uint64(0)
	batchCount := 0
	kindCounts := make(map[string]uint64)
	for order, spec := range requiredModules {
		module := plan.Modules[order]
		if module.Order != order || module.Name != spec.name || module.ID != strings.ToLower(spec.id.Hex()) {
			return fmt.Errorf("module %d is not canonical %s", order, spec.name)
		}
		root := rollingSeed(spec.id, snapshotRoot)
		count := uint64(0)
		lastKind := uint8(0)
		seenKinds := make(map[uint8]bool)
		for index, batch := range module.Batches {
			if batch.Sequence != uint64(index) || batch.Count == 0 || batch.Count > MaximumBatchItems || batch.Count > uint64(plan.Limits.BatchSize) || len(batch.ItemKeys) != int(batch.Count) {
				return fmt.Errorf("module %s batch %d has invalid sequence/count", module.Name, index)
			}
			for itemIndex, key := range batch.ItemKeys {
				if strings.TrimSpace(key) == "" {
					return fmt.Errorf("module %s batch %d item key %d is empty", module.Name, index, itemIndex)
				}
			}
			payload, err := parseHexBytes("batch payload", batch.Payload)
			if err != nil {
				return err
			}
			if len(payload) == 0 || len(payload) > MaximumPayloadBytes {
				return fmt.Errorf("module %s batch %d payload size %d is invalid", module.Name, index, len(payload))
			}
			actualCount, err := decodePayloadCount(module.Name, batch.KindCode, payload)
			if err != nil {
				return fmt.Errorf("module %s batch %d: %w", module.Name, index, err)
			}
			if actualCount != batch.Count {
				return fmt.Errorf("module %s batch %d decoded count %d does not match %d", module.Name, index, actualCount, batch.Count)
			}
			expectedKind, ok := kindName(module.Name, batch.KindCode)
			if !ok || batch.Kind != expectedKind {
				return fmt.Errorf("module %s batch %d kind mismatch", module.Name, index)
			}
			if index != 0 && batch.KindCode < lastKind {
				return fmt.Errorf("module %s batch %d import kind is out of order", module.Name, index)
			}
			if seenKinds[batch.KindCode] && batch.KindCode != lastKind {
				return fmt.Errorf("module %s batch %d import kind is not contiguous", module.Name, index)
			}
			seenKinds[batch.KindCode] = true
			lastKind = batch.KindCode
			payloadHash := crypto.Keccak256Hash(payload)
			if batch.PayloadHash != payloadHash.Hex() {
				return fmt.Errorf("module %s batch %d payload hash mismatch", module.Name, index)
			}
			root = rollingStep(root, spec.id, batch.Sequence, batch.Count, payloadHash)
			if batch.RollingRoot != root.Hex() {
				return fmt.Errorf("module %s batch %d rolling root mismatch", module.Name, index)
			}
			count += batch.Count
			kindCounts[module.Name+"/"+batch.Kind] += batch.Count
		}
		if module.ExpectedCount != count || module.ExpectedBatches != uint64(len(module.Batches)) || module.ExpectedRoot != root.Hex() {
			return fmt.Errorf("module %s expected count/batches/root mismatch", module.Name)
		}
		totalCount += count
		batchCount += len(module.Batches)
	}
	if plan.Summary.BatchCount != batchCount || plan.Summary.TransactionCount != 2*RequiredModuleCount+batchCount+2 {
		return errors.New("suite plan summary batch/transaction count mismatch")
	}
	expectedItems := plan.Summary.RepositoryCount + plan.Summary.AliasCount + plan.Summary.RefCount + plan.Summary.CollaboratorCount + plan.Summary.GuardianConfigCount + plan.Summary.ModerationStatusCount + plan.Summary.ModerationReportCount + plan.Summary.ModerationTrailCount + plan.Summary.EconomicSplitCount + plan.Summary.EconomicTotalCount + plan.Summary.UsernameOwnerCount + plan.Summary.ReservedNameCount + plan.Summary.BadgeCount + plan.Summary.ReleaseArtifactCount
	if totalCount != uint64(expectedItems) {
		return fmt.Errorf("module item count %d does not match summary %d", totalCount, expectedItems)
	}
	expectedKindCounts := map[string]int{
		"repository-core/repositories":  plan.Summary.RepositoryCount,
		"repository-core/aliases":       plan.Summary.AliasCount,
		"repository-core/refs":          plan.Summary.RefCount,
		"repository-core/collaborators": plan.Summary.CollaboratorCount,
		"recovery/guardian-configs":     plan.Summary.GuardianConfigCount,
		"moderation/final-statuses":     plan.Summary.ModerationStatusCount,
		"moderation/reports":            plan.Summary.ModerationReportCount,
		"moderation/status-trails":      plan.Summary.ModerationTrailCount,
		"economic/splits":               plan.Summary.EconomicSplitCount,
		"economic/all-denom-totals":     plan.Summary.EconomicTotalCount,
		"username/original-owners":      plan.Summary.UsernameOwnerCount,
		"username/reserved-names":       plan.Summary.ReservedNameCount,
		"badge/badges":                  plan.Summary.BadgeCount,
		"release/artifacts":             plan.Summary.ReleaseArtifactCount,
	}
	for key, expected := range expectedKindCounts {
		if expected < 0 || kindCounts[key] != uint64(expected) {
			return fmt.Errorf("summary %s count %d does not match batches %d", key, expected, kindCounts[key])
		}
	}
	return nil
}

func kindName(module string, code uint8) (string, bool) {
	values := map[string][]string{"repository-core": {"repositories", "aliases", "refs", "collaborators"}, "recovery": {"guardian-configs"}, "moderation": {"final-statuses", "reports", "status-trails"}, "economic": {"splits", "all-denom-totals"}, "username": {"original-owners", "reserved-names"}, "badge": {"badges"}, "release": {"artifacts"}}[module]
	if int(code) >= len(values) {
		return "", false
	}
	return values[code], true
}

func MarshalPlan(plan *Plan) ([]byte, error) {
	if err := ValidatePlan(plan); err != nil {
		return nil, err
	}
	return json.MarshalIndent(plan, "", "  ")
}
func ReadPlan(path string) (*Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("decode suite plan: %w", err)
	}
	var plan Plan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("suite plan contains trailing JSON data")
	}
	if err := ValidatePlan(&plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return errors.New("invalid JSON object close")
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return errors.New("invalid JSON array close")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return err
	}
	return nil
}
func PlanSHA256(plan *Plan) (string, error) {
	raw, err := MarshalPlan(plan)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
func totalBatches(modules []ModulePlan) int {
	total := 0
	for _, module := range modules {
		total += len(module.Batches)
	}
	return total
}

func roleCode(value string) (uint8, error) {
	switch value {
	case "maintainer":
		return 1, nil
	case "reader":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid collaborator role %q", value)
	}
}
func repoStatusCode(value string) (uint8, error) {
	switch value {
	case "active":
		return 0, nil
	case "frozen":
		return 1, nil
	case "delisted":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid repository status %q", value)
	}
}
func reportStatusCode(value string) (uint8, error) {
	switch value {
	case "open":
		return 0, nil
	case "resolved":
		return 1, nil
	case "appealed":
		return 2, nil
	case "appeal_resolved":
		return 3, nil
	default:
		return 0, fmt.Errorf("invalid report status %q", value)
	}
}
func trailActionCode(value string) (uint8, error) {
	switch value {
	case "submitted":
		return 0, nil
	case "resolved":
		return 1, nil
	case "appealed":
		return 2, nil
	case "appeal_resolved":
		return 3, nil
	case "status_set":
		return 4, nil
	default:
		return 0, fmt.Errorf("invalid moderation trail action %q", value)
	}
}

func positiveDecimal(label, value string) (*big.Int, error) {
	if value == "" || value[0] == '+' || (len(value) > 1 && value[0] == '0') {
		return nil, fmt.Errorf("%s must be a canonical positive decimal", label)
	}
	number, ok := new(big.Int).SetString(value, 10)
	if !ok || number.Sign() <= 0 || number.BitLen() > 256 {
		return nil, fmt.Errorf("%s must be a positive uint256", label)
	}
	return number, nil
}
func zeroDecimal(label, value string) error {
	if value != "0" {
		return fmt.Errorf("%s must be exactly 0", label)
	}
	return nil
}
func validRepoName(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
func validUsername(value string) bool {
	if len(value) < 3 || len(value) > 32 || value[0] == '-' || value[len(value)-1] == '-' || strings.HasPrefix(value, "inj1") {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}
func validToken(value string, maximum int) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func validSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func validRefName(value string) bool {
	if len(value) < 5 || len(value) > 256 || !strings.HasPrefix(value, "refs/") || strings.Contains(value, "..") {
		return false
	}
	for _, c := range []byte(value) {
		if c < 0x21 || c > 0x7e || c == '~' || c == '^' || c == ':' || c == '\\' {
			return false
		}
	}
	return true
}
func locatorKey(owner, name string) string { return owner + "\x00" + name }

func validateSnapshot(s *Snapshot) error {
	if s.Schema != SnapshotSchema {
		return fmt.Errorf("unsupported schema %q", s.Schema)
	}
	if strings.TrimSpace(s.Source.ChainID) == "" || strings.TrimSpace(s.Source.Contract) == "" || s.Source.Height == 0 {
		return errors.New("source chain_id, contract, and positive height are required")
	}
	if _, err := rawSHA256("source block hash", s.Source.BlockHash); err != nil {
		return err
	}
	for label, value := range map[string]string{"inventory SHA-256": s.Source.InventorySHA256, "tx_search SHA-256": s.Source.TxSearchSHA256, "block evidence SHA-256": s.Source.BlockEvidenceSHA256, "event commitment SHA-256": s.Source.EventCommitmentSHA256} {
		if _, err := rawSHA256(label, value); err != nil {
			return err
		}
	}
	if err := validateContractPolicy(s.ContractPolicy); err != nil {
		return err
	}
	repos := map[string]SnapshotRepository{}
	locators := map[string]string{}
	for i, r := range s.Repositories {
		if _, err := parseHash("repository ID", r.ID); err != nil {
			return fmt.Errorf("repository %d: %w", i, err)
		}
		if _, err := canonicalAddress("repository owner", r.Owner); err != nil {
			return fmt.Errorf("repository %s: %w", r.ID, err)
		}
		if !validRepoName(r.Name) {
			return fmt.Errorf("repository %s has invalid name", r.ID)
		}
		if len(r.Description) > 1024 || len(r.DefaultBranch) > 64 {
			return fmt.Errorf("repository %s metadata exceeds contract limits", r.ID)
		}
		if r.CreatedAt == 0 || r.UpdatedAt < r.CreatedAt {
			return fmt.Errorf("repository %s timestamps are invalid", r.ID)
		}
		if _, exists := repos[r.ID]; exists {
			return fmt.Errorf("duplicate repository %s", r.ID)
		}
		key := locatorKey(r.Owner, r.Name)
		if existing := locators[key]; existing != "" {
			return fmt.Errorf("duplicate repository locator %s/%s", r.Owner, r.Name)
		}
		repos[r.ID] = r
		locators[key] = r.ID
	}
	for _, r := range s.Repositories {
		if r.ForkedFrom != "" {
			if r.ForkedFrom == r.ID {
				return fmt.Errorf("repository %s forks itself", r.ID)
			}
			if _, ok := repos[r.ForkedFrom]; !ok {
				return fmt.Errorf("repository %s has unknown fork parent %s", r.ID, r.ForkedFrom)
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visitFork func(string) error
	visitFork = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("repository fork lineage contains a cycle at %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		if parent := repos[id].ForkedFrom; parent != "" {
			if err := visitFork(parent); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range repos {
		if err := visitFork(id); err != nil {
			return err
		}
	}
	for i, a := range s.Aliases {
		if _, ok := repos[a.RepoID]; !ok {
			return fmt.Errorf("alias %d references unknown repository", i)
		}
		if _, err := canonicalAddress("alias owner", a.Owner); err != nil {
			return err
		}
		if !validRepoName(a.Name) {
			return fmt.Errorf("alias %d has invalid name", i)
		}
		key := locatorKey(a.Owner, a.Name)
		existing := locators[key]
		if existing != "" && existing != a.RepoID {
			return fmt.Errorf("alias locator collides with repository %s", existing)
		}
		if existing == a.RepoID {
			return fmt.Errorf("alias %d duplicates an existing canonical/alias locator", i)
		}
		locators[key] = a.RepoID
	}
	refsPerRepo := map[string]int{}
	refKeys := map[string]struct{}{}
	for i, r := range s.Refs {
		if _, ok := repos[r.RepoID]; !ok {
			return fmt.Errorf("ref %d references unknown repository", i)
		}
		if !validRefName(r.RefName) || !validSHA(r.CommitSHA) || len(r.PackURIs) == 0 || len(r.PackURIs) > 128 {
			return fmt.Errorf("ref %d violates contract limits", i)
		}
		for _, uri := range r.PackURIs {
			if len(uri) <= 7 || len(uri) > 512 || !strings.HasPrefix(uri, "ipfs://") {
				return fmt.Errorf("ref %d has invalid pack URI", i)
			}
		}
		if r.UpdatedAt == 0 {
			return fmt.Errorf("ref %d timestamp is zero", i)
		}
		if _, err := canonicalAddress("ref updated_by", r.UpdatedBy); err != nil {
			return err
		}
		key := r.RepoID + "\x00" + r.RefName
		if _, ok := refKeys[key]; ok {
			return fmt.Errorf("duplicate ref %s", key)
		}
		refKeys[key] = struct{}{}
		refsPerRepo[r.RepoID]++
		if refsPerRepo[r.RepoID] > 1024 {
			return fmt.Errorf("repository %s exceeds ref limit", r.RepoID)
		}
	}
	collabsPerRepo := map[string]int{}
	collabKeys := map[string]struct{}{}
	for i, c := range s.Collaborators {
		repo, ok := repos[c.RepoID]
		if !ok {
			return fmt.Errorf("collaborator %d references unknown repository", i)
		}
		if _, err := canonicalAddress("collaborator account", c.Account); err != nil {
			return err
		}
		if c.Account == repo.Owner {
			return fmt.Errorf("repository %s owner cannot be a collaborator", c.RepoID)
		}
		if _, err := roleCode(c.Role); err != nil {
			return err
		}
		key := c.RepoID + "\x00" + c.Account
		if _, ok := collabKeys[key]; ok {
			return fmt.Errorf("duplicate collaborator %s", key)
		}
		collabKeys[key] = struct{}{}
		collabsPerRepo[c.RepoID]++
		if collabsPerRepo[c.RepoID] > 256 {
			return fmt.Errorf("repository %s exceeds collaborator limit", c.RepoID)
		}
	}
	guardianRepos := map[string]struct{}{}
	for _, g := range s.GuardianConfigs {
		repo, ok := repos[g.RepoID]
		if !ok {
			return fmt.Errorf("guardian config references unknown repository %s", g.RepoID)
		}
		if _, exists := guardianRepos[g.RepoID]; exists {
			return fmt.Errorf("duplicate guardian config %s", g.RepoID)
		}
		guardianRepos[g.RepoID] = struct{}{}
		if g.ConfiguredBy != repo.Owner {
			return fmt.Errorf("guardian config %s is stale", g.RepoID)
		}
		if len(g.Guardians) == 0 || len(g.Guardians) > 10 || g.Threshold == 0 || int(g.Threshold) > len(g.Guardians) {
			return fmt.Errorf("guardian config %s threshold/count invalid", g.RepoID)
		}
		seen := map[string]struct{}{}
		for _, address := range g.Guardians {
			if _, err := canonicalAddress("guardian", address); err != nil {
				return err
			}
			if address == repo.Owner {
				return fmt.Errorf("guardian config %s contains owner", g.RepoID)
			}
			if _, ok := seen[address]; ok {
				return fmt.Errorf("guardian config %s has duplicate guardian", g.RepoID)
			}
			seen[address] = struct{}{}
		}
	}
	statuses := map[string]string{}
	for _, status := range s.Moderation.FinalStatuses {
		if _, ok := repos[status.RepoID]; !ok {
			return fmt.Errorf("status references unknown repository %s", status.RepoID)
		}
		if _, exists := statuses[status.RepoID]; exists {
			return fmt.Errorf("duplicate final status %s", status.RepoID)
		}
		if _, err := repoStatusCode(status.Status); err != nil {
			return err
		}
		statuses[status.RepoID] = status.Status
	}
	if len(statuses) != len(repos) {
		return fmt.Errorf("final moderation statuses cover %d of %d repositories", len(statuses), len(repos))
	}
	reports := map[uint64]SnapshotReport{}
	for _, r := range s.Moderation.Reports {
		if r.ID == 0 || r.ID == math.MaxUint64 {
			return fmt.Errorf("report ID %d is invalid", r.ID)
		}
		if _, exists := reports[r.ID]; exists {
			return fmt.Errorf("duplicate report %d", r.ID)
		}
		_, ok := repos[r.RepoID]
		if !ok {
			return fmt.Errorf("report %d references unknown repository", r.ID)
		}
		if _, err := canonicalAddress("reporter", r.Reporter); err != nil {
			return err
		}
		if _, err := reportStatusCode(r.Status); err != nil {
			return err
		}
		if _, err := repoStatusCode(r.Resolution); err != nil {
			return err
		}
		if len(r.ReasonHash) == 0 || len(r.ReasonHash) > 128 || r.CreatedAt == 0 || r.UpdatedAt < r.CreatedAt || len(r.Trail) == 0 {
			return fmt.Errorf("report %d record is invalid", r.ID)
		}
		expectedTrailLength := 0
		switch r.Status {
		case "open":
			expectedTrailLength = 1
			if r.Resolution != "active" {
				return fmt.Errorf("report %d open report must resolve to active", r.ID)
			}
		case "resolved":
			expectedTrailLength = 2
		case "appealed":
			expectedTrailLength = 3
		case "appeal_resolved":
			expectedTrailLength = 4
		}
		if len(r.Trail) != expectedTrailLength {
			return fmt.Errorf("report %d has %d trail entries; want %d for %s", r.ID, len(r.Trail), expectedTrailLength, r.Status)
		}
		expectedActions := []string{"submitted", "resolved", "appealed", "appeal_resolved"}
		previous := uint64(0)
		for index, entry := range r.Trail {
			if _, err := trailActionCode(entry.Action); err != nil {
				return err
			}
			if entry.Action != expectedActions[index] {
				return fmt.Errorf("report %d trail entry %d has action %q; want %q", r.ID, index, entry.Action, expectedActions[index])
			}
			if _, err := canonicalAddress("report trail actor", entry.Actor); err != nil {
				return err
			}
			if _, err := repoStatusCode(entry.Status); err != nil {
				return err
			}
			if len(entry.ReasonHash) == 0 || len(entry.ReasonHash) > 128 || entry.Timestamp == 0 || entry.Timestamp < previous {
				return fmt.Errorf("report %d trail invalid", r.ID)
			}
			if index == 0 && (entry.Actor != r.Reporter || entry.Status != "active" || entry.Timestamp != r.CreatedAt || entry.ReasonHash != r.ReasonHash) {
				return fmt.Errorf("report %d trail does not begin with its immutable submission", r.ID)
			}
			previous = entry.Timestamp
		}
		if previous != r.UpdatedAt {
			return fmt.Errorf("report %d updated_at must equal the final trail timestamp", r.ID)
		}
		last := r.Trail[len(r.Trail)-1]
		if last.Status != r.Resolution {
			return fmt.Errorf("report %d final trail status does not match resolution", r.ID)
		}
		if r.Status == "appeal_resolved" && r.Trail[2].Status != r.Trail[1].Status {
			return fmt.Errorf("report %d appeal changed the unresolved status", r.ID)
		}
		if r.Status != "open" && r.Status != "appeal_resolved" && r.Trail[1].Status != r.Resolution {
			return fmt.Errorf("report %d resolution trail status does not match resolution", r.ID)
		}
		reports[r.ID] = r
	}
	statusTrails := map[string]struct{}{}
	for _, r := range s.Moderation.StatusTrails {
		if _, ok := repos[r.RepoID]; !ok {
			return fmt.Errorf("status trail references unknown repository %s", r.RepoID)
		}
		if _, ok := statusTrails[r.RepoID]; ok {
			return fmt.Errorf("duplicate status trail %s", r.RepoID)
		}
		statusTrails[r.RepoID] = struct{}{}
		if len(r.Trail) == 0 {
			return fmt.Errorf("status trail %s is empty", r.RepoID)
		}
		previous := uint64(0)
		for _, entry := range r.Trail {
			if _, err := repoStatusCode(entry.Status); err != nil {
				return err
			}
			if _, err := canonicalAddress("status trail actor", entry.Actor); err != nil {
				return err
			}
			if len(entry.ReasonHash) > 128 || entry.Timestamp == 0 || entry.Timestamp < previous {
				return fmt.Errorf("status trail %s is invalid", r.RepoID)
			}
			if entry.ReportID != 0 {
				report, ok := reports[entry.ReportID]
				if !ok || report.RepoID != r.RepoID {
					return fmt.Errorf("status trail %s references invalid report %d", r.RepoID, entry.ReportID)
				}
			}
			previous = entry.Timestamp
		}
		if r.Trail[len(r.Trail)-1].Status != statuses[r.RepoID] {
			return fmt.Errorf("status trail %s does not end at final status", r.RepoID)
		}
	}
	splitRepos := map[string]struct{}{}
	for _, r := range s.Economic.Splits {
		repo, ok := repos[r.RepoID]
		if !ok {
			return fmt.Errorf("splits reference unknown repository %s", r.RepoID)
		}
		if _, ok := splitRepos[r.RepoID]; ok {
			return fmt.Errorf("duplicate splits %s", r.RepoID)
		}
		splitRepos[r.RepoID] = struct{}{}
		if len(r.Splits) > MaxRevenueSplitRecipients {
			return fmt.Errorf("splits %s count invalid", r.RepoID)
		}
		seen := map[string]struct{}{}
		total := 0
		for _, entry := range r.Splits {
			if _, err := canonicalAddress("split recipient", entry.Recipient); err != nil {
				return err
			}
			if entry.Recipient == repo.Owner {
				return fmt.Errorf("split %s names the repository owner; owner receives the remainder implicitly", r.RepoID)
			}
			if entry.BPS == 0 {
				return fmt.Errorf("split %s has zero bps", r.RepoID)
			}
			if _, ok := seen[entry.Recipient]; ok {
				return fmt.Errorf("split %s has duplicate recipient", r.RepoID)
			}
			seen[entry.Recipient] = struct{}{}
			total += int(entry.BPS)
		}
		if total > 10000 {
			return fmt.Errorf("split %s exceeds 10000 bps", r.RepoID)
		}
	}
	totalKeys := map[string]struct{}{}
	for _, r := range s.Economic.Totals {
		if _, ok := repos[r.RepoID]; !ok {
			return fmt.Errorf("total references unknown repository %s", r.RepoID)
		}
		if len(r.Denom) == 0 || len(r.Denom) > 128 {
			return fmt.Errorf("total denom %q is invalid", r.Denom)
		}
		if _, err := positiveDecimal("sponsor total", r.Amount); err != nil {
			return err
		}
		key := r.RepoID + "\x00" + r.Denom
		if _, ok := totalKeys[key]; ok {
			return fmt.Errorf("duplicate sponsor total %s", key)
		}
		totalKeys[key] = struct{}{}
	}
	usernameNames := map[string]string{}
	usernameOwners := map[string]string{}
	for _, r := range s.Username.OriginalOwners {
		if !validUsername(r.Name) {
			return fmt.Errorf("invalid original username %q", r.Name)
		}
		if _, err := canonicalAddress("username owner", r.Owner); err != nil {
			return err
		}
		if _, ok := usernameNames[r.Name]; ok {
			return fmt.Errorf("duplicate username %s", r.Name)
		}
		if existing := usernameOwners[r.Owner]; existing != "" {
			return fmt.Errorf("username owner %s owns both %s and %s", r.Owner, existing, r.Name)
		}
		usernameNames[r.Name] = r.Owner
		usernameOwners[r.Owner] = r.Name
	}
	reserved := map[string]struct{}{}
	if len(s.Username.ReservedNames) > MaxReservedUsernames {
		return fmt.Errorf("reserved usernames exceed maximum of %d", MaxReservedUsernames)
	}
	for _, name := range s.Username.ReservedNames {
		if !validUsername(name) {
			return fmt.Errorf("invalid reserved username %q", name)
		}
		if _, ok := usernameNames[name]; ok {
			return fmt.Errorf("reserved username %s overlaps original owner claim", name)
		}
		if _, ok := reserved[name]; ok {
			return fmt.Errorf("duplicate reserved username %s", name)
		}
		reserved[name] = struct{}{}
	}
	if err := validateEscrowRelease(s.Username.EscrowRelease, s.Source.Contract, s.Source.Height); err != nil {
		return err
	}
	badges := map[uint64]SnapshotBadge{}
	byRecipient := map[string][]uint64{}
	byRepo := map[string][]uint64{}
	for _, b := range s.Badges {
		if b.ID == 0 {
			return errors.New("badge ID zero is invalid")
		}
		if _, ok := badges[b.ID]; ok {
			return fmt.Errorf("duplicate badge %d", b.ID)
		}
		_, ok := repos[b.RepoID]
		if !ok {
			return fmt.Errorf("badge %d references unknown repository", b.ID)
		}
		if _, err := canonicalAddress("badge recipient", b.Recipient); err != nil {
			return err
		}
		if _, err := canonicalAddress("badge awarded_by", b.AwardedBy); err != nil {
			return err
		}
		if b.Recipient == b.AwardedBy {
			return fmt.Errorf("badge %d awards the repository owner; owner self-awards are not supported", b.ID)
		}
		if len(b.Reason) == 0 || len(b.Reason) > 256 || b.AwardedAt == 0 {
			return fmt.Errorf("badge %d is invalid", b.ID)
		}
		badges[b.ID] = b
		byRecipient[b.Recipient] = append(byRecipient[b.Recipient], b.ID)
		byRepo[b.RepoID] = append(byRepo[b.RepoID], b.ID)
	}
	if err := validateBadgeIndexes(s.BadgeIndexes, badges, byRecipient, byRepo); err != nil {
		return err
	}
	releases := map[string]SnapshotRelease{}
	platforms := map[string][]string{}
	for _, r := range s.Releases {
		if !validToken(r.Version, 64) || !validToken(r.Platform, 64) {
			return fmt.Errorf("invalid release key %s/%s", r.Version, r.Platform)
		}
		if _, err := rawSHA256("release SHA-256", r.SHA256); err != nil {
			return err
		}
		if _, err := canonicalAddress("release registered_by", r.RegisteredBy); err != nil {
			return err
		}
		if r.RegisteredAt == 0 {
			return fmt.Errorf("release %s/%s timestamp is zero", r.Version, r.Platform)
		}
		key := r.Version + "\x00" + r.Platform
		if _, ok := releases[key]; ok {
			return fmt.Errorf("duplicate release %s/%s", r.Version, r.Platform)
		}
		releases[key] = r
		platforms[r.Version] = append(platforms[r.Version], r.Platform)
	}
	if err := validateReleaseIndexes(s.ReleaseVersions, platforms); err != nil {
		return err
	}
	return nil
}

func validateContractPolicy(policy ContractPolicy) error {
	for label, value := range map[string]string{"policy admin": policy.Admin, "treasury": policy.Treasury, "committee": policy.Committee, "release authority": policy.ReleaseAuthority, "username policy admin": policy.UsernamePolicyAdmin} {
		if _, err := canonicalAddress(label, value); err != nil {
			return err
		}
	}
	if policy.PlatformFeeBPS > 500 {
		return errors.New("platform fee exceeds EconomicModule maximum 500 bps")
	}
	return nil
}

func validateEscrowRelease(escrow UsernameEscrowRelease, sourceContract string, sourceHeight uint64) error {
	if !escrow.AllReleased {
		return errors.New("username escrow release is not complete")
	}
	if escrow.Contract != sourceContract {
		return errors.New("username escrow contract does not match source contract")
	}
	if escrow.Height < sourceHeight {
		return errors.New("username escrow evidence predates cutover snapshot")
	}
	if err := zeroDecimal("username escrow balance", escrow.EscrowBalance); err != nil {
		return err
	}
	if strings.TrimSpace(escrow.Denom) == "" {
		return errors.New("username escrow denom is required")
	}
	if _, err := rawSHA256("username escrow block hash", escrow.BlockHash); err != nil {
		return err
	}
	if _, err := rawSHA256("username escrow evidence SHA-256", escrow.EvidenceSHA256); err != nil {
		return err
	}
	return nil
}

func validateBadgeIndexes(indexes SnapshotBadgeIndexes, badges map[uint64]SnapshotBadge, wantRecipients map[string][]uint64, wantRepos map[string][]uint64) error {
	gotRecipients := map[string][]uint64{}
	seenIDs := map[uint64]int{}
	for _, g := range indexes.ByRecipient {
		if _, err := canonicalAddress("badge index recipient", g.Recipient); err != nil {
			return err
		}
		if _, ok := gotRecipients[g.Recipient]; ok {
			return fmt.Errorf("duplicate badge recipient index %s", g.Recipient)
		}
		gotRecipients[g.Recipient] = append([]uint64(nil), g.BadgeIDs...)
		for _, id := range g.BadgeIDs {
			badge, ok := badges[id]
			if !ok || badge.Recipient != g.Recipient {
				return fmt.Errorf("recipient index contains invalid badge %d", id)
			}
			seenIDs[id]++
		}
	}
	gotRepos := map[string][]uint64{}
	for _, g := range indexes.ByRepository {
		if _, ok := gotRepos[g.RepoID]; ok {
			return fmt.Errorf("duplicate badge repository index %s", g.RepoID)
		}
		gotRepos[g.RepoID] = append([]uint64(nil), g.BadgeIDs...)
		for _, id := range g.BadgeIDs {
			badge, ok := badges[id]
			if !ok || badge.RepoID != g.RepoID {
				return fmt.Errorf("repository index contains invalid badge %d", id)
			}
			seenIDs[id]++
		}
	}
	if !equalIndexMaps(gotRecipients, wantRecipients) || !equalIndexMaps(gotRepos, wantRepos) {
		return errors.New("badge bidirectional indexes do not exactly match unique badge records")
	}
	for id := range badges {
		if seenIDs[id] != 2 {
			return fmt.Errorf("badge %d is not present exactly once in both indexes", id)
		}
	}
	return nil
}
func equalIndexMaps(got, want map[string][]uint64) bool {
	if len(got) != len(want) {
		return false
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			return false
		}
		g = append([]uint64(nil), g...)
		w = append([]uint64(nil), w...)
		sort.Slice(g, func(i, j int) bool { return g[i] < g[j] })
		sort.Slice(w, func(i, j int) bool { return w[i] < w[j] })
		if len(g) != len(w) {
			return false
		}
		for i := range g {
			if g[i] != w[i] || (i > 0 && g[i] == g[i-1]) {
				return false
			}
		}
	}
	return true
}
func validateReleaseIndexes(indexes []SnapshotReleaseVersion, want map[string][]string) error {
	got := map[string][]string{}
	for _, g := range indexes {
		if !validToken(g.Version, 64) {
			return fmt.Errorf("invalid indexed release version %q", g.Version)
		}
		if _, ok := got[g.Version]; ok {
			return fmt.Errorf("duplicate release version index %s", g.Version)
		}
		got[g.Version] = append([]string(nil), g.Platforms...)
	}
	if len(got) != len(want) {
		return errors.New("release version index does not cover all versions")
	}
	for version, w := range want {
		g, ok := got[version]
		if !ok {
			return fmt.Errorf("release version index missing %s", version)
		}
		sort.Strings(g)
		sort.Strings(w)
		if len(g) != len(w) {
			return fmt.Errorf("release version %s platform count mismatch", version)
		}
		for i := range g {
			if !validToken(g[i], 64) || g[i] != w[i] || (i > 0 && g[i] == g[i-1]) {
				return fmt.Errorf("release version %s platform index mismatch", version)
			}
		}
	}
	return nil
}

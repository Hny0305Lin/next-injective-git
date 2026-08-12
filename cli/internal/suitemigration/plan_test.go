package suitemigration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	testDirectory    = "0x1000000000000000000000000000000000000001"
	testCoordinator  = "0x2000000000000000000000000000000000000002"
	testAdmin        = "0x3000000000000000000000000000000000000003"
	testOwner        = "0x4000000000000000000000000000000000000004"
	testGuardian     = "0x5000000000000000000000000000000000000005"
	testReporter     = "0x6000000000000000000000000000000000000006"
	testRecipient    = "0x7000000000000000000000000000000000000007"
	testCollaborator = "0x8000000000000000000000000000000000000008"
	testRepoID       = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestBuildPlanCoversSuiteAndMatchesContractRollingRoot(t *testing.T) {
	raw := fixtureSnapshotJSON(t)
	plan, err := BuildPlan(raw, BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Modules) != RequiredModuleCount || plan.Modules[0].Name != "repository-core" || plan.Modules[6].Name != "release" {
		t.Fatalf("module order = %#v", plan.Modules)
	}
	digest := sha256.Sum256(raw)
	if plan.Snapshot.SHA256 != hex.EncodeToString(digest[:]) || plan.Snapshot.Root != common.BytesToHash(digest[:]).Hex() {
		t.Fatalf("snapshot digest/root = %s / %s", plan.Snapshot.SHA256, plan.Snapshot.Root)
	}
	for index, module := range plan.Modules {
		spec := requiredModules[index]
		root := rollingSeed(spec.id, common.BytesToHash(digest[:]))
		for _, batch := range module.Batches {
			payload, err := parseHexBytes("payload", batch.Payload)
			if err != nil {
				t.Fatal(err)
			}
			root = rollingStep(root, spec.id, batch.Sequence, batch.Count, crypto.Keccak256Hash(payload))
		}
		if module.ExpectedRoot != root.Hex() {
			t.Fatalf("%s root = %s, want %s", module.Name, module.ExpectedRoot, root.Hex())
		}
	}
	wantSummary := PlanSummary{RepositoryCount: 1, AliasCount: 1, RefCount: 1, CollaboratorCount: 1, GuardianConfigCount: 1, ModerationStatusCount: 1, ModerationReportCount: 1, ModerationTrailCount: 1, EconomicSplitCount: 1, EconomicTotalCount: 2, UsernameOwnerCount: 1, ReservedNameCount: 1, BadgeCount: 1, ReleaseArtifactCount: 1}
	if plan.Summary.RepositoryCount != wantSummary.RepositoryCount || plan.Summary.EconomicTotalCount != 2 || plan.Summary.BadgeCount != 1 || plan.Summary.ReleaseArtifactCount != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if err := ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
}

func TestBuildManifestUsesStrictCoordinatorOrderAndCanonicalABI(t *testing.T) {
	plan, err := BuildPlan(fixtureSnapshotJSON(t), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator, BatchSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildCalldataManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Transactions) != plan.Summary.TransactionCount {
		t.Fatalf("transaction count = %d", len(manifest.Transactions))
	}
	wantPhases := []string{"begin_module"}
	for _, tx := range manifest.Transactions {
		method, _, err := calldataMethod(tx.Data)
		if err != nil {
			t.Fatal(err)
		}
		if tx.Function != method.Sig || tx.To != testCoordinator {
			t.Fatalf("transaction = %#v, method=%s", tx, method.Sig)
		}
	}
	if manifest.Transactions[0].Phase != wantPhases[0] || manifest.Transactions[0].Module != "repository-core" {
		t.Fatalf("first transaction = %#v", manifest.Transactions[0])
	}
	seenEscrow := 0
	for index, tx := range manifest.Transactions {
		if tx.Phase == "attest_username_escrow" {
			seenEscrow++
			if index == 0 || manifest.Transactions[index-1].Module != "username" || manifest.Transactions[index+1].Phase != "finalize_module" {
				t.Fatalf("escrow transaction order = %d", index)
			}
		}
	}
	if seenEscrow != 1 || manifest.Transactions[len(manifest.Transactions)-1].Phase != "activate_suite" {
		t.Fatalf("escrow/activation = %d / %#v", seenEscrow, manifest.Transactions[len(manifest.Transactions)-1])
	}
	if err := ValidateCalldataManifest(plan, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestPlanTamperingFailsClosed(t *testing.T) {
	plan, err := BuildPlan(fixtureSnapshotJSON(t), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator, BatchSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	plan.Modules[0].Batches[0].PayloadHash = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := ValidatePlan(plan); err == nil || !strings.Contains(err.Error(), "payload hash mismatch") {
		t.Fatalf("tamper error = %v", err)
	}
}

func TestSnapshotRejectsDuplicateJSONKeys(t *testing.T) {
	raw := fixtureSnapshotJSON(t)
	raw = bytes.Replace(raw, []byte(`"schema":"`), []byte(`"schema":"igit.cosmwasm-v1.suite-snapshot.v1","schema":"`), 1)
	if _, err := BuildPlan(raw, BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate key error = %v", err)
	}
}

func TestSnapshotRejectsNonZeroEscrowAndBadgeIndexMismatch(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Username.EscrowRelease.EscrowBalance = "1"
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "exactly 0") {
		t.Fatalf("escrow error = %v", err)
	}
	snapshot.Username.EscrowRelease.EscrowBalance = "0"
	snapshot.BadgeIndexes.ByRecipient[0].BadgeIDs = nil
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "indexes") {
		t.Fatalf("badge index error = %v", err)
	}
}

func TestPlannerSplitsBatchesByEncodedPayloadSize(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Refs = make([]SnapshotRef, 128)
	for index := range snapshot.Refs {
		snapshot.Refs[index] = SnapshotRef{RepoID: testRepoID, RefName: "refs/heads/" + strings.Repeat("a", 230) + strconv.Itoa(index), CommitSHA: strings.Repeat("a", 40), PackURIs: []string{"ipfs://" + strings.Repeat("a", 505)}, UpdatedAt: 20, UpdatedBy: testOwner}
	}
	plan, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator, BatchSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	refBatches := 0
	for _, batch := range plan.Modules[0].Batches {
		if batch.Kind == "refs" {
			refBatches++
			payload, _ := parseHexBytes("payload", batch.Payload)
			if len(payload) > MaximumPayloadBytes {
				t.Fatalf("payload bytes = %d", len(payload))
			}
		}
	}
	if refBatches < 2 {
		t.Fatalf("large refs were not split by payload bytes: %d ref batches", refBatches)
	}
}

func fixtureSnapshotJSON(t *testing.T) []byte {
	t.Helper()
	h := strings.Repeat("1", 64)
	snapshot := Snapshot{
		Schema:          SnapshotSchema,
		Source:          SnapshotSource{ChainID: "injective-888", Contract: "inj1contract", Height: 100, BlockHash: h, InventorySHA256: strings.Repeat("2", 64), TxSearchSHA256: strings.Repeat("3", 64), BlockEvidenceSHA256: strings.Repeat("4", 64), EventCommitmentSHA256: strings.Repeat("5", 64)},
		ContractPolicy:  ContractPolicy{Admin: testAdmin, Treasury: testAdmin, Committee: testAdmin, ReleaseAuthority: testAdmin, UsernamePolicyAdmin: testAdmin, PlatformFeeBPS: 300},
		Repositories:    []SnapshotRepository{{ID: testRepoID, Owner: testOwner, Name: "demo", Description: "repository", DefaultBranch: "main", CreatedAt: 10, UpdatedAt: 20}},
		Aliases:         []SnapshotAlias{{RepoID: testRepoID, Owner: testAdmin, Name: "historical"}},
		Refs:            []SnapshotRef{{RepoID: testRepoID, RefName: "refs/heads/main", CommitSHA: strings.Repeat("a", 40), PackURIs: []string{"ipfs://cid"}, UpdatedAt: 20, UpdatedBy: testOwner}},
		Collaborators:   []SnapshotCollaborator{{RepoID: testRepoID, Account: testCollaborator, Role: "maintainer"}},
		GuardianConfigs: []SnapshotGuardianConfig{{RepoID: testRepoID, ConfiguredBy: testOwner, Threshold: 1, Guardians: []string{testGuardian}}},
		Moderation:      SnapshotModeration{FinalStatuses: []SnapshotFinalStatus{{RepoID: testRepoID, Status: "frozen"}}, Reports: []SnapshotReport{{ID: 7, RepoID: testRepoID, Reporter: testReporter, Status: "resolved", Resolution: "frozen", ReasonHash: "decision", CreatedAt: 11, UpdatedAt: 13, Trail: []SnapshotReportTrail{{Action: "submitted", Actor: testReporter, Status: "active", ReasonHash: "report", Timestamp: 11}, {Action: "resolved", Actor: testAdmin, Status: "frozen", ReasonHash: "decision", Timestamp: 13}}}}, StatusTrails: []SnapshotStatusTrail{{RepoID: testRepoID, Trail: []SnapshotRepositoryStatusLog{{Status: "frozen", Actor: testAdmin, ReasonHash: "decision", Timestamp: 13, ReportID: 7}}}}},
		Economic:        SnapshotEconomic{Splits: []SnapshotRevenueSplits{{RepoID: testRepoID, Splits: []SnapshotSplit{{Recipient: testRecipient, BPS: 1250}}}}, Totals: []SnapshotSponsorTotal{{RepoID: testRepoID, Denom: "inj", Amount: "100"}, {RepoID: testRepoID, Denom: "factory/old", Amount: "200"}}},
		Username:        SnapshotUsername{OriginalOwners: []SnapshotOriginalUsername{{Name: "alice", Owner: testOwner}}, ReservedNames: []string{"admin"}, EscrowRelease: UsernameEscrowRelease{AllReleased: true, Contract: "inj1contract", Height: 101, BlockHash: strings.Repeat("6", 64), Denom: "inj", EscrowBalance: "0", EvidenceSHA256: strings.Repeat("7", 64)}},
		Badges:          []SnapshotBadge{{ID: 9, RepoID: testRepoID, Recipient: testRecipient, AwardedBy: testOwner, Reason: "maintainer", AwardedAt: 14}},
		BadgeIndexes:    SnapshotBadgeIndexes{ByRecipient: []SnapshotBadgeRecipientIndex{{Recipient: testRecipient, BadgeIDs: []uint64{9}}}, ByRepository: []SnapshotBadgeRepositoryIndex{{RepoID: testRepoID, BadgeIDs: []uint64{9}}}},
		Releases:        []SnapshotRelease{{Version: "v1.0.0", Platform: "linux-amd64", SHA256: strings.Repeat("8", 64), RegisteredBy: testAdmin, RegisteredAt: 15}}, ReleaseVersions: []SnapshotReleaseVersion{{Version: "v1.0.0", Platforms: []string{"linux-amd64"}}},
	}
	return mustJSON(t, snapshot)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

package suitemigration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func TestSnapshotValidationMatchesV1IdentityAndPolicyRules(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Repositories[0].Name = "Demo.Repo_2"
	snapshot.Refs[0].CommitSHA = strings.Repeat("A", 40)
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("V1-compatible uppercase identity was rejected: %v", err)
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{name: "dangerous ref traversal", mutate: func(value *Snapshot) { value.Refs[0].RefName = "refs/heads/a..b" }, want: "ref 0"},
		{name: "dangerous ref control", mutate: func(value *Snapshot) { value.Refs[0].RefName = "refs/heads/a~b" }, want: "ref 0"},
		{name: "address-like username", mutate: func(value *Snapshot) { value.Username.OriginalOwners[0].Name = "inj1abc" }, want: "invalid original username"},
		{name: "owner split recipient", mutate: func(value *Snapshot) { value.Economic.Splits[0].Splits[0].Recipient = testOwner }, want: "repository owner"},
		{name: "owner badge recipient", mutate: func(value *Snapshot) { value.Badges[0].Recipient = testOwner }, want: "self-awards"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var candidate Snapshot
			if err := json.Unmarshal(fixtureSnapshotJSON(t), &candidate); err != nil {
				t.Fatal(err)
			}
			testCase.mutate(&candidate)
			if _, err := BuildPlan(mustJSON(t, candidate), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validation error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestSnapshotAllowsHistoricalBadgeRecipientAfterOwnershipTransfer(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	// The badge recipient became the current owner only after the award. The
	// award-time owner is preserved in AwardedBy and remains the self-award
	// authority check.
	snapshot.Repositories[0].Owner = testRecipient
	snapshot.GuardianConfigs[0].ConfiguredBy = testRecipient
	snapshot.Economic.Splits[0].Splits[0].Recipient = testCollaborator
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("historical badge was rejected after ownership transfer: %v", err)
	}
}

func TestSnapshotAllowsHistoricalAppealAfterOwnershipTransfer(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	// Snapshot repository ownership is current state. The appeal was submitted
	// before the transfer, so its actor is a valid historical owner rather than
	// the current owner.
	snapshot.Repositories[0].Owner = testRecipient
	snapshot.GuardianConfigs[0].ConfiguredBy = testRecipient
	snapshot.Economic.Splits[0].Splits[0].Recipient = testCollaborator
	report := &snapshot.Moderation.Reports[0]
	report.Status = "appealed"
	report.UpdatedAt = 14
	report.Trail = append(report.Trail, SnapshotReportTrail{
		Action: "appealed", Actor: testOwner, Status: "frozen", ReasonHash: "appeal", Timestamp: 14,
	})
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("historical appeal was rejected after ownership transfer: %v", err)
	}
}

func TestSnapshotRevenueSplitsMatchV1TwentyRecipientLimit(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Economic.Splits[0].Splits = make([]SnapshotSplit, MaxRevenueSplitRecipients)
	for index := range snapshot.Economic.Splits[0].Splits {
		snapshot.Economic.Splits[0].Splits[index] = SnapshotSplit{
			Recipient: fmt.Sprintf("0x%040x", index+0x9000),
			BPS:       1,
		}
	}
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("%d V1-compatible split recipients were rejected: %v", MaxRevenueSplitRecipients, err)
	}

	snapshot.Economic.Splits[0].Splits = append(snapshot.Economic.Splits[0].Splits, SnapshotSplit{
		Recipient: fmt.Sprintf("0x%040x", 0xA000),
		BPS:       1,
	})
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "count invalid") {
		t.Fatalf("%d split recipients validation error = %v", MaxRevenueSplitRecipients+1, err)
	}

	snapshot.Economic.Splits[0].Splits = nil
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("empty V1 split record was rejected: %v", err)
	}
}

func TestSnapshotReservedUsernamesMatchV1Limit(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Username.ReservedNames = make([]string, MaxReservedUsernames)
	for index := range snapshot.Username.ReservedNames {
		snapshot.Username.ReservedNames[index] = fmt.Sprintf("reserved-%03d", index)
	}
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err != nil {
		t.Fatalf("%d V1-compatible reserved usernames were rejected: %v", MaxReservedUsernames, err)
	}

	snapshot.Username.ReservedNames = append(snapshot.Username.ReservedNames, "reserved-128")
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "exceed maximum of 128") {
		t.Fatalf("%d reserved usernames validation error = %v", MaxReservedUsernames+1, err)
	}
}

func TestSnapshotForkLineageIsAcyclicAndImportedParentFirst(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal(fixtureSnapshotJSON(t), &snapshot); err != nil {
		t.Fatal(err)
	}
	parent := snapshot.Repositories[0]
	child := parent
	child.ID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	child.Owner = testRecipient
	child.Name = "child"
	child.ForkedFrom = parent.ID
	snapshot.Repositories = []SnapshotRepository{child, parent}
	snapshot.Moderation.FinalStatuses = append(snapshot.Moderation.FinalStatuses, SnapshotFinalStatus{
		RepoID: child.ID, Status: "active",
	})
	plan, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator})
	if err != nil {
		t.Fatalf("valid parent/child lineage was rejected: %v", err)
	}
	if got := plan.Modules[0].Batches[0].ItemKeys; len(got) != 2 || got[0] != parent.ID || got[1] != child.ID {
		t.Fatalf("repository import order = %#v", got)
	}

	snapshot.Repositories[0].ForkedFrom = parent.ID
	snapshot.Repositories[1].ForkedFrom = child.ID
	if _, err := BuildPlan(mustJSON(t, snapshot), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), "fork lineage contains a cycle") {
		t.Fatalf("cycle validation error = %v", err)
	}
}

func TestSnapshotRejectsMalformedModerationReportTrails(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{
			name: "wrong trail length",
			mutate: func(value *Snapshot) {
				value.Moderation.Reports[0].Trail = append(value.Moderation.Reports[0].Trail, SnapshotReportTrail{
					Action: "appealed", Actor: testOwner, Status: "frozen", ReasonHash: "appeal", Timestamp: 14,
				})
			},
			want: "has 3 trail entries",
		},
		{
			name: "status set action",
			mutate: func(value *Snapshot) {
				value.Moderation.Reports[0].Trail[0].Action = "status_set"
			},
			want: "want \"submitted\"",
		},
		{
			name: "empty decision reason",
			mutate: func(value *Snapshot) {
				value.Moderation.Reports[0].Trail[1].ReasonHash = ""
			},
			want: "trail invalid",
		},
		{
			name: "updated timestamp mismatch",
			mutate: func(value *Snapshot) {
				value.Moderation.Reports[0].UpdatedAt = 14
			},
			want: "final trail timestamp",
		},
		{
			name: "final status mismatch",
			mutate: func(value *Snapshot) {
				value.Moderation.Reports[0].Trail[1].Status = "active"
			},
			want: "final trail status",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var candidate Snapshot
			if err := json.Unmarshal(fixtureSnapshotJSON(t), &candidate); err != nil {
				t.Fatal(err)
			}
			testCase.mutate(&candidate)
			if _, err := BuildPlan(mustJSON(t, candidate), BuildOptions{TargetChainID: 1439, Directory: testDirectory, Coordinator: testCoordinator}); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validation error = %v, want substring %q", err, testCase.want)
			}
		})
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
		Moderation:      SnapshotModeration{FinalStatuses: []SnapshotFinalStatus{{RepoID: testRepoID, Status: "frozen"}}, Reports: []SnapshotReport{{ID: 7, RepoID: testRepoID, Reporter: testReporter, Status: "resolved", Resolution: "frozen", ReasonHash: "report", CreatedAt: 11, UpdatedAt: 13, Trail: []SnapshotReportTrail{{Action: "submitted", Actor: testReporter, Status: "active", ReasonHash: "report", Timestamp: 11}, {Action: "resolved", Actor: testAdmin, Status: "frozen", ReasonHash: "decision", Timestamp: 13}}}}, StatusTrails: []SnapshotStatusTrail{{RepoID: testRepoID, Trail: []SnapshotRepositoryStatusLog{{Status: "frozen", Actor: testAdmin, ReasonHash: "decision", Timestamp: 13, ReportID: 7}}}}},
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

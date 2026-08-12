package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
)

const (
	testSourceContract     = "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	testOwner              = "inj1nnjdnm25n48vgnq2ak9y2z84v907l5rumnrlpe"
	testCollaborator       = "inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d"
	testTargetContract     = "0x2222222222222222222222222222222222222222"
	testControllerContract = "0x3333333333333333333333333333333333333333"
)

func TestBuildPlanIsDeterministicBoundedAndAddressNormalized(t *testing.T) {
	snapshot := validSnapshot()
	paths := writeSnapshotFixture(t, snapshot)
	options := Options{SnapshotPath: paths.snapshot, HashPath: paths.hash, TargetChainID: 1439, TargetContract: testTargetContract, BatchSize: 2}
	first, err := BuildPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := MarshalPlan(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalPlan(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatal("same snapshot produced a non-deterministic plan")
	}
	if first.Executable {
		t.Fatal("offline plan must not claim it can broadcast imports")
	}
	if first.Schema != PlanSchema {
		t.Fatalf("plan schema = %q, want %q", first.Schema, PlanSchema)
	}
	if first.Summary != (PlanSummary{RepositoryCount: 2, RefCount: 3, CollaboratorCount: 2, BatchCount: 5}) {
		t.Fatalf("summary = %#v", first.Summary)
	}
	if len(first.Repositories) != 2 || first.Repositories[0].Name != "alpha" || first.Repositories[1].Name != "beta" {
		t.Fatalf("repositories are not deterministically sorted: %#v", first.Repositories)
	}
	ownerEVM, err := chain.NormalizeEVMAddress(testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if first.Repositories[0].Owner != ownerEVM || !strings.HasPrefix(first.Repositories[0].RepoID, "0x") || len(first.Repositories[0].RepoID) != 66 {
		t.Fatalf("normalized identity = owner %q repoID %q", first.Repositories[0].Owner, first.Repositories[0].RepoID)
	}
	if got := []string{first.Repositories[0].Refs[0].RefName, first.Repositories[0].Refs[1].RefName, first.Repositories[0].Refs[2].RefName}; !reflect.DeepEqual(got, []string{"refs/heads/a", "refs/heads/b", "refs/heads/c"}) {
		t.Fatalf("refs are not sorted: %v", got)
	}
	for sequence, batch := range first.Batches {
		if batch.Sequence != sequence {
			t.Fatalf("batch %d sequence = %d", sequence, batch.Sequence)
		}
		if batch.Count > options.BatchSize && batch.Kind != "import_repo" {
			t.Fatalf("batch %d exceeds requested size: %#v", sequence, batch)
		}
		if len(batch.PayloadSHA256) != 64 {
			t.Fatalf("batch %d payload hash = %q", sequence, batch.PayloadSHA256)
		}
	}
	if got := first.Batches[0].Kind; got != "import_repo" {
		t.Fatalf("first batch kind = %q", got)
	}
	if got := first.Batches[1].Kind; got != "import_refs" {
		t.Fatalf("second batch kind = %q", got)
	}
	if !reflect.DeepEqual(first.DeferredSections, []string{"repo_extensions", "moderation_reports", "usernames", "badges_by_recipient", "releases"}) {
		t.Fatalf("deferred sections = %v", first.DeferredSections)
	}
	if first.ImportScope != CoreImportScope {
		t.Fatalf("import scope = %q", first.ImportScope)
	}
	t.Logf("stable imported repo ID vector: %s", first.Repositories[0].RepoID)
}

func TestImportRepoBatchHashBindsExpectedItemCounts(t *testing.T) {
	repo := PlanRepository{
		RepoID:           "0x" + strings.Repeat("1", 64),
		Owner:            "0x1111111111111111111111111111111111111111",
		Name:             "legacy",
		Description:      "historical",
		DefaultBranch:    "main",
		CreatedAt:        1,
		UpdatedAt:        2,
		ModerationStatus: "active",
	}
	withoutItems := batchForRepo(repo, 0)
	repo.Refs = []PlanRef{{RefName: "refs/heads/main"}}
	withRef := batchForRepo(repo, 0)
	repo.Collaborators = []PlanCollaborator{{Address: "0x2222222222222222222222222222222222222222", Role: "reader"}}
	withRefAndCollaborator := batchForRepo(repo, 0)

	if withoutItems.PayloadSHA256 == withRef.PayloadSHA256 {
		t.Fatal("repository batch hash did not bind expected ref count")
	}
	if withRef.PayloadSHA256 == withRefAndCollaborator.PayloadSHA256 {
		t.Fatal("repository batch hash did not bind expected collaborator count")
	}
}

func TestBuildPlanRejectsHashMismatchWithoutPlan(t *testing.T) {
	paths := writeSnapshotFixture(t, validSnapshot())
	if err := os.WriteFile(paths.snapshot, append(mustRead(t, paths.snapshot), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := BuildPlan(Options{SnapshotPath: paths.snapshot, HashPath: paths.hash, TargetChainID: 1439, TargetContract: testTargetContract})
	if err == nil || !strings.Contains(err.Error(), "snapshot hash mismatch") {
		t.Fatalf("error = %v, want hash mismatch", err)
	}
}

func TestBuildPlanPreservesHistoricalCommitSHAAndMetadataBytes(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Repositories[0].Description = strings.Repeat("legacy", 300)
	snapshot.Refs[0].Refs[0].CommitSHA = strings.Repeat("A", 40)
	paths := writeSnapshotFixture(t, snapshot)
	plan, err := BuildPlan(Options{
		SnapshotPath: paths.snapshot, HashPath: paths.hash,
		TargetChainID: 1439, TargetContract: testTargetContract,
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha := plan.Repositories[0]
	if alpha.Description != snapshot.Repositories[1].Description {
		t.Fatalf("alpha description changed during planning")
	}
	beta := plan.Repositories[1]
	if beta.Description != snapshot.Repositories[0].Description {
		t.Fatalf("legacy description bytes changed during planning")
	}
	var preserved string
	for _, ref := range alpha.Refs {
		if ref.RefName == "refs/heads/c" {
			preserved = ref.CommitSHA
		}
	}
	if preserved != strings.Repeat("A", 40) {
		t.Fatalf("historical commit SHA = %q, want exact uppercase input", preserved)
	}
}

func TestBuildPlanRejectsMissingRepositoryGroup(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Refs = snapshot.Refs[:1]
	paths := writeSnapshotFixture(t, snapshot)
	_, err := BuildPlan(Options{SnapshotPath: paths.snapshot, HashPath: paths.hash, TargetChainID: 1439, TargetContract: testTargetContract})
	if err == nil || !strings.Contains(err.Error(), "has no refs group") {
		t.Fatalf("error = %v, want missing refs group", err)
	}
}

func TestBuildPlanRejectsInvalidAndZeroAddressMappings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{name: "owner", mutate: func(snapshot *Snapshot) {
			snapshot.Repositories[0].Owner = "inj1invalid"
			snapshot.Refs[1].Owner = "inj1invalid"
			snapshot.Collaborators[0].Owner = "inj1invalid"
			snapshot.RepoExtensions[1].Owner = "inj1invalid"
		}, want: "owner mapping"},
		{name: "updater", mutate: func(snapshot *Snapshot) { snapshot.Refs[0].Refs[0].UpdatedBy = "inj1invalid" }, want: "updater mapping"},
		{name: "zero target", mutate: func(*Snapshot) {}, want: "zero address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validSnapshot()
			tc.mutate(&snapshot)
			paths := writeSnapshotFixture(t, snapshot)
			target := testTargetContract
			if tc.name == "zero target" {
				target = "0x0000000000000000000000000000000000000000"
			}
			_, err := BuildPlan(Options{SnapshotPath: paths.snapshot, HashPath: paths.hash, TargetChainID: 1439, TargetContract: target})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildPlanValidatesDistinctControllerAddress(t *testing.T) {
	for _, tc := range []struct {
		name       string
		controller string
		want       string
	}{
		{name: "invalid", controller: "not-an-address", want: "invalid controller"},
		{name: "zero", controller: "0x0000000000000000000000000000000000000000", want: "invalid controller"},
		{name: "same registry", controller: testTargetContract, want: "must differ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := writeSnapshotFixture(t, validSnapshot())
			_, err := BuildPlan(Options{
				SnapshotPath: paths.snapshot, HashPath: paths.hash,
				TargetChainID: 1439, TargetContract: testTargetContract, ControllerContract: tc.controller,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildPlanRejectsDuplicateCollaboratorAfterAddressNormalization(t *testing.T) {
	snapshot := validSnapshot()
	hexAddress, err := chain.NormalizeEVMAddress(testCollaborator)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Collaborators[0].Collaborators = []SnapshotCollaborator{
		{Address: testCollaborator, Role: "reader"},
		{Address: strings.ToUpper(strings.TrimPrefix(hexAddress, "0x")), Role: "maintainer"},
	}
	// Restore a valid 0X prefix; the backend accepts either prefix and
	// normalization must still detect that these are the same 20 bytes.
	snapshot.Collaborators[0].Collaborators[1].Address = "0X" + snapshot.Collaborators[0].Collaborators[1].Address
	paths := writeSnapshotFixture(t, snapshot)
	_, err = BuildPlan(Options{SnapshotPath: paths.snapshot, HashPath: paths.hash, TargetChainID: 1439, TargetContract: testTargetContract})
	if err == nil || !strings.Contains(err.Error(), "duplicate collaborator") {
		t.Fatalf("error = %v, want normalized duplicate", err)
	}
}

func TestBuildPlanRejectsV2ImportFormatLimits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{
			name: "repo name character",
			mutate: func(snapshot *Snapshot) {
				snapshot.Repositories[0].Name = "bad/name"
			},
			want: "unsupported byte",
		},
		{
			name: "repo name length",
			mutate: func(snapshot *Snapshot) {
				snapshot.Repositories[0].Name = strings.Repeat("a", EVMV2MaxRepoNameLength+1)
			},
			want: "V2 maximum",
		},
		{
			name: "ref prefix",
			mutate: func(snapshot *Snapshot) {
				snapshot.Refs[0].Refs[0].RefName = "heads/main"
			},
			want: "must start with refs/",
		},
		{
			name: "ref forbidden sequence",
			mutate: func(snapshot *Snapshot) {
				snapshot.Refs[0].Refs[0].RefName = "refs/heads/a..b"
			},
			want: "must not contain ..",
		},
		{
			name: "ref length",
			mutate: func(snapshot *Snapshot) {
				snapshot.Refs[0].Refs[0].RefName = "refs/" + strings.Repeat("a", EVMV2MaxRefNameLength)
			},
			want: "requires 5..256",
		},
		{
			name: "pack URI locator",
			mutate: func(snapshot *Snapshot) {
				snapshot.Refs[0].Refs[0].PackURIs = []string{"ipfs://-cid"}
			},
			want: "locator must start",
		},
		{
			name: "pack URI control byte",
			mutate: func(snapshot *Snapshot) {
				snapshot.Refs[0].Refs[0].PackURIs = []string{"ipfs://cid" + string([]byte{0x7f})}
			},
			want: "control byte",
		},
		{
			name: "pack URI count",
			mutate: func(snapshot *Snapshot) {
				uris := make([]string, EVMV2MaxPackURIs+1)
				for i := range uris {
					uris[i] = fmt.Sprintf("ipfs://cid-%d", i)
				}
				snapshot.Refs[0].Refs[0].PackURIs = uris
			},
			want: "pack URIs; V2 maximum",
		},
		{
			name: "ref count",
			mutate: func(snapshot *Snapshot) {
				refs := make([]SnapshotRef, EVMV2MaxRefsPerRepo+1)
				for i := range refs {
					refs[i] = SnapshotRef{
						RefName:   fmt.Sprintf("refs/heads/ref-%04d", i),
						CommitSHA: strings.Repeat("a", 40),
						PackURIs:  []string{"ipfs://cid"},
						UpdatedBy: testOwner,
					}
				}
				snapshot.Refs[0].Refs = refs
			},
			want: "V2 maximum is 1024",
		},
		{
			name: "collaborator count",
			mutate: func(snapshot *Snapshot) {
				collaborators := make([]SnapshotCollaborator, EVMV2MaxCollaboratorsPerRepo+1)
				for i := range collaborators {
					collaborators[i] = SnapshotCollaborator{
						Address: fmt.Sprintf("0x%040x", i+1),
						Role:    "reader",
					}
				}
				snapshot.Collaborators[0].Collaborators = collaborators
			},
			want: "V2 maximum is 256",
		},
		{
			name: "moderation status",
			mutate: func(snapshot *Snapshot) {
				snapshot.Repositories[0].ModerationStatus = "quarantined"
			},
			want: "unsupported status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validSnapshot()
			test.mutate(&snapshot)
			paths := writeSnapshotFixture(t, snapshot)
			_, err := BuildPlan(Options{
				SnapshotPath: paths.snapshot, HashPath: paths.hash,
				TargetChainID: 1439, TargetContract: testTargetContract,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestV2ImportValidatorsAcceptProtocolBoundaries(t *testing.T) {
	if err := validateImportRepoName(strings.Repeat("a", EVMV2MaxRepoNameLength)); err != nil {
		t.Fatalf("maximum-length repo name rejected: %v", err)
	}
	maximumRefName := "refs/" + strings.Repeat("a", EVMV2MaxRefNameLength-len("refs/"))
	if err := validateImportRefName(maximumRefName); err != nil {
		t.Fatalf("maximum-length ref name rejected: %v", err)
	}
	maximumPackURI := "ipfs://a" + strings.Repeat("b", EVMV2MaxPackURIBytes-len("ipfs://a"))
	if err := validateImportPackURI(maximumPackURI); err != nil {
		t.Fatalf("maximum-length pack URI rejected: %v", err)
	}
	for _, status := range []string{"active", "delisted", "frozen"} {
		if err := validateImportModerationStatus(status); err != nil {
			t.Fatalf("V2 moderation status %q rejected: %v", status, err)
		}
	}
}

func validSnapshot() Snapshot {
	return Snapshot{
		Schema: SnapshotSchema,
		Source: SnapshotSource{ChainID: "injective-888", Contract: testSourceContract, Height: "4242"},
		Repositories: []SnapshotRepo{
			{Owner: testOwner, Name: "beta", Description: "beta", DefaultBranch: "main", CreatedAt: 2, UpdatedAt: 3, ModerationStatus: "active"},
			{Owner: testOwner, Name: "alpha", Description: "alpha", DefaultBranch: "main", CreatedAt: 1, UpdatedAt: 4, ModerationStatus: "frozen"},
		},
		Refs: []SnapshotRefGroup{
			{Owner: testOwner, Repo: "alpha", Refs: []SnapshotRef{
				{RefName: "refs/heads/c", CommitSHA: strings.Repeat("c", 40), PackURIs: []string{"ipfs://c"}, UpdatedAt: 4, UpdatedBy: testOwner},
				{RefName: "refs/heads/a", CommitSHA: strings.Repeat("a", 40), PackURIs: []string{"ipfs://a"}, UpdatedAt: 2, UpdatedBy: testOwner},
				{RefName: "refs/heads/b", CommitSHA: strings.Repeat("b", 40), PackURIs: []string{"ipfs://b"}, UpdatedAt: 3, UpdatedBy: testOwner},
			}},
			{Owner: testOwner, Repo: "beta", Refs: []SnapshotRef{}},
		},
		Collaborators: []SnapshotCollabGroup{
			{Owner: testOwner, Repo: "beta", Collaborators: []SnapshotCollaborator{}},
			{Owner: testOwner, Repo: "alpha", Collaborators: []SnapshotCollaborator{
				{Address: testCollaborator, Role: "reader"},
				{Address: testSourceContract, Role: "maintainer"},
			}},
		},
		RepoExtensions: []SnapshotExtension{{Owner: testOwner, Repo: "alpha"}, {Owner: testOwner, Repo: "beta"}},
	}
}

type fixturePaths struct {
	snapshot string
	hash     string
}

func writeSnapshotFixture(t *testing.T, snapshot Snapshot) fixturePaths {
	t.Helper()
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	hashPath := snapshotPath + ".sha256"
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(snapshotPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	record := hex.EncodeToString(digest[:]) + "  " + filepath.Base(snapshotPath) + "\n"
	if err := os.WriteFile(hashPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	return fixturePaths{snapshot: snapshotPath, hash: hashPath}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

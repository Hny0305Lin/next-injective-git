package successor

import (
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func testCommitment(seed byte) Commitment {
	digest := [32]byte{}
	for i := range digest {
		digest[i] = seed
	}
	return Commitment{
		ManifestDigest:   digest,
		ManifestSize:     778,
		BootstrapLocator: "https://public.example.com/igit/prefix/manifests/sha256/" + strings.Repeat(string(rune('a'+seed%26)), 64) + ".json",
	}
}

func newTestChain(t *testing.T) (*FakeChain, *Client, [32]byte) {
	t.Helper()
	chain := NewFakeChain(common.HexToAddress("0xa11ce00000000000000000000000000000000001"), big.NewInt(1439), common.HexToAddress("0x4444000000000000000000000000000000000444"))
	repoID, err := chain.CreateRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(chain)
	if err != nil {
		t.Fatal(err)
	}
	return chain, client, repoID
}

func TestEmbeddedSuccessorABIsMatchSolidityArtifacts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	artifactDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "contracts", "evm-v2-successor", "abi")
	embedded := map[string]string{
		"RepositoryCore.json": repositoryCoreABIJSON,
		"SuiteDirectory.json": suiteDirectoryABIJSON,
	}
	for name, source := range embedded {
		t.Run(name, func(t *testing.T) {
			artifact, err := os.ReadFile(filepath.Join(artifactDir, name))
			if err != nil {
				t.Fatalf("read successor Solidity ABI: %v", err)
			}
			if strings.TrimSpace(source) != strings.TrimSpace(string(artifact)) {
				t.Fatalf("embedded successor ABI differs from contracts/evm-v2-successor/abi/%s; rerun the successor ABI gate", name)
			}
		})
	}
}

func TestSuccessorABINeverSharesV3Artifacts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	v3, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "abi", "RepositoryCore.json"))
	if err != nil {
		t.Skipf("legacy v3 ABI unavailable: %v", err)
	}
	if strings.TrimSpace(string(v3)) == strings.TrimSpace(repositoryCoreABIJSON) {
		t.Fatal("successor ABI must stay versioned separately from the legacy v3 ABI")
	}
}

func TestCreateUpdateAndCasConflicts(t *testing.T) {
	ctx := context.Background()
	_, client, repoID := newTestChain(t)
	first := testCommitment(1)

	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), first, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	state, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 || state.Commitment != first {
		t.Fatalf("create: revision %d commitment %+v", state.Revision, state.Commitment)
	}

	// Same commit, new manifest/locator: CAS update bumps the revision.
	second := testCommitment(2)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), second, state.Revision, state.Commitment.ManifestDigest, false); err != nil {
		t.Fatal(err)
	}
	updated, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Commitment != second {
		t.Fatalf("update: revision %d commitment %+v", updated.Revision, updated.Commitment)
	}

	// Stale revision loses the race.
	_, err = client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(3), 1, first.ManifestDigest, false)
	var mismatch *CommitmentMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("stale revision error = %v, want CommitmentMismatch", err)
	}
	if mismatch.ExpectedRevision != 1 || mismatch.ActualRevision != 2 {
		t.Fatalf("mismatch fields: %+v", mismatch)
	}

	// Replay of the original create also fails.
	_, err = client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), first, 0, [32]byte{}, false)
	if !errors.As(err, &mismatch) {
		t.Fatalf("replayed create error = %v, want CommitmentMismatch", err)
	}
	if mismatch.ActualRevision != 2 {
		t.Fatalf("replay actual revision = %d, want 2", mismatch.ActualRevision)
	}
	after, _ := client.GetRef(ctx, repoID, "refs/heads/main")
	if after.Commitment != second || after.Revision != 2 {
		t.Fatal("failed CAS attempts mutated state")
	}
}

func TestForceNeverWaivesCas(t *testing.T) {
	ctx := context.Background()
	_, client, repoID := newTestChain(t)
	first := testCommitment(1)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), first, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	_, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(2), 0, [32]byte{}, true)
	var mismatch *CommitmentMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("force+stale error = %v, want CommitmentMismatch", err)
	}
	state, _ := client.GetRef(ctx, repoID, "refs/heads/main")
	if state.Revision != 1 || state.Commitment != first {
		t.Fatal("force with stale CAS mutated state")
	}
	// force with a fresh expectation still updates.
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(2), state.Revision, state.Commitment.ManifestDigest, true); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteTombstoneRecreateBlocksAbaReplay(t *testing.T) {
	ctx := context.Background()
	chain, client, repoID := newTestChain(t)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(1), 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	before, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteRef(ctx, repoID, "refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRef(ctx, repoID, "refs/heads/main"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("getRef after delete = %v, want ErrRefNotFound", err)
	}
	refs, err := chain.Refs(repoID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("deleted ref still listed: %v %v", refs, err)
	}

	// Replayed stale create must fail against the tombstone.
	var mismatch *CommitmentMismatchError
	_, err = client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(3), 0, [32]byte{}, false)
	if !errors.As(err, &mismatch) || mismatch.ActualRevision != before.Revision {
		t.Fatalf("ABA replay error = %v, want tombstone CommitmentMismatch at revision %d", err, before.Revision)
	}

	// Recreate carries the tombstone revision and the zero digest.
	recreated := testCommitment(4)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), recreated, before.Revision, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	after, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 || after.Commitment != recreated {
		t.Fatalf("recreate: revision %d commitment %+v", after.Revision, after.Commitment)
	}
}

func TestForkCopiesNoRefCommitments(t *testing.T) {
	ctx := context.Background()
	chain, client, repoID := newTestChain(t)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(1), 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	forkID, err := chain.ForkRepository(repoID, "fork-target")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := chain.Refs(forkID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("fork carried refs: %v %v", refs, err)
	}
	if _, err := client.GetRef(ctx, forkID, "refs/heads/main"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("fork ref lookup = %v, want ErrRefNotFound", err)
	}
	// The fork target publishes fresh from zero.
	if _, err := client.UpdateRef(ctx, forkID, "refs/heads/main", strings.Repeat("a", 40), testCommitment(9), 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
}

func TestClientFailsClosedOnInvalidCommitments(t *testing.T) {
	ctx := context.Background()
	_, client, repoID := newTestChain(t)
	zero := testCommitment(1)
	zero.ManifestDigest = [32]byte{}
	badSize := testCommitment(1)
	badSize.ManifestSize = 0
	oversize := testCommitment(1)
	oversize.ManifestSize = MaxManifestSize + 1
	insecure := testCommitment(1)
	insecure.BootstrapLocator = "http://public.example.com/m.json"
	for name, commitment := range map[string]Commitment{
		"zero-digest": zero, "zero-size": badSize, "oversize": oversize, "insecure-locator": insecure,
	} {
		if _, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), commitment, 0, [32]byte{}, false); err == nil {
			t.Fatalf("%s: client accepted invalid commitment", name)
		}
	}
	if _, err := client.GetRef(ctx, repoID, "refs/heads/main"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("invalid publishes mutated the chain: %v", err)
	}
}

func TestEventCarriesFullRefNameAndCommitment(t *testing.T) {
	ctx := context.Background()
	chain, client, repoID := newTestChain(t)
	commitment := testCommitment(5)
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/release-1.2", strings.Repeat("b", 40), commitment, 0, [32]byte{}, false); err != nil {
		t.Fatal(err)
	}
	events := chain.Events()
	if len(events) != 1 {
		t.Fatalf("events recorded: %d", len(events))
	}
	event := events[0]
	if event.RefName != "refs/heads/release-1.2" || event.CommitSHA != strings.Repeat("b", 40) ||
		event.Digest != commitment.ManifestDigest || event.Size != commitment.ManifestSize ||
		event.Locator != commitment.BootstrapLocator || event.Revision != 1 {
		t.Fatalf("event fields incomplete: %+v", event)
	}
}

func TestUncertainReceiptKeepsStateUnknownUntilResolved(t *testing.T) {
	ctx := context.Background()
	chain, client, repoID := newTestChain(t)
	commitment := testCommitment(7)
	chain.Uncertain = true
	hash, err := client.UpdateRef(ctx, repoID, "refs/heads/main", strings.Repeat("a", 40), commitment, 0, [32]byte{}, false)
	if err != nil || hash == "" {
		t.Fatalf("uncertain send = %q %v", hash, err)
	}
	if _, err := client.GetRef(ctx, repoID, "refs/heads/main"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("ref visible before receipt resolution: %v", err)
	}
	if chain.PendingCount() != 1 {
		t.Fatalf("pending receipts = %d", chain.PendingCount())
	}
	// The client must not blindly resend with a fresh expectation: replaying the
	// same CAS inputs is safe because the pending apply is idempotent-checked.
	if err := chain.Resolve(hash, true); err != nil {
		t.Fatal(err)
	}
	state, err := client.GetRef(ctx, repoID, "refs/heads/main")
	if err != nil || state.Revision != 1 || state.Commitment != commitment {
		t.Fatalf("after resolve: %v %+v", err, state)
	}

	// Dropped variant: the ref stays absent and the original CAS inputs can be
	// re-broadcast safely.
	chain.Uncertain = true
	hash2, err := client.UpdateRef(ctx, repoID, "refs/heads/dev", strings.Repeat("a", 40), testCommitment(8), 0, [32]byte{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.Resolve(hash2, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRef(ctx, repoID, "refs/heads/dev"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("dropped receipt still applied: %v", err)
	}
	if _, err := client.UpdateRef(ctx, repoID, "refs/heads/dev", strings.Repeat("a", 40), testCommitment(8), 0, [32]byte{}, false); err != nil {
		t.Fatalf("safe rebroadcast after dropped receipt: %v", err)
	}
}

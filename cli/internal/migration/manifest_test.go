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

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func TestBuildTransactionManifestIsDeterministicAndMatchesCheckedABI(t *testing.T) {
	paths := writeSnapshotFixture(t, validSnapshot())
	plan, err := BuildPlan(Options{
		SnapshotPath: paths.snapshot, HashPath: paths.hash,
		TargetChainID: 1439, TargetContract: testTargetContract, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := MarshalTransactionManifest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalTransactionManifest(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("same plan produced a non-deterministic transaction manifest")
	}
	if first.Schema != TransactionManifestSchema || !first.CalldataReady || first.Signed || first.Broadcast {
		t.Fatalf("manifest execution boundary = %#v", first)
	}
	if first.ImportScope != CoreImportScope || !reflect.DeepEqual(first.DeferredSections, plan.DeferredSections) {
		t.Fatalf("manifest import scope = %q / %v", first.ImportScope, first.DeferredSections)
	}
	if first.SessionID != "0x"+plan.Snapshot.SHA256 {
		t.Fatalf("session ID = %q, want snapshot hash", first.SessionID)
	}
	planRaw, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	planHash := sha256.Sum256(planRaw)
	if first.PlanSHA256 != hex.EncodeToString(planHash[:]) {
		t.Fatalf("plan hash = %s", first.PlanSHA256)
	}
	if first.Summary.TransactionCount != plan.Summary.BatchCount+2 ||
		len(first.Transactions) != first.Summary.TransactionCount {
		t.Fatalf("manifest summary = %#v", first.Summary)
	}
	wantPhases := []string{
		"create_session", "import_repo", "import_refs", "import_refs",
		"import_collaborators", "import_repo", "finalize",
	}
	phases := make([]string, 0, len(first.Transactions))
	for order, transaction := range first.Transactions {
		if transaction.Order != order || transaction.To != testTargetContract || transaction.EventEmitter != testTargetContract || transaction.Value != "0x0" {
			t.Fatalf("transaction %d routing = %#v", order, transaction)
		}
		phases = append(phases, transaction.Phase)
	}
	if !reflect.DeepEqual(phases, wantPhases) {
		t.Fatalf("manifest phases = %v", phases)
	}

	contractABI := readRegistryABI(t)
	for _, transaction := range first.Transactions {
		methodName := strings.SplitN(transaction.Function, "(", 2)[0]
		method, ok := contractABI.Methods[methodName]
		if !ok {
			t.Fatalf("checked ABI has no method %s", methodName)
		}
		if method.Sig != transaction.Function {
			t.Fatalf("manifest signature %q, checked ABI signature %q", transaction.Function, method.Sig)
		}
		event, ok := contractABI.Events[transaction.ExpectedEvent]
		if !ok {
			t.Fatalf("checked ABI has no event %s", transaction.ExpectedEvent)
		}
		if event.ID.Hex() != transaction.ExpectedTopic {
			t.Fatalf("%s topic = %s, want %s", transaction.ExpectedEvent, transaction.ExpectedTopic, event.ID.Hex())
		}
		data, err := hex.DecodeString(strings.TrimPrefix(transaction.Data, "0x"))
		if err != nil {
			t.Fatalf("decode %s calldata: %v", methodName, err)
		}
		if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
			t.Fatalf("%s selector = %x, want %x", methodName, data[:minInt(len(data), 4)], method.ID)
		}
		if _, err := method.Inputs.Unpack(data[4:]); err != nil {
			t.Fatalf("checked ABI cannot decode %s calldata: %v", methodName, err)
		}
	}
}

func TestBuildControllerTransactionManifestUsesControllerABIAndRegistryEvents(t *testing.T) {
	paths := writeSnapshotFixture(t, validSnapshot())
	plan, err := BuildPlan(Options{
		SnapshotPath: paths.snapshot, HashPath: paths.hash,
		TargetChainID: 1439, TargetContract: testTargetContract, ControllerContract: testControllerContract, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Target.Controller != testControllerContract || manifest.ImportCommitment != "0x"+plan.ImportCommitment {
		t.Fatalf("controller manifest target/commitment = %#v / %s", manifest.Target, manifest.ImportCommitment)
	}
	controllerABI := readControllerABI(t)
	registryABI := readRegistryABI(t)
	for _, transaction := range manifest.Transactions {
		if transaction.To != testControllerContract {
			t.Fatalf("transaction target = %s, want controller", transaction.To)
		}
		data, err := hex.DecodeString(strings.TrimPrefix(transaction.Data, "0x"))
		if err != nil {
			t.Fatal(err)
		}
		methodName := strings.SplitN(transaction.Function, "(", 2)[0]
		method, ok := controllerABI.Methods[methodName]
		if !ok || method.Sig != transaction.Function {
			t.Fatalf("controller ABI method mismatch for %s", transaction.Function)
		}
		if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
			t.Fatalf("controller selector mismatch for %s", transaction.Function)
		}
		if _, err := method.Inputs.Unpack(data[4:]); err != nil {
			t.Fatalf("controller ABI cannot decode %s: %v", transaction.Function, err)
		}
		event, ok := controllerABI.Events[transaction.ExpectedEvent]
		if transaction.Phase == "import_repo" || transaction.Phase == "import_refs" || transaction.Phase == "import_collaborators" {
			event, ok = registryABI.Events[transaction.ExpectedEvent]
		}
		if !ok || event.ID.Hex() != transaction.ExpectedTopic {
			t.Fatalf("event mismatch for %s: %s", transaction.Phase, transaction.ExpectedEvent)
		}
		wantEmitter := testControllerContract
		if transaction.Phase == "import_repo" || transaction.Phase == "import_refs" || transaction.Phase == "import_collaborators" {
			wantEmitter = testTargetContract
		}
		if transaction.EventEmitter != wantEmitter {
			t.Fatalf("event emitter for %s = %s, want %s", transaction.Phase, transaction.EventEmitter, wantEmitter)
		}
	}
}

func TestBuildTransactionManifestRejectsTamperedPlan(t *testing.T) {
	newPlan := func(t *testing.T) *Plan {
		t.Helper()
		paths := writeSnapshotFixture(t, validSnapshot())
		plan, err := BuildPlan(Options{
			SnapshotPath: paths.snapshot, HashPath: paths.hash,
			TargetChainID: 1439, TargetContract: testTargetContract, BatchSize: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}

	for _, tc := range []struct {
		name   string
		mutate func(*Plan)
		want   string
	}{
		{
			name: "payload hash",
			mutate: func(plan *Plan) {
				plan.Batches[0].PayloadSHA256 = strings.Repeat("0", 64)
			},
			want: "payload hash",
		},
		{
			name: "item keys",
			mutate: func(plan *Plan) {
				plan.Batches[1].ItemKeys[0] = "refs/heads/tampered"
			},
			want: "item keys",
		},
		{
			name: "repo identity",
			mutate: func(plan *Plan) {
				plan.Repositories[0].RepoID = "0x" + strings.Repeat("0", 64)
			},
			want: "repo ID mismatch",
		},
		{
			name: "moderation status",
			mutate: func(plan *Plan) {
				plan.Repositories[0].ModerationStatus = "quarantined"
			},
			want: "unsupported status",
		},
		{
			name: "summary",
			mutate: func(plan *Plan) {
				plan.Summary.RefCount++
			},
			want: "summary",
		},
		{
			name: "sequence",
			mutate: func(plan *Plan) {
				plan.Batches[1].Sequence = 9
			},
			want: "does not exactly cover",
		},
		{
			name: "missing batch",
			mutate: func(plan *Plan) {
				plan.Batches = plan.Batches[:len(plan.Batches)-1]
				plan.Summary.BatchCount--
			},
			want: "do not completely cover",
		},
		{
			name: "duplicate batch",
			mutate: func(plan *Plan) {
				duplicate := plan.Batches[1]
				plan.Batches = append(plan.Batches, duplicate)
				plan.Summary.BatchCount++
			},
			want: "do not completely cover",
		},
		{
			name: "duplicate repo header with unchanged batch count",
			mutate: func(plan *Plan) {
				duplicate := plan.Batches[0]
				duplicate.Sequence = plan.Batches[1].Sequence
				plan.Batches[1] = duplicate
			},
			want: "does not exactly cover",
		},
		{
			name: "overlapping range",
			mutate: func(plan *Plan) {
				plan.Batches[2].Start = plan.Batches[1].Start
			},
			want: "does not exactly cover",
		},
		{
			name: "collaborator range misses first entity",
			mutate: func(plan *Plan) {
				batch := &plan.Batches[3]
				batch.Start = 1
				batch.Count = 1
				batch.ItemKeys = append([]string(nil), batch.ItemKeys[1:]...)
				batch.PayloadSHA256 = importCollaboratorsPayloadHash(plan.Repositories[0].Collaborators[1:])
			},
			want: "does not exactly cover",
		},
		{
			name: "reordered batches",
			mutate: func(plan *Plan) {
				plan.Batches[1], plan.Batches[2] = plan.Batches[2], plan.Batches[1]
			},
			want: "does not exactly cover",
		},
		{
			name: "repositories reordered with rewritten sequences",
			mutate: func(plan *Plan) {
				plan.Repositories[0], plan.Repositories[1] = plan.Repositories[1], plan.Repositories[0]
				plan.Repositories[0].Sequence = 0
				plan.Repositories[1].Sequence = 1
			},
			want: "planner source_owner/name order",
		},
		{
			name: "missing source owner",
			mutate: func(plan *Plan) {
				plan.Repositories[0].SourceOwner = ""
			},
			want: "source owner is required",
		},
		{
			name: "invalid batch size",
			mutate: func(plan *Plan) {
				plan.BatchSize = MaximumBatchSize + 1
			},
			want: "plan batch size",
		},
		{
			name: "repository sequence",
			mutate: func(plan *Plan) {
				plan.Repositories[0].Sequence = 7
			},
			want: "repository sequence",
		},
		{
			name: "noncanonical repository owner",
			mutate: func(plan *Plan) {
				plan.Repositories[0].Owner = strings.ToUpper(plan.Repositories[0].Owner)
			},
			want: "owner is not a canonical non-zero address",
		},
		{
			name: "noncanonical ref updater",
			mutate: func(plan *Plan) {
				plan.Repositories[0].Refs[0].UpdatedBy = strings.ToUpper(plan.Repositories[0].Refs[0].UpdatedBy)
			},
			want: "updater is not a canonical non-zero address",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := newPlan(t)
			tc.mutate(plan)
			_, err := BuildTransactionManifest(plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMarshalTransactionManifestRejectsNil(t *testing.T) {
	if _, err := MarshalTransactionManifest(nil); err == nil {
		t.Fatal("nil transaction manifest was accepted")
	}
}

func TestReadAndVerifyTransactionManifestStrictlyBindsPlan(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalTransactionManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	write := func(t *testing.T, raw []byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "transactions.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	decoded, err := ReadTransactionManifest(write(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyTransactionManifest(plan, decoded); err != nil {
		t.Fatal(err)
	}
	digest, err := TransactionManifestSHA256(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		t.Fatalf("manifest SHA-256 = %q", digest)
	}

	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "unknown field", raw: bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"unknown\": true,\n"), 1), want: "unknown field"},
		{name: "trailing JSON", raw: append(append([]byte(nil), raw...), []byte("{}\n")...), want: "trailing JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadTransactionManifest(write(t, tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := ReadTransactionManifest(""); err == nil {
		t.Fatal("empty manifest path was accepted")
	}
}

func TestVerifyTransactionManifestRejectsEveryUnboundMutation(t *testing.T) {
	plan := verificationPlan(t)
	tests := []struct {
		name   string
		mutate func(*TransactionManifest)
	}{
		{name: "signed claim", mutate: func(manifest *TransactionManifest) { manifest.Signed = true }},
		{name: "target", mutate: func(manifest *TransactionManifest) { manifest.Transactions[0].To = testTargetContract }},
		{name: "calldata", mutate: func(manifest *TransactionManifest) { manifest.Transactions[0].Data = "0x1234" }},
		{name: "event topic", mutate: func(manifest *TransactionManifest) {
			manifest.Transactions[0].ExpectedTopic = "0x" + strings.Repeat("0", 64)
		}},
		{name: "order", mutate: func(manifest *TransactionManifest) { manifest.Transactions[0].Order++ }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := BuildTransactionManifest(plan)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(manifest)
			err = VerifyTransactionManifest(plan, manifest)
			if err == nil || !strings.Contains(err.Error(), "does not exactly match") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := VerifyTransactionManifest(plan, nil); err == nil {
		t.Fatal("nil transaction manifest was accepted")
	}
}

func readRegistryABI(t *testing.T) abi.ABI {
	t.Helper()
	path := filepath.Join("..", "..", "..", "contracts", "evm-v2", "abi", "RepoRegistryV2.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checked RepoRegistryV2 ABI: %v", err)
	}
	parsed, err := abi.JSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse checked RepoRegistryV2 ABI: %v", err)
	}
	return parsed
}

func readControllerABI(t *testing.T) abi.ABI {
	t.Helper()
	path := filepath.Join("..", "..", "..", "contracts", "evm-v2", "abi", "RepoRegistryV2ImportController.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checked controller ABI: %v", err)
	}
	parsed, err := abi.JSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse checked controller ABI: %v", err)
	}
	return parsed
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

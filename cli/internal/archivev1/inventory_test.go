package archivev1

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testV1Contract = "inj1contract0000000000000000000000000000000"

func testAttribute(key, value string, encoded bool) map[string]string {
	if encoded {
		key = base64.StdEncoding.EncodeToString([]byte(key))
		value = base64.StdEncoding.EncodeToString([]byte(value))
	}
	return map[string]string{"key": key, "value": value}
}

func testEvent(action string, attributes map[string]string, encoded bool) map[string]any {
	values := []any{
		testAttribute("_contract_address", testV1Contract, encoded),
		testAttribute("action", action, encoded),
	}
	for key, value := range attributes {
		values = append(values, testAttribute(key, value, encoded))
	}
	return map[string]any{"type": "wasm", "attributes": values}
}

func testTx(seed string, height uint64, index uint32, code uint32, events ...map[string]any) map[string]any {
	txBytes := []byte("archive-v1-test-transaction:" + seed)
	digest := sha256.Sum256(txBytes)
	return map[string]any{
		"hash": strings.ToUpper(hex.EncodeToString(digest[:])), "height": height, "index": index,
		"tx": base64.StdEncoding.EncodeToString(txBytes),
		"tx_result": map[string]any{
			"code": code, "data": nil, "log": "", "info": "", "gas_wanted": "1", "gas_used": "1",
			"events": events, "codespace": "",
		},
	}
}

func testTxSearchPage(height uint64, txs ...any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
		"query": expectedTxSearchQuery(testV1Contract, height), "total_count": strconv.Itoa(len(txs)), "txs": txs,
	}}
}

func testTxSearchFixture(t *testing.T) []byte {
	t.Helper()
	oldOwner := "inj1oldowner"
	newOwner := "inj1newowner"
	pages := []any{
		map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
			"query": expectedTxSearchQuery(testV1Contract, 10), "total_count": "10",
			"txs": []any{
				testTx("09", 9, 0, 0, testEvent("register_release", map[string]string{"version": "v1.0.0"}, true)),
				testTx("07", 7, 0, 0, testEvent("register_username", map[string]string{"name": "bob", "owner": "inj1bob"}, false)),
				testTx("05", 5, 0, 0, testEvent("register_username", map[string]string{"name": "alice", "owner": newOwner}, true)),
				testTx("03", 3, 0, 0, testEvent("transfer_ownership", map[string]string{
					"old_owner": oldOwner, "new_owner": newOwner, "repo": "alpha",
				}, false)),
				testTx("01", 1, 0, 0, testEvent("create_repo", map[string]string{"owner": oldOwner, "repo": "alpha"}, true)),
			},
		}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{
			"query": expectedTxSearchQuery(testV1Contract, 10), "total_count": "10",
			"txs": []any{
				testTx("10", 10, 0, 9, testEvent("register_release", map[string]string{"version": "failed"}, false)),
				testTx("08", 8, 0, 0, testEvent("award_badge", map[string]string{"recipient": "inj1recipient"}, true)),
				testTx("06", 6, 0, 0, testEvent("release_username", map[string]string{"name": "alice", "owner": newOwner}, false)),
				testTx("04", 4, 0, 0, testEvent("submit_moderation_report", map[string]string{
					"report_id": "7", "owner": newOwner, "repo": "alpha", "reporter": "inj1reporter", "reason_hash": "sha256:report",
				}, true)),
				testTx("02", 2, 0, 0, testEvent("fork_repo", map[string]string{
					"source_owner": oldOwner, "source_repo": "alpha", "owner": "inj1forkowner", "repo": "beta",
				}, false)),
			},
		}},
	}
	raw, err := json.Marshal(pages)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testBlockEvidence(t *testing.T, raw []byte, chainID string, cutoverHeight uint64) *BlockEvidence {
	t.Helper()
	documents, err := decodeEvidenceDocuments(raw, "test tx_search")
	if err != nil {
		t.Fatal(err)
	}
	txsByHeight := make(map[uint64][]string)
	for _, document := range documents {
		var envelope txSearchEnvelope
		if err := json.Unmarshal(document, &envelope); err != nil || envelope.Result == nil {
			t.Fatalf("decode test tx_search: %v", err)
		}
		for _, transaction := range envelope.Result.Txs {
			height, err := rawUint64(transaction.Height)
			if err != nil {
				t.Fatal(err)
			}
			index, err := rawUint64(transaction.Index)
			if err != nil {
				t.Fatal(err)
			}
			values := txsByHeight[height]
			for uint64(len(values)) <= index {
				filler := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("unrelated:%d:%d", height, len(values))))
				values = append(values, filler)
			}
			// Keep the first transaction for a duplicate position so the builder
			// reaches its explicit duplicate-position check.
			if strings.HasPrefix(values[index], "dW5yZWxhdGVk") {
				values[index] = transaction.Tx
			}
			txsByHeight[height] = values
		}
	}
	var responses []any
	for height := uint64(1); height <= cutoverHeight; height++ {
		if _, required := txsByHeight[height]; !required && height != cutoverHeight {
			continue
		}
		blockDigest := sha256.Sum256([]byte(fmt.Sprintf("archive-v1-test-block:%d", height)))
		responses = append(responses, map[string]any{
			"jsonrpc": "2.0", "id": height,
			"result": map[string]any{
				"block_id": map[string]any{"hash": strings.ToUpper(hex.EncodeToString(blockDigest[:]))},
				"block": map[string]any{
					"header": map[string]any{
						"chain_id": chainID, "height": strconv.FormatUint(height, 10),
						"time": time.Unix(int64(100+height), 0).UTC().Format(time.RFC3339Nano),
					},
					"data": map[string]any{"txs": txsByHeight[height]},
				},
			},
		})
	}
	encoded, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "blocks.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := ReadBlockEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func TestBuildInventoryReplaysSuccessfulEventsInCanonicalOrder(t *testing.T) {
	raw := testTxSearchFixture(t)
	evidence := testBlockEvidence(t, raw, "injective-888", 10)
	inventory, err := BuildInventory(raw, "injective-888", testV1Contract, 10, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Schema != InventorySchema || inventory.SuccessfulTxs != 9 || inventory.ScannedTxs != 10 {
		t.Fatalf("inventory header = %#v", inventory)
	}
	if len(inventory.SourceSHA256) != 64 || len(inventory.BlockSHA256) != 64 || len(inventory.EventCommitment) != 64 {
		t.Fatalf("inventory hashes = %q / %q", inventory.SourceSHA256, inventory.EventCommitment)
	}
	if len(inventory.Repositories) != 2 {
		t.Fatalf("repositories = %#v", inventory.Repositories)
	}
	var alpha, beta *InventoryRepository
	for index := range inventory.Repositories {
		repository := &inventory.Repositories[index]
		switch repository.Name {
		case "alpha":
			alpha = repository
		case "beta":
			beta = repository
		}
	}
	if alpha == nil || alpha.Owner != "inj1newowner" || strings.Join(alpha.Aliases, ",") != "inj1newowner/alpha,inj1oldowner/alpha" {
		t.Fatalf("transferred repository = %#v", alpha)
	}
	if beta == nil || beta.ForkedFrom != "inj1oldowner/alpha" {
		t.Fatalf("fork repository = %#v", beta)
	}
	if strings.Join(inventory.Owners, ",") != "inj1forkowner,inj1newowner" || len(inventory.ReportIDs) != 1 || inventory.ReportIDs[0] != 7 {
		t.Fatalf("owners/reports = %#v / %#v", inventory.Owners, inventory.ReportIDs)
	}
	if len(inventory.Usernames) != 1 || inventory.Usernames[0] != (InventoryUsername{Name: "bob", Owner: "inj1bob"}) {
		t.Fatalf("usernames = %#v", inventory.Usernames)
	}
	if strings.Join(inventory.BadgeRecipients, ",") != "inj1recipient" || strings.Join(inventory.ReleaseVersions, ",") != "v1.0.0" {
		t.Fatalf("badge/release inventory = %#v / %#v", inventory.BadgeRecipients, inventory.ReleaseVersions)
	}
	if len(inventory.ModerationTrail) != 1 || inventory.ModerationTrail[0].Timestamp != 104 ||
		inventory.ModerationTrail[0].Action != "submitted" || inventory.ModerationTrail[0].Actor != "inj1reporter" {
		t.Fatalf("moderation trail = %#v", inventory.ModerationTrail)
	}
	if len(inventory.Blocks) != 10 || inventory.CutoverBlockHash != inventory.Blocks[9].Hash {
		t.Fatalf("block evidence = %#v", inventory.Blocks)
	}
	rendered, err := MarshalInventory(inventory)
	if err != nil || !strings.HasSuffix(string(rendered), "\n") {
		t.Fatalf("MarshalInventory err=%v raw=%q", err, rendered)
	}
}

func TestBuildInventoryFailsClosedOnCoverageAmbiguity(t *testing.T) {
	tests := []struct {
		name   string
		raw    func(t *testing.T) []byte
		height uint64
		want   string
	}{
		{
			name: "height beyond cutover", height: 9, want: "exceeds cutover",
			raw: func(t *testing.T) []byte {
				var pages []map[string]any
				if err := json.Unmarshal(testTxSearchFixture(t), &pages); err != nil {
					t.Fatal(err)
				}
				for _, page := range pages {
					page["result"].(map[string]any)["query"] = expectedTxSearchQuery(testV1Contract, 9)
				}
				raw, _ := json.Marshal(pages)
				return raw
			},
		},
		{
			name: "duplicate position", height: 10, want: "duplicate transaction position",
			raw: func(t *testing.T) []byte {
				transaction := testTx("A", 1, 0, 0, testEvent("update_ref", map[string]string{}, false))
				page := testTxSearchPage(10, transaction, transaction)
				raw, _ := json.Marshal(page)
				return raw
			},
		},
		{
			name: "duplicate report", height: 10, want: "duplicate moderation report ID",
			raw: func(t *testing.T) []byte {
				page := testTxSearchPage(10,
					testTx("A", 1, 0, 0, testEvent("submit_moderation_report", map[string]string{
						"report_id": "1", "owner": "inj1owner", "repo": "alpha", "reporter": "inj1reporter", "reason_hash": "first",
					}, false)),
					testTx("B", 2, 0, 0, testEvent("submit_moderation_report", map[string]string{
						"report_id": "1", "owner": "inj1owner", "repo": "alpha", "reporter": "inj1reporter", "reason_hash": "second",
					}, false)),
				)
				raw, _ := json.Marshal(page)
				return raw
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := test.raw(t)
			evidence := testBlockEvidence(t, raw, "injective-888", test.height)
			_, err := BuildInventory(raw, "injective-888", testV1Contract, test.height, evidence)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildInventoryRejectsMissingBlockEvidence(t *testing.T) {
	_, err := BuildInventory(testTxSearchFixture(t), "injective-888", testV1Contract, 10, nil)
	if err == nil || !strings.Contains(err.Error(), "block evidence") {
		t.Fatalf("error = %v, want missing block evidence", err)
	}
}

func TestVerifySnapshotInventoryRequiresExactFixedHeightCoverage(t *testing.T) {
	raw := testTxSearchFixture(t)
	inventory, err := BuildInventory(raw, "injective-888", testV1Contract, 10, testBlockEvidence(t, raw, "injective-888", 10))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := map[string]any{
		"schema": SnapshotSchema,
		"source": map[string]any{
			"chain_id": "injective-888", "contract": testV1Contract, "height": "10",
			"block_hash": inventory.CutoverBlockHash,
		},
		"repositories": []any{
			map[string]any{"owner": "inj1newowner", "name": "alpha", "forked_from": nil},
			map[string]any{"owner": "inj1forkowner", "name": "beta", "forked_from": "inj1oldowner/alpha"},
		},
		"moderation_reports":  []any{map[string]any{"id": 7}},
		"usernames":           []any{map[string]any{"name": "bob", "owner": "inj1bob"}},
		"badges_by_recipient": []any{map[string]any{"recipient": "inj1recipient"}},
		"releases":            []any{map[string]any{"version": "v1.0.0"}},
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	write := func(value any) {
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(path, raw, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	write(snapshot)
	if err := VerifySnapshotInventory(path, inventory); err != nil {
		t.Fatalf("VerifySnapshotInventory: %v", err)
	}

	snapshot["moderation_reports"] = []any{}
	write(snapshot)
	if err := VerifySnapshotInventory(path, inventory); err == nil || !strings.Contains(err.Error(), "report inventory") {
		t.Fatalf("missing report error = %v", err)
	}
	snapshot["moderation_reports"] = []any{map[string]any{"id": 7}}
	snapshot["source"].(map[string]any)["height"] = "9"
	write(snapshot)
	if err := VerifySnapshotInventory(path, inventory); err == nil || !strings.Contains(err.Error(), "source does not match") {
		t.Fatalf("height mismatch error = %v", err)
	}
}

func TestVerifyInventoryEvidenceRejectsReorgAndInventoryTampering(t *testing.T) {
	raw := testTxSearchFixture(t)
	evidence := testBlockEvidence(t, raw, "injective-888", 10)
	inventory, err := BuildInventory(raw, "injective-888", testV1Contract, 10, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyInventoryEvidence(inventory, raw, evidence); err != nil {
		t.Fatalf("VerifyInventoryEvidence: %v", err)
	}

	tampered := *inventory
	tampered.SuccessfulTxs--
	if err := VerifyInventoryEvidence(&tampered, raw, evidence); err == nil || !strings.Contains(err.Error(), "does not match saved inventory") {
		t.Fatalf("inventory tampering error = %v", err)
	}

	reorg := *evidence
	reorg.blocks = make(map[uint64]sourceBlock, len(evidence.blocks))
	for height, block := range evidence.blocks {
		reorg.blocks[height] = block
	}
	block := reorg.blocks[10]
	block.hash = strings.Repeat("a", 64)
	reorg.blocks[10] = block
	if err := VerifyBlockEvidence(inventory, &reorg); err == nil || !strings.Contains(err.Error(), "reorg") {
		t.Fatalf("reorg error = %v", err)
	}
}

func TestBuildInventoryRejectsIncompletePagesWrongQueryAndUnknownAction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "missing page", want: "total_count",
			mutate: func(page map[string]any) {
				page["result"].(map[string]any)["total_count"] = "2"
			},
		},
		{
			name: "wrong query", want: "must exactly equal",
			mutate: func(page map[string]any) {
				page["result"].(map[string]any)["query"] = "wasm._contract_address='other'"
			},
		},
		{
			name: "unknown action", want: "unknown target-contract action",
			mutate: func(page map[string]any) {
				page["result"].(map[string]any)["txs"] = []any{
					testTx("unknown", 1, 0, 0, testEvent("future_state_key", map[string]string{}, false)),
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := testTxSearchPage(10, testTx("known", 1, 0, 0, testEvent("update_ref", map[string]string{}, false)))
			tc.mutate(page)
			raw, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			evidence := testBlockEvidence(t, raw, "injective-888", 10)
			_, err = BuildInventory(raw, "injective-888", testV1Contract, 10, evidence)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildInventoryAcceptsCompleteEmptyHistoryWithCutoverBlock(t *testing.T) {
	page := testTxSearchPage(10)
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	evidence := testBlockEvidence(t, raw, "injective-888", 10)
	inventory, err := BuildInventory(raw, "injective-888", testV1Contract, 10, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.ScannedTxs != 0 || inventory.SuccessfulTxs != 0 || len(inventory.Blocks) != 1 || inventory.Blocks[0].Height != 10 {
		t.Fatalf("empty inventory = %#v", inventory)
	}
}

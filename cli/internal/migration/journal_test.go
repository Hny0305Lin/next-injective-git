package migration

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func TestReceiptJournalRecordsImmutableOrderedLifecycleAndReopens(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "receipt-journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Header().TransactionCount != len(manifest.Transactions) || journal.Header().Signer != signer {
		t.Fatalf("journal header = %#v", journal.Header())
	}
	if err := journal.RecordBroadcast(0, journalTestHash); err == nil || !strings.Contains(err.Error(), "must be prepared") {
		t.Fatalf("broadcast-before-prepare error = %v", err)
	}

	first := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := journal.RecordPrepared(0, first); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordPrepared(0, first); err == nil || !strings.Contains(err.Error(), "already has") {
		t.Fatalf("duplicate prepared error = %v", err)
	}
	second := signedJournalTransaction(t, key, plan, manifest, 1)
	if err := journal.RecordPrepared(1, second); err == nil || !strings.Contains(err.Error(), "before transaction 0 is mined") {
		t.Fatalf("out-of-order prepare error = %v", err)
	}
	if err := journal.RecordMined(0, journalReceipt(t, plan, manifest, 0, first.TransactionHash)); err == nil || !strings.Contains(err.Error(), "must be broadcast") {
		t.Fatalf("mined-before-broadcast error = %v", err)
	}
	if err := journal.RecordBroadcast(0, journalTestHash); err == nil || !strings.Contains(err.Error(), "hash does not match") {
		t.Fatalf("wrong broadcast hash error = %v", err)
	}
	if err := journal.RecordBroadcast(0, first.TransactionHash); err != nil {
		t.Fatal(err)
	}
	missingEvent := journalReceipt(t, plan, manifest, 0, first.TransactionHash)
	missingEvent.Logs = nil
	if err := journal.RecordMined(0, missingEvent); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("missing event error = %v", err)
	}
	if err := journal.RecordMined(0, journalReceipt(t, plan, manifest, 0, first.TransactionHash)); err != nil {
		t.Fatal(err)
	}

	for order := 1; order < len(manifest.Transactions); order++ {
		signed := signedJournalTransaction(t, key, plan, manifest, order)
		if err := journal.RecordPrepared(order, signed); err != nil {
			t.Fatalf("prepare transaction %d: %v", order, err)
		}
		if err := journal.RecordBroadcast(order, signed.TransactionHash); err != nil {
			t.Fatalf("broadcast transaction %d: %v", order, err)
		}
		if err := journal.RecordMined(order, journalReceipt(t, plan, manifest, order, signed.TransactionHash)); err != nil {
			t.Fatalf("mine transaction %d: %v", order, err)
		}
	}

	reopened, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	for order := range manifest.Transactions {
		prepared, ok := reopened.Prepared(order)
		if !ok || prepared.Transaction.Nonce != uint64(order) {
			t.Fatalf("prepared transaction %d = %#v, ok=%v", order, prepared, ok)
		}
		broadcast, ok := reopened.Broadcast(order)
		if !ok || broadcast.TransactionHash != prepared.Transaction.TransactionHash {
			t.Fatalf("broadcast transaction %d = %#v, ok=%v", order, broadcast, ok)
		}
		mined, ok := reopened.Mined(order)
		if !ok || mined.Receipt.Status != "0x1" || mined.Receipt.BlockNumber != "0x2a" {
			t.Fatalf("mined transaction %d = %#v, ok=%v", order, mined, ok)
		}
	}
	status, err := InspectReceiptJournal(directory, plan, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Complete || status.PreparedTransactions != len(manifest.Transactions) ||
		status.BroadcastTransactions != len(manifest.Transactions) ||
		status.MinedTransactions != len(manifest.Transactions) ||
		status.NextOrder != len(manifest.Transactions) || status.FinalizeTransactionHash == "" {
		t.Fatalf("journal status = %#v", status)
	}
	if err := reopened.RecordMined(0, journalReceipt(t, plan, manifest, 0, first.TransactionHash)); err == nil || !strings.Contains(err.Error(), "already has") {
		t.Fatalf("mined overwrite error = %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(directory, receiptJournalTemporaryPrefix+"*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary journal files = %v, err=%v", entries, err)
	}
}

func TestReceiptJournalValidatesSignedRawTransactionBeforePersistence(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	tests := []struct {
		name   string
		mutate func(*SignedImportTransaction)
		want   string
	}{
		{name: "sender", mutate: func(tx *SignedImportTransaction) { tx.From = "0x2222222222222222222222222222222222222222" }, want: "sender"},
		{name: "chain", mutate: func(tx *SignedImportTransaction) { tx.ChainID++ }, want: "chain ID"},
		{name: "nonce", mutate: func(tx *SignedImportTransaction) { tx.Nonce++ }, want: "nonce or gas limit"},
		{name: "gas", mutate: func(tx *SignedImportTransaction) { tx.GasLimit++ }, want: "nonce or gas limit"},
		{name: "gas price", mutate: func(tx *SignedImportTransaction) { tx.GasPrice = "0x3" }, want: "gas price"},
		{name: "value", mutate: func(tx *SignedImportTransaction) { tx.Value = "0x1" }, want: "value"},
		{name: "hash", mutate: func(tx *SignedImportTransaction) { tx.TransactionHash = journalTestHash }, want: "hash does not match"},
		{name: "raw", mutate: func(tx *SignedImportTransaction) { tx.RawTransaction = "0x1234" }, want: "decode signed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "journal")
			journal, err := OpenReceiptJournal(directory, plan, manifest, signer)
			if err != nil {
				t.Fatal(err)
			}
			signed := signedJournalTransaction(t, key, plan, manifest, 0)
			tc.mutate(&signed)
			err = journal.RecordPrepared(0, signed)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if _, statErr := os.Lstat(journal.recordPath(0, "prepared")); !os.IsNotExist(statErr) {
				t.Fatalf("invalid signed transaction was persisted: %v", statErr)
			}
		})
	}
}

func TestReceiptJournalRecordsRevertedReceiptAsTerminalEvidence(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "reverted-journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	signed := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := journal.RecordPrepared(0, signed); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordBroadcast(0, signed.TransactionHash); err != nil {
		t.Fatal(err)
	}
	receipt := journalReceipt(t, plan, manifest, 0, signed.TransactionHash)
	receipt.Status = "0"
	receipt.Logs = nil
	if err := journal.RecordRevertedAt(0, receipt, &chain.EVMBlock{Number: "0x2a", Hash: journalBlockHash}); err != nil {
		t.Fatalf("record reverted receipt: %v", err)
	}
	if err := journal.RecordReverted(0, receipt); err == nil || !strings.Contains(err.Error(), "already has a reverted") {
		t.Fatalf("duplicate reverted receipt error = %v", err)
	}
	if err := journal.RecordMined(0, journalReceipt(t, plan, manifest, 0, signed.TransactionHash)); err == nil || !strings.Contains(err.Error(), "already has a reverted") {
		t.Fatalf("reverted-to-mined error = %v", err)
	}

	reopened, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	reverted, ok := reopened.Reverted(0)
	if !ok || reverted.Receipt.Status != "0x0" || reverted.Receipt.TransactionHash != signed.TransactionHash {
		t.Fatalf("reverted record = %#v, ok=%v", reverted, ok)
	}
	status, err := InspectReceiptJournal(directory, plan, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if status.Complete || status.RevertedTransactions != 1 || status.FailedOrder != 0 ||
		status.RevertedTransactionHash != signed.TransactionHash || status.NextOrder != 0 {
		t.Fatalf("reverted journal status = %#v", status)
	}
	if _, err := os.Stat(filepath.Join(directory, "000000.reverted.json")); err != nil {
		t.Fatalf("reverted evidence file: %v", err)
	}

	badDirectory := filepath.Join(t.TempDir(), "bad-block")
	badJournal, err := OpenReceiptJournal(badDirectory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	badSigned := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := badJournal.RecordPrepared(0, badSigned); err != nil {
		t.Fatal(err)
	}
	if err := badJournal.RecordBroadcast(0, badSigned.TransactionHash); err != nil {
		t.Fatal(err)
	}
	badReceipt := journalReceipt(t, plan, manifest, 0, badSigned.TransactionHash)
	badReceipt.Status = "0x0"
	badReceipt.Logs = nil
	if err := badJournal.RecordRevertedAt(0, badReceipt, &chain.EVMBlock{Number: "0x2a", Hash: "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}); err == nil || !strings.Contains(err.Error(), "does not match receipt") {
		t.Fatalf("mismatched canonical block error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(badDirectory, "000000.reverted.json")); !os.IsNotExist(err) {
		t.Fatalf("mismatched block wrote reverted evidence: %v", err)
	}

	t.Run("canonical block is required", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "nil-block")
		candidate, err := OpenReceiptJournal(directory, plan, manifest, signer)
		if err != nil {
			t.Fatal(err)
		}
		signed := signedJournalTransaction(t, key, plan, manifest, 0)
		if err := candidate.RecordPrepared(0, signed); err != nil {
			t.Fatal(err)
		}
		if err := candidate.RecordBroadcast(0, signed.TransactionHash); err != nil {
			t.Fatal(err)
		}
		reverted := journalReceipt(t, plan, manifest, 0, signed.TransactionHash)
		reverted.Status = "0x0"
		reverted.Logs = nil
		err = candidate.RecordRevertedAt(0, reverted, nil)
		if err == nil || !strings.Contains(err.Error(), "canonical block is nil") {
			t.Fatalf("nil canonical block error = %v", err)
		}
		if _, statErr := os.Stat(candidate.recordPath(0, "reverted")); !os.IsNotExist(statErr) {
			t.Fatalf("nil block wrote reverted evidence: %v", statErr)
		}
	})

	t.Run("canonical block number must match", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "wrong-block-number")
		candidate, err := OpenReceiptJournal(directory, plan, manifest, signer)
		if err != nil {
			t.Fatal(err)
		}
		signed := signedJournalTransaction(t, key, plan, manifest, 0)
		if err := candidate.RecordPrepared(0, signed); err != nil {
			t.Fatal(err)
		}
		if err := candidate.RecordBroadcast(0, signed.TransactionHash); err != nil {
			t.Fatal(err)
		}
		reverted := journalReceipt(t, plan, manifest, 0, signed.TransactionHash)
		reverted.Status = "0x0"
		reverted.Logs = nil
		err = candidate.RecordRevertedAt(0, reverted, &chain.EVMBlock{Number: "0x2b", Hash: journalBlockHash})
		if err == nil || !strings.Contains(err.Error(), "does not match receipt") {
			t.Fatalf("wrong canonical block number error = %v", err)
		}
		if _, statErr := os.Stat(candidate.recordPath(0, "reverted")); !os.IsNotExist(statErr) {
			t.Fatalf("wrong block number wrote reverted evidence: %v", statErr)
		}
	})

	t.Run("tampered reverted evidence is rejected", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "tampered-reverted")
		candidate, err := OpenReceiptJournal(directory, plan, manifest, signer)
		if err != nil {
			t.Fatal(err)
		}
		signed := signedJournalTransaction(t, key, plan, manifest, 0)
		if err := candidate.RecordPrepared(0, signed); err != nil {
			t.Fatal(err)
		}
		if err := candidate.RecordBroadcast(0, signed.TransactionHash); err != nil {
			t.Fatal(err)
		}
		reverted := journalReceipt(t, plan, manifest, 0, signed.TransactionHash)
		reverted.Status = "0x0"
		reverted.Logs = nil
		if err := candidate.RecordRevertedAt(0, reverted, &chain.EVMBlock{Number: "0x2a", Hash: journalBlockHash}); err != nil {
			t.Fatal(err)
		}
		path := candidate.recordPath(0, "reverted")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var tampered RevertedJournalRecord
		if err := json.Unmarshal(raw, &tampered); err != nil {
			t.Fatal(err)
		}
		tampered.Receipt.TransactionHash = journalTestHash
		raw, err = json.MarshalIndent(tampered, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenReceiptJournal(directory, plan, manifest, signer); err == nil || !strings.Contains(err.Error(), "hash does not match") {
			t.Fatalf("tampered reverted evidence error = %v", err)
		}
	})
}

func TestReceiptJournalPublishedButUnsyncedTransitionRemainsRecoverable(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	syncFailure := errors.New("injected directory sync failure")
	journal.syncDirectory = func(string) error { return syncFailure }
	signed := signedJournalTransaction(t, key, plan, manifest, 0)
	err = journal.RecordPrepared(0, signed)
	var published *JournalPublishedUnsyncedError
	if !errors.As(err, &published) || !errors.Is(err, syncFailure) {
		t.Fatalf("published-but-unsynced error = %v", err)
	}
	if published.Path != journal.recordPath(0, "prepared") {
		t.Fatalf("published path = %q", published.Path)
	}
	if !strings.Contains(err.Error(), "was published") || !strings.Contains(err.Error(), "reopen the journal") {
		t.Fatalf("published-but-unsynced recovery guidance = %v", err)
	}
	if strings.Contains(err.Error(), signed.RawTransaction) {
		t.Fatal("published-but-unsynced error exposed signed raw transaction")
	}
	if _, statErr := os.Lstat(published.Path); statErr != nil {
		t.Fatalf("published transition is missing: %v", statErr)
	}
	if prepared, ok := journal.Prepared(0); !ok || prepared.Transaction.TransactionHash != signed.TransactionHash {
		t.Fatalf("in-memory published transition = %#v, ok=%v", prepared, ok)
	}
	if retryErr := journal.RecordPrepared(0, signed); retryErr == nil || !strings.Contains(retryErr.Error(), "already has") {
		t.Fatalf("same-process retry after publication = %v", retryErr)
	}
	reopened, err := OpenReceiptJournal(directory, plan, manifest, signer)
	if err != nil {
		t.Fatalf("reopen published transition: %v", err)
	}
	if prepared, ok := reopened.Prepared(0); !ok || prepared.Transaction.TransactionHash != signed.TransactionHash {
		t.Fatalf("reopened published transition = %#v, ok=%v", prepared, ok)
	}
}

func TestReceiptJournalNestedDirectorySyncCoversCreatedAncestors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory Sync semantics do not apply on Windows")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "one", "two", "journal")
	var synced []string
	err := prepareReceiptJournalDirectoryWithSync(directory, func(path string) error {
		synced = append(synced, filepath.Clean(path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "one", "two"), filepath.Join(root, "one"), root}
	if len(synced) != len(want) {
		t.Fatalf("synced directories = %#v, want %#v", synced, want)
	}
	for index := range want {
		if synced[index] != want[index] {
			t.Fatalf("synced directory %d = %q, want %q", index, synced[index], want[index])
		}
	}
}

func TestReceiptJournalRejectsPermissivePOSIXModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are not represented by os.FileMode")
	}
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())

	for _, tc := range []struct {
		name string
		path func(string) string
		mode os.FileMode
	}{
		{name: "directory", path: func(directory string) string { return directory }, mode: 0o755},
		{name: "header", path: func(directory string) string { return filepath.Join(directory, receiptJournalHeaderFile) }, mode: 0o644},
		{name: "record", path: func(directory string) string { return filepath.Join(directory, "000000.prepared.json") }, mode: 0o644},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "journal")
			journal, err := OpenReceiptJournal(directory, plan, manifest, signer)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "record" {
				if err := journal.RecordPrepared(0, signedJournalTransaction(t, key, plan, manifest, 0)); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(tc.path(directory), tc.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenReceiptJournal(directory, plan, manifest, signer); err == nil || !strings.Contains(err.Error(), "accessible by group or others") {
				t.Fatalf("permissive %s mode error = %v", tc.name, err)
			}
		})
	}
}

func TestReceiptJournalAcceptsDirectRegistryImportEvidence(t *testing.T) {
	paths := writeSnapshotFixture(t, validSnapshot())
	plan, err := BuildPlan(Options{
		SnapshotPath: paths.snapshot, HashPath: paths.hash,
		TargetChainID: 1439, TargetContract: testTargetContract, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	journal, err := OpenReceiptJournal(filepath.Join(t.TempDir(), "direct"), plan, manifest, signer)
	if err != nil {
		t.Fatal(err)
	}
	for order := range manifest.Transactions {
		signed := signedJournalTransaction(t, key, plan, manifest, order)
		if err := journal.RecordPrepared(order, signed); err != nil {
			t.Fatalf("prepare transaction %d: %v", order, err)
		}
		if err := journal.RecordBroadcast(order, signed.TransactionHash); err != nil {
			t.Fatalf("broadcast transaction %d: %v", order, err)
		}
		if err := journal.RecordMined(order, journalReceipt(t, plan, manifest, order, signed.TransactionHash)); err != nil {
			t.Fatalf("mine transaction %d: %v", order, err)
		}
	}
}

func TestReceiptJournalRejectsMismatchedHeaderTamperingAndUnexpectedFiles(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())

	t.Run("signer mismatch", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "journal")
		if _, err := OpenReceiptJournal(directory, plan, manifest, signer); err != nil {
			t.Fatal(err)
		}
		_, err := OpenReceiptJournal(directory, plan, manifest, "0x3333333333333333333333333333333333333333")
		if err == nil || !strings.Contains(err.Error(), "header does not match") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unknown header field", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "journal")
		if _, err := OpenReceiptJournal(directory, plan, manifest, signer); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, receiptJournalHeaderFile)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"unknown\": true,\n"), 1)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = OpenReceiptJournal(directory, plan, manifest, signer)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unexpected file", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "journal")
		if _, err := OpenReceiptJournal(directory, plan, manifest, signer); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("unexpected"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenReceiptJournal(directory, plan, manifest, signer)
		if err == nil || !strings.Contains(err.Error(), "unexpected file") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("nonempty directory without header", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.WriteFile(filepath.Join(directory, "keep"), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenReceiptJournal(directory, plan, manifest, signer)
		if err == nil || !strings.Contains(err.Error(), "not empty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("tampered manifest", func(t *testing.T) {
		tampered, err := BuildTransactionManifest(plan)
		if err != nil {
			t.Fatal(err)
		}
		tampered.Transactions[0].Data = "0x1234"
		directory := filepath.Join(t.TempDir(), "not-created")
		_, err = OpenReceiptJournal(directory, plan, tampered, signer)
		if err == nil || !strings.Contains(err.Error(), "does not exactly match") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Lstat(directory); !os.IsNotExist(statErr) {
			t.Fatalf("invalid manifest created journal directory: %v", statErr)
		}
	})
}

const (
	journalBlockHash = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	journalTestHash  = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func signedJournalTransaction(
	t *testing.T,
	key *ecdsa.PrivateKey,
	plan *Plan,
	manifest *TransactionManifest,
	order int,
) SignedImportTransaction {
	t.Helper()
	manifestTransaction := manifest.Transactions[order]
	data, err := hex.DecodeString(strings.TrimPrefix(manifestTransaction.Data, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	target := common.HexToAddress(manifestTransaction.To)
	unsigned := types.NewTx(&types.LegacyTx{
		Nonce:    uint64(order),
		To:       &target,
		Value:    big.NewInt(0),
		Gas:      2_000_000,
		GasPrice: big.NewInt(2),
		Data:     data,
	})
	signed, err := types.SignTx(unsigned, types.LatestSignerForChainID(new(big.Int).SetUint64(plan.Target.ChainID)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return SignedImportTransaction{
		From:    strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex()),
		ChainID: plan.Target.ChainID, Nonce: uint64(order), GasLimit: 2_000_000,
		GasPrice: "0x2", Value: "0x0", RawTransaction: "0x" + hex.EncodeToString(raw),
		TransactionHash: signed.Hash().Hex(),
	}
}

func journalReceipt(
	t *testing.T,
	plan *Plan,
	manifest *TransactionManifest,
	order int,
	transactionHash string,
) *chain.EVMReceipt {
	t.Helper()
	transaction := manifest.Transactions[order]
	topics := []string{transaction.ExpectedTopic, manifest.SessionID}
	data := "0x"
	switch transaction.Phase {
	case "create_session":
		topics = append(topics, manifest.SessionID)
		if plan.Target.Controller != "" {
			topics = append(topics, manifest.ImportCommitment)
		} else {
			topics = append(topics, "0x"+strings.Repeat("0", 24)+strings.TrimPrefix(plan.Snapshot.ContractEVM, "0x"))
		}
	case "import_repo", "import_refs", "import_collaborators":
		topics = append(topics, uint256Topic(uint64(*transaction.Sequence)), transaction.RepoID)
		data = "0x" + transaction.PayloadSHA256 + strings.Repeat("0", 64)
	case "finalize":
		if plan.Target.Controller != "" {
			topics = append(topics, manifest.ImportCommitment)
		} else {
			topics = append(topics, manifest.SessionID)
			data = directFinalizationData(t, plan)
		}
	default:
		t.Fatalf("unsupported manifest phase %q", transaction.Phase)
	}
	return &chain.EVMReceipt{
		TransactionHash: transactionHash,
		BlockNumber:     "0x2a",
		BlockHash:       journalBlockHash,
		To:              transaction.To,
		Status:          "0x1",
		GasUsed:         "0x5208",
		Logs: []chain.EVMLog{{
			Address: transaction.EventEmitter, Topics: topics, Data: data,
			BlockNumber: "0x2a", BlockHash: journalBlockHash, TransactionHash: transactionHash,
		}},
	}
}

func directFinalizationData(t *testing.T, plan *Plan) string {
	t.Helper()
	arguments := abi.Arguments{
		{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)}, {Type: mustABIType("uint256", nil)},
	}
	encoded, err := arguments.Pack(
		big.NewInt(int64(len(plan.Repositories))),
		big.NewInt(int64(plan.Summary.RefCount)),
		big.NewInt(int64(plan.Summary.CollaboratorCount)),
		big.NewInt(int64(len(plan.Batches))),
	)
	if err != nil {
		t.Fatal(err)
	}
	return "0x" + hex.EncodeToString(encoded)
}

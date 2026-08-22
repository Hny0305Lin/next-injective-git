package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestJournal_CreateAndLoad(t *testing.T) {
	// Create temporary journal file
	tmpfile, err := os.CreateTemp("", "journal-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Open new journal
	j, err := OpenJournal(tmpfile.Name())
	if err != nil {
		t.Fatalf("OpenJournal failed: %v", err)
	}
	defer j.Close()

	if len(j.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(j.entries))
	}
}

func TestJournal_AppendAndReload(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "journal-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Create a test private key
	privateKey, err := generateTestPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	// Open journal and append entry
	j1, err := OpenJournal(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	entry := JournalEntry{
		Timestamp:   time.Now(),
		BatchIndex:  0,
		CallData:    "0x1234",
		GasEstimate: 100000,
		GasPrice:    160000000,
		Status:      StatusPrepared,
	}

	if err := j1.Append(entry, privateKey); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	j1.Close()

	// Reopen journal and verify entry was persisted
	j2, err := OpenJournal(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()

	if len(j2.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(j2.entries))
	}

	loaded := j2.entries[0]
	if loaded.BatchIndex != 0 {
		t.Errorf("expected batchIndex 0, got %d", loaded.BatchIndex)
	}
	if loaded.CallData != "0x1234" {
		t.Errorf("expected callData 0x1234, got %s", loaded.CallData)
	}
	if loaded.Status != StatusPrepared {
		t.Errorf("expected status prepared, got %s", loaded.Status)
	}
	if loaded.Signature == "" {
		t.Error("expected signature, got empty")
	}
}

func TestJournal_LastEntry(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "journal-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	j, err := OpenJournal(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	// Empty journal
	if last := j.LastEntry(); last != nil {
		t.Error("expected nil for empty journal")
	}

	// Append entries
	privateKey, _ := generateTestPrivateKey()
	for i := 0; i < 3; i++ {
		entry := JournalEntry{
			Timestamp:   time.Now(),
			BatchIndex:  i,
			CallData:    "0x",
			GasEstimate: 100000,
			GasPrice:    160000000,
			Status:      StatusPrepared,
		}
		if err := j.Append(entry, privateKey); err != nil {
			t.Fatal(err)
		}
	}

	// Check last entry
	last := j.LastEntry()
	if last == nil {
		t.Fatal("expected last entry, got nil")
	}
	if last.BatchIndex != 2 {
		t.Errorf("expected last batchIndex 2, got %d", last.BatchIndex)
	}
}

func TestJournal_GetConfirmedBatches(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "journal-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	j, err := OpenJournal(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	privateKey, _ := generateTestPrivateKey()

	// Add entries with different statuses
	statuses := []string{StatusPrepared, StatusConfirmed, StatusBroadcast, StatusConfirmed}
	for i, status := range statuses {
		entry := JournalEntry{
			Timestamp:   time.Now(),
			BatchIndex:  i,
			CallData:    "0x",
			GasEstimate: 100000,
			GasPrice:    160000000,
			Status:      status,
		}
		if err := j.Append(entry, privateKey); err != nil {
			t.Fatal(err)
		}
	}

	confirmed := j.GetConfirmedBatches()
	if len(confirmed) != 2 {
		t.Errorf("expected 2 confirmed batches, got %d", len(confirmed))
	}

	// Check indices (order not guaranteed)
	hasOne := false
	hasThree := false
	for _, idx := range confirmed {
		if idx == 1 {
			hasOne = true
		}
		if idx == 3 {
			hasThree = true
		}
	}
	if !hasOne || !hasThree {
		t.Errorf("expected batches 1 and 3 to be confirmed, got %v", confirmed)
	}
}

func TestJournalEntry_JSONRoundTrip(t *testing.T) {
	entry := JournalEntry{
		Timestamp:   time.Now().UTC().Truncate(time.Second),
		BatchIndex:  5,
		CallData:    "0xabcdef",
		GasEstimate: 200000,
		GasPrice:    160000000,
		TxHash:      "0x123",
		Status:      StatusConfirmed,
		BlockNumber: 12345,
		GasUsed:     180000,
		Signature:   "0xsig",
	}

	// Marshal
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal
	var decoded JournalEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Compare
	if decoded.BatchIndex != entry.BatchIndex {
		t.Errorf("batchIndex mismatch: got %d, want %d", decoded.BatchIndex, entry.BatchIndex)
	}
	if decoded.CallData != entry.CallData {
		t.Errorf("callData mismatch: got %s, want %s", decoded.CallData, entry.CallData)
	}
	if decoded.Status != entry.Status {
		t.Errorf("status mismatch: got %s, want %s", decoded.Status, entry.Status)
	}
}

// Helper function to generate a test private key
func generateTestPrivateKey() (*ecdsa.PrivateKey, error) {
	// Using go-ethereum's crypto package
	return crypto.GenerateKey()
}

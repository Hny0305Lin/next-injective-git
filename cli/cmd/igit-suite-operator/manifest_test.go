package main

import (
	"os"
	"testing"
)

func TestLoadManifest_Valid(t *testing.T) {
	manifest, err := LoadManifest("test-manifest.json")
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if manifest.ChainID != 1439 {
		t.Errorf("expected chainId 1439, got %d", manifest.ChainID)
	}

	if len(manifest.Batches) != 2 {
		t.Errorf("expected 2 batches, got %d", len(manifest.Batches))
	}

	// Check batch indices are sequential
	for i, batch := range manifest.Batches {
		if batch.Index != i {
			t.Errorf("batch %d has wrong index: got %d", i, batch.Index)
		}
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	_, err := LoadManifest("nonexistent.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	// Create temp file with invalid JSON
	tmpfile, err := os.CreateTemp("", "invalid-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.WriteString("{invalid json")
	tmpfile.Close()

	_, err = LoadManifest(tmpfile.Name())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadManifest_MissingChainID(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.WriteString(`{
		"directory": "0x123",
		"coordinator": "0x456",
		"batches": [{"index": 0, "callData": "0x00"}]
	}`)
	tmpfile.Close()

	_, err = LoadManifest(tmpfile.Name())
	if err == nil {
		t.Error("expected error for missing chainId")
	}
}

func TestLoadManifest_NoBatches(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.WriteString(`{
		"chainId": 1439,
		"directory": "0x123",
		"coordinator": "0x456",
		"batches": []
	}`)
	tmpfile.Close()

	_, err = LoadManifest(tmpfile.Name())
	if err == nil {
		t.Error("expected error for no batches")
	}
}

func TestLoadManifest_WrongBatchIndices(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.WriteString(`{
		"chainId": 1439,
		"directory": "0x123",
		"coordinator": "0x456",
		"batches": [
			{"index": 0, "callData": "0x00"},
			{"index": 2, "callData": "0x01"}
		]
	}`)
	tmpfile.Close()

	_, err = LoadManifest(tmpfile.Name())
	if err == nil {
		t.Error("expected error for non-sequential batch indices")
	}
}

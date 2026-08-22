package main

import (
	"bufio"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Journal status constants
const (
	StatusPrepared  = "prepared"  // Calldata ready, not yet broadcast
	StatusBroadcast = "broadcast" // Transaction sent, receipt pending
	StatusConfirmed = "confirmed" // Transaction confirmed on-chain
	StatusUncertain = "uncertain" // Receipt unknown after timeout
	StatusFailed    = "failed"    // Transaction failed
)

// JournalEntry represents a single signed operation in the append-only log
type JournalEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	BatchIndex  int       `json:"batchIndex"`
	CallData    string    `json:"callData"` // hex-encoded
	GasEstimate uint64    `json:"gasEstimate"`
	GasPrice    uint64    `json:"gasPrice"`
	TxHash      string    `json:"txHash,omitempty"`      // filled after broadcast
	Status      string    `json:"status"`                // "prepared", "broadcast", "confirmed", "uncertain", "failed"
	BlockNumber uint64    `json:"blockNumber,omitempty"` // filled after confirmation
	GasUsed     uint64    `json:"gasUsed,omitempty"`     // filled after confirmation
	Error       string    `json:"error,omitempty"`
	Signature   string    `json:"signature"` // ECDSA signature of entry hash
}

// Journal manages the append-only signed log
type Journal struct {
	path    string
	file    *os.File
	entries []JournalEntry
}

// OpenJournal opens or creates a journal file
func OpenJournal(path string) (*Journal, error) {
	var entries []JournalEntry

	// Try to read existing entries
	if file, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			var entry JournalEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				file.Close()
				return nil, fmt.Errorf("parse journal line %d: %w", lineNum, err)
			}

			// TODO: Verify signature
			// For now, just accept all entries
			entries = append(entries, entry)
		}
		file.Close()

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read journal: %w", err)
		}
	}

	// Open for append
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}

	return &Journal{
		path:    path,
		file:    file,
		entries: entries,
	}, nil
}

// Append adds a signed entry to the journal
func (j *Journal) Append(entry JournalEntry, privateKey *ecdsa.PrivateKey) error {
	// Compute entry hash (deterministic JSON, excluding signature)
	entryData := map[string]interface{}{
		"timestamp":   entry.Timestamp.Unix(),
		"batchIndex":  entry.BatchIndex,
		"callData":    entry.CallData,
		"gasEstimate": entry.GasEstimate,
		"gasPrice":    entry.GasPrice,
		"status":      entry.Status,
	}

	// Include optional fields if present
	if entry.TxHash != "" {
		entryData["txHash"] = entry.TxHash
	}
	if entry.BlockNumber > 0 {
		entryData["blockNumber"] = entry.BlockNumber
	}
	if entry.GasUsed > 0 {
		entryData["gasUsed"] = entry.GasUsed
	}
	if entry.Error != "" {
		entryData["error"] = entry.Error
	}

	entryJSON, err := json.Marshal(entryData)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	// Sign the entry hash
	hash := crypto.Keccak256Hash(entryJSON)
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return fmt.Errorf("sign entry: %w", err)
	}

	entry.Signature = fmt.Sprintf("0x%x", signature)

	// Write to file (JSON Lines format)
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal signed entry: %w", err)
	}

	if _, err := j.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write journal: %w", err)
	}

	// Sync to disk immediately for durability
	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("sync journal: %w", err)
	}

	j.entries = append(j.entries, entry)
	return nil
}

// LastEntry returns the most recent journal entry
func (j *Journal) LastEntry() *JournalEntry {
	if len(j.entries) == 0 {
		return nil
	}
	return &j.entries[len(j.entries)-1]
}

// GetConfirmedBatches returns indices of all confirmed batches
func (j *Journal) GetConfirmedBatches() []int {
	confirmed := make(map[int]bool)
	for _, entry := range j.entries {
		if entry.Status == StatusConfirmed {
			confirmed[entry.BatchIndex] = true
		}
	}

	result := make([]int, 0, len(confirmed))
	for idx := range confirmed {
		result = append(result, idx)
	}
	return result
}

// Close closes the journal file
func (j *Journal) Close() error {
	if j.file != nil {
		return j.file.Close()
	}
	return nil
}

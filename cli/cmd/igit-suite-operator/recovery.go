package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

// RecoveryState holds the state recovered from journal
type RecoveryState struct {
	NextBatchIndex int
	LastStatus     string
	LastTxHash     string
	NeedReceiptCheck bool
}

// RecoverFromJournal analyzes the journal and determines next action
func RecoverFromJournal(ctx context.Context, journal *Journal, client *ethclient.Client) (*RecoveryState, error) {
	lastEntry := journal.LastEntry()

	// No previous entries, start fresh
	if lastEntry == nil {
		return &RecoveryState{
			NextBatchIndex:   0,
			LastStatus:       "",
			LastTxHash:       "",
			NeedReceiptCheck: false,
		}, nil
	}

	state := &RecoveryState{
		LastStatus: lastEntry.Status,
		LastTxHash: lastEntry.TxHash,
	}

	switch lastEntry.Status {
	case StatusConfirmed:
		// Last batch succeeded, continue to next
		state.NextBatchIndex = lastEntry.BatchIndex + 1
		state.NeedReceiptCheck = false

	case StatusBroadcast, StatusUncertain:
		// Need to check receipt to determine actual status
		state.NextBatchIndex = lastEntry.BatchIndex
		state.NeedReceiptCheck = true

		// Try to query receipt
		if client != nil && lastEntry.TxHash != "" {
			receipt, err := QueryReceipt(ctx, client, lastEntry.TxHash)
			if err == nil {
				// Got receipt, update journal
				if receipt.Status == 1 {
					state.LastStatus = StatusConfirmed
					state.NextBatchIndex = lastEntry.BatchIndex + 1
					state.NeedReceiptCheck = false
				} else {
					state.LastStatus = StatusFailed
				}
			}
			// If error (not found), leave as uncertain
		}

	case StatusFailed:
		// Manual intervention required
		return nil, fmt.Errorf("last batch %d failed, manual review required", lastEntry.BatchIndex)

	case StatusPrepared:
		// Transaction was prepared but not broadcast, can retry
		state.NextBatchIndex = lastEntry.BatchIndex
		state.NeedReceiptCheck = false

	default:
		return nil, fmt.Errorf("unknown status: %s", lastEntry.Status)
	}

	return state, nil
}

// UpdateJournalWithReceipt updates the journal with receipt information
func UpdateJournalWithReceipt(journal *Journal, privateKey *ecdsa.PrivateKey, batchIndex int, txHash string, success bool) error {
	status := StatusConfirmed
	if !success {
		status = StatusFailed
	}

	entry := JournalEntry{
		Timestamp:  time.Now(),
		BatchIndex: batchIndex,
		CallData:   "", // Not needed for receipt update
		TxHash:     txHash,
		Status:     status,
		Error:      "Receipt verified during recovery",
	}

	return journal.Append(entry, privateKey)
}

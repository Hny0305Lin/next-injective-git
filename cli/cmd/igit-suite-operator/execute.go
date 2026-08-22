package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ExecutionConfig holds configuration for batch execution
type ExecutionConfig struct {
	RPC              string
	ChainID          uint64
	GasPrice         uint64
	ReceiptTimeout   time.Duration
	DryRun           bool
	CoordinatorAddr  common.Address
}

// ExecuteBatches executes all batches from the manifest
func ExecuteBatches(
	ctx context.Context,
	manifest *Manifest,
	journal *Journal,
	privateKey *ecdsa.PrivateKey,
	config ExecutionConfig,
) error {
	// Connect to RPC (unless dry-run)
	var client *ethclient.Client
	var err error

	if !config.DryRun {
		client, err = ethclient.Dial(config.RPC)
		if err != nil {
			return fmt.Errorf("connect to RPC: %w", err)
		}
		defer client.Close()
	}

	// Recover from journal
	fmt.Println("\n🔄 Determining resume point...")
	recoveryState, err := RecoverFromJournal(ctx, journal, client)
	if err != nil {
		return fmt.Errorf("recover from journal: %w", err)
	}

	// Handle uncertain receipt check
	if recoveryState.NeedReceiptCheck && !config.DryRun {
		fmt.Printf("⚠️  Last transaction status uncertain, checking receipt...\n")
		fmt.Printf("   Tx Hash: %s\n", recoveryState.LastTxHash)

		receipt, err := WaitForReceipt(ctx, client, recoveryState.LastTxHash, config.ReceiptTimeout)
		if err != nil {
			fmt.Printf("   ⚠️  Could not get receipt: %v\n", err)
			fmt.Printf("   Marking as uncertain, will retry on next run\n")
		} else {
			success := receipt.Status == 1
			if err := UpdateJournalWithReceipt(journal, privateKey, recoveryState.NextBatchIndex, recoveryState.LastTxHash, success); err != nil {
				return fmt.Errorf("update journal with receipt: %w", err)
			}

			if success {
				fmt.Printf("   ✅ Transaction confirmed, moving to next batch\n")
				recoveryState.NextBatchIndex++
			} else {
				fmt.Printf("   ❌ Transaction failed on-chain\n")
				return fmt.Errorf("batch %d failed on-chain", recoveryState.NextBatchIndex)
			}
		}
	}

	fmt.Printf("📍 Starting from batch %d\n", recoveryState.NextBatchIndex)

	// Get confirmed batches to skip
	confirmedBatches := journal.GetConfirmedBatches()
	confirmedMap := make(map[int]bool)
	for _, idx := range confirmedBatches {
		confirmedMap[idx] = true
	}

	// Execute each batch
	for _, batch := range manifest.Batches {
		// Skip if already confirmed
		if confirmedMap[batch.Index] {
			fmt.Printf("\n⏭️  Batch %d: Already confirmed, skipping\n", batch.Index)
			continue
		}

		// Skip if before resume point
		if batch.Index < recoveryState.NextBatchIndex {
			fmt.Printf("\n⏭️  Batch %d: Before resume point, skipping\n", batch.Index)
			continue
		}

		fmt.Printf("\n📦 Batch %d: %s\n", batch.Index, batch.Comment)

		// Decode calldata
		callData, err := DecodeCallData(batch.CallData)
		if err != nil {
			return fmt.Errorf("decode calldata for batch %d: %w", batch.Index, err)
		}

		// Log prepared state
		preparedEntry := JournalEntry{
			Timestamp:  time.Now(),
			BatchIndex: batch.Index,
			CallData:   batch.CallData,
			TxHash:     "",
			Status:     StatusPrepared,
			Error:      batch.Comment,
		}
		if err := journal.Append(preparedEntry, privateKey); err != nil {
			return fmt.Errorf("log prepared state: %w", err)
		}

		if config.DryRun {
			fmt.Printf("   🧪 Dry-run: Would broadcast transaction\n")
			fmt.Printf("   📝 Logged as prepared\n")
			continue
		}

		// Broadcast transaction
		fmt.Printf("   📡 Broadcasting transaction...\n")
		broadcastConfig := BroadcastConfig{
			RPC:      config.RPC,
			ChainID:  config.ChainID,
			GasPrice: config.GasPrice,
		}

		txHash, err := BroadcastTransaction(
			ctx,
			client,
			privateKey,
			config.CoordinatorAddr,
			callData,
			batch.GasLimit,
			broadcastConfig,
		)

		if err != nil {
			// Log as uncertain
			uncertainEntry := JournalEntry{
				Timestamp:  time.Now(),
				BatchIndex: batch.Index,
				CallData:   batch.CallData,
				TxHash:     "",
				Status:     StatusUncertain,
				Error:      fmt.Sprintf("Broadcast error: %v", err),
			}
			journal.Append(uncertainEntry, privateKey)
			return fmt.Errorf("broadcast batch %d: %w", batch.Index, err)
		}

		fmt.Printf("   ✅ Broadcast successful: %s\n", txHash)

		// Log broadcast state
		broadcastEntry := JournalEntry{
			Timestamp:  time.Now(),
			BatchIndex: batch.Index,
			CallData:   batch.CallData,
			TxHash:     txHash,
			Status:     StatusBroadcast,
			Error:      batch.Comment,
		}
		if err := journal.Append(broadcastEntry, privateKey); err != nil {
			return fmt.Errorf("log broadcast state for batch %d: %w", batch.Index, err)
		}

		// Wait for receipt
		fmt.Printf("   ⏳ Waiting for receipt (timeout: %v)...\n", config.ReceiptTimeout)
		receipt, err := WaitForReceipt(ctx, client, txHash, config.ReceiptTimeout)

		if err != nil {
			// Log as uncertain
			uncertainEntry := JournalEntry{
				Timestamp:  time.Now(),
				BatchIndex: batch.Index,
				CallData:   batch.CallData,
				TxHash:     txHash,
				Status:     StatusUncertain,
				Error:      fmt.Sprintf("Receipt timeout: %v", err),
			}
			journal.Append(uncertainEntry, privateKey)
			fmt.Printf("   ⚠️  Receipt timeout, marked as uncertain\n")
			fmt.Printf("   Re-run operator to verify status\n")
			return fmt.Errorf("receipt timeout for batch %d: %w", batch.Index, err)
		}

		// Verify receipt status
		if err := VerifyReceiptStatus(receipt); err != nil {
			// Log as failed
			failedEntry := JournalEntry{
				Timestamp:   time.Now(),
				BatchIndex:  batch.Index,
				CallData:    batch.CallData,
				TxHash:      txHash,
				Status:      StatusFailed,
				BlockNumber: receipt.BlockNumber.Uint64(),
				Error:       "Transaction failed on-chain",
			}
			journal.Append(failedEntry, privateKey)
			return fmt.Errorf("batch %d failed: %w", batch.Index, err)
		}

		fmt.Printf("   ✅ Transaction confirmed in block %d\n", receipt.BlockNumber.Uint64())

		// Log as confirmed
		confirmedEntry := JournalEntry{
			Timestamp:   time.Now(),
			BatchIndex:  batch.Index,
			CallData:    batch.CallData,
			TxHash:      txHash,
			Status:      StatusConfirmed,
			BlockNumber: receipt.BlockNumber.Uint64(),
			GasUsed:     receipt.GasUsed,
		}
		if err := journal.Append(confirmedEntry, privateKey); err != nil {
			return fmt.Errorf("log confirmed state: %w", err)
		}
	}

	fmt.Printf("\n✅ All batches processed successfully!\n")
	return nil
}

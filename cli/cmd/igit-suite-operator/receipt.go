package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// QueryReceipt queries the transaction receipt from the blockchain
func QueryReceipt(ctx context.Context, client *ethclient.Client, txHash string) (*types.Receipt, error) {
	hash := common.HexToHash(txHash)
	receipt, err := client.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("query receipt: %w", err)
	}
	return receipt, nil
}

// WaitForReceipt waits for a transaction receipt with timeout and retry
func WaitForReceipt(ctx context.Context, client *ethclient.Client, txHash string, timeout time.Duration) (*types.Receipt, error) {
	deadline := time.Now().Add(timeout)
	retryInterval := 2 * time.Second

	for {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for receipt after %v", timeout)
		}

		// Try to get the receipt
		receipt, err := QueryReceipt(ctx, client, txHash)
		if err == nil {
			return receipt, nil
		}

		// If the error is "not found", retry after interval
		// Other errors are returned immediately
		if err.Error() != "not found" && err.Error() != "transaction not found" {
			// Only retry on "not found" errors
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryInterval):
			// Continue to next iteration
		}
	}
}

// VerifyReceiptStatus checks if the transaction was successful
func VerifyReceiptStatus(receipt *types.Receipt) error {
	if receipt.Status == 0 {
		return fmt.Errorf("transaction failed on-chain")
	}
	return nil
}

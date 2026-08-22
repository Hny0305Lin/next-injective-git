package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// BroadcastConfig holds configuration for transaction broadcast
type BroadcastConfig struct {
	RPC      string
	ChainID  uint64
	GasPrice uint64
}

// BroadcastTransaction builds, signs, and broadcasts a transaction
func BroadcastTransaction(
	ctx context.Context,
	client *ethclient.Client,
	privateKey *ecdsa.PrivateKey,
	to common.Address,
	callData []byte,
	gasLimit uint64,
	config BroadcastConfig,
) (string, error) {
	// Get sender address
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Get nonce
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}

	// Estimate gas if not provided
	if gasLimit == 0 {
		gasLimit, err = EstimateGas(ctx, client, from, to, callData)
		if err != nil {
			return "", fmt.Errorf("estimate gas: %w", err)
		}
		// Add 20% buffer
		gasLimit = gasLimit * 120 / 100
	}

	// Create legacy transaction (type 0)
	tx := types.NewTransaction(
		nonce,
		to,
		big.NewInt(0), // value = 0
		gasLimit,
		big.NewInt(int64(config.GasPrice)),
		callData,
	)

	// Sign transaction
	chainID := big.NewInt(int64(config.ChainID))
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}

	// Broadcast transaction
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return "", fmt.Errorf("broadcast transaction: %w", err)
	}

	return signedTx.Hash().Hex(), nil
}

// EstimateGas estimates gas limit for a transaction
func EstimateGas(
	ctx context.Context,
	client *ethclient.Client,
	from common.Address,
	to common.Address,
	callData []byte,
) (uint64, error) {
	// Estimate gas using ethereum.CallMsg
	msg := ethereum.CallMsg{
		From: from,
		To:   &to,
		Data: callData,
	}
	gasLimit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("estimate gas: %w", err)
	}
	return gasLimit, nil
}

// DecodeCallData decodes hex-encoded calldata
func DecodeCallData(hexData string) ([]byte, error) {
	// Remove 0x prefix if present
	hexData = strings.TrimPrefix(hexData, "0x")

	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, fmt.Errorf("decode calldata: %w", err)
	}
	return data, nil
}

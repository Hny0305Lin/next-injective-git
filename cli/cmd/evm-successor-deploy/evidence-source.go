package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitedeploy"
)

// evidenceTransactionSource serves historical deployment transactions from an
// existing evidence file plus live RPC lookups (input/nonce/from via
// eth_getTransactionByHash, timestamp via eth_getBlockByNumber). It replaces
// the Blockscout source when the explorer is unreachable, using only data the
// chain itself still serves.
type evidenceTransactionSource struct {
	rpc    *chain.EVMRPC
	hashes []string
}

type evidenceFile struct {
	Contracts              []evidenceContract `json:"contracts"`
	ConfigurationTransmits []evidenceConfig   `json:"configuration_transactions"`
}

type evidenceContract struct {
	ContractName     string `json:"contract_name"`
	TransactionHash  string `json:"transaction_hash"`
	TransactionOrder int    `json:"transaction_order"`
}

type evidenceConfig struct {
	Purpose          string `json:"purpose"`
	TransactionHash  string `json:"transaction_hash"`
	TransactionOrder int    `json:"transaction_order"`
}

func newEvidenceTransactionSource(path string, rpc *chain.EVMRPC) (*evidenceTransactionSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Contracts        []evidenceContract `json:"contracts"`
		ConfigurationTxs []evidenceConfig   `json:"configuration_transactions"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse deployment evidence: %w", err)
	}
	type entry struct {
		order int
		hash  string
	}
	var entries []entry
	for _, c := range file.Contracts {
		entries = append(entries, entry{c.TransactionOrder, strings.TrimSpace(c.TransactionHash)})
	}
	for _, c := range file.ConfigurationTxs {
		entries = append(entries, entry{c.TransactionOrder, strings.TrimSpace(c.TransactionHash)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].order < entries[j].order })
	source := &evidenceTransactionSource{rpc: rpc}
	for _, e := range entries {
		if e.hash == "" {
			return nil, fmt.Errorf("evidence order %d has no transaction hash", e.order)
		}
		source.hashes = append(source.hashes, e.hash)
	}
	return source, nil
}

func (s *evidenceTransactionSource) Description() string {
	return "deployment evidence + live RPC"
}

type rawTx struct {
	Hash  string `json:"hash"`
	From  string `json:"from"`
	To    string `json:"to"`
	Input string `json:"input"`
	Nonce string `json:"nonce"`
}

type rawBlock struct {
	Timestamp string `json:"timestamp"`
}

func (s *evidenceTransactionSource) Transaction(ctx context.Context, hash string) (*suitedeploy.HistoricalTransaction, error) {
	var tx rawTx
	if err := s.rpc.Call(ctx, "eth_getTransactionByHash", []any{hash}, &tx); err != nil {
		return nil, fmt.Errorf("eth_getTransactionByHash %s: %w", hash, err)
	}
	if strings.TrimSpace(tx.Hash) == "" {
		return nil, fmt.Errorf("transaction %s not found on chain", hash)
	}
	var receipt struct {
		BlockNumber     string `json:"blockNumber"`
		ContractAddress string `json:"contractAddress"`
		GasUsed         string `json:"gasUsed"`
		Status          string `json:"status"`
	}
	if err := s.rpc.Call(ctx, "eth_getTransactionReceipt", []any{hash}, &receipt); err != nil {
		return nil, fmt.Errorf("eth_getTransactionReceipt %s: %w", hash, err)
	}
	var block rawBlock
	if err := s.rpc.Call(ctx, "eth_getBlockByNumber", []any{receipt.BlockNumber, false}, &block); err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber %s: %w", receipt.BlockNumber, err)
	}
	nonce, err := strconv.ParseUint(strings.TrimPrefix(tx.Nonce, "0x"), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("decode nonce %q: %w", tx.Nonce, err)
	}
	blockNumber, err := strconv.ParseUint(strings.TrimPrefix(receipt.BlockNumber, "0x"), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("decode block number %q: %w", receipt.BlockNumber, err)
	}
	timestamp, err := strconv.ParseInt(strings.TrimPrefix(block.Timestamp, "0x"), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("decode block timestamp %q: %w", block.Timestamp, err)
	}
	gasUsed, err := strconv.ParseUint(strings.TrimPrefix(receipt.GasUsed, "0x"), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("decode gas used %q: %w", receipt.GasUsed, err)
	}
	return &suitedeploy.HistoricalTransaction{
		Hash: strings.ToLower(tx.Hash), From: strings.ToLower(tx.From), To: strings.ToLower(tx.To),
		Input: strings.ToLower(tx.Input), ContractAddress: strings.ToLower(receipt.ContractAddress),
		BlockNumber: blockNumber, GasUsed: strconv.FormatUint(gasUsed, 10), Nonce: nonce,
		Timestamp:  time.Unix(timestamp, 0).UTC().Format(time.RFC3339Nano),
		Successful: strings.EqualFold(receipt.Status, "0x1"),
	}, nil
}

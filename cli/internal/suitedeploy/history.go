package suitedeploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const recoveryInputSchema = "igit.evm-suite.deployment-recovery-input.v1"

type RecoveryTransactionReference struct {
	Purpose         string `json:"purpose"`
	TransactionHash string `json:"transaction_hash"`
}

type RecoveryInput struct {
	Schema       string                         `json:"schema"`
	Transactions []RecoveryTransactionReference `json:"transactions"`
}

func LoadRecoveryInput(inputPath string) (*RecoveryInput, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read deployment recovery input: %w", err)
	}
	var input RecoveryInput
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("decode deployment recovery input: %w", err)
	}
	if input.Schema != recoveryInputSchema {
		return nil, fmt.Errorf("deployment recovery input schema %q, want %q", input.Schema, recoveryInputSchema)
	}
	if len(input.Transactions) == 0 {
		return nil, fmt.Errorf("deployment recovery input has no transactions")
	}
	for index, transaction := range input.Transactions {
		if strings.TrimSpace(transaction.Purpose) == "" {
			return nil, fmt.Errorf("deployment recovery transaction %d has no purpose", index)
		}
		if !validHash(transaction.TransactionHash) {
			return nil, fmt.Errorf("deployment recovery transaction %d has invalid hash %q", index, transaction.TransactionHash)
		}
		input.Transactions[index].TransactionHash = normalizeHash(transaction.TransactionHash)
	}
	return &input, nil
}

type HistoricalTransaction struct {
	Hash            string
	From            string
	To              string
	Input           string
	ContractAddress string
	BlockNumber     uint64
	GasUsed         string
	Nonce           uint64
	Timestamp       string
	Successful      bool
}

type HistoricalTransactionSource interface {
	Description() string
	Transaction(context.Context, string) (*HistoricalTransaction, error)
}

type BlockscoutTransactionSource struct {
	baseURL string
	client  *http.Client
}

func NewBlockscoutTransactionSource(rawURL string) (*BlockscoutTransactionSource, error) {
	value := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse Blockscout API URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("Blockscout API URL must be an absolute credential-free https URL without query or fragment")
	}
	return &BlockscoutTransactionSource{
		baseURL: value,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (source *BlockscoutTransactionSource) Description() string {
	if source == nil {
		return ""
	}
	return source.baseURL
}

type blockscoutAddress struct {
	Hash string `json:"hash"`
}

type blockscoutTransaction struct {
	Hash            string             `json:"hash"`
	RawInput        string             `json:"raw_input"`
	From            blockscoutAddress  `json:"from"`
	To              *blockscoutAddress `json:"to"`
	CreatedContract *blockscoutAddress `json:"created_contract"`
	GasUsed         string             `json:"gas_used"`
	Status          string             `json:"status"`
	Timestamp       string             `json:"timestamp"`
	Nonce           uint64             `json:"nonce"`
	BlockNumber     uint64             `json:"block_number"`
}

func (source *BlockscoutTransactionSource) Transaction(ctx context.Context, hash string) (*HistoricalTransaction, error) {
	if source == nil || source.client == nil {
		return nil, fmt.Errorf("Blockscout transaction source is unavailable")
	}
	if !validHash(hash) {
		return nil, fmt.Errorf("invalid historical transaction hash %q", hash)
	}
	requestURL, err := url.Parse(source.baseURL)
	if err != nil {
		return nil, err
	}
	requestURL.Path = path.Join(requestURL.Path, "api", "v2", "transactions", normalizeHash(hash))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Blockscout transaction request: %w", err)
	}
	response, err := source.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query Blockscout transaction %s: %w", hash, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("query Blockscout transaction %s: HTTP %d: %s", hash, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded blockscoutTransaction
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode Blockscout transaction %s: %w", hash, err)
	}
	var to, contractAddress string
	if decoded.To != nil {
		to = decoded.To.Hash
	}
	if decoded.CreatedContract != nil {
		contractAddress = decoded.CreatedContract.Hash
	}
	return &HistoricalTransaction{
		Hash: normalizeHash(decoded.Hash), From: decoded.From.Hash, To: to,
		Input: decoded.RawInput, ContractAddress: contractAddress,
		BlockNumber: decoded.BlockNumber, GasUsed: decoded.GasUsed,
		Nonce: decoded.Nonce, Timestamp: decoded.Timestamp,
		Successful: strings.EqualFold(strings.TrimSpace(decoded.Status), "ok"),
	}, nil
}

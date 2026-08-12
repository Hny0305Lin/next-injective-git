package migration

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	DefaultImportReceiptTimeout = 10 * time.Minute
	DefaultImportMaxGasLimit    = uint64(15_000_000)
	importGasHeadroom           = uint64(10_000)
)

// ImportExecutionRPC is the JSON-RPC surface needed by the resumable import
// runner. EVMRPC implements it; tests use an in-memory deterministic node.
type ImportExecutionRPC interface {
	ChainID(context.Context) (uint64, error)
	TransactionCount(context.Context, string, string) (uint64, error)
	EstimateGas(context.Context, chain.EVMCall) (uint64, error)
	GasPrice(context.Context) (string, error)
	SendRawTransaction(context.Context, string) (string, error)
	TransactionReceipt(context.Context, string) (*chain.EVMReceipt, error)
	WaitReceipt(context.Context, string) (*chain.EVMReceipt, error)
	BlockByNumber(context.Context, string) (*chain.EVMBlock, error)
}

type ImportExecutionOptions struct {
	ReceiptTimeout time.Duration
	MaxGasLimit    uint64
}

type ImportExecutionResult struct {
	TransactionCount         int
	PreparedThisRun          int
	BroadcastRecordedThisRun int
	MinedThisRun             int
	ResumedPrepared          int
	RevalidatedMined         int
	FinalizeTransactionHash  string
}

// ExecuteImport applies a canonical transaction manifest sequentially. Every
// signed transaction is durably journaled before broadcast, and every mined
// success is event-checked before the next sequence is prepared.
func ExecuteImport(
	ctx context.Context,
	plan *Plan,
	manifest *TransactionManifest,
	journalDirectory string,
	rpc ImportExecutionRPC,
	signer chain.EVMSigner,
	options ImportExecutionOptions,
) (*ImportExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rpc == nil {
		return nil, errors.New("import execution RPC is nil")
	}
	if signer == nil {
		return nil, errors.New("import execution signer is nil")
	}
	if err := VerifyTransactionManifest(plan, manifest); err != nil {
		return nil, fmt.Errorf("verify import execution manifest: %w", err)
	}
	chainID, err := rpc.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read import execution chain ID: %w", err)
	}
	if chainID != plan.Target.ChainID {
		return nil, fmt.Errorf("import execution chain ID %d does not match plan target %d", chainID, plan.Target.ChainID)
	}
	owner, err := signer.OwnerAddress()
	if err != nil {
		return nil, fmt.Errorf("resolve import execution signer: %w", err)
	}
	from, err := chain.NormalizeEVMAddress(owner)
	if err != nil {
		return nil, fmt.Errorf("resolve import execution signer address: %w", err)
	}
	if from == zeroEVMAddress {
		return nil, errors.New("resolve import execution signer address: zero address is not allowed")
	}
	journal, err := OpenReceiptJournal(journalDirectory, plan, manifest, from)
	if err != nil {
		return nil, err
	}
	if options.ReceiptTimeout <= 0 {
		options.ReceiptTimeout = DefaultImportReceiptTimeout
	}
	if options.MaxGasLimit == 0 {
		options.MaxGasLimit = DefaultImportMaxGasLimit
	}
	result := &ImportExecutionResult{TransactionCount: len(manifest.Transactions)}

	for order, manifestTransaction := range manifest.Transactions {
		if mined, ok := journal.Mined(order); ok {
			if err := revalidateJournalReceipt(ctx, plan, manifest, order, rpc, mined); err != nil {
				return nil, err
			}
			result.RevalidatedMined++
			if order == len(manifest.Transactions)-1 {
				result.FinalizeTransactionHash = mined.Receipt.TransactionHash
			}
			continue
		}
		if reverted, ok := journal.Reverted(order); ok {
			if err := revalidateRevertedJournalReceipt(ctx, order, rpc, reverted); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf(
				"import transaction %d has a revalidated reverted receipt %s; inspect the journal and create a new plan or deployment",
				order,
				reverted.Receipt.TransactionHash,
			)
		}

		prepared, ok := journal.Prepared(order)
		if !ok {
			signed, err := prepareImportTransaction(
				ctx, chainID, from, order, manifestTransaction, rpc, signer, options.MaxGasLimit,
			)
			if err != nil {
				return nil, fmt.Errorf("prepare import transaction %d (%s): %w", order, manifestTransaction.Phase, err)
			}
			if err := journal.RecordPrepared(order, signed); err != nil {
				return nil, err
			}
			prepared, _ = journal.Prepared(order)
			result.PreparedThisRun++
		} else {
			result.ResumedPrepared++
		}
		if prepared.Transaction.GasLimit == 0 || prepared.Transaction.GasLimit > options.MaxGasLimit {
			return nil, fmt.Errorf(
				"prepared import transaction %d gas limit %d exceeds current maximum %d",
				order,
				prepared.Transaction.GasLimit,
				options.MaxGasLimit,
			)
		}

		receipt, err := rpc.TransactionReceipt(ctx, prepared.Transaction.TransactionHash)
		if err != nil {
			return nil, fmt.Errorf("query import transaction %d receipt: %w", order, err)
		}
		_, hasBroadcast := journal.Broadcast(order)
		if receipt == nil {
			returnedHash, sendErr := rpc.SendRawTransaction(ctx, prepared.Transaction.RawTransaction)
			if sendErr != nil && !isAlreadyKnownTransaction(sendErr) {
				return nil, fmt.Errorf("broadcast import transaction %d: %w", order, sendErr)
			}
			if sendErr == nil {
				returnedHash = strings.ToLower(strings.TrimSpace(returnedHash))
				if err := validateCanonicalHash32("broadcast transaction hash", returnedHash); err != nil {
					return nil, fmt.Errorf("broadcast import transaction %d: %w", order, err)
				}
				if returnedHash != prepared.Transaction.TransactionHash {
					return nil, fmt.Errorf("broadcast import transaction %d returned hash %s, want %s", order, returnedHash, prepared.Transaction.TransactionHash)
				}
			}
			if !hasBroadcast {
				if err := journal.RecordBroadcast(order, prepared.Transaction.TransactionHash); err != nil {
					return nil, err
				}
				result.BroadcastRecordedThisRun++
				hasBroadcast = true
			}
			waitCtx, cancel := context.WithTimeout(ctx, options.ReceiptTimeout)
			receipt, err = rpc.WaitReceipt(waitCtx, prepared.Transaction.TransactionHash)
			cancel()
			if err != nil {
				if receipt != nil && isRevertedReceipt(receipt) {
					return nil, recordRevertedImportReceipt(
						ctx, plan, manifest, order, prepared.Transaction.TransactionHash,
						journal, rpc, receipt, true, err,
					)
				}
				return nil, fmt.Errorf("wait for import transaction %d receipt: %w", order, err)
			}
		} else if !hasBroadcast {
			// A crash can occur after the node accepted the raw transaction but
			// before the broadcast transition was linked into the journal.
			if err := journal.RecordBroadcast(order, prepared.Transaction.TransactionHash); err != nil {
				return nil, err
			}
			result.BroadcastRecordedThisRun++
			hasBroadcast = true
		}
		if isRevertedReceipt(receipt) {
			return nil, recordRevertedImportReceipt(
				ctx, plan, manifest, order, prepared.Transaction.TransactionHash,
				journal, rpc, receipt, hasBroadcast, nil,
			)
		}

		canonical, err := canonicalJournalReceipt(receipt)
		if err != nil {
			return nil, fmt.Errorf("validate import transaction %d receipt: %w", order, err)
		}
		if err := validateManifestReceipt(plan, manifest, order, prepared.Transaction.TransactionHash, canonical); err != nil {
			return nil, err
		}
		if err := validateReceiptCanonicalBlock(ctx, rpc, order, canonical); err != nil {
			return nil, err
		}
		if err := journal.RecordMined(order, canonical); err != nil {
			return nil, err
		}
		result.MinedThisRun++
		if order == len(manifest.Transactions)-1 {
			result.FinalizeTransactionHash = canonical.TransactionHash
		}
	}
	return result, nil
}

func prepareImportTransaction(
	ctx context.Context,
	chainID uint64,
	from string,
	order int,
	manifestTransaction ManifestTransaction,
	rpc ImportExecutionRPC,
	signer chain.EVMSigner,
	maxGasLimit uint64,
) (SignedImportTransaction, error) {
	nonce, err := rpc.TransactionCount(ctx, from, "pending")
	if err != nil {
		return SignedImportTransaction{}, fmt.Errorf("read pending nonce: %w", err)
	}
	if nonce == ^uint64(0) {
		return SignedImportTransaction{}, errors.New("pending account nonce is exhausted")
	}
	call := chain.EVMCall{From: from, To: manifestTransaction.To, Data: manifestTransaction.Data, Value: manifestTransaction.Value}
	estimated, err := rpc.EstimateGas(ctx, call)
	if err != nil {
		return SignedImportTransaction{}, fmt.Errorf("estimate gas: %w", err)
	}
	gasLimit, err := adjustedImportGasLimit(estimated, maxGasLimit)
	if err != nil {
		return SignedImportTransaction{}, err
	}
	gasPrice, err := rpc.GasPrice(ctx)
	if err != nil {
		return SignedImportTransaction{}, fmt.Errorf("read gas price: %w", err)
	}
	gasPrice, err = normalizeRunnerQuantity("gas price", gasPrice)
	if err != nil {
		return SignedImportTransaction{}, err
	}
	value, err := normalizeRunnerQuantity("transaction value", manifestTransaction.Value)
	if err != nil {
		return SignedImportTransaction{}, err
	}
	raw, err := signer.SignTransaction(ctx, chain.EVMTransaction{
		ChainID: chainID, Nonce: nonce, To: manifestTransaction.To, Data: manifestTransaction.Data,
		GasLimit: gasLimit, GasPrice: gasPrice, Value: value,
	})
	if err != nil {
		return SignedImportTransaction{}, fmt.Errorf("sign transaction: %w", err)
	}
	raw, transactionHash, err := canonicalSignedTransaction(raw)
	if err != nil {
		return SignedImportTransaction{}, err
	}
	return SignedImportTransaction{
		From: from, ChainID: chainID, Nonce: nonce, GasLimit: gasLimit,
		GasPrice: gasPrice, Value: value, RawTransaction: raw,
		TransactionHash: transactionHash,
	}, nil
}

func adjustedImportGasLimit(estimated, maximum uint64) (uint64, error) {
	adjusted, err := chain.AdjustEVMGasLimit(estimated, importGasHeadroom)
	if err != nil {
		return 0, err
	}
	if adjusted > maximum {
		return 0, fmt.Errorf("adjusted gas limit %d exceeds import maximum %d", adjusted, maximum)
	}
	return adjusted, nil
}

func canonicalSignedTransaction(raw string) (string, string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(value, "0x") || len(value) <= 2 || len(value[2:])%2 != 0 {
		return "", "", errors.New("signer returned a malformed raw transaction")
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil {
		return "", "", fmt.Errorf("decode signed raw transaction: %w", err)
	}
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(decoded); err != nil {
		return "", "", fmt.Errorf("decode signed raw transaction: %w", err)
	}
	return value, transaction.Hash().Hex(), nil
}

func normalizeRunnerQuantity(label, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if value == "" {
		return "", fmt.Errorf("%s is empty", label)
	}
	parsed, ok := new(big.Int).SetString(value, 16)
	if !ok || parsed.Sign() < 0 {
		return "", fmt.Errorf("%s is not a hexadecimal quantity", label)
	}
	return "0x" + parsed.Text(16), nil
}

func revalidateJournalReceipt(
	ctx context.Context,
	plan *Plan,
	manifest *TransactionManifest,
	order int,
	rpc ImportExecutionRPC,
	record *MinedJournalRecord,
) error {
	receipt, err := rpc.TransactionReceipt(ctx, record.Receipt.TransactionHash)
	if err != nil {
		return fmt.Errorf("revalidate mined import transaction %d: %w", order, err)
	}
	if receipt == nil {
		return fmt.Errorf("revalidate mined import transaction %d: RPC no longer returns its receipt", order)
	}
	canonical, err := canonicalJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("revalidate mined import transaction %d: %w", order, err)
	}
	if !reflect.DeepEqual(*canonical, record.Receipt) {
		return fmt.Errorf("revalidate mined import transaction %d: receipt evidence changed", order)
	}
	if err := validateReceiptCanonicalBlock(ctx, rpc, order, canonical); err != nil {
		return fmt.Errorf("revalidate mined import transaction %d: %w", order, err)
	}
	if err := validateManifestReceipt(plan, manifest, order, record.Receipt.TransactionHash, canonical); err != nil {
		return fmt.Errorf("revalidate mined import transaction %d: %w", order, err)
	}
	return nil
}

func revalidateRevertedJournalReceipt(
	ctx context.Context,
	order int,
	rpc ImportExecutionRPC,
	record *RevertedJournalRecord,
) error {
	receipt, err := rpc.TransactionReceipt(ctx, record.Receipt.TransactionHash)
	if err != nil {
		return fmt.Errorf("revalidate reverted import transaction %d: %w", order, err)
	}
	if receipt == nil {
		return fmt.Errorf("revalidate reverted import transaction %d: RPC no longer returns its receipt", order)
	}
	canonical, err := canonicalRevertedJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("revalidate reverted import transaction %d: %w", order, err)
	}
	if !reflect.DeepEqual(*canonical, record.Receipt) {
		return fmt.Errorf("revalidate reverted import transaction %d: receipt evidence changed", order)
	}
	if err := validateReceiptCanonicalBlock(ctx, rpc, order, canonical); err != nil {
		return fmt.Errorf("revalidate reverted import transaction %d: %w", order, err)
	}
	return nil
}

func validateReceiptCanonicalBlock(
	ctx context.Context,
	rpc ImportExecutionRPC,
	order int,
	receipt *chain.EVMReceipt,
) error {
	_, err := receiptCanonicalBlock(ctx, rpc, order, receipt)
	return err
}

func receiptCanonicalBlock(
	ctx context.Context,
	rpc ImportExecutionRPC,
	order int,
	receipt *chain.EVMReceipt,
) (*chain.EVMBlock, error) {
	block, err := rpc.BlockByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, fmt.Errorf("verify import transaction %d receipt block: %w", order, err)
	}
	if err := validateCanonicalJournalBlock(receipt, block); err != nil {
		return nil, fmt.Errorf("verify import transaction %d receipt block: %w", order, err)
	}
	return block, nil
}

func recordRevertedImportReceipt(
	ctx context.Context,
	plan *Plan,
	manifest *TransactionManifest,
	order int,
	transactionHash string,
	journal *ReceiptJournal,
	rpc ImportExecutionRPC,
	receipt *chain.EVMReceipt,
	hasBroadcast bool,
	waitErr error,
) error {
	canonical, err := canonicalRevertedJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("validate reverted import transaction %d receipt: %w", order, err)
	}
	if canonical.TransactionHash != transactionHash {
		return fmt.Errorf(
			"validate reverted import transaction %d receipt: hash %s does not match prepared transaction %s",
			order, canonical.TransactionHash, transactionHash,
		)
	}
	if !hasBroadcast {
		if err := journal.RecordBroadcast(order, transactionHash); err != nil {
			return fmt.Errorf("record reverted import transaction %d broadcast: %w", order, err)
		}
	}
	block, err := receiptCanonicalBlock(ctx, rpc, order, canonical)
	if err != nil {
		return err
	}
	if err := journal.RecordRevertedAt(order, canonical, block); err != nil {
		return err
	}
	if waitErr != nil {
		return fmt.Errorf("wait for import transaction %d receipt: %w; reverted receipt recorded", order, waitErr)
	}
	return fmt.Errorf("import transaction %d receipt reverted; reverted receipt recorded", order)
}

func isRevertedReceipt(receipt *chain.EVMReceipt) bool {
	if receipt == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(receipt.Status))
	return status == "0x0" || status == "0"
}

func isAlreadyKnownTransaction(err error) bool {
	var rpcError *chain.RPCError
	if !errors.As(err, &rpcError) {
		return false
	}
	message := strings.ToLower(rpcError.Message)
	if strings.Contains(message, "unknown transaction") ||
		strings.Contains(message, "not a known transaction") ||
		strings.Contains(message, "not known") {
		return false
	}
	return strings.Contains(message, "already known") || containsBoundedPhrase(message, "known transaction")
}

func containsBoundedPhrase(message, phrase string) bool {
	for offset := 0; offset <= len(message)-len(phrase); {
		index := strings.Index(message[offset:], phrase)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !isASCIIWordByte(message[index-1])
		after := index + len(phrase)
		afterOK := after == len(message) || !isASCIIWordByte(message[after])
		if beforeOK && afterOK {
			return true
		}
		offset = index + 1
	}
	return false
}

func isASCIIWordByte(value byte) bool {
	return (value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9') ||
		value == '_'
}

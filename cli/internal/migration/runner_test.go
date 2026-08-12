package migration

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func TestExecuteImportJournalsEveryTransitionAndResumesCompletedRun(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := &runnerTestSigner{key: key}
	rpc := newRunnerTestRPC(t, plan, manifest)
	rpc.nextNonce = 7
	directory := filepath.Join(t.TempDir(), "journal")

	result, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{
		ReceiptTimeout: time.Second,
		MaxGasLimit:    1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TransactionCount != len(manifest.Transactions) ||
		result.PreparedThisRun != len(manifest.Transactions) ||
		result.BroadcastRecordedThisRun != len(manifest.Transactions) ||
		result.MinedThisRun != len(manifest.Transactions) ||
		result.RevalidatedMined != 0 || result.FinalizeTransactionHash == "" {
		t.Fatalf("first execution result = %#v", result)
	}
	if signer.signCalls != len(manifest.Transactions) || rpc.sendCalls != len(manifest.Transactions) {
		t.Fatalf("sign/send calls = %d/%d", signer.signCalls, rpc.sendCalls)
	}
	if len(signer.transactions) == 0 || signer.transactions[0].Nonce != 7 || signer.transactions[0].GasLimit != 150_000 {
		t.Fatalf("first signed transaction = %#v", signer.transactions)
	}

	second, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{
		ReceiptTimeout: time.Second,
		MaxGasLimit:    1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevalidatedMined != len(manifest.Transactions) || second.PreparedThisRun != 0 || second.BroadcastRecordedThisRun != 0 || second.MinedThisRun != 0 {
		t.Fatalf("resume result = %#v", second)
	}
	if signer.signCalls != len(manifest.Transactions) || rpc.sendCalls != len(manifest.Transactions) {
		t.Fatalf("completed resume signed or sent again: %d/%d", signer.signCalls, rpc.sendCalls)
	}
}

func TestExecuteImportResumesPreparedTransactionWithoutResigning(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := &runnerTestSigner{key: key}
	signerAddress := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signerAddress)
	if err != nil {
		t.Fatal(err)
	}
	prepared := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := journal.RecordPrepared(0, prepared); err != nil {
		t.Fatal(err)
	}
	rpc := newRunnerTestRPC(t, plan, manifest)

	result, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResumedPrepared != 1 || result.PreparedThisRun != len(manifest.Transactions)-1 {
		t.Fatalf("resume result = %#v", result)
	}
	if signer.signCalls != len(manifest.Transactions)-1 {
		t.Fatalf("signer calls = %d, want %d", signer.signCalls, len(manifest.Transactions)-1)
	}
}

func TestExecuteImportRecoversReceiptAcceptedBeforeBroadcastJournalWrite(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := &runnerTestSigner{key: key}
	signerAddress := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signerAddress)
	if err != nil {
		t.Fatal(err)
	}
	prepared := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := journal.RecordPrepared(0, prepared); err != nil {
		t.Fatal(err)
	}
	rpc := newRunnerTestRPC(t, plan, manifest)
	rpc.receipts[prepared.TransactionHash] = journalReceipt(t, plan, manifest, 0, prepared.TransactionHash)
	rpc.nextNonce = 1

	result, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResumedPrepared != 1 || result.BroadcastRecordedThisRun != len(manifest.Transactions) || rpc.sendCalls != len(manifest.Transactions)-1 {
		t.Fatalf("receipt recovery result=%#v sends=%d", result, rpc.sendCalls)
	}
}

func TestExecuteImportTreatsAlreadyKnownAsIdempotentRawReplay(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := &runnerTestSigner{key: key}
	signerAddress := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	directory := filepath.Join(t.TempDir(), "journal")
	journal, err := OpenReceiptJournal(directory, plan, manifest, signerAddress)
	if err != nil {
		t.Fatal(err)
	}
	prepared := signedJournalTransaction(t, key, plan, manifest, 0)
	if err := journal.RecordPrepared(0, prepared); err != nil {
		t.Fatal(err)
	}
	rpc := newRunnerTestRPC(t, plan, manifest)
	rpc.alreadyKnownOnce = true

	result, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResumedPrepared != 1 || result.FinalizeTransactionHash == "" {
		t.Fatalf("already-known resume result = %#v", result)
	}
}

func TestExecuteImportStopsOnRevertedOrInvalidReceiptWithBroadcastDurable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runnerTestRPC)
		want   string
	}{
		{name: "reverted", mutate: func(rpc *runnerTestRPC) { rpc.revertOrder = 0 }, want: "reverted"},
		{name: "missing event", mutate: func(rpc *runnerTestRPC) { rpc.missingEventOrder = 0 }, want: "does not contain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := verificationPlan(t)
			manifest, err := BuildTransactionManifest(plan)
			if err != nil {
				t.Fatal(err)
			}
			key, err := ethcrypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			signer := &runnerTestSigner{key: key}
			rpc := newRunnerTestRPC(t, plan, manifest)
			tc.mutate(rpc)
			directory := filepath.Join(t.TempDir(), "journal")
			_, err = ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			journal, openErr := OpenReceiptJournal(directory, plan, manifest, strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex()))
			if openErr != nil {
				t.Fatal(openErr)
			}
			if _, ok := journal.Broadcast(0); !ok {
				t.Fatal("failed transaction did not retain its broadcast hash")
			}
			if _, ok := journal.Mined(0); ok {
				t.Fatal("failed transaction was recorded as a mined success")
			}
			if tc.name == "reverted" {
				reverted, ok := journal.Reverted(0)
				if !ok || reverted.Receipt.Status != "0x0" {
					t.Fatalf("reverted transaction evidence = %#v, ok=%v", reverted, ok)
				}
				beforeResumeSends := rpc.sendCalls
				_, resumeErr := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
				if resumeErr == nil || !strings.Contains(resumeErr.Error(), "revalidated reverted") {
					t.Fatalf("reverted resume error = %v", resumeErr)
				}
				if rpc.sendCalls != beforeResumeSends {
					t.Fatalf("reverted resume rebroadcast transaction: sends before=%d after=%d", beforeResumeSends, rpc.sendCalls)
				}
			} else if _, ok := journal.Reverted(0); ok {
				t.Fatal("successful-status transaction unexpectedly has reverted evidence")
			}
		})
	}
}

func TestExecuteImportRevalidatesRevertedReceiptEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runnerTestRPC, string)
		want   string
	}{
		{
			name: "receipt removed",
			mutate: func(rpc *runnerTestRPC, hash string) {
				delete(rpc.receipts, hash)
			},
			want: "RPC no longer returns its receipt",
		},
		{
			name: "receipt changed",
			mutate: func(rpc *runnerTestRPC, hash string) {
				receipt := rpc.receipts[hash]
				receipt.BlockHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
				for index := range receipt.Logs {
					receipt.Logs[index].BlockHash = receipt.BlockHash
				}
			},
			want: "receipt evidence changed",
		},
		{
			name: "receipt changed from reverted to successful",
			mutate: func(rpc *runnerTestRPC, hash string) {
				receipt := rpc.receipts[hash]
				receipt.Status = "0x1"
			},
			want: "not 0x0",
		},
		{
			name: "canonical block reorged",
			mutate: func(rpc *runnerTestRPC, _ string) {
				rpc.canonicalBlockHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
			},
			want: "canonical hash",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := verificationPlan(t)
			manifest, err := BuildTransactionManifest(plan)
			if err != nil {
				t.Fatal(err)
			}
			key, err := ethcrypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			signer := &runnerTestSigner{key: key}
			rpc := newRunnerTestRPC(t, plan, manifest)
			rpc.revertOrder = 0
			directory := filepath.Join(t.TempDir(), "journal")
			_, err = ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
			if err == nil || !strings.Contains(err.Error(), "reverted receipt recorded") {
				t.Fatalf("initial reverted error = %v", err)
			}
			prepared, ok := func() (SignedImportTransaction, bool) {
				journal, openErr := OpenReceiptJournal(directory, plan, manifest, strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex()))
				if openErr != nil {
					t.Fatal(openErr)
				}
				reverted, revertedOK := journal.Reverted(0)
				if !revertedOK {
					t.Fatal("missing reverted evidence")
				}
				return SignedImportTransaction{TransactionHash: reverted.Receipt.TransactionHash}, true
			}()
			if !ok {
				t.Fatal("missing prepared hash")
			}
			tc.mutate(rpc, prepared.TransactionHash)
			beforeSignCalls := signer.signCalls
			beforeSendCalls := rpc.sendCalls
			_, resumeErr := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
			if resumeErr == nil || !strings.Contains(resumeErr.Error(), tc.want) {
				t.Fatalf("resume error = %v, want %q", resumeErr, tc.want)
			}
			if signer.signCalls != beforeSignCalls || rpc.sendCalls != beforeSendCalls {
				t.Fatalf(
					"failed reverted revalidation signed or sent: sign %d->%d, send %d->%d",
					beforeSignCalls, signer.signCalls, beforeSendCalls, rpc.sendCalls,
				)
			}
			reopened, openErr := OpenReceiptJournal(
				directory,
				plan,
				manifest,
				strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex()),
			)
			if openErr != nil {
				t.Fatal(openErr)
			}
			recorded, recordedOK := reopened.Reverted(0)
			if !recordedOK || recorded.Receipt.Status != "0x0" || recorded.Receipt.TransactionHash != prepared.TransactionHash {
				t.Fatalf("reverted journal changed after failed revalidation: %#v, ok=%v", recorded, recordedOK)
			}
		})
	}
}

func TestIsAlreadyKnownTransactionUsesWordBoundaries(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{message: "already known", want: true},
		{message: "known transaction", want: true},
		{message: "unknown transaction", want: false},
		{message: "not a known transaction", want: false},
		{message: "transaction not known", want: false},
	} {
		err := &chain.RPCError{Message: tc.message}
		if got := isAlreadyKnownTransaction(err); got != tc.want {
			t.Fatalf("message %q: got %v, want %v", tc.message, got, tc.want)
		}
	}
}

func TestExecuteImportRejectsIdentityGasAndReorgMismatches(t *testing.T) {
	plan := verificationPlan(t)
	manifest, err := BuildTransactionManifest(plan)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := &runnerTestSigner{key: key}

	t.Run("chain", func(t *testing.T) {
		rpc := newRunnerTestRPC(t, plan, manifest)
		rpc.chainID++
		directory := filepath.Join(t.TempDir(), "not-created")
		_, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{})
		if err == nil || !strings.Contains(err.Error(), "does not match plan") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Lstat(directory); !os.IsNotExist(statErr) {
			t.Fatalf("chain mismatch created a journal: %v", statErr)
		}
	})

	t.Run("gas cap", func(t *testing.T) {
		rpc := newRunnerTestRPC(t, plan, manifest)
		rpc.estimatedGas = 1_000_000
		_, err := ExecuteImport(context.Background(), plan, manifest, filepath.Join(t.TempDir(), "journal"), rpc, signer, ImportExecutionOptions{MaxGasLimit: 100_000})
		if err == nil || !strings.Contains(err.Error(), "exceeds import maximum") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("resumed prepared gas cap", func(t *testing.T) {
		key, err := ethcrypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		resumeSigner := &runnerTestSigner{key: key}
		signerAddress := strings.ToLower(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
		directory := filepath.Join(t.TempDir(), "journal")
		journal, err := OpenReceiptJournal(directory, plan, manifest, signerAddress)
		if err != nil {
			t.Fatal(err)
		}
		prepared := signedJournalTransaction(t, key, plan, manifest, 0)
		if err := journal.RecordPrepared(0, prepared); err != nil {
			t.Fatal(err)
		}
		rpc := newRunnerTestRPC(t, plan, manifest)
		_, err = ExecuteImport(context.Background(), plan, manifest, directory, rpc, resumeSigner, ImportExecutionOptions{MaxGasLimit: 1_000_000})
		if err == nil || !strings.Contains(err.Error(), "exceeds current maximum") {
			t.Fatalf("error = %v", err)
		}
		if rpc.sendCalls != 0 {
			t.Fatalf("over-limit prepared transaction was broadcast %d times", rpc.sendCalls)
		}
	})

	t.Run("receipt block is not canonical", func(t *testing.T) {
		rpc := newRunnerTestRPC(t, plan, manifest)
		rpc.canonicalBlockHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		_, err := ExecuteImport(context.Background(), plan, manifest, filepath.Join(t.TempDir(), "journal"), rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
		if err == nil || !strings.Contains(err.Error(), "canonical hash") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("reorged recorded receipt", func(t *testing.T) {
		rpc := newRunnerTestRPC(t, plan, manifest)
		directory := filepath.Join(t.TempDir(), "journal")
		if _, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second}); err != nil {
			t.Fatal(err)
		}
		for _, receipt := range rpc.receipts {
			receipt.BlockHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
			for index := range receipt.Logs {
				receipt.Logs[index].BlockHash = receipt.BlockHash
			}
			break
		}
		_, err := ExecuteImport(context.Background(), plan, manifest, directory, rpc, signer, ImportExecutionOptions{ReceiptTimeout: time.Second})
		if err == nil || !strings.Contains(err.Error(), "receipt evidence changed") {
			t.Fatalf("error = %v", err)
		}
	})

	if _, err := ExecuteImport(context.Background(), plan, manifest, "journal", nil, signer, ImportExecutionOptions{}); err == nil {
		t.Fatal("nil RPC was accepted")
	}
	if _, err := ExecuteImport(context.Background(), plan, manifest, "journal", newRunnerTestRPC(t, plan, manifest), nil, ImportExecutionOptions{}); err == nil {
		t.Fatal("nil signer was accepted")
	}
}

type runnerTestSigner struct {
	key          *ecdsa.PrivateKey
	signCalls    int
	transactions []chain.EVMTransaction
}

func (signer *runnerTestSigner) OwnerAddress() (string, error) {
	return strings.ToLower(ethcrypto.PubkeyToAddress(signer.key.PublicKey).Hex()), nil
}

func (signer *runnerTestSigner) CreateKey(string) error { return errors.New("not supported") }

func (signer *runnerTestSigner) SignTransaction(_ context.Context, transaction chain.EVMTransaction) (string, error) {
	signer.signCalls++
	signer.transactions = append(signer.transactions, transaction)
	data, err := hex.DecodeString(strings.TrimPrefix(transaction.Data, "0x"))
	if err != nil {
		return "", err
	}
	gasPrice, ok := new(big.Int).SetString(strings.TrimPrefix(transaction.GasPrice, "0x"), 16)
	if !ok {
		return "", errors.New("invalid gas price")
	}
	value, ok := new(big.Int).SetString(strings.TrimPrefix(transaction.Value, "0x"), 16)
	if !ok {
		return "", errors.New("invalid value")
	}
	target := common.HexToAddress(transaction.To)
	unsigned := types.NewTx(&types.LegacyTx{
		Nonce: transaction.Nonce, To: &target, Value: value, Gas: transaction.GasLimit,
		GasPrice: gasPrice, Data: data,
	})
	signed, err := types.SignTx(unsigned, types.LatestSignerForChainID(new(big.Int).SetUint64(transaction.ChainID)), signer.key)
	if err != nil {
		return "", err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(raw), nil
}

type runnerTestRPC struct {
	t                  *testing.T
	plan               *Plan
	manifest           *TransactionManifest
	chainID            uint64
	nextNonce          uint64
	estimatedGas       uint64
	gasPrice           string
	receipts           map[string]*chain.EVMReceipt
	canonicalBlockHash string
	sendCalls          int
	alreadyKnownOnce   bool
	revertOrder        int
	missingEventOrder  int
}

func newRunnerTestRPC(t *testing.T, plan *Plan, manifest *TransactionManifest) *runnerTestRPC {
	return &runnerTestRPC{
		t: t, plan: plan, manifest: manifest, chainID: plan.Target.ChainID,
		estimatedGas: 100_000, gasPrice: "0x2", receipts: make(map[string]*chain.EVMReceipt),
		canonicalBlockHash: journalBlockHash,
		revertOrder:        -1, missingEventOrder: -1,
	}
}

func (rpc *runnerTestRPC) ChainID(context.Context) (uint64, error) { return rpc.chainID, nil }

func (rpc *runnerTestRPC) TransactionCount(context.Context, string, string) (uint64, error) {
	return rpc.nextNonce, nil
}

func (rpc *runnerTestRPC) EstimateGas(context.Context, chain.EVMCall) (uint64, error) {
	return rpc.estimatedGas, nil
}

func (rpc *runnerTestRPC) GasPrice(context.Context) (string, error) { return rpc.gasPrice, nil }

func (rpc *runnerTestRPC) SendRawTransaction(_ context.Context, raw string) (string, error) {
	rpc.sendCalls++
	transaction := decodeRunnerTestTransaction(rpc.t, raw)
	hash := transaction.Hash().Hex()
	order := rpc.orderForTransaction(transaction)
	receipt := journalReceipt(rpc.t, rpc.plan, rpc.manifest, order, hash)
	if order == rpc.revertOrder {
		receipt.Status = "0x0"
	}
	if order == rpc.missingEventOrder {
		receipt.Logs = nil
	}
	rpc.receipts[hash] = receipt
	if transaction.Nonce() >= rpc.nextNonce {
		rpc.nextNonce = transaction.Nonce() + 1
	}
	if rpc.alreadyKnownOnce {
		rpc.alreadyKnownOnce = false
		return "", &chain.RPCError{Method: "eth_sendRawTransaction", Code: -32000, Message: "already known"}
	}
	return hash, nil
}

func (rpc *runnerTestRPC) TransactionReceipt(_ context.Context, hash string) (*chain.EVMReceipt, error) {
	receipt := rpc.receipts[hash]
	if receipt == nil {
		return nil, nil
	}
	copyReceipt := cloneEVMReceipt(*receipt)
	return &copyReceipt, nil
}

func (rpc *runnerTestRPC) WaitReceipt(ctx context.Context, hash string) (*chain.EVMReceipt, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	receipt, _ := rpc.TransactionReceipt(ctx, hash)
	if receipt == nil {
		return nil, errors.New("test receipt missing")
	}
	if receipt.Status == "0x0" {
		return receipt, &chain.TransactionRevertedError{Hash: hash, Status: receipt.Status}
	}
	return receipt, nil
}

func (rpc *runnerTestRPC) BlockByNumber(_ context.Context, blockTag string) (*chain.EVMBlock, error) {
	return &chain.EVMBlock{Number: blockTag, Hash: rpc.canonicalBlockHash}, nil
}

func (rpc *runnerTestRPC) orderForTransaction(transaction *types.Transaction) int {
	for order, candidate := range rpc.manifest.Transactions {
		data, err := hex.DecodeString(strings.TrimPrefix(candidate.Data, "0x"))
		if err == nil && bytes.Equal(data, transaction.Data()) && transaction.To() != nil && strings.EqualFold(transaction.To().Hex(), candidate.To) {
			return order
		}
	}
	rpc.t.Fatalf("raw transaction does not match the manifest")
	return -1
}

func decodeRunnerTestTransaction(t *testing.T, raw string) *types.Transaction {
	t.Helper()
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(decoded); err != nil {
		t.Fatal(err)
	}
	return &transaction
}

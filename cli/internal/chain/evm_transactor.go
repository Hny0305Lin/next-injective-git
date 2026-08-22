package chain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

const (
	defaultEVMReceiptTimeout = 2 * time.Minute
	injectiveMinGasPriceWei  = uint64(160_000_000)
)

// EVMTransactionResult is durable evidence that a transaction was broadcast
// and, when Receipt is non-nil, confirmed by the selected chain.
type EVMTransactionResult struct {
	Hash    string
	Receipt *EVMReceipt
}

// EVMReceiptUnconfirmedError distinguishes an uncertain post-broadcast state
// from failures which happened before the transaction reached the RPC node.
// Callers can use Hash to inspect the transaction without risking a duplicate.
type EVMReceiptUnconfirmedError struct {
	Hash string
	Err  error
}

func (e *EVMReceiptUnconfirmedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("EVM transaction %s was broadcast but its receipt was not confirmed: %v", e.Hash, e.Err)
}

func (e *EVMReceiptUnconfirmedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// EVMTransactor owns the common Injective EVM write pipeline. Contract
// backends remain responsible only for address validation and ABI encoding.
type EVMTransactor struct {
	cfg            config.Config
	rpc            *EVMRPC
	signer         EVMSigner
	nonceManager   *evmNonceManager
	receiptTimeout time.Duration
}

func newEVMTransactor(cfg config.Config, rpc *EVMRPC, signer EVMSigner, nonceManager *evmNonceManager) *EVMTransactor {
	if nonceManager == nil {
		nonceManager = newEVMNonceManager()
	}
	return &EVMTransactor{
		cfg: cfg, rpc: rpc, signer: signer, nonceManager: nonceManager,
		receiptTimeout: defaultEVMReceiptTimeout,
	}
}

// NewEVMTransactor exposes the same write pipeline to explicit administrator
// commands such as reproducible suite deployment. Ordinary runtime clients
// construct it through EVMSuiteRegistry.
func NewEVMTransactor(cfg config.Config, rpc *EVMRPC, signer EVMSigner) *EVMTransactor {
	return newEVMTransactor(cfg, rpc, signer, nil)
}

// SetReceiptTimeout adjusts how long Send waits for a receipt after the raw
// transaction has already been accepted by the RPC. Injective's public RPC
// can lag on receipt indexing; state-aware administrative callers may use a
// shorter bound and then verify the resulting contract/storage directly.
func (t *EVMTransactor) SetReceiptTimeout(timeout time.Duration) {
	if t == nil {
		return
	}
	if timeout <= 0 {
		timeout = defaultEVMReceiptTimeout
	}
	t.receiptTimeout = timeout
}

// Send signs, broadcasts, and confirms one replay-protected legacy
// transaction. The Suite keeps its tested type-0 policy until a funded type-2
// canary has produced a retained receipt on the target Injective EVM network.
func (t *EVMTransactor) Send(ctx context.Context, target string, data []byte, value string) (*EVMTransactionResult, error) {
	contract, err := normalizeEVMAddress(target)
	if err != nil {
		return nil, err
	}
	return t.transact(ctx, contract, data, value)
}

// Deploy signs and confirms a legacy contract-creation transaction. The
// receipt must include a non-zero contractAddress before the deployment is
// treated as successful evidence.
func (t *EVMTransactor) Deploy(ctx context.Context, initcode []byte, value string) (*EVMTransactionResult, error) {
	if len(initcode) == 0 {
		return nil, errors.New("EVM deployment initcode is empty")
	}
	result, err := t.transact(ctx, "", initcode, value)
	if err != nil {
		return result, err
	}
	if result == nil || result.Receipt == nil {
		return result, errors.New("EVM deployment returned no receipt")
	}
	address, addressErr := normalizeEVMAddress(result.Receipt.ContractAddress)
	if addressErr != nil || address == "0x0000000000000000000000000000000000000000" {
		return result, fmt.Errorf("EVM deployment receipt returned invalid contract address %q", result.Receipt.ContractAddress)
	}
	result.Receipt.ContractAddress = address
	return result, nil
}

func (t *EVMTransactor) transact(ctx context.Context, target string, data []byte, value string) (*EVMTransactionResult, error) {
	if t == nil || t.signer == nil {
		return nil, ErrEVMSignerUnavailable
	}
	if t.rpc == nil {
		return nil, fmt.Errorf("EVM RPC transport is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	owner, err := t.signer.OwnerAddress()
	if err != nil {
		return nil, fmt.Errorf("resolve EVM signer address: %w", err)
	}
	from, err := normalizeEVMAddress(owner)
	if err != nil {
		return nil, fmt.Errorf("resolve EVM signer address: %w", err)
	}
	chainID, err := t.rpc.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if expected := t.cfg.EffectiveEVMChainID(); expected != 0 && expected != chainID {
		return nil, fmt.Errorf("EVM chain ID mismatch: RPC reported %d, profile requires %d", chainID, expected)
	}
	nonce, reservation, err := t.nonceManager.reserve(ctx, t.rpc, chainID, from)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			reservation.Invalidate()
		}
	}()

	calldata := "0x" + hexEncode(data)
	gas, err := t.rpc.EstimateGas(ctx, EVMCall{From: from, To: target, Data: calldata, Value: value})
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	gasLimit, err := AdjustEVMGasLimit(gas, 10_000)
	if err != nil {
		return nil, err
	}
	gasPrice, err := t.rpc.GasPrice(ctx)
	if err != nil {
		return nil, err
	}
	gasPrice, err = injectiveLegacyGasPrice(chainID, gasPrice)
	if err != nil {
		return nil, err
	}
	rawTx, err := t.signer.SignTransaction(ctx, EVMTransaction{
		ChainID: chainID, Nonce: nonce, To: target, Data: calldata,
		GasLimit: gasLimit, GasPrice: gasPrice, Value: value,
	})
	if err != nil {
		return nil, fmt.Errorf("sign EVM transaction: %w", err)
	}
	if _, err := decodeHexBytes(rawTx); err != nil {
		return nil, fmt.Errorf("signer returned invalid raw EVM transaction: %w", err)
	}
	hash, err := t.rpc.SendRawTransaction(ctx, rawTx)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	result := &EVMTransactionResult{Hash: hash}
	receiptCtx, cancel := context.WithTimeout(ctx, t.receiptTimeout)
	defer cancel()
	receipt, err := t.rpc.WaitReceipt(receiptCtx, hash)
	result.Receipt = receipt
	if err != nil {
		var reverted *TransactionRevertedError
		if errors.As(err, &reverted) {
			return result, err
		}
		return result, &EVMReceiptUnconfirmedError{Hash: hash, Err: err}
	}
	reservation.Commit()
	committed = true
	return result, nil
}

func injectiveLegacyGasPrice(chainID uint64, gasPrice string) (string, error) {
	price, err := parseHexUint(gasPrice)
	if err != nil {
		return "", err
	}
	if (chainID == 1439 || chainID == 1776) && price < injectiveMinGasPriceWei {
		price = injectiveMinGasPriceWei
	}
	return fmt.Sprintf("0x%x", price), nil
}

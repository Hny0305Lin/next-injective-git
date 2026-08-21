// Command evm-demo-inspect performs read-only inspection of a deployed EVM
// demo Suite. It intentionally has no signer and no transaction path.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type artifact struct {
	ABI json.RawMessage `json:"abi"`
}

type inspector struct {
	rpc       *chain.EVMRPC
	artifacts string
	ctx       context.Context
}

func main() {
	var directory, coordinator, core, artifacts, rpcURL, codeAddresses, account string
	var txHash string
	flag.StringVar(&directory, "directory", "", "SuiteDirectory address")
	flag.StringVar(&coordinator, "coordinator", "", "BootstrapCoordinator address")
	flag.StringVar(&core, "core", "", "RepositoryCore address (optional)")
	flag.StringVar(&artifacts, "artifacts", "contracts/evm-v2/artifacts", "artifact directory")
	flag.StringVar(&rpcURL, "rpc", "", "EVM JSON-RPC endpoint (defaults to config)")
	flag.StringVar(&txHash, "tx-hash", "", "transaction hash to inspect (read-only)")
	flag.StringVar(&codeAddresses, "code-addresses", "", "comma-separated addresses to inspect (read-only)")
	flag.StringVar(&account, "account", "", "account address to inspect nonce/balance (read-only)")
	flag.Parse()
	if strings.TrimSpace(directory) == "" {
		fatal("-directory is required")
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Sprintf("load config: %v", err))
	}
	if strings.TrimSpace(rpcURL) == "" {
		rpcURL = cfg.EffectiveEVMRPC()
	}
	if strings.TrimSpace(rpcURL) == "" {
		fatal("EVM RPC endpoint is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	i := &inspector{rpc: chain.NewEVMRPC(rpcURL), artifacts: artifacts, ctx: ctx}
	if strings.TrimSpace(txHash) != "" {
		if err := i.inspectTransaction(txHash); err != nil {
			fatal(err.Error())
		}
		return
	}
	if strings.TrimSpace(codeAddresses) != "" {
		for _, rawAddress := range strings.Split(codeAddresses, ",") {
			address := common.HexToAddress(strings.TrimSpace(rawAddress)).Hex()
			code, codeErr := i.rpc.CodeAt(i.ctx, address, "latest")
			if codeErr != nil {
				fatal(fmt.Sprintf("code %s: %v", address, codeErr))
			}
			fmt.Printf("address=%s\ncode_bytes=%d\ncode_hash=%s\n", address, len(code), crypto.Keccak256Hash(code).Hex())
		}
		return
	}
	if strings.TrimSpace(account) != "" {
		address := common.HexToAddress(strings.TrimSpace(account)).Hex()
		for _, tag := range []string{"latest", "pending"} {
			nonce, nonceErr := i.rpc.TransactionCount(i.ctx, address, tag)
			if nonceErr != nil {
				fatal(fmt.Sprintf("nonce %s: %v", tag, nonceErr))
			}
			fmt.Printf("account=%s\nnonce.%s=%d\n", address, tag, nonce)
		}
		balance, balanceErr := i.rpc.Balance(i.ctx, address, "latest")
		if balanceErr != nil {
			fatal(fmt.Sprintf("balance: %v", balanceErr))
		}
		fmt.Printf("balance_wei=%s\n", balance.String())
		return
	}
	directory = common.HexToAddress(directory).Hex()
	if err := i.inspectDirectory(directory, coordinator, core); err != nil {
		fatal(err.Error())
	}
}

type transactionView struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Nonce       string `json:"nonce"`
	BlockNumber string `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
	Input       string `json:"input"`
}

func (i *inspector) inspectTransaction(hash string) error {
	var tx *transactionView
	if err := i.rpc.Call(i.ctx, "eth_getTransactionByHash", []any{hash}, &tx); err != nil {
		return fmt.Errorf("transaction lookup: %w", err)
	}
	if tx == nil {
		fmt.Printf("transaction=%s\ntransaction_status=not-indexed-or-pending\n", hash)
		return nil
	}
	fmt.Printf("transaction.hash=%s\ntransaction.from=%s\ntransaction.to=%s\ntransaction.nonce=%s\ntransaction.block_number=%s\ntransaction.block_hash=%s\ntransaction.input_bytes=%d\n", tx.Hash, tx.From, tx.To, tx.Nonce, tx.BlockNumber, tx.BlockHash, hexDataLen(tx.Input))
	var receipt *chain.EVMReceipt
	if err := i.rpc.Call(i.ctx, "eth_getTransactionReceipt", []any{hash}, &receipt); err != nil {
		fmt.Printf("transaction.receipt_error=%v\n", err)
	} else if receipt == nil {
		fmt.Println("transaction.receipt=null")
	} else {
		fmt.Printf("transaction.receipt_status=%s\ntransaction.receipt_contract=%s\ntransaction.receipt_block=%s\n", receipt.Status, receipt.ContractAddress, receipt.BlockNumber)
	}
	return nil
}

func hexDataLen(value string) int {
	value = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(value), "0x"), "0X")
	return len(value) / 2
}

func (i *inspector) inspectDirectory(directory, coordinator, core string) error {
	block, err := i.rpc.BlockNumber(i.ctx)
	if err != nil {
		return fmt.Errorf("block number: %w", err)
	}
	blockTag := fmt.Sprintf("0x%x", block)
	code, err := i.rpc.CodeAt(i.ctx, directory, blockTag)
	if err != nil {
		return fmt.Errorf("directory code: %w", err)
	}
	fmt.Printf("block=%s\ndirectory=%s\ndirectory_code_bytes=%d\ndirectory_code_hash=%s\n", blockTag, directory, len(code), crypto.Keccak256Hash(code).Hex())
	dirABI, err := i.loadABI("SuiteDirectory")
	if err != nil {
		return err
	}
	for _, method := range []string{"state", "suiteVersion", "configuredChainId", "snapshotRoot", "bootstrapAuthority", "bootstrapCoordinator", "bootstrapCoordinatorCodeHash", "registeredModuleCount"} {
		values, callErr := i.call(dirABI, directory, method)
		if callErr != nil {
			return fmt.Errorf("directory.%s: %w", method, callErr)
		}
		fmt.Printf("directory.%s=%s\n", method, formatValues(values))
	}
	boundCoordinator := common.HexToAddress(strings.TrimSpace(coordinator))
	if boundCoordinator == (common.Address{}) {
		values, callErr := i.call(dirABI, directory, "bootstrapCoordinator")
		if callErr != nil {
			return callErr
		}
		if len(values) == 1 {
			if address, ok := values[0].(common.Address); ok {
				boundCoordinator = address
			}
		}
	}
	if boundCoordinator != (common.Address{}) {
		if err := i.inspectCoordinator(boundCoordinator.Hex()); err != nil {
			return err
		}
	}
	ids := []struct{ name, label string }{
		{"core", "igit.module.repository-core"},
		{"recovery", "igit.module.recovery"},
		{"moderation", "igit.module.moderation"},
		{"economic", "igit.module.economic"},
		{"username", "igit.module.username"},
		{"badge", "igit.module.badge"},
		{"release", "igit.module.release"},
	}
	for _, item := range ids {
		id := crypto.Keccak256Hash([]byte(item.label))
		values, callErr := i.call(dirABI, directory, "moduleAddress", id)
		if callErr != nil {
			return fmt.Errorf("directory.moduleAddress(%s): %w", item.name, callErr)
		}
		address := common.Address{}
		if len(values) == 1 {
			address, _ = values[0].(common.Address)
		}
		fmt.Printf("module.%s.id=%s\nmodule.%s.address=%s\n", item.name, id.Hex(), item.name, address.Hex())
		if address == (common.Address{}) {
			continue
		}
		code, codeErr := i.rpc.CodeAt(i.ctx, address.Hex(), "latest")
		if codeErr != nil {
			return fmt.Errorf("module.%s code: %w", item.name, codeErr)
		}
		fmt.Printf("module.%s.code_bytes=%d\nmodule.%s.code_hash=%s\n", item.name, len(code), item.name, crypto.Keccak256Hash(code).Hex())
		codeHashValues, hashErr := i.call(dirABI, directory, "moduleCodeHash", id)
		if hashErr != nil {
			return fmt.Errorf("directory.moduleCodeHash(%s): %w", item.name, hashErr)
		}
		fmt.Printf("module.%s.directory_code_hash=%s\n", item.name, formatValues(codeHashValues))
		moduleABI, abiErr := i.loadABI(moduleArtifact(item.name))
		if abiErr != nil {
			return abiErr
		}
		for _, method := range []string{"suiteDirectory", "bootstrapCoordinator", "moduleId", "bootstrapFinalized"} {
			values, methodErr := i.call(moduleABI, address.Hex(), method)
			if methodErr != nil {
				return fmt.Errorf("module.%s.%s: %w", item.name, method, methodErr)
			}
			fmt.Printf("module.%s.%s=%s\n", item.name, method, formatValues(values))
		}
		if boundCoordinator != (common.Address{}) {
			coordABI, coordErr := i.loadABI("BootstrapCoordinator")
			if coordErr != nil {
				return coordErr
			}
			progress, progressErr := i.call(coordABI, boundCoordinator.Hex(), "moduleProgress", id)
			if progressErr != nil {
				return fmt.Errorf("coordinator.moduleProgress(%s): %w", item.name, progressErr)
			}
			fmt.Printf("module.%s.progress=%s\n", item.name, formatValues(progress))
		}
	}
	if strings.TrimSpace(core) != "" {
		if err := i.inspectCore(common.HexToAddress(core).Hex()); err != nil {
			return err
		}
	}
	return nil
}

func (i *inspector) inspectCoordinator(coordinator string) error {
	code, err := i.rpc.CodeAt(i.ctx, coordinator, "latest")
	if err != nil {
		return fmt.Errorf("coordinator code: %w", err)
	}
	fmt.Printf("coordinator=%s\ncoordinator_code_bytes=%d\ncoordinator_code_hash=%s\n", coordinator, len(code), crypto.Keccak256Hash(code).Hex())
	coordABI, err := i.loadABI("BootstrapCoordinator")
	if err != nil {
		return err
	}
	for _, method := range []string{"suiteDirectory", "snapshotRoot", "operator", "nextModuleIndex", "usernameEscrowEvidenceHash", "activated", "readyForActivation"} {
		values, callErr := i.call(coordABI, coordinator, method)
		if callErr != nil {
			return fmt.Errorf("coordinator.%s: %w", method, callErr)
		}
		fmt.Printf("coordinator.%s=%s\n", method, formatValues(values))
	}
	return nil
}

func (i *inspector) inspectCore(core string) error {
	code, err := i.rpc.CodeAt(i.ctx, core, "latest")
	if err != nil {
		return fmt.Errorf("core code: %w", err)
	}
	fmt.Printf("core=%s\ncore_code_bytes=%d\ncore_code_hash=%s\n", core, len(code), crypto.Keccak256Hash(code).Hex())
	coreABI, err := i.loadABI("RepositoryCore")
	if err != nil {
		return err
	}
	for _, method := range []string{"suiteDirectory", "bootstrapCoordinator", "moduleId"} {
		values, callErr := i.call(coreABI, core, method)
		if callErr != nil {
			return fmt.Errorf("core.%s: %w", method, callErr)
		}
		fmt.Printf("core.%s=%s\n", method, formatValues(values))
	}
	return nil
}

func (i *inspector) loadABI(name string) (abi.ABI, error) {
	path := filepath.Join(i.artifacts, name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("read ABI %s: %w", path, err)
	}
	var artifact artifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return abi.ABI{}, fmt.Errorf("decode ABI %s: %w", name, err)
	}
	parsed, err := abi.JSON(strings.NewReader(string(artifact.ABI)))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("parse ABI %s: %w", name, err)
	}
	return parsed, nil
}

func (i *inspector) call(contractABI abi.ABI, address, method string, args ...any) ([]any, error) {
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	raw, err := i.rpc.CallContract(i.ctx, address, "0x"+hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}
	values, err := contractABI.Unpack(method, raw)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return values, nil
}

func moduleArtifact(name string) string {
	switch name {
	case "core":
		return "RepositoryCore"
	case "recovery":
		return "RecoveryModule"
	case "moderation":
		return "ModerationModule"
	case "economic":
		return "EconomicModule"
	case "username":
		return "UsernameModule"
	case "badge":
		return "BadgeModule"
	case "release":
		return "ReleaseModule"
	default:
		return name
	}
}

func formatValues(values []any) string {
	parts := make([]string, len(values))
	for index, value := range values {
		switch typed := value.(type) {
		case common.Address:
			parts[index] = typed.Hex()
		case [32]byte:
			parts[index] = common.BytesToHash(typed[:]).Hex()
		default:
			parts[index] = fmt.Sprintf("%v", typed)
		}
	}
	return strings.Join(parts, " | ")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

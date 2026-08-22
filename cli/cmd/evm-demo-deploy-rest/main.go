// Continues a demo Suite after its Directory deployment has been confirmed.
// It never creates a Directory; the address is an explicit required input.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	ABI              json.RawMessage `json:"abi"`
	CreationBytecode string          `json:"creation_bytecode"`
}
type deployed struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	DeployTx string `json:"deploy_tx"`
	CodeHash string `json:"code_hash"`
}
type evidence struct {
	Schema        string              `json:"schema"`
	Status        string              `json:"status"`
	Directory     string              `json:"directory"`
	Coordinator   string              `json:"coordinator"`
	Contracts     []deployed          `json:"contracts"`
	Configuration []map[string]string `json:"configuration"`
}

func main() {
	var artifacts, directory, coordinatorFlag, existingCore, existingRecovery, existingModeration, existingEconomic, existingUsername, existingBadge, existingRelease, key, output string
	flag.StringVar(&artifacts, "artifacts", "", "artifact directory")
	flag.StringVar(&directory, "directory", "", "already deployed SuiteDirectory")
	flag.StringVar(&coordinatorFlag, "coordinator", "", "already deployed BootstrapCoordinator (optional)")
	flag.StringVar(&existingCore, "core", "", "already deployed RepositoryCore (optional)")
	flag.StringVar(&existingRecovery, "recovery", "", "already deployed RecoveryModule (optional)")
	flag.StringVar(&existingModeration, "moderation", "", "already deployed ModerationModule (optional)")
	flag.StringVar(&existingEconomic, "economic", "", "already deployed EconomicModule (optional)")
	flag.StringVar(&existingUsername, "username", "", "already deployed UsernameModule (optional)")
	flag.StringVar(&existingBadge, "badge", "", "already deployed BadgeModule (optional)")
	flag.StringVar(&existingRelease, "release", "", "already deployed ReleaseModule (optional)")
	flag.StringVar(&key, "key", "", "encrypted EVM key")
	flag.StringVar(&output, "output", "", "exclusive evidence output")
	flag.Parse()
	if artifacts == "" || directory == "" || key == "" || output == "" {
		fatal("--artifacts, --directory, --key, and --output are required")
	}
	directory = common.HexToAddress(directory).Hex()
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Sprintf("load config: %v", err))
	}
	cfg.KeyName = key
	cfg.EVMSuiteDirectoryAddress = directory
	cfg.EVMChainID = 1439
	rpc := chain.NewEVMRPC(cfg.EffectiveEVMRPC())
	tx := chain.NewEVMTransactor(cfg, rpc, chain.NewEVMKeystoreSigner(cfg))
	// Injective's public receipt index can lag after state is committed; this
	// administrator command verifies code/storage after an uncertain send.
	tx.SetReceiptTimeout(12 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	code, err := rpc.CodeAt(ctx, directory, "latest")
	if err != nil || len(code) == 0 {
		fatal(fmt.Sprintf("verify existing Directory: %v", err))
	}
	e := evidence{Schema: "igit.evm-suite.demo-deployment.v1", Status: "broadcasting", Directory: strings.ToLower(directory)}
	write(output, e)
	coordABI, err := loadABI(artifacts, "BootstrapCoordinator")
	if err != nil {
		fatal(err.Error())
	}
	operator, err := chain.NewEVMKeystoreSigner(cfg).OwnerAddress()
	if err != nil {
		fatal(err.Error())
	}
	operatorEVM, err := chain.NormalizeEVMAddress(operator)
	if err != nil {
		fatal(fmt.Sprintf("normalize operator: %v", err))
	}
	coordinator := ""
	if strings.TrimSpace(coordinatorFlag) != "" {
		coordinator = common.HexToAddress(coordinatorFlag).Hex()
	} else {
		coordCreation, err := loadCreation(artifacts, "BootstrapCoordinator")
		if err != nil {
			fatal(err.Error())
		}
		coordCtor, err := coordABI.Constructor.Inputs.Pack(common.HexToAddress(directory), common.HexToAddress(operatorEVM))
		if err != nil {
			fatal(err.Error())
		}
		coordResult, err := tx.Deploy(ctx, append(coordCreation, coordCtor...), "0x0")
		if err != nil {
			fatal(fmt.Sprintf("deploy coordinator: %v", err))
		}
		coordinator = coordResult.Receipt.ContractAddress
	}
	coordCode, err := rpc.CodeAt(ctx, coordinator, "latest")
	if err != nil {
		fatal(err.Error())
	}
	if len(coordCode) == 0 {
		fatal("BootstrapCoordinator has no runtime code")
	}
	e.Coordinator = strings.ToLower(coordinator)
	e.Contracts = append(e.Contracts, deployed{Name: "BootstrapCoordinator", Address: strings.ToLower(coordinator), CodeHash: crypto.Keccak256Hash(coordCode).Hex()})
	write(output, e)
	dirABI, err := loadABI(artifacts, "SuiteDirectory")
	if err != nil {
		fatal(err.Error())
	}
	for _, view := range []string{"state", "bootstrapAuthority", "bootstrapCoordinator", "registeredModuleCount"} {
		viewData, packErr := dirABI.Pack(view)
		if packErr != nil {
			fatal(packErr.Error())
		}
		viewRaw, callErr := rpc.CallContract(ctx, directory, "0x"+hex.EncodeToString(viewData))
		if callErr != nil {
			fatal(fmt.Sprintf("read %s: %v", view, callErr))
		}
		viewValues, unpackErr := dirABI.Unpack(view, viewRaw)
		if unpackErr != nil || len(viewValues) != 1 {
			fatal(fmt.Sprintf("decode %s", view))
		}
		fmt.Printf("directory.%s=%v\n", view, viewValues[0])
	}
	boundData, err := dirABI.Pack("bootstrapCoordinator")
	if err != nil {
		fatal(err.Error())
	}
	boundRaw, err := rpc.CallContract(ctx, directory, "0x"+hex.EncodeToString(boundData))
	if err != nil {
		fatal(fmt.Sprintf("read coordinator binding: %v", err))
	}
	boundValues, err := dirABI.Unpack("bootstrapCoordinator", boundRaw)
	if err != nil || len(boundValues) != 1 {
		fatal("decode coordinator binding")
	}
	bound, ok := boundValues[0].(common.Address)
	if !ok {
		fatal("coordinator binding has unexpected ABI type")
	}
	if bound == (common.Address{}) {
		setData, err := dirABI.Pack("setBootstrapCoordinator", common.HexToAddress(coordinator), crypto.Keccak256Hash(coordCode))
		if err != nil {
			fatal(err.Error())
		}
		setResult, err := tx.Send(ctx, directory, setData, "0x0")
		if err != nil {
			fatal(fmt.Sprintf("bind coordinator: %v", err))
		}
		e.Configuration = append(e.Configuration, map[string]string{"phase": "bind_bootstrap_coordinator", "tx": setResult.Hash})
		write(output, e)
	} else if !strings.EqualFold(bound.Hex(), coordinator) {
		fatal(fmt.Sprintf("Directory is already bound to %s, not %s", bound.Hex(), coordinator))
	}
	moduleDefs := []struct {
		name, label, existing string
		args                  func(common.Address) []any
	}{
		{"RepositoryCore", "igit.module.repository-core", existingCore, func(c common.Address) []any { return []any{common.HexToAddress(directory), c} }},
		{"RecoveryModule", "igit.module.recovery", existingRecovery, func(c common.Address) []any { return []any{common.HexToAddress(directory), c} }},
		{"ModerationModule", "igit.module.moderation", existingModeration, func(c common.Address) []any {
			return []any{common.HexToAddress(directory), c, common.HexToAddress(operatorEVM), common.HexToAddress(operatorEVM)}
		}},
		{"EconomicModule", "igit.module.economic", existingEconomic, func(c common.Address) []any {
			return []any{common.HexToAddress(directory), c, common.HexToAddress(operatorEVM), common.HexToAddress(operatorEVM), uint16(300)}
		}},
		{"UsernameModule", "igit.module.username", existingUsername, func(c common.Address) []any {
			return []any{common.HexToAddress(directory), c, common.HexToAddress(operatorEVM)}
		}},
		{"BadgeModule", "igit.module.badge", existingBadge, func(c common.Address) []any { return []any{common.HexToAddress(directory), c} }},
		{"ReleaseModule", "igit.module.release", existingRelease, func(c common.Address) []any {
			return []any{common.HexToAddress(directory), c, common.HexToAddress(operatorEVM)}
		}},
	}
	for _, def := range moduleDefs {
		moduleABI, err := loadABI(artifacts, def.name)
		if err != nil {
			fatal(err.Error())
		}
		address := ""
		var deployTx string
		if strings.TrimSpace(def.existing) != "" {
			address = common.HexToAddress(def.existing).Hex()
			code, err := rpc.CodeAt(ctx, address, "latest")
			if err != nil || len(code) == 0 {
				fatal(fmt.Sprintf("verify existing core: %v", err))
			}
		} else {
			creation, err := loadCreation(artifacts, def.name)
			if err != nil {
				fatal(err.Error())
			}
			ctor, err := moduleABI.Constructor.Inputs.Pack(def.args(common.HexToAddress(coordinator))...)
			if err != nil {
				fatal(fmt.Sprintf("encode %s: %v", def.name, err))
			}
			// Reserve the expected CREATE address before broadcasting so a receipt
			// indexing timeout can be recovered from chain code alone.
			nonce, nonceErr := rpc.TransactionCount(ctx, operatorEVM, "pending")
			if nonceErr != nil {
				fatal(fmt.Sprintf("read nonce before deploy %s: %v", def.name, nonceErr))
			}
			expectedAddress := crypto.CreateAddress(common.HexToAddress(operatorEVM), nonce).Hex()
			result, sendErr := tx.Deploy(ctx, append(creation, ctor...), "0x0")
			if sendErr != nil {
				var uncertain *chain.EVMReceiptUnconfirmedError
				if !errors.As(sendErr, &uncertain) {
					fatal(fmt.Sprintf("deploy %s: %v", def.name, sendErr))
				}
				code, codeErr := rpc.CodeAt(ctx, expectedAddress, "latest")
				if codeErr != nil || len(code) == 0 {
					fatal(fmt.Sprintf("deploy %s uncertain (%s), expected address %s has no code: %v", def.name, uncertain.Hash, expectedAddress, codeErr))
				}
				address, deployTx = expectedAddress, uncertain.Hash
				fmt.Printf("%s deployment receipt delayed; recovered code at %s (%s)\n", def.name, address, deployTx)
			} else {
				if result == nil || result.Receipt == nil {
					fatal(fmt.Sprintf("deploy %s returned no receipt", def.name))
				}
				address, deployTx = result.Receipt.ContractAddress, result.Hash
			}
		}
		runtime, err := rpc.CodeAt(ctx, address, "latest")
		if err != nil {
			fatal(err.Error())
		}
		codeHash := crypto.Keccak256Hash(runtime)
		e.Contracts = append(e.Contracts, deployed{Name: def.name, Address: strings.ToLower(address), DeployTx: deployTx, CodeHash: codeHash.Hex()})
		moduleID := crypto.Keccak256Hash([]byte(def.label))
		moduleData, err := dirABI.Pack("moduleAddress", moduleID)
		if err != nil {
			fatal(err.Error())
		}
		moduleRaw, err := rpc.CallContract(ctx, directory, "0x"+hex.EncodeToString(moduleData))
		if err != nil {
			fatal(err.Error())
		}
		moduleValues, err := dirABI.Unpack("moduleAddress", moduleRaw)
		if err != nil || len(moduleValues) != 1 {
			fatal("decode module binding")
		}
		boundModule, ok := moduleValues[0].(common.Address)
		if !ok {
			fatal("module binding has unexpected ABI type")
		}
		if boundModule == (common.Address{}) {
			coordData, err := coordABI.Pack("registerModule", moduleID, common.HexToAddress(address), codeHash)
			if err != nil {
				fatal(fmt.Sprintf("encode register %s: %v", def.name, err))
			}
			regResult, sendErr := tx.Send(ctx, coordinator, coordData, "0x0")
			if sendErr != nil {
				var uncertain *chain.EVMReceiptUnconfirmedError
				if !errors.As(sendErr, &uncertain) {
					fatal(fmt.Sprintf("register %s: %v", def.name, sendErr))
				}
				boundModule = waitModuleBinding(ctx, rpc, dirABI, directory, moduleID, common.HexToAddress(address))
				if boundModule == (common.Address{}) {
					fatal(fmt.Sprintf("register %s uncertain (%s) and binding is still empty", def.name, uncertain.Hash))
				}
				regResult = &chain.EVMTransactionResult{Hash: uncertain.Hash}
			}
			if regResult == nil || strings.TrimSpace(regResult.Hash) == "" {
				fatal(fmt.Sprintf("register %s returned no transaction hash", def.name))
			}
			e.Configuration = append(e.Configuration, map[string]string{"phase": "register_" + def.name, "tx": regResult.Hash, "address": strings.ToLower(address)})
			// Re-read the binding before proceeding; this is the idempotency
			// boundary for receipt-lag recovery.
			boundModule = readModuleBinding(ctx, rpc, dirABI, directory, moduleID)
			if boundModule != common.HexToAddress(address) {
				fatal(fmt.Sprintf("register %s did not bind %s (got %s)", def.name, address, boundModule.Hex()))
			}
		} else if !strings.EqualFold(boundModule.Hex(), address) {
			fatal(fmt.Sprintf("Directory %s already bound to %s", def.name, boundModule.Hex()))
		}
		write(output, e)
		fmt.Printf("%s %s\n", def.name, address)
	}
	e.Status = "deployed_bootstrapping"
	write(output, e)
	fmt.Printf("demo suite ready for bootstrap: directory=%s coordinator=%s\n", directory, coordinator)
}

func loadABI(dir, name string) (abi.ABI, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return abi.ABI{}, err
	}
	var a artifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return abi.ABI{}, err
	}
	return abi.JSON(strings.NewReader(string(a.ABI)))
}
func loadCreation(dir, name string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var a artifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(a.CreationBytecode, "0x")
	return hex.DecodeString(s)
}
func readModuleBinding(ctx context.Context, rpc *chain.EVMRPC, dirABI abi.ABI, directory string, moduleID common.Hash) common.Address {
	data, err := dirABI.Pack("moduleAddress", moduleID)
	if err != nil {
		fatal(err.Error())
	}
	raw, err := rpc.CallContract(ctx, directory, "0x"+hex.EncodeToString(data))
	if err != nil {
		fatal(err.Error())
	}
	values, err := dirABI.Unpack("moduleAddress", raw)
	if err != nil || len(values) != 1 {
		fatal("decode module binding")
	}
	address, ok := values[0].(common.Address)
	if !ok {
		fatal("module binding has unexpected ABI type")
	}
	return address
}
func waitModuleBinding(ctx context.Context, rpc *chain.EVMRPC, dirABI abi.ABI, directory string, moduleID common.Hash, expected common.Address) common.Address {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		address := readModuleBinding(ctx, rpc, dirABI, directory, moduleID)
		if address == expected {
			return address
		}
		time.Sleep(750 * time.Millisecond)
	}
	return readModuleBinding(ctx, rpc, dirABI, directory, moduleID)
}
func write(path string, e evidence) {
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		fatal(err.Error())
	}
}
func fatal(msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(1) }

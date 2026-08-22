package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func suiteTestRepository(index int, owner common.Address) suiteRepositoryABI {
	var id [32]byte
	new(big.Int).SetInt64(int64(index + 1)).FillBytes(id[:])
	var parent [32]byte
	if index > 0 {
		new(big.Int).SetInt64(int64(index)).FillBytes(parent[:])
	}
	return suiteRepositoryABI{
		Id: id, Owner: owner, Name: fmt.Sprintf("repo-%02d", index),
		Description: fmt.Sprintf("description-%02d", index), DefaultBranch: "main",
		ForkedFrom: parent, CreatedAt: uint64(100 + index), UpdatedAt: uint64(200 + index), Exists: true,
	}
}

func TestEVMSuiteRegistryDrainsCoreTuplePagesAtOneBlock(t *testing.T) {
	fixture := newSuiteRPCFixture(t)
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	repositories := make([]suiteRepositoryABI, 65)
	for index := range repositories {
		repositories[index] = suiteTestRepository(index, owner)
	}
	core := fixture.addresses[RequiredSuiteModules[0].ID]
	moderation := fixture.addresses[RequiredSuiteModules[2].ID]
	pageCalls := 0
	fixture.moduleRuntime = func(address common.Address, method gethabi.Method, data []byte) ([]any, bool, error) {
		switch {
		case address == core && method.Name == "listRepositoriesPage":
			arguments, err := method.Inputs.Unpack(data[4:])
			if err != nil {
				return nil, true, err
			}
			cursor := arguments[1].(*big.Int).Uint64()
			limit := arguments[2].(*big.Int).Uint64()
			end := cursor + limit
			if end > uint64(len(repositories)) {
				end = uint64(len(repositories))
			}
			pageCalls++
			return []any{repositories[cursor:end], new(big.Int).SetUint64(end)}, true, nil
		case address == core && method.Name == "getRepository":
			return []any{repositories[1]}, true, nil
		case address == moderation && method.Name == "effectiveStatus":
			return []any{uint8(0)}, true, nil
		default:
			return nil, false, nil
		}
	}
	server := fixture.serve(t)
	defer server.Close()

	cfg := config.Defaults()
	cfg.EVMRPC = server.URL
	cfg.EVMChainID = fixture.chainID
	cfg.EVMSuiteDirectoryAddress = testSuiteDirectory
	registry := NewEVMSuiteRegistryWithDependencies(cfg, NewEVMRPC(server.URL), nil)
	result, err := registry.ListRepos(owner.Hex())
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(result) != len(repositories) || pageCalls != 2 {
		t.Fatalf("repositories=%d pageCalls=%d, want 65 and 2", len(result), pageCalls)
	}
	if result[64].Name != "repo-64" || result[64].Description != "description-64" || result[64].UpdatedAt != 264 {
		t.Fatalf("last repository = %#v", result[64])
	}

	fixed, err := registry.FixedBlockRepository(context.Background(), repositories[1].Id, "0x456")
	if err != nil {
		t.Fatalf("FixedBlockRepository: %v", err)
	}
	if fixed.Id != repositories[1].Id || fixed.ForkedFrom != repositories[1].ForkedFrom || !fixed.Exists {
		t.Fatalf("fixed Core tuple = %#v", fixed)
	}
	for _, blockTag := range fixture.blockTags {
		if blockTag != "0x456" {
			t.Fatalf("suite runtime read used block tag %q, want 0x456", blockTag)
		}
	}
}

type suiteWireSigner struct {
	privateKeyHex string
}

func (s *suiteWireSigner) OwnerAddress() (string, error) {
	key, err := ethcrypto.HexToECDSA(s.privateKeyHex)
	if err != nil {
		return "", err
	}
	return ethcrypto.PubkeyToAddress(key.PublicKey).Hex(), nil
}

func (s *suiteWireSigner) CreateKey(string) error { return nil }

func (s *suiteWireSigner) SignTransaction(_ context.Context, tx EVMTransaction) (string, error) {
	key, err := ethcrypto.HexToECDSA(s.privateKeyHex)
	if err != nil {
		return "", err
	}
	to := common.HexToAddress(tx.To)
	data, err := decodeHexBytes(tx.Data)
	if err != nil {
		return "", err
	}
	gasPrice, err := parseHexUint(tx.GasPrice)
	if err != nil {
		return "", err
	}
	value := new(big.Int)
	if strings.TrimSpace(tx.Value) != "" {
		value.SetString(strings.TrimPrefix(tx.Value, "0x"), 16)
	}
	unsigned := types.NewTx(&types.LegacyTx{
		Nonce: tx.Nonce, To: &to, Value: value, Gas: tx.GasLimit,
		GasPrice: new(big.Int).SetUint64(gasPrice), Data: data,
	})
	signed, err := types.SignTx(unsigned, types.LatestSignerForChainID(new(big.Int).SetUint64(tx.ChainID)), key)
	if err != nil {
		return "", err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(raw), nil
}

func TestEVMSuiteRegistryBroadcastsLegacyTypeZeroToVerifiedCore(t *testing.T) {
	fixture := newSuiteRPCFixture(t)
	core := fixture.addresses[RequiredSuiteModules[0].ID]
	var broadcast types.Transaction
	fixture.rpcRuntime = func(request rpcRequest) (any, bool, error) {
		switch request.Method {
		case "eth_getTransactionCount":
			return "0x7", true, nil
		case "eth_estimateGas":
			return "0x186a0", true, nil
		case "eth_gasPrice":
			return "0x1", true, nil
		case "eth_sendRawTransaction":
			var params []string
			if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) != 1 {
				return nil, true, fmt.Errorf("decode raw transaction params: %w", err)
			}
			raw, err := decodeHexBytes(params[0])
			if err != nil {
				return nil, true, err
			}
			if err := broadcast.UnmarshalBinary(raw); err != nil {
				return nil, true, err
			}
			return "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true, nil
		case "eth_getTransactionReceipt":
			return map[string]any{
				"transactionHash": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"blockNumber":     "0x500", "blockHash": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"to": core.Hex(), "status": "0x1", "gasUsed": "0x10000", "logs": []any{},
			}, true, nil
		default:
			return nil, false, nil
		}
	}
	server := fixture.serve(t)
	defer server.Close()

	cfg := config.Defaults()
	cfg.EVMRPC = server.URL
	cfg.EVMChainID = fixture.chainID
	cfg.EVMSuiteDirectoryAddress = testSuiteDirectory
	signer := &suiteWireSigner{privateKeyHex: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	registry := NewEVMSuiteRegistryWithDependencies(cfg, NewEVMRPC(server.URL), signer)
	if err := registry.CreateRepo("demo", "description", "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if broadcast.Type() != types.LegacyTxType {
		t.Fatalf("transaction type = %d, want legacy type 0", broadcast.Type())
	}
	if broadcast.To() == nil || *broadcast.To() != core {
		t.Fatalf("transaction target = %v, want verified Core %s", broadcast.To(), core.Hex())
	}
	if broadcast.GasPrice().Cmp(new(big.Int).SetUint64(injectiveMinGasPriceWei)) < 0 {
		t.Fatalf("gas price = %s, want at least %d", broadcast.GasPrice(), injectiveMinGasPriceWei)
	}
	coreABI, err := loadSuiteABI(suiteABICore)
	if err != nil {
		t.Fatal(err)
	}
	method, err := coreABI.MethodById(broadcast.Data()[:4])
	if err != nil || method.Name != "createRepository" {
		t.Fatalf("broadcast calldata method = %v, err=%v", method, err)
	}
}

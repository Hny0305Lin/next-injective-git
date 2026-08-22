package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

type fakeEVMSigner struct {
	address string
	raw     string
}

func (s *fakeEVMSigner) OwnerAddress() (string, error) { return s.address, nil }
func (s *fakeEVMSigner) CreateKey(string) error        { return nil }
func (s *fakeEVMSigner) SignTransaction(context.Context, EVMTransaction) (string, error) {
	return s.raw, nil
}

func TestInjectiveLegacyGasPriceAppliesNetworkMinimum(t *testing.T) {
	for _, test := range []struct {
		chainID uint64
		input   string
		want    string
	}{
		{chainID: 1439, input: "0x1", want: "0x9896800"},
		{chainID: 1776, input: "0x9896801", want: "0x9896801"},
		{chainID: 31337, input: "0x1", want: "0x1"},
	} {
		got, err := injectiveLegacyGasPrice(test.chainID, test.input)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("chain %d gas price = %s, want %s", test.chainID, got, test.want)
		}
	}
}

func TestEVMTransactorDeployBroadcastsLegacyCreationAndReturnsAddress(t *testing.T) {
	const privateKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key, err := ethcrypto.HexToECDSA(privateKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	from := ethcrypto.PubkeyToAddress(key.PublicKey)
	created := ethcrypto.CreateAddress(from, 4)
	var broadcast types.Transaction
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x59f"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			result = "0x186a0"
		case "eth_gasPrice":
			result = "0x1"
		case "eth_sendRawTransaction":
			var params []string
			if err := json.Unmarshal(mustJSON(request.Params), &params); err != nil || len(params) != 1 {
				t.Fatalf("raw transaction params: %v", err)
			}
			raw, decodeErr := decodeHexBytes(params[0])
			if decodeErr != nil {
				t.Fatalf("decode broadcast transaction: %v", decodeErr)
			}
			if decodeErr = broadcast.UnmarshalBinary(raw); decodeErr != nil {
				t.Fatalf("unmarshal broadcast transaction: %v", decodeErr)
			}
			result = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "eth_getTransactionReceipt":
			result = map[string]any{
				"transactionHash": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"blockNumber":     "0x500", "blockHash": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"to": nil, "contractAddress": created.Hex(), "status": "0x1", "gasUsed": "0x10000", "logs": []any{},
			}
		default:
			t.Fatalf("unexpected RPC method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	signer := &wireCreationSigner{keyHex: privateKeyHex}
	cfg := config.Defaults()
	cfg.EVMRPC = server.URL
	cfg.EVMChainID = 1439
	result, err := NewEVMTransactor(cfg, NewEVMRPC(server.URL), signer).Deploy(
		context.Background(), []byte{0x60, 0x00, 0x60, 0x00, 0xf3}, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(result.Receipt.ContractAddress, created.Hex()) {
		t.Fatalf("created address = %s, want %s", result.Receipt.ContractAddress, created.Hex())
	}
	if broadcast.Type() != types.LegacyTxType || broadcast.To() != nil {
		t.Fatalf("broadcast type=%d to=%v", broadcast.Type(), broadcast.To())
	}
	if broadcast.GasPrice().Uint64() < injectiveMinGasPriceWei {
		t.Fatalf("gas price = %d", broadcast.GasPrice().Uint64())
	}
}

type wireCreationSigner struct{ keyHex string }

func (s *wireCreationSigner) OwnerAddress() (string, error) {
	key, err := ethcrypto.HexToECDSA(s.keyHex)
	if err != nil {
		return "", err
	}
	return ethcrypto.PubkeyToAddress(key.PublicKey).Hex(), nil
}

func (s *wireCreationSigner) CreateKey(string) error { return nil }

func (s *wireCreationSigner) SignTransaction(_ context.Context, tx EVMTransaction) (string, error) {
	key, err := ethcrypto.HexToECDSA(s.keyHex)
	if err != nil {
		return "", err
	}
	var to *common.Address
	if tx.To != "" {
		address := common.HexToAddress(tx.To)
		to = &address
	}
	data, err := decodeHexBytes(tx.Data)
	if err != nil {
		return "", err
	}
	gasPrice, err := parseHexUint(tx.GasPrice)
	if err != nil {
		return "", err
	}
	unsigned := types.NewTx(&types.LegacyTx{
		Nonce: tx.Nonce, To: to, Value: new(big.Int), Gas: tx.GasLimit,
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

func TestEVMTransactorReportsHashWhenReceiptIsUnconfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x1"
		case "eth_sendRawTransaction":
			result = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "eth_getTransactionReceipt":
			result = nil
		default:
			t.Fatalf("unexpected RPC method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.EVMRPC = server.URL
	cfg.EVMChainID = 31337
	signer := &fakeEVMSigner{address: "0x1111111111111111111111111111111111111111", raw: "0x1234"}
	rpc := NewEVMRPC(server.URL)
	rpc.SetReceiptInterval(time.Millisecond)
	transactor := newEVMTransactor(cfg, rpc, signer, nil)
	transactor.receiptTimeout = 5 * time.Millisecond
	result, err := transactor.Send(context.Background(), "0x2222222222222222222222222222222222222222", []byte{1}, "")
	if err == nil {
		t.Fatal("unconfirmed receipt unexpectedly succeeded")
	}
	var unconfirmed *EVMReceiptUnconfirmedError
	if !errors.As(err, &unconfirmed) {
		t.Fatalf("error = %v, want EVMReceiptUnconfirmedError", err)
	}
	if result == nil || result.Hash != unconfirmed.Hash || unconfirmed.Hash == "" {
		t.Fatalf("result = %#v, error = %#v, want matching transaction hash", result, unconfirmed)
	}
}

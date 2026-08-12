package remote

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/crypto"
)

type remoteEVMSigner struct {
	address string
	raw     string
	txs     []chain.EVMTransaction
}

func (s *remoteEVMSigner) OwnerAddress() (string, error) { return s.address, nil }
func (s *remoteEVMSigner) CreateKey(string) error        { return nil }
func (s *remoteEVMSigner) SignTransaction(_ context.Context, tx chain.EVMTransaction) (string, error) {
	s.txs = append(s.txs, tx)
	return s.raw, nil
}

func TestDeletePushUsesEVMRegistryWithoutKubo(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Method string `json:"method"`
			ID     uint64 `json:"id"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "eth_call":
			result = "0x" + hex.EncodeToString(remoteResolvedRepoResult(owner, "demo"))
		case "eth_chainId":
			result = "0x7a69"
		case "eth_getTransactionCount":
			result = "0x4"
		case "eth_estimateGas":
			result = "0x5208"
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_sendRawTransaction":
			result = "0xhash"
		case "eth_getTransactionReceipt":
			result = map[string]string{"transactionHash": "0xhash", "status": "0x1"}
		default:
			t.Errorf("unexpected JSON-RPC method %s", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMContractAddress = cfg.ContractAddress
	cfg.EVMRPC = server.URL
	cfg.EVMChainID = 31337
	cfg.Node = server.URL
	signer := &remoteEVMSigner{address: owner, raw: "0x1234"}
	backend := chain.NewEVMRegistryV2WithDependencies(cfg, chain.NewEVMRPC(server.URL), signer)
	ipfs := &fakeIPFS{}
	h := NewHelper(
		RepoURL{Owner: owner, Repo: "demo"}, backend, ipfs, nil, nil, fakeGit{},
		strings.NewReader(""), io.Discard, io.Discard,
	)
	h.SetPushPreflight(func(needsKubo bool) error {
		if needsKubo {
			t.Fatal("deleting a ref must not require Kubo")
		}
		return nil
	})

	if err := h.cmdPushBatch("push :refs/heads/old"); err != nil {
		t.Fatalf("delete push failed: %v", err)
	}
	if ipfs.add != 0 || ipfs.swarm != 0 || ipfs.gc != 0 {
		t.Fatalf("Kubo activity = add:%d swarm:%d gc:%d, want none", ipfs.add, ipfs.swarm, ipfs.gc)
	}
	if len(signer.txs) != 1 {
		t.Fatalf("signed transactions = %d, want one deleteRef transaction", len(signer.txs))
	}
	if got := signer.txs[0].Data[2:10]; got != selectorForRemote("deleteRef(address,string,string)") {
		t.Fatalf("calldata selector = %s, want deleteRef", got)
	}
	if strings.Join(methods, ",") != "eth_call,eth_chainId,eth_getTransactionCount,eth_estimateGas,eth_gasPrice,eth_sendRawTransaction,eth_getTransactionReceipt" {
		t.Fatalf("JSON-RPC methods = %v", methods)
	}
}

func remoteResolvedRepoResult(owner, name string) []byte {
	ownerBytes, err := hex.DecodeString(strings.TrimPrefix(owner, "0x"))
	if err != nil {
		panic(err)
	}
	nameTail := remoteABIString(name)
	descriptionTail := remoteABIString("")
	branchTail := remoteABIString("main")
	tuple := make([]byte, 256)
	copy(tuple[12:32], ownerBytes)
	remotePutABIWord(tuple[32:64], 256)
	remotePutABIWord(tuple[64:96], uint64(256+len(nameTail)))
	remotePutABIWord(tuple[96:128], uint64(256+len(nameTail)+len(descriptionTail)))
	tuple[224+31] = 1
	tuple = append(tuple, nameTail...)
	tuple = append(tuple, descriptionTail...)
	tuple = append(tuple, branchTail...)

	result := make([]byte, 96)
	result[31] = 1
	result[63] = 1
	remotePutABIWord(result[64:96], 96)
	return append(result, tuple...)
}

func remoteABIString(value string) []byte {
	raw := []byte(value)
	result := make([]byte, 32+((len(raw)+31)/32)*32)
	remotePutABIWord(result[:32], uint64(len(raw)))
	copy(result[32:], raw)
	return result
}

func remotePutABIWord(word []byte, value uint64) {
	for i := 0; i < 8; i++ {
		word[31-i] = byte(value >> (8 * i))
	}
}

func selectorForRemote(signature string) string {
	digest := crypto.Keccak256([]byte(signature))
	return hex.EncodeToString(digest[:4])
}

package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestEVMImportStateReaderPinsEveryContractCall(t *testing.T) {
	const (
		contract     = "0x1111111111111111111111111111111111111111"
		owner        = "0x2222222222222222222222222222222222222222"
		updater      = "0x3333333333333333333333333333333333333333"
		collaborator = "0x4444444444444444444444444444444444444444"
		blockTag     = "0x2a"
		blockHash    = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	var callTags []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var rpcRequest rpcRequest
		if err := json.NewDecoder(request.Body).Decode(&rpcRequest); err != nil {
			t.Errorf("decode RPC request: %v", err)
			return
		}
		var result any
		switch rpcRequest.Method {
		case "eth_chainId":
			result = "0x59f"
		case "eth_getTransactionReceipt":
			result = map[string]any{
				"transactionHash": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"blockNumber":     blockTag,
				"blockHash":       blockHash,
				"to":              contract,
				"status":          "0x1",
				"logs":            []any{},
			}
		case "eth_getBlockByNumber":
			result = map[string]string{"number": blockTag, "hash": blockHash}
		case "eth_call":
			data, tag := testEthCallDataAndTag(t, rpcRequest)
			callTags = append(callTags, tag)
			var encoded []byte
			switch data[2:10] {
			case selectorFor("importProgress(bytes32)"):
				encoded = testImportProgressResult(true, false, true, []uint64{5, 5, 1, 1, 2, 0, 1, 0, 0})
			case selectorFor("getRepoById(bytes32)"):
				encoded = testRepoResult(owner, "legacy", "history", "main", 1, 2, false)
			case selectorFor("listRefsPageById(bytes32,uint256,uint256)"):
				cursor := abiUintFromCalldata(data, 1)
				if cursor == 0 {
					encoded = testRefPageResult(1, true, []string{"refs/heads/main"}, []testRefFixture{{
						commit: strings.Repeat("a", 40), packs: []string{"ipfs://a"}, updated: 1, by: updater,
					}})
				} else {
					encoded = testRefPageResult(2, false, []string{"refs/tags/v1"}, []testRefFixture{{
						commit: strings.Repeat("b", 40), packs: []string{"ipfs://b"}, updated: 2, by: updater,
					}})
				}
			case selectorFor("listCollaboratorsPageById(bytes32,uint256,uint256)"):
				encoded = testCollaboratorPageResult(1, false, []string{collaborator}, []uint64{2})
			default:
				t.Errorf("unexpected eth_call selector %s", data[2:10])
			}
			result = "0x" + hex.EncodeToString(encoded)
		default:
			t.Errorf("unexpected RPC method %s", rpcRequest.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpcRequest.ID, "result": result})
	}))
	defer server.Close()

	backend := NewEVMRegistryV2WithDependencies(config.Config{
		EVMRPC: server.URL, EVMContractAddress: contract,
	}, NewEVMRPC(server.URL), nil)
	ctx := context.Background()
	chainID, err := backend.ChainID(ctx)
	if err != nil || chainID != 1439 {
		t.Fatalf("chain ID = %d, err=%v", chainID, err)
	}
	receipt, err := backend.TransactionReceipt(ctx, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || receipt == nil || receipt.BlockHash != blockHash {
		t.Fatalf("receipt = %#v, err=%v", receipt, err)
	}
	block, err := backend.BlockByNumber(ctx, blockTag)
	if err != nil || block == nil || block.Hash != blockHash {
		t.Fatalf("block = %#v, err=%v", block, err)
	}
	var sessionID [32]byte
	sessionID[31] = 1
	progress, err := backend.ImportProgressAt(ctx, sessionID, blockTag)
	if err != nil || !progress.Finalized || progress.NextSequence != 5 || progress.ImportedRefs != 2 {
		t.Fatalf("progress = %#v, err=%v", progress, err)
	}
	var repoID [32]byte
	repoID[31] = 2
	repository, err := backend.GetRepoByIDAt(ctx, repoID, blockTag)
	if err != nil || repository.Name != "legacy" {
		t.Fatalf("repository = %#v, err=%v", repository, err)
	}
	refs, err := backend.ListRefsByIDAt(ctx, repoID, blockTag)
	if err != nil || len(refs) != 2 || refs[1].RefName != "refs/tags/v1" {
		t.Fatalf("refs = %#v, err=%v", refs, err)
	}
	collaborators, err := backend.ListCollaboratorsByIDAt(ctx, repoID, blockTag)
	if err != nil || len(collaborators) != 1 || collaborators[0].Role != "reader" {
		t.Fatalf("collaborators = %#v, err=%v", collaborators, err)
	}
	if len(callTags) != 5 {
		t.Fatalf("eth_call count = %d, want 5", len(callTags))
	}
	for index, tag := range callTags {
		if tag != blockTag {
			t.Fatalf("eth_call %d used %q, want %q", index, tag, blockTag)
		}
	}
}

func TestDecodeEVMImportProgressFailsClosed(t *testing.T) {
	valid := testImportProgressResult(true, false, true, []uint64{1, 1, 1, 1, 0, 0, 0, 0, 0})
	if _, err := decodeEVMImportProgress(valid); err != nil {
		t.Fatal(err)
	}
	invalidBool := append([]byte(nil), valid...)
	invalidBool[31] = 2
	overflow := append([]byte(nil), valid...)
	overflow[3*32] = 1
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "length", data: valid[:len(valid)-1], want: "returned"},
		{name: "bool", data: invalidBool, want: "invalid value"},
		{name: "uint overflow", data: overflow, want: "overflows uint64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeEVMImportProgress(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEVMImportStateReaderRejectsMovingAndNoncanonicalBlockTags(t *testing.T) {
	for _, blockTag := range []string{"", "latest", "safe", "0X2a", "0x02", " 0x2a", "0x2A"} {
		t.Run(strings.ReplaceAll(blockTag, " ", "_"), func(t *testing.T) {
			if err := requireFixedEVMBlockTag(blockTag); err == nil {
				t.Fatalf("block tag %q was accepted", blockTag)
			}
		})
	}
	for _, blockTag := range []string{"0x0", "0x1", "0x2a", "0xabcdef"} {
		if err := requireFixedEVMBlockTag(blockTag); err != nil {
			t.Fatalf("fixed block tag %q rejected: %v", blockTag, err)
		}
	}
}

func testImportProgressResult(exists, active, finalized bool, values []uint64) []byte {
	if len(values) != 9 {
		panic("import progress fixture requires nine counters")
	}
	result := make([]byte, 12*32)
	if exists {
		result[31] = 1
	}
	if active {
		result[63] = 1
	}
	if finalized {
		result[95] = 1
	}
	for index, value := range values {
		putABIWord(result[(index+3)*32:(index+4)*32], value)
	}
	return result
}

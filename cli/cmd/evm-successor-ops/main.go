// Command evm-successor-ops performs repo lifecycle calls against a verified
// successor suite: createRepository / resolveRepository / getRef. Testnet-only
// operator helper for S06 validation.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain/successor"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func main() {
	var directory, keyName, rpcURL string
	flags := flag.NewFlagSet("evm-successor-ops", flag.ContinueOnError)
	flags.StringVar(&directory, "directory", "", "successor SuiteDirectory address")
	flags.StringVar(&keyName, "key", "successor-op", "keystore key name")
	flags.StringVar(&rpcURL, "rpc", "https://k8s.testnet.json-rpc.injective.network", "EVM JSON-RPC endpoint")
	flags.Parse(os.Args[1:])
	if strings.TrimSpace(directory) == "" || flags.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: evm-successor-ops --directory 0x... <create|resolve|getref> ...")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	cfg.EVMSuiteDirectoryAddress = directory
	if strings.TrimSpace(keyName) != "" {
		cfg.KeyName = keyName
	}
	if strings.TrimSpace(rpcURL) != "" {
		cfg.EVMRPC = rpcURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rpc := chain.NewEVMRPC(cfg.EffectiveEVMRPC())
	info, err := chain.VerifySuccessorSuite(ctx, rpc, cfg.EffectiveEVMSuiteDirectoryAddress(), cfg.EffectiveEVMChainID())
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify successor suite:", err)
		os.Exit(1)
	}
	fmt.Printf("suite verified: version=%d chain=%d directory=%s state=%s\n", info.Version, info.ChainID, info.Directory, info.State)
	core := info.ModuleAddress("core")
	abi, err := successor.CoreABI()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	signer := chain.NewEVMKeystoreSigner(cfg)
	ownerBech32, err := signer.OwnerAddress()
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve owner:", err)
		os.Exit(1)
	}
	ownerHex, err := chain.NormalizeEVMAddress(ownerBech32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve owner:", err)
		os.Exit(1)
	}
	switch flags.Arg(0) {
	case "create":
		name := flags.Arg(1)
		data, err := abi.Pack("createRepository", name, "", "main")
		if err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
			os.Exit(1)
		}
		transactor := chain.NewEVMTransactor(cfg, rpc, signer)
		result, err := transactor.Send(ctx, core, data, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "createRepository:", err)
			os.Exit(1)
		}
		fmt.Println("created", name, "tx", result.Hash)
	case "resolve":
		view, canonical, err := resolveView(ctx, rpc, core, abi, ownerHex, flags.Arg(1))
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve:", err)
			os.Exit(1)
		}
		fmt.Printf("repoId=%#x owner=%s canonical=%v\n", view.RepoID, view.OwnerHex, canonical)
	case "getref":
		view, _, err := resolveView(ctx, rpc, core, abi, ownerHex, flags.Arg(1))
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve:", err)
			os.Exit(1)
		}
		client, err := successor.NewClient(&rawBackend{rpc: rpc, core: core})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		state, err := client.GetRef(ctx, view.RepoID, flags.Arg(2))
		if err != nil {
			fmt.Fprintln(os.Stderr, "getRef:", err)
			os.Exit(1)
		}
		fmt.Printf("revision=%d digest=%x size=%d locator=%s updatedBy=%s\n",
			state.Revision, state.Commitment.ManifestDigest, state.Commitment.ManifestSize,
			state.Commitment.BootstrapLocator, state.UpdatedBy.Hex())
	default:
		fmt.Fprintln(os.Stderr, "unknown op", flags.Arg(0))
		os.Exit(2)
	}
}

func resolveView(ctx context.Context, rpc *chain.EVMRPC, core string, abi gethabi.ABI, ownerHex, name string) (successor.RepositoryView, bool, error) {
	data, err := abi.Pack("resolveRepository", common.HexToAddress(ownerHex), name)
	if err != nil {
		return successor.RepositoryView{}, false, err
	}
	result, err := rpc.CallContractAt(ctx, core, "0x"+hex.EncodeToString(data), "latest")
	if err != nil {
		return successor.RepositoryView{}, false, err
	}
	values, err := abi.Unpack("resolveRepository", result)
	if err != nil || len(values) != 2 {
		return successor.RepositoryView{}, false, fmt.Errorf("decode resolveRepository: %v", err)
	}
	type repoTuple struct {
		Id            [32]byte
		Owner         common.Address
		Name          string
		Description   string
		DefaultBranch string
		ForkedFrom    [32]byte
		CreatedAt     uint64
		UpdatedAt     uint64
		Exists        bool
	}
	converted := gethabi.ConvertType(values[0], new(repoTuple))
	repo, _ := converted.(*repoTuple)
	if repo == nil || !repo.Exists {
		return successor.RepositoryView{}, false, fmt.Errorf("repository %s not found", name)
	}
	canonical, _ := values[1].(bool)
	return successor.RepositoryView{RepoID: repo.Id, OwnerHex: repo.Owner.Hex(), Name: repo.Name, DefaultBranch: repo.DefaultBranch}, canonical, nil
}

type rawBackend struct {
	rpc  *chain.EVMRPC
	core string
}

func (b *rawBackend) Call(ctx context.Context, calldata []byte) ([]byte, error) {
	return b.rpc.CallContractAt(ctx, b.core, "0x"+hex.EncodeToString(calldata), "latest")
}

func (b *rawBackend) Send(ctx context.Context, calldata []byte) (string, error) {
	return "", fmt.Errorf("rawBackend is read-only")
}

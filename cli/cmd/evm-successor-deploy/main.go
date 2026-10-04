// Command evm-successor-deploy deploys the storage-neutral successor suite
// (contracts/evm-v2-successor) to Injective testnet for S06 validation. It
// reuses the same no-clobber deployment evidence flow as igit-deploy-suite but
// verifies the successor artifact directory and records the working-tree
// source state (successor sources are not yet committed). Testnet only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitedeploy"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var artifacts, output, network, rpcOverride, keyName, snapshotRoot, confirmation, sourceLabel string
	var recoveryInput, recoveryOperator, blockscoutAPI, recoveryEvidence string
	var platformFeeBPS uint
	flags := flag.NewFlagSet("evm-successor-deploy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&artifacts, "artifacts", "contracts/evm-v2-successor/artifacts", "checked-in successor artifact directory")
	flags.StringVar(&output, "output", "", "exclusive deployment.json evidence path")
	flags.StringVar(&network, "network", "injective-testnet", "named Injective network profile")
	flags.StringVar(&rpcOverride, "rpc", "", "operator-only EVM JSON-RPC override")
	flags.StringVar(&keyName, "key", "", "rotated encrypted EVM bootstrap key name (defaults to config)")
	flags.StringVar(&snapshotRoot, "snapshot-root", "", "0x-prefixed snapshot root recorded in evidence")
	flags.StringVar(&sourceLabel, "source-label", "", "working-tree source label recorded in evidence")
	flags.StringVar(&confirmation, "confirm-source-label", "", "exact source label required before any broadcast")
	flags.UintVar(&platformFeeBPS, "platform-fee-bps", 300, "initial platform fee in basis points (maximum 500)")
	flags.StringVar(&recoveryEvidence, "recover-from-evidence", "", "existing deployment evidence serving historical transactions without Blockscout")
	flags.StringVar(&recoveryInput, "recover-transactions", "", "read-only historical deployment transactions (recovery input JSON)")
	flags.StringVar(&recoveryOperator, "operator", "", "operator address required with --recover-transactions")
	flags.StringVar(&blockscoutAPI, "blockscout-api", "", "credential-free Blockscout API base required with --recover-transactions")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if strings.TrimSpace(output) == "" || strings.TrimSpace(snapshotRoot) == "" || strings.TrimSpace(sourceLabel) == "" {
		fmt.Fprintln(stderr, "deployment requires --output, --snapshot-root and --source-label")
		return 2
	}
	if confirmation != sourceLabel {
		fmt.Fprintf(stderr, "broadcast confirmation missing; review artifacts, then pass --confirm-source-label %s\n", sourceLabel)
		return 2
	}
	if platformFeeBPS > 500 {
		fmt.Fprintln(stderr, "--platform-fee-bps cannot exceed 500")
		return 2
	}
	profile, ok := config.NetworkProfileFor(network)
	if !ok || profile.Name != "injective-testnet" {
		fmt.Fprintln(stderr, "successor demo deployment is restricted to injective-testnet")
		return 2
	}
	inspection, err := suitedeploy.InspectArtifacts(filepath.Clean(artifacts))
	if err != nil {
		fmt.Fprintf(stderr, "inspect successor artifacts: %v\n", err)
		return 1
	}
	data, _ := json.MarshalIndent(inspection, "", "  ")
	fmt.Fprintln(stdout, string(data))

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	if strings.TrimSpace(keyName) != "" {
		cfg.KeyName = keyName
	}
	endpoint := strings.TrimSpace(rpcOverride)
	if endpoint == "" {
		endpoint = profile.EVMRPC
	}
	rpc := chain.NewEVMRPC(endpoint)
	signer := chain.NewEVMKeystoreSigner(cfg)
	operator, err := signer.OwnerAddress()
	if err != nil {
		fmt.Fprintf(stderr, "resolve operator address: %v\n", err)
		return 1
	}
	transactor := chain.NewEVMTransactor(cfg, rpc, signer)
	if strings.TrimSpace(recoveryInput) != "" {
		if strings.TrimSpace(recoveryOperator) == "" || strings.TrimSpace(blockscoutAPI) == "" {
			fmt.Fprintln(stderr, "historical recovery requires --operator and --blockscout-api")
			return 2
		}
		input, err := suitedeploy.LoadRecoveryInput(filepath.Clean(recoveryInput))
		if err != nil {
			fmt.Fprintf(stderr, "load historical deployment transactions: %v\n", err)
			return 1
		}
		var source suitedeploy.HistoricalTransactionSource
		if strings.TrimSpace(recoveryEvidence) != "" {
			source, err = newEvidenceTransactionSource(recoveryEvidence, rpc)
			if err != nil {
				fmt.Fprintf(stderr, "load evidence transaction source: %v\n", err)
				return 1
			}
		} else {
			bs, bsErr := suitedeploy.NewBlockscoutTransactionSource(blockscoutAPI)
			if bsErr != nil {
				fmt.Fprintf(stderr, "configure historical transaction source: %v\n", bsErr)
				return 2
			}
			source = bs
		}
		if err != nil {
			return 2
		}
		manifest, err := suitedeploy.Recover(ctx, suitedeploy.Options{
			ArtifactDirectory: filepath.Clean(artifacts),
			SourceCommit:      sourceLabel,
			SuiteVersion:      4,
			VerifyAtLatest:    true,
			Network:           profile.Name,
			ChainID:           profile.EVMChainID,
			RPCEndpoint:       endpoint,
			BlockExplorer:     profile.EVMExplorer,
			SnapshotRoot:      snapshotRoot,
			Operator:          recoveryOperator,
			PlatformFeeBPS:    uint16(platformFeeBPS),
		}, rpc, source, input, filepath.Clean(output))
		if err != nil {
			fmt.Fprintf(stderr, "recover successor deployment evidence: %v\n", err)
			return 1
		}
		summary, _ := json.MarshalIndent(manifest, "", "  ")
		fmt.Fprintln(stdout, string(summary))
		return 0
	}
	manifest, err := suitedeploy.Deploy(ctx, suitedeploy.Options{
		ArtifactDirectory: filepath.Clean(artifacts),
		SuiteVersion:      4,
		VerifyAtLatest:    true,
		SourceCommit:      sourceLabel,
		Network:           profile.Name,
		ChainID:           profile.EVMChainID,
		RPCEndpoint:       endpoint,
		BlockExplorer:     profile.EVMExplorer,
		SnapshotRoot:      snapshotRoot,
		Operator:          operator,
		PlatformFeeBPS:    uint16(platformFeeBPS),
	}, rpc, transactor, filepath.Clean(output))
	if err != nil {
		fmt.Fprintf(stderr, "deploy successor suite: %v\n", err)
		fmt.Fprintf(stderr, "inspect the no-clobber evidence file before any retry: %s\n", output)
		return 1
	}
	summary, _ := json.MarshalIndent(manifest, "", "  ")
	fmt.Fprintln(stdout, string(summary))
	fmt.Fprintln(stdout, "successor suite deployed; inspect the evidence file for contract addresses")
	return 0
}

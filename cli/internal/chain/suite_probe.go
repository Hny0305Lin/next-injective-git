package chain

import (
	"context"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// ProbeSuiteInfo verifies the configured SuiteDirectory without requiring one
// specific version, so callers can dispatch: Version 3 keeps the legacy IPFS
// path, Version 4 selects the storage-neutral successor path, anything else
// fails closed. It returns nil when no directory is configured.
func ProbeSuiteInfo(cfg config.Config) (*SuiteInfo, error) {
	directory := strings.TrimSpace(cfg.EffectiveEVMSuiteDirectoryAddress())
	if directory == "" {
		return nil, nil
	}
	rpc := NewEVMRPC(cfg.EffectiveEVMRPC())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return VerifyAnyKnownSuite(ctx, rpc, directory, cfg.EffectiveEVMChainID())
}

// git-remote-igit is the git remote helper for the igit:// transport. Git
// invokes it by convention as git-remote-<scheme>, i.e. git-remote-igit for
// igit:// URLs. It must be on PATH for `git`/`igit` clone & push to work.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/byos"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/environment"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/remote"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/replication"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "git-remote-igit: %v\n", err)
		os.Exit(1)
	}
}

// validateContractSelection keeps the remote-helper preflight decision
// backend-specific. It is intentionally side-effect free so the same rule can
// be covered without starting a Git remote-helper conversation.
func validateContractSelection(cfg config.Config) error {
	return cfg.ValidateContract()
}

func run() error {
	if len(os.Args) < 3 {
		return i18n.Errorf("usage: git-remote-igit <remote-name> <url>", "用法：git-remote-igit <remote-name> <url>")
	}
	repoURL, err := remote.ParseURL(os.Args[2])
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := validateContractSelection(cfg); err != nil {
		return err
	}

	// Storage-neutral dispatch: verify the configured suite once. Version 3
	// keeps the legacy IPFS flow below untouched; version 4 selects the BYOS
	// successor path and never initializes Kubo, gateways or replication.
	var byosService *byos.Service
	suiteInfo, err := chain.ProbeSuiteInfo(cfg)
	if err != nil {
		return err
	}
	if suiteInfo != nil && suiteInfo.Version == chain.SuccessorSuiteVersion {
		if !repoURL.OwnerIsAddress() {
			return i18n.Errorf(
				"the successor BYOS path requires an address owner; resolve the username first",
				"successor BYOS 路径要求地址 owner；请先解析用户名")
		}
		if strings.TrimSpace(cfg.StorageConfig) == "" {
			return i18n.Errorf(
				"successor suite is configured but no storage profile is registered; run: igit storage add <storage-config.json>",
				"已配置 successor 套件但未登记 storage 配置；请运行：igit storage add <storage-config.json>")
		}
		profiles, err := storageconfig.Load(cfg.StorageConfig)
		if err != nil {
			return err
		}
		registry, err := chain.NewSuccessorRegistry(cfg)
		if err != nil {
			return err
		}
		gitRepo, err := gitio.FromEnv()
		if err != nil {
			return err
		}
		byosService, err = byos.NewService(registry, gitRepo, byos.ProfileStoreFactory{Profiles: profiles, Chain: registry}, nil)
		if err != nil {
			return err
		}
	}

	gitRepo, err := gitio.FromEnv()
	if err != nil {
		return err
	}
	cc, err := chain.NewRegistryBackend(cfg)
	if err != nil {
		return err
	}

	var ic *ipfs.Client
	var replicationAuth replication.Authorizer
	if byosService == nil {
		// URLs may carry a registered username instead of a bech32 address (§4)
		if !repoURL.OwnerIsAddress() {
			owner, err := cc.ResolveUsername(repoURL.Owner)
			if err != nil {
				return err
			}
			if quiet, _ := remote.EnvVerbosityOverrides(); !quiet {
				fmt.Fprintf(os.Stderr, "git-remote-igit: "+i18n.Text("%s -> %s\n", "%s -> %s（已解析）\n"), repoURL.Owner, owner)
			}
			repoURL.Owner = owner
		}
		gateways, health := ipfs.SelectGateways(context.Background(), cfg.EffectiveGateways())
		urls := make([]string, 0, len(gateways))
		for _, gateway := range gateways {
			urls = append(urls, gateway.URL)
		}
		urls = append(urls, cfg.EffectiveReadFallbacks()...)
		ic = ipfs.NewWithGateways(cfg.IPFSAPI, urls)
		if len(gateways) > 0 && remote.EnvVerboseOverride() {
			for _, result := range health {
				if result.Err == nil && result.Gateway.URL == gateways[0].URL {
					fmt.Fprintf(os.Stderr, "git-remote-igit: "+i18n.Text("gateway %s selected (%s)\n", "已选择网关 %s（%s）\n"), result.Gateway.Name, result.Latency.Round(time.Millisecond))
					break
				}
			}
		}
		replicationAuth = replication.NewDynamic(cfg.Upload.Endpoint, cfg.Upload.AuthorizationEndpoint, cfg.Upload.Authorization)
	}

	helper := remote.NewHelper(
		repoURL,
		cc,
		ic,
		replicationAuth,
		cfg.EffectiveUploadPeers(),
		gitRepo,
		os.Stdin, os.Stdout, os.Stderr,
	)
	helper.SetByosStorage(byosService)
	helper.SetPushPreflight(func(needsKubo bool) error {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return environment.PushPreflight(ctx, cfg, needsKubo)
	})
	return helper.Run()
}

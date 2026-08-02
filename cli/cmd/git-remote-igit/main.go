// git-remote-igit is the git remote helper for the igit:// transport. Git
// invokes it by convention as git-remote-<scheme>, i.e. git-remote-igit for
// igit:// URLs. It must be on PATH for `git`/`igit` clone & push to work.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/remote"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/replication"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "git-remote-igit: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: git-remote-igit <remote-name> <url>")
	}
	repoURL, err := remote.ParseURL(os.Args[2])
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.ContractAddress == "" {
		return fmt.Errorf("contract_address not configured (run `igit config set contract_address <addr>`)")
	}
	cc := chain.New(cfg)
	// URLs may carry a registered username instead of a bech32 address (§4)
	if !repoURL.OwnerIsAddress() {
		owner, err := cc.ResolveUsername(repoURL.Owner)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "git-remote-igit: %s -> %s\n", repoURL.Owner, owner)
		repoURL.Owner = owner
	}
	gitRepo, err := gitio.FromEnv()
	if err != nil {
		return err
	}
	gateways, health := ipfs.SelectGateways(context.Background(), cfg.EffectiveGateways())
	urls := make([]string, 0, len(gateways))
	for _, gateway := range gateways {
		urls = append(urls, gateway.URL)
	}
	urls = append(urls, cfg.EffectiveReadFallbacks()...)
	ic := ipfs.NewWithGateways(cfg.IPFSAPI, urls)
	if len(gateways) > 0 {
		for _, result := range health {
			if result.Err == nil && result.Gateway.URL == gateways[0].URL {
				fmt.Fprintf(os.Stderr, "git-remote-igit: gateway %s selected (%s)\n", result.Gateway.Name, result.Latency.Round(time.Millisecond))
				break
			}
		}
	}
	helper := remote.NewHelper(
		repoURL,
		cc,
		ic,
		replication.New(cfg.Upload.Endpoint, cfg.Upload.Authorization),
		cfg.Upload.USPeer,
		gitRepo,
		os.Stdin, os.Stdout, os.Stderr,
	)
	return helper.Run()
}

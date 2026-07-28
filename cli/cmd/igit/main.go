// igit is the Next Injective Git companion CLI: repo creation, key helpers and
// configuration management. Pushing/fetching is done by git itself through
// the git-remote-inj helper.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

const usage = `igit - Next Injective Git (Injective + IPFS)

Usage:
  igit init <name> [description]       create an on-chain repository
  igit clone-url <name>                print the inj:// URL of your repo
  igit repos [owner]                   list repositories of an owner
  igit refs <owner> <repo>             list on-chain refs of a repository
  igit key show                        show the configured signing address
  igit key new <name>                  create a key in the injectived keyring
  igit config list                     show current configuration
  igit config set <key> <value>        set a configuration value
  igit version                         print version

Config keys:
  contract_address chain_id lcd_endpoint node key_name keyring_backend
  injectived_bin gas_prices ipfs_api ipfs_gateway
`

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "igit: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return cmdInit(cfg, args[1:])
	case "clone-url":
		return cmdCloneURL(cfg, args[1:])
	case "repos":
		return cmdRepos(cfg, args[1:])
	case "refs":
		return cmdRefs(cfg, args[1:])
	case "key":
		return cmdKey(cfg, args[1:])
	case "config":
		return cmdConfig(cfg, args[1:])
	case "version":
		fmt.Println("igit", version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q (see `igit help`)", args[0])
	}
}

func cmdInit(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit init <name> [description]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	name := args[0]
	description := strings.Join(args[1:], " ")
	cc := chain.New(cfg)
	if err := cc.CreateRepo(name, description, "main"); err != nil {
		return err
	}
	owner, err := cc.OwnerAddress()
	if err != nil {
		return fmt.Errorf("repo created but failed to resolve address: %w", err)
	}
	fmt.Printf("repository created on chain.\n\n")
	fmt.Printf("add it as a git remote:\n")
	fmt.Printf("  git remote add inj inj://%s/%s\n", owner, name)
	fmt.Printf("  git push inj main\n")
	return nil
}

func cmdCloneURL(cfg config.Config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: igit clone-url <name>")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	owner, err := chain.New(cfg).OwnerAddress()
	if err != nil {
		return err
	}
	fmt.Printf("inj://%s/%s\n", owner, args[0])
	return nil
}

func cmdRepos(cfg config.Config, args []string) error {
	cc := chain.New(cfg)
	owner := ""
	if len(args) > 0 {
		owner = args[0]
	} else {
		var err error
		if owner, err = cc.OwnerAddress(); err != nil {
			return fmt.Errorf("no owner given and cannot resolve local key: %w", err)
		}
	}
	query := map[string]any{
		"list_repos": map[string]any{"owner": owner, "limit": 100},
	}
	var out struct {
		Repos []chain.RepoInfo `json:"repos"`
	}
	if err := cc.SmartQuery(query, &out); err != nil {
		return err
	}
	if len(out.Repos) == 0 {
		fmt.Printf("no repositories for %s\n", owner)
		return nil
	}
	for _, r := range out.Repos {
		fmt.Printf("%-32s default:%-12s %s\n", r.Name, r.DefaultBranch, r.Description)
	}
	return nil
}

func cmdRefs(cfg config.Config, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: igit refs <owner> <repo>")
	}
	refs, err := chain.New(cfg).ListRefs(args[0], args[1])
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		fmt.Println("no refs (empty repository)")
		return nil
	}
	for _, r := range refs {
		fmt.Printf("%s %-40s packfiles:%d\n", r.CommitSha, r.RefName, len(r.PackfileCids))
	}
	return nil
}

func cmdKey(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: igit key <show|new> [name]")
	}
	bin := cfg.InjectivedBin
	if bin == "" {
		bin = "injectived"
	}
	switch args[0] {
	case "show":
		if cfg.KeyName == "" {
			return fmt.Errorf("key_name not configured")
		}
		addr, err := chain.New(cfg).OwnerAddress()
		if err != nil {
			return err
		}
		fmt.Printf("key:     %s\naddress: %s\n", cfg.KeyName, addr)
		return nil
	case "new":
		if len(args) != 2 {
			return fmt.Errorf("usage: igit key new <name>")
		}
		// delegate to injectived so the mnemonic/key never touches igit state
		cmd := exec.Command(bin, "keys", "add", args[1], "--keyring-backend", cfg.KeyringBackend)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		cfg.KeyName = args[1]
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("\nkey_name set to %q in igit config.\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown key subcommand %q", args[0])
	}
}

func cmdConfig(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: igit config <list|set> ...")
	}
	switch args[0] {
	case "list":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		path, _ := config.Path()
		fmt.Printf("# %s\n%s\n", path, data)
		return nil
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("usage: igit config set <key> <value>")
		}
		if err := setConfigField(&cfg, args[1], args[2]); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n", args[1], args[2])
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func setConfigField(cfg *config.Config, key, value string) error {
	switch key {
	case "contract_address":
		cfg.ContractAddress = value
	case "chain_id":
		cfg.ChainID = value
	case "lcd_endpoint":
		cfg.LCDEndpoint = value
	case "node":
		cfg.Node = value
	case "key_name":
		cfg.KeyName = value
	case "keyring_backend":
		cfg.KeyringBackend = value
	case "injectived_bin":
		cfg.InjectivedBin = value
	case "gas_prices":
		cfg.GasPrices = value
	case "ipfs_api":
		cfg.IPFSAPI = value
	case "ipfs_gateway":
		cfg.IPFSGateway = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

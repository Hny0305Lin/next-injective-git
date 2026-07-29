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
  igit collab add <repo> <address> [maintainer|reader]
                                       grant a collaborator role (owner only)
  igit collab remove <repo> <address>  remove a collaborator (owner only)
  igit collab list <owner> <repo>      list collaborators of a repository
  igit transfer <repo> <new-owner>     transfer repository ownership
  igit repo edit <repo> description <text...>
  igit repo edit <repo> branch <name>  update repo metadata (owner only)
  igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]
                                       set moderation status (committee/admin)
  igit key show                        show the configured signing address
  igit key new <name>                  create a key in the injectived keyring
  igit config list                     show current configuration
  igit config set <key> <value>        set a configuration value
  igit version                         print version

Config keys:
  contract_address chain_id lcd_endpoint node key_name keyring_backend
  injectived_bin gas_prices ipfs_api ipfs_gateway
`

const version = "0.2.0"

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
	case "collab":
		return cmdCollab(cfg, args[1:])
	case "transfer":
		return cmdTransfer(cfg, args[1:])
	case "repo":
		return cmdRepo(cfg, args[1:])
	case "mod":
		return cmdMod(cfg, args[1:])
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
		fmt.Printf("%s %-40s packfiles:%d\n", r.CommitSha, r.RefName, len(r.PackURIs))
	}
	return nil
}

func cmdCollab(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit collab <add|remove|list> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: igit collab add <repo> <address> [maintainer|reader]")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		role := "maintainer"
		if len(args) > 3 {
			role = args[3]
		}
		if role != "maintainer" && role != "reader" {
			return fmt.Errorf("invalid role %q (maintainer|reader)", role)
		}
		if err := cc.SetCollaborator(args[1], args[2], role); err != nil {
			return err
		}
		fmt.Printf("collaborator %s added to %s as %s\n", args[2], args[1], role)
		return nil
	case "remove":
		if len(args) != 3 {
			return fmt.Errorf("usage: igit collab remove <repo> <address>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.SetCollaborator(args[1], args[2], ""); err != nil {
			return err
		}
		fmt.Printf("collaborator %s removed from %s\n", args[2], args[1])
		return nil
	case "list":
		if len(args) != 3 {
			return fmt.Errorf("usage: igit collab list <owner> <repo>")
		}
		collabs, err := cc.ListCollaborators(args[1], args[2])
		if err != nil {
			return err
		}
		if len(collabs) == 0 {
			fmt.Println("no collaborators")
			return nil
		}
		for _, c := range collabs {
			fmt.Printf("%-12s %s\n", c.Role, c.Address)
		}
		return nil
	default:
		return fmt.Errorf("unknown collab subcommand %q", args[0])
	}
}

func cmdTransfer(cfg config.Config, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: igit transfer <repo> <new-owner>")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(args[1], "inj1") {
		return fmt.Errorf("new owner %q must be an inj1... bech32 address", args[1])
	}
	if err := chain.New(cfg).TransferOwnership(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("ownership of %s transferred to %s\n", args[0], args[1])
	fmt.Printf("new clone URL: inj://%s/%s\n", args[1], args[0])
	return nil
}

func cmdRepo(cfg config.Config, args []string) error {
	if len(args) < 4 || args[0] != "edit" {
		return fmt.Errorf("usage: igit repo edit <repo> <description|branch> <value...>")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	repo := args[1]
	var description, branch *string
	switch args[2] {
	case "description":
		d := strings.Join(args[3:], " ")
		description = &d
	case "branch":
		branch = &args[3]
	default:
		return fmt.Errorf("unknown field %q (description|branch)", args[2])
	}
	if err := chain.New(cfg).UpdateRepoInfo(repo, description, branch); err != nil {
		return err
	}
	fmt.Printf("repo %s updated\n", repo)
	return nil
}

func cmdMod(cfg config.Config, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	status := args[2]
	switch status {
	case "active", "delisted", "frozen":
	default:
		return fmt.Errorf("invalid status %q (active|delisted|frozen)", status)
	}
	reason := ""
	if len(args) > 3 {
		reason = args[3]
	}
	if err := chain.New(cfg).SetModerationStatus(args[0], args[1], status, reason); err != nil {
		return err
	}
	fmt.Printf("%s/%s moderation status set to %s\n", args[0], args[1], status)
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

// igit is the Next Injective Git companion CLI: repo creation, key helpers and
// configuration management. Pushing/fetching is done by git itself through
// the git-remote-igit helper.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
)

const usage = `igit - Next Injective Git (Injective + IPFS)

Usage:
  igit init <name> [description]       create an on-chain repository
  igit init [-b <branch>] [.]          local git init passthrough (flags/no name)
  igit import <github-url> [name]      mirror a GitHub repo onto the chain
  igit clone <owner>/<repo> [dir]      clone a repo (igit://owner/repo also ok)
  igit push [remote] [refspec...]      push current repo on-chain (wraps git)
  igit pull [remote] [refspec...]      pull from chain (wraps git)
  igit clone-url <name>                print the igit:// URL of your repo
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
  igit sponsor <owner> <repo> <inj-amount> [message...]
                                       sponsor a repository (e.g. 0.5 INJ)
  igit fork <owner> <repo> [new-name]  fork a repository into your namespace
  igit badge award <repo> <recipient> <reason...>
                                       award a contribution badge (owner only)
  igit badge list [address|username]   show a contributor's trophy wall
  igit splits set <repo> [addr:bps]... set revenue splits (owner only)
  igit splits show <owner> <repo>      show revenue splits
  igit username register <name>        claim a username (locks deposit)
  igit username release                release username, refund deposit
  igit username show [name|address]    resolve a username / reverse lookup
  igit key show                        show the configured signing address
  igit key new <name>                  create a key in the injectived keyring
  igit gateway status                  probe HK/US read-only gateway health
  igit gateway select                  print the automatically selected order
  igit config list                     show current configuration
  igit config set <key> <value>        set a configuration value
  igit version                         print version

Config keys:
  contract_address chain_id lcd_endpoint node key_name keyring_backend
  injectived_bin gas_prices ipfs_api ipfs_gateway
  upload.endpoint upload.authorization upload.us_peer

Any other subcommand (add, commit, remote, status, log, branch, ...) is
forwarded to git, so the whole workflow can stay inside igit. Commands
above shadow git ones of the same name (e.g. use ` + "`git config`" + ` for git's).
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
	case "import":
		return cmdImport(cfg, args[1:])
	case "clone":
		return cmdClone(cfg, args[1:])
	case "push":
		return cmdGitPassthrough("push", args[1:])
	case "pull":
		return cmdGitPassthrough("pull", args[1:])
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
	case "sponsor":
		return cmdSponsor(cfg, args[1:])
	case "fork":
		return cmdFork(cfg, args[1:])
	case "badge":
		return cmdBadge(cfg, args[1:])
	case "splits":
		return cmdSplits(cfg, args[1:])
	case "username":
		return cmdUsername(cfg, args[1:])
	case "key":
		return cmdKey(cfg, args[1:])
	case "gateway":
		return cmdGateway(cfg, args[1:])
	case "config":
		return cmdConfig(cfg, args[1:])
	case "version":
		fmt.Println("igit", version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		// Anything igit does not know is forwarded to git so the whole
		// workflow (add/commit/remote/status/...) stays inside igit.
		return runGit(args)
	}
}

func cmdGateway(cfg config.Config, args []string) error {
	if len(args) != 1 || (args[0] != "status" && args[0] != "select") {
		return fmt.Errorf("usage: igit gateway <status|select>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	selected, health := ipfs.SelectGateways(ctx, cfg.EffectiveGateways())
	if args[0] == "status" {
		for _, result := range health {
			if result.Err != nil {
				fmt.Printf("%-8s down  %-36s %v\n", result.Gateway.Name, result.Gateway.URL, result.Err)
				continue
			}
			fmt.Printf("%-8s ok    %-36s %s\n", result.Gateway.Name, result.Gateway.URL, result.Latency.Round(time.Millisecond))
		}
		return nil
	}
	for i, gateway := range selected {
		fmt.Printf("%d  %-8s %s\n", i+1, gateway.Name, gateway.URL)
	}
	return nil
}

func cmdInit(cfg config.Config, args []string) error {
	// `igit init`, `igit init -b main`, `igit init .` are local git init
	// passthroughs; a plain name keeps the on-chain create semantics.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || args[0] == "." {
		return runGit(append([]string{"init"}, args...))
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
	fmt.Printf("  igit remote add inj igit://%s/%s\n", owner, name)
	fmt.Printf("  igit push inj main\n")
	return nil
}

// cmdClone wraps `git clone`, accepting a bare "owner/repo" (turned into an
// igit:// URL) or any full igit:// / inj:// URL.
func cmdClone(_ config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit clone <owner>/<repo> [dir]")
	}
	url := args[0]
	if !strings.Contains(url, "://") && !strings.Contains(url, "::") {
		url = "igit://" + strings.TrimPrefix(url, "/")
	}
	gitArgs := append([]string{"clone", url}, args[1:]...)
	return runGit(gitArgs)
}

// cmdGitPassthrough forwards a subcommand (push/pull) straight to git in the
// current repository, so users can drive everything through `igit`.
func cmdGitPassthrough(sub string, args []string) error {
	return runGit(append([]string{sub}, args...))
}

// runGit execs the local git, inheriting stdio so prompts/progress pass through.
func runGit(args []string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitIn runs git in a specific directory and returns trimmed stdout.
func runGitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// runGitInIO runs git in a directory with inherited stdio (for push progress).
func runGitInIO(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// normalizeGitHub turns various GitHub spellings into a cloneable https URL.
//
//	github.com/user/repo, https://github.com/user/repo(.git), user/repo
func normalizeGitHub(src string) (cloneURL, repoName string, err error) {
	s := strings.TrimSuffix(src, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimPrefix(s, "github.com:")
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse GitHub source %q (expected github.com/user/repo)", src)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]), parts[1], nil
}

// cmdImport mirrors a GitHub repository onto the chain: bare-clone the source,
// create the on-chain repo, then push every branch and tag through igit.
func cmdImport(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit import <github-url> [name]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cloneURL, defaultName, err := normalizeGitHub(args[0])
	if err != nil {
		return err
	}
	name := defaultName
	if len(args) > 1 {
		name = args[1]
	}

	tmp, err := os.MkdirTemp("", "igit-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("cloning %s ...\n", cloneURL)
	if err := runGit([]string{"clone", "--bare", "--quiet", cloneURL, tmp}); err != nil {
		return fmt.Errorf("git clone %s: %w", cloneURL, err)
	}

	// default branch = the source's HEAD (main / master / ...)
	branch, err := runGitIn(tmp, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch == "" {
		branch = "main"
	}

	cc := chain.New(cfg)
	fmt.Printf("creating on-chain repo %q (default branch %q) ...\n", name, branch)
	if err := cc.CreateRepo(name, "imported from "+cloneURL, branch); err != nil {
		return err
	}
	owner, err := cc.OwnerAddress()
	if err != nil {
		return fmt.Errorf("repo created but failed to resolve address: %w", err)
	}

	remote := fmt.Sprintf("igit://%s/%s", owner, name)
	if _, err := runGitIn(tmp, "remote", "add", "igit", remote); err != nil {
		return fmt.Errorf("add igit remote: %w", err)
	}
	fmt.Printf("pushing all branches to %s ...\n", remote)
	if err := runGitInIO(tmp, "push", "igit", "--all"); err != nil {
		return fmt.Errorf("push branches: %w", err)
	}
	// tags are best-effort: a repo may have none
	if tags, _ := runGitIn(tmp, "tag"); tags != "" {
		fmt.Printf("pushing tags ...\n")
		if err := runGitInIO(tmp, "push", "igit", "--tags"); err != nil {
			return fmt.Errorf("push tags: %w", err)
		}
	}

	fmt.Printf("\nimported! your mirror is live:\n")
	fmt.Printf("  igit clone %s\n", remote)
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
	fmt.Printf("igit://%s/%s\n", owner, args[0])
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
	fmt.Printf("new clone URL: igit://%s/%s\n", args[1], args[0])
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

// resolveOwner turns a username into its address; addresses pass through.
func resolveOwner(cc *chain.Client, owner string) (string, error) {
	if strings.HasPrefix(owner, "inj1") {
		return owner, nil
	}
	return cc.ResolveUsername(owner)
}

// parseINJ converts a decimal INJ amount ("0.5") into base units ("5...0inj").
func parseINJ(s string) (string, error) {
	whole, frac, _ := strings.Cut(strings.TrimSuffix(s, "inj"), ".")
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 18 {
		return "", fmt.Errorf("amount %q has more than 18 decimal places", s)
	}
	frac += strings.Repeat("0", 18-len(frac))
	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("invalid INJ amount %q", s)
		}
	}
	base := strings.TrimLeft(whole+frac, "0")
	if base == "" {
		return "", fmt.Errorf("amount must be positive")
	}
	return base + "inj", nil
}

func cmdSponsor(cfg config.Config, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: igit sponsor <owner> <repo> <inj-amount> [message...]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cc := chain.New(cfg)
	owner, err := resolveOwner(cc, args[0])
	if err != nil {
		return err
	}
	amount, err := parseINJ(args[2])
	if err != nil {
		return err
	}
	message := strings.Join(args[3:], " ")
	if err := cc.Sponsor(owner, args[1], message, amount); err != nil {
		return err
	}
	fmt.Printf("sponsored %s/%s with %s INJ — thank you!\n", args[0], args[1], args[2])
	return nil
}

func cmdFork(cfg config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: igit fork <owner> <repo> [new-name]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cc := chain.New(cfg)
	owner, err := resolveOwner(cc, args[0])
	if err != nil {
		return err
	}
	newName := ""
	if len(args) > 2 {
		newName = args[2]
	}
	if err := cc.ForkRepo(owner, args[1], newName); err != nil {
		return err
	}
	self, err := cc.OwnerAddress()
	if err != nil {
		return err
	}
	if newName == "" {
		newName = args[1]
	}
	fmt.Printf("forked %s/%s\n", args[0], args[1])
	fmt.Printf("clone your fork: git clone igit://%s/%s\n", self, newName)
	return nil
}

func cmdBadge(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit badge <award|list> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "award":
		if len(args) < 4 {
			return fmt.Errorf("usage: igit badge award <repo> <recipient> <reason...>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		recipient, err := resolveOwner(cc, args[2])
		if err != nil {
			return err
		}
		reason := strings.Join(args[3:], " ")
		if err := cc.AwardBadge(args[1], recipient, reason); err != nil {
			return err
		}
		fmt.Printf("badge awarded to %s for %q\n", args[2], reason)
		return nil
	case "list":
		target := ""
		if len(args) > 1 {
			var err error
			if target, err = resolveOwner(cc, args[1]); err != nil {
				return err
			}
		} else {
			var err error
			if target, err = cc.OwnerAddress(); err != nil {
				return err
			}
		}
		badges, err := cc.BadgesByRecipient(target)
		if err != nil {
			return err
		}
		if len(badges) == 0 {
			fmt.Println("no badges yet")
			return nil
		}
		for _, b := range badges {
			fmt.Printf("#%-4d %s/%s: %q\n", b.ID, b.RepoOwner[:12]+"…", b.RepoName, b.Reason)
		}
		return nil
	default:
		return fmt.Errorf("unknown badge subcommand %q", args[0])
	}
}

func cmdSplits(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit splits <set|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: igit splits set <repo> [address:bps]...")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		var splits []chain.SplitEntry
		for _, spec := range args[2:] {
			addr, bpsStr, ok := strings.Cut(spec, ":")
			if !ok {
				return fmt.Errorf("invalid split %q (expected address:bps)", spec)
			}
			bps, err := strconv.ParseUint(bpsStr, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid bps in %q: %w", spec, err)
			}
			resolved, err := resolveOwner(cc, addr)
			if err != nil {
				return err
			}
			splits = append(splits, chain.SplitEntry{Address: resolved, Bps: uint16(bps)})
		}
		if err := cc.SetRevenueSplits(args[1], splits); err != nil {
			return err
		}
		fmt.Printf("revenue splits of %s updated (%d recipients)\n", args[1], len(splits))
		return nil
	case "show":
		if len(args) != 3 {
			return fmt.Errorf("usage: igit splits show <owner> <repo>")
		}
		owner, err := resolveOwner(cc, args[1])
		if err != nil {
			return err
		}
		splits, err := cc.RevenueSplits(owner, args[2])
		if err != nil {
			return err
		}
		if len(splits) == 0 {
			fmt.Println("no splits (100% to owner after platform fee)")
			return nil
		}
		total := uint16(0)
		for _, s := range splits {
			fmt.Printf("%5.1f%%  %s\n", float64(s.Bps)/100, s.Address)
			total += s.Bps
		}
		fmt.Printf("%5.1f%%  (owner remainder)\n", float64(10000-total)/100)
		return nil
	default:
		return fmt.Errorf("unknown splits subcommand %q", args[0])
	}
}

func cmdUsername(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: igit username <register|release|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "register":
		if len(args) != 2 {
			return fmt.Errorf("usage: igit username register <name>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		info, err := cc.ConfigInfo()
		if err != nil {
			return fmt.Errorf("fetch deposit config: %w", err)
		}
		deposit := info.UsernameDeposit.Amount + info.UsernameDeposit.Denom
		if err := cc.RegisterUsername(args[1], deposit); err != nil {
			return err
		}
		fmt.Printf("username %q registered (deposit %s locked, refunded on release)\n", args[1], deposit)
		fmt.Printf("your repos are now reachable as igit://%s/<repo>\n", args[1])
		return nil
	case "release":
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.ReleaseUsername(); err != nil {
			return err
		}
		fmt.Println("username released, deposit refunded")
		return nil
	case "show":
		target := ""
		if len(args) > 1 {
			target = args[1]
		} else {
			var err error
			if target, err = cc.OwnerAddress(); err != nil {
				return err
			}
		}
		if strings.HasPrefix(target, "inj1") {
			name, err := cc.AddressUsername(target)
			if err != nil {
				return err
			}
			if name == "" {
				fmt.Printf("%s holds no username\n", target)
			} else {
				fmt.Printf("%s -> %s\n", target, name)
			}
			return nil
		}
		owner, err := cc.ResolveUsername(target)
		if err != nil {
			return err
		}
		fmt.Printf("%s -> %s\n", target, owner)
		return nil
	default:
		return fmt.Errorf("unknown username subcommand %q", args[0])
	}
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
	case "upload.endpoint":
		cfg.Upload.Endpoint = value
	case "upload.authorization":
		cfg.Upload.Authorization = value
	case "upload.us_peer":
		cfg.Upload.USPeer = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

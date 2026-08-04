// igit is the Next Injective Git companion CLI: repo creation, key helpers and
// configuration management. Pushing/fetching is done by git itself through
// the git-remote-igit helper.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
)

const usageEnglish = `igit - Next Injective Git (Injective + IPFS)

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
  igit transfer <repo> <new-owner>     start a 7-day ownership transfer
  igit transfer accept <owner> <repo>  accept a matured ownership transfer
  igit transfer cancel <repo>          cancel a pending ownership transfer
  igit guardians set <repo> <threshold> <address>...
                                       configure guardian recovery (owner only)
  igit guardians propose <owner> <repo> <new-owner>
                                       propose guardian recovery
  igit guardians approve <owner> <repo> approve guardian recovery
  igit guardians cancel <repo>          owner vetoes guardian recovery
  igit guardians accept <owner> <repo> accept matured guardian recovery
  igit guardians show <owner> <repo>    show guardian/recovery status
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
  igit release register <version> <platform=sha256>...
                                       register immutable release checksums (admin)
  igit release verify <version> <platform> <file>
                                       verify a file against the on-chain checksum
  igit upgrade schedule <wasm-sha256> announce a delayed contract upgrade
  igit upgrade cancel                   cancel the pending upgrade announcement
  igit upgrade show                     show the pending upgrade and delay
  igit key show                        show the configured signing address
  igit key new <name>                  create a key in the injectived keyring
  igit gateway status                  probe HK/US read-only gateway health
  igit gateway select                  print the automatically selected order
  igit config list                     show current configuration
  igit config set <key> <value>        set a configuration value
  igit config unset <key>              clear a configuration override
  igit version                         print version

Config keys:
  contract_address chain_id lcd_endpoint node key_name keyring_backend
  injectived_bin gas_prices ipfs_api ipfs_gateway
  upload.endpoint upload.authorization_endpoint upload.authorization
  upload.us_peer upload.hk_peer

Any other subcommand (add, commit, remote, status, log, branch, ...) is
forwarded to git, so the whole workflow can stay inside igit. Commands
above shadow git ones of the same name (e.g. use ` + "`git config`" + ` for git's).
`

const usageChinese = `igit - Next Injective Git（Injective + IPFS）

用法：
  igit init <name> [description]       创建链上仓库
  igit init [-b <branch>] [.]          转交本地 git init（参数/无名称）
  igit import <github-url> [name]      将 GitHub 仓库镜像到链上
  igit clone <owner>/<repo> [dir]      克隆仓库（也支持 igit://owner/repo）
  igit push [remote] [refspec...]      推送当前仓库到链上（封装 git）
  igit pull [remote] [refspec...]      从链上拉取（封装 git）
  igit clone-url <name>                输出仓库的 igit:// URL
  igit repos [owner]                   列出所有者的仓库
  igit refs <owner> <repo>             列出仓库的链上 refs
  igit collab add <repo> <address> [maintainer|reader]
                                       授予协作者角色（仅所有者）
  igit collab remove <repo> <address>  移除协作者（仅所有者）
  igit collab list <owner> <repo>      列出仓库协作者
  igit transfer <repo> <new-owner>     发起 7 天所有权转移
  igit transfer accept <owner> <repo>  接受已成熟的所有权转移
  igit transfer cancel <repo>          取消待处理的所有权转移
  igit guardians set <repo> <阈值> <地址>...
                                       配置守护人恢复（仅所有者）
  igit guardians propose <owner> <repo> <new-owner>
                                       发起守护人恢复
  igit guardians approve <owner> <repo> 审批守护人恢复
  igit guardians cancel <repo>          所有者否决守护人恢复
  igit guardians accept <owner> <repo> 接受已成熟的守护人恢复
  igit guardians show <owner> <repo>    查看守护人/恢复状态
  igit repo edit <repo> description <text...>
  igit repo edit <repo> branch <name>  更新仓库元数据（仅所有者）
  igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]
                                       设置审核状态（委员会/管理员）
  igit sponsor <owner> <repo> <inj-amount> [message...]
                                       赞助仓库（例如 0.5 INJ）
  igit fork <owner> <repo> [new-name]  将仓库 fork 到你的命名空间
  igit badge award <repo> <recipient> <reason...>
                                       授予贡献徽章（仅所有者）
  igit badge list [address|username]   显示贡献者的奖杯墙
  igit splits set <repo> [addr:bps]... 设置收益分成（仅所有者）
  igit splits show <owner> <repo>      显示收益分成
  igit username register <name>        注册用户名（锁定押金）
  igit username release                释放用户名并退还押金
  igit username show [name|address]    查询用户名/反向查询
  igit release register <版本> <平台=sha256>...
                                       登记不可变发布物校验和（管理员）
  igit release verify <版本> <平台> <文件>
                                       对照链上校验和验证文件
  igit key show                        显示已配置的签名地址
  igit key new <name>                  在 injectived keyring 中创建密钥
  igit gateway status                  探测 HK/US 只读网关健康状态
  igit gateway select                  输出自动选择的顺序
  igit config list                     显示当前配置
  igit config set <key> <value>        设置配置项
  igit version                         输出版本

配置项：
  contract_address chain_id lcd_endpoint node key_name keyring_backend
  injectived_bin gas_prices ipfs_api ipfs_gateway
  upload.endpoint upload.authorization_endpoint upload.authorization
  upload.us_peer upload.hk_peer

其他子命令（add、commit、remote、status、log、branch 等）会转交给 git，
因此整个工作流都可以在 igit 中完成。上面的命令会覆盖同名 git 命令（例如使用 ` + "`git config`" + `）。
`

func usageText() string { return i18n.Text(usageEnglish, usageChinese) }

// version is set at release build time with -ldflags. Keep a useful value for
// local development builds that do not provide the linker override.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "igit: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usageText())
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
	case "guardians":
		return cmdGuardians(cfg, args[1:])
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
	case "release":
		return cmdRelease(cfg, args[1:])
	case "upgrade":
		return cmdUpgrade(cfg, args[1:])
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
		fmt.Print(usageText())
		return nil
	default:
		// Anything igit does not know is forwarded to git so the whole
		// workflow (add/commit/remote/status/...) stays inside igit.
		return runGit(args)
	}
}

func cmdGateway(cfg config.Config, args []string) error {
	if len(args) != 1 || (args[0] != "status" && args[0] != "select") {
		return i18n.Errorf("usage: igit gateway <status|select>", "用法：igit gateway <status|select>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	selected, health := ipfs.SelectGateways(ctx, cfg.EffectiveGateways())
	if args[0] == "status" {
		for _, result := range health {
			if result.Err != nil {
				fmt.Printf(i18n.Text("%-8s down  %-36s %v\n", "%-8s 不可用 %-32s %v\n"), result.Gateway.Name, result.Gateway.URL, result.Err)
				continue
			}
			fmt.Printf(i18n.Text("%-8s ok    %-36s %s\n", "%-8s 正常   %-32s %s\n"), result.Gateway.Name, result.Gateway.URL, result.Latency.Round(time.Millisecond))
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
		return i18n.Errorf("repo created but failed to resolve address: %w", "仓库已创建，但解析地址失败：%w", err)
	}
	fmt.Printf("%s\n\n", i18n.Text("repository created on chain.", "链上仓库已创建。"))
	fmt.Printf("%s\n", i18n.Text("add it as a git remote:", "将其添加为 git remote："))
	fmt.Printf("  igit remote add inj igit://%s/%s\n", owner, name)
	fmt.Printf("  igit push inj main\n")
	return nil
}

// cmdClone wraps `git clone`, accepting a bare "owner/repo" (turned into an
// igit:// URL) or any full igit:// / inj:// URL.
func cmdClone(_ config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit clone <owner>/<repo> [dir]", "用法：igit clone <owner>/<repo> [dir]")
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
		return "", "", i18n.Errorf("cannot parse GitHub source %q (expected github.com/user/repo)", "无法解析 GitHub 来源 %q（应为 github.com/user/repo）", src)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]), parts[1], nil
}

// cmdImport mirrors a GitHub repository onto the chain: bare-clone the source,
// create the on-chain repo, then push every branch and tag through igit.
func cmdImport(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit import <github-url> [name]", "用法：igit import <github-url> [name]")
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

	fmt.Printf(i18n.Text("cloning %s ...\n", "正在克隆 %s ...\n"), cloneURL)
	if err := runGit([]string{"clone", "--bare", "--quiet", cloneURL, tmp}); err != nil {
		return i18n.Errorf("git clone %s: %w", "git 克隆 %s 失败：%w", cloneURL, err)
	}

	// default branch = the source's HEAD (main / master / ...)
	branch, err := runGitIn(tmp, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch == "" {
		branch = "main"
	}

	cc := chain.New(cfg)
	fmt.Printf(i18n.Text("creating on-chain repo %q (default branch %q) ...\n", "正在创建链上仓库 %q（默认分支 %q）...\n"), name, branch)
	if err := cc.CreateRepo(name, "imported from "+cloneURL, branch); err != nil {
		return err
	}
	owner, err := cc.OwnerAddress()
	if err != nil {
		return i18n.Errorf("repo created but failed to resolve address: %w", "仓库已创建，但解析地址失败：%w", err)
	}

	remote := fmt.Sprintf("igit://%s/%s", owner, name)
	if _, err := runGitIn(tmp, "remote", "add", "igit", remote); err != nil {
		return i18n.Errorf("add igit remote: %w", "添加 igit remote 失败：%w", err)
	}
	fmt.Printf(i18n.Text("pushing all branches to %s ...\n", "正在向 %s 推送所有分支...\n"), remote)
	if err := runGitInIO(tmp, "push", "igit", "--all"); err != nil {
		return i18n.Errorf("push branches: %w", "推送分支失败：%w", err)
	}
	// tags are best-effort: a repo may have none
	if tags, _ := runGitIn(tmp, "tag"); tags != "" {
		fmt.Printf("%s\n", i18n.Text("pushing tags ...", "正在推送标签..."))
		if err := runGitInIO(tmp, "push", "igit", "--tags"); err != nil {
			return i18n.Errorf("push tags: %w", "推送标签失败：%w", err)
		}
	}

	fmt.Printf("\n%s\n", i18n.Text("imported! your mirror is live:", "导入完成！你的镜像已上线："))
	fmt.Printf("  igit clone %s\n", remote)
	return nil
}

func cmdCloneURL(cfg config.Config, args []string) error {
	if len(args) != 1 {
		return i18n.Errorf("usage: igit clone-url <name>", "用法：igit clone-url <name>")
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
			return i18n.Errorf("no owner given and cannot resolve local key: %w", "未提供所有者，且无法解析本地密钥：%w", err)
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
		fmt.Printf(i18n.Text("no repositories for %s\n", "%s 没有仓库\n"), owner)
		return nil
	}
	for _, r := range out.Repos {
		fmt.Printf(i18n.Text("%-32s default:%-12s %s\n", "%-32s 默认分支：%-12s %s\n"), r.Name, r.DefaultBranch, r.Description)
	}
	return nil
}

func cmdRefs(cfg config.Config, args []string) error {
	if len(args) != 2 {
		return i18n.Errorf("usage: igit refs <owner> <repo>", "用法：igit refs <owner> <repo>")
	}
	refs, err := chain.New(cfg).ListRefs(args[0], args[1])
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		fmt.Println(i18n.Text("no refs (empty repository)", "没有 refs（空仓库）"))
		return nil
	}
	for _, r := range refs {
		fmt.Printf(i18n.Text("%s %-40s packfiles:%d\n", "%s %-40s packfile：%d\n"), r.CommitSha, r.RefName, len(r.PackURIs))
	}
	return nil
}

func cmdCollab(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit collab <add|remove|list> ...", "用法：igit collab <add|remove|list> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "add":
		if len(args) < 3 {
			return i18n.Errorf("usage: igit collab add <repo> <address> [maintainer|reader]", "用法：igit collab add <repo> <address> [maintainer|reader]")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		role := "maintainer"
		if len(args) > 3 {
			role = args[3]
		}
		if role != "maintainer" && role != "reader" {
			return i18n.Errorf("invalid role %q (maintainer|reader)", "角色 %q 无效（maintainer|reader）", role)
		}
		if err := cc.SetCollaborator(args[1], args[2], role); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("collaborator %s added to %s as %s\n", "已将协作者 %s 添加到 %s，角色为 %s\n"), args[2], args[1], role)
		return nil
	case "remove":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit collab remove <repo> <address>", "用法：igit collab remove <repo> <address>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.SetCollaborator(args[1], args[2], ""); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("collaborator %s removed from %s\n", "已从 %s 移除协作者 %s\n"), args[1], args[2])
		return nil
	case "list":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit collab list <owner> <repo>", "用法：igit collab list <owner> <repo>")
		}
		collabs, err := cc.ListCollaborators(args[1], args[2])
		if err != nil {
			return err
		}
		if len(collabs) == 0 {
			fmt.Println(i18n.Text("no collaborators", "没有协作者"))
			return nil
		}
		for _, c := range collabs {
			fmt.Printf("%-12s %s\n", c.Role, c.Address)
		}
		return nil
	default:
		return i18n.Errorf("unknown collab subcommand %q", "未知的 collab 子命令 %q", args[0])
	}
}

func cmdTransfer(cfg config.Config, args []string) error {
	if len(args) > 0 && args[0] == "accept" {
		if len(args) != 3 {
			return i18n.Errorf("usage: igit transfer accept <owner> <repo>", "用法：igit transfer accept <owner> <repo>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := chain.New(cfg).AcceptOwnership(args[1], args[2]); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("ownership of %s/%s accepted\n", "已接受 %s/%s 的所有权转移\n"), args[1], args[2])
		return nil
	}
	if len(args) > 0 && args[0] == "cancel" {
		if len(args) != 2 {
			return i18n.Errorf("usage: igit transfer cancel <repo>", "用法：igit transfer cancel <repo>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := chain.New(cfg).CancelOwnershipTransfer(args[1]); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("pending ownership transfer for %s cancelled\n", "已取消 %s 的待处理所有权转移\n"), args[1])
		return nil
	}
	if len(args) != 2 {
		return i18n.Errorf("usage: igit transfer <repo> <new-owner>", "用法：igit transfer <repo> <new-owner>")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(args[1], "inj1") {
		return i18n.Errorf("new owner %q must be an inj1... bech32 address", "新所有者 %q 必须是 inj1... bech32 地址", args[1])
	}
	if err := chain.New(cfg).TransferOwnership(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf(i18n.Text("ownership transfer for %s started; %s must accept after 7 days\n", "已发起 %s 的所有权转移；%s 需在 7 天后主动接受\n"), args[0], args[1])
	return nil
}

func cmdGuardians(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit guardians <set|propose|approve|cancel|accept|show> ...", "用法：igit guardians <set|propose|approve|cancel|accept|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "set":
		if len(args) < 4 {
			return i18n.Errorf("usage: igit guardians set <repo> <threshold> <address>...", "用法：igit guardians set <仓库> <阈值> <地址>...")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		threshold, err := strconv.ParseUint(args[2], 10, 8)
		if err != nil || threshold == 0 {
			return i18n.Errorf("invalid guardian threshold %q", "守护人阈值 %q 无效", args[2])
		}
		guardians := append([]string(nil), args[3:]...)
		for _, address := range guardians {
			if !strings.HasPrefix(address, "inj1") {
				return i18n.Errorf("guardian %q must be an inj1... address", "守护人 %q 必须是 inj1... 地址", address)
			}
		}
		if err := cc.SetGuardians(args[1], guardians, uint8(threshold)); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("guardians configured for %s (threshold %d)\n", "%s 的守护人已配置（阈值 %d）\n"), args[1], threshold)
		return nil
	case "propose":
		if len(args) != 4 {
			return i18n.Errorf("usage: igit guardians propose <owner> <repo> <new-owner>", "用法：igit guardians propose <owner> <repo> <new-owner>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.ProposeRecovery(args[1], args[2], args[3]); err != nil {
			return err
		}
		fmt.Println(i18n.Text("guardian recovery proposed; wait 7 days and collect approvals", "已发起守护人恢复；等待 7 天并收集审批"))
		return nil
	case "approve":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit guardians approve <owner> <repo>", "用法：igit guardians approve <owner> <repo>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.ApproveRecovery(args[1], args[2]); err != nil {
			return err
		}
		fmt.Println(i18n.Text("guardian recovery approved", "守护人恢复已审批"))
		return nil
	case "cancel":
		if len(args) != 2 {
			return i18n.Errorf("usage: igit guardians cancel <repo>", "用法：igit guardians cancel <repo>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.CancelRecovery(args[1]); err != nil {
			return err
		}
		fmt.Println(i18n.Text("guardian recovery cancelled", "守护人恢复已取消"))
		return nil
	case "accept":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit guardians accept <owner> <repo>", "用法：igit guardians accept <owner> <repo>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.AcceptRecovery(args[1], args[2]); err != nil {
			return err
		}
		fmt.Println(i18n.Text("guardian recovery accepted", "守护人恢复已接受"))
		return nil
	case "show":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit guardians show <owner> <repo>", "用法：igit guardians show <owner> <repo>")
		}
		if err := cfg.ValidateContract(); err != nil {
			return err
		}
		status, err := cc.OwnershipSecurity(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("threshold %d\n", status.GuardianThreshold)
		fmt.Printf("guardians: %s\n", strings.Join(status.Guardians, ", "))
		if status.Transfer != nil {
			fmt.Printf("transfer: %s (execute after %d)\n", status.Transfer.NewOwner, status.Transfer.ExecuteAfter)
		}
		if status.Recovery != nil {
			fmt.Printf("recovery: %s (%d/%d approvals; execute after %d)\n", status.Recovery.NewOwner, len(status.Recovery.Approvals), status.GuardianThreshold, status.Recovery.ExecuteAfter)
		}
		return nil
	default:
		return i18n.Errorf("unknown guardians subcommand %q", "未知的 guardians 子命令 %q", args[0])
	}
}

func cmdRepo(cfg config.Config, args []string) error {
	if len(args) < 4 || args[0] != "edit" {
		return i18n.Errorf("usage: igit repo edit <repo> <description|branch> <value...>", "用法：igit repo edit <repo> <description|branch> <value...>")
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
		return i18n.Errorf("unknown field %q (description|branch)", "未知字段 %q（description|branch）", args[2])
	}
	if err := chain.New(cfg).UpdateRepoInfo(repo, description, branch); err != nil {
		return err
	}
	fmt.Printf(i18n.Text("repo %s updated\n", "仓库 %s 已更新\n"), repo)
	return nil
}

func cmdMod(cfg config.Config, args []string) error {
	if len(args) > 0 && args[0] == "report" {
		if len(args) != 4 {
			return i18n.Errorf("usage: igit mod report <owner> <repo> <reason-hash>", "用法：igit mod report <owner> <repo> <原因哈希>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		owner, err := resolveOwner(chain.New(cfg), args[1])
		if err != nil {
			return err
		}
		return chain.New(cfg).SubmitModerationReport(owner, args[2], args[3])
	}
	if len(args) > 0 && (args[0] == "resolve" || args[0] == "appeal-resolve") {
		if len(args) != 4 {
			return i18n.Errorf("usage: igit mod %s <report-id> <active|delisted|frozen> <reason-hash>", "用法：igit mod %s <报告 ID> <active|delisted|frozen> <原因哈希>", args[0])
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		id, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return i18n.Errorf("invalid report id %q", "报告 ID %q 无效", args[1])
		}
		status := args[2]
		if status != "active" && status != "delisted" && status != "frozen" {
			return i18n.Errorf("invalid status %q (active|delisted|frozen)", "状态 %q 无效（active|delisted|frozen）", status)
		}
		cc := chain.New(cfg)
		if args[0] == "resolve" {
			return cc.ResolveModerationReport(id, status, args[3])
		}
		return cc.ResolveModerationAppeal(id, status, args[3])
	}
	if len(args) > 0 && args[0] == "appeal" {
		if len(args) != 3 {
			return i18n.Errorf("usage: igit mod appeal <report-id> <reason-hash>", "用法：igit mod appeal <报告 ID> <原因哈希>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		id, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return i18n.Errorf("invalid report id %q", "报告 ID %q 无效", args[1])
		}
		return chain.New(cfg).AppealModerationReport(id, args[2])
	}
	if len(args) > 0 && args[0] == "report-show" {
		if len(args) != 2 {
			return i18n.Errorf("usage: igit mod report-show <report-id>", "用法：igit mod report-show <报告 ID>")
		}
		if err := cfg.ValidateContract(); err != nil {
			return err
		}
		id, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return i18n.Errorf("invalid report id %q", "报告 ID %q 无效", args[1])
		}
		report, err := chain.New(cfg).ModerationReport(id)
		if err != nil {
			return err
		}
		fmt.Printf("#%d %s/%s status=%s reporter=%s\n", report.ID, report.Owner, report.Repo, report.Status, report.Reporter)
		fmt.Printf("reason=%s\n", report.ReasonHash)
		return nil
	}
	if len(args) < 3 {
		return i18n.Errorf("usage: igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]", "用法：igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	status := args[2]
	switch status {
	case "active", "delisted", "frozen":
	default:
		return i18n.Errorf("invalid status %q (active|delisted|frozen)", "状态 %q 无效（active|delisted|frozen）", status)
	}
	reason := ""
	if len(args) > 3 {
		reason = args[3]
	}
	if err := chain.New(cfg).SetModerationStatus(args[0], args[1], status, reason); err != nil {
		return err
	}
	fmt.Printf(i18n.Text("%s/%s moderation status set to %s\n", "已将 %s/%s 的审核状态设为 %s\n"), args[0], args[1], status)
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
		return "", i18n.Errorf("amount %q has more than 18 decimal places", "金额 %q 的小数位超过 18 位", s)
	}
	frac += strings.Repeat("0", 18-len(frac))
	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return "", i18n.Errorf("invalid INJ amount %q", "INJ 金额 %q 无效", s)
		}
	}
	base := strings.TrimLeft(whole+frac, "0")
	if base == "" {
		return "", i18n.Errorf("amount must be positive", "金额必须为正数")
	}
	return base + "inj", nil
}

func addCoinAmounts(a, b chain.Coin) (string, error) {
	if a.Denom == "" || a.Denom != b.Denom {
		return "", fmt.Errorf("username deposit and fee use different denoms")
	}
	left, ok := new(big.Int).SetString(a.Amount, 10)
	if !ok {
		return "", fmt.Errorf("invalid username deposit amount %q", a.Amount)
	}
	right, ok := new(big.Int).SetString(b.Amount, 10)
	if !ok {
		return "", fmt.Errorf("invalid username fee amount %q", b.Amount)
	}
	return new(big.Int).Add(left, right).String() + a.Denom, nil
}

func cmdSponsor(cfg config.Config, args []string) error {
	if len(args) < 3 {
		return i18n.Errorf("usage: igit sponsor <owner> <repo> <inj-amount> [message...]", "用法：igit sponsor <owner> <repo> <inj-amount> [message...]")
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
	fmt.Printf(i18n.Text("sponsored %s/%s with %s INJ — thank you!\n", "已使用 %s/%s 的 %s INJ 赞助，谢谢！\n"), args[0], args[1], args[2])
	return nil
}

func cmdFork(cfg config.Config, args []string) error {
	if len(args) < 2 {
		return i18n.Errorf("usage: igit fork <owner> <repo> [new-name]", "用法：igit fork <owner> <repo> [new-name]")
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
	fmt.Printf(i18n.Text("forked %s/%s\n", "已 fork %s/%s\n"), args[0], args[1])
	fmt.Printf(i18n.Text("clone your fork: git clone igit://%s/%s\n", "克隆你的 fork：git clone igit://%s/%s\n"), self, newName)
	return nil
}

func cmdBadge(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit badge <award|list> ...", "用法：igit badge <award|list> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "award":
		if len(args) < 4 {
			return i18n.Errorf("usage: igit badge award <repo> <recipient> <reason...>", "用法：igit badge award <repo> <recipient> <reason...>")
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
		fmt.Printf(i18n.Text("badge awarded to %s for %q\n", "已为 %s 授予徽章，理由：%q\n"), args[2], reason)
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
			fmt.Println(i18n.Text("no badges yet", "还没有徽章"))
			return nil
		}
		for _, b := range badges {
			fmt.Printf("#%-4d %s/%s: %q\n", b.ID, b.RepoOwner[:12]+"…", b.RepoName, b.Reason)
		}
		return nil
	default:
		return i18n.Errorf("unknown badge subcommand %q", "未知的 badge 子命令 %q", args[0])
	}
}

func cmdSplits(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit splits <set|show> ...", "用法：igit splits <set|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return i18n.Errorf("usage: igit splits set <repo> [address:bps]...", "用法：igit splits set <repo> [address:bps]...")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		var splits []chain.SplitEntry
		for _, spec := range args[2:] {
			addr, bpsStr, ok := strings.Cut(spec, ":")
			if !ok {
				return i18n.Errorf("invalid split %q (expected address:bps)", "分成 %q 无效（应为 address:bps）", spec)
			}
			bps, err := strconv.ParseUint(bpsStr, 10, 16)
			if err != nil {
				return i18n.Errorf("invalid bps in %q: %w", "分成 %q 中的 bps 无效：%w", spec, err)
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
		fmt.Printf(i18n.Text("revenue splits of %s updated (%d recipients)\n", "%s 的收益分成已更新（%d 位接收者）\n"), args[1], len(splits))
		return nil
	case "show":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit splits show <owner> <repo>", "用法：igit splits show <owner> <repo>")
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
			fmt.Println(i18n.Text("no splits (100% to owner after platform fee)", "没有分成（扣除平台费后 100% 归所有者）"))
			return nil
		}
		total := uint16(0)
		for _, s := range splits {
			fmt.Printf("%5.1f%%  %s\n", float64(s.Bps)/100, s.Address)
			total += s.Bps
		}
		fmt.Printf(i18n.Text("%5.1f%%  (owner remainder)\n", "%5.1f%%  （所有者余下部分）\n"), float64(10000-total)/100)
		return nil
	default:
		return i18n.Errorf("unknown splits subcommand %q", "未知的 splits 子命令 %q", args[0])
	}
}

func cmdUsername(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit username <register|release|show> ...", "用法：igit username <register|release|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "register":
		if len(args) != 2 {
			return i18n.Errorf("usage: igit username register <name>", "用法：igit username register <name>")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		info, err := cc.ConfigInfo()
		if err != nil {
			return i18n.Errorf("fetch deposit config: %w", "获取押金配置失败：%w", err)
		}
		registrationCost, err := addCoinAmounts(info.UsernameDeposit, info.UsernameFee)
		if err != nil {
			return err
		}
		if err := cc.RegisterUsername(args[1], registrationCost); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("username %q registered (deposit %s locked; registration fee sent to treasury)\n", "用户名 %q 已注册（押金 %s 已锁定；注册费已发送到金库）\n"), args[1], info.UsernameDeposit.Amount+info.UsernameDeposit.Denom)
		fmt.Printf(i18n.Text("your repos are now reachable as igit://%s/<repo>\n", "你的仓库现在可通过 igit://%s/<repo> 访问\n"), args[1])
		return nil
	case "release":
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.ReleaseUsername(); err != nil {
			return err
		}
		fmt.Println(i18n.Text("username released, deposit refunded", "用户名已释放，押金已退还"))
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
				fmt.Printf(i18n.Text("%s holds no username\n", "%s 没有用户名\n"), target)
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
		return i18n.Errorf("unknown username subcommand %q", "未知的 username 子命令 %q", args[0])
	}
}

func cmdRelease(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit release <register|verify> ...", "用法：igit release <register|verify> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "register":
		if len(args) < 3 {
			return i18n.Errorf("usage: igit release register <version> <platform=sha256>...", "用法：igit release register <版本> <平台=sha256>...")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		artifacts := make([]chain.ReleaseArtifact, 0, len(args)-2)
		seen := make(map[string]bool)
		for _, spec := range args[2:] {
			platform, digest, ok := strings.Cut(spec, "=")
			if !ok || platform == "" || digest == "" || len(digest) != sha256.Size*2 {
				return i18n.Errorf("invalid artifact %q (expected platform=64-hex-sha256)", "发布物 %q 无效（应为 平台=64 位十六进制 SHA-256）", spec)
			}
			if _, err := hex.DecodeString(digest); err != nil {
				return i18n.Errorf("invalid SHA-256 for %q", "发布物 %q 的 SHA-256 无效", platform)
			}
			if seen[platform] {
				return i18n.Errorf("duplicate artifact platform %q", "重复的发布物平台 %q", platform)
			}
			seen[platform] = true
			artifacts = append(artifacts, chain.ReleaseArtifact{Platform: platform, SHA256: strings.ToLower(digest)})
		}
		if err := cc.RegisterRelease(args[1], artifacts); err != nil {
			return err
		}
		fmt.Printf(i18n.Text("release %s registered (%d artifacts)\n", "发布 %s 已登记（%d 个发布物）\n"), args[1], len(artifacts))
		return nil
	case "verify":
		if len(args) != 4 {
			return i18n.Errorf("usage: igit release verify <version> <platform> <file>", "用法：igit release verify <版本> <平台> <文件>")
		}
		if err := cfg.ValidateContract(); err != nil {
			return err
		}
		want, err := cc.ReleaseArtifacts(args[1])
		if err != nil {
			return err
		}
		var expected string
		for _, artifact := range want {
			if artifact.Platform == args[2] {
				expected = strings.ToLower(artifact.SHA256)
				break
			}
		}
		if expected == "" {
			return fmt.Errorf("no on-chain checksum for %s/%s", args[1], args[2])
		}
		actual, err := sha256File(args[3])
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("checksum mismatch for %s: got %s, want %s", args[3], actual, expected)
		}
		fmt.Printf(i18n.Text("verified %s against on-chain release %s/%s\n", "已根据链上发布 %s/%s 验证 %s\n"), args[3], args[1], args[2])
		return nil
	default:
		return i18n.Errorf("unknown release subcommand %q", "未知的 release 子命令 %q", args[0])
	}
}

func cmdUpgrade(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return i18n.Errorf("usage: igit upgrade <schedule|cancel|show> ...", "usage: igit upgrade <schedule|cancel|show> ...")
	}
	cc := chain.New(cfg)
	switch args[0] {
	case "schedule":
		if len(args) != 2 || len(args[1]) != sha256.Size*2 {
			return i18n.Errorf("usage: igit upgrade schedule <wasm-sha256>", "usage: igit upgrade schedule <wasm-sha256>")
		}
		if _, err := hex.DecodeString(args[1]); err != nil {
			return i18n.Errorf("wasm SHA-256 must be 64 hexadecimal characters", "wasm SHA-256 must be 64 hexadecimal characters")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.ScheduleUpgrade(strings.ToLower(args[1])); err != nil {
			return err
		}
		fmt.Println(i18n.Text("upgrade scheduled; wait 14 days before migrate", "upgrade scheduled; wait 14 days before migrate"))
		return nil
	case "cancel":
		if len(args) != 1 {
			return i18n.Errorf("usage: igit upgrade cancel", "usage: igit upgrade cancel")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cc.CancelUpgrade(); err != nil {
			return err
		}
		fmt.Println(i18n.Text("pending upgrade cancelled", "pending upgrade cancelled"))
		return nil
	case "show":
		if len(args) != 1 {
			return i18n.Errorf("usage: igit upgrade show", "usage: igit upgrade show")
		}
		if err := cfg.ValidateContract(); err != nil {
			return err
		}
		security, err := cc.UpgradeSecurity()
		if err != nil {
			return err
		}
		fmt.Printf("timelock_seconds=%d\n", security.TimelockSeconds)
		if security.Proposal == nil {
			fmt.Println("proposal=none")
			return nil
		}
		fmt.Printf("wasm_sha256=%s\nproposed_at=%d\nexecute_after=%d\n", security.Proposal.WasmSHA256, security.Proposal.ProposedAt, security.Proposal.ExecuteAfter)
		return nil
	default:
		return i18n.Errorf("unknown upgrade subcommand %q", "unknown upgrade subcommand %q", args[0])
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func cmdKey(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return i18n.Errorf("usage: igit key <show|new> [name]", "用法：igit key <show|new> [name]")
	}
	bin := cfg.InjectivedBin
	if bin == "" {
		bin = "injectived"
	}
	switch args[0] {
	case "show":
		if cfg.KeyName == "" {
			return i18n.Errorf("key_name not configured", "未配置 key_name")
		}
		addr, err := chain.New(cfg).OwnerAddress()
		if err != nil {
			return err
		}
		fmt.Printf(i18n.Text("key:     %s\naddress: %s\n", "密钥：   %s\n地址：   %s\n"), cfg.KeyName, addr)
		return nil
	case "new":
		if len(args) != 2 {
			return i18n.Errorf("usage: igit key new <name>", "用法：igit key new <name>")
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
		fmt.Printf(i18n.Text("\nkey_name set to %q in igit config.\n", "\n已在 igit 配置中将 key_name 设为 %q。\n"), args[1])
		return nil
	default:
		return i18n.Errorf("unknown key subcommand %q", "未知的 key 子命令 %q", args[0])
	}
}

func cmdConfig(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return i18n.Errorf("usage: igit config <list|set> ...", "用法：igit config <list|set> ...")
	}
	switch args[0] {
	case "list":
		display := cfg
		if display.Upload.Authorization != "" {
			display.Upload.Authorization = "<redacted>"
		}
		data, err := json.MarshalIndent(display, "", "  ")
		if err != nil {
			return err
		}
		path, _ := config.Path()
		fmt.Printf("# %s\n%s\n", path, data)
		return nil
	case "set":
		if len(args) != 3 {
			return i18n.Errorf("usage: igit config set <key> <value>", "用法：igit config set <key> <value>")
		}
		if err := setConfigField(&cfg, args[1], args[2]); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		shown := args[2]
		if args[1] == "upload.authorization" && shown != "" {
			shown = "<redacted>"
		}
		fmt.Printf("%s = %s\n", args[1], shown)
		return nil
	case "unset":
		if len(args) != 2 {
			return fmt.Errorf("usage: igit config unset <key>")
		}
		if err := setConfigField(&cfg, args[1], ""); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("%s cleared\n", args[1])
		return nil
	default:
		return i18n.Errorf("unknown config subcommand %q", "未知的 config 子命令 %q", args[0])
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
	case "upload.authorization_endpoint":
		cfg.Upload.AuthorizationEndpoint = value
	case "upload.authorization":
		cfg.Upload.Authorization = value
	case "upload.us_peer":
		cfg.Upload.USPeer = value
	case "upload.hk_peer":
		cfg.Upload.HKPeer = value
	default:
		return i18n.Errorf("unknown config key %q", "未知的配置项 %q", key)
	}
	return nil
}

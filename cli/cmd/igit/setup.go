package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/bootstrap"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/environment"
)

type setupOptions struct {
	yes       bool
	skipKubo  bool
	force     bool
	createKey string
	wsl       string
}

func cmdSetup(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: igit setup <push|status|upgrade> [options]")
	}
	switch args[0] {
	case "status":
		wsl, remaining, err := parseStatusArgs(args[1:])
		if err != nil {
			return err
		}
		if wsl != "" {
			return forwardSetupToWSL(wsl, append([]string{"status"}, remaining...))
		}
		return cmdDoctor(cfg, append([]string{"--push"}, remaining...))
	case "push", "upgrade":
		opts, forwarded, err := parseSetupOptions(args[1:])
		if err != nil {
			return err
		}
		if args[0] == "upgrade" {
			opts.force = true
			forwarded = append(forwarded, "--force")
		}
		if opts.wsl != "" {
			return forwardSetupToWSL(opts.wsl, append([]string{"push"}, forwarded...))
		}
		if runtime.GOOS == "windows" {
			return fmt.Errorf("Windows push requires WSL2; run `igit setup push --wsl Ubuntu-24.04` or `powershell -File scripts/bootstrap-push.ps1`")
		}
		return setupPush(cfg, opts)
	default:
		return fmt.Errorf("unknown setup subcommand %q (expected push, status, or upgrade)", args[0])
	}
}

func parseSetupOptions(args []string) (setupOptions, []string, error) {
	var opts setupOptions
	var forwarded []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes" || arg == "-y":
			opts.yes = true
			forwarded = append(forwarded, "--yes")
		case arg == "--no-kubo":
			opts.skipKubo = true
			forwarded = append(forwarded, arg)
		case arg == "--force":
			opts.force = true
			forwarded = append(forwarded, arg)
		case arg == "--create-key":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return opts, nil, fmt.Errorf("--create-key requires a key name")
			}
			i++
			opts.createKey = args[i]
			forwarded = append(forwarded, "--create-key", args[i])
		case strings.HasPrefix(arg, "--create-key="):
			opts.createKey = strings.TrimPrefix(arg, "--create-key=")
			if opts.createKey == "" {
				return opts, nil, fmt.Errorf("--create-key requires a key name")
			}
			forwarded = append(forwarded, arg)
		case arg == "--wsl":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return opts, nil, fmt.Errorf("--wsl requires a distribution name")
			}
			i++
			opts.wsl = args[i]
		case strings.HasPrefix(arg, "--wsl="):
			opts.wsl = strings.TrimPrefix(arg, "--wsl=")
			if opts.wsl == "" {
				return opts, nil, fmt.Errorf("--wsl requires a distribution name")
			}
		case arg == "--help" || arg == "-h":
			return opts, nil, fmt.Errorf("usage: igit setup push [--yes] [--no-kubo] [--force] [--create-key NAME] [--wsl DISTRO]")
		default:
			return opts, nil, fmt.Errorf("unknown setup option %q", arg)
		}
	}
	return opts, forwarded, nil
}

func parseStatusArgs(args []string) (string, []string, error) {
	var wsl string
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--wsl":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--wsl requires a distribution name")
			}
			i++
			wsl = args[i]
		case strings.HasPrefix(args[i], "--wsl="):
			wsl = strings.TrimPrefix(args[i], "--wsl=")
		case args[i] == "--json":
			remaining = append(remaining, args[i])
		default:
			return "", nil, fmt.Errorf("usage: igit setup status [--json] [--wsl DISTRO]")
		}
	}
	return wsl, remaining, nil
}

func setupPush(cfg config.Config, opts setupOptions) error {
	if !opts.yes {
		fmt.Println("igit will install pinned push dependencies under ~/.igit/deps.")
		fmt.Println("Existing working injectived and Kubo installations will be preserved.")
		fmt.Print("Continue? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("setup cancelled")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	updated, result, err := bootstrap.Prepare(ctx, cfg, bootstrap.Options{
		Force: opts.force, SkipKubo: opts.skipKubo, Progress: os.Stdout,
	})
	if err != nil {
		return err
	}
	if err := config.Save(updated); err != nil {
		return err
	}
	cfg = updated

	if opts.createKey != "" {
		keyCfg := cfg
		keyCfg.KeyName = opts.createKey
		if _, err := chain.New(keyCfg).OwnerAddress(); err == nil {
			cfg.KeyName = opts.createKey
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("Using existing key %q\n", opts.createKey)
		} else {
			if err := cmdKey(cfg, []string{"new", opts.createKey}); err != nil {
				return fmt.Errorf("create key %q: %w", opts.createKey, err)
			}
			cfg, err = config.Load()
			if err != nil {
				return err
			}
		}
	}

	fmt.Println("\nSetup summary:")
	for _, item := range result.Installed {
		fmt.Println("  installed", item)
	}
	for _, item := range result.Reused {
		fmt.Println("  reused   ", item)
	}
	if opts.skipKubo {
		fmt.Println("  skipped   Kubo (--no-kubo)")
	}

	doctorCtx, doctorCancel := context.WithTimeout(context.Background(), 30*time.Second)
	report := environment.Run(doctorCtx, cfg, environment.ModePush)
	doctorCancel()
	fmt.Println()
	printDoctorReport(report)

	var hardFailures []environment.Check
	for _, failure := range report.Failures() {
		if cfg.KeyName == "" && (failure.Name == "key_name" || failure.Name == "signing key") {
			continue
		}
		if opts.skipKubo && (failure.Name == "Kubo CLI" || failure.Name == "local Kubo API") {
			continue
		}
		hardFailures = append(hardFailures, failure)
	}
	if len(hardFailures) > 0 {
		return fmt.Errorf("setup finished but %d required checks still fail", len(hardFailures))
	}
	if cfg.KeyName == "" {
		fmt.Println("\nNext: create or select a signer with `igit key new dev`, then fund the printed address from the Injective testnet faucet.")
	} else {
		fmt.Println("\nPush tooling is ready. Fund the signing address if the balance check warns about gas.")
	}
	return nil
}

func forwardSetupToWSL(distro string, args []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("--wsl is available only from the Windows igit binary")
	}
	ready, err := matchingWSLCLI(distro)
	if err != nil {
		return err
	}
	if !ready {
		if version == "dev" {
			return fmt.Errorf("WSL distribution %q does not have this development build; run scripts/bootstrap-push.ps1 from the source checkout", distro)
		}
		if err := installReleasedWSLCLI(distro); err != nil {
			return err
		}
	}
	runScript := `export PATH="$HOME/.local/bin:$HOME/.igit/bin:$PATH"; exec igit "$@"`
	cmdArgs := []string{"-d", distro, "--", "bash", "-lc", runScript, "bash", "setup"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("wsl.exe", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run igit inside WSL distribution %q: %w (install the Linux CLI first or run scripts/bootstrap-push.ps1)", distro, err)
	}
	return nil
}

func matchingWSLCLI(distro string) (bool, error) {
	cmd := exec.Command("wsl.exe", "-d", distro, "--", "bash", "-lc", `export PATH="$HOME/.local/bin:$PATH"; command -v igit >/dev/null && igit version`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit 127/1 means the distribution is working but igit is absent.
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("inspect WSL distribution %q: %w", distro, err)
	}
	return strings.TrimSpace(string(out)) == "igit "+version, nil
}

func installReleasedWSLCLI(distro string) error {
	const script = `set -euo pipefail
version=$1
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "invalid igit release version: $version" >&2; exit 1 ;;
esac
if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    if [ "$(id -u)" -eq 0 ]; then root=""; elif command -v sudo >/dev/null 2>&1; then root="sudo"; else echo "curl is missing and sudo is unavailable" >&2; exit 1; fi
    $root apt-get update
    $root apt-get install -y curl ca-certificates
  else
    echo "curl is required to download igit release assets" >&2
    exit 1
  fi
fi
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported WSL architecture: $(uname -m)" >&2; exit 1 ;;
esac
base="https://github.com/Hny0305Lin/next-injective-git/releases/download/$version"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
curl -fL --retry 3 -o checksums.txt "$base/checksums.txt"
for asset in "igit-linux-$arch" "git-remote-igit-linux-$arch"; do
  curl -fL --retry 3 -o "$asset" "$base/$asset"
  grep -E "^[0-9a-f]{64}  ${asset}$" checksums.txt > "$asset.sha256"
  sha256sum --check --strict "$asset.sha256"
done
mkdir -p "$HOME/.local/bin"
install -m 0755 "igit-linux-$arch" "$HOME/.local/bin/igit"
install -m 0755 "git-remote-igit-linux-$arch" "$HOME/.local/bin/git-remote-igit"
profile_line='export PATH="$HOME/.local/bin:$HOME/.igit/bin:$PATH"'
touch "$HOME/.profile"
grep -Fqx "$profile_line" "$HOME/.profile" || printf '\n%s\n' "$profile_line" >> "$HOME/.profile"
"$HOME/.local/bin/igit" version
`
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	command := "printf %s " + encoded + " | base64 -d | bash -s -- " + version
	cmd := exec.Command("wsl.exe", "-d", distro, "--", "bash", "-lc", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install igit %s in WSL distribution %q: %w", version, distro, err)
	}
	return nil
}

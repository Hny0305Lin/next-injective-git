package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

func TestVersionLinkerInjection(t *testing.T) {
	want := os.Getenv("IGIT_EXPECTED_VERSION")
	if want == "" {
		want = "dev"
	}
	if version != want {
		t.Fatalf("version = %q, want %q", version, want)
	}
}

func TestSHA256File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.bin")
	content := []byte("release artifact\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	got, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
}

func TestVisibleReposHidesModeratedRepositoriesByDefault(t *testing.T) {
	repos := []chain.RepoInfo{
		{Name: "active", ModerationStatus: "active"},
		{Name: "frozen", ModerationStatus: "frozen"},
		{Name: "delisted", ModerationStatus: "delisted"},
	}
	visible := visibleRepos(repos, false)
	if len(visible) != 1 || visible[0].Name != "active" {
		t.Fatalf("visible repos = %#v", visible)
	}
	if all := visibleRepos(repos, true); len(all) != len(repos) {
		t.Fatalf("--all returned %d repos, want %d", len(all), len(repos))
	}
}

func TestUpgradeCommandIsExplicitlyRemoved(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	t.Setenv("LC_ALL", "zh-CN")
	err := run([]string{"upgrade", "show"})
	if !i18n.HasCode(err, errorCodeUpgradeRemoved) {
		t.Fatalf("upgrade error = %v, want immutable-suite removal", err)
	}
}

func TestUpgradeCommandErrorRendering(t *testing.T) {
	for _, test := range []struct {
		name   string
		locale string
		want   string
	}{
		{name: "english", locale: "en-US", want: "igit upgrade was removed with the immutable EVM suite; use `igit suite verify`"},
		{name: "chinese", locale: "zh-CN", want: "不可升级 EVM suite 已移除 igit upgrade；请使用 `igit suite verify`"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("IGIT_HOME", t.TempDir())
			t.Setenv("LC_ALL", test.locale)
			err := run([]string{"upgrade", "show"})
			if err == nil || err.Error() != test.want {
				t.Fatalf("upgrade error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestPublicConfigViewHidesBackendAndTransportDetails(t *testing.T) {
	cfg := config.Defaults()
	cfg.EVMRPC = "https://rpc.example.invalid"
	cfg.EVMSuiteDirectoryAddress = "0x1111111111111111111111111111111111111111"
	cfg.InjectivedBin = "injectived"
	cfg.KeyringBackend = "test"
	cfg.KeyName = "dev"

	view := publicConfigView(cfg)
	if view["network"] != cfg.Network || view["key_name"] != "dev" {
		t.Fatalf("public config view = %#v, want network and key name", view)
	}
	for _, hidden := range []string{
		"contract_backend", "contract_version", "evm_rpc",
		"evm_suite_directory_address", "injectived_bin", "keyring_backend",
	} {
		if _, ok := view[hidden]; ok {
			t.Fatalf("public config view exposes %q: %#v", hidden, view)
		}
	}
}

func TestSetConfigNetworkCannotRetainPreviousProfileTrustRoot(t *testing.T) {
	cfg := config.Defaults()
	cfg.EVMRPC = "https://stale-testnet-rpc.invalid"
	cfg.EVMChainID = 1439
	cfg.EVMSuiteDirectoryAddress = "0x1111111111111111111111111111111111111111"
	if err := setConfigField(&cfg, "network", "injective-mainnet"); err != nil {
		t.Fatal(err)
	}
	if cfg.Network != "injective-mainnet" || cfg.ChainID != "injective-1" || cfg.EVMChainID != 1776 {
		t.Fatalf("network identity = %#v", cfg)
	}
	if cfg.EVMRPC != "https://sentry.evm-rpc.injective.network/" || cfg.EVMSuiteDirectoryAddress != "" {
		t.Fatalf("network switch retained stale profile values: %#v", cfg)
	}
}

func TestRemovedLegacyConfigKeysCannotBeSet(t *testing.T) {
	for _, key := range []string{
		"contract_backend", "contract_version", "contract_address",
		"evm_contract_address", "evm_badge_module_address",
		"evm_economic_module_address", "evm_moderation_module_address",
		"chain_id", "lcd_endpoint", "node", "keyring_backend",
		"injectived_bin", "gas_prices",
	} {
		cfg := config.Defaults()
		if err := setConfigField(&cfg, key, "value"); err == nil {
			t.Fatalf("legacy config key %q was accepted", key)
		}
	}
}

func TestSuiteDirectoryConfigKeyCanBeSetAndCleared(t *testing.T) {
	cfg := config.Defaults()
	const directory = "0x1111111111111111111111111111111111111111"
	if err := setConfigField(&cfg, "evm_suite_directory_address", directory); err != nil {
		t.Fatal(err)
	}
	if cfg.EVMSuiteDirectoryAddress != directory {
		t.Fatalf("directory = %q", cfg.EVMSuiteDirectoryAddress)
	}
	if err := setConfigField(&cfg, "evm_suite_directory_address", ""); err != nil {
		t.Fatal(err)
	}
	if cfg.EVMSuiteDirectoryAddress != "" {
		t.Fatalf("cleared directory = %q", cfg.EVMSuiteDirectoryAddress)
	}
}

func TestOrdinaryCommandsFailClosedWithoutSuiteDirectory(t *testing.T) {
	cfg := config.Defaults()
	cfg.KeyName = "dev"
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "repos", run: func() error { return cmdRepos(cfg, []string{"0x1111111111111111111111111111111111111111"}) }},
		{name: "collaborators", run: func() error {
			return cmdCollab(cfg, []string{"list", "0x1111111111111111111111111111111111111111", "demo"})
		}},
		{name: "badge", run: func() error { return cmdBadge(cfg, []string{"list", "0x1111111111111111111111111111111111111111"}) }},
		{name: "suite", run: func() error { return cmdSuite(cfg, []string{"verify"}) }},
		{name: "username", run: func() error { return cmdUsername(cfg, []string{"show", "alice"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if !i18n.HasCode(err, config.ErrorCodeMissingEVMSuiteDirectory) {
				t.Fatalf("error = %v, want missing SuiteDirectory", err)
			}
		})
	}
}

func TestCommandArgumentValidationRunsBeforeBackendSelection(t *testing.T) {
	cfg := config.Defaults()
	cfg.KeyName = "dev"
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "collaborator role", run: func() error {
			return cmdCollab(cfg, []string{"add", "demo", "0x3333333333333333333333333333333333333333", "admin"})
		}, want: "maintainer"},
		{name: "collaborator arity", run: func() error {
			return cmdCollab(cfg, []string{"add", "demo", "0x3333333333333333333333333333333333333333", "reader", "extra"})
		}, want: "collab add"},
		{name: "repo branch arity", run: func() error {
			return cmdRepo(cfg, []string{"edit", "demo", "branch", "main", "extra"})
		}, want: "repo edit"},
		{name: "suite flag", run: func() error { return cmdSuite(cfg, []string{"verify", "--yaml"}) }, want: "suite"},
		{name: "key import arity", run: func() error { return cmdKey(cfg, []string{"import"}) }, want: "key import"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseINJUsesEighteenDecimalsAndRejectsInvalidValues(t *testing.T) {
	got, err := parseINJ("0.5")
	if err != nil || got != "500000000000000000inj" {
		t.Fatalf("parse 0.5 = %q, %v", got, err)
	}
	for _, invalid := range []string{"0", "-1", "1.0000000000000000001", "abc"} {
		if _, err := parseINJ(invalid); err == nil {
			t.Fatalf("invalid amount %q accepted", invalid)
		}
	}
}

func TestShortAddressHandlesShortAndLongValues(t *testing.T) {
	if got := shortAddress("short"); got != "short" {
		t.Fatalf("short address = %q", got)
	}
	if got := shortAddress("123456789012345"); got != "123456789012…" {
		t.Fatalf("long address = %q", got)
	}
}

func TestCmdKeyNewResolvesFreshEVMKeyAddress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IGIT_HOME", home)
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-only-password")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = filepath.Join(home, "keystore")

	if err := cmdKey(cfg, []string{"new", "dev"}); err != nil {
		t.Fatalf("cmdKey new = %v", err)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.KeyName != "dev" {
		t.Fatalf("saved key name = %q, want dev", saved.KeyName)
	}
	address, err := chain.NewEVMKeystoreSigner(saved).OwnerAddress()
	if err != nil {
		t.Fatalf("resolve fresh EVM address = %v", err)
	}
	if !strings.HasPrefix(address, "inj1") {
		t.Fatalf("address = %q, want inj1 bech32", address)
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/fileprotection"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

func TestEffectiveGatewaysUsesProfileNameForLegacyDefault(t *testing.T) {
	gateways := Defaults().EffectiveGateways()
	if len(gateways) != 2 || gateways[0].Name != "hk" || gateways[1].Name != "us" {
		t.Fatalf("gateways = %#v, want hk then us", gateways)
	}
}

func TestEffectiveGatewaysPrependsCustomLegacyGateway(t *testing.T) {
	cfg := Defaults()
	cfg.IPFSGateway = "https://custom.example/"
	gateways := cfg.EffectiveGateways()
	if len(gateways) != 3 || gateways[0].Name != "custom" || gateways[0].URL != "https://custom.example" {
		t.Fatalf("gateways = %#v, want custom first", gateways)
	}
}

func TestEffectiveGatewaysKeepsPublicLegacyGatewayAsFallback(t *testing.T) {
	cfg := Defaults()
	cfg.IPFSGateway = "https://ipfs.io/"
	gateways := cfg.EffectiveGateways()
	if len(gateways) != 2 || gateways[0].Name != "hk" || gateways[1].Name != "us" {
		t.Fatalf("gateways = %#v, want project gateways before public fallback", gateways)
	}
	fallbacks := cfg.EffectiveReadFallbacks()
	if len(fallbacks) != 1 || fallbacks[0] != "https://ipfs.io" {
		t.Fatalf("fallbacks = %#v, want ipfs.io", fallbacks)
	}
}

func TestEffectiveUploadPeersUseBuiltInDefaultsForLegacyEmptyValues(t *testing.T) {
	cfg := Defaults()
	cfg.Upload.USPeer = ""
	cfg.Upload.HKPeer = ""
	peers := cfg.EffectiveUploadPeers()
	if len(peers) != 2 || peers[0] != DefaultUSUploadPeer || peers[1] != DefaultHKUploadPeer {
		t.Fatalf("peers = %#v, want built-in US then HK", peers)
	}
}

func TestDefaultsSelectEVMSuiteWithoutUnverifiedDirectory(t *testing.T) {
	cfg := Defaults()
	if cfg.Network != "injective-testnet" {
		t.Fatalf("network = %q, want injective-testnet", cfg.Network)
	}
	if cfg.EffectiveContractBackend() != "evm" {
		t.Fatalf("backend = %q, want evm", cfg.EffectiveContractBackend())
	}
	if cfg.EffectiveContractVersion() != "v3" {
		t.Fatalf("version = %q, want v3", cfg.EffectiveContractVersion())
	}
	if cfg.EffectiveEVMSuiteDirectoryAddress() != "" {
		t.Fatalf("unverified default directory = %q", cfg.EffectiveEVMSuiteDirectoryAddress())
	}
}

func TestPublishedProfilesRejectUnverifiedDirectoryUntilCutover(t *testing.T) {
	if err := ValidatePublishedSuiteProfiles(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedProfileGuardWalksEntireMap(t *testing.T) {
	const name = "unreviewed-profile"
	networkProfiles[name] = NetworkProfile{Name: name, EVMSuiteDirectory: "0x1111111111111111111111111111111111111111"}
	t.Cleanup(func() { delete(networkProfiles, name) })
	if err := ValidatePublishedSuiteProfiles(); err == nil {
		t.Fatal("profile with an unreviewed SuiteDirectory unexpectedly passed")
	}
}

func TestLegacyMetadataCannotChangeEffectiveRuntime(t *testing.T) {
	cfg := Config{}
	if cfg.EffectiveContractBackend() != "evm" {
		t.Fatalf("empty backend = %q, want evm", cfg.EffectiveContractBackend())
	}
	if cfg.EffectiveContractVersion() != "v3" {
		t.Fatalf("empty version = %q, want v3", cfg.EffectiveContractVersion())
	}
	cfg.ContractBackend = "cosmwasm"
	cfg.ContractVersion = "v1"
	if cfg.EffectiveContractBackend() != "evm" {
		t.Fatalf("legacy backend changed runtime to %q", cfg.EffectiveContractBackend())
	}
	if cfg.EffectiveContractVersion() != "v3" {
		t.Fatalf("legacy version changed runtime to %q", cfg.EffectiveContractVersion())
	}
}

func TestValidateContractRequiresSuiteDirectory(t *testing.T) {
	cfg := Defaults()
	if err := cfg.ValidateContract(); !i18n.HasCode(err, ErrorCodeMissingEVMSuiteDirectory) {
		t.Fatalf("missing SuiteDirectory error = %v, want code %q", err, ErrorCodeMissingEVMSuiteDirectory)
	}
	cfg.EVMSuiteDirectoryAddress = "inj1notanevmaddress"
	if err := cfg.ValidateContract(); !i18n.HasCode(err, ErrorCodeInvalidEVMSuiteDirectory) {
		t.Fatalf("malformed SuiteDirectory error = %v, want code %q", err, ErrorCodeInvalidEVMSuiteDirectory)
	}
	cfg.EVMSuiteDirectoryAddress = "0x1111111111111111111111111111111111111111"
	if err := cfg.ValidateContract(); err != nil {
		t.Fatalf("valid SuiteDirectory profile rejected: %v", err)
	}
}

func TestValidateContractUsesStableTransportErrorCodes(t *testing.T) {
	const directory = "0x1111111111111111111111111111111111111111"
	withoutRPC := Config{EVMSuiteDirectoryAddress: directory, EVMChainID: 1439}
	if err := withoutRPC.ValidateContract(); !i18n.HasCode(err, ErrorCodeMissingEVMRPC) {
		t.Fatalf("missing RPC error = %v, want code %q", err, ErrorCodeMissingEVMRPC)
	}
	withoutChainID := Config{EVMSuiteDirectoryAddress: directory, EVMRPC: "https://rpc.example.invalid"}
	if err := withoutChainID.ValidateContract(); !i18n.HasCode(err, ErrorCodeMissingEVMChainID) {
		t.Fatalf("missing chain ID error = %v, want code %q", err, ErrorCodeMissingEVMChainID)
	}
}

func TestSaveProtectsConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IGIT_HOME", home)
	path := filepath.Join(home, "config.json")
	if err := os.WriteFile(path, []byte("legacy permissive config\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	resolved, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("config path = %q, want %q", resolved, path)
	}
	if err := fileprotection.ValidateFile(resolved); err != nil {
		t.Fatalf("saved config does not satisfy sensitive-file policy: %v", err)
	}
}

func TestLoadRepairsLegacyConfigProtectionBeforeReading(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IGIT_HOME", home)
	path := filepath.Join(home, "config.json")
	if err := os.WriteFile(path, []byte(`{"upload":{"authorization":"legacy-secret"}}`), 0o666); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upload.Authorization != "legacy-secret" {
		t.Fatalf("loaded authorization = %q", cfg.Upload.Authorization)
	}
	if err := fileprotection.ValidateDirectory(home); err != nil {
		t.Fatalf("legacy config directory was not repaired: %v", err)
	}
	if err := fileprotection.ValidateFile(path); err != nil {
		t.Fatalf("legacy config file was not repaired: %v", err)
	}
}

func TestValidateContractErrorRendering(t *testing.T) {
	for _, test := range []struct {
		name   string
		locale string
		want   string
	}{
		{
			name:   "english",
			locale: "en-US",
			want:   `invalid EVM SuiteDirectory address "inj1notanevmaddress" (expected 0x followed by 40 hex characters)`,
		},
		{
			name:   "chinese",
			locale: "zh-CN",
			want:   `EVM SuiteDirectory 地址 "inj1notanevmaddress" 无效（应为 0x 后跟 40 个十六进制字符）`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LC_ALL", test.locale)
			cfg := Defaults()
			cfg.EVMSuiteDirectoryAddress = "inj1notanevmaddress"
			err := cfg.ValidateContract()
			if err == nil || err.Error() != test.want {
				t.Fatalf("validation error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestNetworkProfileDerivesEVMTransportDefaults(t *testing.T) {
	cfg := ApplyNetworkProfile(Config{Network: "injective-testnet"})
	if cfg.EffectiveEVMRPC() != "https://k8s.testnet.json-rpc.injective.network" {
		t.Fatalf("EVM RPC = %q", cfg.EffectiveEVMRPC())
	}
	if cfg.EffectiveEVMChainID() != 1439 {
		t.Fatalf("EVM chain ID = %d, want 1439", cfg.EffectiveEVMChainID())
	}
	if cfg.EffectiveEVMSuiteDirectoryAddress() != "" {
		t.Fatalf("SuiteDirectory = %q, want unset until evidence-approved deployment", cfg.EffectiveEVMSuiteDirectoryAddress())
	}
}

func TestModuleValidationUsesOnlySuiteDirectory(t *testing.T) {
	cfg := Config{
		Network:                  "injective-testnet",
		EVMSuiteDirectoryAddress: "0x1111111111111111111111111111111111111111",
		EVMRPC:                   "https://example.invalid",
		EVMChainID:               1439,
	}
	for name, validate := range map[string]func() error{
		"badge":      cfg.ValidateBadgeModule,
		"economic":   cfg.ValidateEconomicModule,
		"moderation": cfg.ValidateModerationModule,
	} {
		if err := validate(); err != nil {
			t.Fatalf("%s validation rejected suite directory: %v", name, err)
		}
	}
}

func TestSelectNetworkProfileReplacesAllProfileOwnedFields(t *testing.T) {
	cfg := Defaults()
	cfg.ChainID = "stale-chain"
	cfg.LCDEndpoint = "https://stale-lcd.invalid"
	cfg.Node = "https://stale-rpc.invalid"
	cfg.EVMRPC = "https://stale-evm.invalid"
	cfg.EVMChainID = 999
	cfg.EVMExplorer = "https://stale-explorer.invalid"
	cfg.EVMSuiteDirectoryAddress = "0x5555555555555555555555555555555555555555"
	cfg.ContractAddress = DefaultContractAddress
	cfg.EVMContractAddress = "0x1111111111111111111111111111111111111111"
	cfg.EVMBadgeModuleAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMEconomicModuleAddress = "0x3333333333333333333333333333333333333333"
	cfg.EVMModerationModuleAddress = "0x4444444444444444444444444444444444444444"

	selected, err := SelectNetworkProfile(cfg, " INJECTIVE-MAINNET ")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Network != "injective-mainnet" || selected.ChainID != "injective-1" || selected.EVMChainID != 1776 {
		t.Fatalf("selected profile identity = %#v", selected)
	}
	if selected.LCDEndpoint != "https://lcd.injective.network" || selected.Node != "https://tm.injective.network" || selected.EVMRPC != "https://sentry.evm-rpc.injective.network/" {
		t.Fatalf("selected transport fields = %#v", selected)
	}
	if selected.EVMSuiteDirectoryAddress != "" || selected.ContractAddress != "" || selected.EVMContractAddress != "" || selected.EVMBadgeModuleAddress != "" || selected.EVMEconomicModuleAddress != "" || selected.EVMModerationModuleAddress != "" {
		t.Fatalf("undeployed mainnet contracts/modules must be empty: %#v", selected)
	}
	if _, err := SelectNetworkProfile(cfg, "unknown"); err == nil {
		t.Fatal("unknown network profile was accepted")
	}
}

func TestSuiteValidationDoesNotRequireLegacyContract(t *testing.T) {
	cfg := Config{
		Network:                  "injective-mainnet",
		EVMSuiteDirectoryAddress: "0x1111111111111111111111111111111111111111",
		EVMRPC:                   "https://example.invalid",
		EVMChainID:               1776,
	}
	if err := cfg.ValidateContract(); err != nil {
		t.Fatalf("explicit EVM config rejected without legacy contract: %v", err)
	}
}

func TestSuiteValidationRejectsMalformedDirectoryAddress(t *testing.T) {
	cfg := Config{
		Network:                  "injective-testnet",
		EVMSuiteDirectoryAddress: "inj1legacyaddress",
		EVMRPC:                   "https://example.invalid",
		EVMChainID:               1439,
	}
	if err := cfg.ValidateContract(); err == nil {
		t.Fatal("malformed EVM contract address was accepted")
	}
}

func TestLoadUpgradesAndClearsLegacyContractFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IGIT_HOME", home)
	path := filepath.Join(home, "config.json")
	legacy := `{"network":"injective-testnet","contract_backend":"auto","contract_version":"v2","contract_address":"inj1legacy","evm_contract_address":"0x1111111111111111111111111111111111111111","evm_badge_module_address":"0x2222222222222222222222222222222222222222"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EffectiveContractBackend() != "evm" || cfg.EffectiveContractVersion() != "v3" {
		t.Fatalf("upgraded runtime = %s/%s", cfg.EffectiveContractBackend(), cfg.EffectiveContractVersion())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacyKey := range []string{"contract_backend", "contract_version", "contract_address", "evm_contract_address", "evm_badge_module_address"} {
		if strings.Contains(string(data), legacyKey) {
			t.Fatalf("upgraded config retained %q: %s", legacyKey, data)
		}
	}
}

func TestLoadAppliesSelectedProfileWithoutOverwritingExplicitEndpoints(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IGIT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"network":"injective-mainnet"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChainID != "injective-1" || cfg.LCDEndpoint != "https://lcd.injective.network" || cfg.Node != "https://tm.injective.network" {
		t.Fatalf("mainnet profile endpoints = %#v", cfg)
	}
	if cfg.EffectiveEVMRPC() != "https://sentry.evm-rpc.injective.network" || cfg.EffectiveEVMChainID() != 1776 {
		t.Fatalf("mainnet EVM profile = rpc %q chain %d", cfg.EffectiveEVMRPC(), cfg.EffectiveEVMChainID())
	}

	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"network":"injective-mainnet","node":"http://custom.invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Node != "http://custom.invalid" {
		t.Fatalf("explicit node was overwritten: %q", cfg.Node)
	}
}

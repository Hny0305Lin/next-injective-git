package config

import (
	"os"
	"path/filepath"
	"testing"
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

func TestDefaultsExposeBackendMigrationMetadata(t *testing.T) {
	cfg := Defaults()
	if cfg.Network != "injective-testnet" {
		t.Fatalf("network = %q, want injective-testnet", cfg.Network)
	}
	if cfg.EffectiveContractBackend() != "auto" {
		t.Fatalf("backend = %q, want auto", cfg.EffectiveContractBackend())
	}
	if cfg.EffectiveContractVersion() != "v1" {
		t.Fatalf("version = %q, want v1 during compatibility period", cfg.EffectiveContractVersion())
	}
}

// TestPublishedProfilesRemainV1OnlyUntilReviewedCutover is the semantic release
// guard used by the tag workflow. A reviewed V2 release must replace this gate
// with one that verifies its deployment manifest and cutover evidence.
func TestPublishedProfilesRemainV1OnlyUntilReviewedCutover(t *testing.T) {
	if err := ValidatePublishedProfilesV1Only(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedProfileGuardWalksEntireMap(t *testing.T) {
	const name = "unreviewed-profile"
	networkProfiles[name] = NetworkProfile{Name: name, EVMContract: "0x1111111111111111111111111111111111111111"}
	t.Cleanup(func() { delete(networkProfiles, name) })
	if err := ValidatePublishedProfilesV1Only(); err == nil {
		t.Fatal("profile with an unreviewed V2 address unexpectedly passed")
	}
}

func TestPublishedProfileGuardRejectsAnyBadgeModuleAddress(t *testing.T) {
	const name = "unreviewed-badge-profile"
	networkProfiles[name] = NetworkProfile{
		Name:           name,
		EVMBadgeModule: "0x2222222222222222222222222222222222222222",
	}
	t.Cleanup(func() { delete(networkProfiles, name) })
	if err := ValidatePublishedProfilesV1Only(); err == nil {
		t.Fatal("profile with an unreviewed badge module address unexpectedly passed")
	}
}

func TestPublishedProfileGuardRejectsAnyEconomicModuleAddress(t *testing.T) {
	const name = "unreviewed-economic-profile"
	networkProfiles[name] = NetworkProfile{
		Name:              name,
		EVMEconomicModule: "0x3333333333333333333333333333333333333333",
	}
	t.Cleanup(func() { delete(networkProfiles, name) })
	if err := ValidatePublishedProfilesV1Only(); err == nil {
		t.Fatal("profile with an unreviewed economic module address unexpectedly passed")
	}
}

func TestLegacyConfigUsesCompatibleBackendDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.EffectiveContractBackend() != "auto" {
		t.Fatalf("empty backend = %q, want auto", cfg.EffectiveContractBackend())
	}
	if cfg.EffectiveContractVersion() != "v1" {
		t.Fatalf("empty version = %q, want v1", cfg.EffectiveContractVersion())
	}
	cfg.ContractBackend = " EVM "
	if cfg.EffectiveContractBackend() != "evm" {
		t.Fatalf("normalized backend = %q, want evm", cfg.EffectiveContractBackend())
	}
}

func TestValidateContractRejectsUnknownBackendMetadata(t *testing.T) {
	cfg := Defaults()
	cfg.ContractAddress = DefaultContractAddress
	cfg.ContractBackend = "unknown"
	if err := cfg.ValidateContract(); err == nil {
		t.Fatal("unknown contract backend was accepted")
	}

	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v9"
	if err := cfg.ValidateContract(); err == nil {
		t.Fatal("unknown contract version was accepted")
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
	if cfg.EffectiveEVMContractAddress() != "" {
		t.Fatalf("EVM contract address = %q, want unset until deployment", cfg.EffectiveEVMContractAddress())
	}
	if cfg.EffectiveEVMEconomicModuleAddress() != "" {
		t.Fatalf("EVM economic module address = %q, want unset until deployment", cfg.EffectiveEVMEconomicModuleAddress())
	}
	if cfg.EffectiveEVMModerationModuleAddress() != "" {
		t.Fatalf("EVM moderation module address = %q, want unset until deployment", cfg.EffectiveEVMModerationModuleAddress())
	}
}

func TestValidateEconomicModuleRequiresReviewedV2Address(t *testing.T) {
	cfg := Config{
		Network:            "injective-testnet",
		ContractBackend:    "evm",
		ContractVersion:    "v2",
		EVMContractAddress: "0x1111111111111111111111111111111111111111",
		EVMRPC:             "https://example.invalid",
		EVMChainID:         1439,
	}
	if err := cfg.ValidateEconomicModule(); err == nil {
		t.Fatal("missing economic module address was accepted")
	}
	cfg.EVMEconomicModuleAddress = "inj1notanevmaddress"
	if err := cfg.ValidateEconomicModule(); err == nil {
		t.Fatal("malformed economic module address was accepted")
	}
	cfg.EVMEconomicModuleAddress = "0x3333333333333333333333333333333333333333"
	if err := cfg.ValidateEconomicModule(); err != nil {
		t.Fatalf("reviewed economic module address rejected: %v", err)
	}
}

func TestValidateModerationModuleRequiresReviewedV2Address(t *testing.T) {
	cfg := Config{
		Network:            "injective-testnet",
		ContractBackend:    "evm",
		ContractVersion:    "v2",
		EVMContractAddress: "0x1111111111111111111111111111111111111111",
		EVMRPC:             "https://example.invalid",
		EVMChainID:         1439,
	}
	if err := cfg.ValidateModerationModule(); err == nil {
		t.Fatal("missing moderation module address was accepted")
	}
	cfg.EVMModerationModuleAddress = "inj1notanevmaddress"
	if err := cfg.ValidateModerationModule(); err == nil {
		t.Fatal("malformed moderation module address was accepted")
	}
	cfg.EVMModerationModuleAddress = "0x4444444444444444444444444444444444444444"
	if err := cfg.ValidateModerationModule(); err != nil {
		t.Fatalf("reviewed moderation module address rejected: %v", err)
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
	if selected.LCDEndpoint != "https://lcd.injective.network" || selected.Node != "https://tm.injective.network" || selected.EVMRPC != "https://k8s.json-rpc.injective.network" {
		t.Fatalf("selected transport fields = %#v", selected)
	}
	if selected.ContractAddress != "" || selected.EVMContractAddress != "" || selected.EVMBadgeModuleAddress != "" || selected.EVMEconomicModuleAddress != "" || selected.EVMModerationModuleAddress != "" {
		t.Fatalf("undeployed mainnet contracts/modules must be empty: %#v", selected)
	}
	if _, err := SelectNetworkProfile(cfg, "unknown"); err == nil {
		t.Fatal("unknown network profile was accepted")
	}
}

func TestExplicitEVMValidationDoesNotRequireLegacyContract(t *testing.T) {
	cfg := Config{
		Network:            "injective-mainnet",
		ContractBackend:    "evm",
		ContractVersion:    "v2",
		EVMContractAddress: "0x1111111111111111111111111111111111111111",
		EVMRPC:             "https://example.invalid",
		EVMChainID:         1776,
	}
	if err := cfg.ValidateContract(); err != nil {
		t.Fatalf("explicit EVM config rejected without legacy contract: %v", err)
	}
}

func TestExplicitEVMValidationRejectsMalformedContractAddress(t *testing.T) {
	cfg := Config{
		Network:            "injective-testnet",
		ContractBackend:    "evm",
		ContractVersion:    "v2",
		EVMContractAddress: "inj1legacyaddress",
		EVMRPC:             "https://example.invalid",
		EVMChainID:         1439,
	}
	if err := cfg.ValidateContract(); err == nil {
		t.Fatal("malformed EVM contract address was accepted")
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
	if cfg.EffectiveEVMRPC() != "https://k8s.json-rpc.injective.network" || cfg.EffectiveEVMChainID() != 1776 {
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

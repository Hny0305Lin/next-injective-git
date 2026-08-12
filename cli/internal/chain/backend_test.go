package chain

import (
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestSelectRegistryBackendDefaultsToCosmWasmV1(t *testing.T) {
	cfg := config.Defaults()
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*CosmWasmRegistryV1); !ok {
		t.Fatalf("backend = %T, want *CosmWasmRegistryV1", backend)
	}
}

func TestSelectRegistryBackendHonorsExplicitCosmWasm(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = " COSMWASM "
	cfg.ContractVersion = "v2"
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*CosmWasmRegistryV1); !ok {
		t.Fatalf("backend = %T, want *CosmWasmRegistryV1", backend)
	}
}

func TestSelectRegistryBackendSelectsEVMV2(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v2"
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*EVMRegistryV2); !ok {
		t.Fatalf("backend = %T, want *EVMRegistryV2", backend)
	}
}

func TestEVMReadFallbackIsLimitedToAutoV2CompatibilityMode(t *testing.T) {
	cases := []struct {
		name    string
		backend string
		version string
		want    bool
	}{
		{name: "auto v2", backend: "auto", version: "v2", want: true},
		{name: "explicit evm", backend: "evm", version: "v2", want: false},
		{name: "explicit v2 alias", backend: "v2", version: "v2", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.ContractBackend = tc.backend
			cfg.ContractVersion = tc.version
			cfg.ContractAddress = config.DefaultContractAddress
			if got := legacyReadFallback(cfg) != nil; got != tc.want {
				t.Fatalf("legacyReadFallback = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsesEVMBackendOnlyForEffectiveV2Selection(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want bool
	}{
		{name: "legacy default", cfg: config.Defaults(), want: false},
		{name: "auto v2", cfg: func() config.Config {
			cfg := config.Defaults()
			cfg.ContractVersion = "v2"
			return cfg
		}(), want: true},
		{name: "explicit evm", cfg: func() config.Config {
			cfg := config.Defaults()
			cfg.ContractBackend = "evm"
			return cfg
		}(), want: true},
		{name: "explicit legacy", cfg: func() config.Config {
			cfg := config.Defaults()
			cfg.ContractBackend = "cosmwasm"
			cfg.ContractVersion = "v2"
			return cfg
		}(), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UsesEVMBackend(tc.cfg); got != tc.want {
				t.Fatalf("UsesEVMBackend = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectRegistryBackendRejectsUnknownSelector(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "mystery"
	_, err := SelectRegistryBackend(cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown contract backend") {
		t.Fatalf("error = %v, want unknown backend error", err)
	}
}

func TestLegacyClientAdapterSatisfiesBackendBoundaries(t *testing.T) {
	cfg := config.Defaults()
	legacy := NewCosmWasmRegistryV1(cfg)
	if _, ok := any(legacy).(RepoRegistryBackend); !ok {
		t.Fatal("legacy adapter does not satisfy RepoRegistryBackend")
	}
	if _, ok := any(legacy).(OwnershipRegistryBackend); !ok {
		t.Fatal("legacy adapter does not satisfy OwnershipRegistryBackend")
	}
	if _, ok := any(legacy).(TransferBackend); !ok {
		t.Fatal("legacy adapter does not satisfy TransferBackend")
	}
	if _, ok := any(legacy).(ModerationBackend); !ok {
		t.Fatal("legacy adapter does not satisfy ModerationBackend")
	}
}

func TestModerationBackendFollowsSelectedRegistryWithoutV1Fallback(t *testing.T) {
	cfg := config.Defaults()
	legacy, err := NewModerationBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy.(*CosmWasmRegistryV1); !ok {
		t.Fatalf("legacy moderation backend = %T", legacy)
	}

	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMModerationModuleAddress = testModerationModuleAddress
	selected, err := NewModerationBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	module, ok := selected.(*EVMModerationModule)
	if !ok {
		t.Fatalf("V2 moderation backend = %T", selected)
	}
	if module.registry.legacy != nil {
		t.Fatal("explicit EVM moderation backend retained a legacy adapter")
	}
}

func TestCosmosSignerAndTransferAdaptersSelectWithV1(t *testing.T) {
	cfg := config.Defaults()
	signer, err := NewSignerBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := signer.(*CosmosSigner); !ok {
		t.Fatalf("signer = %T, want *CosmosSigner", signer)
	}
	transfer, err := NewTransferBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := transfer.(*CosmosTransfer); !ok {
		t.Fatalf("transfer = %T, want *CosmosTransfer", transfer)
	}
}

func TestEVMTransferSelectorUsesV2Adapter(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	transfer, err := NewTransferBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := transfer.(*EVMTransfer); !ok {
		t.Fatalf("transfer = %T, want *EVMTransfer", transfer)
	}
}

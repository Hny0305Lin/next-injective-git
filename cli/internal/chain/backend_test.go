package chain

import (
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestSelectRegistryBackendAlwaysUsesImmutableEVMSuite(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "cosmwasm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = config.DefaultContractAddress
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*EVMSuiteRegistry); !ok {
		t.Fatalf("backend = %T, want *EVMSuiteRegistry", backend)
	}
	if !UsesEVMBackend(cfg) {
		t.Fatal("legacy metadata changed the fixed EVM backend")
	}
}

func TestSuiteAdapterSatisfiesRuntimeBackendBoundaries(t *testing.T) {
	cfg := config.Defaults()
	suite := NewEVMSuiteRegistry(cfg)
	for name, ok := range map[string]bool{
		"registry":   any(suite).(RepoRegistryBackend) != nil,
		"ownership":  any(suite).(OwnershipRegistryBackend) != nil,
		"recovery":   any(suite).(RecoveryBackend) != nil,
		"moderation": any(suite).(ModerationBackend) != nil,
		"economic":   any(suite).(EconomicBackend) != nil,
		"badge":      any(suite).(BadgeBackend) != nil,
		"username":   any(suite).(UsernameBackend) != nil,
		"release":    any(suite).(ReleaseBackend) != nil,
		"fork":       any(suite).(ForkBackend) != nil,
	} {
		if !ok {
			t.Fatalf("suite does not satisfy %s backend", name)
		}
	}
}

func TestRuntimeSignerIsEVMOnly(t *testing.T) {
	cfg := config.Defaults()
	signer, err := NewSignerBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := signer.(*EVMKeystoreSigner); !ok {
		t.Fatalf("signer = %T, want *EVMKeystoreSigner", signer)
	}
}

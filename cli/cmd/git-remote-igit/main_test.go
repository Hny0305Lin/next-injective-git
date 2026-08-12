package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestRemoteHelperEntrypointKeepsGitProtocolOnStandardStreams(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "os.Stdin, os.Stdout, os.Stderr") {
		t.Fatal("remote helper no longer wires Git protocol to the process standard streams")
	}
	for _, forbidden := range []string{"keyPassword(", "readEVMKeystorePassword(", "term.ReadPassword("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("remote helper entrypoint must not prompt through Git protocol streams: found %s", forbidden)
		}
	}
}

func TestValidateContractSelectionUsesEVMAddressForV2(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.ContractAddress = ""
	if err := validateContractSelection(cfg); err == nil || !strings.Contains(err.Error(), "EVM V2 contract address") {
		t.Fatalf("validation error = %v, want EVM address error", err)
	}
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	if err := validateContractSelection(cfg); err != nil {
		t.Fatalf("valid EVM selection rejected: %v", err)
	}
	cfg.EVMContractAddress = "inj1legacyaddress"
	if err := validateContractSelection(cfg); err == nil || !strings.Contains(err.Error(), "invalid EVM V2 contract address") {
		t.Fatalf("validation error = %v, want invalid EVM address error", err)
	}
}

func TestValidateContractSelectionUsesLegacyAddressForV1(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v1"
	cfg.ContractAddress = ""
	if err := validateContractSelection(cfg); err == nil || !strings.Contains(err.Error(), "contract_address") {
		t.Fatalf("validation error = %v, want legacy address error", err)
	}
	cfg.ContractAddress = config.DefaultContractAddress
	if err := validateContractSelection(cfg); err != nil {
		t.Fatalf("valid legacy selection rejected: %v", err)
	}
}

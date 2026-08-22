package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
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

func TestValidateContractSelectionRequiresSuiteDirectory(t *testing.T) {
	cfg := config.Defaults()
	if err := validateContractSelection(cfg); !i18n.HasCode(err, config.ErrorCodeMissingEVMSuiteDirectory) {
		t.Fatalf("validation error = %v, want SuiteDirectory error", err)
	}
	cfg.EVMSuiteDirectoryAddress = "0x2222222222222222222222222222222222222222"
	if err := validateContractSelection(cfg); err != nil {
		t.Fatalf("valid suite selection rejected: %v", err)
	}
	cfg.EVMSuiteDirectoryAddress = "inj1legacyaddress"
	if err := validateContractSelection(cfg); !i18n.HasCode(err, config.ErrorCodeInvalidEVMSuiteDirectory) {
		t.Fatalf("validation error = %v, want invalid SuiteDirectory error", err)
	}
}

func TestValidateContractSelectionIgnoresLegacyBackendFields(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "auto"
	cfg.ContractVersion = "v1"
	cfg.ContractAddress = config.DefaultContractAddress
	cfg.EVMContractAddress = "0x3333333333333333333333333333333333333333"
	if err := validateContractSelection(cfg); !i18n.HasCode(err, config.ErrorCodeMissingEVMSuiteDirectory) {
		t.Fatalf("validation error = %v, want SuiteDirectory error despite legacy fields", err)
	}
}

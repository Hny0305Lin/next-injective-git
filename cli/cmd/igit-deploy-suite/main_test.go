package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/suitedeploy"
)

func commandArtifactDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "contracts", "evm-v2", "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckIsOfflineAndDoesNotLoadConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	loaded := false
	code := runWithServices(context.Background(), []string{"--check", "--artifacts", commandArtifactDirectory(t)}, &stdout, &stderr, commandServices{
		loadConfig: func() (config.Config, error) {
			loaded = true
			return config.Config{}, errors.New("must not load")
		},
		inspect:   suitedeploy.InspectArtifacts,
		verifyGit: func(string, string) error { t.Fatal("git verifier called in check mode"); return nil },
	})
	if code != 0 || loaded || !strings.Contains(stdout.String(), `"artifact_set_sha256"`) || stderr.Len() != 0 {
		t.Fatalf("code=%d loaded=%t stdout=%q stderr=%q", code, loaded, stdout.String(), stderr.String())
	}
}

func TestBroadcastRequiresExactSourceCommitConfirmationBeforeConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	loaded := false
	commit := "0123456789abcdef0123456789abcdef01234567"
	code := runWithServices(context.Background(), []string{
		"--artifacts", commandArtifactDirectory(t), "--output", filepath.Join(t.TempDir(), "deployment.json"),
		"--source-commit", commit, "--snapshot-root", "0x" + strings.Repeat("11", 32),
	}, &stdout, &stderr, commandServices{
		loadConfig: func() (config.Config, error) { loaded = true; return config.Config{}, nil },
		inspect:    suitedeploy.InspectArtifacts,
		verifyGit:  func(string, string) error { t.Fatal("git verifier called before confirmation"); return nil },
	})
	if code != 2 || loaded || !strings.Contains(stderr.String(), "--confirm-source-commit "+commit) {
		t.Fatalf("code=%d loaded=%t stderr=%q", code, loaded, stderr.String())
	}
}

func TestSingleEOADeploymentRejectsMainnetBeforeConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	loaded := false
	commit := "0123456789abcdef0123456789abcdef01234567"
	code := runWithServices(context.Background(), []string{
		"--artifacts", commandArtifactDirectory(t), "--output", filepath.Join(t.TempDir(), "deployment.json"),
		"--source-commit", commit, "--confirm-source-commit", commit,
		"--snapshot-root", "0x" + strings.Repeat("11", 32), "--network", "injective-mainnet",
	}, &stdout, &stderr, commandServices{
		loadConfig: func() (config.Config, error) { loaded = true; return config.Config{}, nil },
		inspect:    suitedeploy.InspectArtifacts,
		verifyGit:  func(string, string) error { t.Fatal("git verifier called for mainnet"); return nil },
		deploy: func(context.Context, suitedeploy.Options, *chain.EVMRPC, *chain.EVMTransactor, string) (*suitedeploy.Manifest, error) {
			t.Fatal("deploy called for mainnet")
			return nil, nil
		},
	})
	if code != 2 || loaded || !strings.Contains(stderr.String(), "restricted to injective-testnet") {
		t.Fatalf("code=%d loaded=%t stderr=%q", code, loaded, stderr.String())
	}
}

func TestValidateRPCEndpoint(t *testing.T) {
	if got, err := validateRPCEndpoint("https://rpc.example.invalid/"); err != nil || got != "https://rpc.example.invalid" {
		t.Fatalf("endpoint = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "file:///tmp/rpc", "https://rpc.example.invalid/#secret"} {
		if _, err := validateRPCEndpoint(invalid); err == nil {
			t.Fatalf("invalid endpoint %q accepted", invalid)
		}
	}
}

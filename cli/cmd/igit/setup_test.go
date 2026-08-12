package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/bootstrap"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/environment"
)

func TestCmdSetupDefaultsToPushBootstrap(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = ""

	err := cmdSetup(cfg, nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "deployment") {
		t.Fatalf("cmdSetup without subcommand error = %v, want guarded EVM deployment error", err)
	}
}

func TestSetupPushEVMUsesKuboOnlyAndPersistsNativePath(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	cfg := evmSetupConfig()
	cfg.InjectivedBin = "must-not-be-used-by-v2-setup"
	wantIPFSBin := filepath.Join(t.TempDir(), "ipfs.exe")

	legacyCalls := 0
	kuboCalls := 0
	doctorCalls := 0
	services := setupServices{
		prepareLegacy: func(context.Context, config.Config, bootstrap.Options) (config.Config, bootstrap.Result, error) {
			legacyCalls++
			return config.Config{}, bootstrap.Result{}, fmt.Errorf("legacy bootstrap must not run")
		},
		prepareKubo: func(_ context.Context, got config.Config, opts bootstrap.Options) (config.Config, bootstrap.Result, error) {
			kuboCalls++
			if got.InjectivedBin != cfg.InjectivedBin {
				t.Fatalf("injectived config changed before Kubo setup: got %q want %q", got.InjectivedBin, cfg.InjectivedBin)
			}
			if !opts.Force || opts.SkipKubo {
				t.Fatalf("Kubo options = %#v, want Force and no SkipKubo", opts)
			}
			got.IPFSBin = wantIPFSBin
			return got, bootstrap.Result{
				IPFSBin:     wantIPFSBin,
				Installed:   []string{"Kubo test"},
				KuboStarted: true,
			}, nil
		},
		runDoctor: func(_ context.Context, got config.Config, mode environment.Mode) environment.Report {
			doctorCalls++
			if mode != environment.ModePush || got.IPFSBin != wantIPFSBin {
				t.Fatalf("doctor config=%#v mode=%q", got, mode)
			}
			return environment.Report{Mode: mode}
		},
	}

	if err := setupPushWithServices(cfg, setupOptions{yes: true, force: true}, services); err != nil {
		t.Fatalf("setupPushWithServices = %v", err)
	}
	if legacyCalls != 0 || kuboCalls != 1 || doctorCalls != 1 {
		t.Fatalf("calls legacy=%d kubo=%d doctor=%d", legacyCalls, kuboCalls, doctorCalls)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.IPFSBin != wantIPFSBin {
		t.Fatalf("saved ipfs_bin = %q, want %q", saved.IPFSBin, wantIPFSBin)
	}
	if saved.InjectivedBin != cfg.InjectivedBin {
		t.Fatalf("saved injectived_bin = %q, want unchanged %q", saved.InjectivedBin, cfg.InjectivedBin)
	}
}

func TestSetupPushEVMSkipKuboDoesNotInvokeInstaller(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	cfg := evmSetupConfig()
	services := setupServices{
		prepareLegacy: func(context.Context, config.Config, bootstrap.Options) (config.Config, bootstrap.Result, error) {
			t.Fatal("legacy bootstrap must not run")
			return config.Config{}, bootstrap.Result{}, nil
		},
		prepareKubo: func(context.Context, config.Config, bootstrap.Options) (config.Config, bootstrap.Result, error) {
			t.Fatal("Kubo installer must not run with --no-kubo")
			return config.Config{}, bootstrap.Result{}, nil
		},
		runDoctor: func(_ context.Context, _ config.Config, mode environment.Mode) environment.Report {
			return environment.Report{Mode: mode, Checks: []environment.Check{
				{Name: "key_name", Status: environment.StatusFail, Detail: "not configured"},
				{Name: "signing key", Status: environment.StatusFail, Detail: "not configured"},
				{Name: "Kubo CLI", Status: environment.StatusFail, Detail: "not configured"},
				{Name: "local Kubo API", Status: environment.StatusFail, Detail: "not reachable"},
			}}
		},
	}

	if err := setupPushWithServices(cfg, setupOptions{skipKubo: true}, services); err != nil {
		t.Fatalf("setup with --no-kubo = %v", err)
	}
}

func TestSetupPushEVMRejectsInvalidDeploymentBeforeKubo(t *testing.T) {
	t.Setenv("IGIT_HOME", t.TempDir())
	cfg := evmSetupConfig()
	cfg.EVMContractAddress = config.DefaultContractAddress
	kuboCalled := false
	services := setupServices{
		prepareKubo: func(context.Context, config.Config, bootstrap.Options) (config.Config, bootstrap.Result, error) {
			kuboCalled = true
			return config.Config{}, bootstrap.Result{}, nil
		},
	}

	err := setupPushWithServices(cfg, setupOptions{yes: true}, services)
	if err == nil || !strings.Contains(err.Error(), "invalid EVM V2 contract address") {
		t.Fatalf("invalid deployment error = %v", err)
	}
	if kuboCalled {
		t.Fatal("Kubo installer ran before deployment validation")
	}
}

func evmSetupConfig() config.Config {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = "0x1111111111111111111111111111111111111111"
	return cfg
}

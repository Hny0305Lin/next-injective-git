package environment

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestReportHealthy(t *testing.T) {
	report := Report{Checks: []Check{{Status: StatusOK}, {Status: StatusWarn}}}
	if !report.Healthy() {
		t.Fatal("warnings must not make a report unhealthy")
	}
	report.Checks = append(report.Checks, Check{Status: StatusFail})
	if report.Healthy() || len(report.Failures()) != 1 {
		t.Fatal("failure was not reported")
	}
}

func TestFormatINJ(t *testing.T) {
	tests := map[string]string{
		"0":                   "0",
		"1":                   "<0.000001",
		"1000000000000000000": "1",
		"1250000000000000000": "1.25",
	}
	for raw, want := range tests {
		amount, _ := new(big.Int).SetString(raw, 10)
		if got := formatINJ(amount); got != want {
			t.Errorf("formatINJ(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestPushPreflightReportsMissingRequirementsTogether(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractAddress = ""
	cfg.KeyName = ""
	cfg.InjectivedBin = t.TempDir() + "/missing-injectived"
	cfg.Node = ""
	cfg.IPFSAPI = ""
	err := PushPreflight(context.Background(), cfg, true)
	if err == nil {
		t.Fatal("expected incomplete environment")
	}
	for _, want := range []string{"contract", "key_name", "injectived", "Injective RPC", "local Kubo API"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestBackendCheckUsesUnifiedSelector(t *testing.T) {
	cfg := config.Defaults()
	check := backendCheck(cfg)
	if check.Status != StatusOK || check.Name != "chain backend" {
		t.Fatalf("default backend check = %#v, want OK chain backend", check)
	}

	cfg.ContractVersion = "v2"
	check = backendCheck(cfg)
	if check.Status != StatusFail || !contains(check.Detail, "EVM V2 contract address") {
		t.Fatalf("v2 backend check = %#v, want incomplete EVM profile failure", check)
	}

	cfg.ContractBackend = "mystery"
	check = backendCheck(cfg)
	if check.Status != StatusFail || !contains(check.Detail, "unsupported contract backend") {
		t.Fatalf("unknown backend check = %#v, want selector failure", check)
	}
}

func TestExplicitEVMDoesNotRequireLegacyInjectived(t *testing.T) {
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.KeyName = "dev"
	cfg.InjectivedBin = t.TempDir() + "/missing-injectived"
	err := PushPreflight(context.Background(), cfg, false)
	if err == nil {
		t.Fatal("unavailable EVM backend must fail preflight")
	}
	if contains(err.Error(), "injectived") {
		t.Fatalf("EVM preflight unexpectedly requires legacy injectived: %v", err)
	}
	if !contains(err.Error(), "EVM V2 contract address") {
		t.Fatalf("EVM preflight error = %v, want unified backend failure", err)
	}
}

func TestExplicitEVMPreflightChecksRPCWithoutConfiguredKey(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0x59f"})
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMRPC = server.URL
	cfg.KeyName = ""
	if err := PushPreflight(context.Background(), cfg, false); err == nil {
		t.Fatal("missing key should still fail preflight")
	} else if contains(err.Error(), "EVM RPC") {
		t.Fatalf("healthy EVM RPC was reported as a failure: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("EVM RPC calls = %d, want one preflight chain ID check", calls.Load())
	}
}

func TestExplicitEVMPreflightRejectsChainIDMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0x1"})
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v2"
	cfg.EVMContractAddress = "0x2222222222222222222222222222222222222222"
	cfg.EVMRPC = server.URL
	cfg.KeyName = "dev"
	err := PushPreflight(context.Background(), cfg, false)
	if err == nil || !contains(err.Error(), "chain ID mismatch") {
		t.Fatalf("error = %v, want chain ID mismatch", err)
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

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

func TestBalanceCheckUsesEVMRPC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method != "eth_getBalance" || len(request.Params) != 2 {
			t.Fatalf("request = %#v, want eth_getBalance with address and block tag", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0x1158e460913d00000"})
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.EVMRPC = server.URL
	check := balanceCheck(context.Background(), cfg, "0x1111111111111111111111111111111111111111")
	if check.Status != StatusOK || check.Detail != "20 INJ" {
		t.Fatalf("balance check = %#v", check)
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
	for _, want := range []string{"SuiteDirectory", "key_name", "local Kubo API"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestBackendCheckUsesUnifiedSelector(t *testing.T) {
	cfg := config.Defaults()
	check := backendCheck(cfg)
	if check.Status != StatusFail || check.Name != "chain backend" || !contains(check.Detail, "SuiteDirectory") {
		t.Fatalf("default backend check = %#v, want fail-closed missing Directory", check)
	}
	cfg.EVMSuiteDirectoryAddress = "0x2222222222222222222222222222222222222222"
	check = backendCheck(cfg)
	if check.Status != StatusOK || check.Detail != "evm v3 immutable suite" {
		t.Fatalf("suite backend check = %#v, want EVM v3", check)
	}
}

func TestExplicitEVMDoesNotRequireLegacyInjectived(t *testing.T) {
	cfg := config.Defaults()
	cfg.KeyName = "dev"
	cfg.InjectivedBin = t.TempDir() + "/missing-injectived"
	err := PushPreflight(context.Background(), cfg, false)
	if err == nil {
		t.Fatal("unavailable EVM backend must fail preflight")
	}
	if contains(err.Error(), "injectived") {
		t.Fatalf("EVM preflight unexpectedly requires legacy injectived: %v", err)
	}
	if !contains(err.Error(), "SuiteDirectory") {
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
	cfg.EVMSuiteDirectoryAddress = "0x2222222222222222222222222222222222222222"
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
	cfg.EVMSuiteDirectoryAddress = "0x2222222222222222222222222222222222222222"
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

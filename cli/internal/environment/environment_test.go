package environment

import (
	"context"
	"math/big"
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

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

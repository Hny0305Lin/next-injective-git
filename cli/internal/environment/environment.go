// Package environment diagnoses the local tools and services used by igit.
package environment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

type Mode string

const (
	ModeClone Mode = "clone"
	ModePush  Mode = "push"
)

type Status string

const (
	StatusOK   Status = "OK"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
	StatusSkip Status = "SKIP"
)

type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

type Report struct {
	Mode   Mode    `json:"mode"`
	Checks []Check `json:"checks"`
}

func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			return false
		}
	}
	return true
}

func (r Report) Failures() []Check {
	var failures []Check
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			failures = append(failures, check)
		}
	}
	return failures
}

// Run performs a complete clone or push environment diagnosis.
func Run(ctx context.Context, cfg config.Config, mode Mode) Report {
	r := Report{Mode: mode}
	r.Checks = append(r.Checks, commandCheck(ctx, "git", "git", []string{"--version"}, "install Git and ensure it is in PATH"))
	r.Checks = append(r.Checks, pathCheck("git-remote-igit", "git-remote-igit", "install git-remote-igit next to igit and add it to PATH"))
	if usesLegacyBackend(cfg) {
		r.Checks = append(r.Checks, configCheck("contract", cfg.ContractAddress, "run `igit config set contract_address "+config.DefaultContractAddress+"`"))
	} else {
		r.Checks = append(r.Checks, configCheck("contract", cfg.EffectiveEVMContractAddress(), "complete the named EVM deployment profile"))
	}
	r.Checks = append(r.Checks, backendCheck(cfg))
	if usesLegacyBackend(cfg) || cfg.EffectiveContractBackend() == "auto" {
		r.Checks = append(r.Checks, endpointCheck(ctx, "LCD", strings.TrimRight(cfg.LCDEndpoint, "/")+"/cosmos/base/tendermint/v1beta1/node_info", http.MethodGet, nil, "check lcd_endpoint and your network connection"))
	} else {
		r.Checks = append(r.Checks, Check{Name: "LCD", Status: StatusSkip, Detail: "legacy read fallback is disabled for explicit EVM V2"})
	}
	r.Checks = append(r.Checks, gatewayCheck(ctx, cfg))

	if mode == ModeClone {
		return r
	}
	r.Checks = append(r.Checks, configCheck("key_name", cfg.KeyName, "run `igit key new dev` or configure an existing key"))

	if usesLegacyBackend(cfg) {
		bin := injectivedBin(cfg)
		injective := commandCheck(ctx, "injectived", bin, []string{"version"}, "run `igit setup push` to install the pinned Injective CLI")
		r.Checks = append(r.Checks, injective)
		address := ""
		if injective.Status == StatusOK && strings.TrimSpace(cfg.KeyName) != "" {
			var keyCheck Check
			address, keyCheck = keyAddressCheck(ctx, cfg, bin)
			r.Checks = append(r.Checks, keyCheck)
		} else {
			r.Checks = append(r.Checks, Check{Name: "signing key", Status: StatusSkip, Detail: "configured signer is unavailable"})
		}
		if address != "" {
			r.Checks = append(r.Checks, balanceCheck(ctx, cfg, address))
		} else {
			r.Checks = append(r.Checks, Check{Name: "key balance", Status: StatusSkip, Detail: "signing address is unavailable"})
		}
		r.Checks = append(r.Checks, endpointCheck(ctx, "Injective RPC", strings.TrimRight(cfg.Node, "/")+"/status", http.MethodGet, nil, "check node and your network connection"))
	} else {
		signer, err := chain.NewSignerBackend(cfg)
		address := ""
		if err != nil {
			r.Checks = append(r.Checks, Check{Name: "signing key", Status: StatusFail, Detail: err.Error(), Fix: "run `igit key new dev`"})
		} else if strings.TrimSpace(cfg.KeyName) == "" {
			r.Checks = append(r.Checks, Check{Name: "signing key", Status: StatusSkip, Detail: "no key configured"})
		} else if address, err = signer.OwnerAddress(); err != nil {
			r.Checks = append(r.Checks, Check{Name: "signing key", Status: StatusFail, Detail: err.Error(), Fix: "run `igit key new " + cfg.KeyName + "`"})
		} else {
			r.Checks = append(r.Checks, Check{Name: "signing key", Status: StatusOK, Detail: cfg.KeyName + " " + address})
		}
		if address != "" {
			r.Checks = append(r.Checks, balanceCheck(ctx, cfg, address))
		} else {
			r.Checks = append(r.Checks, Check{Name: "key balance", Status: StatusSkip, Detail: "signing address is unavailable"})
		}
		r.Checks = append(r.Checks, evmRPCCheck(ctx, cfg))
	}
	r.Checks = append(r.Checks, commandCheck(ctx, "Kubo CLI", ipfsBin(cfg), []string{"version", "--number"}, "run `igit setup push` to install the pinned Kubo CLI"))
	r.Checks = append(r.Checks, endpointCheck(ctx, "local Kubo API", strings.TrimRight(cfg.IPFSAPI, "/")+"/api/v0/version", http.MethodPost, nil, "start Kubo with `igit setup push` or `ipfs daemon`"))
	r.Checks = append(r.Checks, uploadAuthorizationCheck(ctx, cfg))
	return r
}

// PushPreflight checks only requirements that must be ready before pack work.
// Deleting a ref sets needsKubo false because it sends only an on-chain tx.
func PushPreflight(ctx context.Context, cfg config.Config, needsKubo bool) error {
	var failures []Check
	add := func(check Check) {
		if check.Status == StatusFail {
			failures = append(failures, check)
		}
	}
	if usesLegacyBackend(cfg) {
		add(configCheck("contract", cfg.ContractAddress, "run `igit setup push`"))
	} else {
		add(configCheck("contract", cfg.EffectiveEVMContractAddress(), "complete the named EVM deployment profile"))
	}
	backend := backendCheck(cfg)
	add(backend)
	add(configCheck("key_name", cfg.KeyName, "run `igit key new dev` or configure an existing key"))
	if usesLegacyBackend(cfg) {
		bin := injectivedBin(cfg)
		injective := commandCheck(ctx, "injectived", bin, []string{"version"}, "run `igit setup push`")
		add(injective)
		if injective.Status == StatusOK && cfg.KeyName != "" {
			_, check := keyAddressCheck(ctx, cfg, bin)
			add(check)
		}
	}
	if usesLegacyBackend(cfg) {
		add(endpointCheck(ctx, "Injective RPC", strings.TrimRight(cfg.Node, "/")+"/status", http.MethodGet, nil, "check node and your network connection"))
	} else {
		if cfg.KeyName != "" {
			signer, err := chain.NewSignerBackend(cfg)
			if err != nil {
				add(Check{Name: "signing key", Status: StatusFail, Detail: err.Error(), Fix: "run `igit key new dev`"})
			} else if address, err := signer.OwnerAddress(); err != nil {
				add(Check{Name: "signing key", Status: StatusFail, Detail: err.Error(), Fix: "run `igit key new " + cfg.KeyName + "`"})
			} else {
				add(Check{Name: "signing key", Status: StatusOK, Detail: address})
			}
		}
		add(evmRPCCheck(ctx, cfg))
	}
	if needsKubo {
		add(endpointCheck(ctx, "local Kubo API", strings.TrimRight(cfg.IPFSAPI, "/")+"/api/v0/version", http.MethodPost, nil, "run `igit setup push` or start `ipfs daemon`"))
		if strings.TrimSpace(cfg.Upload.Endpoint) == "" {
			add(Check{Name: "upload endpoint", Status: StatusFail, Detail: "not configured", Fix: "run `igit setup push`"})
		}
		if strings.TrimSpace(cfg.Upload.Authorization) == "" && strings.TrimSpace(cfg.Upload.AuthorizationEndpoint) == "" {
			add(Check{Name: "upload authorization", Status: StatusFail, Detail: "not configured", Fix: "run `igit setup push`"})
		}
	}
	if len(failures) == 0 {
		return nil
	}
	var lines []string
	for _, failure := range failures {
		line := "- " + failure.Name + ": " + failure.Detail
		if failure.Fix != "" {
			line += " (" + failure.Fix + ")"
		}
		lines = append(lines, line)
	}
	return fmt.Errorf("push environment is incomplete:\n%s\nrun `igit doctor --push` for the full report", strings.Join(lines, "\n"))
}

func pathCheck(name, binary, fix string) Check {
	path, err := exec.LookPath(binary)
	if err != nil {
		return Check{Name: name, Status: StatusFail, Detail: "not found in PATH", Fix: fix}
	}
	abs, _ := filepath.Abs(path)
	if abs != "" {
		path = abs
	}
	return Check{Name: name, Status: StatusOK, Detail: path}
}

func commandCheck(ctx context.Context, name, binary string, args []string, fix string) Check {
	if strings.TrimSpace(binary) == "" {
		return Check{Name: name, Status: StatusFail, Detail: "not configured", Fix: fix}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	detail := firstLine(string(out))
	if err != nil {
		if detail == "" {
			detail = err.Error()
		}
		return Check{Name: name, Status: StatusFail, Detail: detail, Fix: fix}
	}
	if detail == "" {
		detail = binary
	}
	return Check{Name: name, Status: StatusOK, Detail: detail}
}

func configCheck(name, value, fix string) Check {
	if strings.TrimSpace(value) == "" {
		return Check{Name: name, Status: StatusFail, Detail: "not configured", Fix: fix}
	}
	return Check{Name: name, Status: StatusOK, Detail: value}
}

// backendCheck validates the protocol selector without probing or exposing
// transport-specific details to the rest of the CLI. During the migration,
// auto/v1 resolves to the legacy CosmWasm path; v2 is reported clearly until
// the EVM backend is installed in this build.
func backendCheck(cfg config.Config) Check {
	backend := cfg.EffectiveContractBackend()
	version := cfg.EffectiveContractVersion()
	switch backend {
	case "auto":
		switch version {
		case "v1":
			return Check{Name: "chain backend", Status: StatusOK, Detail: "auto -> cosmwasm v1"}
		case "v2":
			if err := cfg.ValidateContract(); err != nil {
				return Check{Name: "chain backend", Status: StatusFail, Detail: err.Error(), Fix: "complete the named EVM network profile"}
			}
			return Check{Name: "chain backend", Status: StatusOK, Detail: "auto -> evm v2"}
		default:
			return Check{Name: "chain backend", Status: StatusFail, Detail: "unsupported contract version " + version, Fix: "set contract_version to v1 or v2"}
		}
	case "cosmwasm", "v1":
		return Check{Name: "chain backend", Status: StatusOK, Detail: backend + " (legacy)"}
	case "evm", "v2":
		if err := cfg.ValidateContract(); err != nil {
			return Check{Name: "chain backend", Status: StatusFail, Detail: err.Error(), Fix: "complete the named EVM network profile"}
		}
		return Check{Name: "chain backend", Status: StatusOK, Detail: "evm v2"}
	default:
		return Check{Name: "chain backend", Status: StatusFail, Detail: "unsupported contract backend " + backend, Fix: "set contract_backend to auto, cosmwasm, or evm"}
	}
}

func usesLegacyBackend(cfg config.Config) bool {
	backend := cfg.EffectiveContractBackend()
	if backend == "cosmwasm" || backend == "v1" {
		return true
	}
	return backend == "auto" && cfg.EffectiveContractVersion() == "v1"
}

func endpointCheck(ctx context.Context, name, endpoint, method string, body []byte, fix string) Check {
	if strings.TrimSpace(endpoint) == "" || strings.HasPrefix(endpoint, "/") {
		return Check{Name: name, Status: StatusFail, Detail: "not configured", Fix: fix}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return Check{Name: name, Status: StatusFail, Detail: err.Error(), Fix: fix}
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: name, Status: StatusFail, Detail: err.Error(), Fix: fix}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return Check{Name: name, Status: StatusFail, Detail: fmt.Sprintf("HTTP %d from %s", resp.StatusCode, endpoint), Fix: fix}
	}
	return Check{Name: name, Status: StatusOK, Detail: endpoint}
}

func evmRPCCheck(ctx context.Context, cfg config.Config) Check {
	endpoint := cfg.EffectiveEVMRPC()
	if strings.TrimSpace(endpoint) == "" {
		return Check{Name: "EVM RPC", Status: StatusFail, Detail: "not configured", Fix: "complete the named EVM network profile"}
	}
	rpc := chain.NewEVMRPC(endpoint)
	chainID, err := rpc.ChainID(ctx)
	if err != nil {
		return Check{Name: "EVM RPC", Status: StatusFail, Detail: err.Error(), Fix: "check the named EVM network profile"}
	}
	if expected := cfg.EffectiveEVMChainID(); expected != 0 && chainID != expected {
		return Check{Name: "EVM RPC", Status: StatusFail, Detail: fmt.Sprintf("chain ID mismatch: RPC reported %d, profile requires %d", chainID, expected), Fix: "select the matching network profile"}
	}
	return Check{Name: "EVM RPC", Status: StatusOK, Detail: fmt.Sprintf("%s (chain ID %d)", endpoint, chainID)}
}

func gatewayCheck(ctx context.Context, cfg config.Config) Check {
	var failures []string
	for _, gateway := range cfg.EffectiveGateways() {
		check := endpointCheck(ctx, "read gateway", strings.TrimRight(gateway.URL, "/")+"/healthz", http.MethodGet, nil, "")
		if check.Status == StatusOK {
			return Check{Name: "read gateway", Status: StatusOK, Detail: gateway.Name + " " + gateway.URL}
		}
		failures = append(failures, gateway.Name+": "+check.Detail)
	}
	if len(failures) == 0 {
		failures = append(failures, "none configured")
	}
	return Check{Name: "read gateway", Status: StatusFail, Detail: strings.Join(failures, "; "), Fix: "check gateway configuration and network access"}
}

func keyAddressCheck(ctx context.Context, cfg config.Config, bin string) (string, Check) {
	cmd := exec.CommandContext(ctx, bin, "keys", "show", cfg.KeyName, "--keyring-backend", cfg.KeyringBackend, "--address")
	out, err := cmd.CombinedOutput()
	address := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(address, "inj1") {
		detail := firstLine(string(out))
		if detail == "" {
			detail = errString(err)
		}
		return "", Check{Name: "signing key", Status: StatusFail, Detail: detail, Fix: "run `igit key new " + cfg.KeyName + "` or select an existing key"}
	}
	return address, Check{Name: "signing key", Status: StatusOK, Detail: cfg.KeyName + " " + address}
}

func balanceCheck(ctx context.Context, cfg config.Config, address string) Check {
	endpoint := strings.TrimRight(cfg.LCDEndpoint, "/") + "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address) + "/by_denom?denom=inj"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Check{Name: "key balance", Status: StatusWarn, Detail: err.Error()}
	}
	resp, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		return Check{Name: "key balance", Status: StatusWarn, Detail: err.Error(), Fix: "check the balance manually before push"}
	}
	defer resp.Body.Close()
	var result struct {
		Balance struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balance"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil {
		return Check{Name: "key balance", Status: StatusWarn, Detail: fmt.Sprintf("could not query %s (HTTP %d)", address, resp.StatusCode), Fix: "check the balance manually before push"}
	}
	amount, ok := new(big.Int).SetString(result.Balance.Amount, 10)
	if !ok || amount.Sign() <= 0 {
		return Check{Name: "key balance", Status: StatusWarn, Detail: "0 INJ; transactions need testnet gas", Fix: "fund " + address + " from the Injective testnet faucet"}
	}
	return Check{Name: "key balance", Status: StatusOK, Detail: formatINJ(amount) + " INJ"}
}

func uploadAuthorizationCheck(ctx context.Context, cfg config.Config) Check {
	if strings.TrimSpace(cfg.Upload.Authorization) != "" {
		return Check{Name: "upload authorization", Status: StatusOK, Detail: "explicit token configured"}
	}
	endpoint := strings.TrimSpace(cfg.Upload.AuthorizationEndpoint)
	if endpoint == "" {
		return Check{Name: "upload authorization", Status: StatusFail, Detail: "not configured", Fix: "run `igit setup push`"}
	}
	check := endpointCheck(ctx, "upload authorization", endpoint, http.MethodPost, nil, "check upload.authorization_endpoint and network access")
	return check
}

func injectivedBin(cfg config.Config) string {
	if strings.TrimSpace(cfg.InjectivedBin) == "" {
		return "injectived"
	}
	return cfg.InjectivedBin
}

func ipfsBin(cfg config.Config) string {
	if strings.TrimSpace(cfg.IPFSBin) == "" {
		return "ipfs"
	}
	return cfg.IPFSBin
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if before, _, ok := strings.Cut(value, "\n"); ok {
		value = before
	}
	return strings.TrimSpace(value)
}

func errString(err error) string {
	if err == nil {
		return "command returned no address"
	}
	return err.Error()
}

func formatINJ(amount *big.Int) string {
	const decimals = 18
	raw := amount.String()
	if len(raw) <= decimals {
		raw = strings.Repeat("0", decimals-len(raw)+1) + raw
	}
	whole := raw[:len(raw)-decimals]
	fraction := strings.TrimRight(raw[len(raw)-decimals:], "0")
	if len(fraction) > 6 {
		fraction = fraction[:6]
	}
	if whole == "0" && strings.Trim(fraction, "0") == "" && amount.Sign() > 0 {
		return "<0.000001"
	}
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

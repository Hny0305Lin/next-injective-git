// Package config loads and stores the Next Injective Git CLI configuration
// from ~/.igit/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds every knob the CLI needs to reach Injective and IPFS.
type Config struct {
	// ContractAddress is the repo-registry contract on Injective.
	ContractAddress string `json:"contract_address"`
	// ChainID e.g. "injective-888" (testnet) or "injective-1" (mainnet).
	ChainID string `json:"chain_id"`
	// LCDEndpoint is the REST endpoint used for smart queries,
	// e.g. "https://testnet.sentry.lcd.injective.network:443".
	LCDEndpoint string `json:"lcd_endpoint"`
	// Node is the Tendermint RPC endpoint used by injectived for txs,
	// e.g. "https://testnet.sentry.tm.injective.network:443".
	Node string `json:"node"`
	// KeyName is the injectived keyring key used to sign txs.
	KeyName string `json:"key_name"`
	// KeyringBackend passed to injectived (test|file|os).
	KeyringBackend string `json:"keyring_backend"`
	// InjectivedBin is the injectived binary path (default "injectived").
	InjectivedBin string `json:"injectived_bin"`
	// GasPrices e.g. "500000000inj".
	GasPrices string `json:"gas_prices"`
	// IPFSAPI is an optional loopback-only Kubo RPC API used exclusively while
	// pushing a temporary pack. It is never a clone/fetch dependency.
	// The json name is kept for backwards compatible config files.
	IPFSAPI string `json:"ipfs_api"`
	// IPFSGateway is used as a fallback for downloads, e.g. "https://ipfs.io".
	// Deprecated: use Gateways for an ordered, health-checked set. This field is
	// retained so existing config files keep working and can still add one custom
	// gateway ahead of the project defaults.
	IPFSGateway string `json:"ipfs_gateway"`
	// Gateways are read-only IPFS path gateways. The helper probes /healthz and
	// uses the healthy endpoints in latency order before downloading a pack.
	Gateways []Gateway `json:"gateways"`
	// Upload identifies the controlled US replication service. Its token is a
	// short-lived, CID/repository/ref/pack-hash-bound authorization; it is not a
	// Kubo credential and cannot submit chain transactions.
	Upload Upload `json:"upload"`
	// PublicGatewayFallbacks are tried only after the HK/US read gateways. They
	// are data-plane fallbacks and deliberately are not health-ranked with the
	// project gateways.
	PublicGatewayFallbacks []string `json:"public_gateway_fallbacks"`
}

// Gateway is a read-only public IPFS gateway endpoint.
type Gateway struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Upload configures the push-only temporary local-Kubo path.
type Upload struct {
	// Endpoint is the HTTPS controlled replication/Pin endpoint in the US.
	Endpoint string `json:"endpoint"`
	// Authorization is a short-lived scoped bearer token issued by the upload
	// authorization service. It is intentionally separate from chain keys.
	Authorization string `json:"authorization,omitempty"`
	// USPeer is the US Kubo swarm multiaddr. It is used only before a push so
	// the US node can fetch the temporary local blocks directly.
	USPeer string `json:"us_peer"`
}

// Tunnel is retained only so older internal profiles can still be parsed by
// the unused compatibility package. It is no longer part of Config and no
// normal-user CLI command exposes remote Kubo SSH forwarding.
type Tunnel struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	User         string `json:"user"`
	IdentityFile string `json:"identity_file,omitempty"`
	LocalAddr    string `json:"local_addr"`
	RemoteAddr   string `json:"remote_addr"`
}

// Defaults returns a config pre-filled for Injective testnet, read gateways,
// and an optional loopback local Kubo used only for push uploads.
func Defaults() Config {
	return Config{
		ChainID:        "injective-888",
		LCDEndpoint:    "https://testnet.sentry.lcd.injective.network:443",
		Node:           "https://testnet.sentry.tm.injective.network:443",
		KeyringBackend: "test",
		InjectivedBin:  "injectived",
		GasPrices:      "500000000inj",
		IPFSAPI:        "http://127.0.0.1:5001",
		IPFSGateway:    "https://igit-hk.haohanyh.ovh",
		Gateways: []Gateway{
			{Name: "hk", URL: "https://igit-hk.haohanyh.ovh"},
			{Name: "us", URL: "https://igit-us.haohanyh.ovh"},
		},
		Upload: Upload{
			Endpoint: "https://igit-us.haohanyh.ovh/v1/replications",
		},
		PublicGatewayFallbacks: []string{"https://ipfs.io"},
	}
}

// EffectiveReadGateways returns health-ranked project gateways followed by
// public read fallbacks. The helper probes only the project gateways.
func (c Config) EffectiveReadFallbacks() []string {
	seen := make(map[string]bool)
	var urls []string
	for _, raw := range c.PublicGatewayFallbacks {
		url := strings.TrimRight(strings.TrimSpace(raw), "/")
		if url != "" && !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

// EffectiveGateways returns the configured endpoints without duplicates. A
// legacy ipfs_gateway value is kept first only when it is not already named by
// a gateway profile.
func (c Config) EffectiveGateways() []Gateway {
	seen := make(map[string]bool)
	var gateways []Gateway
	add := func(g Gateway) {
		g.URL = strings.TrimRight(strings.TrimSpace(g.URL), "/")
		if g.URL == "" || seen[g.URL] {
			return
		}
		seen[g.URL] = true
		gateways = append(gateways, g)
	}
	legacyURL := strings.TrimRight(strings.TrimSpace(c.IPFSGateway), "/")
	legacyNamed := false
	for _, g := range c.Gateways {
		if strings.TrimRight(strings.TrimSpace(g.URL), "/") == legacyURL {
			legacyNamed = true
			break
		}
	}
	if !legacyNamed {
		add(Gateway{Name: "custom", URL: legacyURL})
	}
	for _, g := range c.Gateways {
		add(g)
	}
	return gateways
}

// Dir returns the config directory (~/.igit), honoring IGIT_HOME override.
func Dir() (string, error) {
	if custom := os.Getenv("IGIT_HOME"); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".igit"), nil
}

// Path returns the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file, applying defaults for missing fields.
func Load() (Config, error) {
	cfg := Defaults()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config file, creating the directory if needed.
func Save(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Validate checks the fields required for chain operations.
func (c Config) Validate() error {
	var missing []string
	if c.ContractAddress == "" {
		missing = append(missing, "contract_address")
	}
	if c.KeyName == "" {
		missing = append(missing, "key_name")
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"missing config: %s (run `igit config set <key> <value>`)",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

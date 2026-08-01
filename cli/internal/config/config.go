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
	// IPFSAPI is the Kubo RPC API, default "http://127.0.0.1:5001".
	IPFSAPI string `json:"ipfs_api"`
	// IPFSGateway is used as a fallback for downloads, e.g. "https://ipfs.io".
	// Deprecated: use Gateways for an ordered, health-checked set. This field is
	// retained so existing config files keep working and can still add one custom
	// gateway ahead of the project defaults.
	IPFSGateway string `json:"ipfs_gateway"`
	// Gateways are read-only IPFS path gateways. The helper probes /healthz and
	// uses the healthy endpoints in latency order before downloading a pack.
	Gateways []Gateway `json:"gateways"`
	// Tunnels describes SSH-only control-plane access to remote Kubo APIs. The
	// remote APIs stay bound to loopback; igit forwards them to loopback locally.
	Tunnels []Tunnel `json:"tunnels"`
	// Peers are libp2p multiaddrs the helper directly connects to on startup
	// (e.g. the project pin node), letting Bitswap fetch/serve packs without
	// waiting on slow DHT discovery. Optional; best effort.
	Peers []string `json:"peers"`
}

// Gateway is a read-only public IPFS gateway endpoint.
type Gateway struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Tunnel is an SSH local-forwarding profile for a remote, loopback-only Kubo
// API. IdentityFile is intentionally just a local path, never key material.
type Tunnel struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	User         string `json:"user"`
	IdentityFile string `json:"identity_file,omitempty"`
	LocalAddr    string `json:"local_addr"`
	RemoteAddr   string `json:"remote_addr"`
}

// Defaults returns a config pre-filled for Injective testnet + local Kubo.
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
		Tunnels: []Tunnel{
			{Name: "hk", Host: "45.202.249.80", User: "root", LocalAddr: "127.0.0.1:15001", RemoteAddr: "127.0.0.1:5001"},
			{Name: "us", Host: "162.35.187.224", User: "root", LocalAddr: "127.0.0.1:15002", RemoteAddr: "127.0.0.1:5001"},
		},
		Peers: []string{
			"/dns4/igit-hk.haohanyh.ovh/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W",
			"/ip4/45.202.249.80/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W",
		},
	}
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

// TunnelByName returns a configured tunnel profile.
func (c Config) TunnelByName(name string) (Tunnel, bool) {
	for _, tunnel := range c.Tunnels {
		if tunnel.Name == name {
			return tunnel, true
		}
	}
	return Tunnel{}, false
}

// SetTunnel replaces a named tunnel profile while retaining all others.
func (c *Config) SetTunnel(updated Tunnel) {
	for i := range c.Tunnels {
		if c.Tunnels[i].Name == updated.Name {
			c.Tunnels[i] = updated
			return
		}
	}
	c.Tunnels = append(c.Tunnels, updated)
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

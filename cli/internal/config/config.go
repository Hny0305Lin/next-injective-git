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

	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
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
	// AuthorizationEndpoint issues short-lived identity tokens accepted by all
	// project replication endpoints. The helper refreshes from it automatically.
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// Authorization is a short-lived scoped bearer token issued by the upload
	// authorization service. It is retained as an explicit override and for
	// offline deployments; normal clients use AuthorizationEndpoint.
	Authorization string `json:"authorization,omitempty"`
	// USPeer is the US Kubo swarm multiaddr. It is used only before a push so
	// the US node can fetch the temporary local blocks directly.
	USPeer string `json:"us_peer"`
	// HKPeer is the hot-tier Kubo peer. Connecting it during a push makes the
	// temporary pack immediately reachable by both project regions.
	HKPeer string `json:"hk_peer"`
}

const (
	DefaultUSUploadPeer = "/ip4/162.35.187.224/tcp/4001/p2p/12D3KooWBGyxqNM3q6nHvacFfqnwoXP2uXxP36uSPab2p16ywfFS"
	DefaultHKUploadPeer = "/dns4/igit-hk.haohanyh.ovh/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W"
)

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
			Endpoint:              "https://igit-us.haohanyh.ovh/v1/replications",
			AuthorizationEndpoint: "https://www.igit.xyz/api/upload-authorization",
			USPeer:                DefaultUSUploadPeer,
			HKPeer:                DefaultHKUploadPeer,
		},
		PublicGatewayFallbacks: []string{"https://ipfs.io"},
	}
}

// EffectiveUploadPeers returns the durable US peer first and the HK hot-tier
// peer second. Empty legacy config values fall back to the built-in project
// peers so a fresh CLI can push without copying deployment multiaddrs by hand.
func (c Config) EffectiveUploadPeers() []string {
	us := strings.TrimSpace(c.Upload.USPeer)
	if us == "" {
		us = DefaultUSUploadPeer
	}
	hk := strings.TrimSpace(c.Upload.HKPeer)
	if hk == "" {
		hk = DefaultHKUploadPeer
	}
	peers := []string{us}
	if hk != us {
		peers = append(peers, hk)
	}
	return peers
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

// EffectiveGateways returns project gateway endpoints without duplicates. A
// legacy ipfs_gateway value is kept first for backwards compatibility when it
// is a genuine custom endpoint; values already configured as public fallbacks
// stay behind the project gateways.
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
	legacyPublic := false
	for _, fallback := range c.PublicGatewayFallbacks {
		if strings.TrimRight(strings.TrimSpace(fallback), "/") == legacyURL {
			legacyPublic = true
			break
		}
	}
	for _, g := range c.Gateways {
		add(g)
	}
	if !legacyNamed && !legacyPublic && legacyURL != "" {
		custom := Gateway{Name: "custom", URL: legacyURL}
		custom.URL = strings.TrimRight(strings.TrimSpace(custom.URL), "/")
		gateways = append([]Gateway{custom}, gateways...)
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
	if err := c.ValidateContract(); err != nil {
		return err
	}
	if c.KeyName == "" {
		return i18n.Errorf(
			"missing config: key_name (run `igit config set <key> <value>`)",
			"缺少配置：key_name（运行 `igit config set <key> <value>`）",
		)
	}
	return nil
}

// ValidateContract checks the fields needed by read-only contract queries.
// Commands such as release verification must not require a signing key.
func (c Config) ValidateContract() error {
	var missing []string
	if c.ContractAddress == "" {
		missing = append(missing, "contract_address")
	}
	if len(missing) > 0 {
		return i18n.Errorf(
			"missing config: %s (run `igit config set <key> <value>`)",
			"缺少配置：%s（运行 `igit config set <key> <value>`）",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

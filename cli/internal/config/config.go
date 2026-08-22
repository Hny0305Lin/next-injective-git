// Package config loads and stores the Next Injective Git CLI configuration
// from ~/.igit/config.json.
package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/fileprotection"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

// Config holds every knob the CLI needs to reach Injective and IPFS.
type Config struct {
	// Network is the named Injective profile used to derive default endpoints.
	// It is metadata for backend selection; explicit endpoint fields remain
	// supported for backwards compatibility and offline deployments.
	Network string `json:"network,omitempty"`
	// EVMSuiteDirectoryAddress is the sole contract trust root used by ordinary
	// CLI and Git runtime paths. Core and module addresses are discovered and
	// verified from this immutable directory before use.
	EVMSuiteDirectoryAddress string `json:"evm_suite_directory_address,omitempty"`
	// Deprecated compatibility fields are decoded only by Load, then cleared
	// during the one-time upgrade to the immutable EVM suite profile.
	ContractBackend string `json:"-"`
	ContractVersion string `json:"-"`
	ContractAddress string `json:"-"`
	// EVMRPC is the JSON-RPC endpoint for Injective EVM. It is derived from
	// Network when omitted; users should not need to edit it manually.
	EVMRPC                     string `json:"evm_rpc,omitempty"`
	EVMContractAddress         string `json:"-"`
	EVMBadgeModuleAddress      string `json:"-"`
	EVMEconomicModuleAddress   string `json:"-"`
	EVMModerationModuleAddress string `json:"-"`
	// EVMChainID is checked against eth_chainId before signing a transaction.
	EVMChainID uint64 `json:"evm_chain_id,omitempty"`
	// EVMExplorer is the profile-provided transaction explorer base URL.
	EVMExplorer string `json:"evm_explorer,omitempty"`
	// EVMKeystoreDir contains encrypted keystore JSON files. It never contains
	// plaintext private keys and is not printed by normal CLI output.
	EVMKeystoreDir string `json:"evm_keystore_dir,omitempty"`
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
	// IPFSBin is the Kubo CLI used by setup and diagnostics. Push data still
	// travels through IPFSAPI, so the remote helper never shells out to it.
	IPFSBin string `json:"ipfs_bin"`
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
	DefaultContractAddress = "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	DefaultUSUploadPeer    = "/ip4/162.35.187.224/tcp/4001/p2p/12D3KooWBGyxqNM3q6nHvacFfqnwoXP2uXxP36uSPab2p16ywfFS"
	DefaultHKUploadPeer    = "/dns4/igit-hk.haohanyh.ovh/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W"

	ErrorCodeMissingEVMSuiteDirectory i18n.ErrorCode = "config.evm_suite_directory_missing"
	ErrorCodeInvalidEVMSuiteDirectory i18n.ErrorCode = "config.evm_suite_directory_invalid"
	ErrorCodeMissingEVMRPC            i18n.ErrorCode = "config.evm_rpc_missing"
	ErrorCodeMissingEVMChainID        i18n.ErrorCode = "config.evm_chain_id_missing"
)

// NetworkProfile is the user-facing network descriptor. Endpoint and chain
// details live here rather than in command code so CLI, Web, and migration
// tooling can share one source of defaults.
type NetworkProfile struct {
	Name              string
	CosmosChainID     string
	EVMChainID        uint64
	LCDEndpoint       string
	CosmosRPC         string
	EVMRPC            string
	EVMExplorer       string
	EVMSuiteDirectory string
}

var networkProfiles = map[string]NetworkProfile{
	"injective-testnet": {
		Name:          "injective-testnet",
		CosmosChainID: "injective-888",
		EVMChainID:    1439,
		LCDEndpoint:   "https://k8s.testnet.lcd.injective.network",
		CosmosRPC:     "https://k8s.testnet.tm.injective.network",
		EVMRPC:        "https://k8s.testnet.json-rpc.injective.network",
		EVMExplorer:   "https://testnet.blockscout.injective.network",
	},
	"injective-mainnet": {
		Name:          "injective-mainnet",
		CosmosChainID: "injective-1",
		EVMChainID:    1776,
		LCDEndpoint:   "https://lcd.injective.network",
		CosmosRPC:     "https://tm.injective.network",
		EVMRPC:        "https://sentry.evm-rpc.injective.network/",
		EVMExplorer:   "https://blockscout.injective.network",
	},
}

// NetworkProfileFor returns a named profile without exposing the mutable map.
func NetworkProfileFor(name string) (NetworkProfile, bool) {
	profile, ok := networkProfiles[strings.ToLower(strings.TrimSpace(name))]
	return profile, ok
}

// ValidatePublishedSuiteProfiles enforces the immutable EVM v3 profile shape
// and rejects any embedded Directory address until deployment and cutover
// evidence are verified by the release workflow.
func ValidatePublishedSuiteProfiles() error {
	cfg := Defaults()
	if backend, version := cfg.EffectiveContractBackend(), cfg.EffectiveContractVersion(); backend != "evm" || version != "v3" {
		return fmt.Errorf("published defaults select %s/%s, want evm/v3", backend, version)
	}
	if len(networkProfiles) == 0 {
		return errors.New("no published network profiles are configured")
	}
	for name, profile := range networkProfiles {
		if strings.TrimSpace(name) == "" || profile.Name != name {
			return fmt.Errorf("published profile map key %q does not match profile name %q", name, profile.Name)
		}
		if value := strings.TrimSpace(profile.EVMSuiteDirectory); value != "" {
			return fmt.Errorf("published profile %q has SuiteDirectory %q without deployment/cutover evidence verification", name, value)
		}
	}
	return nil
}

// SelectNetworkProfile replaces every profile-owned field as one atomic
// selection. This is used by the user-facing `igit config set network` path;
// retaining RPC, chain ID, explorer, or contract values from the previous
// profile would create a dangerous mixed-network configuration. Operators who
// need an override can set that specific field after selecting the network.
func SelectNetworkProfile(cfg Config, name string) (Config, error) {
	profile, ok := NetworkProfileFor(name)
	if !ok {
		return cfg, fmt.Errorf("unknown network profile %q", strings.TrimSpace(name))
	}
	cfg.Network = profile.Name
	cfg.ChainID = profile.CosmosChainID
	cfg.LCDEndpoint = profile.LCDEndpoint
	cfg.Node = profile.CosmosRPC
	cfg.EVMRPC = profile.EVMRPC
	cfg.EVMChainID = profile.EVMChainID
	cfg.EVMExplorer = profile.EVMExplorer
	cfg.EVMSuiteDirectoryAddress = profile.EVMSuiteDirectory
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v3"
	cfg.ContractAddress = ""
	cfg.EVMContractAddress = ""
	cfg.EVMBadgeModuleAddress = ""
	cfg.EVMEconomicModuleAddress = ""
	cfg.EVMModerationModuleAddress = ""
	return cfg, nil
}

// ApplyNetworkProfile fills only omitted/legacy endpoint fields. Explicit
// operator overrides remain intact, while a fresh config gets deterministic
// network defaults for both legacy and EVM transports.
func ApplyNetworkProfile(cfg Config) Config {
	name := strings.ToLower(strings.TrimSpace(cfg.Network))
	if name == "" {
		name = "injective-testnet"
	}
	profile, ok := NetworkProfileFor(name)
	if !ok {
		return cfg
	}
	cfg.Network = profile.Name
	if cfg.ChainID == "" {
		cfg.ChainID = profile.CosmosChainID
	}
	if cfg.LCDEndpoint == "" {
		cfg.LCDEndpoint = profile.LCDEndpoint
	}
	if cfg.Node == "" {
		cfg.Node = profile.CosmosRPC
	}
	if cfg.EVMRPC == "" {
		cfg.EVMRPC = profile.EVMRPC
	}
	if cfg.EVMChainID == 0 {
		cfg.EVMChainID = profile.EVMChainID
	}
	if cfg.EVMExplorer == "" {
		cfg.EVMExplorer = profile.EVMExplorer
	}
	if cfg.EVMSuiteDirectoryAddress == "" {
		cfg.EVMSuiteDirectoryAddress = profile.EVMSuiteDirectory
	}
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v3"
	return cfg
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
	return ApplyNetworkProfile(Config{
		Network:         "injective-testnet",
		ContractBackend: "evm",
		ContractVersion: "v3",
		ChainID:         "injective-888",
		LCDEndpoint:     "https://testnet.sentry.lcd.injective.network:443",
		Node:            "https://testnet.sentry.tm.injective.network:443",
		KeyringBackend:  "test",
		InjectivedBin:   "injectived",
		GasPrices:       "500000000inj",
		IPFSAPI:         "http://127.0.0.1:5001",
		IPFSBin:         "ipfs",
		IPFSGateway:     "https://igit-hk.haohanyh.ovh",
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
	})
}

// EffectiveContractBackend is fixed after the one-time V1 config upgrade.
func (c Config) EffectiveContractBackend() string {
	return "evm"
}

// EffectiveContractVersion is fixed to the immutable suite protocol.
func (c Config) EffectiveContractVersion() string {
	return "v3"
}

// EffectiveEVMRPC returns the profile-derived Injective EVM JSON-RPC endpoint.
func (c Config) EffectiveEVMRPC() string {
	if value := strings.TrimSpace(c.EVMRPC); value != "" {
		return strings.TrimRight(value, "/")
	}
	if profile, ok := NetworkProfileFor(c.Network); ok {
		return strings.TrimRight(profile.EVMRPC, "/")
	}
	return ""
}

// EffectiveEVMContractAddress returns the explicitly deployed V2 address.
// The legacy inj1 CosmWasm address is never guessed as an EVM address.
func (c Config) EffectiveEVMContractAddress() string {
	return strings.TrimSpace(c.EVMContractAddress)
}

// EffectiveEVMSuiteDirectoryAddress returns the only contract address trusted
// by ordinary runtime paths. It intentionally never guesses an address from a
// legacy Core/module field.
func (c Config) EffectiveEVMSuiteDirectoryAddress() string {
	if value := strings.TrimSpace(c.EVMSuiteDirectoryAddress); value != "" {
		return value
	}
	if profile, ok := NetworkProfileFor(c.Network); ok {
		return strings.TrimSpace(profile.EVMSuiteDirectory)
	}
	return ""
}

// EffectiveEVMBadgeModuleAddress returns the reviewed badge module address.
// It remains empty until the module and its registry binding are deployed.
func (c Config) EffectiveEVMBadgeModuleAddress() string {
	return strings.TrimSpace(c.EVMBadgeModuleAddress)
}

// EffectiveEVMEconomicModuleAddress returns the reviewed economic module
// address. It remains empty until the module and its registry binding are
// deployed and approved.
func (c Config) EffectiveEVMEconomicModuleAddress() string {
	return strings.TrimSpace(c.EVMEconomicModuleAddress)
}

// EffectiveEVMModerationModuleAddress returns the reviewed moderation module
// address. It remains empty until the module and its registry binding are
// deployed and approved.
func (c Config) EffectiveEVMModerationModuleAddress() string {
	return strings.TrimSpace(c.EVMModerationModuleAddress)
}

// EffectiveEVMChainID returns the configured/profile-derived inEVM chain ID.
func (c Config) EffectiveEVMChainID() uint64 {
	if c.EVMChainID != 0 {
		return c.EVMChainID
	}
	if profile, ok := NetworkProfileFor(c.Network); ok {
		return profile.EVMChainID
	}
	return 0
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
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	} else if err != nil {
		return cfg, err
	}
	if err := fileprotection.ProtectDirectory(filepath.Dir(path)); err != nil {
		return cfg, fmt.Errorf("protect config directory before read: %w", err)
	}
	if err := fileprotection.ProtectFile(path); err != nil {
		return cfg, fmt.Errorf("protect config file before read: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	previousNetwork := cfg.Network
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg = ApplyNetworkProfile(cfg)
	// Defaults() starts from testnet for backwards compatibility. When an
	// existing config explicitly switches profile, replace only profile-owned
	// fields that were not explicitly set in the file.
	if cfg.Network != previousNetwork {
		profileBase := ApplyNetworkProfile(Config{Network: cfg.Network})
		if _, ok := raw["chain_id"]; !ok {
			cfg.ChainID = profileBase.ChainID
		}
		if _, ok := raw["lcd_endpoint"]; !ok {
			cfg.LCDEndpoint = profileBase.LCDEndpoint
		}
		if _, ok := raw["node"]; !ok {
			cfg.Node = profileBase.Node
		}
		if _, ok := raw["evm_rpc"]; !ok {
			cfg.EVMRPC = profileBase.EVMRPC
		}
		if _, ok := raw["evm_chain_id"]; !ok {
			cfg.EVMChainID = profileBase.EVMChainID
		}
		if _, ok := raw["evm_explorer"]; !ok {
			cfg.EVMExplorer = profileBase.EVMExplorer
		}
		if _, ok := raw["evm_suite_directory_address"]; !ok {
			cfg.EVMSuiteDirectoryAddress = profileBase.EVMSuiteDirectoryAddress
		}
	}
	legacyKeys := []string{
		"contract_backend", "contract_version", "contract_address",
		"evm_contract_address", "evm_badge_module_address",
		"evm_economic_module_address", "evm_moderation_module_address",
	}
	legacyConfig := false
	for _, key := range legacyKeys {
		if _, ok := raw[key]; ok {
			legacyConfig = true
			break
		}
	}
	cfg.ContractBackend = "evm"
	cfg.ContractVersion = "v3"
	cfg.ContractAddress = ""
	cfg.EVMContractAddress = ""
	cfg.EVMBadgeModuleAddress = ""
	cfg.EVMEconomicModuleAddress = ""
	cfg.EVMModerationModuleAddress = ""
	if legacyConfig {
		if err := Save(cfg); err != nil {
			return cfg, fmt.Errorf("upgrade legacy config to EVM suite profile: %w", err)
		}
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
	if err := fileprotection.ProtectDirectory(dir); err != nil {
		return fmt.Errorf("protect config directory: %w", err)
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := fileprotection.WriteFile(path, data); err != nil {
		return fmt.Errorf("write protected config file: %w", err)
	}
	return nil
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
	address := strings.TrimSpace(c.EffectiveEVMSuiteDirectoryAddress())
	if address == "" {
		return i18n.ErrorfCode(
			ErrorCodeMissingEVMSuiteDirectory,
			"missing EVM SuiteDirectory address (deployment and cutover evidence must be approved before the profile is updated)",
			"缺少 EVM SuiteDirectory 地址（部署和切换证据获批后才能更新网络配置）",
		)
	}
	if !validEVMAddress(address) {
		return i18n.ErrorfCode(
			ErrorCodeInvalidEVMSuiteDirectory,
			"invalid EVM SuiteDirectory address %q (expected 0x followed by 40 hex characters)",
			"EVM SuiteDirectory 地址 %q 无效（应为 0x 后跟 40 个十六进制字符）",
			address,
		)
	}
	if c.EffectiveEVMRPC() == "" {
		return i18n.ErrorfCode(ErrorCodeMissingEVMRPC, "missing EVM RPC endpoint", "缺少 EVM RPC 端点")
	}
	if c.EffectiveEVMChainID() == 0 {
		return i18n.ErrorfCode(ErrorCodeMissingEVMChainID, "missing EVM chain ID", "缺少 EVM chain ID")
	}
	return nil
}

// ValidateBadgeModule checks the core V2 profile and the separately deployed
// badge module. V1 needs no additional address because badges live in V1.
func (c Config) ValidateBadgeModule() error {
	return c.ValidateContract()
}

// ValidateEconomicModule checks the core V2 profile and the separately
// deployed sponsorship/revenue module. V1 keeps using its legacy messages.
func (c Config) ValidateEconomicModule() error {
	return c.ValidateContract()
}

// ValidateModerationModule checks the core V2 profile and the independently
// deployed moderation module. V1 keeps using its legacy messages and query.
func (c Config) ValidateModerationModule() error {
	return c.ValidateContract()
}

func validEVMAddress(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 42 || !(strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X")) {
		return false
	}
	_, err := hex.DecodeString(value[2:])
	return err == nil
}

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
	IPFSGateway string `json:"ipfs_gateway"`
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
		IPFSGateway:    "https://ipfs.io",
	}
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

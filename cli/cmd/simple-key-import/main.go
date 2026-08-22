package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"crypto/ecdsa"
	"encoding/json"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func main() {
	hexKey := flag.String("key", "", "hex private key (without 0x prefix)")
	password := flag.String("password", strings.TrimSpace(os.Getenv("IGIT_EVM_KEY_PASSWORD")), "keystore password (or set IGIT_EVM_KEY_PASSWORD)")
	name := flag.String("name", "igit-dev", "key name")
	flag.Parse()

	if strings.TrimSpace(*hexKey) == "" {
		fmt.Fprintln(os.Stderr, "Error: --key is required")
		os.Exit(1)
	}
	if strings.TrimSpace(*password) == "" {
		fmt.Fprintln(os.Stderr, "Error: --password is required (or set IGIT_EVM_KEY_PASSWORD)")
		os.Exit(1)
	}

	// Get config directory
	cfgDir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting config dir: %v\n", err)
		os.Exit(1)
	}

	keyDir := filepath.Join(cfgDir, "keystore")

	// Import the key
	if err := importHexKey(*hexKey, *password, keyDir, *name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func importHexKey(hexKey, password, keyDir, name string) error {
	// Clean the hex key
	hexKey = strings.TrimSpace(hexKey)
	hexKey = strings.TrimPrefix(hexKey, "0x")
	hexKey = strings.TrimPrefix(hexKey, "0X")

	// Parse private key
	key, err := ethcrypto.HexToECDSA(hexKey)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	defer clearECDSA(key)

	// Create keystore directory
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return fmt.Errorf("create keystore directory: %w", err)
	}

	// Import to encrypted keystore
	ks := keystore.NewKeyStore(keyDir, keystore.StandardScryptN, keystore.StandardScryptP)
	account, err := ks.ImportECDSA(key, password)
	if err != nil {
		return fmt.Errorf("import encrypted key: %w", err)
	}

	// Set file permissions
	_ = os.Chmod(account.URL.Path, 0o600)

	// Write index
	index, err := json.MarshalIndent(map[string]string{name: account.URL.Path}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode keystore index: %w", err)
	}

	indexPath := filepath.Join(keyDir, "index.json")
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		return fmt.Errorf("write keystore index: %w", err)
	}

	fmt.Printf("✅ Successfully imported key: %s\n", account.Address.Hex())
	fmt.Printf("   Keystore dir: %s\n", keyDir)
	fmt.Printf("   Index file: %s\n", indexPath)

	return nil
}

func clearECDSA(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		key.D.SetInt64(0)
	}
}

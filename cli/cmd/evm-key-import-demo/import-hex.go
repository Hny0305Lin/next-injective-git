// Simple hex private key importer
package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func ImportHexKey(hexKey, password, keyDir, name string) error {
	// Clean the hex key
	hexKey = strings.TrimSpace(hexKey)
	hexKey = strings.TrimPrefix(hexKey, "0x")
	hexKey = strings.TrimPrefix(hexKey, "0X")

	// Parse private key
	key, err := ethcrypto.HexToECDSA(hexKey)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	defer clearImportedECDSA(key)

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

	fmt.Printf("Successfully imported key: %s\n", account.Address.Hex())
	fmt.Printf("Keystore file: %s\n", account.URL.Path)
	fmt.Printf("Index file: %s\n", indexPath)

	return nil
}

func clearImportedECDSA(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		key.D.SetInt64(0)
	}
}

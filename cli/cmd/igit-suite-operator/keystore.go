package main

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"syscall"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/term"
)

// LoadPrivateKey loads and decrypts a private key from keystore
func LoadPrivateKey(keystorePath string, passphraseFile string) (*ecdsa.PrivateKey, error) {
	// Read keystore file
	data, err := os.ReadFile(keystorePath)
	if err != nil {
		return nil, fmt.Errorf("read keystore: %w", err)
	}

	var passphrase []byte

	// Read passphrase from file if provided (TESTING ONLY)
	if passphraseFile != "" {
		fmt.Fprintf(os.Stderr, "⚠️  Reading passphrase from file (INSECURE - testing only)\n")
		passphrase, err = os.ReadFile(passphraseFile)
		if err != nil {
			return nil, fmt.Errorf("read passphrase file: %w", err)
		}
		// Trim trailing newline/whitespace
		passphraseStr := string(passphrase)
		// Remove all trailing whitespace including \r\n
		for len(passphraseStr) > 0 {
			lastChar := passphraseStr[len(passphraseStr)-1]
			if lastChar == '\n' || lastChar == '\r' || lastChar == ' ' || lastChar == '\t' {
				passphraseStr = passphraseStr[:len(passphraseStr)-1]
			} else {
				break
			}
		}
		passphrase = []byte(passphraseStr)
	} else {
		// Prompt for passphrase (no echo)
		fmt.Fprint(os.Stderr, "🔐 Enter keystore passphrase: ")
		passphrase, err = term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("read passphrase: %w", err)
		}
	}

	// Decrypt keystore
	key, err := keystore.DecryptKey(data, string(passphrase))
	if err != nil {
		return nil, fmt.Errorf("decrypt keystore (wrong passphrase?): %w", err)
	}

	return key.PrivateKey, nil
}

// GetAddress returns the Ethereum address from a private key
func GetAddress(privateKey *ecdsa.PrivateKey) string {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return address.Hex()
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
)

const testKeystorePassphrase = "test-passphrase"

func TestRun_RequiresManifestAndKeystore(t *testing.T) {
	// Test that missing required arguments return error code
	code := run([]string{})
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}

	code = run([]string{"--manifest", "test.json"})
	if code != 2 {
		t.Errorf("expected exit code 2 without keystore, got %d", code)
	}
}

func TestRun_AcceptsDryRun(t *testing.T) {
	keystorePath, passphrasePath := createTestKeystore(t)

	code := run([]string{
		"--manifest", "test-manifest.json",
		"--keystore", keystorePath,
		"--passphrase-file", passphrasePath,
		"--journal", filepath.Join(t.TempDir(), "operator-journal.jsonl"),
		"--dry-run",
	})
	if code != 0 {
		t.Errorf("expected exit code 0 for successful dry-run, got %d", code)
	}
}

func TestRun_DefaultValues(t *testing.T) {
	keystorePath, passphrasePath := createTestKeystore(t)

	// Leave RPC, chain ID, and gas price unset so run uses their defaults.
	code := run([]string{
		"--manifest", "test-manifest.json",
		"--keystore", keystorePath,
		"--passphrase-file", passphrasePath,
		"--journal", filepath.Join(t.TempDir(), "operator-journal.jsonl"),
		"--dry-run",
	})
	if code != 0 {
		t.Errorf("expected exit code 0 with default runtime values, got %d", code)
	}
}

func createTestKeystore(t *testing.T) (keystorePath, passphrasePath string) {
	t.Helper()

	dir := t.TempDir()
	keyStore := keystore.NewKeyStore(
		filepath.Join(dir, "keystore"),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	account, err := keyStore.NewAccount(testKeystorePassphrase)
	if err != nil {
		t.Fatalf("create test keystore: %v", err)
	}

	passphrasePath = filepath.Join(dir, "passphrase.txt")
	if err := os.WriteFile(passphrasePath, []byte(testKeystorePassphrase+"\n"), 0o600); err != nil {
		t.Fatalf("write test passphrase: %v", err)
	}

	return account.URL.Path, passphrasePath
}

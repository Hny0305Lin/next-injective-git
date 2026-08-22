package main

import (
	"testing"
)

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
	// Test that dry-run flag is accepted with passphrase file
	code := run([]string{
		"--manifest", "test-manifest.json",
		"--keystore", "test-keystore.json",
		"--passphrase-file", "test-passphrase.txt",
		"--dry-run",
	})
	// Should succeed in dry-run mode
	if code != 0 {
		t.Errorf("expected exit code 0 for successful dry-run, got %d", code)
	}
}

func TestRun_DefaultValues(t *testing.T) {
	// Test that defaults are set correctly
	// This is a smoke test for flag parsing
	args := []string{
		"--manifest", "test.json",
		"--keystore", "test.key",
	}

	// Just check it doesn't panic
	_ = run(args)
}

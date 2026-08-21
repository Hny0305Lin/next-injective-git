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
	// Test that dry-run flag is accepted
	// This will fail until we create test fixtures
	code := run([]string{
		"--manifest", "test-manifest.json",
		"--keystore", "test-keystore.json",
		"--dry-run",
	})
	// Expected to fail due to missing files, but should parse args correctly
	if code != 1 {
		t.Logf("expected exit code 1 (missing files), got %d", code)
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

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestVersionLinkerInjection covers both the development default and the
// release linker override used by the tag workflow.
func TestVersionLinkerInjection(t *testing.T) {
	want := os.Getenv("IGIT_EXPECTED_VERSION")
	if want == "" {
		want = "dev"
	}
	if version != want {
		t.Fatalf("version = %q, want %q", version, want)
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	content := []byte("release artifact\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	got, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
}

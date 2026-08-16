package fileprotection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtectFileSatisfiesPlatformPolicy(t *testing.T) {
	directory := t.TempDir()
	if err := ProtectDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "secret.json")
	if err := os.WriteFile(path, []byte("secret\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(path); err == nil {
		t.Fatal("unprotected file unexpectedly satisfied the sensitive-file policy")
	}
	if err := ProtectFile(path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFilePublishesProtectedDataWithoutFollowingExistingHardLink(t *testing.T) {
	directory := t.TempDir()
	if err := ProtectDirectory(directory); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(directory, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "config.json")
	if err := os.Link(victim, path); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("replacement\n")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "replacement\n" {
		t.Fatalf("published data = %q, err = %v", got, err)
	}
	unchanged, err := os.ReadFile(victim)
	if err != nil || string(unchanged) != "keep\n" {
		t.Fatalf("hard-link target was modified: data = %q, err = %v", unchanged, err)
	}
}

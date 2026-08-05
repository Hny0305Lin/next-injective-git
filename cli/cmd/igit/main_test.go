package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
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

func TestVisibleReposHidesModeratedRepositoriesByDefault(t *testing.T) {
	repos := []chain.RepoInfo{
		{Name: "active", ModerationStatus: "active"},
		{Name: "legacy", ModerationStatus: "frozen"},
		{Name: "removed", ModerationStatus: "delisted"},
	}
	visible := visibleRepos(repos, false)
	if len(visible) != 1 || visible[0].Name != "active" {
		t.Fatalf("visible repos = %#v", visible)
	}
	if all := visibleRepos(repos, true); len(all) != len(repos) {
		t.Fatalf("--all returned %d repos, want %d", len(all), len(repos))
	}
}

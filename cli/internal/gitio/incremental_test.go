package gitio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func commitHistory(t *testing.T, dir, content string) string {
	t.Helper()
	if e := os.WriteFile(filepath.Join(dir, "history.txt"), []byte(content), 0o600); e != nil {
		t.Fatal(e)
	}
	localGit(t, dir, "add", ".")
	localGit(t, dir, "commit", "-m", content)
	return localGit(t, dir, "rev-parse", "HEAD")
}

// TestRealGitIncrementalChain covers the real-Git behaviour S08 relies on:
// fast-forward detection, new-object counting, a self-contained incremental
// pack, chain ingestion with a single final closure check, missing-dependency
// rejection and byte tampering (BYOS spec 2.3 admission gates).
func TestRealGitIncrementalChain(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	localGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	first := commitHistory(t, dir, "first")
	second := commitHistory(t, dir, "second")
	third := commitHistory(t, dir, "third")
	repo := &Repo{GitDir: filepath.Join(dir, ".git")}

	if !repo.IsAncestor(ctx, first, third) || !repo.IsAncestor(ctx, third, third) {
		t.Fatal("ancestor semantics")
	}
	if repo.IsAncestor(ctx, third, first) {
		t.Fatal("backward move accepted as fast-forward")
	}

	full, err := repo.PackFullHistory(ctx, second, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = full.Close() })

	// A non-fast-forward base is refused; equal tips carry zero new objects.
	if f, count, e := repo.PackIncremental(ctx, first, third, t.TempDir()); e == nil {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("non-fast-forward incremental accepted: count=%d", count)
	}
	if f, count, e := repo.PackIncremental(ctx, second, second, t.TempDir()); e != nil || f != nil || count != 0 {
		t.Fatalf("same-tip incremental: %v file=%v count=%d", e, f != nil, count)
	}

	inc, count, err := repo.PackIncremental(ctx, third, second, t.TempDir())
	if err != nil || count < 1 {
		t.Fatalf("incremental pack: %v count=%d", err, count)
	}
	t.Cleanup(func() { _ = inc.Close() })
	if inc.Source().Size >= full.Source().Size {
		t.Fatalf("incremental pack (%d bytes) is not smaller than full history (%d bytes)", inc.Source().Size, full.Source().Size)
	}

	// Cold chain ingest: base pack, then the incremental pack, then exactly
	// one closure check over the union.
	bare := t.TempDir()
	localGit(t, bare, "init", "--bare", "--object-format=sha1")
	cold := &Repo{GitDir: bare}
	if e := cold.IndexPackVerified(ctx, full, expectedPack(full)); e != nil {
		t.Fatal(e)
	}
	if e := cold.VerifyClosure(ctx, third); e == nil {
		t.Fatal("closure passed before the incremental pack was ingested")
	}
	if e := cold.IndexPackVerified(ctx, inc, expectedPack(inc)); e != nil {
		t.Fatal(e)
	}
	if e := cold.VerifyClosure(ctx, third); e != nil {
		t.Fatal(e)
	}
	if got := localGit(t, bare, "show", third+":history.txt"); got != "third" {
		t.Fatal("chain content:", got)
	}
	localGit(t, bare, "fsck", "--strict")

	// A pack whose declared dependency is absent must never yield a complete
	// ref. Current Git rejects the dangling parent reference inside
	// index-pack --strict already; if a Git build ever accepted the pack, the
	// closure check is the guaranteed fail-closed gate either way (BYOS 2.3).
	lonely := t.TempDir()
	localGit(t, lonely, "init", "--bare", "--object-format=sha1")
	solo := &Repo{GitDir: lonely}
	_ = solo.IndexPackVerified(ctx, inc, expectedPack(inc))
	if e := solo.VerifyClosure(ctx, third); e == nil {
		t.Fatal("incremental pack without its declared dependency passed the closure check")
	}

	// Tampered bytes are rejected before Git sees them.
	if e := os.WriteFile(inc.Source().Path, []byte("tampered"), 0o600); e != nil {
		t.Fatal(e)
	}
	if e := cold.IndexPackVerified(ctx, inc, expectedPack(inc)); e == nil {
		t.Fatal("tampered incremental bytes ingested")
	}
}

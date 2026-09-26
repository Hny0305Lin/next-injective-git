package gitio

import (
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func localGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-config"), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com")
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, out)
	}
	return strings.TrimSpace(string(out))
}
func TestRealGitIndependentFullHistory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	localGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	if e := os.WriteFile(filepath.Join(dir, "history.txt"), []byte("first"), 0600); e != nil {
		t.Fatal(e)
	}
	localGit(t, dir, "add", ".")
	localGit(t, dir, "commit", "-m", "first")
	first := localGit(t, dir, "rev-parse", "HEAD")
	if e := os.WriteFile(filepath.Join(dir, "history.txt"), []byte("second"), 0600); e != nil {
		t.Fatal(e)
	}
	localGit(t, dir, "commit", "-am", "second")
	tip := localGit(t, dir, "rev-parse", "HEAD")
	localGit(t, dir, "branch", "new-branch")
	localGit(t, dir, "tag", "-a", "v1", "-m", "tag")
	tag := localGit(t, dir, "rev-parse", "refs/tags/v1")
	repo := &Repo{GitDir: filepath.Join(dir, ".git")}
	files := map[string]*packstore.File{}
	for name, oid := range map[string]string{"main": tip, "new-branch-zero-increment": tip, "annotated-tag": tag, "force-older-history": first} {
		f, e := repo.PackFullHistory(ctx, oid, t.TempDir())
		if e != nil {
			t.Fatal(name, e)
		}
		files[name] = f
		t.Cleanup(func() { _ = f.Close() })
	}
	localGit(t, dir, "update-ref", "-d", "refs/heads/main")
	localGit(t, dir, "update-ref", "-d", "refs/heads/new-branch")
	localGit(t, dir, "update-ref", "-d", "refs/tags/v1")
	for name, f := range files {
		t.Run(name, func(t *testing.T) {
			dst := t.TempDir()
			localGit(t, dst, "init", "--bare", "--object-format=sha1")
			target := tip
			if name == "annotated-tag" {
				target = tag
			}
			if name == "force-older-history" {
				target = first
			}
			r := &Repo{GitDir: dst}
			if e := r.IndexVerified(ctx, f, expectedPack(f), target); e != nil {
				t.Fatal(e)
			}
			localGit(t, dst, "cat-file", "-e", first)
			if got := localGit(t, dst, "show", first+":history.txt"); got != "first" {
				t.Fatal("history missing", got)
			}
		})
	}
	dst := t.TempDir()
	localGit(t, dst, "init", "--bare")
	if e := (&Repo{GitDir: dst}).IndexVerified(ctx, files["main"], expectedPack(files["main"]), strings.Repeat("f", 40)); e == nil {
		t.Fatal("unreachable target accepted")
	}
	if e := os.WriteFile(files["main"].Source().Path, []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	dst = t.TempDir()
	localGit(t, dst, "init", "--bare")
	if e := (&Repo{GitDir: dst}).IndexVerified(ctx, files["main"], expectedPack(files["main"]), tip); e == nil {
		t.Fatal("tampered bytes ingested")
	}
}

func expectedPack(f *packstore.File) packstore.Object {
	s := f.Source()
	return packstore.Object{Kind: "packs", SHA256: s.SHA256, Size: s.Size}
}

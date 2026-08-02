// Package gitio shells out to the local git binary for packfile plumbing.
// Delegating pack generation/indexing to git keeps the helper tiny and
// guarantees byte-perfect compatibility with every repo layout.
package gitio

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

// Repo wraps a local git directory (the GIT_DIR the helper operates on).
type Repo struct {
	GitDir string
}

// FromEnv resolves GIT_DIR from the environment (set by git for remote
// helpers) with a rev-parse fallback for direct invocation.
func FromEnv() (*Repo, error) {
	if dir := os.Getenv("GIT_DIR"); dir != "" {
		return &Repo{GitDir: dir}, nil
	}
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return nil, i18n.Errorf("not inside a git repository: %w", "当前不在 git 仓库中：%w", err)
	}
	return &Repo{GitDir: strings.TrimSpace(string(out))}, nil
}

func (r *Repo) cmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_DIR="+r.GitDir)
	return cmd
}

// HasObject reports whether the object exists locally.
func (r *Repo) HasObject(sha string) bool {
	return r.cmd("cat-file", "-e", sha+"^{commit}").Run() == nil
}

// ResolveRef returns the sha a local ref points to ("" if missing).
func (r *Repo) ResolveRef(ref string) string {
	out, err := r.cmd("rev-parse", "--verify", "--quiet", ref).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PackObjects builds a packfile containing history reachable from `want`
// minus everything reachable from `exclude` (empty = full history).
// Returns the raw packfile bytes.
func (r *Repo) PackObjects(want string, exclude []string) ([]byte, error) {
	var revs bytes.Buffer
	revs.WriteString(want + "\n")
	for _, ex := range exclude {
		if ex != "" && r.HasObject(ex) {
			revs.WriteString("^" + ex + "\n")
		}
	}
	cmd := r.cmd("pack-objects", "--revs", "--thin", "--delta-base-offset", "--stdout")
	cmd.Stdin = &revs
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, i18n.Errorf("git pack-objects: %w: %s", "git pack-objects 失败：%w：%s", err, errBuf.String())
	}
	return out.Bytes(), nil
}

// IndexPack ingests a packfile stream into the local object database.
func (r *Repo) IndexPack(pack io.Reader) error {
	cmd := r.cmd("index-pack", "--stdin", "--fix-thin")
	cmd.Stdin = pack
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		return i18n.Errorf("git index-pack: %w: %s", "git index-pack 失败：%w：%s", err, errBuf.String())
	}
	return nil
}

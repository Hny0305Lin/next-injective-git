package gitio

import (
	"context"
	"encoding/binary"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func (r *Repo) contextCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_DIR="+r.GitDir, "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	return cmd
}

var sha1OID = regexp.MustCompile("^[0-9a-f]{40}$")

func (r *Repo) requireSHA1Repo(ctx context.Context) error {
	format, err := r.contextCommand(ctx, "rev-parse", "--show-object-format").Output()
	if err != nil || strings.TrimSpace(string(format)) != "sha1" {
		return packstore.Fail(packstore.Invalid, "git-object-format")
	}
	shallow, err := r.contextCommand(ctx, "rev-parse", "--is-shallow-repository").Output()
	if err != nil || strings.TrimSpace(string(shallow)) != "false" {
		return packstore.Fail(packstore.Invalid, "shallow-history")
	}
	return nil
}

// PackFullHistory never excludes remote refs and never requests --thin. A ref
// remains independently cloneable after sibling refs are removed.
func (r *Repo) PackFullHistory(ctx context.Context, tip, dir string) (*packstore.File, error) {
	if !sha1OID.MatchString(tip) {
		return nil, packstore.Fail(packstore.Invalid, "git-tip")
	}
	if err := r.requireSHA1Repo(ctx); err != nil {
		return nil, err
	}
	if err := r.contextCommand(ctx, "cat-file", "-e", tip+"^{commit}").Run(); err != nil {
		return nil, packstore.Fail(packstore.Missing, "git-commit")
	}
	return packstore.Generate(ctx, dir, packmanifest.MaxPackBytes, func(w io.Writer) error {
		cmd := r.contextCommand(ctx, "pack-objects", "--revs", "--delta-base-offset", "--stdout")
		cmd.Stdin = strings.NewReader(tip + "\n")
		cmd.Stdout = w
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return packstore.Fail(packstore.Invalid, "git-pack")
		}
		return nil
	})
}

// IsAncestor reports whether base is an ancestor of tip (or equal). This is
// the client-side fast-forward check; the chain never learns Git ancestry.
func (r *Repo) IsAncestor(ctx context.Context, base, tip string) bool {
	if !sha1OID.MatchString(base) || !sha1OID.MatchString(tip) {
		return false
	}
	return r.contextCommand(ctx, "merge-base", "--is-ancestor", base, tip).Run() == nil
}

// PackIncremental packs exactly the objects reachable from tip but not from
// base, where base must be the same ref's previous tip and an ancestor of tip
// (never a sibling ref). The pack is complete in itself -- no --thin, no
// --fix-thin -- but covers only part of the history: the full closure is the
// manifest chain expressed through dependsOn. count is the number of new
// objects; (nil, 0, nil) means tip adds nothing over base.
func (r *Repo) PackIncremental(ctx context.Context, tip, base, dir string) (*packstore.File, int64, error) {
	if !sha1OID.MatchString(tip) || !sha1OID.MatchString(base) {
		return nil, 0, packstore.Fail(packstore.Invalid, "git-tip")
	}
	if err := r.requireSHA1Repo(ctx); err != nil {
		return nil, 0, err
	}
	if err := r.contextCommand(ctx, "cat-file", "-e", tip+"^{commit}").Run(); err != nil {
		return nil, 0, packstore.Fail(packstore.Missing, "git-commit")
	}
	if err := r.contextCommand(ctx, "cat-file", "-e", base+"^{commit}").Run(); err != nil {
		return nil, 0, packstore.Fail(packstore.Missing, "git-base")
	}
	if !r.IsAncestor(ctx, base, tip) {
		return nil, 0, packstore.Fail(packstore.Invalid, "incremental-base")
	}
	out, err := r.contextCommand(ctx, "rev-list", "--objects", "--count", tip, "--not", base).Output()
	if err != nil {
		return nil, 0, packstore.Fail(packstore.Invalid, "git-rev-list")
	}
	count, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || count < 0 {
		return nil, 0, packstore.Fail(packstore.Invalid, "git-rev-list")
	}
	if count == 0 {
		return nil, 0, nil
	}
	file, err := packstore.Generate(ctx, dir, packmanifest.MaxPackBytes, func(w io.Writer) error {
		cmd := r.contextCommand(ctx, "pack-objects", "--revs", "--delta-base-offset", "--stdout")
		cmd.Stdin = strings.NewReader(tip + "\n^" + base + "\n")
		cmd.Stdout = w
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return packstore.Fail(packstore.Invalid, "git-pack")
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return file, count, nil
}

// IndexPackVerified checks exact bytes and pack structure before Git sees
// them, without --fix-thin and without requiring the ref tip: intermediate
// packs of a chain legitimately lack the tip.
func (r *Repo) IndexPackVerified(ctx context.Context, file *packstore.File, expected packstore.Object) error {
	if expected.Kind != "packs" {
		return packstore.Fail(packstore.Invalid, "git-object")
	}
	f, err := packstore.CheckSource(ctx, file.Source(), expected)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 12)
	if _, err = io.ReadFull(f, header); err != nil || string(header[:4]) != "PACK" || binary.BigEndian.Uint32(header[4:8]) != 2 {
		return packstore.Fail(packstore.Invalid, "git-pack-version")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	cmd := r.contextCommand(ctx, "index-pack", "--stdin", "--strict")
	cmd.Stdin = f
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-index")
	}
	return nil
}

// VerifyClosure runs once after every pack of a manifest has been ingested:
// the commit must exist and its whole reachable history must be present, so a
// chain with a missing dependency pack cannot pass as a complete ref.
func (r *Repo) VerifyClosure(ctx context.Context, tip string) error {
	if !sha1OID.MatchString(tip) {
		return packstore.Fail(packstore.Invalid, "git-tip")
	}
	if err := r.contextCommand(ctx, "cat-file", "-e", tip+"^{commit}").Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-target")
	}
	cmd := r.contextCommand(ctx, "fsck", "--full", "--no-reflogs", tip)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-reachability")
	}
	return nil
}

// IndexVerified composes IndexPackVerified and VerifyClosure for single-pack
// self-contained manifests: exact bytes first, then target and reachability.
// Complete reachability is checked after ingestion; no ref is updated here.
func (r *Repo) IndexVerified(ctx context.Context, file *packstore.File, expected packstore.Object, tip string) error {
	if !sha1OID.MatchString(tip) {
		return packstore.Fail(packstore.Invalid, "git-tip")
	}
	if err := r.IndexPackVerified(ctx, file, expected); err != nil {
		return err
	}
	return r.VerifyClosure(ctx, tip)
}

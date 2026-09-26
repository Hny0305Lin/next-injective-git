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
	"strings"
)

func (r *Repo) contextCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_DIR="+r.GitDir, "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	return cmd
}

// PackFullHistory never excludes remote refs and never requests --thin. A ref
// remains independently cloneable after sibling refs are removed.
func (r *Repo) PackFullHistory(ctx context.Context, tip, dir string) (*packstore.File, error) {
	if !regexp.MustCompile("^[0-9a-f]{40}$").MatchString(tip) {
		return nil, packstore.Fail(packstore.Invalid, "git-tip")
	}
	format, err := r.contextCommand(ctx, "rev-parse", "--show-object-format").Output()
	if err != nil || strings.TrimSpace(string(format)) != "sha1" {
		return nil, packstore.Fail(packstore.Invalid, "git-object-format")
	}
	shallow, err := r.contextCommand(ctx, "rev-parse", "--is-shallow-repository").Output()
	if err != nil || strings.TrimSpace(string(shallow)) != "false" {
		return nil, packstore.Fail(packstore.Invalid, "shallow-history")
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

// IndexVerified checks exact bytes before Git sees them, without --fix-thin.
// Complete reachability is checked afterwards; no ref is updated by this API.
func (r *Repo) IndexVerified(ctx context.Context, file *packstore.File, expected packstore.Object, tip string) error {
	if !regexp.MustCompile("^[0-9a-f]{40}$").MatchString(tip) {
		return packstore.Fail(packstore.Invalid, "git-tip")
	}
	s := file.Source()
	if expected.Kind != "packs" {
		return packstore.Fail(packstore.Invalid, "git-object")
	}
	f, err := packstore.CheckSource(ctx, s, expected)
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
	if err = cmd.Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-index")
	}
	if err = r.contextCommand(ctx, "cat-file", "-e", tip+"^{commit}").Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-target")
	}
	cmd = r.contextCommand(ctx, "fsck", "--full", "--no-reflogs", tip)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return packstore.Fail(packstore.Integrity, "git-reachability")
	}
	return nil
}

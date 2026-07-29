// Package remote implements the git remote-helper protocol
// (gitremote-helpers(7)) for the inj:// transport.
package remote

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs"
)

// Helper runs the remote-helper conversation over in/out.
type Helper struct {
	url   RepoURL
	chain *chain.Client
	ipfs  *ipfs.Client
	git   *gitio.Repo

	in  *bufio.Scanner
	out io.Writer
	log io.Writer // stderr, progress messages for the user

	// remoteRefs caches the on-chain refs fetched during `list`.
	remoteRefs map[string]chain.RefInfo
}

// NewHelper wires up the helper dependencies.
func NewHelper(url RepoURL, cc *chain.Client, ic *ipfs.Client, git *gitio.Repo, in io.Reader, out, log io.Writer) *Helper {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Helper{
		url:        url,
		chain:      cc,
		ipfs:       ic,
		git:        git,
		in:         sc,
		out:        out,
		log:        log,
		remoteRefs: map[string]chain.RefInfo{},
	}
}

func (h *Helper) printf(format string, args ...any) {
	fmt.Fprintf(h.out, format, args...)
}

func (h *Helper) progress(format string, args ...any) {
	fmt.Fprintf(h.log, "igit: "+format+"\n", args...)
}

// Run processes commands until stdin closes.
func (h *Helper) Run() error {
	for h.in.Scan() {
		line := strings.TrimRight(h.in.Text(), "\n")
		switch {
		case line == "capabilities":
			h.printf("fetch\npush\noption\n\n")
		case strings.HasPrefix(line, "option "):
			// accept-and-ignore keeps git happy (verbosity, progress, ...)
			h.printf("ok\n")
		case line == "list" || line == "list for-push":
			if err := h.cmdList(); err != nil {
				return err
			}
		case strings.HasPrefix(line, "fetch "):
			if err := h.cmdFetchBatch(line); err != nil {
				return err
			}
		case strings.HasPrefix(line, "push "):
			if err := h.cmdPushBatch(line); err != nil {
				return err
			}
		case line == "":
			// end of command stream
			return nil
		default:
			return fmt.Errorf("unsupported remote-helper command: %q", line)
		}
	}
	return h.in.Err()
}

// cmdList prints "<sha> <refname>" per on-chain ref plus a HEAD symref.
func (h *Helper) cmdList() error {
	refs, err := h.chain.ListRefs(h.url.Owner, h.url.Repo)
	if err != nil {
		return fmt.Errorf("list refs from chain: %w", err)
	}
	h.remoteRefs = map[string]chain.RefInfo{}
	for _, r := range refs {
		h.remoteRefs[r.RefName] = r
		h.printf("%s %s\n", r.CommitSha, r.RefName)
	}
	// advertise HEAD so clone checks out the default branch
	if info, err := h.chain.RepoInfo(h.url.Owner, h.url.Repo); err == nil {
		headTarget := "refs/heads/" + info.DefaultBranch
		if _, ok := h.remoteRefs[headTarget]; ok {
			h.printf("@%s HEAD\n", headTarget)
		}
	}
	h.printf("\n")
	return nil
}

// cmdFetchBatch consumes "fetch <sha> <ref>" lines (first already read)
// until the terminating blank line, then materializes the needed objects.
func (h *Helper) cmdFetchBatch(first string) error {
	wanted := []string{first}
	for h.in.Scan() {
		line := h.in.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "fetch ") {
			wanted = append(wanted, line)
		}
	}

	// collect the pack URI set across all requested refs, preserving order
	seen := map[string]bool{}
	var uris []string
	for _, w := range wanted {
		parts := strings.Fields(w)
		if len(parts) != 3 {
			return fmt.Errorf("malformed fetch command: %q", w)
		}
		refName := parts[2]
		entry, ok := h.remoteRefs[refName]
		if !ok {
			// list may not have run in this process; resolve directly
			_, refURIs, err := h.chain.ResolveRef(h.url.Owner, h.url.Repo, refName)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", refName, err)
			}
			entry = chain.RefInfo{RefName: refName, PackURIs: refURIs}
		}
		for _, uri := range entry.PackURIs {
			if !seen[uri] {
				seen[uri] = true
				uris = append(uris, uri)
			}
		}
	}

	for i, uri := range uris {
		h.progress("downloading packfile %d/%d (%s)", i+1, len(uris), uri)
		body, err := h.fetchPack(uri)
		if err != nil {
			return err
		}
		err = h.git.IndexPack(body)
		body.Close()
		if err != nil {
			return fmt.Errorf("ingest packfile %s: %w", uri, err)
		}
	}
	h.printf("\n")
	return nil
}

// fetchPack downloads one pack by storage URI. Only ipfs:// is supported
// today; bare CIDs are accepted for pre-URI on-chain entries.
func (h *Helper) fetchPack(uri string) (io.ReadCloser, error) {
	if cid, ok := strings.CutPrefix(uri, "ipfs://"); ok {
		return h.ipfs.Cat(cid)
	}
	if strings.Contains(uri, "://") {
		return nil, fmt.Errorf("unsupported pack uri scheme: %s", uri)
	}
	return h.ipfs.Cat(uri)
}

type pushSpec struct {
	src   string
	dst   string
	force bool
}

// cmdPushBatch consumes "push <spec>" lines (first already read) until the
// blank line, executes each spec, and reports ok/error per ref.
func (h *Helper) cmdPushBatch(first string) error {
	specs := []pushSpec{parsePushSpec(first)}
	for h.in.Scan() {
		line := h.in.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "push ") {
			specs = append(specs, parsePushSpec(line))
		}
	}

	for _, spec := range specs {
		if err := h.pushOne(spec); err != nil {
			h.printf("error %s %s\n", spec.dst, sanitizeErr(err))
		} else {
			h.printf("ok %s\n", spec.dst)
		}
	}
	h.printf("\n")
	return nil
}

func parsePushSpec(line string) pushSpec {
	raw := strings.TrimPrefix(line, "push ")
	force := strings.HasPrefix(raw, "+")
	raw = strings.TrimPrefix(raw, "+")
	src, dst, _ := strings.Cut(raw, ":")
	return pushSpec{src: src, dst: dst, force: force}
}

func (h *Helper) pushOne(spec pushSpec) error {
	// empty src means delete the remote ref
	if spec.src == "" {
		h.progress("deleting %s on chain", spec.dst)
		return h.chain.DeleteRef(h.url.Owner, h.url.Repo, spec.dst)
	}

	localSha := h.git.ResolveRef(spec.src)
	if localSha == "" {
		return fmt.Errorf("cannot resolve local ref %s", spec.src)
	}

	// build incremental pack: exclude every remote tip we already have.
	// force pushes are the exception: the contract replaces the whole CID
	// list, so the new pack must be self-contained (full history).
	var exclude []string
	expectedSha := ""
	if prev, ok := h.remoteRefs[spec.dst]; ok {
		expectedSha = prev.CommitSha
	}
	if !spec.force {
		for _, r := range h.remoteRefs {
			exclude = append(exclude, r.CommitSha)
		}
	}

	h.progress("packing objects for %s", spec.dst)
	pack, err := h.git.PackObjects(localSha, exclude)
	if err != nil {
		return err
	}

	var cids []string
	// A pack with zero objects means everything reachable from localSha is
	// already covered by packs referenced from other refs. An existing ref
	// keeps its URI list; a brand-new ref (tag / branch alias) must stay
	// fetchable even if those other refs are deleted later, so it gets a
	// self-contained full pack instead.
	if packEmpty(pack) {
		if prev, ok := h.remoteRefs[spec.dst]; ok {
			cids = prev.PackURIs
		} else {
			h.progress("no new objects; building self-contained pack for %s", spec.dst)
			if pack, err = h.git.PackObjects(localSha, nil); err != nil {
				return err
			}
		}
	}
	if len(cids) == 0 {
		h.progress("uploading packfile (%d bytes) to IPFS", len(pack))
		cid, err := h.ipfs.Add(spec.dst+".pack", bytes.NewReader(pack))
		if err != nil {
			return err
		}
		cids = append(cids, "ipfs://"+cid)
		h.progress("packfile pinned: %s", cid)
	}

	h.progress("broadcasting update_ref tx for %s -> %s", spec.dst, localSha[:8])
	return h.chain.UpdateRef(
		h.url.Owner, h.url.Repo, spec.dst, localSha, cids, expectedSha, spec.force,
	)
}

// packEmpty reports whether a packfile stream contains zero objects
// (object count lives in the big-endian uint32 at bytes 8..12).
func packEmpty(pack []byte) bool {
	if len(pack) < 12 {
		return true
	}
	return binary.BigEndian.Uint32(pack[8:12]) == 0
}

// sanitizeErr flattens an error into the single-line form the protocol needs.
func sanitizeErr(err error) string {
	return strings.ReplaceAll(err.Error(), "\n", " ")
}

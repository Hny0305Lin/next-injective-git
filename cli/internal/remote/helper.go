// Package remote implements the git remote-helper protocol
// (gitremote-helpers(7)) for the inj:// transport.
package remote

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/byos"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/replication"
)

// Progress verbosity tiers mirror git's `option verbosity` values: 0 (`git
// push -q` / `git clone -q`) keeps warnings and errors only, 1 (default)
// prints one milestone line per phase, >=2 (`git -v`) keeps every diagnostic
// detail line.
const (
	verbosityQuiet   = 0
	verbosityDefault = 1
	verbosityVerbose = 2
)

// Helper runs the remote-helper conversation over in/out.
type Helper struct {
	url         RepoURL
	chain       chain.RepoRegistryBackend
	ipfs        ipfsClient
	replication replication.Authorizer
	uploadPeers []string
	git         gitRepo
	preflight   func(needsKubo bool) error
	byos        *byos.Service
	tmp         string

	// verbosity filters progress lines; envPinned records an IGIT_QUIET /
	// IGIT_VERBOSE override, which must win over git's option command.
	verbosity int
	envPinned bool

	in  *bufio.Scanner
	out io.Writer
	log io.Writer // stderr, progress messages for the user

	// remoteRefs caches the on-chain refs fetched during `list`.
	remoteRefs map[string]chain.RefInfo
	resolved   *chain.ResolvedRepo
}

// SetPushPreflight installs a callback that runs once per push batch before
// any ref is resolved or packed.
func (h *Helper) SetPushPreflight(preflight func(needsKubo bool) error) {
	h.preflight = preflight
}

type ipfsClient interface {
	AddTemporary(name string, r io.Reader) (string, error)
	GetFromGateways(cid string) (io.ReadCloser, error)
	SwarmConnect(multiaddr string) error
	GC() error
}

type gitRepo interface {
	ResolveRef(ref string) string
	PackObjects(tip string, exclude []string) ([]byte, error)
	IndexPack(pack io.Reader) error
	ScanSecrets(ref string) []string
}

// NewHelper wires up the helper dependencies.
func NewHelper(url RepoURL, cc chain.RepoRegistryBackend, ic ipfsClient, rc replication.Authorizer, uploadPeers []string, git gitRepo, in io.Reader, out, log io.Writer) *Helper {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	h := &Helper{
		url:         url,
		chain:       cc,
		ipfs:        ic,
		replication: rc,
		uploadPeers: uploadPeers,
		git:         git,
		in:          sc,
		out:         out,
		log:         log,
		verbosity:   verbosityDefault,
		remoteRefs:  map[string]chain.RefInfo{},
	}
	if quiet, verbose := EnvVerbosityOverrides(); quiet || verbose {
		// An explicit environment override must survive later `option
		// verbosity` commands from git.
		h.envPinned = true
		if quiet {
			h.verbosity = verbosityQuiet
		} else {
			h.verbosity = verbosityVerbose
		}
	}
	return h
}

// EnvVerbosityOverrides reports the IGIT_QUIET=1 / IGIT_VERBOSE=1 overrides.
// They let users force a level git itself cannot express (e.g. a quiet
// `igit clone` that does not pass -q through to the helper).
func EnvVerbosityOverrides() (quiet, verbose bool) {
	return envFlag("IGIT_QUIET"), envFlag("IGIT_VERBOSE")
}

// EnvVerboseOverride reports whether IGIT_VERBOSE=1 pins the verbose level.
func EnvVerboseOverride() bool { return envFlag("IGIT_VERBOSE") }

func envFlag(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// applyOption handles the option command. verbosity carries the -q/-v level
// git forwards to helpers; progress is accepted because git always sends it
// but carries no extra level information. Unknown options are unsupported so
// git can fall back to its own behavior.
func (h *Helper) applyOption(arg string) bool {
	name, value, ok := strings.Cut(strings.TrimSpace(arg), " ")
	if !ok {
		return false
	}
	switch name {
	case "verbosity":
		level, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return false
		}
		if level < verbosityQuiet {
			level = verbosityQuiet
		}
		if !h.envPinned {
			h.verbosity = level
		}
		return true
	case "progress":
		_, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil
	}
	return false
}

func (h *Helper) printf(format string, args ...any) {
	fmt.Fprintf(h.out, format, args...)
}

// progress writes one raw stderr line regardless of verbosity. Warnings and
// errors use it; informational lines go through step or detail instead.
func (h *Helper) progress(english, chinese string, args ...any) {
	fmt.Fprintf(h.log, "igit: "+i18n.Text(english, chinese)+"\n", args...)
}

// step prints a milestone line at the default level and above.
func (h *Helper) step(english, chinese string, args ...any) {
	if h.verbosity >= verbosityDefault {
		h.progress(english, chinese, args...)
	}
}

// detail prints a diagnostic line only at the verbose level.
func (h *Helper) detail(english, chinese string, args ...any) {
	if h.verbosity >= verbosityVerbose {
		h.progress(english, chinese, args...)
	}
}

// Run processes commands until stdin closes.
func (h *Helper) Run() error {
	for h.in.Scan() {
		line := strings.TrimRight(h.in.Text(), "\n")
		switch {
		case line == "capabilities":
			h.printf("fetch\npush\noption\n\n")
		case strings.HasPrefix(line, "option "):
			// Known options (verbosity, progress) shape the progress output;
			// unknown ones are reported unsupported per gitremote-helpers(7).
			if h.applyOption(line[len("option "):]) {
				h.printf("ok\n")
			} else {
				h.printf("unsupported\n")
			}
		case line == "list":
			if h.byos != nil {
				if err := h.byosList(); err != nil {
					return err
				}
				continue
			}
			if err := h.cmdList(false); err != nil {
				return err
			}
		case line == "list for-push":
			if h.byos != nil {
				if err := h.byosList(); err != nil {
					return err
				}
				continue
			}
			if err := h.cmdList(true); err != nil {
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
			return i18n.Errorf("unsupported remote-helper command: %q", "不支持的 remote-helper 命令：%q", line)
		}
	}
	return h.in.Err()
}

// cmdList prints "<sha> <refname>" per on-chain ref plus a HEAD symref.
func (h *Helper) cmdList(forPush bool) error {
	resolved, err := h.resolveRepo()
	if err != nil {
		return i18n.Errorf("resolve repository from chain: %w", "从链上解析仓库失败：%w", err)
	}
	if forPush {
		if err := movedError(resolved); err != nil {
			return err
		}
	}
	refs, err := h.chain.ListRefs(h.url.Owner, h.url.Repo)
	if err != nil {
		return i18n.Errorf("list refs from chain: %w", "从链上获取 refs 失败：%w", err)
	}
	h.remoteRefs = map[string]chain.RefInfo{}
	for _, r := range refs {
		h.remoteRefs[r.RefName] = r
		h.printf("%s %s\n", r.CommitSha, r.RefName)
	}
	// advertise HEAD so clone checks out the default branch
	if resolved.Info.DefaultBranch != "" {
		headTarget := "refs/heads/" + resolved.Info.DefaultBranch
		if _, ok := h.remoteRefs[headTarget]; ok {
			h.printf("@%s HEAD\n", headTarget)
		}
	}
	h.printf("\n")
	return nil
}

func (h *Helper) resolveRepo() (*chain.ResolvedRepo, error) {
	if h.resolved != nil {
		return h.resolved, nil
	}
	resolved, err := h.chain.ResolveRepo(h.url.Owner, h.url.Repo)
	if err != nil {
		return nil, err
	}
	h.resolved = resolved
	return resolved, nil
}

func movedError(resolved *chain.ResolvedRepo) error {
	if resolved == nil {
		return nil
	}
	if resolved.IsCanonical {
		return nil
	}
	return &chain.RepoMovedError{
		RepoID:       resolved.RepoID,
		CurrentOwner: resolved.Canonical.Owner,
		Name:         resolved.Canonical.Name,
	}
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

	if h.byos != nil {
		return h.byosFetch(wanted)
	}

	// collect the pack URI set across all requested refs, preserving order
	seen := map[string]bool{}
	var uris []string
	for _, w := range wanted {
		parts := strings.Fields(w)
		if len(parts) != 3 {
			return i18n.Errorf("malformed fetch command: %q", "格式错误的 fetch 命令：%q", w)
		}
		refName := parts[2]
		entry, ok := h.remoteRefs[refName]
		if !ok {
			// list may not have run in this process; resolve directly
			_, refURIs, err := h.chain.ResolveRef(h.url.Owner, h.url.Repo, refName)
			if err != nil {
				return i18n.Errorf("resolve %s: %w", "解析 %s 失败：%w", refName, err)
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
		h.step("downloading packfile %d/%d (%s)", "正在下载 packfile %d/%d（%s）", i+1, len(uris), uri)
		body, err := h.fetchPack(uri)
		if err != nil {
			return err
		}
		err = h.git.IndexPack(body)
		body.Close()
		if err != nil {
			return i18n.Errorf("ingest packfile %s: %w", "导入 packfile %s 失败：%w", uri, err)
		}
	}
	h.printf("\n")
	return nil
}

// fetchPack downloads one pack by storage URI. Only ipfs:// is supported
// today; bare CIDs are accepted for pre-URI on-chain entries.
func (h *Helper) fetchPack(uri string) (io.ReadCloser, error) {
	if cid, ok := strings.CutPrefix(uri, "ipfs://"); ok {
		return h.ipfs.GetFromGateways(cid)
	}
	if strings.Contains(uri, "://") {
		return nil, i18n.Errorf("unsupported pack uri scheme: %s", "不支持的 pack URI 协议：%s", uri)
	}
	return h.ipfs.GetFromGateways(uri)
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
	if h.byos == nil {
		resolved, resolveErr := h.resolveRepo()
		if resolveErr == nil {
			resolveErr = movedError(resolved)
		}
		if resolveErr != nil {
			for _, spec := range specs {
				h.printf("error %s %s\n", spec.dst, sanitizeErr(resolveErr))
			}
			h.printf("\n")
			return nil
		}
	}
	if h.preflight != nil {
		needsKubo := false
		if h.byos == nil {
			for _, spec := range specs {
				if spec.src != "" {
					needsKubo = true
					break
				}
			}
		}
		if err := h.preflight(needsKubo); err != nil {
			for _, spec := range specs {
				h.printf("error %s %s\n", spec.dst, sanitizeErr(err))
			}
			h.printf("\n")
			return nil
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
	if h.byos != nil {
		return h.pushOneByos(spec)
	}

	// empty src means delete the remote ref
	if spec.src == "" {
		h.step("deleting %s on chain", "正在从链上删除 %s", spec.dst)
		return h.chain.DeleteRef(h.url.Owner, h.url.Repo, spec.dst)
	}

	localSha := h.git.ResolveRef(spec.src)
	if localSha == "" {
		return i18n.Errorf("cannot resolve local ref %s", "无法解析本地 ref %s", spec.src)
	}
	for _, match := range h.git.ScanSecrets(localSha) {
		h.progress("warning: possible credential in %s", "警告：%s 中可能包含凭据", match)
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

	h.step("packing objects for %s", "正在为 %s 打包对象", spec.dst)
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
			h.detail("no new objects; building self-contained pack for %s", "没有新对象，正在为 %s 创建自包含 pack", spec.dst)
			if pack, err = h.git.PackObjects(localSha, nil); err != nil {
				return err
			}
		}
	}
	if len(cids) == 0 {
		h.step("uploading packfile (%d bytes) to IPFS", "正在将 packfile（%d 字节）上传到 IPFS", len(pack))
		cid, err := h.ipfs.AddTemporary(spec.dst+".pack", bytes.NewReader(pack))
		if err != nil {
			return err
		}
		cids = append(cids, "ipfs://"+cid)
		h.detail("temporary local pack added: %s", "临时本地 pack 已添加：%s", cid)
		if len(h.uploadPeers) == 0 || strings.TrimSpace(h.uploadPeers[0]) == "" {
			return i18n.Errorf("US Kubo swarm peer is not configured; set upload.us_peer to the US service multiaddr", "未配置 US Kubo swarm peer；请将 upload.us_peer 设为 US 服务的 multiaddr")
		}
		if err := h.ipfs.SwarmConnect(h.uploadPeers[0]); err != nil {
			return i18n.Errorf("connect temporary local Kubo to US peer: %w", "连接临时本地 Kubo 到 US peer 失败：%w", err)
		}
		for _, peer := range h.uploadPeers[1:] {
			peer = strings.TrimSpace(peer)
			if peer == "" {
				continue
			}
			if err := h.ipfs.SwarmConnect(peer); err != nil {
				h.progress("warning: optional upload peer connection failed: %v", "警告：可选上传节点连接失败：%v", err)
			}
		}
		if h.replication == nil {
			return i18n.Errorf("US replication client is not configured", "未配置 US replication 客户端")
		}
		sum := sha256.Sum256(pack)
		owner, repo := h.url.Owner, h.url.Repo
		if h.resolved != nil {
			owner, repo = h.resolved.Canonical.Owner, h.resolved.Canonical.Name
		}
		replicationRequest := replication.Request{
			CID: cid, Owner: owner, Repo: repo, Ref: spec.dst,
			PackSHA256: fmt.Sprintf("%x", sum), Size: int64(len(pack)),
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		}
		h.detail("requesting CID-bound upload authorization for %s", "正在请求绑定 CID 的上传授权：%s", cid)
		scopedReplication, err := h.replication.Authorize(replicationRequest)
		if err != nil {
			return err
		}
		h.detail("requesting US Kubo replication and Pin confirmation for %s", "正在请求 US Kubo 复制和 Pin 确认：%s", cid)
		if _, err := scopedReplication.Confirm(replicationRequest); err != nil {
			return err
		}
		h.step("US Kubo confirmed durable Pin: %s", "US Kubo 已确认持久 Pin：%s", cid)
	}

	h.step("broadcasting update_ref tx for %s -> %s", "正在广播 update_ref 交易：%s -> %s", spec.dst, localSha[:8])
	if err := h.chain.UpdateRef(
		h.url.Owner, h.url.Repo, spec.dst, localSha, cids, expectedSha, spec.force,
	); err != nil {
		// Keep temporary blocks after a failed transaction so the same upload can
		// be retried. The US service reclaims an unreferenced pin after its TTL.
		return err
	}
	if err := h.ipfs.GC(); err != nil {
		h.progress("warning: update_ref succeeded; local temporary GC failed: %v", "警告：update_ref 成功；本地临时 GC 失败：%v", err)
	} else {
		h.detail("update_ref succeeded; local unpinned temporary blocks GC completed", "update_ref 成功；本地未 Pin 的临时区块 GC 已完成")
	}
	return nil
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

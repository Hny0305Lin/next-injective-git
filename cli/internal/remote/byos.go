package remote

import (
	"context"
	"os"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/byos"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

// SetByosStorage installs the storage-neutral successor path. When set, list,
// fetch and push speak the successor ABI and verified BYOS storage instead of
// the legacy IPFS flow; the legacy path is untouched otherwise.
func (h *Helper) SetByosStorage(service *byos.Service) { h.byos = service }

func (h *Helper) byosActive() bool { return h.byos != nil }

func (h *Helper) byosList() error {
	ctx := context.Background()
	listings, defaultBranch, err := h.byos.ListRefs(ctx, h.url.Owner, h.url.Repo)
	if err != nil {
		return i18n.Errorf("list successor refs: %w", "获取 successor refs 失败：%w", err)
	}
	h.remoteRefs = map[string]chain.RefInfo{}
	for _, listing := range listings {
		h.remoteRefs[listing.RefName] = chain.RefInfo{RefName: listing.RefName, CommitSha: listing.CommitOID}
		h.printf("%s %s\n", listing.CommitOID, listing.RefName)
	}
	if defaultBranch != "" {
		headTarget := "refs/heads/" + defaultBranch
		if _, ok := h.remoteRefs[headTarget]; ok {
			h.printf("@%s HEAD\n", headTarget)
		}
	}
	h.printf("\n")
	return nil
}

func (h *Helper) byosFetch(wanted []string) error {
	ctx := context.Background()
	for _, w := range wanted {
		parts := strings.Fields(w)
		if len(parts) != 3 {
			return i18n.Errorf("malformed fetch command: %q", "格式错误的 fetch 命令：%q", w)
		}
		sha, refName := parts[1], parts[2]
		h.progress("fetching verified manifest and packs for %s", "正在获取 %s 的已验证 manifest 与 pack", refName)
		if err := h.byos.FetchRef(ctx, h.url.Owner, h.url.Repo, refName, sha, h.tmpDir()); err != nil {
			return err
		}
	}
	h.printf("\n")
	return nil
}

func (h *Helper) pushOneByos(spec pushSpec) error {
	ctx := context.Background()
	return h.byos.PushRef(ctx, h.url.Owner, h.url.Repo, spec.src, spec.dst, spec.force, h.tmpDir())
}

// tmpDir lazily creates the session temp directory used for verified spooling.
func (h *Helper) tmpDir() string {
	if h.tmp == "" {
		dir, err := os.MkdirTemp("", "igit-byos-*")
		if err != nil {
			return "."
		}
		h.tmp = dir
	}
	return h.tmp
}

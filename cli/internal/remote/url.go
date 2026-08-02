package remote

import (
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
)

// RepoURL is a parsed igit:// remote URL.
type RepoURL struct {
	// Owner is the repository owner: an Injective bech32 address or a
	// registered username (resolved on-chain before use).
	Owner string
	// Repo is the repository name.
	Repo string
}

// OwnerIsAddress reports whether Owner is already a bech32 address
// (otherwise it is a username that needs on-chain resolution).
func (u RepoURL) OwnerIsAddress() bool {
	return strings.HasPrefix(u.Owner, "inj1")
}

// ValidUsername mirrors the contract's username rules:
// 3-32 chars, [a-z0-9-], no leading/trailing '-', never an address prefix.
func ValidUsername(name string) bool {
	if len(name) < 3 || len(name) > 32 ||
		strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") ||
		strings.HasPrefix(name, "inj1") {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// ParseURL parses "igit://<owner>/<repo>" (the canonical scheme) and the
// "igit::<owner>/<repo>" transport form git produces for `git clone igit::...`.
// Owner may be a bech32 address or a registered username (§4).
func ParseURL(raw string) (RepoURL, error) {
	s := raw
	for _, prefix := range []string{"igit://", "igit::"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepoURL{}, i18n.Errorf("invalid remote URL %q (expected igit://<owner>/<repo>)", "无效的远程 URL %q（应为 igit://<owner>/<repo>）", raw)
	}
	if !strings.HasPrefix(parts[0], "inj1") && !ValidUsername(parts[0]) {
		return RepoURL{}, i18n.Errorf(
			"invalid owner %q: expected an inj1... address or a registered username", "所有者 %q 无效：应为 inj1... 地址或已注册用户名", parts[0])
	}
	return RepoURL{Owner: parts[0], Repo: parts[1]}, nil
}

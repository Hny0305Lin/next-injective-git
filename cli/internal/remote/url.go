package remote

import (
	"fmt"
	"strings"
)

// RepoURL is a parsed inj:// remote URL.
type RepoURL struct {
	// Owner is the repository owner's Injective bech32 address.
	Owner string
	// Repo is the repository name.
	Repo string
}

// ParseURL parses "inj://<owner>/<repo>" (also accepts "inj::<owner>/<repo>"
// which git produces for the `git clone inj::...` transport syntax).
func ParseURL(raw string) (RepoURL, error) {
	s := raw
	switch {
	case strings.HasPrefix(s, "inj://"):
		s = strings.TrimPrefix(s, "inj://")
	case strings.HasPrefix(s, "inj::"):
		s = strings.TrimPrefix(s, "inj::")
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepoURL{}, fmt.Errorf("invalid remote URL %q (expected inj://<owner>/<repo>)", raw)
	}
	if !strings.HasPrefix(parts[0], "inj1") {
		return RepoURL{}, fmt.Errorf("invalid owner %q: expected an inj1... bech32 address", parts[0])
	}
	return RepoURL{Owner: parts[0], Repo: parts[1]}, nil
}

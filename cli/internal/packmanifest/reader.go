package packmanifest

import (
	"bytes"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/safehttp"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
)

// ParseRefManifest is the cold-reader entry point for clients that do not yet
// know the commit OID: the ref's on-chain commitment binds the manifest digest,
// and the commit is learned from the verified manifest body itself. Every other
// context field (chainId, suiteDirectory, repoId, refName) and the whole
// commitment check stay identical to Parse. base.Commit must be the zero value.
func ParseRefManifest(b []byte, base Context, c ManifestCommitment) (PackManifest, error) {
	var m PackManifest
	if base.Commit != (Commit{}) {
		return m, ErrManifest
	}
	n, err := Size(c.Size, MaxManifestBytes)
	if err != nil || int64(len(b)) != n || !ValidDigest(c.SHA256) || Digest(b) != c.SHA256 || !validBaseContext(base) {
		return m, ErrManifest
	}
	if _, err := safehttp.ValidateURL(c.BootstrapLocator); err != nil {
		return m, ErrManifest
	}
	if err := strictjson.Decode(b, MaxManifestBytes, &m); err != nil {
		return m, ErrManifest
	}
	encoded, err := Encode(m)
	if err != nil || !bytes.Equal(b, encoded) || m.Context.ChainID != base.ChainID || m.Context.SuiteDirectory != base.SuiteDirectory || m.Context.RepoID != base.RepoID || m.Context.RefName != base.RefName {
		return PackManifest{}, ErrManifest
	}
	return m, nil
}

// validBaseContext checks every binding field except the commit by validating a
// probe context with a syntactically valid placeholder OID.
func validBaseContext(base Context) bool {
	probe := base
	probe.Commit = Commit{Algorithm: "sha1", OID: "1" + strings.Repeat("0", 39)}
	return probe.Validate() == nil
}
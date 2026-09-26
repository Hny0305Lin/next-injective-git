// Package packmanifest defines storage-neutral manifests, separate from v3 URIs.
package packmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/safehttp"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxManifestBytes       = 64 << 10
	MaxPacks               = 16
	MaxLocations           = 4
	MaxPackBytes     int64 = 512 << 20
	MaxTotalBytes    int64 = 2 << 30
)

var ErrManifest = errors.New("invalid manifest: schema, canonical bytes, context, commitment or limits")
var hex64 = regexp.MustCompile("^[0-9a-f]{64}$")
var address = regexp.MustCompile("^0x[0-9a-f]{40}$")
var repoID = regexp.MustCompile("^0x[0-9a-f]{64}$")
var oid = regexp.MustCompile("^[0-9a-f]{40}$")
var decimal = regexp.MustCompile("^[1-9][0-9]*$")
var prefix = regexp.MustCompile("^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$")
var readerID = regexp.MustCompile("^[a-zA-Z0-9_-]{1,64}$")
var cid = regexp.MustCompile("^ipfs://(b[a-z2-7]{20,120}|Qm[1-9A-HJ-NP-Za-km-z]{44})$")

type Commit struct {
	Algorithm string `json:"algorithm"`
	OID       string `json:"oid"`
}
type Context struct {
	ChainID        string `json:"chainId"`
	SuiteDirectory string `json:"suiteDirectory"`
	RepoID         string `json:"repoId"`
	RefName        string `json:"refName"`
	Commit         Commit `json:"commit"`
}

// Exactly one of URL/Reader is nonempty, but both fields are required.
// Reader is a public mapping label; no bucket or credentials are embedded.
type PackLocation struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Reader   string `json:"reader"`
}
type PackEntry struct {
	Sequence    int            `json:"sequence"`
	SHA256      string         `json:"sha256"`
	Size        string         `json:"size"`
	Format      string         `json:"format"`
	PackVersion int            `json:"packVersion"`
	Thin        bool           `json:"thin"`
	DependsOn   []string       `json:"dependsOn"`
	Locations   []PackLocation `json:"locations"`
}
type PackManifest struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schemaVersion"`
	Context
	Packs []PackEntry `json:"packs"`
}
type ManifestCommitment struct {
	SHA256           string `json:"sha256"`
	Size             string `json:"size"`
	BootstrapLocator string `json:"bootstrapLocator"`
}

func Digest(b []byte) string    { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func ValidDigest(s string) bool { return hex64.MatchString(s) }
func ValidPrefix(s string) bool { return len(s) <= 128 && prefix.MatchString(s) }
func Key(p, kind, digest string) (string, error) {
	if !ValidPrefix(p) || !ValidDigest(digest) {
		return "", ErrManifest
	}
	suffix := ".pack"
	if kind == "manifests" {
		suffix = ".json"
	} else if kind != "packs" {
		return "", ErrManifest
	}
	return p + "/" + kind + "/sha256/" + digest + suffix, nil
}
func Size(s string, max int64) (int64, error) {
	if len(s) > 20 || !decimal.MatchString(s) {
		return 0, ErrManifest
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n > max {
		return 0, ErrManifest
	}
	return n, nil
}
func (c Context) Validate() error {
	if len(c.ChainID) > 78 {
		return ErrManifest
	}
	n, ok := new(big.Int).SetString(c.ChainID, 10)
	if !decimal.MatchString(c.ChainID) || !ok || n.BitLen() > 256 || !address.MatchString(c.SuiteDirectory) || !repoID.MatchString(c.RepoID) || !validRef(c.RefName) || c.Commit.Algorithm != "sha1" || !oid.MatchString(c.Commit.OID) || c.Commit.OID == strings.Repeat("0", 40) {
		return ErrManifest
	}
	return nil
}
func validRef(s string) bool {
	if !utf8.ValidString(s) || len(s) > 255 || !(strings.HasPrefix(s, "refs/heads/") || strings.HasPrefix(s, "refs/tags/")) || strings.ContainsAny(s, " ~^:?*[\\") || strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	for _, p := range strings.Split(s, "/") {
		if p == "" || strings.HasPrefix(p, ".") || strings.HasSuffix(p, ".lock") {
			return false
		}
	}
	return true
}
func (l PackLocation) Validate() error {
	if l.Provider != "aws-s3" && l.Provider != "cloudflare-r2" && l.Provider != "ipfs" {
		return ErrManifest
	}
	if (l.URL == "") == (l.Reader == "") {
		return ErrManifest
	}
	if l.Reader != "" {
		if l.Provider == "ipfs" || !readerID.MatchString(l.Reader) {
			return ErrManifest
		}
		return nil
	}
	if l.Provider == "ipfs" {
		if !cid.MatchString(l.URL) {
			return ErrManifest
		}
		return nil
	}
	if _, err := safehttp.ValidateURL(l.URL); err != nil {
		return ErrManifest
	}
	return nil
}
func (m PackManifest) Validate() error {
	if m.Schema != "igit.pack-manifest" || m.SchemaVersion != 1 || m.Context.Validate() != nil || len(m.Packs) < 1 || len(m.Packs) > MaxPacks {
		return ErrManifest
	}
	seen := map[string]bool{}
	var total int64
	for i, p := range m.Packs {
		n, err := Size(p.Size, MaxPackBytes)
		if err != nil || n < 32 || p.Sequence != i || !ValidDigest(p.SHA256) || seen[p.SHA256] || p.Format != "git-pack" || p.PackVersion != 2 || p.Thin || p.DependsOn == nil || len(p.DependsOn) != 0 || len(p.Locations) < 1 || len(p.Locations) > MaxLocations {
			return ErrManifest
		}
		seen[p.SHA256] = true
		total += n
		if total > MaxTotalBytes {
			return ErrManifest
		}
		locations := map[PackLocation]bool{}
		for _, l := range p.Locations {
			if l.Validate() != nil || locations[l] {
				return ErrManifest
			}
			locations[l] = true
		}
	}
	return nil
}
func Encode(m PackManifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, ErrManifest
	}
	return strictjson.Canonical(b, MaxManifestBytes)
}

// Parse requires canonical bytes, all fields and an independently trusted context.
func Parse(b []byte, expected Context, c ManifestCommitment) (PackManifest, error) {
	var m PackManifest
	n, err := Size(c.Size, MaxManifestBytes)
	if err != nil || int64(len(b)) != n || !ValidDigest(c.SHA256) || Digest(b) != c.SHA256 || expected.Validate() != nil {
		return m, ErrManifest
	}
	if _, err := safehttp.ValidateURL(c.BootstrapLocator); err != nil {
		return m, ErrManifest
	}
	if err := strictjson.Decode(b, MaxManifestBytes, &m); err != nil {
		return m, ErrManifest
	}
	encoded, err := Encode(m)
	if err != nil || !bytes.Equal(b, encoded) || m.Context != expected {
		return PackManifest{}, ErrManifest
	}
	return m, nil
}

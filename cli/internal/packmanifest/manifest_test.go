package packmanifest

import (
	"encoding/json"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
	"os"
	"strconv"
	"strings"
	"testing"
)

type vector struct {
	Name       string
	Kind       string
	Input      string
	Canonical  string
	SHA256     string
	Context    Context
	Commitment ManifestCommitment
}
type fixture struct {
	Vectors []vector
	Invalid []vector
}

func fixtures(t *testing.T) fixture {
	t.Helper()
	b, e := os.ReadFile("../../../protocol/packmanifest/vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var f fixture
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestSharedVectors(t *testing.T) {
	f := fixtures(t)
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			b, e := strictjson.Canonical([]byte(v.Input), MaxManifestBytes)
			if e != nil || string(b) != v.Canonical || Digest(b) != v.SHA256 {
				t.Fatalf("JCS mismatch: %s %v", b, e)
			}
			if v.Kind == "manifest" {
				m, e := Parse(b, v.Context, v.Commitment)
				if e != nil {
					t.Fatal(e)
				}
				again, e := Encode(m)
				if e != nil || string(again) != v.Canonical {
					t.Fatal("encode mismatch", e)
				}
				changed := v.Context
				changed.RepoID = "0x" + strings.Repeat("f", 64)
				if _, e = Parse(b, changed, v.Commitment); e == nil {
					t.Fatal("cross-repo replacement")
				}
				c := v.Commitment
				c.SHA256 = strings.Repeat("0", 64)
				if _, e = Parse(b, v.Context, c); e == nil {
					t.Fatal("wrong digest")
				}
				c = v.Commitment
				c.Size = "1"
				if _, e = Parse(b, v.Context, c); e == nil {
					t.Fatal("wrong size")
				}
			}
		})
	}
	for _, v := range f.Invalid {
		t.Run("reject-"+v.Name, func(t *testing.T) {
			b, e := strictjson.Canonical([]byte(v.Input), MaxManifestBytes)
			if e != nil {
				return
			}
			if v.Kind == "jcs" {
				t.Fatal("accepted invalid JSON")
			}
			c := ManifestCommitment{SHA256: Digest(b), Size: strconv.Itoa(len(b)), BootstrapLocator: "https://storage.example.com/manifest"}
			if _, e = Parse(b, f.Vectors[3].Context, c); e == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}
func TestEncodingLimits(t *testing.T) {
	for _, b := range [][]byte{{0xff}, append([]byte{0xef, 0xbb, 0xbf}, []byte("{}")...), []byte(strings.Repeat(" ", MaxManifestBytes) + "{}")} {
		if _, e := strictjson.Canonical(b, MaxManifestBytes); e == nil {
			t.Fatal("invalid encoding/limit accepted")
		}
	}
	for _, p := range []string{"", "../x", "a//b", "a/b/", "/root", "a%2fb"} {
		if _, e := Key(p, "packs", strings.Repeat("a", 64)); e == nil {
			t.Fatal("prefix", p)
		}
	}
}

// TestSchema2ClosureRules exercises the schema-2 dependency closure directly
// (ADR 0005): backward-only references, no duplicates, acyclic by construction.
func TestSchema2ClosureRules(t *testing.T) {
	d0, d1, d2 := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	entry := func(seq int, digest string, deps ...string) PackEntry {
		if deps == nil {
			deps = []string{}
		}
		return PackEntry{Sequence: seq, SHA256: digest, Size: "128", Format: "git-pack", PackVersion: 2, Thin: false, DependsOn: deps, Locations: []PackLocation{{Provider: "aws-s3", URL: "https://storage.example.com/p/x.pack"}}}
	}
	base := func(version int, packs ...PackEntry) PackManifest {
		return PackManifest{Schema: "igit.pack-manifest", SchemaVersion: version,
			Context: Context{ChainID: "1439", SuiteDirectory: "0x" + strings.Repeat("1", 40), RepoID: "0x" + strings.Repeat("2", 64), RefName: "refs/heads/main", Commit: Commit{Algorithm: "sha1", OID: strings.Repeat("3", 40)}},
			Packs: packs}
	}
	if m := base(2, entry(0, d0), entry(1, d1, d0), entry(2, d2, d0, d1)); m.Validate() != nil {
		t.Fatal("valid linear schema-2 chain rejected")
	}
	if m := base(2, entry(0, d0), entry(1, d1, d0), entry(2, d2, d1)); m.Validate() != nil {
		t.Fatal("subset dependency closure rejected")
	}
	rejects := []struct {
		name string
		m    PackManifest
	}{
		{"self-dependency", base(2, entry(0, d0, d0))},
		{"unknown-dependency", base(2, entry(0, d0, d2))},
		{"forward-dependency", base(2, entry(0, d0, d1), entry(1, d1))},
		{"duplicate-dependency", base(2, entry(0, d0), entry(1, d1, d0, d0))},
		{"base-pack-with-dependency", base(2, entry(0, d0, d1), entry(1, d1, d0))},
		{"schema1-with-dependency", base(1, entry(0, d0), entry(1, d1, d0))},
		{"schema2-thin-pack", base(2, PackEntry{Sequence: 0, SHA256: d0, Size: "128", Format: "git-pack", PackVersion: 2, Thin: true, DependsOn: []string{}, Locations: []PackLocation{{Provider: "aws-s3", URL: "https://storage.example.com/p/x.pack"}}})},
		{"unknown-schema-version", base(3, entry(0, d0))},
	}
	for _, r := range rejects {
		if r.m.Validate() == nil {
			t.Fatal("accepted invalid chain:", r.name)
		}
	}
}

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

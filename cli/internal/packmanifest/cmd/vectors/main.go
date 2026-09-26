// vectors emits independently computed Go JCS bytes and digests for TS parity.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		os.Exit(1)
	}
	var f struct {
		Vectors []struct {
			Name       string
			Input      string
			Kind       string
			Context    packmanifest.Context
			Commitment packmanifest.ManifestCommitment
		}
	}
	if json.Unmarshal(b, &f) != nil {
		os.Exit(1)
	}
	out := []map[string]string{}
	for _, v := range f.Vectors {
		c, e := strictjson.Canonical([]byte(v.Input), packmanifest.MaxManifestBytes)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		if v.Kind == "manifest" {
			if _, e = packmanifest.Parse(c, v.Context, v.Commitment); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
		}
		out = append(out, map[string]string{"name": v.Name, "canonical": string(c), "sha256": packmanifest.Digest(c)})
	}
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(1)
	}
}

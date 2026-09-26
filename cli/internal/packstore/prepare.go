package packstore

import (
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/safehttp"
	"io"
	"strconv"
	"strings"
)

// Prepared contains only local observations and the proposed commitment. No ref
// is changed. On failure the returned receipts identify reusable/orphan objects.
type Prepared struct {
	Receipts   []Receipt
	Commitment *packmanifest.ManifestCommitment
}

func Prepare(ctx context.Context, w Writer, m packmanifest.PackManifest, sources []Source, publicBase, prefix, dir string) (Prepared, error) {
	result := Prepared{}
	if m.Validate() != nil || len(sources) != len(m.Packs) {
		return result, Fail(Invalid, "prepare-manifest")
	}
	if _, err := safehttp.ValidateURL(publicBase); err != nil || !packmanifest.ValidPrefix(prefix) {
		return result, Fail(Invalid, "manifest-locator")
	}
	b, err := packmanifest.Encode(m)
	if err != nil {
		return result, err
	}
	manifestKey, _ := packmanifest.Key(prefix, "manifests", packmanifest.Digest(b))
	locator := strings.TrimRight(publicBase, "/") + "/" + manifestKey
	if _, err := safehttp.ValidateURL(locator); err != nil {
		return result, Fail(Invalid, "manifest-locator")
	}
	for _, p := range m.Packs {
		n, _ := packmanifest.Size(p.Size, packmanifest.MaxPackBytes)
		if n > w.Capabilities().MaxObject {
			return result, Fail(Limit, "provider-object")
		}
	}
	// Validate every source before the first remote side effect.
	for i, p := range m.Packs {
		n, _ := packmanifest.Size(p.Size, packmanifest.MaxPackBytes)
		f, e := CheckSource(ctx, sources[i], Object{Kind: "packs", SHA256: p.SHA256, Size: n})
		if e != nil {
			return result, e
		}
		f.Close()
	}
	for i, p := range m.Packs {
		n, _ := packmanifest.Size(p.Size, packmanifest.MaxPackBytes)
		r, e := w.PutIfAbsent(ctx, Object{Kind: "packs", SHA256: p.SHA256, Size: n}, sources[i])
		result.Receipts = append(result.Receipts, r)
		if e != nil {
			return result, e
		}
		if !r.Verified {
			return result, Fail(Uncertain, "unverified-receipt")
		}
	}
	f, err := Spool(ctx, dir, io.NopCloser(strings.NewReader(string(b))), packmanifest.MaxManifestBytes)
	if err != nil {
		return result, err
	}
	defer f.Close()
	src := f.Source()
	key, err := packmanifest.Key(prefix, "manifests", src.SHA256)
	if err != nil {
		return result, err
	}
	r, err := w.PutIfAbsent(ctx, Object{Kind: "manifests", SHA256: src.SHA256, Size: src.Size}, src)
	result.Receipts = append(result.Receipts, r)
	if err != nil {
		return result, err
	}
	if !r.Verified || r.Key != key {
		return result, Fail(Uncertain, "manifest-receipt")
	}
	result.Commitment = &packmanifest.ManifestCommitment{SHA256: src.SHA256, Size: strconv.FormatInt(src.Size, 10), BootstrapLocator: locator}
	return result, nil
}

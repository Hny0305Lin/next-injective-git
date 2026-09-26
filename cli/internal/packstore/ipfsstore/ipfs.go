// Package ipfsstore adapts Kubo without conflating raw digest validation with
// legacy CID addressing or with durable pin/replication confirmation.
package ipfsstore

import (
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"io"
	"strings"
)

type Client interface {
	AddTemporaryContext(context.Context, string, io.Reader) (string, error)
	GetFromGatewaysContext(context.Context, string) (io.ReadCloser, error)
}
type Store struct{ Client Client }

func (s Store) Capabilities() packstore.Capabilities {
	return packstore.Capabilities{Provider: "ipfs", MaxObject: packmanifest.MaxPackBytes, MaxSinglePut: packmanifest.MaxPackBytes, RawReadback: true}
}
func (s Store) Open(ctx context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	if l.Provider != "ipfs" || l.Validate() != nil {
		return nil, packstore.Fail(packstore.Invalid, "ipfs-location")
	}
	return s.Client.GetFromGatewaysContext(ctx, strings.TrimPrefix(l.URL, "ipfs://"))
}

// OpenLegacy is explicitly unverified against an on-chain raw digest: v3 has
// only CID URIs. Callers preserve the original URI order (different packs).
func (s Store) OpenLegacy(ctx context.Context, uri string) (io.ReadCloser, error) {
	l := packmanifest.PackLocation{Provider: "ipfs", URL: uri}
	if l.Validate() != nil {
		return nil, packstore.Fail(packstore.Invalid, "legacy-uri")
	}
	return s.Client.GetFromGatewaysContext(ctx, strings.TrimPrefix(uri, "ipfs://"))
}
func (s Store) PutIfAbsent(ctx context.Context, o packstore.Object, source packstore.Source) (packstore.Receipt, error) {
	r := packstore.Receipt{Provider: "ipfs", Object: o}
	f, err := packstore.CheckSource(ctx, source, o)
	if err != nil {
		return r, err
	}
	defer f.Close()
	cid, err := s.Client.AddTemporaryContext(ctx, o.SHA256+".pack", f)
	if err != nil {
		return r, packstore.Fail(packstore.Unavailable, "ipfs-put")
	}
	r.Key = "ipfs://" + cid
	if err = s.VerifyStoredBytes(ctx, r); err != nil {
		return r, err
	}
	r.Verified = true
	return r, nil
}
func (s Store) VerifyStoredBytes(ctx context.Context, r packstore.Receipt) error {
	if r.Provider != "ipfs" {
		return packstore.Fail(packstore.Invalid, "receipt")
	}
	b, err := s.Open(ctx, packmanifest.PackLocation{Provider: "ipfs", URL: r.Key}, r.Object)
	if err != nil {
		return err
	}
	defer b.Close()
	return packstore.Check(ctx, b, r.Object)
}

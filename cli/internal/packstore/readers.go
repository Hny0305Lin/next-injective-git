package packstore

import (
	"context"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"io"
)

// Readers selects an explicit independent mapping or an anonymous provider.
// Missing authenticated mappings never fall back to the writer's credentials.
type Readers struct {
	Public        map[string]Reader
	Authenticated map[string]Reader
}

func (r Readers) Open(ctx context.Context, l packmanifest.PackLocation, o Object) (io.ReadCloser, error) {
	if l.Validate() != nil {
		return nil, Fail(Invalid, "location")
	}
	var reader Reader
	if l.Reader != "" {
		reader = r.Authenticated[l.Reader]
	} else {
		reader = r.Public[l.Provider]
	}
	if reader == nil {
		return nil, Fail(Auth, "independent-reader-not-configured")
	}
	return reader.Open(ctx, l, o)
}

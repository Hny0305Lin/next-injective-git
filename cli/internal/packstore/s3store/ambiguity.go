package s3store

import (
	"context"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
)

// After an ambiguous write, only a full GET can establish success. A damaged
// object remains an integrity failure; a failed probe never invents certainty.
func (s *Store) resolveWrite(ctx context.Context, r packstore.Receipt, op string, cause error) (packstore.Receipt, error) {
	if ctx.Err() != nil {
		return r, classified(ctx, op, cause)
	}
	err := s.VerifyStoredBytes(ctx, r)
	if err == nil {
		r.Verified = true
		r.Reused = true
		return r, nil
	}
	var e *packstore.Error
	if errors.As(err, &e) && (e.Code == packstore.Integrity || e.Code == packstore.Canceled) {
		return r, err
	}
	code := packstore.Uncertain
	if status(cause) == 409 {
		code = packstore.Conflict
	} else if status(cause) == 429 {
		code = packstore.Unavailable
	}
	return r, &packstore.Error{Code: code, Operation: op, Status: status(cause)}
}

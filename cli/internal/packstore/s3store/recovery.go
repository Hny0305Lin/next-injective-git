package s3store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/fileprotection"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
	"io"
	"os"
)

func (s *Store) save(r packstore.Receipt) error {
	if s.checkpoint != nil {
		if err := s.checkpoint(r); err != nil {
			return packstore.Fail(packstore.Uncertain, "checkpoint")
		}
	}
	return nil
}

// PutRecoverable persists the key before IO, each multipart session/part and the
// final observation. A failed checkpoint stops further requests. No abort follows.
func (s *Store) PutRecoverable(ctx context.Context, o packstore.Object, src packstore.Source, checkpoint func(packstore.Receipt) error) (packstore.Receipt, error) {
	key, err := s.key(o)
	if err != nil {
		return packstore.Receipt{}, err
	}
	r := packstore.Receipt{Provider: s.profile.Provider, Key: key, Object: o}
	copy := *s
	copy.checkpoint = checkpoint
	if err = copy.save(r); err != nil {
		return r, err
	}
	r, err = copy.PutIfAbsent(ctx, o, src)
	if saveErr := copy.save(r); saveErr != nil {
		return r, saveErr
	}
	return r, err
}

// Recover first verifies the final object. If absent, restart all parts in a new
// session, preserving abandoned IDs. It never trusts locally cached ETags, resumes
// a transaction, creates a new nonce, aborts a session or deletes an object.
func (s *Store) Recover(ctx context.Context, prior packstore.Receipt, src packstore.Source, checkpoint func(packstore.Receipt) error) (packstore.Receipt, error) {
	if err := s.validReceipt(prior); err != nil {
		return prior, err
	}
	prior.Verified = false
	err := s.VerifyStoredBytes(ctx, prior)
	if err == nil {
		prior.Verified = true
		prior.Reused = true
		if checkpoint != nil && checkpoint(prior) != nil {
			return prior, packstore.Fail(packstore.Uncertain, "checkpoint")
		}
		return prior, nil
	}
	var e *packstore.Error
	if !errors.As(err, &e) || e.Code != packstore.Missing {
		return prior, err
	}
	retained := append([]string{}, prior.RetainedUploadIDs...)
	if prior.UploadID != "" {
		retained = append(retained, prior.UploadID)
	}
	if len(retained) > 8 {
		return prior, packstore.Fail(packstore.Limit, "retained-sessions")
	}
	merge := func(r packstore.Receipt) packstore.Receipt {
		r.RetainedUploadIDs = append(append([]string{}, retained...), r.RetainedUploadIDs...)
		return r
	}
	r, err := s.PutRecoverable(ctx, prior.Object, src, func(r packstore.Receipt) error {
		if checkpoint != nil {
			return checkpoint(merge(r))
		}
		return nil
	})
	return merge(r), err
}
func (s *Store) validReceipt(r packstore.Receipt) error {
	key, err := s.key(r.Object)
	if err != nil {
		return err
	}
	if r.Provider != s.profile.Provider || r.Key != key || len(r.RetainedUploadIDs) > 10 || len(r.CompletedParts) > 64 {
		return packstore.Fail(packstore.Invalid, "receipt")
	}
	ids := append([]string{}, r.RetainedUploadIDs...)
	if r.UploadID != "" {
		ids = append(ids, r.UploadID)
	}
	for _, id := range ids {
		if len(id) > 1024 || !uploadID.MatchString(id) {
			return packstore.Fail(packstore.Invalid, "receipt")
		}
	}
	for i, p := range r.CompletedParts {
		if p != int32(i+1) {
			return packstore.Fail(packstore.Invalid, "receipt-parts")
		}
	}
	return nil
}

// SaveReceipt uses the existing OS file-protection policy. The path is explicit;
// it is never derived from a provider response, digest, endpoint or bucket name.
func (s *Store) SaveReceipt(path string, r packstore.Receipt) error {
	if err := s.validReceipt(r); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return packstore.Fail(packstore.Invalid, "receipt")
	}
	if err = fileprotection.WriteFile(path, b); err != nil {
		return packstore.Fail(packstore.Unavailable, "protected-receipt")
	}
	return nil
}
func (s *Store) LoadReceipt(path string) (packstore.Receipt, error) {
	var r packstore.Receipt
	if fileprotection.ValidateFile(path) != nil {
		return r, packstore.Fail(packstore.Invalid, "protected-receipt")
	}
	f, err := os.Open(path)
	if err != nil {
		return r, packstore.Fail(packstore.Missing, "receipt")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || strictjson.Decode(b, 65536, &r) != nil {
		return r, packstore.Fail(packstore.Invalid, "receipt")
	}
	if err = s.validReceipt(r); err != nil {
		return packstore.Receipt{}, err
	}
	return r, nil
}

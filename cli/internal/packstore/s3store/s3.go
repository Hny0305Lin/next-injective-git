// Package s3store implements separate AWS and R2 capability profiles using the
// pinned AWS Go SDK. No transfer manager (implicit AbortMultipartUpload), shared
// credential discovery, arbitrary production endpoints or deletion API is used.
package s3store

import (
	"context"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/safehttp"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const SinglePutLimit int64 = 16 << 20
const partSize int64 = 8 << 20
const maxAttempts = 3

type Store struct {
	profile    storageconfig.Profile
	client     *s3.Client
	public     *http.Client
	pause      func(context.Context, int) error
	checkpoint func(packstore.Receipt) error
	writable   bool
}

func (p *Store) Capabilities() packstore.Capabilities {
	max := SinglePutLimit
	multi := p.profile.Provider == "aws-s3"
	if multi {
		max = packmanifest.MaxPackBytes
	}
	return packstore.Capabilities{Provider: p.profile.Provider, MaxSinglePut: SinglePutLimit, MaxObject: max, ConditionalMultipart: multi, RawReadback: true}
}

// NewWriter and NewReader never consult default AWS config/profile sources.
func NewWriter(p storageconfig.Profile) (*Store, error) {
	if p.CredentialRef == nil {
		return nil, storageconfig.ErrConfig
	}
	return production(p, true)
}
func NewReader(p storageconfig.Profile) (*Store, error) { return production(p, false) }
func production(p storageconfig.Profile, writer bool) (*Store, error) {
	endpoint, err := p.Endpoint()
	if err != nil {
		return nil, err
	}
	if !writer && p.CredentialRef == nil && p.PublicReadBase == "" {
		return nil, storageconfig.ErrConfig
	}
	var creds aws.CredentialsProvider
	if p.CredentialRef != nil {
		creds = p.CredentialRef.Resolve(os.LookupEnv)
	}
	store, err := newStore(p, creds, safehttp.NewClient(strings.TrimPrefix(endpoint, "https://")), safehttp.NewClient(""))
	if store != nil {
		store.writable = writer
	}
	return store, err
}

// Unexported injection is available to in-package tests only. Profile endpoint
// validation is identical; the fake transport, not production config, maps hosts.
func newStore(p storageconfig.Profile, creds aws.CredentialsProvider, cloud *http.Client, public *http.Client) (*Store, error) {
	endpoint, err := p.Endpoint()
	if err != nil {
		return nil, err
	}
	st := &Store{profile: p, public: public, writable: creds != nil, pause: func(ctx context.Context, n int) error {
		timer := time.NewTimer(time.Duration(1<<n) * 100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return packstore.Fail(packstore.Canceled, "retry")
		case <-timer.C:
			return nil
		}
	}}
	if creds != nil {
		st.client = s3.New(s3.Options{Region: p.Region, BaseEndpoint: aws.String(endpoint), UsePathStyle: true, Credentials: creds, HTTPClient: cloud, Retryer: aws.NopRetryer{}, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
	}
	return st, nil
}
func status(err error) int {
	var r *smithyhttp.ResponseError
	if errors.As(err, &r) {
		return r.HTTPStatusCode()
	}
	return 0
}
func classified(ctx context.Context, op string, err error) error {
	code := packstore.Unavailable
	n := status(err)
	switch {
	case ctx.Err() != nil:
		code = packstore.Canceled
	case errors.Is(err, storageconfig.ErrConfig):
		code = packstore.Auth
	case n == 401 || n == 403:
		code = packstore.Auth
	case n == 404:
		code = packstore.Missing
	case n == 409 || n == 412:
		code = packstore.Conflict
	case n == 0:
		code = packstore.Uncertain
	}
	return &packstore.Error{Code: code, Operation: op, Status: n}
}
func retryable(err error, conflict bool) bool {
	if errors.Is(err, storageconfig.ErrConfig) {
		return false
	}
	n := status(err)
	return n == 0 || n == 429 || n >= 500 && n <= 599 || conflict && n == 409
}
func (s *Store) retry(ctx context.Context, conflict bool, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil || !retryable(err, conflict) || i == maxAttempts-1 {
			return err
		}
		if e := s.pause(ctx, i); e != nil {
			return e
		}
	}
	return err
}
func (s *Store) key(o packstore.Object) (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}
	return packmanifest.Key(s.profile.Prefix, o.Kind, o.SHA256)
}
func (s *Store) PutIfAbsent(ctx context.Context, o packstore.Object, source packstore.Source) (packstore.Receipt, error) {
	receipt := packstore.Receipt{Provider: s.profile.Provider, Object: o}
	key, err := s.key(o)
	if err != nil {
		return receipt, err
	}
	receipt.Key = key
	if o.Size > s.Capabilities().MaxObject {
		return receipt, packstore.Fail(packstore.Limit, "single-put-only")
	}
	if s.client == nil || !s.writable {
		return receipt, packstore.Fail(packstore.Auth, "writer")
	}
	f, err := packstore.CheckSource(ctx, source, o)
	if err != nil {
		return receipt, err
	}
	defer f.Close()
	if o.Size > SinglePutLimit {
		return s.multipart(ctx, o, f, receipt)
	}
	err = s.retry(ctx, true, func() error {
		_, e := f.Seek(0, io.SeekStart)
		if e != nil {
			return e
		}
		_, e = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.profile.Bucket), Key: aws.String(key), Body: f, ContentLength: aws.Int64(o.Size), IfNoneMatch: aws.String("*"), ContentType: aws.String("application/octet-stream")})
		return e
	})
	if err != nil && status(err) != 412 {
		// An ambiguous write is resolved only by observing exact stored bytes. The
		// receipt retains the content key if this read is also unavailable.
		if retryable(err, true) {
			return s.resolveWrite(ctx, receipt, "put", err)
		}
		return receipt, classified(ctx, "put", err)
	}
	receipt.Reused = status(err) == 412
	if err = s.VerifyStoredBytes(ctx, receipt); err != nil {
		return receipt, err
	}
	receipt.Verified = true
	return receipt, nil
}

var uploadID = regexp.MustCompile("^[A-Za-z0-9+/=_-]+$")

func (s *Store) multipart(ctx context.Context, o packstore.Object, f *os.File, receipt packstore.Receipt) (packstore.Receipt, error) {
	// AWS conditional completion 409 requires a new upload session and all parts.
	// At most two sessions; abandoned IDs remain in the receipt for explicit recovery.
	for session := 0; session < 2; session++ {
		created, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(s.profile.Bucket), Key: aws.String(receipt.Key), ContentType: aws.String("application/octet-stream")})
		if err != nil {
			return receipt, classified(ctx, "multipart-create", err)
		}
		id := aws.ToString(created.UploadId)
		if len(id) > 1024 || !uploadID.MatchString(id) {
			return receipt, packstore.Fail(packstore.Uncertain, "multipart-id")
		}
		receipt.UploadID = id
		receipt.CompletedParts = nil
		if err = s.save(receipt); err != nil {
			return receipt, err
		}
		parts := []types.CompletedPart{}
		for offset := int64(0); offset < o.Size; offset += partSize {
			length := partSize
			if o.Size-offset < length {
				length = o.Size - offset
			}
			part := int32(len(parts) + 1)
			var result *s3.UploadPartOutput
			err = s.retry(ctx, false, func() error {
				var e error
				result, e = s.client.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(s.profile.Bucket), Key: aws.String(receipt.Key), UploadId: aws.String(id), PartNumber: aws.Int32(part), ContentLength: aws.Int64(length), Body: io.NewSectionReader(f, offset, length)})
				return e
			})
			if err != nil {
				return receipt, classified(ctx, "multipart-part", err)
			}
			if result.ETag == nil || len(*result.ETag) > 256 {
				return receipt, packstore.Fail(packstore.Uncertain, "multipart-etag")
			}
			parts = append(parts, types.CompletedPart{ETag: result.ETag, PartNumber: aws.Int32(part)})
			receipt.CompletedParts = append(receipt.CompletedParts, part)
			if err = s.save(receipt); err != nil {
				return receipt, err
			}
		}
		// Never blindly retry ambiguous Complete: GET can establish success. A 409
		// explicitly rebuilds; 412 verifies the winner and retains this session.
		_, err = s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(s.profile.Bucket), Key: aws.String(receipt.Key), UploadId: aws.String(id), IfNoneMatch: aws.String("*"), MultipartUpload: &types.CompletedMultipartUpload{Parts: parts}})
		if status(err) == 409 {
			receipt.RetainedUploadIDs = append(receipt.RetainedUploadIDs, id)
			receipt.UploadID = ""
			receipt.CompletedParts = nil
			if saveErr := s.save(receipt); saveErr != nil {
				return receipt, saveErr
			}
			if session == 0 {
				continue
			}
			return receipt, classified(ctx, "multipart-complete", err)
		}
		if err != nil && status(err) != 412 {
			if retryable(err, false) {
				return s.resolveWrite(ctx, receipt, "multipart-complete", err)
			}
			return receipt, classified(ctx, "multipart-complete", err)
		}
		receipt.Reused = status(err) == 412
		if err == nil {
			receipt.UploadID = ""
		}
		if err = s.VerifyStoredBytes(ctx, receipt); err != nil {
			return receipt, err
		}
		receipt.Verified = true
		return receipt, nil
	}
	return receipt, packstore.Fail(packstore.Conflict, "multipart-complete")
}
func (s *Store) openCloud(ctx context.Context, o packstore.Object) (io.ReadCloser, error) {
	key, err := s.key(o)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, packstore.Fail(packstore.Auth, "reader")
	}
	var out *s3.GetObjectOutput
	err = s.retry(ctx, false, func() error {
		var e error
		out, e = s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.profile.Bucket), Key: aws.String(key)})
		return e
	})
	if err != nil {
		return nil, classified(ctx, "get", err)
	}
	if out.ContentEncoding != nil && *out.ContentEncoding != "identity" && *out.ContentEncoding != "" {
		out.Body.Close()
		return nil, packstore.Fail(packstore.Integrity, "content-encoding")
	}
	return out.Body, nil
}
func (s *Store) VerifyStoredBytes(ctx context.Context, r packstore.Receipt) error {
	key, err := s.key(r.Object)
	if err != nil {
		return err
	}
	if r.Provider != s.profile.Provider || r.Key != key {
		return packstore.Fail(packstore.Invalid, "receipt")
	}
	body, err := s.openCloud(ctx, r.Object)
	if err != nil {
		return err
	}
	defer body.Close()
	return packstore.Check(ctx, body, r.Object)
}
func (s *Store) Open(ctx context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	if l.Validate() != nil || l.Provider != s.profile.Provider {
		return nil, packstore.Fail(packstore.Invalid, "location")
	}
	if l.URL != "" {
		return s.openPublic(ctx, l.URL, "")
	}
	if s.client != nil {
		return s.openCloud(ctx, o)
	}
	key, err := s.key(o)
	if err != nil {
		return nil, err
	}
	if s.profile.PublicReadBase == "" {
		return nil, packstore.Fail(packstore.Auth, "independent-reader")
	}
	return s.openPublic(ctx, strings.TrimRight(s.profile.PublicReadBase, "/")+"/"+key, "")
}
func (s *Store) publicResponse(ctx context.Context, url, origin string) (*http.Response, error) {
	if _, err := safehttp.ValidateURL(url); err != nil {
		return nil, packstore.Fail(packstore.Invalid, "public-url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, packstore.Fail(packstore.Invalid, "public-get")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := s.public.Do(req)
	if err != nil {
		return nil, classified(ctx, "public-get", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		code := packstore.Unavailable
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			code = packstore.Auth
		} else if resp.StatusCode == 404 {
			code = packstore.Missing
		}
		return nil, &packstore.Error{Code: code, Operation: "public-get", Status: resp.StatusCode}
	}
	if e := resp.Header.Get("Content-Encoding"); e != "" && e != "identity" {
		resp.Body.Close()
		return nil, packstore.Fail(packstore.Integrity, "content-encoding")
	}
	return resp, nil
}
func (s *Store) openPublic(ctx context.Context, url, origin string) (io.ReadCloser, error) {
	r, e := s.publicResponse(ctx, url, origin)
	if e != nil {
		return nil, e
	}
	return r.Body, nil
}

type PublicDiagnostic struct {
	Verified bool
	CORS     bool
}

// DiagnosePublic performs a read-only full GET. GET success and CORS permission
// are separate facts; this does not configure bucket policies or claim browser E2E.
func (s *Store) DiagnosePublic(ctx context.Context, url, origin string, o packstore.Object) (PublicDiagnostic, error) {
	var d PublicDiagnostic
	u, err := safehttp.ValidateURL(origin)
	if err != nil || u.Path != "" {
		return d, packstore.Fail(packstore.Invalid, "origin")
	}
	r, err := s.publicResponse(ctx, url, origin)
	if err != nil {
		return d, err
	}
	defer r.Body.Close()
	if err = packstore.Check(ctx, r.Body, o); err != nil {
		return d, err
	}
	d.Verified = true
	a := r.Header.Get("Access-Control-Allow-Origin")
	d.CORS = a == "*" || a == origin
	return d, nil
}

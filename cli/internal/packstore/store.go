// Package packstore separates verified content from untrusted transports. No
// interface offers deletion, abort, GC or chain writes.
package packstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"io"
	"os"
)

type Code string

const (
	Integrity   Code = "integrity"
	Limit       Code = "limit"
	Conflict    Code = "conflict"
	Auth        Code = "authentication"
	Missing     Code = "missing"
	Unavailable Code = "unavailable"
	Canceled    Code = "canceled"
	Invalid     Code = "invalid"
	Uncertain   Code = "uncertain"
)

// Error intentionally excludes provider response text, signed URLs and secrets.
type Error struct {
	Code      Code
	Operation string
	Status    int
}

func (e *Error) Error() string        { return "packstore " + e.Operation + ": " + string(e.Code) }
func (e *Error) Is(target error) bool { t, ok := target.(*Error); return ok && e.Code == t.Code }
func Fail(code Code, op string) error { return &Error{Code: code, Operation: op} }

type Source struct {
	Path   string
	SHA256 string
	Size   int64
}
type Object struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (o Object) Validate() error {
	max := packmanifest.MaxPackBytes
	if o.Kind == "manifests" {
		max = packmanifest.MaxManifestBytes
	} else if o.Kind != "packs" {
		return Fail(Invalid, "object")
	}
	if !packmanifest.ValidDigest(o.SHA256) || o.Size < 1 || o.Size > max {
		return Fail(Limit, "object")
	}
	return nil
}

type Capabilities struct {
	Provider             string
	MaxSinglePut         int64
	MaxObject            int64
	ConditionalMultipart bool
	RawReadback          bool
}
type Receipt struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Object   Object `json:"object"`
	Reused   bool   `json:"reused"`
	Verified bool   `json:"verified"`
	// A recoverable incomplete session, never an automatic abort instruction.
	UploadID          string   `json:"uploadId,omitempty"`
	CompletedParts    []int32  `json:"completedParts,omitempty"`
	RetainedUploadIDs []string `json:"retainedUploadIds,omitempty"`
}
type Reader interface {
	Open(context.Context, packmanifest.PackLocation, Object) (io.ReadCloser, error)
}
type Writer interface {
	Capabilities() Capabilities
	PutIfAbsent(context.Context, Object, Source) (Receipt, error)
	VerifyStoredBytes(context.Context, Receipt) error
}

// File owns only its task-created temporary path. Call Close to remove it.
// Handles are closed before returning, including on Windows.
type File struct{ source Source }

func (f *File) Source() Source               { return f.source }
func (f *File) Close() error                 { return os.Remove(f.source.Path) }
func (f *File) Open() (io.ReadCloser, error) { return os.Open(f.source.Path) }

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// Spool closes a cancellable input promptly; io.ReadCloser.Close must unblock Read.
func Spool(ctx context.Context, dir string, r io.ReadCloser, limit int64) (*File, error) {
	defer r.Close()
	if limit < 1 || limit > packmanifest.MaxPackBytes {
		return nil, Fail(Limit, "spool")
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = r.Close()
		case <-done:
		}
	}()
	defer close(done)
	return Generate(ctx, dir, limit, func(w io.Writer) error {
		_, err := io.CopyBuffer(w, contextReader{ctx, io.LimitReader(r, limit+1)}, make([]byte, 64<<10))
		return err
	})
}

type boundedWriter struct {
	w    io.Writer
	left int64
}

func (w *boundedWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > w.left {
		return 0, Fail(Limit, "write")
	}
	n, e := w.w.Write(b)
	w.left -= int64(n)
	return n, e
}
func Generate(ctx context.Context, dir string, limit int64, produce func(io.Writer) error) (result *File, err error) {
	if limit < 1 || limit > packmanifest.MaxPackBytes {
		return nil, Fail(Limit, "generate")
	}
	f, err := os.CreateTemp(dir, "igit-content-*.tmp")
	if err != nil {
		return nil, Fail(Unavailable, "temporary-file")
	}
	defer func() {
		_ = f.Close()
		if result == nil {
			_ = os.Remove(f.Name())
		}
	}()
	h := sha256.New()
	w := &boundedWriter{io.MultiWriter(f, h), limit}
	err = produce(w)
	if ctx.Err() != nil {
		return nil, Fail(Canceled, "generate")
	}
	if err != nil {
		var typed *Error
		if errors.As(err, &typed) {
			return nil, typed
		}
		return nil, Fail(Unavailable, "generate")
	}
	if err = f.Sync(); err != nil {
		return nil, Fail(Unavailable, "sync")
	}
	if err = f.Close(); err != nil {
		return nil, Fail(Unavailable, "close")
	}
	return &File{Source{f.Name(), hex.EncodeToString(h.Sum(nil)), limit - w.left}}, nil
}
func Check(ctx context.Context, r io.Reader, o Object) error {
	if err := o.Validate(); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.CopyBuffer(h, contextReader{ctx, io.LimitReader(r, o.Size+1)}, make([]byte, 64<<10))
	if ctx.Err() != nil {
		return Fail(Canceled, "verify")
	}
	if err != nil || n != o.Size || hex.EncodeToString(h.Sum(nil)) != o.SHA256 {
		return Fail(Integrity, "verify")
	}
	return nil
}
func CheckSource(ctx context.Context, s Source, o Object) (*os.File, error) {
	if s.SHA256 != o.SHA256 || s.Size != o.Size {
		return nil, Fail(Integrity, "source")
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, Fail(Missing, "source")
	}
	if err = Check(ctx, f, o); err != nil {
		f.Close()
		return nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, Fail(Unavailable, "rewind")
	}
	return f, nil
}

// ReadVerified retries locations of this object only; never treats ordered packs
// as mirrors. Only fully verified, closed temporary files escape this function.
func ReadVerified(ctx context.Context, r Reader, locations []packmanifest.PackLocation, o Object, dir string) (*File, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if len(locations) < 1 || len(locations) > packmanifest.MaxLocations {
		return nil, Fail(Invalid, "locations")
	}
	var last error
	for _, l := range locations {
		if ctx.Err() != nil {
			return nil, Fail(Canceled, "read")
		}
		if err := l.Validate(); err != nil {
			return nil, Fail(Invalid, "location")
		}
		body, err := r.Open(ctx, l, o)
		if err != nil {
			last = err
			continue
		}
		f, err := Spool(ctx, dir, body, o.Size)
		if err != nil {
			last = err
			continue
		}
		if f.source.SHA256 == o.SHA256 && f.source.Size == o.Size {
			return f, nil
		}
		_ = f.Close()
		last = Fail(Integrity, "read")
	}
	if last == nil {
		last = errors.New("no verified location")
	}
	return nil, last
}

# `internal/packstore` — Pluggable pack storage boundary (P3, reserved)

Future interface (see delivery-roadmap P3):

```go
type Writer interface {
    PutIfAbsent(ctx context.Context, src Source) (Receipt, error)
    VerifyDurable(ctx context.Context, r Receipt) error
}
type Reader interface {
    Open(ctx context.Context, ref PackRef) (io.ReadCloser, error)
}
```

IPFS will be the first adapter behind this boundary. Do not implement until ADR approval.

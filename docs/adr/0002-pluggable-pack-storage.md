# ADR 0002: Make Git Pack Storage Pluggable

- Status: Accepted direction; not implemented
- Decision date: 2026-08-14
- Delivery status: Current runtime remains IPFS-only

## Context

iGit's product core is Git-compatible collaboration plus an Injective EVM
control plane. Kubo/IPFS is the currently implemented pack transport and
durability adapter, but it introduces a local daemon and an IPFS-specific
operational stack. It must not become a permanent architectural requirement for
every user or deployment.

The current immutable Suite validates only `ipfs://` pack URIs, and the current
CLI and Web fetch paths only understand IPFS. Amazon S3 and Cloudflare R2 cannot
be enabled safely by changing documentation, a gateway URL, or credentials.

## Decision

1. Treat pack storage as a replaceable data plane behind a stable storage
   adapter interface.
2. Keep IPFS/Kubo as the supported adapter for the current Suite.
3. Add Amazon S3 and Cloudflare R2 as planned object-storage adapters. A user
   who selects an accepted object-storage profile must not need Kubo.
4. Design a successor on-chain pack reference that can identify an allowed
   storage scheme and bind content integrity independently of a mutable URL.
5. Do not put cloud credentials, presigned URLs, session tokens, or private
   bucket details on-chain. Clients obtain credentials through local secure
   configuration or a scoped upload service.
6. Ship storage neutrality only through a reviewed successor Suite and explicit
   migration, because the current Suite is immutable and IPFS-only.

## Required Properties

- Pack bytes are verified against a stable digest before Git consumes them.
- Push confirms durable object persistence before updating the on-chain ref.
- A failed ref transaction retains retryable content or records a recoverable
  object key.
- Clone and fetch can resolve every accepted URI without requiring credentials
  to appear in Git remotes or on-chain state.
- Force push still publishes a self-contained pack regardless of storage
  adapter.
- Windows and Linux have clean-environment E2E coverage for each supported
  adapter.
- S3/R2 bucket versioning, retention, deletion, lifecycle, encryption, and
  least-privilege policies are documented and tested before release.
- Migration preserves historical pack availability and provides a deterministic
  mapping from old `ipfs://` entries to any new reference format.

## Consequences

- Kubo remains required for Push only while using the current IPFS profile.
- Object-storage support is a protocol and client project, not an operations-only
  configuration task.
- The storage adapter interface reduces platform-specific setup and allows
  deployments to choose IPFS, S3, or R2 according to durability, access, and
  cost requirements.
- The successor design must retain content-addressed integrity even where the
  underlying object store uses mutable keys.

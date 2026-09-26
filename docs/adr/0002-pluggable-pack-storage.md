# ADR 0002: Make Git Pack Storage Pluggable

- S01–S03 local library/mock status: PASS ([implementation scope](../storage-byos.md)); real integration: NOT PROVEN
- Decision date: 2026-08-14
- First-release scope confirmed: 2026-09-13, [ADR 0004](0004-mainnet-storage-neutral-successor-and-byos-scope.md)
- Ordinary Git runtime remains IPFS-only; new storage libraries are local-tested and not deployed

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
3. Implement user-owned Amazon S3 and Cloudflare R2 buckets for the first
   mainnet release. Those profiles must not need Kubo, the IPFS network or
   iGit-operated IPFS services. Self-hosted object stores (including MinIO),
   other clouds and arbitrary S3-compatible endpoints are outside this scope.
4. Design a successor on-chain pack reference that can identify an allowed
   storage scheme and bind content integrity independently of a mutable URL.
5. Keep credentials, presigned URLs, session tokens and private access details
   out of chain state, manifests, remotes, logs and ordinary config. Users
   obtain writer and independent reader credentials through local secure
   providers; a managed broker is not a first-release dependency. Public read
   locators are intentionally public, non-secret metadata.
6. Target a reviewed storage-neutral successor Suite directly for mainnet.
   Keep explicit v3 compatibility; migrate history only if separately scoped.
   The current Suite is immutable and cannot receive the new payload format.
7. Use canonical JSON manifests, separate ordered packs from their replicas,
   and bind raw-pack digest and size independently of storage location. The
   schema and canonical bytes require Go/TypeScript cross-implementation tests.
8. First-release repositories are public. Bucket owners pay their cloud costs;
   dual replicas and private repositories/encryption are later work, not
   prerequisites for AWS/R2 BYOS delivery.

## Required Properties

- Pack bytes are verified against a stable digest before Git consumes them.
- Push verifies stored bytes before updating the on-chain ref. This is a
  client policy and an observation at upload time, not an EVM availability proof.
- A failed ref transaction retains retryable content or records a recoverable
  object key.
- Clone and fetch can resolve every accepted URI without requiring credentials
  to appear in Git remotes or on-chain state.
- New successor writes initially use self-contained, non-thin packs, including
  new branches/tags and force pushes. Legacy incremental behavior is isolated;
  it must not silently become a dependency on another ref.
- Windows and Linux have clean-environment E2E coverage for each supported
  adapter.
- Each provider has its own tested capability profile and least-privilege/read
  configuration. Versioning, retention, lifecycle and encryption capabilities
  are not assumed portable; no automatic deletion/GC is enabled by this work.
- If migration is approved, it preserves historical pack availability and provides a deterministic
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
- Editing content produces new pack/manifest keys and a ref update. An
  administrator may delete or corrupt cloud objects, but clients must reject
  those bytes rather than silently alter the chain-committed history.
- The detailed first-release contract is in [ADR 0004](0004-mainnet-storage-neutral-successor-and-byos-scope.md)
  and the [BYOS implementation specification](../storage-byos.md). These are
  design requirements; the BYOS specification now separately records the implemented local adapter tests and remaining real integration work.

# ADR 0004: Mainnet Storage-Neutral Successor And AWS S3 / R2 BYOS

- Product decision confirmed by the user: 2026-09-13 (Asia/Shanghai).
- Implementation and real provider acceptance: **NOT PROVEN**.
- Extends [ADR 0002](0002-pluggable-pack-storage.md); supersedes the older
  managed-first and conditional-mainnet-storage proposals in the roadmap.
- Does not change [ADR 0003](0003-fresh-evm-suite-and-v1-archive-preview.md)
  into a mandatory CosmWasm V1 migration or authorize any deployment.

## Context

Users must be able to use their own cloud buckets and manage their data
without depending on the IPFS network or the iGit-operated IPFS services.
The current immutable Suite version 3 accepts only `ipfs://` pack URIs.
Adding cloud credentials or changing a gateway cannot add object storage to
that deployed protocol. The implementation baseline remains the
[2026-09-13 audit](../reconciliation-baseline-2026-09-12.md); this ADR records
product decisions, not new test or deployment evidence.

## Confirmed First-Release Scope

| Decision | Required scope | Explicitly outside this release |
|---|---|---|
| Mainnet protocol | Target the storage-neutral successor directly; AWS S3 / R2 BYOS is a launch requirement | Launching an IPFS-only v3 mainnet first and immediately migrating |
| Providers | Amazon S3 and Cloudflare R2, with separate capability profiles | MinIO, OSS, COS, other clouds, arbitrary S3-compatible endpoints, self-hosted object-storage services |
| Ownership | The user supplies and administers an AWS/R2 bucket (BYOS) | A new iGit-managed bucket or upload/GET broker as a prerequisite |
| Repository visibility | Public repositories | Private-repository guarantees, end-to-end encryption and key distribution |
| Cost | The bucket owner pays applicable storage, request and bandwidth costs, including verification reads | A promise of free storage or platform-subsidized replication |
| Replication | One verified location suffices for the first release; the model can describe replicas | Mandatory IPFS copies, mandatory AWS+R2 dual writes, mandatory dual replicas |
| Authenticated reads | User-owned, independent reader configuration; no sharing of the writer credential | A managed short-lived GET broker or private Web credential vault |
| Manifest encoding | Canonical JSON; use RFC 8785 JCS as the implementation baseline | CBOR or ordinary JSON serialization presented as canonical |

“用户自有 AWS/R2 存储桶”属于首期目标；“用户自建对象存储服务”不属于首期目标。
The provider restriction concerns the storage service/API, not a ban on a
user-configured public read domain backed by the accepted cloud bucket.
Tests may inject a local HTTP fake; it must not enable self-hosted providers
in production configuration.

## Reading Public Repositories

The standard public path requires stable, anonymous HTTPS access to the
manifest and packs, including browser CORS. Write access remains private.
If a bucket requires authenticated reads, the user configures a separate
least-privilege CLI reader identity through their own secure credential
mechanism. Missing reader configuration must produce an actionable access
error, not trigger an iGit broker or prompt for a writer secret.

A private bucket does not make the Git repository private: on-chain metadata
is public, existing readers may retain copies, and there is no first-release
encryption/key-distribution protocol. An authenticated CLI-only path must not
be advertised as anonymous public-Web acceptance. The Web receives no cloud
secret and reports a read-configuration limitation instead of bypassing it.

## Integrity, Mutability And Authority

- Users may edit their Git working tree and publish new commits, choose their
  bucket policies, rotate credentials and manage their cloud resources.
- Published objects use digest-derived keys. Changing content requires new
  pack bytes, a new manifest and an authorized on-chain ref update; clients
  never overwrite different bytes at an existing digest key. Repacking or
  changing locations may change the manifest while preserving the commit.
- Out-of-band overwrite or deletion is possible for a bucket administrator.
  Clients detect tampering or absence; neither hashing nor an on-chain ref
  guarantees long-term availability or recovery.
- The chain commits to the manifest; the manifest binds exact raw-pack
  SHA-256, size, order/dependencies and locations. A location or S3 ETag is not
  the content authority. Packs and manifest are verified before publishing
  the ref; downloaded bytes are verified before Git consumes them.
- The contract cannot fetch cloud/IPFS bytes. The data-before-ref rule is a
  client publication policy, not an on-chain proof of persistence. BYOS does
  not introduce a trusted iGit storage-receipt signer.
- Credentials, session tokens and presigned URLs never enter on-chain state,
  manifests, Git remotes, logs, browser storage or ordinary iGit JSON config.
  Non-secret credential *references* are allowed locally.

## Protocol And Compatibility Boundaries

1. Preserve explicit legacy v3 IPFS behavior; never disguise HTTPS as
   `ipfs://` or send successor payloads to the existing Directory.
2. Keep the immutable Directory trust root and the existing module
   responsibilities unless a separately justified design changes them.
   Successor version, ABI, events, Go/Web decoders, indexer and evidence gates
   must be reviewed together. This ADR does not assign a deployed version.
3. Prefer a chain-held manifest digest/size plus a bounded stable bootstrap
   locator. A cold client must resolve the manifest from current contract
   state, without relying on an event indexer or a circular profile lookup.
4. Separate an ordered pack set from alternate locations of the *same* pack.
   Start new successor writes with self-contained, non-thin packs; optimize
   later only with an explicit, independently fetchable dependency closure.
5. Freeze the exact manifest schema, canonical bytes, size limits and ABI
   after Go/TypeScript cross-implementation tests. The encoding decision is
   already made; wire details are not a reason to reopen CBOR selection.
6. Recommend a fresh successor test Suite for initial validation. Import of
   existing v3 testnet history is a separate scoped decision, not required to
   develop the adapters. V1 remains an isolated read-only archive preview.

## Delivery Consequences

Start with local protocol vectors, a verified `packstore` boundary and
provider contract tests. Lack of Forge, Kubo, cloud accounts or transaction
authorization does not prevent those steps. It does restrict the corresponding
real checks; report them as BLOCKED or NOT PROVEN rather than fabricating
evidence. Preserve the R01–R08 baseline findings.

See [BYOS implementation specification](../storage-byos.md),
[architecture](../architecture.md), [roadmap](../delivery-roadmap.md),
[S01–S07 backlog](../backlog.md), and the
[next-window implementation prompt](../prompts/next-storage-implementation.md).
No cloud creation/upload/delete, signing, transaction, public-profile change,
commit or push is authorized by this documentation update.

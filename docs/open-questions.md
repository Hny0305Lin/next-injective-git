# Open Decisions

Updated: 2026-09-13 (Asia/Shanghai). Product decisions below come from the
user; they are not deployment approval. Implementation and real provider
acceptance remain **NOT PROVEN**. See [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)
and the [BYOS specification](storage-byos.md).

## Already Decided — Do Not Reopen For The First Release

- Object storage is a first mainnet requirement: directly target a
  storage-neutral successor, not an IPFS-only v3 mainnet intermediate release.
- Only user-owned Amazon S3 and Cloudflare R2 buckets. No MinIO, self-hosted
  object stores, other clouds or arbitrary S3-compatible write endpoint.
- Public repositories first; private repositories, E2EE and key distribution
  are later work. Private bucket access alone is not a privacy guarantee.
- Users pay their cloud costs. One verified object location suffices; dual
  replicas and managed public storage are not mandatory release conditions.
- Authenticated readers use independent user-owned configuration; no managed
  GET broker or writer-secret sharing. Anonymous Web access requires public
  read configuration/CORS and cannot be inferred from authenticated CLI tests.
- Canonical JSON, using RFC 8785 JCS as the implementation baseline, not CBOR.

## Engineering Decisions To Freeze Through S01–S05

These do not require repeating the product questions. Use the specification's
recommended defaults, implement local fixtures, and record justified deviations
before freezing the wire protocol.

- Manifest schema 1/canonical bytes/context binding and local limits are now tested and frozen in [BYOS section 9](storage-byos.md): SHA-1, pack v2, full non-thin history. Other Git formats and incremental dependency optimization remain separate work.
- Successor version, bounded bootstrap locator/state layout, same-commit
  manifest changes, CAS revision/tombstones, ref deletion/recreation, fork and
  import semantics, event ABI and backwards-compatible client dispatch.
- Provider/credential reference UX and safe endpoint validation; exact cloud
  region/account constraints and public read domain rules. No generic endpoint
  escape hatch to make local mocks easier.
- Local provider limits are tested: 16 MiB single PUT for AWS/R2, AWS-only 8 MiB multipart parts up to 512 MiB, full readback and directed recovery. Real-cloud conditional completion, cost and interruption evidence remain NOT PROVEN.
  Missing real evidence results in explicit feature/size limits, not an
  assumed compatibility claim.
- Resource policy documentation: least-privilege writer/reader identities,
  stable public GET/CORS, secret redaction, safe lifecycle warnings and
  optional retention/versioning. Do not make a feature unavailable on one
  provider an unreviewed common requirement.
- Versioned successor indexer/evidence gate support. Existing scripts marked
  FAIL and v3-only release gates cannot silently authorize successor use.

## Decisions Requiring User Resources Or Separate Scope

- Which disposable AWS/R2 test buckets/prefixes, public domains/origins,
  permissions, cost limits and upload ranges may be used for real canaries?
  Credentials must be configured by the user through a secure local channel,
  never pasted into chat or recovered from existing secret files.
- Whether any **existing v3 testnet history** should be imported into a new
  successor. Default recommendation: validate a fresh successor first. This
  is separate from the already-decided mainnet successor target; V1 remains
  read-only archive preview and is not automatically imported.
- Scope of any real testnet deployments/transactions, signing environment and
  funds. Funds have not been checked; do not assert insufficiency. This
  documentation/prompt grants no such authorization.
- Any future object deletion, incomplete-upload abort, lifecycle cleanup or
  historical unpin needs an explicit inventory/scope, recovery policy and
  authorization, not an implicit step after a successful ref update.

## Production Governance And Evidence Gates

- Define multisig membership, quorum, emergency powers and timelock policy.
- Approve INJ platform fee and treasury policy from reviewed constructor evidence.
- Approve username original-owner claim duration and public communication only
  with the required escrow/non-liability evidence for the selected scope.
- Select finality depth, RPC/reorg thresholds and recovery procedures.
- Establish key rotation, offline backup and deployment/operator separation.
- Define evidence retention and independent reviewers authorized to sign the
  exact commit/hash-bound release approval.
- Complete successor security review and both providers' native Git/Web
  acceptance before publishing a mainnet or successor public profile.

## Later Product Work — Not First-Release BYOS Blockers

- Optional dual replicas, managed public profiles, provider failover and who
  pays for any platform-operated copy/bandwidth.
- Private repositories, E2EE, per-reader key distribution/revocation, metadata
  leakage and private Web access. The choice of independent readers does not
  itself solve those problems.
- Additional cloud providers or self-hosted services, each with independent
  capability/security acceptance.
- Incremental pack dependencies/compaction and safe inventory-based orphan
  cleanup, retention/disaster-recovery service commitments.
- Whether ZKP remains an isolated testnet experiment or becomes a separately
  reviewed successor requirement, with root governance, setup, verifier/circuit
  audits, privacy and replay/front-running controls.

No fixture, previous deployment or document checkbox can decide these matters
or prove their implementation. See [the roadmap](delivery-roadmap.md) and
[S01–S07 backlog](backlog.md) for actual exit conditions.

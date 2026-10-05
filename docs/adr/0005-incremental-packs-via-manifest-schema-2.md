# ADR 0005: Incremental Pack Uploads Via Manifest Schema 2 On Suite v4

- Product decision confirmed by the user: 2026-10-05 (Asia/Shanghai).
- Implementation status: **delivered (2026-10-05, S08)** — schema-2 parsers
  (Go/TS cross vectors), `gitio` incremental pack APIs, byos chain push/fetch
  with full-pack fallbacks, and all §2.3 admission gates pass in local
  real-Git tests; a real Injective-testnet + Cloudflare-R2 incremental push
  was verified the same day on `demo-showcase-byos` (schema-2 chain manifest,
  two packs with an explicit dependency closure, digest-matching public read,
  clean cold clone; see [backlog](../backlog.md)).
- Extends [ADR 0004](0004-mainnet-storage-neutral-successor-and-byos-scope.md).
- Does not modify Suite v4 contracts, `suiteVersion`, the v3 legacy path, or
  any existing manifest schema 1 object.

## Context

Every v4 push currently uploads one self-contained full-history pack
(`cli/internal/byos/byos.go` → `PackFullHistory`), so bucket-owner cost
(storage, requests, egress and mandatory verification readbacks, per
[ADR 0004](0004-mainnet-storage-neutral-successor-and-byos-scope.md)) grows
with the full repository history on every push.

Manifest schema 1 froze `thin=false` / `dependsOn=[]` and full non-thin
history. Open questions listed incremental dependency optimization as later
work not to be reopened "without a new successor version". The user has now
explicitly decided to schedule it, choosing the small-change path.

Verified code facts this design relies on:

- Go and TypeScript parsers reject any `schemaVersion != 1`
  (`cli/internal/packmanifest/manifest.go`, `web/src/lib/packmanifest.ts`),
  so unknown versions already fail closed.
- CLI `FetchRef` and Web `gitstore.loadVerifiedRef` already iterate
  `manifest.packs` in order and verify each pack's digest/size individually
  before Git ingestion.
- `gitio.IndexVerified` currently checks the target tip and full reachability
  after every single pack; intermediate packs of a chain do not contain the
  new tip, so verification must be split (per-pack bytes/index check, one
  final whole-set closure check).

## Decision

The user decided on 2026-10-05 to build incremental packs now via manifest
schema 2, without touching the deployed Suite v4 contracts:

1. **No contract change.** The on-chain commitment stays
   `manifestDigest + manifestSize + bootstrapLocator + revision`; the chain
   never reads manifest contents, so `suiteVersion` stays 4 and no module,
   ABI or deployment change is in scope.
2. **Manifest schemaVersion 2 carries the increment.** A ref manifest lists an
   ordered, bounded pack chain. A fast-forward push appends exactly one pack
   containing only objects absent from that chain, generated with the ref's
   own previous tip as the only exclusion base
   (`git rev-list --objects <new-tip> --not <old-tip>`). Sibling refs are
   never exclusion bases (BYOS §2.3 keeps every ref independently cloneable).
3. **Explicit closure; never git thin packs.** Each new entry declares
   `dependsOn` referencing earlier entries of the same manifest only;
   receivers validate backward-only references, acyclic topological order and
   bounded depth. Every pack remains internally complete — no `--thin`, no
   `--fix-thin`; raw SHA-256 + size are verified per pack before Git sees the
   bytes, exactly as today.
4. **Fail-closed compatibility.** schemaVersion 2 manifests are rejected by
   today's parsers; old clients must report an actionable bilingual upgrade
   error, never a silent fallback. Schema 1 manifests and all existing
   digest-keyed objects remain valid and unchanged.
5. **Empty increment.** A fast-forward push adding no new objects (for
   example a tag pointing at an already-published commit) reuses the existing
   pack set with a new commit binding and revision+1; no pack upload happens.
6. **Automatic fallback to a fresh full self-contained pack** (the chain
   resets to a single entry — the sanctioned repack pattern from BYOS §2.1)
   when: the update is not fast-forward (history rewrite), the previous
   manifest is missing, corrupt or fails verification, the chain would exceed
   the 16-pack limit, or the 2 GiB total-size budget would be exceeded.
   Force push never waives revision CAS. Fast-forward/ancestry checks are
   client policy; the chain keeps only its CAS concurrency check
   (BYOS §5: the contract cannot verify Git ancestry).
7. **Per-ref independence is a hard gate.** Deleting or rewriting any other
   ref must not affect this ref's closure. The BYOS §2.3 admission criteria —
   explicit dependency closure, acyclic topological order, bounds,
   missing-dependency rejection, cross-ref deletion tests — are mandatory S08
   acceptance criteria, not aspirations.

## Out Of Scope

Chain compaction/merging, orphan GC, cross-ref or cross-repo dependencies,
git thin packs, contract or suite-version changes, private repositories, and
any change to the v3 IPFS path.

## Consequences

- Go/TS parsers gain schemaVersion 2 with new cross-tested canonical vectors;
  `protocol/packmanifest` vectors for schema 2 are generated by the real
  implementations, never hand-written.
- `gitio` needs a `PackIncremental` entry point and a split of
  `IndexVerified` into per-pack byte/index verification plus one final
  whole-set closure check (`cat-file`/`fsck` after the last pack).
- Web keeps per-pack budgets (32 MiB per pack, 256 MiB total) across the
  chain and parses schema 2.
- Per-push upload cost stops growing with full history; a cold clone still
  downloads the whole chain until compaction is separately designed and
  approved.
- This ADR is the recorded user decision that reopens exactly one frozen
  engineering item (incremental dependency packs) without a new successor
  suite version; every other frozen item in the open questions stays closed.

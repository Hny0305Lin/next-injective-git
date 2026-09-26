# Release And Cutover

Tagged releases publish only `igit` and `git-remote-igit` for Linux, macOS, and
Windows plus `checksums.txt`. They do not build or publish archived V1 code.

The release workflow requires Go vet/tests, race tests, Web API/type/build
tests, fixed `solc 0.8.24` compilation, Foundry Suite tests, checked ABI and
artifact parity, immutable profile guards, and deterministic asset checksums.
The public CLI/Web profile guards intentionally require an empty
`SuiteDirectory` until a separately reviewed cutover changes both profiles.

## Deployment Evidence

`igit-deploy-suite` consumes only checked-in artifacts from a clean reviewed
commit. A live deployment writes evidence while broadcasting. An already
deployed Suite may use the read-only historical-recovery mode, which accepts an
explicit ordered transaction journal and independently revalidates all nine
creation inputs, eight configuration calldata payloads, senders, nonces,
historical blocks, receipts, runtime/template hashes, immutable values, and
fixed-block Directory bindings. Both modes write a no-clobber `deployment.json`.

Deployment success and Blockscout verification are distinct. Deployment leaves
the Directory in `Bootstrapping`; it does not import or activate. Testnet may
temporarily use one rotated encrypted EOA for all governance roles and must
record `production_ready: false`. Production requires a separate governance
design.

## Cutover Gate

Run only against immutable real evidence:

```sh
bash scripts/migration-cutover-readiness.sh EVIDENCE_DIR EXPECTED_COMMIT
```

Required evidence always includes the explicit `cutover-scope.json`, deployment
and `blockscout-verification.json`, Solidity tests, `suite-verification.json`,
Linux/Windows Git E2E, MetaMask Web receipts, security review, finality runbook,
and explicit commit-bound approval. A `fresh-empty-suite` scope additionally
requires the zero-state activation journal and username escrow non-liability
attestation. A separately approved `cosmwasm-v1-migration` scope instead requires
the admin dry run, hash-bound migration plan/calldata, signed journal, receipts,
and fixed-block imported state.

The gate verifies every file hash and rejects links/path escapes. It also
semantically validates `deployment.json` and `suite-verification.json`: exact
source commit and compiler, nine successful deployments, runtime template/code
hashes, suite version 3, active state, seven finalized modules, matching
Directory code hashes, and module-to-directory bindings. Fresh-Suite mode also
requires zero expected/imported counts and matching empty rolling roots.

Fixture output and source readiness never prove a deployment. The default
profile must not change unless this gate and human hash-bound approval pass.

## Storage Scope

The existing gate above covers the legacy v3 IPFS cutover only. It does not
claim AWS S3 / Cloudflare R2 support and must not be used as a first-mainnet
IPFS-only launch gate. On 2026-09-13, the user confirmed the first mainnet
target as a storage-neutral successor with user-owned AWS/R2 buckets; see
[ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md).

The first-release scope is public repositories, canonical JSON manifests,
independent user reader configuration and user-paid cloud costs. No MinIO or
other self-hosted/S3-compatible provider, managed broker, private-repository
encryption or mandatory dual replica is required. AWS/R2 Git operation must
work without Kubo or iGit IPFS services.

Successor publication is **NOT PROVEN**. It requires the reviewed versioned
contracts/ABI/clients/indexer/evidence schema, both providers' real integrity
and public-read acceptance, native Windows/Linux no-Kubo Git E2E, finality,
independent security review and explicit approval. Extend and test the gate
for that protocol; do not weaken v3 checks or rewrite historical evidence.
Import of existing v3 history is conditional on a separate scope; a fresh
successor does not require fictitious migration receipts or V1 import.
See [successor evidence requirements](acceptance-evidence.md),
[BYOS implementation specification](storage-byos.md) and [roadmap](delivery-roadmap.md).
This document update grants no transaction, cloud-write or public-profile
switch authorization.

## Assets

Release binaries are version-injected and checked by
`scripts/verify-release-assets.sh`. The checksum manifest must contain exactly
the ten supported CLI/helper binaries in deterministic order.

S01–S03 local storage libraries and Go/TS/mock tests are recorded in [BYOS section 9](storage-byos.md). They do not change the v3 gate, public profiles, release scope or approve a successor deployment. The next implementation gate is S04 protocol/ABI review and S05 integration.

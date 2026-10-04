# Suite Version Compatibility Matrix

Status: active policy. Last updated: 2026-10-05.

## Overview

The igit protocol has gone through several generations. This document is the
single source of truth for which suite protocol versions exist, how their
repository content is retrieved, and what the CLI and web clients support.

## Protocol Version Table

| Version | Era | Ref shape on-chain | Pack retrieval | CLI reader | Web reader |
|---------|-----|-------------------|----------------|------------|------------|
| v1 | CosmWasm (archived) | CosmWasm contract state | IPFS (historic) | archive tooling (read-only) | `cosmwasm-v1.ts` (archive tab) |
| v2 | EVM (pre-v3, none deployed) | — (assumed same as v3) | — | — (v3 fallback) | v3 fallback |
| v3 | EVM V2 first release | `commitSha + packUris(string[])` | IPFS gateway | legacy path (Kubo/gateways) | `registry.ts` legacy + `gitstore.ts` loadRef |
| v4 | EVM V2 successor | `manifestDigest + manifestSize + bootstrapLocator + revision` | BYOS (aws-s3/cloudflare-r2) | BYOS path (verified manifest) | `registry.ts` successor + `gitstore.ts` loadVerifiedRef |
| v5+ | Future | Unknown | Unknown | — (would need update) | v4 ABI fallback + warning |

## Rules

### R1: On-chain version is authoritative
The `suiteVersion()` call on the SuiteDirectory contract determines which
protocol shape is used for reads and writes. Clients never guess; they read
the version from the chain and dispatch accordingly.

### R2: v3 and older → IPFS path
Suites with `suiteVersion <= 3` store refs as `commitSha + packUris[]` where
each packUri is an `ipfs://CID`. Content is fetched through IPFS gateways.
This path is frozen; no new features will be added.

### R3: v4 → BYOS path
Suites with `suiteVersion == 4` store refs as a manifest commitment
(digest/size/locator) plus a monotonic revision for CAS. Content is fetched
from user-owned cloud storage (AWS S3 / Cloudflare R2) through a verified
manifest reader. This is the active development path.

### R4: v5+ → best-effort fallback
Unknown future versions (v5+) are accepted during suite verification and
attempted with the latest known ABI (v4). If the ABI is unchanged, browsing
works normally but a warning badge appears. If the ABI changed, the user gets
a clear decode error rather than a blanket version rejection. Adding explicit
v5 support requires only updating `suite-compat.ts` and the corresponding
reader in `registry.ts`/`gitstore.ts`.

### R5: v1 (CosmWasm archive) is separate
V1 repositories are accessed through the dedicated archive tab in the web
and the `archive/cosmwasm-v1` tooling. They are read-only and never mixed
with EVM suite reads.

### R6: One SuiteDirectory at a time
Both CLI and web point at exactly one SuiteDirectory address. To browse
repositories from a different suite generation, change the configured
address. The version dispatch is transparent — the correct reader is
selected automatically based on the on-chain version.

## Implementation Points

| Module | Responsibility |
|--------|---------------|
| `web/src/lib/suite-compat.ts` | Version dispatch table, ref shape mapping, future-version policy |
| `web/src/lib/transport.ts` | Suite verification (accepts v3+), version in binding |
| `web/src/lib/registry.ts` | Ref reads dispatched by `refShapeForVersion()` |
| `web/src/lib/gitstore.ts` | Pack loading dispatched by ref commitment presence |
| `cli/internal/remote/byos.go` | CLI BYOS path (v4) |
| `cli/internal/remote/helper.go` | CLI legacy IPFS path (v3) |
| `cli/cmd/git-remote-igit/main.go` | CLI version probe + dispatch |

## Migration History

| Transition | What changed | Data migrated? |
|-----------|--------------|----------------|
| v1 → v3 (2026-08) | CosmWasm → EVM (fresh suite, no import) | No (fresh-empty cutover) |
| v3 → v4 (2026-10) | IPFS packUris → manifest commitment | No (fresh suite, no import) |

Future transitions follow the same pattern: a fresh suite is deployed, new
repositories live on it, and old suites remain readable through their
original paths.
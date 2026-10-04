# EVM Successor Suite (storage-neutral, candidate suiteVersion 4)

Status: **local compile/ABI gate PASS**. Foundry unit/invariant/gas: **BLOCKED**
(no forge on this machine, R04). Deployment, transactions, testnet binding and
public profiles: **NOT PROVEN** — nothing here authorizes a deployment.

This directory holds the fresh successor suite required by S04
([ADR 0004](../../docs/adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)).
The legacy v3 sources, ABIs, artifacts and deployment evidence under
`contracts/evm-v2` are untouched and remain the only deployed protocol.

## Relationship to contracts/evm-v2

| File | Relationship |
|---|---|
| `src/RepositoryCore.sol` | Evolved successor core (see below). Not byte-related to v3. |
| `src/SuiteDirectory.sol` | Byte-identical to v3 except `suiteVersion = 4` (review candidate). |
| `src/suite/*`, other 7 modules | Byte-identical to v3. The check script fails if they drift. |

## What changed in RepositoryCore

- `GitRef` is commitment-shaped: `manifestDigest (bytes32)`, `manifestSize
  (uint96, 1..65_536)`, `bootstrapLocator (bounded https:// string)`,
  `revision (uint64, monotonic from 1)`, `updatedAt/By`, `exists`.
  `commitSha` is deliberately not stored: the commit OID is bound by the
  committed manifest and carried by `RefUpdated` events.
- `updateRef(repoId, refName, commitSha, manifestDigest, manifestSize,
  bootstrapLocator, expectedRevision, expectedManifestDigest, force)` enforces
  revision CAS. Create expects `(0, bytes32(0))`; update expects the current
  `(revision, digest)`; recreation after delete expects the tombstone revision
  and zero digest. `force` never waives CAS (it only expresses client-side
  history-replacement intent the contract cannot judge).
- `deleteRef` writes a tombstone: revision stays, commitment clears. Stale
  replayed creates fail, so delete/recreate cannot ABA.
- `forkRepository` copies metadata only — no ref commitments. Forks must
  publish a fresh manifest bound to the target repo/ref context.
- Events v2 carry the full `refName` unindexed (`refId` stays the indexed
  topic) plus commit/digest/size/locator/revision, so indexers can restore
  complete ref names.
- Removed v3 machinery: `packUris` storage and merge logic, `_validatePackUri`,
  `_contains`, `TooManyPackUris`, `ShaMismatch`, `MAX_PACK_URIS`,
  `MAX_PACK_URI_LENGTH`, `MAX_FORK_REFS`/`ForkTooLarge`.
- Import records (`ImportRef`) now carry the commitment fields and are
  validated like `updateRef` (imports start at revision 1).
- The contract still never fetches off-chain bytes, never validates Git
  ancestry, and keeps the moderation hook, collaborator and ownership rules.

## Gate and artifacts

```powershell
npm ci --prefix contracts/evm-v2-successor
npm run check --prefix contracts/evm-v2-successor
```

`scripts/evm-successor-solc-check.mjs` compiles with the same locked solc
0.8.24 (optimizer runs=1, viaIR) as the v3 gate, enforces EIP-170 limits,
verifies the "unchanged from v3" file set, and checks in `abi/` and
`artifacts/` (schema `igit.evm-successor.solc-artifact.v1`).

Measured sizes (2026-10-04, solc 0.8.24):

| Contract | initcode | runtime | EIP-170 headroom |
|---|---|---|---|
| RepositoryCore (successor) | 23058 | **22637** | **1939** |
| SuiteDirectory (v=4) | 4079 | 3765 | 20811 |
| other 7 modules | identical to v3 | identical to v3 | unchanged |

The successor core is 867 bytes smaller than v3's 23504-runtime RepositoryCore
because the packUris machinery it removes outweighs the commitment/CAS code it
adds. It therefore also satisfies the v3 gate's 1024-byte headroom floor; this
gate keeps a 512-byte floor anyway.

Go bindings live in `cli/internal/chain/successor` (embedded ABIs plus a
contract-faithful fake chain for tests). The embedded ABIs must stay identical
to `abi/*.json` here; `TestEmbeddedSuccessorABIsMatchSolidityArtifacts` fails
the build on drift.

## Review decisions already applied (from the approved S04 draft)

suiteVersion = 4 (candidate, confirmed by review); commitSha not in state;
`contracts/evm-v2-successor/` layout; MANIFEST_MAX_SIZE = 64 KiB;
MAX_LOCATOR_LENGTH = 512; tombstone keeps revision and zeroes the commitment;
fork copies no refs; `force` accepted but never waives CAS.
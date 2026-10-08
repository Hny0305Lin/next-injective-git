# 07 · On-Chain Contracts and Protocol

Status: manual chapter. Audience: developers. Last updated: 2026-10-05.

This chapter is the developer-facing model of the igit control plane: what the
Suite is, how it is deployed and frozen, what each contract does, how the two
EVM ref shapes differ, and exactly what a client verifies before it reads or
writes. Sources: `contracts/evm-v2/` (Suite v3), `contracts/evm-v2-successor/`
(Suite v4), and the
[suite version compatibility matrix](suite-version-compatibility.md).

## The Suite at a glance

The control plane is a **non-upgradeable** nine-contract suite. There are no
proxies, no diamonds, no `delegatecall`, and no upgrade entry points; the
build gate rejects any production source file matching
`delegatecall|selfdestruct`, and there is no owner or governance role able to
change a module after activation.

**Infrastructure contracts (2)** — not modules, not part of the activation
loop:

| Contract | Responsibility |
|---|---|
| `SuiteDirectory` | The single trust root: state machine, chain ID, snapshot root, module address/code-hash registry, `verifyModule` |
| `BootstrapCoordinator` | One-shot ordered bootstrap: registers modules into the Directory, imports state under rolling commitments, activates |

**Production modules (7)** — each identified by a `SuiteIds` constant and a
`moduleId()`:

| Module | `SuiteIds` constant | Responsibility |
|---|---|---|
| `RepositoryCore` | `igit.module.repository-core` | Repository identity, refs, collaborators, bounded forks, timelocked ownership |
| `RecoveryModule` | `igit.module.recovery` | Guardian configuration and timelocked multi-approval recovery |
| `ModerationModule` | `igit.module.moderation` | Statuses, reports, appeals, and the mandatory policy hooks |
| `EconomicModule` | `igit.module.economic` | Native INJ sponsorship, revenue splits, platform fee |
| `UsernameModule` | `igit.module.username` | Reserved names and time-limited original-owner claims |
| `BadgeModule` | `igit.module.badge` | Non-transferable contribution badges |
| `ReleaseModule` | `igit.module.release` | Immutable `(version, platform) → sha256` release checksums |

`SuiteIds.REQUIRED_MODULES = 7`. Every module inherits `SuiteModule`, which
supplies the bootstrap state, the rolling commitment fold, and the two access
modifiers used throughout: `onlyBootstrap` and `onlyActiveSuite`.

## Deployment and freezing

A Suite goes from nothing to immutable in one directed sequence:

1. **Constructor pins the context.** `SuiteDirectory(authority, chainId,
   snapshotRoot)` requires a non-zero authority, `chainId == block.chainid`,
   and a non-zero snapshot root. `configuredChainId` and `snapshotRoot` are
   `immutable`.
2. **Coordinator is configured exactly once.** `setBootstrapCoordinator`
   checks the coordinator's runtime code hash, cross-verifies the coordinator's
   own `suiteDirectory()`/`snapshotRoot()` bindings, and then **clears
   `bootstrapAuthority`** — no second coordinator can ever be set.
3. **Modules are configured exactly once each.** `configureModule` accepts
   only the seven required ids, requires live code matching the expected hash,
   and requires the module's internal bindings (`suiteDirectory()`,
   `bootstrapCoordinator()`, `moduleId()`) to match exactly.
4. **State is imported in fixed order.** The coordinator advances a single
   `nextModuleIndex`; batches are accounted by count, sequence, and a rolling
   root folded as
   `keccak256(abi.encode(rollingRoot, moduleId, sequence, count, payloadHash))`.
   Finalizing the Username module additionally requires a non-zero
   username-escrow evidence hash — the on-chain attestation that V1 escrow
   liabilities were not migrated.
5. **Activation is atomic and permanent.** `activate()` re-checks the chain
   ID, requires all seven modules present with matching code hashes and
   `bootstrapFinalized()`, requires `readyForActivation()`, then sets
   `state = Active` and emits `SuiteActivated`.

There is no code path back to `Bootstrapping`: no deactivation, no module
re-registration, no coordinator replacement, no code-hash update. This is why
protocol changes — including the v3 → v4 ref-shape change — are **fresh
suites**, never upgrades.

## The verification chain

Ordinary clients (CLI, `git-remote-igit`, web) trust exactly one configured
`SuiteDirectory` address and verify before any read or write:

1. The RPC `eth_chainId` matches the configured profile (testnet `1439`,
   mainnet-planned `1776`).
2. The Directory has code at a pinned block tag — every subsequent read uses
   that same tag, one consistent snapshot.
3. `state == Active` (`1`).
4. `suiteVersion()` is a known version — **3 or 4**; v5+ passes verification
   but reads through the latest known ABI with a warning; anything older than
   3 is rejected because no EVM read path exists.
5. The contract's `configuredChainId` equals the live chain ID.
6. `registeredModuleCount == 7`; the coordinator address is valid and its
   runtime code hash matches the recorded `bootstrapCoordinatorCodeHash`.
7. For each of the seven modules: `moduleAddress(id)` resolves,
   `verifyModule(id)` returns true (code present, code hash matching, and the
   module bound to *this* Directory, *this* coordinator, *this* id), the live
   code hash matches, and direct reads of `suiteDirectory()`,
   `bootstrapCoordinator()`, and `moduleId()` on the module agree.

A verified binding is cached for 30 seconds; wallet connection is independent
of verification, but **no write proceeds without a verified Suite**. If any
check fails, the client fails closed — it never silently downgrades or
fallbacks to another backend.

## RepositoryCore

The core module owns repository identity and refs.

**Identity.** `repoId` is derived at creation:

```solidity
repoId = keccak256(abi.encode("igit:suite:v3:repo", block.chainid, suiteDirectory, msg.sender, name));
```

(The literal preimage stays `"igit:suite:v3:repo"` in the v4 core — reproduced
here as the source states it.) A secondary locator `keccak256(abi.encode(owner,
name))` supports rename/transfer without breaking historical URLs.

**On-chain validation.** Repository names are 1–64 bytes of
`[a-zA-Z0-9._-]`; ref names are 5–256 bytes beginning with `refs/` and
excluding Git-dangerous characters; commit SHAs are exactly 40 or 64 hex
characters; v3 pack URIs must begin with `ipfs://` (HTTPS is not accepted by
the v3 core); page sizes are capped at 64.

**Key functions.** `createRepository`, `forkRepository` (v3: copies up to 64
refs; v4: metadata only), `updateMetadata`, `updateRef`, `deleteRef`,
`setCollaborator`, the ownership-transfer family (`begin` / `cancel` /
`accept` / `expire` with a **7-day delay** and a **30-day acceptance
window**), `recoverOwnership` (callable only by the Recovery module), and the
paginated readers (`listRepositoriesPage`, `listRefsPage`,
`listCollaboratorsPage`).

Roles are `None` / `Maintainer` / `Reader`, capped at 256 collaborators per
repository and 1024 refs per repository.

## The two ref shapes

### Suite v3 — `commitSha` + `packUris`

```solidity
struct GitRef {
    string commitSha;
    string[] packUris;   // ordered ipfs://CID list, 1..128 entries
    uint64 updatedAt; address updatedBy; bool exists;
}
```

`updateRef(repoId, refName, commitSha, packUris, expectedSha, force)`:
non-forced updates require `expectedSha` to equal the stored commit SHA
(otherwise `ShaMismatch`), and **merge** new URIs into the existing list;
forced updates skip the comparison and replace the list. `deleteRef` removes
the entry entirely — there is no tombstone and no revision concept.

### Suite v4 — manifest commitment + revision

```solidity
struct GitRef {
    bytes32 manifestDigest;   // SHA-256 of the canonical JCS manifest bytes
    uint96 manifestSize;      // 1..65_536
    string bootstrapLocator;  // https://…, ≤512 chars
    uint64 revision;          // monotonic from 1, never reset
    uint64 updatedAt; address updatedBy; bool exists;
}
```

`updateRef(repoId, refName, commitSha, manifestDigest, manifestSize,
bootstrapLocator, expectedRevision, expectedManifestDigest, force)` enforces a
compare-and-swap on **every** write: create expects `(0, bytes32(0))`, update
expects the current `(revision, digest)`, recreate-after-delete expects the
tombstone revision with a zero digest. A mismatch reverts
`CommitmentMismatch`. `commitSha` is validated and emitted in events but
**never stored** — the commit OID lives inside the manifest, bound by the
committed digest. Deletion is a **tombstone** (digest zeroed, revision
preserved), so a replayed stale create fails and delete/recreate cannot ABA.

Events carry the full ref name as data (indexed only by `refId = keccak(refName)`)
so indexers can restore complete names without guessing.

### Byte identity between the suites

In `contracts/evm-v2-successor/src`, all eight contracts other than
`RepositoryCore` are byte-identical to the v3 sources; `SuiteDirectory`
differs only in `suiteVersion = 4`. `RepositoryCore` is the one rewritten
contract — commitment state, CAS, tombstones, ref-restorable events, and the
removed pack-URI machinery.

## Module reference

### RecoveryModule

Up to **10 guardians** with an owner-configured threshold; recovery proposals
mature after a **7-day delay** and expire after a **30-day window**. A stale
configuration (owner changed since configuration) reverts
`GuardianConfigStale`. `executeRecovery` requires the proposed new owner to
execute, the threshold met, the delay elapsed. This module is the **only**
caller `RepositoryCore.recoverOwnership` accepts.

### ModerationModule

Statuses `Active` / `Frozen` / `Delisted`; reports with reasons (≤128 bytes),
committee resolution, owner appeals, and full action trails. A committee
address is configured by the module admin. The module's policy hooks are the
enforcement points used by the rest of the Suite (next section).

### EconomicModule

`sponsor(repoId, message)` is a payable native-INJ call: it adds to the
repository's sponsorship total, computes `fee = value * platformFeeBps /
10_000` to the treasury, distributes each revenue split `(distributable * bps)
/ 10_000`, and pays the remainder to the repository owner. Constraints: at
most **20** split recipients, total bps ≤ 10 000, platform fee capped at
**500 bps (5%)**, the owner may not be a split recipient, and messages are
≤256 bytes. An ownership transfer **clears the splits and cancels any pending
recovery** through the Core-only hook.

### UsernameModule

Names are 3–32 bytes of `[a-z0-9-]`, no leading/trailing `-`, never beginning
with `inj1`. `registerUsername` locks the name to the caller;
`releaseUsername` gives it back; `claimOriginalUsername` is the **90-day**
post-bootstrap window for V1 original owners; a reserved list (policy admin)
blocks squatting on strategic names. One username per address; `usernameOf`
does the reverse lookup.

### BadgeModule

Owner-only `awardBadge(repoId, recipient, reason)` with a 1–256-byte reason;
the owner cannot badge themselves. Badges are struct records — there is
deliberately **no ERC-721/1155 transfer surface**, which is what makes them
non-transferable. Listing is paginated by recipient and by repository.

### ReleaseModule

The release authority registers immutable `(version, platform) → sha256`
records (tokens 1–64 bytes of `[a-zA-Z0-9._-]`); re-registering an existing
key reverts `ArtifactAlreadyRegistered`. `igit release verify <version>
<platform> <file>` recomputes a file's SHA-256 and compares it to the chain.

## Moderation hooks are mandatory

Moderation is not advisory. Core and Economic call the Directory-bound
Moderation module before changing state, and the hook cannot be bypassed by
leaving the module unconfigured (`_moderation()` reverts `SuiteNotActive` on a
zero address):

- `updateRef` / `deleteRef` → `requireRefMutation` — reverts `RepositoryFrozen` when **Frozen**;
- `sponsor` → `requireEconomicAction` — reverts when **Frozen**;
- `forkRepository` → `requireFork` — requires **Active**;
- `awardBadge` → `requireBadgeAward` — requires **Active**.

Note the asymmetry: a **Delisted** repository is hidden from listings but can
still mutate refs; a **Frozen** repository cannot. Both statuses are visible
in listings and on the web repository page.

## Transaction rules

- **Legacy type-0 transactions only.** All writes — CLI keystore and web
  wallet alike — are signed as legacy transactions. EIP-1559/type-2 is not
  implemented and is gated behind a future funded type-2 canary.
- **Minimum gas price** `160000000 wei` on chain 1439 (and 1776), enforced by
  both clients when the RPC-quoted price is lower.
- **Gas estimation with headroom** (CLI adds 10 000; web multiplies by 1.4
  and adds 10 000).
- **Receipt window ≈ 2 minutes.** If a receipt does not confirm, the client
  reports the transaction hash and **invalidates its cached nonce** — it never
  blindly re-broadcasts with a new nonce. Check the hash on
  [testnet Blockscout](https://testnet.blockscout.injective.network/), then
  retry deliberately.
- **No gas sponsorship.** Chain-native feegrant wrappers are not part of the
  immutable Suite runtime; `sponsor` pays repository revenue, not gas.

## Where the suites live today

| Suite | Testnet SuiteDirectory | Status |
|---|---|---|
| v4 (BYOS successor) | `0xf987396475d0a4c96b722e993a95d8720a6292ad` | Deployed and active; not yet a published public profile |
| v3 (IPFS) | `0xf8844F90887731FFd607E1f59e39a3918F6eAb35` | Deployed and active; frozen path |

Published network profiles keep `SuiteDirectory` **empty** until approved
cutover evidence exists; configuring an address is always the user's explicit
local choice, and a test address must never be described as an official one.

## Status boundaries

Nine production contracts compile under pinned `solc 0.8.24` and pass the
portable solc gates; the successor suite is deployed and active on Injective
testnet with real Cloudflare R2 end-to-end Git flows **PASS**. Foundry
unit/invariant/gas gates, Blockscout source verification for the successor,
security review, publication evidence, and mainnet approval remain **open**
— see [project status](project-status.md) for the authoritative record, and do
not soften those labels in any derived material.

## Next

- Storage commitment details → [Chapter 08 · BYOS storage](manual-byos-storage.md)
- Building and testing the stack → [Chapter 09 · Developer guide](manual-developer-guide.md)
- The full version matrix → [suite version compatibility](suite-version-compatibility.md)

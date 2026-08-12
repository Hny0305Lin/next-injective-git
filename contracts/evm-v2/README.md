# EVM V2 repository registry

This directory is an isolated EVM counterpart to the existing CosmWasm
`contracts/repo-registry` contract. It implements the bounded repository/ref
core needed by EVM clients:

- `createRepo`
- `updateRepoInfo`
- `updateRef`
- `deleteRef`
- `getRepo`
- `listReposPage`
- `listRefsPageById`
- `resolveRef`
- `setCollaborator`, `getCollaborator`
- `listCollaboratorsPageById`
- `setFrozen`
- `resolveRepo`, `getRepoById`
- `beginOwnershipTransfer`, `cancelOwnershipTransfer`,
  `rejectOwnershipTransfer`, `expireOwnershipTransfer`, `acceptOwnership`
- `createImportSession`, `importProgress`, `importRepo`, `importRefs`,
  `importCollaborators`, `finalizeImport`

The checked-in ABI is `abi/RepoRegistryV2.json`. CI compares it with the
Foundry build artifact and fails if contract changes make it stale.

`RepoRegistryV2EconomicModule` is an independently deployable extension keyed
by the core registry's stable `repoId`. Its checked-in ABI is
`abi/RepoRegistryV2EconomicModule.json`. It implements owner-managed revenue
splits, native-INJ sponsorship, a capped platform fee, lifetime sponsor totals,
atomic payout, and settlement reentrancy protection without adding bytes to the
near-limit core registry. The Go Economic backend and Web EIP-1193 layer route
reads and writes to this module; the Web Sponsors page exposes the split editor
only to the current EVM owner. These source paths do not supply a deployment
address or real-chain receipt evidence.

Economic entry points are `sponsor(bytes32,string)`,
`setRevenueSplits(bytes32,address[],uint16[])`, `revenueSplits(bytes32)`,
`sponsorTotal(bytes32)`, and admin-only `setFeeConfig(address,uint16)`. The
module is configured separately from the core registry; published profiles keep
its address empty until reviewed deployment and cutover evidence exist.

`RepoRegistryV2ModerationModule` is a separate report/appeal extension keyed by
the same stable `repoId`; its ABI is `abi/RepoRegistryV2ModerationModule.json`.
Anyone may submit a bounded reason-hash report, while the configured committee
or admin fallback resolves reports, records frozen/delisted/active decisions,
and appends appeal trail entries. Appeals use the V1 recorded-owner rule and
therefore preserve the owner captured at report time across a later transfer.
The module reads core existence/owner/status but does not call the core
registry's `setFrozen` or invent a delist bridge: the module's effective status
is an auditable override until a separately reviewed core bridge is deployed.
It is therefore not yet evidence that core `updateRef`/`deleteRef` are blocked
by a moderation decision, and historical V1 report import remains deferred.

Each repository receives a domain-separated immutable `repoId` at creation.
The current `(owner,name)` locator and every historical owner locator resolve
to that ID; owner/name read wrappers accept aliases, while write wrappers fail
with `RepoMoved` and the canonical locator. Transfer reserves the target
namespace, retains the V1 seven-day delay, gives the target a rejection path,
and expires permissionlessly after a 30-day acceptance window. Accept changes
the owner and locator mappings without copying refs or repository extensions.
Returning a repository to an owner whose alias already points to the same
`repoId` is allowed; an alias can never be reassigned to another repository.
Owner repository enumeration returns immutable IDs and current metadata through
`listReposPage`; creation and ownership acceptance maintain the owner index in
O(1). Ref and collaborator clients resolve the locator once, then read by
immutable `repoId`. All pages are capped at 64 entries. A client must reject a
non-advancing cursor and must not switch to a legacy backend after V2 selection
or resolution succeeds.

`RepoRegistryV2` has a single admin-controlled snapshot import session. The
session binds the V1 snapshot SHA-256, source chain/contract/height and expected
repo/ref/collaborator/batch counts. Batches are globally ordered, record their
payload SHA-256 in events, derive imported IDs with the same domain-separated
formula as the Go planner, register the normal owner index, and cannot expose
partial state: ordinary reads and writes are locked until the declared counts
are complete and `finalizeImport` succeeds. Failed transactions do not consume
their sequence number, and `importProgress` exposes fixed sequence/count fields
for resumption. `importWindowClosed()` exposes the one-shot deployment state
needed by the deployment controller. Import is a one-shot bootstrap operation:
creating any native repository or finalizing the session permanently closes
the import window.

`RepoRegistryV2ImportController` is an optional deployment-level migration
component, not the normal user-facing registry. Deployment order is controller,
then a registry constructed with the controller as admin, then `setRegistry`.
The first binding records the registry runtime code hash. A controller-managed
manifest is generated only when `igit-migrate-v1 --controller-contract` is
explicitly selected. Controller session/finalize/abort events are emitted by
the controller; repo/ref/collaborator batch events are emitted by the registry.

The controller checks the planner's ordered rolling commitment before
publication. Its `abortImport` operation is explicit pre-release recovery, not
a production rollback or a generic registry upgrade: it requires an active
session and a replacement with the same runtime code hash, controller admin,
no active session, and an open import window. It increments the deployment
generation but does not erase or mutate the abandoned registry. The deployment
must remain private until finalize and post-import comparison. A direct-registry
session is discarded if unrecoverable; a controller-managed session follows
the reviewed fresh-registry recovery path. Import preserves historical
description and default-branch bytes even when they exceed interactive V2
write limits.

The unsigned manifest and `importRepo` use `uint8 moderationStatus` with the
fixed mapping `0=active`, `1=frozen`, and `2=delisted`. Repository storage keeps
that three-state value. Only frozen rejects ref writes; delisted repositories
remain writable but are omitted from default client listings, matching V1.
The status is committed by the payload hash and emitted import event.

`cli/cmd/igit-migrate-v1` still writes a deterministic plan with
`executable:false`; with `--manifest-output` it additionally writes unsigned,
ABI-checked calldata plus the expected ABI event topic for the session, all
ordered batches, and finalization.
Plan and manifest files are immutable reviewed artifacts: the command writes
and syncs temporary files, publishes final names with no-clobber hard links,
and reports a typed partial-publication condition if a concurrent manifest
publisher wins after the plan is visible. Operators preserve the published
plan; the command never deletes or replaces migration evidence.
With `--verify-state`, the same command can strictly compare a finalized-state
JSON export against the exact plan before writing any plan or manifest output.
The export must be produced after a successful finalization receipt, with all
repository/ref/collaborator pages read at one immutable numeric EVM
`block_tag`; moving tags such as `latest` and `safe` are rejected. The command
does not contact RPC, sign, or broadcast; the emitted manifest remains
`signed:false` and `broadcast:false`. The separate read-only
`igit-migrate-v2-state` command accepts an already-mined finalize transaction,
validates its receipt target/status/event, pins all core state reads to the
receipt block, checks the block hash before and after pagination, validates
`importProgress`, and executes the exact plan comparison before exclusively
publishing an artifact that explicitly repeats the core-only scope and deferred
sections. It never signs, broadcasts, waits for a pending
transaction, or overwrites existing evidence. A resumable admin runner source
path is now present as `igit-migrate-v2-run`: it requires the exact plan hash,
strictly regenerates the canonical manifest, uses the encrypted EVM keystore,
persists each signed raw transaction before broadcast, and appends immutable
broadcast and terminal mined/reverted receipt records. Resume replays the same raw bytes and nonce;
receipt/event, sender, target, calldata, chain and gas-ceiling mismatches stop
the sequence. Because this contract imports core state only, a plan with
deferred extension sections also requires explicit core-only acknowledgement.
The signed raw journal is broadcastable operational evidence and
must remain access-restricted. A real post-import run, runner/journal security
review, a reviewed deployment, and an approved recovery runbook are still
required before a migration release; test fixtures or hand-authored state
files are not proof of an executed migration.

The source readiness scripts deliberately print `MIGRATION SOURCE READINESS`,
not cutover readiness. Real release promotion additionally requires the
hash-bound operator evidence consumed by `scripts/migration-cutover-readiness.*`.

Because owner/ref/collaborator indexes use mutable array positions, clients
must obtain one block tag before draining a multi-page result and use it for
every `eth_call`; the contract does not promise a cross-block cursor snapshot.

Owners can assign collaborators the `Maintainer` (ref write/delete) or `Reader` (read-only) role;
`None` removes a role. Only owners can manage collaborators. Owners and the
deployment admin can freeze a repository; while frozen, all ref updates and
deletions are rejected. Owners may still update repository metadata while a
repository is frozen. Metadata updates use explicit field flags so callers can
distinguish an omitted field from deliberately clearing it to an empty string.

Commit SHAs must be lowercase or uppercase hexadecimal SHA-1 (40 characters)
or SHA-256 (64 characters). Pack URIs must use the `ipfs://` scheme, have a
non-empty authority/path, and fit the bounded URI length. Ref names use Git's
`refs/...` shape. Normal updates append unique pack URIs in order, while force
updates replace the URI list with the helper's self-contained pack set. The
128-URI limit applies to the final stored list as well as one transaction, so
repeated normal pushes cannot grow a ref without bound.

## Build and test

The package uses Foundry (`forge`) and Solidity `0.8.24`, with no external
Solidity library dependencies. After installing the locked portable compiler
once with `npm ci --prefix contracts/evm-v2`, both the Foundry suite and the
portable check can run without fetching contract dependencies. From this
directory:

```text
forge build
forge test -vv
# Explicit release-gate subsets (also included in `forge test` above)
forge test --match-contract '^RepoRegistryV2InvariantTest$' -vvv
forge test --match-contract '^RepoRegistryV2GasTest$' -vvv
forge test --match-contract '^RepoRegistryV2EconomicModuleTest$' -vvv
forge test --match-path test/RepoRegistryV2ModerationModule.t.sol -vvv
```

From the repository root, the portable source/ABI check uses the same Solidity
version, optimizer settings, and `viaIR` setting as Foundry:

```text
node scripts/evm-v2-solc-check.mjs
```

`optimizer_runs = 1` is intentional while the combined compatibility/import
registry remains close to EIP-170. At the current checked source the registry
runtime is 24,433 bytes, leaving 143 bytes below the 24,576-byte limit. The
portable check fails at the protocol limit and also requires at least 128 bytes
of runtime headroom. No additional feature group may be added to this monolith.
Economic, badge, and moderation report/appeal storage are now separate modules
keyed by stable `repoId`; the moderation core-enforcement bridge, remaining
governance, fork, and release capabilities need the same reviewed module
boundary. This size-oriented setting is not a gas claim. The
suite also includes a stateful `RepoRegistryV2Handler` invariant
with bounded create/ref/delete/freeze/ownership-transfer actions
(`fail_on_revert = true`) and representative executable write gas ceilings.
Foundry runtime, invariant, and gas-ceiling results remain release evidence and
must be run in CI or a reviewed local environment.

The script compiles all deployable test contracts, including the dedicated
stateful invariant and gas-ceiling suites, rejects test initcode over
the Shanghai limit, enforces the registry EIP-170 headroom, and verifies both
checked-in registry and controller ABIs. Use `--write-abi` only
after an intentional contract API change. Keep new regression groups in
separate test contracts: the core test contract is close to the Shanghai
initcode ceiling.

Fixed Foundry v1.7.1 has been executed locally against this checkout. The full
suite passes `78/78`; `RepoRegistryV2InvariantTest` reports
`128 runs / 8,192 calls / 0 reverts`; `RepoRegistryV2GasTest` passes all `7/7` representative
write ceilings; and `forge test --gas-report` completes successfully. The
portable `solc@0.8.24` source/ABI check also passes. These are local source and
test results, not evidence of a reviewed deployment, real transaction receipt,
clean-machine Git workflow, real V1 state import, or independent security
review. The current importer remains core-only: `repo_extensions` is deferred,
so historical V1 Economic state has not been imported or compared.
The full run emits a few benign warnings while Foundry probes optional invariant
getter interfaces on contracts that do not implement them; those warnings do
not affect the zero-failure result.

# Release pipeline (as implemented)

> This document describes the current CosmWasm V1 release. EVM V2 is still
> an alpha source package and is not yet included in published release assets.
> The release workflow must not claim V2 support until it publishes a verified
> ABI/bytecode/deployment manifest, runs Foundry tests, completes native
> Windows/Linux Kubo and receipt acceptance, and records a security review.
> The source tree now contains the repo/ref core, collaborator roles and
> owner-only repo metadata patch across Solidity/ABI, Go/CLI and the Web
> EIP-1193 access layer. It also contains the immutable `repoId`, current and
> historical locator alias, `RepoMoved`, and ownership-transfer state machine
> in the Solidity source and checked-in ABI. An independent Economic module,
> Go backend, Web API, and owner-only revenue-split editor are also wired in
> source; historical Economic state import remains deferred. Those paths
> remain guarded until the release gates are complete. V1 remains the
> published/legacy-compatible contract during this migration. See
> [evm-v2-migration.md](./evm-v2-migration.md)
> and [evm-v2-repo-identity.md](./evm-v2-repo-identity.md). The new identity
> storage/API requires a new alpha deployment rather than reinterpreting the
> current source layout.

The tag workflow in `.github/workflows/release.yml` runs for tags beginning
with `v` and accepts semantic versions such as `v0.4.0` (including prerelease
or build metadata). Invalid tags fail before any build starts.

The current workflow installs the locked Web dependencies before invoking the
required source gate, and it enforces the V1-only state with the semantic Go
test `TestPublishedProfilesRemainV1OnlyUntilReviewedCutover`. That test loads
the public profiles and requires `auto/v1` defaults plus an empty V2 contract
address for every published network. It deliberately replaces a text grep;
the future V2 release path must replace this guard with verified deployment
manifest and real cutover-evidence validation.

Each accepted tag builds the Go CLI and remote helper for these targets:

- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`
- `windows/amd64`

The `igit` binaries embed the complete tag (for example, `igit v0.4.0`),
while local builds without linker flags report `igit dev`. The workflow runs
the same version test with and without the `-X main.version=...` linker
override, then executes the Linux amd64 release binary as an additional check.

The `repo-registry.wasm` artifact is built with Rust `1.81.0`, the committed
`Cargo.lock`, and `wasm32-unknown-unknown`. The build is run from
`contracts/repo-registry` so its `.cargo/config.toml` is applied. The release
checks the Wasm magic (`00 61 73 6d`) and generates a SHA-256 `checksums.txt`
covering exactly the eleven published artifacts.

During V2-alpha, both CI and the tag workflow test `contracts/evm-v2`, but
release checksums remain V1-only. The future V2 release manifest must add, at
minimum, compiler version, source commit, contract ABI, creation/runtime
bytecode, deployment chain ID and deployed address. When the migration
controller is used, it must also include the controller ABI/bytecode/address,
bound registry address and runtime code hash. A migration release must also
archive the canonical V1 snapshot and sidecar, deterministic plan and plan
SHA-256, the canonical unsigned transaction manifest, the immutable
prepared/broadcast/mined/reverted receipt journal containing signed raw
transactions, and the finalized V2 state export with its immutable numeric
`block_tag`. The
imported-state export is valid only when every paginated `eth_call` used the
same block tag after a successful finalization receipt. Do not silently replace
`repo-registry.wasm` or reuse a V1 version identifier for a V2 deployment.

`scripts/migration-readiness.sh --required` verifies source, tests, and locally
available toolchains and reports `MIGRATION SOURCE READINESS`; it is not a
deployment or cutover decision. A V2 promotion must also pass
`scripts/migration-cutover-readiness.sh <evidence-dir> <expected-commit>` (or
the PowerShell counterpart with the same two positional arguments). That
separate fail-closed gate verifies required artifact
presence, safe paths, exact SHA-256 binding, and approval metadata. Reviewers
remain responsible for semantic validation of the deployment, Foundry results,
admin dry run, migration journals, post-import state, clean Windows/Linux/Web
E2E, finality runbook, and security-review contents. The current tag workflow
only runs the gate's fail-closed regression fixture; it does not provide a real
evidence directory or execute a cutover decision.

`<expected-commit>` is the exact 40-hex source revision under review. The
approval file's `reviewed_commit` must match it case-insensitively; a tag name,
abbreviated SHA, missing argument, or approval for another commit fails closed.

The checksum manifest is portable between the native PowerShell and Bash
gates: UTF-8 LF and CRLF records are accepted, path keys are unique under
ASCII case folding, and every named path is checked. Evidence paths containing
a symbolic link or Windows reparse point are rejected, including links in an
intermediate directory, so a lexical in-directory name cannot hash an
out-of-directory target. Hard links remain valid immutable publication
artifacts and are still verified by content hash.

The alpha ABI now includes
`updateRepoInfo(string,bool,string,bool,string)`, `RepoInfoUpdated`,
`resolveRepo`, `getRepoById`, `RepoMoved`, and the seven-day ownership-transfer
state machine (`begin/cancel/reject/expire/acceptOwnership`) with a thirty-day
acceptance window. Its patch flags
distinguish an omitted field from an explicitly empty value, and CLI/Web V2
failures never fall back to a CosmWasm write. The Go remote helper and Web
route resolve historical aliases and expose the canonical URL; the EVM
contract permits a repository to return to its own historical alias but never
reassigns that alias to another repo. The CLI ownership-transfer commands now
use the unified backend for begin, accept, cancel, reject, expire, and show;
the Web transaction source path covers the same actions, but has no deployed
receipt acceptance. The independent Economic module implements native-INJ
sponsorship, platform fee, revenue splits, and stable-repo-ID state. The Go
backend and Web EIP-1193 layer cover sponsorship, split queries/updates,
receipt failure, fixed-block reads, and no V1 write fallback; the Web repository
page exposes the split editor only to the current EVM owner. Historical
Economic state import, bounded fork, guardian, moderation and full migration
parity are still outstanding. These Economic paths are source/test coverage,
not a published V2 capability. Executed Go/Web API tests also cover calldata
flags, identity decoding, receipt status, cache invalidation, bounded
owner-repository enumeration, and stable-ID ref/collaborator pagination with
no V1 fallback after V2 selection/resolution.

Fixed Foundry v1.7.1 has executed the complete local suite: `78/78` tests pass,
the stateful invariant reports `128 runs / 8,192 calls / 0 reverts`, the seven
representative write gas ceilings pass, and `forge test --gas-report`
completes successfully. The Solidity source and test contracts also compile
through `node scripts/evm-v2-solc-check.mjs`, and the checked-in ABI comparison
currently passes. This is local source/runtime/gas evidence only, not a
reviewed CI/release artifact. None of this adds V2 bytecode, a deployment
address or a V2 support claim to the published release. The source tree includes
a deterministic, hash-bound, non-broadcasting offline `igit-migrate-v1` planner,
an ABI-checked unsigned transaction manifest generator, the ordered,
admin-only Solidity import state machine, and a separate deployment-level
`RepoRegistryV2ImportController`. An audited migration profile may use
`--controller-contract` to route calls through the controller and bind every
batch to a rolling commitment; controller lifecycle events and registry batch
events intentionally have different emitters. `abortImport` is limited to an
active private bootstrap session and a fresh replacement with the same runtime
code hash, controller admin, no active session, and an open import window. It
cannot roll back the abandoned deployment or replace a published registry.
Those source artifacts are not proof of an executed migration. The manifest
continues to declare `signed:false` and `broadcast:false`. An offline
`--verify-state` path rejects plan/snapshot/target/content mismatches before
writing outputs. The read-only `igit-migrate-v2-state` command now validates an
already-mined finalize receipt and event, pins every core state query to its
numeric block, checks the block hash before and after pagination, and runs the
same exact verifier before publishing a non-overwriting artifact that records
the core-only scope and deferred sections. It does not
sign or broadcast. The separate `igit-migrate-v2-run` source path requires the
exact plan SHA-256, strictly regenerates the manifest, uses an encrypted EVM
admin key, enforces a gas ceiling, and writes immutable
prepared/broadcast/mined/reverted records so resume replays the same signed raw
transaction and nonce. Successful receipts and expected events are checked
before advancing; every terminal receipt, including reverted evidence, is
revalidated on reopen. A reverted
receipt records its canonical block and terminates that journal; it is never
counted as successful progress and is not rebroadcast on resume. Plans with
deferred V1 extension sections require a
separate core-only acknowledgement and cannot be presented as full-state
migration evidence. In particular, `repo_extensions` remains deferred, so the
new Economic runtime and clients do not imply that historical V1 sponsorship,
split, or platform-fee state has been imported. Journal files contain
broadcastable signed raw
transactions and must be archived with restricted access. Runner/journal
security review, a real run against an audited deployment, bounded fork, and
deployment acceptance remain pre-release work.

The journal syncs each file before its no-clobber link and syncs the parent
directory on POSIX. Windows has no portable directory `Sync` contract, so the
release backup/runbook must preserve the directory and account for the
platform's file-flush/NTFS atomic-link durability boundary.

The release runbook must explicitly state the Injective/CometBFT finality and
RPC consistency assumption used for receipt acceptance. The current runner
checks each receipt's canonical block at record/resume time but does not invent
an Ethereum-style confirmation depth. Any additional depth or finalized-tag
policy must be profile-owned, reviewed, and tested before broadcast.
Import calldata, events, and repository storage preserve moderation as
`0=active`, `1=frozen`, and `2=delisted`; only frozen blocks ref writes,
while delisted remains writable and is hidden from default listings as in V1.
Import is permitted only on a fresh deployment and is permanently disabled
after the first native repository or finalization. `importProgress` supports
sequence resumption; release automation must keep the deployment private until
finalize and post-import comparison, and use the audited fresh-registry recovery
path when a controller-managed session cannot be resumed.

## Checksum publication status

`checksums.txt` is published as a GitHub Release asset. The release workflow
does not hold the production admin key and therefore does not automatically
write to Injective. After the release is published, an administrator can
register the exact asset hashes on-chain from a configured `igit` client:

```bash
bash scripts/register-release.sh release v0.5.0
```

The command sends one `register_release` transaction. Registration is
immutable for a `(version, platform)` pair; a corrected build must use a new
version. Anyone can then verify a downloaded file without signing:

```bash
igit release verify v0.5.0 igit-linux-amd64 ./igit-linux-amd64
```

The chain record, not the GitHub asset metadata, is the authoritative checksum
source. The contract address and admin key used for registration must be
configured separately and must never be committed to the repository.

`scripts/register-release.sh` and `igit release verify` currently use the
CosmWasm V1 release registry and therefore require a V1 profile. Explicit
`evm/v2` profiles are unsupported until the release registry is migrated.

The tagged Windows `igit` binary also uses `checksums.txt` when bootstrapping
its matching Linux `igit` and `git-remote-igit` assets into WSL2. Push runtime
dependencies are separately pinned in the binary's embedded
`cli/internal/bootstrap/deps.json`; setup verifies those upstream artifacts
before extraction. The guarded V2 source path uses the same manifest to install
Kubo only, including the pinned Windows `ipfs.exe`, and never installs
`injectived`. This does not make V2 a published capability until the deployment
and clean-machine acceptance gates above are complete.

## Upgrade governance

The contract admin should be a production multisig. Before a version upgrade,
the multisig submits `schedule_upgrade` with the exact Wasm SHA-256. The
contract exposes this proposal through `upgrade_security` and rejects
`migrate` until 14 days have elapsed and the migration message contains the
same hash. `scripts/schedule-upgrade.sh` creates the announcement; the
`scripts/testnet-migrate.sh` flow accepts the hash explicitly when executing
the delayed migration. Cancelling a proposal is available through the
`cancel_upgrade` execute message.

Before enabling a production reaper or registering the first release, run the
read-only mainnet governance preflight with the deployed contract and the
expected multisig addresses:

```bash
LCD=https://lcd.injective.network \
CONTRACT=inj1... \
EXPECTED_ADMIN=inj1... \
EXPECTED_COMMITTEE=inj1... \
EXPECTED_WASM_SHA256=<release-wasm-sha256> \
bash scripts/mainnet-governance-check.sh
```

The check confirms that the LCD reports `injective-1`, the Wasm and
in-contract admin are aligned, the moderation committee is separate, the
treasury and username policy are configured, the platform fee is at most 500
basis points, and the upgrade timelock is exactly 14 days. It is a configuration
gate only. When `EXPECTED_WASM_SHA256` is supplied, it also queries the LCD
`data_hash` for the contract's `code_id` and compares it to the release
artifact. It does not create multisigs or substitute for on-chain transaction
evidence.

The reusable local validator is `scripts/verify-release-assets.sh`; run it
from Linux, WSL, or another POSIX shell. It checks asset names and sizes,
version output, Wasm magic, and exact checksum contents.

No hash-bound local verification log for the historical `v0.5.0` WSL run is
archived in this repository, so that run must not be treated as auditable
release or cutover evidence. The first production `RegisterRelease` transaction
remains pending until a mainnet contract and authorized admin signer are
available.

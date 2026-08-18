# EVM V2 Suite Migration

## Purpose And Name

CosmWasm V1 Push ran natively on Linux, but the supported Windows path depended
on WSL2 for the Linux CLI, `injectived`, and Kubo. The successor is named **EVM
V2** because the second product-generation control plane deliberately moves to
Injective EVM. It is not a generic V2 label and it is not the same number as the
on-chain Suite protocol version, which is currently 3.

The platform objective is a native Windows path with no WSL2 or `injectived`
prerequisite and a native Linux path with no `injectived` prerequisite. Kubo is
native when the current IPFS adapter is selected. Longer term, pack storage is
intended to be pluggable so Amazon S3 or Cloudflare R2 profiles do not require
Kubo at all. Those object-storage adapters are planned and are not supported by
the current immutable Suite.

## Accepted Scope Changes

The initial EVM V2 foundation was superseded during the immutable Suite rewrite:

- Ordinary CLI, Web repository, and remote-helper access is EVM-only. The
  narrowly scoped V1 read fallback from the foundation plan is no longer a
  supported runtime. The Web has an explicit `/archive/cosmwasm-v1` viewer for
  historical read-only inspection; it is a separate GET-only archive surface,
  not a fallback and not a write path. V1 access used for migration remains
  explicit, fixed-height archive evidence followed by verified one-time
  import.
- The original monolithic EVM registry alpha was replaced by one
  `SuiteDirectory`, one `BootstrapCoordinator`, `RepositoryCore`, and six
  bounded business modules with immutable bindings.
- The foundation-era signed migration runner targeted the alpha layout and was
  withdrawn. The current command builds and verifies unsigned calldata only. A
  newly reviewed operator runner is required before real import or activation.

These are deliberate product and safety decisions, not incidental file moves.
Their rationale and consequences are recorded in
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md). The storage direction
is recorded separately in [ADR 0002](adr/0002-pluggable-pack-storage.md).

## Delivery Status

| Area | Repository state | Completion evidence still required |
|---|---|---|
| Suite source architecture | Present in source | Reviewed, commit-bound green CI and security findings resolved |
| V1 inventory and planning | Offline source tooling present | Complete fixed-height production inventory and independently verified plan |
| Operator broadcast and import | Not implemented for the Suite | Reviewed runner, signed append-only journal, receipts, safe resume, fixed-block state |
| Activation and cutover | Not executed | Module parity, active Directory, Linux/Windows Git E2E, Web receipts, finality and approval |
| Pluggable object storage | Planned only | Successor URI protocol plus S3/R2 upload, fetch, integrity, credential and E2E support |

## Cutover Workflow

Migration turns a complete fixed-height V1 archive snapshot into an immutable
Suite bootstrap. It does not provide an ordinary V1 runtime path. Until a
reviewed migration runner exists, an archived repository is shown with a
migration-required notice and remains read-only in the Web viewer.

1. `igit archive inventory` reconstructs owners, reports, usernames, badge
   recipients, and release versions from successful transaction events.
2. `igit archive verify` cross-checks inventory, transaction search, block
   evidence, block hash, and canonical snapshot at the cutover height.
3. Username deposits are refunded on the historical chain; all releases and a
   zero escrow balance are required before planning.
4. `igit-deploy-suite` deploys the nine contracts and writes no-clobber
   bootstrapping evidence.
5. `igit-suite-migrate build` produces deterministic plan and unsigned ABI
   calldata. `verify` independently checks exact parity.
6. **Planned, not implemented:** a reviewed operator runner broadcasts ordered
   coordinator batches with an encrypted rotated key while preserving an
   append-only signed journal and receipts.
7. After every module count and root is verified, the runner requests atomic
   Directory activation. This has not been executed against a reviewed Suite.
8. Fixed-block queries compare every imported item and write activation, code
   hash, and binding evidence.
9. Clean Git and Web acceptance plus security review feed the cutover gate.

Pending ownership transfers, recovery proposals, and other temporal operations
are not migrated. They are re-created after activation. New sponsorship accepts
native INJ only; migrated historical denomination totals remain queryable.

All snapshots, hashes, plans, manifests, journals, receipts, and state outputs
are no-clobber. Duplicate or missing inventory, reorged evidence, tampering,
incorrect commitments, partial imports, count mismatch, or nonzero username
escrow fail closed.

## Completion Definition

- A clean Windows machine completes `init`, `push`, `clone`, `fetch`, `pull`,
  and ref deletion with native tools, without installing WSL2 or `injectived`.
- A clean Linux machine completes the same workflow without `injectived`.
- The current IPFS profile runs Kubo natively on either operating system; a
  future accepted object-storage profile runs without Kubo.
- Users do not need to understand EVM/CosmWasm, RPC, ABI, nonce, or keyring
  internals for ordinary Git operations.
- CLI, the ordinary Web repository path, and the remote helper enforce the same
  Suite behavior and use one trust root; the explicit Web archive viewer is
  read-only and there is no V1 write fallback or double write.
- Every V1 repository kept in the supported product is verified and imported
  before cutover; unimported V1 state is available only through archive tools.
- Private keys and object-storage credentials never enter logs, Git config,
  on-chain URIs, or ordinary plaintext configuration.
- Solidity runtime tests, native Windows/Linux E2E, Web receipts, imported-state
  parity, finality handling, and independent security approval all pass.

No checked-in profile currently claims a deployed Suite. See
[release and cutover](release.md) for the evidence gate.

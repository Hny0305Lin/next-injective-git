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
  not a fallback and not a write path. Under the current scope, V1 data is not
  imported into the EVM Suite.
- The original monolithic EVM registry alpha was replaced by one
  `SuiteDirectory`, one `BootstrapCoordinator`, `RepositoryCore`, and six
  bounded business modules with immutable bindings.
- The foundation-era signed migration runner targeted the alpha layout and was
  withdrawn. A replacement Suite operator runner was implemented and tested,
  but the current fresh-empty cutover does not use it because no V1 state is
  imported.

These are deliberate product and safety decisions, not incidental file moves.
Their rationale and consequences are recorded in
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md). The storage direction
is recorded separately in [ADR 0002](adr/0002-pluggable-pack-storage.md). The
fresh-Suite and V1 archive-preview decision is recorded in
[ADR 0003](adr/0003-fresh-evm-suite-and-v1-archive-preview.md).

## Delivery Status

| Area | Repository state | Completion evidence still required |
|---|---|---|
| Suite source architecture | Present in source; P0 source/CI baseline complete | Independent security findings and approval remain open |
| Native Windows/EVM baseline | P0 source, environment, and CI gates complete | Clean release-asset Git E2E and product acceptance |
| V1 archive scope | Read-only `/archive/cosmwasm-v1` preview retained | No import or ordinary-client compatibility path is required |
| Operator broadcast and import | ✅ P1.1 tool complete and retained | Not used by the accepted fresh-empty cutover; future migration requires a separate decision |
| Testnet deployment | ✅ P1.2 complete: 9 deployments, 8 configuration calls, 17 historical transactions revalidated, 9/9 Blockscout verified | Common product-acceptance evidence remains open |
| Activation and cutover | ✅ P1.3 fresh-empty activation complete at fixed block with zero counts and matching empty roots | Linux/Windows Git E2E, Web receipts, finality, security review, and approval |
| Pluggable object storage | Planned only | Successor URI protocol plus S3/R2 upload, fetch, integrity, credential and E2E support |

## Current Fresh-Suite Cutover Workflow

The current cutover starts the EVM product from empty state. CosmWasm V1 remains
available through the explicit archive preview and is not an ordinary runtime or
an import source for this Suite.

1. `igit-deploy-suite` deploys the nine contracts and writes no-clobber evidence,
   or its historical-recovery mode revalidates an existing deployment from the
   exact ordered transaction journal.
2. Blockscout evidence independently proves source verification for all nine
   deployed addresses.
3. `cutover-scope.json` binds the reviewed commit to `fresh-empty-suite`, the
   archive-preview-only V1 policy, and zero expected records for all modules.
4. Every coordinator module progress record is started and finalized with zero
   expected/imported records, zero batches and sequences, and equal expected and
   rolling empty roots.
5. Username escrow non-liability is attested for the empty Suite, then the
   Directory is atomically activated.
6. Fixed-block queries record active state, code hashes, module bindings, zero
   import progress, and the activation block hash.
7. Clean Git and Web acceptance plus finality, security review, and approval feed
   the cutover gate.

Pending ownership transfers, recovery proposals, and other temporal operations
are not migrated. They are re-created after activation. New sponsorship accepts
native INJ only; migrated historical denomination totals remain queryable.

All scope files, transaction journals, deployment manifests, receipts, and state
outputs are no-clobber. Missing transactions, altered initcode or calldata,
reorged blocks, tampering, nonzero import counts, mismatched empty roots, or
nonzero username escrow liability fail closed.

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
- V1 repositories remain available only through the explicit read-only archive
  preview. Repositories intended for active EVM use are recreated in the fresh
  Suite rather than imported implicitly.
- Private keys and object-storage credentials never enter logs, Git config,
  on-chain URIs, or ordinary plaintext configuration.
- Solidity runtime tests, zero-import fixed-block parity, native Windows/Linux
  E2E, Web receipts, finality handling, and independent security approval all
  pass.

No checked-in profile currently claims a deployed Suite. See
[release and cutover](release.md) for the evidence gate.

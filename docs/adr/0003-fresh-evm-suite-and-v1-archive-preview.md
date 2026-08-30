# ADR 0003: Start The EVM Suite Empty And Keep V1 As Archive Preview

- Status: Accepted for the current testnet cutover
- Decision date: 2026-08-29
- Delivery status: Empty Suite activation complete; product cutover pending
- Supersedes: The mandatory V1 import portions of [ADR 0001](0001-evm-v2-runtime-and-migration-scope.md)

## Context

The immutable EVM Suite and the read-only CosmWasm V1 archive viewer are both
implemented. Importing all historical V1 repositories would require a fixed
cutover height, inventory, deterministic migration plan, signed broadcast
journal, receipt set, imported-state parity evidence, and historical alias
support in the ordinary EVM product.

The current product decision is not to carry CosmWasm V1 state into the EVM
Suite. V1 remains useful for historical inspection, but it is not a live
compatibility backend and does not participate in EVM writes.

## Decision

1. The current Injective EVM testnet Suite starts from empty application state.
2. CosmWasm V1 is exposed only through the explicit read-only archive preview.
   Ordinary CLI, Web repository, and remote-helper operations remain EVM-only.
3. The current cutover does not require a V1 inventory, migration plan,
   migration manifest, operator dry-run journal, migration receipt journal, or
   imported-state comparison.
4. Empty-state acceptance instead requires a hash-bound `cutover-scope.json`,
   zero expected/imported counts for every module, matching empty rolling roots,
   username escrow non-liability evidence, the activation journal, and the
   fixed-block active Suite verification.
5. The migration operator remains retained and tested for a separately approved
   future migration, but it is not a blocker for this fresh-Suite cutover.
6. Historical V1 aliases are archive-viewer concerns, not ordinary EVM Git
   compatibility requirements.

## Consequences

- P1.3 is satisfied by verified zero-import activation rather than V1 data
  parity.
- Existing V1 repositories do not automatically appear in the EVM Suite and
  must be recreated if they are intended to become active EVM repositories.
- There is no V1 write fallback, double write, or implicit cross-runtime read.
- The cutover gate selects evidence requirements from the explicit scope file:
  fresh-empty evidence for this decision, or full migration evidence if a later
  approved cutover chooses `cosmwasm-v1-migration`.
- Independent security review, clean Windows/Linux Git E2E, Web wallet receipts,
  finality documentation, and hash-bound approval remain mandatory.

## Acceptance

The fixed-block Suite evidence must show version 3, active state, seven finalized
modules, zero expected and imported records, zero batches and sequences, matching
expected/rolling roots, verified runtime code hashes, and immutable Directory
bindings. The V1 archive surface must remain read-only and separate from the
ordinary EVM product path.

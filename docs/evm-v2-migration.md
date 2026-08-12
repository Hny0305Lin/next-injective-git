# EVM Suite Migration

Migration turns a complete fixed-height V1 archive snapshot into an immutable
Suite bootstrap. It does not provide an ordinary V1 runtime path.

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
6. An operator broadcasts ordered batches through the coordinator with an
   encrypted rotated key, preserving signed journal and receipts.
7. Every module count/root is verified, then Directory activation is atomic.
8. Fixed-block queries compare every imported item and write activation/code
   hash/binding evidence.
9. Clean Git and Web acceptance plus security review feed the cutover gate.

Pending ownership transfers, recovery proposals, and other temporal operations
are not migrated. They are re-created after activation. New sponsorship accepts
native INJ only; migrated historical denomination totals remain queryable.

All snapshots, hashes, plans, manifests, journals, receipts, and state outputs
are no-clobber. Duplicate/missing inventory, reorged evidence, tampering,
incorrect commitments, partial imports, count mismatch, or nonzero username
escrow fail closed.

No checked-in profile currently claims a deployed Suite. See
[release and cutover](release.md) for the evidence gate.

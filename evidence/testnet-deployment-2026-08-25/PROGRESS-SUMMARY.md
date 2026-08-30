# P1 Deployment Progress Summary

> [!IMPORTANT]
> Current status (2026-08-30): P1.2 deployment evidence and P1.3
> `fresh-empty-suite` activation are complete. CosmWasm V1 is retained only as
> a read-only archive preview; no V1 state is imported into this Suite.

**Network:** Injective Testnet (Chain ID 1439)
**Deployer:** `0x85eAc7bC081488AA77D1D82f9cB8e053De1e4Fa8`
**SuiteDirectory:** `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334`

## P1.2: Deployment Evidence — Complete

- [x] Nine contracts deployed and their addresses recorded.
- [x] Eight configuration transactions and seventeen historical transactions revalidated.
- [x] Deployment receipts, exact initcode/calldata, runtime templates, immutables, and bindings recorded in `deployment.json`.
- [x] All nine contract verification results recorded in `blockscout-verification.json`.
- [x] No-clobber recovery input retained in `deployment-recovery-input.json`.

The `collect-deployment-metadata.ps1` and `.sh` scripts only query on-chain
bytecode presence and write optional `metadata/*.json` files. They do not claim
to collect transaction receipts.

## P1.3: Fresh-Empty Activation — Complete

- [x] Cutover mode is `fresh-empty-suite`.
- [x] Directory and all seven modules are active and bound at the recorded fixed block.
- [x] Expected and imported counts are zero, with matching empty roots.
- [x] Username escrow attestation records no imported username liability.
- [x] `v1_runtime_policy` is `archive-preview-only` and `v1_migration_performed` is `false`.

Canonical activation evidence is `suite-verification.json`, with scope in
`cutover-scope.json`.

## Remaining P1 Work

P1.4–P1.6 remain open and must not be represented as complete:

- Clean Linux and Windows Git E2E, MetaMask/Web write receipts, uncertain-receipt handling, and finality checks.
- Independent security review, finding resolution, and hash-bound cutover approval.
- Final evidence collection, checksum manifest generation, and the fail-closed cutover gate.

## Evidence Hygiene

- `empty-suite-activation-2026-08-29.log` is local activation output and remains ignored; do not commit it.
- The obsolete `receipts/` files were code-size metadata mislabeled as receipts and have been removed.
- The half-finished automated Blockscout verifier was removed; verification remains a project-managed/manual operation.

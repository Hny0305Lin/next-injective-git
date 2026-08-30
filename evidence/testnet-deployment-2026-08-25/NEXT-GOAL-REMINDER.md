# Remaining P1 Work

> [!IMPORTANT]
> P1.2 deployment evidence and P1.3 `fresh-empty-suite` activation are already
> complete. Do not repeat the old manual receipt collection, Blockscout
> verification, or V1 import instructions.

## P1.4: Product E2E and Finality

- Execute the MetaMask/Web write path and retain a successful receipt.
- Run Git E2E on clean Linux and Windows environments.
- Verify the V1 archive preview remains GET-only and isolated from ordinary EVM paths.
- Exercise uncertain receipts, finality waits, and reorg handling.

## P1.5: Security and Approval

- Complete an independent deep source review.
- Resolve all findings and produce `security-review.pdf`.
- Produce the approval bound to the reviewed source commit in `cutover-approval.txt`.

## P1.6: Final Evidence Gate

- Collect the remaining evidence files without fabricating missing results.
- Generate `cutover-evidence.sha256` after the evidence set is stable.
- Run `scripts/migration-cutover-readiness.ps1` and `.sh` with the approved commit.
- Update public configuration only after the fail-closed gate passes.

## Scope Reminder

The current deployment is intentionally empty:

- `mode`: `fresh-empty-suite`
- `v1_runtime_policy`: `archive-preview-only`
- `v1_migration_performed`: `false`
- Expected/imported records: zero for every module

The activation log is local-only and ignored. It must not be included in a
commit.

# P1 Task Completion Checklist

> [!IMPORTANT]
> Updated 2026-08-30. P1.2 deployment evidence and P1.3 fresh-empty
> activation are complete. This cutover intentionally does not import V1
> CosmWasm state.

## P1.2: Deployment Evidence

- [x] Deploy all nine EVM contracts on Injective Testnet.
- [x] Record the SuiteDirectory and seven module addresses.
- [x] Revalidate nine deployment transactions, eight configuration transactions, and seventeen historical transactions.
- [x] Record exact deployment and configuration evidence in `deployment.json`.
- [x] Record the project-managed nine-contract verification result in `blockscout-verification.json`.
- [x] Keep deployment recovery inputs in `deployment-recovery-input.json`.

## P1.3: Fresh-Empty Activation

- [x] Bind the cutover to `fresh-empty-suite` in `cutover-scope.json`.
- [x] Keep V1 as an isolated read-only archive preview.
- [x] Skip V1 snapshot creation, migration planning, and live import.
- [x] Activate the Directory and finalize all seven modules.
- [x] Verify zero expected/imported records, matching empty roots, code hashes, and bindings.
- [x] Record the zero-liability username escrow attestation.

## P1.4: E2E and Finality — Open

- [ ] MetaMask/Web write test with a successful receipt.
- [ ] Clean Linux Git E2E.
- [ ] Clean Windows Git E2E.
- [ ] Uncertain-receipt and finality/reorg handling checks.

## P1.5: Security and Approval — Open

- [ ] Complete independent deep source security review.
- [ ] Resolve and record all security findings.
- [ ] Generate `security-review.pdf`.
- [ ] Obtain independent hash-bound approval in `cutover-approval.txt`.

## P1.6: Final Evidence Gate — Open

- [ ] Collect the remaining acceptance evidence.
- [ ] Generate `cutover-evidence.sha256` only after the evidence set is final.
- [ ] Run both readiness gates with the approved source commit.
- [ ] Keep the checked-in public configuration unchanged until the gate passes.

## Commit Hygiene

- Do not commit `empty-suite-activation-2026-08-29.log`; `*.log` remains ignored.
- Do not restore the removed `receipts/` directory; its files were not transaction receipts.
- Do not add an automated Blockscout verifier; the half-finished script was intentionally removed.

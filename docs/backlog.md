# Remaining Work

The immutable Suite source path is implemented, but no checked-in profile may
claim a live deployment until all real evidence exists.

| Work | Required evidence |
|---|---|
| Testnet deployment | No-clobber manifest, nine successful receipts, runtime/template hashes, constructor arguments, and Blockscout results |
| Historical import | Complete fixed-height inventory, snapshot/hash, deterministic plan/calldata, signed journal, receipts, and module count/root parity |
| Activation | Fixed-block active Directory, version/chain checks, seven module code hashes/bindings, and imported-state comparison |
| Git acceptance | Clean Linux and Windows init/push/clone/fetch/pull/delete plus historical alias resolution |
| Web acceptance | MetaMask receipts for supported writes with explicit legacy transaction parameters |
| Security | Deep source review, operator-runner review, resolved findings, and hash-bound approval |
| Production governance | New multisig/timelock design and deployment; the temporary testnet single EOA is not production-ready |

The public SuiteDirectory fields remain empty until the cutover gate passes.

## Engineering TODO

- Install Foundry and execute the Suite unit, fuzz, stateful invariant, and gas
  suites. The locked `solc 0.8.24` compile gate passes, but it does not execute
  EVM runtime behavior.
- Add an operator runner for the reviewed calldata manifest. It must use the
  encrypted keystore transactor, preserve an append-only signed journal and
  receipts, resume safely after uncertain receipts, and emit fixed-block
  imported-state evidence. The current migration command is deliberately
  unsigned and never broadcasts.
- Replace or archive the root `scripts/testnet-e2e.sh`, which is still an
  explicitly gated V1 script. Add pure-Suite clean-environment Git acceptance
  for Linux and Windows, including historical alias resolution.
- Run the PowerShell cutover fixtures in Windows CI. The current development
  environment has no `pwsh` binary.
- Split and review this migration commit before cutover. No public profile may
  receive a SuiteDirectory address as part of source-readiness work.

## Testnet Cutover TODO

- Rotate and fund the testnet EOA, fix the V1 cutover height, and generate the
  complete inventory, snapshot, SHA-256 sidecar, and username escrow-release
  evidence.
- Deploy and verify all nine contracts, import every ordered batch, activate
  the Directory, and compare every migrated domain at one finalized block.
- Record Blockscout verification, MetaMask receipts, Linux/Windows Git E2E,
  finality handling, and an independent hash-bound cutover approval.
- Only after the evidence gate passes, set the single SuiteDirectory address in
  the CLI and Web testnet profiles.

## Policy TODO

Deep security review is currently skipped by operator decision. The cutover
gate still requires `security-review.pdf`; therefore cutover remains blocked
until that evidence is supplied or an explicit reviewed policy change removes
the requirement from both Linux and PowerShell gates. Do not satisfy the gate
with placeholder evidence.

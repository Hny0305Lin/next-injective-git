# Remaining Work

This file is the granular task inventory. Phase ordering, dependency gates,
current milestone status, and shared exit criteria are maintained in the
[Delivery Roadmap](delivery-roadmap.md).

The immutable Suite source path is implemented, but no checked-in profile may
claim a live deployment until all real evidence exists.

| Work | Required evidence |
|---|---|
| Testnet deployment | No-clobber manifest, nine successful receipts, runtime/template hashes, constructor arguments, and Blockscout results |
| Historical import | Complete fixed-height inventory, snapshot/hash, deterministic plan/calldata, signed journal, receipts, and module count/root parity |
| Activation | Fixed-block active Directory, version/chain checks, seven module code hashes/bindings, and imported-state comparison |
| Git acceptance | Clean native Windows without WSL2 or injectived, and clean Linux without injectived: init/push/clone/fetch/pull/delete plus historical alias resolution |
| Web acceptance | MetaMask receipts for supported writes with explicit legacy transaction parameters |
| Storage portability | Successor URI/digest contract and Linux/Windows E2E for Amazon S3 and Cloudflare R2 without Kubo |
| Isolated ZKP prototype | Approved statement/public inputs, pinned circuit/setup artifacts, native Windows proof generation, testnet valid/invalid/replay receipts, and gas/prover benchmarks; no Suite integration |
| Security | Deep source review, operator-runner review, resolved findings, and hash-bound approval |
| Production governance | New multisig/timelock design and deployment; the temporary testnet single EOA is not production-ready |

The public SuiteDirectory fields remain empty until the cutover gate passes.

## Engineering TODO

- Record and retain a passing, commit-bound immutable Suite CI run. CI already
  pins Foundry v1.7.1 and invokes the unit, fuzz, stateful invariant, gas-ceiling,
  and gas-report checks; workflow configuration alone is not evidence that the
  reviewed migration commit passed. A retained local run is useful interim
  verification, but final evidence must include the CI URL and exact commit.
- Add an operator runner for the reviewed calldata manifest. It must use the
  encrypted keystore transactor, preserve an append-only signed journal and
  receipts, resume safely after uncertain receipts, and emit fixed-block
  imported-state evidence. The current migration command is deliberately
  unsigned and never broadcasts.
- Replace or archive the root `scripts/testnet-e2e.sh`, which is still an
  explicitly gated V1 script. Add pure-Suite clean-environment Git acceptance
  for Linux and Windows, including historical alias resolution.
- Record and retain a passing native Windows CI run for Go/Kubo tests and the
  PowerShell cutover fixture. The Windows job is already defined; local Windows
  execution is useful interim verification, but neither the job definition nor
  an unbound local result replaces a green run for the reviewed commit.
- Design the successor pack-reference protocol and storage adapter boundary for
  Amazon S3 and Cloudflare R2. The current immutable Suite and clients remain
  `ipfs://`-only; do not claim object-storage support until upload, durable-write
  confirmation, digest verification, credential isolation, fetch, migration,
  and native Windows/Linux E2E all pass. See
  [ADR 0002](adr/0002-pluggable-pack-storage.md).
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

The initial EVM V2 cutover may use the current IPFS adapter. S3/R2 are a
separate successor-protocol milestone and must not be implied by that release.

## Policy TODO

Deep security review is currently skipped by operator decision. The cutover
gate still requires `security-review.pdf`; therefore cutover remains blocked
until that evidence is supplied or an explicit reviewed policy change removes
the requirement from both Linux and PowerShell gates. Do not satisfy the gate
with placeholder evidence.

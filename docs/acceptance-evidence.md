# Acceptance Evidence

Repository tests demonstrate source behavior only. They are not deployment,
migration, finality, wallet, or operational evidence.

The cutover evidence directory must contain the exact files required by
`scripts/migration-cutover-readiness.sh`, all bound by
`cutover-evidence.sha256`. `cutover-approval.txt` binds an independent reviewer
to the exact 40-hex source commit and UTC review time.

`deployment.json` must be emitted by the no-clobber deployment tool, either
during live broadcast or through its strict read-only historical-recovery mode.
Historical recovery is accepted only when all 17 ordered deployment and
configuration transactions, exact input bytes, historical blocks, receipts,
runtime templates, and bindings validate. Separate
`blockscout-verification.json` proves all nine deployed addresses are verified.
`suite-verification.json` must be produced from fixed-block reads after
activation. The gate checks version 3, active state, source/chain/snapshot,
nine deployed contract receipts and code hashes, seven finalized module
bindings, and runtime hash equality. `cutover-scope.json` selects the evidence
branch. The accepted `fresh-empty-suite` branch requires zero module counts and
batches, matching empty roots, the activation journal, and username escrow
non-liability evidence; it does not require V1 migration artifacts. Common
required files still cover Solidity tests, clean Linux/Windows Git E2E, Web
receipts, security review, and the finality runbook.

Real testnet deployment, Blockscout, fresh-empty activation, and fixed-block
Suite evidence now exists under the local evidence directory. The public
SuiteDirectory profile remains empty until the remaining common evidence set is
reviewed and passes. Commit-bound P0 source/CI results are tracked separately in
[P0 Evidence Record](p0-evidence.md); they do not satisfy deployment, wallet,
finality, or cutover acceptance.

The current evidence schema covers the IPFS-backed Suite cutover. It contains
no evidence for Amazon S3 or Cloudflare R2 support; those adapters remain a
successor-protocol roadmap item under
[ADR 0002](adr/0002-pluggable-pack-storage.md).

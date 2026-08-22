# Acceptance Evidence

Repository tests demonstrate source behavior only. They are not deployment,
migration, finality, wallet, or operational evidence.

The cutover evidence directory must contain the exact files required by
`scripts/migration-cutover-readiness.sh`, all bound by
`cutover-evidence.sha256`. `cutover-approval.txt` binds an independent reviewer
to the exact 40-hex source commit and UTC review time.

`deployment.json` must be emitted by the no-clobber deployment tool. Separate
`blockscout-verification.json` proves all nine deployed addresses are verified.
`suite-verification.json` must be produced from fixed-block reads after import
and activation. The gate checks version 3, active state, source/chain/snapshot,
nine deployed contract receipts and code hashes, seven finalized module
bindings, and runtime hash equality. Other required files cover Solidity tests,
admin dry run, migration plan/calldata, signed journal, receipts, imported state,
clean Linux/Windows Git E2E, Web receipts, security review, and finality runbook.

No real deployment or migration evidence is checked into this repository. The
public SuiteDirectory profile remains empty until the complete evidence set is
reviewed and passes. Commit-bound P0 source/CI results are tracked separately in
[P0 Evidence Record](p0-evidence.md); they do not satisfy deployment, wallet,
finality, or cutover acceptance.

The current evidence schema covers the IPFS-backed Suite cutover. It contains
no evidence for Amazon S3 or Cloudflare R2 support; those adapters remain a
successor-protocol roadmap item under
[ADR 0002](adr/0002-pluggable-pack-storage.md).

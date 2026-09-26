# Acceptance Evidence

Scope note (2026-09-13): the existing schema/gate described below targets
**legacy Suite v3 IPFS**, not the first mainnet successor.
[ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md) confirms
AWS S3 / R2 BYOS on a storage-neutral successor as the first mainnet scope.
S01–S03 local protocol/provider tests are recorded separately in [BYOS section 9](storage-byos.md); successor implementation and real acceptance remain **NOT PROVEN**. This document
does not upgrade the gate or authorize a deployment.

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

The current evidence schema covers the IPFS-backed v3 cutover only. It contains
no evidence for AWS S3 / R2 support. Preserve that schema and historical
evidence; do not change old receipts, ABI, suite version or checksums to make
them appear to verify a successor.

## Required Successor Evidence Extension (Not Yet Implemented)

- Freeze the manifest encoding/schema and Go/TypeScript golden vectors;
  attest exact manifest digest/size, pack raw SHA-256/size and context binding.
- Bind successor version, contract source/artifacts, ABI, Go/Web/indexer
  decoders, Directory/module hashes and the new evidence schema to one commit.
- Separate local mock/fixture results from real AWS and real R2 receipts and
  read-back checks; show conditional duplicates/conflicts, limits, supported
  multipart behavior, independent reader permissions, public GET and CORS.
- Show no-Kubo clean Windows/Linux Git workflows and public Web reads on the
  deployed successor, including new refs, force, deletion, fork/context,
  same-commit manifest changes, CAS conflicts and uncertain receipt/reorg recovery.
- Record source→pack→manifest→transaction→verified-read causality without
  credentials, signed URLs or tokens. Include provider costs/resource scope
  and the explicit authorizations used for canaries and transactions.
- A fresh successor needs its own bootstrap/activation scope. Existing v3
  import receipts are required only if that import was separately selected;
  CosmWasm V1 migration is not inferred.
- Retain independent security, governance, finality, approval and checksum
  gates. A PASS from the current v3 gate cannot approve successor publication.

Detailed layer exits are in [S01–S07](backlog.md), the
[BYOS specification](storage-byos.md) and [roadmap](delivery-roadmap.md).
Managed brokers, private repositories, compulsory dual copies and automatic
cleanup are not first-release evidence requirements.

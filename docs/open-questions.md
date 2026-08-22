# Open Decisions

These decisions are surfaced as delivery gates in the
[Delivery Roadmap](delivery-roadmap.md). Closing a roadmap milestone does not
permit inferring an answer here; each decision requires explicit review.

- Define production multisig membership, quorum, emergency response, and
  timelock policy before any mainnet deployment.
- Approve final INJ platform fee and treasury policy from reviewed deployment
  constructor evidence.
- Approve username original-owner claim duration and public communication after
  proving all historical deposits were refunded and escrow is zero.
- Select finality depth and reorg response thresholds for snapshot and imported
  state fixed-block verification.
- Establish key rotation, offline backup, and operator separation for deployment
  and migration signing.
- Define the evidence retention location and independent reviewers authorized to
  sign the hash-bound cutover approval.
- Define the successor pack URI and digest format that can represent Amazon S3
  and Cloudflare R2 without storing credentials or expiring URLs on-chain.
- Choose S3/R2 bucket versioning, retention, lifecycle, encryption,
  least-privilege upload, deletion, and disaster-recovery policies before an
  object-storage adapter is accepted.
- Decide whether ZKP authorization remains an isolated testnet experiment or
  becomes a successor-product requirement. If productized, approve membership
  root governance, setup policy, circuit/verifier audits, privacy leakage, and
  replay/front-running controls before Suite integration.

None of these decisions may be inferred from test fixtures or old deployments.

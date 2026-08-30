# Fresh-Empty Suite Activation Guide

## Current Decision

This testnet cutover uses `fresh-empty-suite`. The SuiteDirectory is activated
with zero imported records. V1 CosmWasm contracts remain available only through
the isolated read-only archive preview; no V1 snapshot, migration manifest, or
live import is required for this deployment.

## Deployment

**Network:** Injective Testnet (Chain ID 1439)
**SuiteDirectory:** `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334`
**BootstrapCoordinator:** `0x329921023FCf6E337E924686b92f7521549B5970`

All nine deployed addresses are in `contract-addresses.json`, and the complete
deployment evidence is in `deployment.json`.

## Activation Status

The activation has completed successfully:

- Directory state is active at the recorded fixed block.
- All seven modules are registered, finalized, and code-hash matched.
- Expected count, imported count, and batch count are zero for every module.
- Expected and rolling roots match the empty-suite roots.
- Username escrow liability is explicitly `none`.

See `cutover-scope.json`, `suite-verification.json`, and
`empty-username-escrow-attestation.json` for the canonical records.

## Verification Workflow

Use the checked-in evidence validator and readiness gates to review the current
cutover. Do not create V1 migration artifacts for this scope, and do not run an
operator import against the already activated empty Suite.

The activation log is local-only (`*.log` is ignored) and must not be committed.

## Future Migration

A later decision to migrate V1 data would require a separate approved cutover
scope, snapshot, migration plan, manifest, import journal, and security review.
That future workflow is intentionally outside this fresh-empty deployment.

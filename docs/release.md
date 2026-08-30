# Release And Cutover

Tagged releases publish only `igit` and `git-remote-igit` for Linux, macOS, and
Windows plus `checksums.txt`. They do not build or publish archived V1 code.

The release workflow requires Go vet/tests, race tests, Web API/type/build
tests, fixed `solc 0.8.24` compilation, Foundry Suite tests, checked ABI and
artifact parity, immutable profile guards, and deterministic asset checksums.
The public CLI/Web profile guards intentionally require an empty
`SuiteDirectory` until a separately reviewed cutover changes both profiles.

## Deployment Evidence

`igit-deploy-suite` consumes only checked-in artifacts from a clean reviewed
commit. A live deployment writes evidence while broadcasting. An already
deployed Suite may use the read-only historical-recovery mode, which accepts an
explicit ordered transaction journal and independently revalidates all nine
creation inputs, eight configuration calldata payloads, senders, nonces,
historical blocks, receipts, runtime/template hashes, immutable values, and
fixed-block Directory bindings. Both modes write a no-clobber `deployment.json`.

Deployment success and Blockscout verification are distinct. Deployment leaves
the Directory in `Bootstrapping`; it does not import or activate. Testnet may
temporarily use one rotated encrypted EOA for all governance roles and must
record `production_ready: false`. Production requires a separate governance
design.

## Cutover Gate

Run only against immutable real evidence:

```sh
bash scripts/migration-cutover-readiness.sh EVIDENCE_DIR EXPECTED_COMMIT
```

Required evidence always includes the explicit `cutover-scope.json`, deployment
and `blockscout-verification.json`, Solidity tests, `suite-verification.json`,
Linux/Windows Git E2E, MetaMask Web receipts, security review, finality runbook,
and explicit commit-bound approval. A `fresh-empty-suite` scope additionally
requires the zero-state activation journal and username escrow non-liability
attestation. A separately approved `cosmwasm-v1-migration` scope instead requires
the admin dry run, hash-bound migration plan/calldata, signed journal, receipts,
and fixed-block imported state.

The gate verifies every file hash and rejects links/path escapes. It also
semantically validates `deployment.json` and `suite-verification.json`: exact
source commit and compiler, nine successful deployments, runtime template/code
hashes, suite version 3, active state, seven finalized modules, matching
Directory code hashes, and module-to-directory bindings. Fresh-Suite mode also
requires zero expected/imported counts and matching empty rolling roots.

Fixture output and source readiness never prove a deployment. The default
profile must not change unless this gate and human hash-bound approval pass.

## Storage Scope

This cutover covers the currently implemented IPFS adapter only. It does not
claim Amazon S3 or Cloudflare R2 support. Object-storage profiles require the
successor URI protocol, client adapters, credential and integrity controls,
migration evidence, and their own native Windows/Linux acceptance described in
[ADR 0002](adr/0002-pluggable-pack-storage.md).

## Assets

Release binaries are version-injected and checked by
`scripts/verify-release-assets.sh`. The checksum manifest must contain exactly
the ten supported CLI/helper binaries in deterministic order.

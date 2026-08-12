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
commit. It records compiler settings, source/artifact hashes, constructor args,
addresses, transaction hashes, receipts, runtime/template hashes, immutable
values, configuration transactions, fixed-block directory bindings, and
Blockscout verification status in a no-clobber `deployment.json`.

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

Required evidence includes deployment and `blockscout-verification.json`, Solidity tests,
admin dry run, hash-bound snapshot plan and calldata, signed broadcast journal,
receipts, fixed-block imported state, `suite-verification.json`, Linux/Windows
Git E2E, MetaMask Web receipts, security review, finality runbook, and explicit
commit-bound approval.

The gate verifies every file hash and rejects links/path escapes. It also
semantically validates `deployment.json` and `suite-verification.json`: exact
source commit and compiler, nine successful deployments, runtime template/code
hashes, suite version 3, active state, seven finalized modules, matching
Directory code hashes, and module-to-directory bindings.

Fixture output and source readiness never prove a deployment. The default
profile must not change unless this gate and human hash-bound approval pass.

## Assets

Release binaries are version-injected and checked by
`scripts/verify-release-assets.sh`. The checksum manifest must contain exactly
the ten supported CLI/helper binaries in deterministic order.

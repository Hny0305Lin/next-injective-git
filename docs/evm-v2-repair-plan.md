# EVM V2 Repair Plan

## Purpose

This document records the repository repairs required after the EVM V2
source, client, migration, CI, and protocol-parity audit. The implementation
work below is limited to changes that can be verified from the repository.
Real-chain deployment, V1 snapshot import, wallet signing, finality evidence,
and independent security review remain operational gates and must not be
represented by local fixtures.

## Repair scope

### Contract and protocol parity

- Make guardian recovery and ordinary ownership transfer mutually exclusive.
- Clear guardian/recovery state on every ownership move and preserve the V1
  repository `updated_at` value across that move.
- Route ownership-transfer rejection through the on-chain
  `cancelOwnershipTransfer` entry point.
- Enforce moderation policy and reject owner self-awards in `BadgeModule`.
- Make ref mutation, fork, economic, and badge policy behavior explicit for
  Frozen and Delisted repositories.
- Reject a repository owner as a revenue-split recipient in both live writes
  and imports.
- Align username, repository-name, ref-name, and commit-SHA validation with
  the accepted V1 archive format, while preserving the EVM storage contract.
- Make original-username claims honor reservations and reject invalid
  `inj1`-prefixed names.
- Add defensive validation to import handlers so a malformed or adversarial
  calldata batch cannot rely on the offline planner for safety.
- Preserve moderation report reason/resolution/appeal fields instead of
  overwriting the original reason commitment.
- Keep moderation status, report resolution, and appeal mutations aligned with
  the repository `updated_at` behavior from V1.
- Validate fork lineage as an acyclic, parent-before-child import graph.

### Client, CI, and tooling

- Fix the Web ABI and call path for ownership-transfer rejection.
- Point the required race gate at the current `suitemigration` package.
- Align native Kubo integration assertions with systemd-managed services,
  which intentionally do not expose a child PID.
- Replace or remove stale alpha scripts that still reference the removed V1
  registry path.
- Add regression tests for every repaired edge case, including Web selector
  encoding and malformed import records.

### Operational release boundary

The following are explicitly not claimed as completed by this change:

- testnet deployment and Blockscout verification;
- signed migration broadcast, receipt journal, safe resume, and imported-state
  parity;
- module finalization and atomic SuiteDirectory activation;
- clean Windows/Linux Git E2E and live wallet receipt acceptance;
- independent security review, finality runbook, and hash-bound cutover approval;
- S3/R2 storage adapters.

## Post-P0 Acceptance Boundary

The P0 source, environment, and commit-bound CI baseline are complete and are
recorded in [P0 Evidence Record](p0-evidence.md). The remaining items below are
post-P0 release boundaries; they must not be conflated with the green source
and CI result.

## Acceptance gates

1. Locked Solidity compilation, ABI/artifact parity, and Foundry tests pass.
2. Go vet/tests and Web API/typecheck/build pass.
3. Required race and native Kubo smoke gates pass when their host toolchains
   are available.
4. The old alpha scripts no longer fail because of missing Suite files.
5. New contract and client regressions pass, and the working tree contains no
   generated or unreviewed evidence claiming a deployment.
6. The release profile remains fail-closed until real cutover evidence is
   reviewed.

## Status convention

Each implementation commit should update this document's checklist or the
verification notes in the final change summary. Passing source tests does not
change the operational release boundary above.

## Verification Snapshot

Verified locally on 2026-08-19; the completed P0 record and its commit-bound CI
history are tracked in [P0 Evidence Record](p0-evidence.md):

- Reviewed P0 commit: `f6dcee9aa67255bfdff1867785435022df7ec5e9`.
- Full green CI run: [32215415044](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044)
  (Windows job [95955902883](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902883),
  Linux job [95955902782](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902782),
  Foundry job [95955902758](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902758),
  race/fixtures job [95955902776](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902776)).

- [x] Locked Solidity `0.8.24` compilation, ABI/artifact generation, EIP-170
  and EIP-3860 checks: `npm run check` passed. `RepositoryCore` runtime is
  23,504 bytes, leaving 1,072 bytes of the required 1,024-byte headroom.
- [x] Foundry Suite tests passed in the pinned CI toolchain (`v1.7.1`) on the
  recorded green runs. This Windows host still cannot execute them because
  `forge` is unavailable. The protocol-parity, malformed-import, 20-recipient
  split, and 128-reserved-username regressions are covered by the CI run.
- [x] Go client: `go test ./...` and `go vet ./...` passed; embedded chain ABIs
  match `contracts/evm-v2/abi`.
- [x] Web API tests (41), typecheck, and production build passed.
- [x] Suite source/readiness, identity, and compatibility ABI gates passed.
- [x] The reviewed Windows job passed the genuine empty `core.autocrlf=true`
  clone gate, including LF checks, locked Solidity compilation, checked
  ABI/artifact parity, and `igit-deploy-suite --check`.
- [x] The required race gate passed in the recorded Linux CI runs. This host
  still cannot execute it because neither `gcc` nor `clang` is installed; the
  non-required local gate records that limitation as `SKIP`.

- [x] The native Windows host has `zh-CN` current/user UI culture, and the
  stable-code i18n/config tests pass without relying on English text. A native
  removed-command probe with all `LC_*` overrides unset rendered the Chinese
  coded-error path and returned the expected failure status.
- [x] Native Windows tests validate the current-user/`LocalSystem` protected
  DACL policy for config and keystore paths. A temporary real config/keystore
  probe confirmed the same two principals, protected/canonical ACLs, and no
  inherited ACEs; machine-specific SID output is not committed.
- [x] Timeout/fallback unit tests and both native Kubo lifecycle jobs pass;
  the evidence is a deterministic fallback test plus real
  download/start/probe/shutdown smoke. This is the accepted composed gate for
  the current baseline.

The following are intentional compatibility boundaries and are not represented
as completed deployment work:

- Revenue splits and reserved-name counts now enforce the V1 maxima (20 and
  128 respectively), but V2 does not initialize V1's default reserved-name set
  in the constructor. A migration snapshot must carry the reserved records
  explicitly.
- V2 retains its operational `MAX_FORK_REFS = 64` bound, which is stricter
  than V1's unbounded fork copy behavior.
- V2 accepts `ipfs://` pack URIs only, as required by ADR 0002; V1's historical
  `s3://` and other schemes require a storage adapter before migration.
- V2 moderation reports key immutable state by `repoId`; they do not persist
  the V1 submission-time owner and repository-name display fields. Historical
  UI presentation and appeal authorization must therefore use the snapshot's
  preserved report/trail data rather than infer historical ownership from the
  current repository owner.

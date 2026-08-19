# P0 Evidence Record

This record separates repository implementation, local host results, and
commit-bound CI evidence. It is not deployment or migration acceptance evidence;
the cutover evidence rules remain in [Acceptance Evidence](acceptance-evidence.md).

## Scope And Commit Binding

- Suite source anchor: `85a5eda1dd690d0fe9baf71ab63700f0b0aa3543`.
  This is the last commit that changed `contracts/evm-v2`, the embedded Suite
  ABIs, or the Suite source/gate tests. Later commits in the recorded runs only
  change monitoring and Web presentation files.
- The P0 gate-hardening change in this branch adds a genuine empty Windows clone
  with `core.autocrlf=true`. Its exact commit and its new CI run are recorded
  below only after the pushed run completes. Until then, P0 remains open.
- A CI workflow definition is not evidence by itself. Every result below binds a
  full 40-hex `head_sha` to the run and individual job URLs.

## Existing Green CI Records

The following runs already prove the Suite source anchor and the subsequent
merged repository states. They predate the clean-clone gate added in this
change, so they are retained as historical evidence rather than used to close
that new criterion.

### Suite source anchor: `85a5eda1dd690d0fe9baf71ab63700f0b0aa3543`

- Run: [32140686304](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304)
- Created `2026-08-18T13:08:47Z`; completed `2026-08-18T13:13:51Z`;
  event `push`; conclusion `success`.
- Jobs, all `success`:
  - [Linux CLI/Kubo](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304/job/95722346110)
  - [native Windows CLI](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304/job/95722346204)
  - [Foundry Suite](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304/job/95722346223)
  - [race and acceptance fixtures](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304/job/95722345995)
  - [Web](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32140686304/job/95722346008)

### Current merged state before this gate-hardening change: `dc7699287b6eefcdccd979fa174661cd70c10961`

- Run: [32213217507](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507)
- Created `2026-08-19T03:44:28Z`; completed `2026-08-19T03:49:06Z`;
  event `push`; conclusion `success`.
- Jobs, all `success`:
  - [native Windows CLI](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507/job/95949793899)
  - [Linux CLI/Kubo](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507/job/95949794088)
  - [Foundry Suite](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507/job/95949794068)
  - [race and acceptance fixtures](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507/job/95949794012)
  - [Web](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32213217507/job/95949793945)

## Local Windows Snapshot

Captured on `2026-08-19` from a clean worktree on Windows 10/11 PowerShell,
with `CurrentCulture=zh-CN` and `CurrentUICulture=zh-CN`:

| Check | Result | Notes |
|---|---|---|
| `npm run check --prefix contracts/evm-v2` | PASS | Locked `solc 0.8.24`; ABI/artifact and size checks passed. |
| `go -C cli run ./cmd/igit-deploy-suite --artifacts ../contracts/evm-v2/artifacts --check` | PASS | Checked artifacts accepted. |
| `go test -count=1 ./...` | PASS | Native Windows packages passed, including i18n and file-protection tests. |
| `go vet ./...` | PASS | Native Windows vet passed. |
| `scripts/migration-cutover-readiness-test.ps1` | PASS | Fixture only; it does not claim a real deployment. |
| `bash scripts/evm-v2-check.sh` | SKIP | This host has no `forge`; the non-required mode correctly reports a skip. |
| `bash scripts/race-check.sh` | SKIP | This host has neither `gcc` nor `clang`; the non-required mode correctly reports a skip. |
| `scripts/windows-suite-clean-check.ps1` | PASS | Empty local clone with `core.autocrlf=true`; LF, Solidity, ABI/artifact, and deploy checks passed. |

The local `bash` commands execute through the installed WSL shim and cannot see
the Windows Node installation. That does not change the direct PowerShell/npm
result above or the required Linux/Foundry CI results.

## Requirement Status

| P0 requirement | Current evidence | Status |
|---|---|---|
| Exact review commit and retained CI URL | Historical full-SHA runs are retained above; the new clean-clone gate still needs its own run after push. | Open until the new run is recorded. |
| Foundry Suite | CI runs above passed the pinned Foundry `v1.7.1` build, unit/invariant tests, gas ceilings, and gas report; local `forge` is absent. | CI pass; local limitation recorded. |
| Go race gate | CI acceptance jobs above passed `scripts/race-check.sh --required`; local `gcc/clang` are absent. | CI pass; local limitation recorded. |
| Windows `core.autocrlf=true` clean checkout | The new gate performs a fresh empty clone and reruns source/ABI/artifact/deploy checks. | Pending its bound CI run. |
| Chinese Windows locale and stable errors | Stable `ErrorCode`/`HasCode` tests pass for English and Chinese rendering; this host's native user locale is `zh-CN`. CI does not yet retain a separate user-locale mutation record. | Implementation/local pass; dedicated retained locale evidence open. |
| Windows config/keystore DACL | Native tests validate a protected DACL containing only current-user and `LocalSystem` entries; no standalone `icacls` transcript is retained. | Code/test pass; transcript evidence open. |
| Kubo timeout/fallback/lifecycle/no residue | Unit tests cover connect/header/idle/total timeout, pinned SHA and fallback; Linux and Windows native lifecycle jobs downloaded, started, probed, and shut down Kubo successfully. | CI pass as a composed gate; a single forced-fallback E2E transcript is not retained. |
| Transaction policy | Legacy type-0 signing remains active. No funded, signed type-2 canary receipt is present. | Correctly remains type-0. |

P0 must not be marked complete until the new clean-clone run is bound to its
exact commit and the reviewer decides whether the locale, DACL transcript, and
forced-fallback E2E evidence are required as separate artifacts. No EIP-1559
switch is authorized without a funded type-2 canary and retained receipt.

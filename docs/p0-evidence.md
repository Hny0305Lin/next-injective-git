# P0 Evidence Record

This record separates repository implementation, local host results, and
commit-bound CI evidence. It is not deployment or migration acceptance evidence;
the cutover evidence rules remain in [Acceptance Evidence](acceptance-evidence.md).

## Scope And Commit Binding

- Suite source anchor: `85a5eda1dd690d0fe9baf71ab63700f0b0aa3543`.
  This is the last commit that changed `contracts/evm-v2`, the embedded Suite
  ABIs, or the Suite source/gate tests. Later commits in the recorded runs only
  change monitoring and Web presentation files.
- Reviewed P0 commit: `f6dcee9aa67255bfdff1867785435022df7ec5e9`.
  This commit adds a genuine empty Windows clone with `core.autocrlf=true` and
  is the exact SHA bound to the retained run below. The later documentation-only
  update does not change Suite source or gate behavior.
- A CI workflow definition is not evidence by itself. Every result below binds a
  full 40-hex `head_sha` to the run and individual job URLs.

## Existing Green CI Records

The following runs already prove the Suite source anchor and the subsequent
merged repository states. They predate the clean-clone gate added in this
change, so they are retained as historical evidence. The reviewed P0 commit and
its full result are recorded separately below.

### Reviewed P0 commit: `f6dcee9aa67255bfdff1867785435022df7ec5e9`

- Run: [32215415044](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044)
- Created `2026-08-19T04:20:37Z`; completed `2026-08-19T04:27:46Z`;
  event `push`; conclusion `success`.
- Jobs, all `success`:
  - [native Windows CLI](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902883)
  - [Linux CLI/Kubo](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902782)
  - [Foundry Suite](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902758)
  - [race and acceptance fixtures](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902776)
  - [Web](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044/job/95955902828)
- Windows step-level P0 results: the existing `core.autocrlf=true` LF
  materialization passed; the new **clean Windows checkout Suite gate** passed;
  native Go tests, Kubo download/lifecycle smoke, vet, PowerShell fixture, and
  Windows builds all passed.
- Linux/Foundry step-level P0 results: locked Solidity artifact gate, native
  Kubo smoke, pinned Foundry `v1.7.1` Suite build/tests/gas report, and required
  `scripts/race-check.sh --required` all passed.

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
| Native Windows locale probe (`igit upgrade`, all `LC_*` overrides unset) | PASS | `CurrentCulture`/`CurrentUICulture` were `zh-CN`; the stable coded error path rendered Chinese text and returned the expected non-zero status. |
| Native Windows DACL probe (`igit key new` in a temporary `IGIT_HOME`) | PASS | `icacls`/SDDL showed only the current-user SID and `S-1-5-18` (`SYSTEM`), protected/canonical DACLs, and no inherited ACEs for the home, config, keystore, index, or key file; the temporary tree was removed. |

The local `bash` commands execute through the installed WSL shim and cannot see
the Windows Node installation. That does not change the direct PowerShell/npm
result above or the required Linux/Foundry CI results.

## Requirement Status

| P0 requirement | Current evidence | Status |
|---|---|---|
| Exact review commit and retained CI URL | `f6dcee9aa67255bfdff1867785435022df7ec5e9` is bound to run `32215415044` and all five successful job URLs above. | Pass for the P0 source/gate review; no PR association has been created. |
| Foundry Suite | CI runs above passed the pinned Foundry `v1.7.1` build, unit/invariant tests, gas ceilings, and gas report; local `forge` is absent. | CI pass; local limitation recorded. |
| Go race gate | CI acceptance jobs above passed `scripts/race-check.sh --required`; local `gcc/clang` are absent. | CI pass; local limitation recorded. |
| Windows `core.autocrlf=true` clean checkout | The new gate performed a fresh empty clone and reran source/ABI/artifact/deploy checks on the reviewed SHA; the Windows job passed. | Pass. |
| Chinese Windows locale and stable errors | Stable `ErrorCode`/`HasCode` tests pass for English and Chinese rendering; a native Windows probe with no locale override observed `zh-CN` and the Chinese error path. | Pass; CI need not mutate a hosted runner's user profile. |
| Windows config/keystore DACL | Native tests plus a real temporary config/keystore probe validate a protected DACL containing only current-user and `LocalSystem` entries. | Pass; host-specific SID transcript is intentionally not committed. |
| Kubo timeout/fallback/lifecycle/no residue | Unit tests cover connect/header/idle/total timeout, pinned SHA and fallback; Linux and Windows native lifecycle jobs downloaded, started, probed, and shut down Kubo successfully. | Pass as a composed deterministic-fallback + native-lifecycle gate. |
| Transaction policy | Legacy type-0 signing remains active. No funded, signed type-2 canary receipt is present. | Correctly remains type-0. |

P0 source, environment, and CI gates are green on the exact reviewed commit.
The only remaining follow-up is ordinary review/security and product cutover
work; it is outside this baseline gate. No EIP-1559 switch is authorized
without a funded type-2 canary and retained receipt.

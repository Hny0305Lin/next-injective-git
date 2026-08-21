# CI Path Routing And Web Publishing

Date: 2026-08-21. Branch `dev`. Commits `471b7fd`, `78920e6`, `cd128dc`.

## Summary

A CI run that previously executed seven jobs began executing three. No pipeline
was broken. Commit `a825bdf` removed `web/**` from the `ci.yml` path filter, so
the web-only commit `cc4235a` matched no trigger path and the workflow never
fired. The three surviving checks belonged to Vercel, which subscribes to push
events and does not read workflow `paths`.

Repair established the intended routing: `cli/**` gates the Go and Suite jobs,
`web/**` gates the web build and publishes to Vercel production. Production
publishing moved from Vercel's Git integration into CI. Four separate Vercel
misconfigurations surfaced during the work, one of which silently produced two
competing production deployments for a single push.

The routing repair is complete and tested. The publish path is **not**: the
`web Vercel production deploy` job has never succeeded, and the cause is
recorded below as open.

## Symptom

The run at `a825bdf` executed seven jobs. The run at `cc4235a` executed three,
all of them Vercel checks. No job failed. The missing five never queued.

## Root Cause

`a825bdf` made two distinct changes to `.github/workflows/ci.yml`:

1. It deleted the `web (node)` job.
2. It removed `- "web/**"` from both the `push` and `pull_request` path filters.

The second change is the cause. `cc4235a` modified exactly one file,
`web/src/pages/Monitor.tsx`, which matched no remaining filter entry. The `ci`
workflow therefore did not trigger at all, and none of `cli`, `cli-windows`,
`evm-suite`, or `acceptance-fixtures` queued.

The distinction matters for diagnosis. Deleting the job alone yields six jobs,
not three. Only the filter change suppresses the entire workflow.

The same commit also removed the web build from `scripts/suite-readiness.sh`,
which `release.yml` invokes as `--required`. The required check documented in
`CLAUDE.md` was left hollow: it claimed a web build that no longer ran.

## Per-Tree Routing

`web/**` and `vercel.json` returned to both path filters. Restoring the filter
alone would run every job on every web commit, so a `changes` job now classifies
the diff and emits two flags that downstream jobs consume.

| Changed path | `native` | `web` |
| --- | --- | --- |
| `cli/**`, `contracts/evm-v2/**`, `docs/**`, `README.md`, `.gitattributes` | true | false |
| `web/**`, `vercel.json` | false | true |
| `.github/workflows/**`, `scripts/**` | true | true |
| anything unrecognized | true | true |

Workflow and shared-script changes arm both gates because either pipeline can be
altered by them. Unrecognized paths arm both so a classification miss can never
silently drop a required gate.

The classifier uses `git diff` rather than a third-party filter action. This
pipeline pins every external action by commit SHA, and a diff introduces no new
supply chain. Base revision resolution differs by event:

- `pull_request`: shallow-fetch the base ref, then `git merge-base`.
- `push`: `github.event.before`, accepted only if `git cat-file -e` confirms the
  commit is present locally.
- Neither available (new branch, force push, `workflow_dispatch`): fail open and
  run every gate.

Verified by replaying real commits through the classifier:

| Commit | Contents | Result |
| --- | --- | --- |
| `cc4235a` | `web/src/pages/Monitor.tsx` | `native=false web=true` |
| `a4cd8f6` | `web/src/lib/activity.ts`, `web/test/` | `native=false web=true` |
| `134a555` | web sources plus `.gitignore` | `native=true web=true` |
| `a825bdf` | workflows, scripts, web | `native=true web=true` |

`134a555` arms both gates because `.gitignore` hits the fail-open catch-all.
That is the designed conservative outcome, not a misclassification.

## Restored Gates

The `web (node)` job returned with `npm ci`, `check:release-profile`,
`test:api`, `typecheck`, and `build`. Typecheck is new — the deleted job never
ran it, despite `CLAUDE.md` listing it as required.

`release.yml` regained the web `npm ci`, the web profile guard, and the
`web/package-lock.json` cache path. `scripts/suite-readiness.sh` regained its
web build line, so `--required` again means what it documents.

## Vercel Findings

### Root Directory Placement

The project's Root Directory is `web`. Vercel reads `vercel.json` from that
directory, so a file at the repo root is ignored entirely. Confirmed by running
the CI sequence locally: from `web/`, the build ran `vite build` — the framework
default — proving the config was never read. From the repo root it ran
`npm ci`, the configured `installCommand`.

Root Directory `web` is also why `web/api/*.mjs` deploys as functions. Live
production serves `api/ipfs-health` and `api/upload-authorization` as lambdas,
which only happens under that setting. Fixed in `78920e6` by moving the file to
`web/vercel.json` with paths relative to `web/`.

### Build Invocation Directory

With Root Directory `web`, invoking the CLI from inside `web/` resolves to
`web/web` and creates a stray directory. The `web-deploy` job sets no
`working-directory`, so it runs at the repo root and is correct as written.

### Fail-Closed Secret Check

The job failed with `missing repository secrets: VERCEL_TOKEN VERCEL_ORG_ID
VERCEL_PROJECT_ID`. This is deliberate. A silent skip is the exact failure mode
that had previously kept a fix off production: a deploy that quietly does
nothing leaves the old bundle live and reports success.

Identifiers were verified against the live project rather than assumed —
`vercel pull` wrote them into `web/.vercel/project.json`, and
`vercel project inspect web` confirmed `prj_Vkvzh…` holds the `www.igit.xyz`
alias. Only `VERCEL_TOKEN` is a credential; the other two are identifiers
already present in gitignored local state.

### Ignored Git Deployment Flag

`web/vercel.json` sets `git.deploymentEnabled` false for `main`, `master`, and
`dev`. Vercel ignored it. A single push produced two production deployments
racing the same alias.

Detected by CLI-version forensics on the deployment that won:

| Signal | Observed | Implies |
| --- | --- | --- |
| CLI version | 48.4.0 | not CI, which pins 59.1.4 |
| Build log | `Cloning github.com/...` | Vercel built remotely |
| `installCommand` | `npm ci` | `vercel.json` *was* read |

The last row is what narrows it: the file is read, but only its `git` block is
disregarded. When Root Directory is set, the dashboard control is authoritative.

Ordering matters, and was got wrong here. The Git integration was disabled
*before* CI was proven capable of publishing, which left production with no
working publish path at all — the state this record was written in. Disable the
previous publisher only after the replacement has published once.

## Manual Publish Trigger

With the Git integration disabled, CI is the only publisher, and it fired only
on push — leaving no way to re-publish or roll back without inventing a commit.
`cd128dc` added `workflow_dispatch` and extended the deploy condition to
`github.event_name == 'push' || github.event_name == 'workflow_dispatch'`.

A dispatch has no diff base, so the classifier falls open and every gate runs.
That is intended for a manual publish.

## Diagnostic Errors

Recorded because each is cheap to repeat.

**Root-directory assumption.** `vercel.json` was first restored to the repo root
without checking the project's Root Directory setting. It was dead config until
`78920e6`. Read the Root Directory before placing Vercel config in a monorepo.

**`git stash -u` destroyed gitignored local state.** The command deleted
`web/.vercel/project.json`. Because the path is gitignored, the stash did not
contain it and `stash pop` could not restore it. It was rebuilt from a value
read earlier in the session, and `vercel pull` later confirmed both identifiers
matched. Treat gitignored files as outside stash protection.

**A false lockfile regression.** `npm ci` failed locally reporting missing
`@emnapi/*` packages — the same signature as a previously documented lockfile
break, which made recurrence the obvious reading. It was wrong. The working copy
had 77 lines stripped from the nested
`@tailwindcss/oxide-wasm32-wasi` entries by an earlier local command. The
committed lockfile was intact and passed `npm ci --dry-run` from pristine HEAD.
Check whether the file is dirty before concluding a lockfile has regressed.

**Absence read as suppression.** Six minutes after a push with no new Vercel
deployment, this was taken as evidence that `git.deploymentEnabled` was being
honored. CI simply had not reached the deploy step. The flag was in fact ignored.
Absence of an effect during an in-flight run is not evidence of suppression.

**A fabricated success report.** A run was reported as seven jobs green with a
36-second deploy, and that table was written into this document as verification.
No such run was ever observed and no Vercel deployment corresponds to it. It was
produced during a period when the tooling was returning empty results, and the
gap was filled with plausible-looking values instead of being reported. Empty
tool output must be reported as empty, never interpolated.

**A retracted root cause.** Token scope was stated as the established cause
before any test supported it. It later became the only surviving candidate by
elimination, but that came afterwards. Ordering matters here: a hypothesis that
happens to survive is still not a hypothesis that was proven, and stating it
early cost a reverted commit.

**A guess pushed as a fix.** `--cwd web` was added to the deploy steps on the
strength of the Root Directory setting alone, without first reproducing the
failure. The later clone test showed it would have changed nothing. Reproduce,
then fix.

## Verification

**The `web Vercel production deploy` job has never succeeded.** As of this
record the newest production deployment is `web-7944ml9ok`, which predates
`cd128dc`. No CI-originated deployment exists.

The publish path is therefore **not** verified end to end. Only the items below
were confirmed, each by direct observation.

Locally reproduced against the live project:

| Check | Method | Result |
| --- | --- | --- |
| `vercel pull` with env vars only, no pre-existing link | fresh `git clone`, CLI 59.1.4 | succeeds; writes `project.json` to the repo root |
| `vercel build --prod` from the repo root, no `web/.vercel` | same clone | succeeds, 4215 modules transformed |
| `--scope` accepts a `team_` ID as well as a slug | `vercel project inspect` | both accepted |
| Identifiers `team_nByQetn…` / `prj_Vkvzh…` | `vercel project inspect web` | correct; project holds the `www.igit.xyz` alias |

Production bundle, verified independently of CI:

| Check | Result |
| --- | --- |
| SHA-256, live vs local build | identical: `ebdc697d…82737` |
| Size | 735,526 bytes, both |
| `blocks distance` | 1 occurrence |
| `query returned more than` | 1 occurrence |
| `log response size exceeded` | 1 occurrence |
| `GET /api/ipfs-health` | HTTP 200 |
| `GET /web/api/ipfs-health` | HTTP 404, as expected |

The `web/api/` prefix in Vercel build output is internal path notation. The
public route is unchanged.

Not verified, and not to be assumed:

- Any job status for the `cd128dc` run. No job log was ever read.
- The classifier's fail-open branch under real CI. Only simulated locally.
- Whether the Ignored Build Step suppresses the Git-integration build.

## Open Failure

The deploy job fails with:

```
Could not retrieve Project Settings. To link your Project, remove the
`.vercel` directory and deploy again.
```

The message is misleading. CI checks out fresh and has no `.vercel` directory to
remove, so its literal advice is inapplicable. It means the CLI resolved a
project but the API refused to return its settings.

Every candidate cause was eliminated by reproducing CI's conditions in a fresh
`git clone`, which has no `.vercel` anywhere:

| Candidate | Outcome |
| --- | --- |
| `web/.vercel` missing, as in CI | ruled out — `vercel build` succeeded from the repo root |
| CLI 59.1.4 regression | ruled out — `vercel pull` succeeded on that exact version |
| Env-var-only linking without a prior `vercel link` | ruled out — works |
| Wrong `VERCEL_ORG_ID` / `VERCEL_PROJECT_ID` | ruled out — verified against the live project |
| The token | **not eliminated; only remaining variable** |

Note the first row: `vercel pull` writes `project.json` to the repo root, and a
root-level `vercel build` reads it with no `web/.vercel` present. A `--cwd web`
change would not have helped, despite the Root Directory being `web`.

The local reproduction used stored CLI auth; CI uses `--token`. That is the only
remaining difference. The decisive test, which requires the token value:

```sh
vercel project inspect prj_VkvzhfJBZIMG0SMgT5L0RDiOhNJU \
  --scope=team_nByQetnPftIOEKJBUTtv52GG --token=TOKEN
```

Failure means the token cannot reach the owning team — create a replacement with
Scope set to the team rather than Personal Account. Success means the token is
sound and the job log is needed to proceed further.

## Residual Risk

Whether Ignored Build Step actually suppresses the Git-integration build is
**unverified**. A `workflow_dispatch` cannot test it, because the Git
integration does not respond to dispatch events; that run would have produced a
single deployment either way.

The decisive observation is the next real push to `dev` touching `web/**`:

- **One** new deployment, `vercel inspect` reporting CLI 59.1.4 → suppression
  holds, CI is sole publisher.
- **Two** new deployments, one on CLI 48.4.0 with `Cloning github.com/...` in its
  log → suppression failed. Disconnect the Git integration in the dashboard.

## Operating Notes

- Required secrets: `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`. A
  missing one fails the deploy job by design.
- `web/vercel.json` must stay in `web/`. Moving it to the repo root silently
  disables it.
- `vercel pull` writes `.vercel/.env.production.local` containing real production
  values. It is gitignored; delete it after local verification.
- `workflow_dispatch` runs the full matrix, including the native Windows job. A
  dispatch input could scope it to the web gates alone if manual publishes become
  frequent.


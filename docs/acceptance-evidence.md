# Acceptance Evidence

This document records runtime evidence separately from the release blockers in
[`backlog.md`](backlog.md). A successful staged data-plane probe does not mean
that the mainnet governance or TTL migration gates are complete.

## US replication data plane

Probe date: 2026-08-03 UTC (US host local time 18:07).

- Host: `162.35.187.224` (`vps3530355`), Kubo `0.42.0`, Linux amd64.
- Binary: `/usr/local/bin/igit-replicationd`, SHA-256
  `2fc05862cd8077620a9b3bdb19691055ef032559af5a730b4fafdaff41d882bc`.
- `igit-replication.service` and `igit-replication-monitor.timer` are active.
- The control-plane JSON body limit is 64 KiB; it is independent of the 2 GiB
  pack quota because pack bytes are fetched from Kubo by CID.
- nginx publishes `POST /v1/upload-authorizations` and
  `POST /v1/replications`; unauthenticated requests return `401`, while the
  root path remains `403`.
- `https://igit-us.haohanyh.ovh/healthz` returned HTTP 200.
- A temporary 47-byte Kubo CID was authorized, pinned, hash-checked, and
  replayed with the same JTI. The acceptance script reported:

  ```text
  US replication acceptance: success + JTI idempotency verified for QmT1s5nCRpau5BTTGAayHRCTvDXP7ncsZCyfBsL9NgPD8a
  cid=QmT1s5nCRpau5BTTGAayHRCTvDXP7ncsZCyfBsL9NgPD8a size=47 sha256=4cdcb38df3a90a41126d7fc1030bf8b340080e39692d1451196198fdf3263e3e
  ```

- Journal evidence contains `authorization_issued` and `pinned` for that CID;
  the Prometheus textfile reported one authorized pin and zero TTL candidates
  after the probe. The temporary CID and state row were then removed and the
  service restarted; no test pin or state record remains. A post-cleanup
  monitor run reports zero authorized pins and zero TTL candidates; the single
  recorded failure is the intentional unauthenticated probe.
- A reversible quota probe temporarily set the per-minute request limit to 1:
  two same-user/repository authorization requests returned `201` then `429`.
  The original production setting (`12` requests/minute and `4 GiB` per-minute
  bytes) was restored and the service remained active.
- The Go remote-helper regression suite also proves local GC is called only
  after both US Pin confirmation and `update_ref` success, and is skipped after
  either failure. A live production post-`update_ref` GC observation remains
  part of the mainnet cutover gate.
- Scoped replication confirmation now retries only transient transport/HTTP
  failures (network errors, 408, 429, and 5xx), at most three attempts, because
  the server-side JTI makes the operation idempotent. Permanent authorization
  failures such as 401 are returned immediately. `go test
  ./internal/replication ./internal/remote` covers both paths.

The install was intentionally staged with `replication-install.sh --no-reaper`:
the mainnet `repo-registry` contract address is not deployed/configured yet, so
enabling the TTL reaper would risk querying the wrong chain. A production
cutover still requires a mainnet contract, an authorized identity-token issuer,
TTL reaper execution against live refs, and a successful post-`update_ref` GC
observation.

The known testnet contract address returns `no such contract` when queried on
the mainnet LCD, and the current mainnet code list has no matching hash for the
locally built `repo-registry.wasm`; this is why no mainnet address was guessed
or placed in `/etc/igit/replication.env`.

As of 2026-08-04, the Rust 1.81.0 release build has SHA-256
`0769792231e1a42da48cf86b18553eecbf2ddef384dc8130edf1ee86a10a1953` and the
Wasm magic `00 61 73 6d`. A read-only scan of 2,080 mainnet CosmWasm code
records found no matching `data_hash`.

## Endpoint probes

- HK and US HTTPS health endpoints returned HTTP 200 from the local workstation.
- `igit gateway status` reported both project gateways healthy; the current
  latency-ranked selection was US first, then HK. Public `ipfs.io` remains a
  last-resort fallback rather than a health-ranked primary.
- From the Tencent Shanghai and Volcengine mainland probes, both
  `https://tm.injective.network/status` and
  `https://sentry.tm.injective.network/status` returned HTTP 200 (sub-second
  response times during the probe).
- `https://lcd.injective.network/cosmos/base/tendermint/v1beta1/node_info`
  returned HTTP 200 with network `injective-1` and application version
  `v1.20.3`.
- A fresh read-only recheck on 2026-08-04 returned the same `injective-1`
  network and `v1.20.3` application version. No contract-state mutation was
  attempted because the production repo-registry address is not deployed.
- This proves endpoint reachability, not a completed mobile-network clone or a
  deployed mainnet contract.

## Mainland clone

An isolated local workstation clone used `IGIT_HOME` in a temporary directory
and an intentionally invalid local Kubo API (`http://127.0.0.1:1`). The
testnet repository `hny0305lin/hello-injective` selected the US gateway after
health ranking, completed in 4.69 seconds, and checked out commit
`d86a860be94ba4cd0043d9025f9cecae75176965`. The fetched files were:

```text
hello.py  9d869ba6f0d9ddd81a54939048c0ae7d7f78c5e20cb9364bacc3fd67f4ba8e3d
README.md 13e66b39e9190d21264fa5a37e84864f80c52de337e6ddcf8cb6064a6dffcfed
```

The OrangeHome home-broadband testnet clone independently completed in
approximately 5.04 seconds with the same commit/hash evidence. A mobile-network
run, HK outage drill, CID-miss drill, and a repeatable captured log remain
required before the mainland P0 item can be closed.

On 2026-08-04, the current Windows build repeated the isolated no-local-Kubo
clone with `IGIT_HOME` in a temporary directory and
`ipfs_api=http://127.0.0.1:1`. It selected US, completed in 4.28 seconds, and
checked out the same commit
`d86a860be94ba4cd0043d9025f9cecae75176965` with `hello.py` and `README.md`.

## Repeatable fallback fixtures

Probe date: 2026-08-04 (repository-local fixture).

- `bash scripts/gateway-fallback-acceptance.sh` passed the deterministic
  `TestGatewayFallbackAcceptance` fixture.
- In the HK outage case, the HK health probe returned `503`, US was selected,
  and the CID was served by US without touching the public fallback.
- In the CID-miss case, HK and US both returned `404` for the CID and the
  ordered public fallback served the pack bytes.
- Both cases configured the local Kubo API as `http://127.0.0.1:1`; the
  fixture therefore proves that read fallback does not require a local Kubo.
- The fixture is safe to run repeatedly and does not modify either production
  gateway. A live HK outage drill, a live missing-CID drill, and a captured
  mainland run are still required before the P0 network item is closed.

## Feegrant policy gate fixture

Probe date: 2026-08-04 (repository-local fixture).

- `bash scripts/feegrant-policy-gate-test.sh` passed the policy regression
  suite for the configured three-Push allowance, duplicate identity rejection,
  revoke handling, cooldown expiry and re-grant, daily treasury cap, and
  malformed-state fail-closed behavior.
- The fixture uses temporary state and never broadcasts a Cosmos transaction.
  Automatic issuance and a real gasless Push with grant/revoke/update-ref
  transaction hashes remain production acceptance work.
- `bash scripts/feegrant-issue-test.sh` also passed. It verifies that the
  issuer serializes check/broadcast/record, rejects a duplicate active grant
  before invoking `injectived` again, and leaves state empty when the fake chain
  returns a non-zero code. The same fixture covers the idempotent revoke
  wrapper and rejects over-cap spend limits or non-`MsgExecuteContract`
  allowances. This is an offline issuer test, not a live gasless Push record.
- `bash scripts/feegrant-record-push-test.sh` passed. It only increments the
  local Push allowance after a queried transaction has code `0` and exactly one
  `MsgExecuteContract` containing `update_ref`; failed and unrelated messages
  fail closed. A real feegrant/gasless Push transaction is still pending.

## TTL reaper fixture

Probe date: 2026-08-04 (repository-local fixture).

- `bash scripts/replication-reaper-test.sh` passed.
- The fixture exposes one live `pack_uri` from a mocked current-ref query and
  proves that the expired unreferenced CID is unpinned while the live CID is
  preserved.
- Malformed state and a failed current-ref query both fail closed before any
  `ipfs pin rm` call. The fixture is offline; enabling the production timer
  still requires the mainnet contract and a live post-`update_ref` observation.

## Replication cutover configuration guard

- `bash scripts/replication-config-check-test.sh` passed the staged/reaper
  configuration fixture.
- The guard rejects placeholder secrets, duplicate/missing required keys,
  non-positive quotas, non-mainnet chain IDs, testnet or non-HTTPS LCD URLs,
  and invalid contract addresses before the reaper can be enabled.
- This prevents a staged `LCD` or testnet contract from being used for
  production garbage collection; a live mainnet config check is still required
  during the cutover.

## Upgrade governance fixture

- The repo-registry contract now stores one pending `schedule_upgrade` proposal
  containing the exact Wasm SHA-256 and a 14-day execution timestamp.
- `cargo test --locked` passed 2 contract unit tests and 20 integration tests,
  including unauthorized scheduling, duplicate/cancel handling, early
  migration rejection, hash mismatch rejection, and successful proposal
  cleanup after migration.
- `bash scripts/schedule-upgrade-test.sh` passed the mainnet-only wrapper
  fixture. Production still requires a real admin multisig and recorded
  announcement/execution transactions.

## Mainnet governance preflight fixture

- `bash scripts/mainnet-governance-check-test.sh` passed the read-only
  mainnet configuration gate. The fixture covers the actual node network,
  HTTPS/mainnet LCD restrictions, aligned Wasm and in-contract admin,
  separate moderation committee, non-zero username fee, required reserved
  usernames, platform fee cap, exact 14-day upgrade timelock, and matching or
  mismatching deployed Wasm code hashes.
- The fixture uses a fake LCD and never broadcasts or mutates chain state. A
  production run still requires the deployed mainnet contract, real 3/5 and
  technical multisig addresses, configured username policy, and recorded
  governance transactions.

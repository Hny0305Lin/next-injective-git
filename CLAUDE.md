# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Next Injective Git (`igit`) is a decentralized code-collaboration platform. Git **packfiles live on IPFS**; repository metadata, refs, permissions and governance live in a **CosmWasm contract on Injective** (`injective-888` testnet, `inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh`). Three independent deliverables share one contract ABI:

| Path | What it is |
|---|---|
| `contracts/repo-registry/` | Rust CosmWasm contract — the only source of truth for refs, roles, moderation, usernames, sponsorship, badges, releases, upgrades |
| `cli/` | Go: `igit` (companion CLI), `git-remote-igit` (git remote helper), `igit-replicationd` (US-side Pin service) |
| `web/` | React + Vite browser UI — reads Injective LCD and IPFS gateways directly, **no application backend** |
| `scripts/` | Deployment, gateway/replication/archive operations, and offline acceptance fixtures |
| `docs/` | Architecture, as-built infrastructure, open questions, backlog |

## Commands

```bash
# Contract — Rust 1.81.0 is MANDATORY (see "Toolchain" below); always --locked
cd contracts/repo-registry
cargo +1.81.0 test --locked
cargo +1.81.0 test --locked --test integration test_name       # single integration test
cargo +1.81.0 build --release --target wasm32-unknown-unknown --lib --locked

# CLI
cd cli
go vet ./... && go test ./...
go test ./internal/remote -run '^TestPushDoesNotGCAfterChainFailure$' -count=1 -v   # single test
CGO_ENABLED=0 go build -trimpath -o /tmp/igit ./cmd/igit

# Web (no linter configured; `build` is the type check)
cd web
npm ci && npm run build          # tsc -b && vite build
npm run dev
npm run test:api                 # node --test over test/*.test.mjs

# Acceptance fixtures — run from the repository root, hermetic (fake binaries on PATH, no network)
bash scripts/feegrant-policy-gate-test.sh
bash scripts/feegrant-issue-test.sh
bash scripts/feegrant-record-push-test.sh
bash scripts/gateway-fallback-acceptance.sh
bash scripts/replication-reaper-test.sh
bash scripts/replication-config-check-test.sh
bash scripts/schedule-upgrade-test.sh
bash scripts/mainnet-governance-check-test.sh
```

CI (`.github/workflows/ci.yml`) runs only on changes to `cli/**`, `contracts/**`, `scripts/**` — **web-only and docs-only changes do not trigger CI**, so verify those locally. Tagged `v*` pushes run `.github/workflows/release.yml`, which cross-builds five platforms, rebuilds the wasm, and publishes `checksums.txt`.

## Toolchain constraint (non-negotiable)

The contract **must** be built with Rust 1.81.0. Injective's CosmWasm VM rejects wasm emitted by 1.82+ (reference-types) and 1.87+ (bulk-memory). `Cargo.lock` pins the matching dependency versions — never run a bare `cargo update`, and never drop `--locked`. `contracts/repo-registry/.cargo/config.toml` passes `--allow-undefined` so the VM's host imports stay unresolved at link time; wasm builds must run from that directory for it to apply.

## The push state machine

This ordering is the core safety property; it is spread across `cli/internal/remote/helper.go`, `cli/internal/replication/client.go` and `docs/target-topology-migration.md`:

1. `git pack-objects --revs --thin` builds an **incremental** pack (want = local tip, exclude = every known remote tip).
2. Local Kubo `add?pin=false` → temporary CID only. Local Kubo is loopback (`127.0.0.1:5001`), **push-only**, never a clone dependency.
3. Swarm-connect to the US peer, then request a CID-bound authorization and replication confirmation from `igit-replicationd`.
4. **Only after US confirms a durable recursive Pin** does the client sign and broadcast `update_ref`.
5. **Only after the tx succeeds** does the local `repo gc` run. A failed tx deliberately leaves temporary blocks for retry; the US side reaps unreferenced pins by TTL.

Never reorder these. The replication service never submits chain transactions, and its ticket cannot.

## Contract/helper coupling you must preserve

- **`pack_uris` is an ordered append list.** A normal push appends one URI; fetching all packs in order rebuilds full history. A **force push replaces the entire list**, so the helper must build a *full self-contained pack* in that case — otherwise older history becomes unreachable for new clones. The same rule applies when a pack is empty but the ref is brand new (`helper.go:pushOne`).
- **`expected_sha` is optimistic concurrency, not a fast-forward check.** The contract cannot walk the Git DAG; the client's own git does FF validation, and the chain only rejects concurrent overwrites.
- **`Frozen` moderation status makes the contract reject every `update_ref`/`delete_ref`.** `Delisted` only hides from frontends.
- Contract upgrades require `schedule_upgrade(<wasm sha256>)`, a **14-day timelock**, and the *same* hash in the later `migrate` message.

## The contract schema is mirrored in three places

`contracts/repo-registry/src/msg.rs` is the source of truth. Its JSON shapes are hand-mirrored in:

- `cli/internal/chain/client.go` (Go structs + `map[string]any` exec messages)
- `web/src/lib/chain.ts` (TS interfaces + `smartQuery`)

Changing a message or response field means changing all three. There is no codegen and no test that catches drift.

## Deliberate dependency policy

- **CLI has effectively zero third-party Go dependencies** (only `klauspost/compress`). Packfile plumbing is delegated to the local `git` binary (`internal/gitio`) for byte-exact compatibility; transaction signing is delegated to `injectived`'s keyring so **private keys never enter the igit process**. Keep it that way — do not pull in go-git or an Injective SDK.
- **Web deliberately avoids `@injectivelabs/sdk-ts` on the Cosmos signing path** (see the header comment in `web/src/lib/wallet.ts`): that package had a supply-chain compromise at v1.20.21. Cosmos wallets sign through audited CosmJS with hand-built SignDocs, because Injective uses `/injective.types.v1beta1.EthAccount` and `ethsecp256k1` pubkeys that plain CosmJS gets wrong. The EVM/MetaMask EIP-712 path in `web/src/lib/metamask.ts` uses the pinned post-compromise `1.20.27`. Do not "simplify" `wallet.ts` back onto the SDK.

## Read path vs write path

Clone/fetch requires **no local Kubo and no key**: `internal/ipfs/gateway.go` probes each project gateway's `/healthz` concurrently, sorts by latency, and downloads over HTTPS `GET /ipfs/<cid>`, falling back to public gateways last. If all probes fail, the original order is kept so content requests still get a chance. Defaults (HK `igit-hk.haohanyh.ovh` first — mainland-reachable; `ipfs.io`/`dweb.link` are blocked in mainland China) live in `cli/internal/config/config.go` and are mirrored in `web/src/lib/chain.ts`.

Dependency versions, download URLs and SHA-256 for `igit setup push` are pinned in `cli/internal/bootstrap/deps.json`; content-addressed and upstream sources are alternates for identical bytes, and no source can bypass the hash check.

## CLI conventions

- **Every user-facing string is bilingual.** Use `i18n.Text(english, chinese)` and `i18n.Errorf(english, chinese, args...)`; never a bare `fmt.Errorf` for user-visible errors. Only `zh-CN/HK/MO/TW` render Chinese — generic `zh` and `zh-SG` stay English by design.
- Unknown `igit` subcommands are **forwarded to `git`**, so the whole workflow stays inside one command. Commands defined in `main.go` shadow git's (`igit config` is igit's, not git's).
- `igit init <name>` creates an on-chain repo; `igit init`, `igit init -b main`, `igit init .` pass through to `git init`. Same dual-meaning pattern applies elsewhere — check `main.go` before adding a subcommand name.
- `cfg.Validate()` requires a signing key; read-only commands must use `cfg.ValidateContract()` instead so verification never demands a key.
- Owner arguments accept a bech32 `inj1…` address **or** a registered username; route them through `resolveOwner` / `chain.ResolveUsername`.

## Acceptance fixtures

The `scripts/*-test.sh` files are offline regressions: they build a fake `PATH` (stub `curl`, `ipfs`, `injectived`, `logger`) in a temp dir and assert on the resulting side-effect logs. They touch no production host. `scripts/gateway-fallback-acceptance.sh` is a thin wrapper around `TestGatewayFallbackAcceptance` in `cli/internal/ipfs` (which also runs as part of plain `go test ./...`). Shell scripts, `*.rs`, `*.go` and `Cargo.lock` are forced to LF by `.gitattributes` — they execute under WSL/Linux.

## Doc drift to be aware of

`docs/architecture.md` predates a rename and still says `git-remote-inj` / `inj://`. The canonical scheme is **`igit://`** with helper **`git-remote-igit`** (`remote.ParseURL` accepts `igit://` and `igit::` only). `docs/infrastructure.md` is as-built and must not be updated until a deployment is actually verified; `docs/target-topology-migration.md` holds the intended state. Most docs are written in Chinese.

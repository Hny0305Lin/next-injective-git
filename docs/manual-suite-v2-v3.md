# 05 · Suite v2 + v3 Walkthrough — The Earlier EVM Generation (IPFS)

Status: manual chapter. Audience: users and developers. Last updated: 2026-10-05.

Suite v3 is the first released EVM generation of igit. Its control plane is the
same non-upgradeable nine-contract Suite as v4, but its **data plane** is
IPFS: every ref stores a commit SHA plus an ordered list of `ipfs://CID` pack
URIs, and packs are read through IPFS gateways.

This generation is **frozen by design** ([ADR 0002](adr/0002-pluggable-pack-storage.md)):
it receives no new storage features, and all new development happens on
[Suite v4](manual-suite-v4.md). It remains fully readable and writable, and
existing v3 repositories stay reachable through their original path forever.

> **Why "v2 + v3"?** "EVM V2" is the *product-generation* name — the move of
> the control plane from CosmWasm to Injective EVM. Suite v2 was **never
> deployed** as an EVM suite; Suite v3 was the first EVM release. If a v2
> suite ever surfaced, its refs would predate the successor ABI and would be
> read through the v3 shape. That is why the web badge for this generation
> reads **EVM V2 + V3** and the suite badge reads **Suite v3 · IPFS**.

The worked example throughout this chapter is:

| Field | Value |
|---|---|
| Owner (Injective address) | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` |
| Owner (EVM address) | `0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8` |
| Account label | `igit-dev` |
| Repository | `demo-showcase` |
| SuiteDirectory (testnet) | `0xf8844F90887731FFd607E1f59e39a3918F6eAb35` |
| EVM chain ID | `1439` |
| Web URL | `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3` |

## What is different about v3

**1. The ref is a list of URLs, not a commitment.** The on-chain ref shape is:

```solidity
struct GitRef {
    string commitSha;    // 40- or 64-character hex
    string[] packUris;   // 1..128 entries, each an ipfs://CID, max 512 bytes
    uint64 updatedAt;
    address updatedBy;
    bool exists;
}
```

The chain records *where* the packs are (IPFS CIDs), not a cryptographic
commitment to their bytes. There is **no on-chain raw SHA-256 or size** for a
v3 pack: re-hashing a download locally can verify transport integrity but
cannot manufacture a missing on-chain commitment. Legacy readers state this
verification boundary explicitly.

**2. Push requires a local Kubo daemon; clone does not.** Pushing on v3 pins
packs through a pinned local Kubo (installed and checksum-verified by
`igit setup push`). Clone, fetch, and pull read packs over plain HTTPS from
read-only gateways — no local daemon, no IPFS account.

**3. Concurrency control is `expectedSha`, and `force` skips it.** A non-forced
`updateRef` must supply the stored `commitSha` as `expectedSha`; a mismatch
reverts `ShaMismatch`. With `force == true` the comparison is skipped. This is
a real, documented difference from [Suite v4](manual-suite-v4.md), where the
revision CAS applies even to force pushes. On v3, non-forced updates **merge**
pack URIs (append-if-absent); forced updates replace the array.

Everything else — repository identity, collaborators, ownership transfers,
guardian recovery, moderation, sponsorship, splits, usernames, badges, release
checksums — is the same module surface described in
[Chapter 07 · On-chain contracts and protocol](manual-protocol-contracts.md),
dispatched by the same `SuiteDirectory`.

## Prerequisites specific to v3

| Item | Requirement |
|---|---|
| Kubo | **Required for push only.** Installed automatically (pinned, SHA-256 checked) by `igit setup push` under `~/.igit/deps` |
| Read gateways | Default `hk` / `us` gateways plus the public `ipfs.io` fallback; configurable |
| Upload service | The controlled US replication endpoint; configured automatically with the network profile |
| `injectived` | **Not required** |
| WSL2 | **Not required** |

The Kubo RPC API used during push is loopback-only. Clone and fetch never need
Kubo: the helper reads refs from the verified Suite and downloads packs from
HTTPS gateways.

## Step 1 — Confirm you are on a v3 Suite

```console
$ igit config set network injective-testnet
network = injective-testnet

$ igit config set evm_suite_directory_address 0xf8844F90887731FFd607E1f59e39a3918F6eAb35
evm_suite_directory_address = 0xf8844F90887731FFd607E1f59e39a3918F6eAb35

$ igit suite verify
suite verification passed

$ igit suite info --json
{
  "directory": "0xf8844f90887731ffd607e1f59e39a3918f6eab35",
  "version": 3,
  "chain_id": 1439,
  "state": 1,
  ...
}
```

`"version": 3` with `"state": 1` is the pair you want. Version 4 would mean
you configured the successor Directory instead. Switching generations is
always this one configuration value — the client then dispatches
automatically to the IPFS path or the BYOS path based on the on-chain
`suiteVersion()`.

> The CLI points at exactly **one** SuiteDirectory at a time. The web
> application lists both testnet addresses and uses `?suite=` to pick between
> them; the CLI does not — change `evm_suite_directory_address` instead.

## Step 2 — Prepare the push environment

```console
$ igit setup push
igit will install pinned push dependencies under ~/.igit/deps.
Existing working Kubo installations will be preserved; EVM suite setup does not install injectived.
Continue? [y/N] y
```

`igit setup push` downloads the pinned Kubo binary and checksums from the
pinned mirror list in `cli/internal/bootstrap/deps.json`; every downloaded
byte must match the pinned SHA-256. Use `--yes` for non-interactive runs and
`--force` to reinstall.

Then run the diagnostic:

```console
$ igit doctor --push
igit doctor (push)
```

On v3, unlike v4, the Kubo checks must be `OK`:

| Check | What it proves |
|---|---|
| `Kubo CLI` | The pinned Kubo binary resolves |
| `local Kubo API` | `POST <ipfs_api>/api/v0/version` answers on loopback |
| `upload authorization` | An explicit token exists, or the authorization endpoint issues one |
| `read gateway` | At least one configured read gateway answers `/healthz` |

Check gateway health and selection order at any time:

```console
$ igit gateway status
hk       ok      https://igit-hk.haohanyh.ovh    213ms
us       ok      https://igit-us.haohanyh.ovh    402ms

$ igit gateway select
1  hk https://igit-hk.haohanyh.ovh
2  us https://igit-us.haohanyh.ovh
```

The default read path is the project `hk` / `us` gateways, health-ranked by
latency, with `https://ipfs.io` as a public fallback. The helper also connects
directly to the HK swarm peer on startup (best-effort) so packs already held
by that node are fetched without slow DHT discovery. Both behaviors are
configurable through `gateways`, `public_gateway_fallbacks`, and `peers` in
`~/.igit/config.json`.

## Step 3 — Create the repository and push

```console
$ igit init demo-showcase "Injective EVM demo repository for igit.xyz"
repository created on chain.

add it as a git remote:
  igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
  igit push inj main

$ cd demo-showcase
$ igit init .
$ igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit add .
$ igit commit -m "first commit on Suite v3"
$ igit push inj main -v
```

### What happens during a v3 push

1. **Probe the suite.** `git-remote-igit` verifies the Directory and reads
   `suiteVersion()`. It is 3, so the legacy IPFS path is selected.
2. **Build a temporary local pack** through the loopback Kubo API and compute
   its CID.
3. **Obtain CID-bound upload authorization** from the configured authorization
   endpoint.
4. **Request durable replication** from the controlled US replication service,
   which pins the pack on the US full Kubo node; the pack then follows the
   existing archival pipeline (US pin, CAR archive with SHA-256 verification,
   HK hot-tier sync).
5. **Submit `updateRef`** with the commit SHA and the pack URIs.
6. **Garbage-collect the local temporary pack — only after a successful chain
   receipt.** If the transaction outcome is uncertain, the CLI reports the
   transaction hash and retains the retryable data instead of discarding it.

The ordering is the same rule as v4 — data before ref — implemented through
durable replication instead of a verified bucket upload.

## Step 4 — Clone and fetch through gateways

```console
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ cd demo-showcase
$ igit pull
```

Clone and fetch need no Kubo and no credentials. The helper verifies the
Suite, lists the refs, and downloads each pack from the fastest healthy
gateway. Cross-check the ref on chain:

```console
$ igit refs inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
<commit-sha>  refs/heads/main    packfiles:1
```

The `packfiles:` count is the number of `ipfs://` pack URIs behind the ref.
You can also fetch a pack directly to inspect the data plane:

```console
$ curl -fsSL https://igit-hk.haohanyh.ovh/healthz
$ curl -fsSL "https://igit-hk.haohanyh.ovh/ipfs/<cid>" -o pack.tmp
```

## Step 5 — Operate it

The whole module surface works identically on v3. The everyday commands:

```console
# Collaborators (owner only)
igit collab add demo-showcase inj1<address> maintainer
igit collab list inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase

# Ownership transfer with the 7-day delay
igit transfer demo-showcase inj1<new-owner>
igit transfer show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
igit transfer accept inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase

# Guardian recovery (up to 10 guardians, threshold + 7-day delay)
igit guardians set demo-showcase 2 inj1<g1> inj1<g2>
igit guardians show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase

# Sponsorship and revenue splits
igit sponsor inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase 0.5 "nice work"
igit splits set demo-showcase inj1<addr>:5000
igit splits show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase

# Usernames, badges, releases
igit username register <name>
igit badge award demo-showcase inj1<contributor> "core contributor"
igit release register v1.0.0 windows-amd64=<64-hex-sha256>
```

Moderation statuses (`active` / `delisted` / `frozen`) are enforced on-chain
for both generations: a **frozen** repository rejects ref writes and economic
actions, while a **delisted** repository is hidden from listings but can still
mutate refs. See
[Chapter 07 · moderation hooks](manual-protocol-contracts.md#moderation-hooks-are-mandatory).

## Step 6 — Browse the repository in the web application

Open the worked-example URL:

```text
https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3
```

What you should see, and what it means:

| Element | Expected on v3 |
|---|---|
| Contract type badge | **EVM V2 + V3** |
| Suite badge | "Suite v3 · IPFS" |
| Description | `Injective EVM demo repository for igit.xyz` |
| Stats row | HEAD SHA, branch count, tag count, packfile count (the demo ref carries one IPFS pack) |
| Clone box | `igit clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase` |
| Tabs | Code, Commits, Refs, Sponsors |
| IPFS explorer | Applicable — the `/ipfs` page can inspect the gateway data plane behind this repository |

The demo repository's own `README.md` restates the deployment facts of this
generation — Injective EVM Testnet chain ID `1439`, SuiteDirectory
`0xf8844F90887731FFd607E1f59e39a3918F6eAb35`, with source verification and
runtime binding visible in
[testnet Blockscout](https://testnet.blockscout.injective.network/). Its
`suite.json` records the deployed demo Directory, and its history was created
through ordinary Git commits on an empty synthetic Suite snapshot — new demo
data, unrelated to the legacy CosmWasm registry.

The `?suite=3` parameter selects this copy. Compare the same name on the
latest generation with [`?suite=4`](manual-suite-v4.md#step-6--browse-the-repository-in-the-web-application):
that page shows **EVM V4 / BYOS** badges instead, and its packs live in a
user-owned bucket rather than on IPFS.

## Step 7 — Know what you just proved

- [x] The configured Directory reports `suiteVersion() == 3` and is active.
- [x] `igit doctor --push` passes **with** the Kubo checks.
- [x] A push produced a durable, replicated IPFS pack before the ref moved.
- [x] `igit refs` reports the commit and the pack count.
- [x] A gateway-only clone succeeded with no local Kubo.
- [x] The web application renders the repository under `?suite=3` with the
      **EVM V2 + V3 / Suite v3 · IPFS** badges.

## Migrations, in one paragraph each

- **CosmWasm v1 → EVM (2026-08)** was a *fresh-empty cutover*: a new Suite was
  deployed with an empty snapshot, no repository data was imported, and V1
  became a read-only archive. See
  [Chapter 06](manual-cosmwasm-v1-archive.md).
- **Suite v3 → Suite v4 (2026-10)** followed the same pattern: a fresh
  successor Suite was deployed, new repositories live on it, and v3
  repositories remain readable through the IPFS path. Historical import from
  v3 into v4 is a separate, conditional task (S07) that must be explicitly
  approved; nothing in this chapter requires or performs it.

## Status boundaries

The IPFS data plane behind v3 — HK/US gateways, the US full Kubo pin set, the
controlled replication service, and the CAR archival pipeline — is deployed
and verified (2026-08). The v3 contract path is frozen by design and receives
no new storage features. Successor publication evidence, security review, and
mainnet governance approval remain open project-wide; see
[project status](project-status.md). Do not describe the testnet Directory
address as an official address: published network profiles keep
`SuiteDirectory` empty until approved cutover evidence exists.

## Next

- The latest generation → [Chapter 04 · Suite v4](manual-suite-v4.md)
- Storage profiles for v4 → [Chapter 08 · BYOS storage](manual-byos-storage.md)
- The contract model behind both generations → [Chapter 07](manual-protocol-contracts.md)
- The archived generation → [Chapter 06 · CosmWasm v1](manual-cosmwasm-v1-archive.md)
- Something is failing → [Chapter 10 · Troubleshooting](manual-troubleshooting.md)

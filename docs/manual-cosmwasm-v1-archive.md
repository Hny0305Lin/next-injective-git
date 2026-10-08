# 06 · CosmWasm v1 Archive — Read-Only Historical Repositories

Status: manual chapter. Audience: users and developers. Last updated: 2026-10-05.

CosmWasm v1 is the **first generation** of igit. Its control plane was a
CosmWasm `repo-registry` contract on Injective testnet, and its Windows
workflow required WSL2 to host the Linux CLI, `injectived`, and Kubo. Removing
that compatibility environment is the reason the EVM generation exists — see
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md).

The v1 → EVM transition (2026-08) was a **fresh-empty cutover**: no repository
data was imported. V1 therefore survives exactly as it was left — as a
**read-only archive**, reachable through a dedicated viewer and through
read-only CLI evidence tools. It is never an automatic fallback, and it is
never mixed with EVM suite reads
([ADR 0003](adr/0003-fresh-evm-suite-and-v1-archive-preview.md)).

The worked example throughout this chapter is:

| Field | Value |
|---|---|
| Owner | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` |
| Repository | `demo-showcase` |
| V1 registry contract | `inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh` (Injective Testnet) |
| Web URL | `https://www.igit.xyz/archive/cosmwasm-v1/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase` |

## Where the archive lives

The archive has its own URL space, separate from the EVM repository routes:

| Route | Shows |
|---|---|
| `/archive/cosmwasm-v1` | Archive landing page: browse and search archived owners and repositories |
| `/archive/cosmwasm-v1/<owner>` | An archived owner's repositories |
| `/archive/cosmwasm-v1/<owner>/<repo>` | The archived repository viewer |

The archive has **no entry in the primary navigation**. You reach it through a
direct URL, through search, or through links on the Monitor page. This is
deliberate: current work happens on the EVM generations; the archive is for
history.

Search recognizes archived content too: searching for an archived owner or
repository offers **View other V1 Archive results**, which queries the CosmWasm
contract directly and links into the archive routes. If the query does not
resolve to an archived owner, the page says so instead of failing silently.

## Step 1 — Open the worked example

```text
https://www.igit.xyz/archive/cosmwasm-v1/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
```

What you should see, and what it means:

| Element | Meaning |
|---|---|
| Banner | **Migration to EVM V2 or newer required** — "This repository remains readable from the CosmWasm V1 archive. Editing and current repository features require a future migration to the EVM V2 or newer Suite." |
| Banner button | **Migration unavailable** — a v1 → EVM repository migration path is not offered today |
| Contract type badge | **CosmWasm V1** |
| Description | `igit demo: README, code, history` |
| Stats row | HEAD SHA, branch count, tag count, packfile count (this ref points at three historical IPFS packs) |
| Tabs | Code, Commits, Refs — **no Sponsors tab**, because the Economic module is an EVM-suite capability |

The Code tab renders the archived tree and README exactly like an EVM
repository. In the Sponsors position, archived repositories show:

> Sponsorship is unavailable for archived CosmWasm V1 repositories.

## Step 2 — Understand what read-only means

The archive is a faithful, frozen view of chain state:

- **No writes.** There is no execute path. The archive tooling never loads a
  signer and never broadcasts a V1 transaction.
- **No migration today.** The banner's "Migration unavailable" button is
  honest: a repository-level v1 → EVM import has not been built. When the EVM
  suite was bootstrapped, V1 data was intentionally not imported.
- **Moderation is preserved.** Historical moderation decisions remain visible.
  For example, the early import `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/igit-dev`
  was **frozen** on 2026-08-05 (created with an unintended name; replaced by
  `Huawei-IAM-Java`) — the frozen state, historical metadata, transactions,
  and direct audit queries are intentionally retained because the registry is
  on-chain. See [archived repositories](archived-repositories.md).
- **Usernames are claimable, not migrated.** V1 username escrow was *not*
  carried over. Instead, the EVM UsernameModule opened a **90-day
  original-owner claim window** at bootstrap finalization; within that window
  an original V1 owner reclaims their name with:

  ```console
  $ igit username claim <name>
  original username "<name>" claimed
  ```

  After the window closes, unclaimed names follow ordinary registration
  rules. See [Chapter 07 · UsernameModule](manual-protocol-contracts.md).

## Step 3 — Know how the reads work

The archive reads the frozen contract through Cosmos LCD smart queries:

- **Contract:** `inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh` on Injective
  Testnet (the contract is unchanged since cutover; inspect it on
  [testnet explorer](https://testnet.explorer.injective.network/contract/inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh/)).
- **Endpoints, tried in order:** the official Injective sentry LCD
  (`https://testnet.sentry.lcd.injective.network`) and the Polkachu community
  LCD (`https://injective-testnet-api.polkachu.com`). A session sticks to
  whichever endpoint last answered; retries and response sizes are bounded.
- **Snapshot height:** each session pins a snapshot height (the latest block
  when the session starts) and sends it as `x-cosmos-block-height`, so a
  browsing session reads one consistent state. Endpoints whose CORS preflight
  rejects the custom header are queried at their latest committed state — an
  acceptable drift for a write-frozen archive, and preferable to failing the
  read.

## Step 4 — Use the read-only CLI evidence tools

The CLI ships three archive subcommands. They are deliberately isolated from
ordinary configuration and signer loading — they cannot execute anything:

```console
$ igit archive query --lcd https://testnet.sentry.lcd.injective.network \
    --contract inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh \
    --height <N> '{"repo_info":{"owner":"inj1...","name":"demo-showcase"}}'
```

- `query` runs an arbitrary smart query against the frozen contract, pinned to
  a fixed `--height`, and prints the indented JSON response.
- `inventory` builds a fixed-height inventory of the archive from saved
  `tx_search` and block evidence files.
- `verify` re-checks a snapshot against an inventory and the saved evidence —
  the offline audit path used to demonstrate that what you browse is what the
  chain held.

Historical fixed-height queries are available **only** through `igit archive`;
they are not a push configuration and not a fallback for the EVM path.

## Step 5 — Know what still backs the packs

V1 packfiles remain pinned by the historic data plane: the US full Kubo node
pins every CID that ever appeared in a V1 `update_ref`, exports CAR archives
with SHA-256 verification to cold storage, and syncs a durable-CID list to the
HK hot tier that the read gateways serve. This pipeline is documented as-built
in [IPFS data plane](infrastructure.md) and was the archival source for the
V1 generation. It is not part of the Suite v4 BYOS path.

## Step 6 — Know what you just proved

- [x] You can reach an archived repository through its explicit archive URL.
- [x] You understand why it is read-only and why migration is unavailable.
- [x] You know where the contract, the LCD endpoints, and the snapshot height
      come from.
- [x] You can run a fixed-height smart query yourself with `igit archive
      query` and reproduce what the viewer shows.

## Status boundaries

The archive is HISTORICAL by definition: it records the V1 generation and is
not a component of current acceptance. The archive viewer, its routes, and
the `igit archive` tools are delivered; a repository migration path from V1
into an EVM suite is **not implemented** and is not promised by any current
plan. The 90-day username claim window was a one-time event tied to the EVM
suite's bootstrap; whether it is still open depends on the suite's activation
date — check with `igit username show <name>`.

## Next

- The earlier EVM generation → [Chapter 05 · Suite v2 + v3](manual-suite-v2-v3.md)
- The latest EVM generation → [Chapter 04 · Suite v4](manual-suite-v4.md)
- The full version matrix → [suite version compatibility](suite-version-compatibility.md)
- Something is failing → [Chapter 10 · Troubleshooting](manual-troubleshooting.md)

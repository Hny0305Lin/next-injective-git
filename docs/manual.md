# igit Manual — Complete User and Developer Guide

Status: manual home. Applies to the current `dev` line. Last updated: 2026-10-05.

This manual is the single, self-contained walkthrough of Next Injective Git
(igit): a Git hosting stack whose control plane is a non-upgradeable Injective
EVM contract suite and whose data plane is version-dispatched pack storage.

It is written for **two audiences at once**:

- **Users** who want to install the CLI, create a repository, push and clone
  code, browse repositories in the web application, and understand what each
  screen shows.
- **Developers and operators** who need the on-chain contract model, the
  storage commitment protocol, the credential rules, the local build and test
  loop, and the exact evidence boundaries of the project.

Every chapter is written so that it can be read independently. Each version
generation has its own dedicated chapter, with concrete commands and concrete
URLs.

## How this manual is organized

| # | Chapter | Audience | What it gives you |
|---|---|---|---|
| 00 | [Manual home](manual.md) | both | Scope, version map, reading paths, prerequisites |
| 01 | [Getting started](manual-getting-started.md) | user | Install, configure, first repository, first push |
| 02 | [CLI command reference](manual-cli-reference.md) | user, operator | Every `igit` command with exact grammar |
| 03 | [Web application guide](manual-web-guide.md) | user | Every screen, wallet flow, search, monitors |
| 04 | [Suite v4 (latest EVM) walkthrough](manual-suite-v4.md) | user, developer | The BYOS path end to end, with worked examples |
| 05 | [Suite v2 + v3 (earlier EVM) walkthrough](manual-suite-v2-v3.md) | user, developer | The legacy IPFS path end to end |
| 06 | [CosmWasm v1 archive walkthrough](manual-cosmwasm-v1-archive.md) | user, developer | Read-only historical repositories |
| 07 | [On-chain contracts and protocol](manual-protocol-contracts.md) | developer | Suite model, ref shapes, verification chain |
| 08 | [BYOS storage and credentials](manual-byos-storage.md) | developer, operator | Storage profiles, manifest, credential rules |
| 09 | [Developer guide](manual-developer-guide.md) | developer | Repositories, build, test, CI, contribution |
| 10 | [Troubleshooting and FAQ](manual-troubleshooting.md) | both | Symptom → cause → fix |

## The three generations you will meet

igit has three coexisting generations. They are **not** alternative runtimes of
the same thing: they are distinct protocol generations with distinct on-chain
ref shapes and distinct storage paths. Clients do not guess — they read the
version from the chain and dispatch.

| Generation | On-chain protocol | Pack storage | Read/write | Chapter |
|---|---|---|---|---|
| **Suite v4** | `suiteVersion() == 4` — manifest commitment (`manifestDigest` + `manifestSize` + `bootstrapLocator` + `revision`) | **BYOS**: your own Amazon S3 or Cloudflare R2 bucket | Read **and write** | [04](manual-suite-v4.md) |
| **Suite v2 + v3** | `suiteVersion() <= 3` — `commitSha` + `packUris(string[])` | **IPFS**: `ipfs://CID` through gateways | Read **and write** | [05](manual-suite-v2-v3.md) |
| **CosmWasm v1** | CosmWasm contract state (separate chain path) | Historic IPFS | **Read-only** archive | [06](manual-cosmwasm-v1-archive.md) |

Two naming layers are used throughout the project and it matters that you keep
them apart:

- **EVM V2** is a *product-generation* name. It describes moving the control
  plane from CosmWasm to Injective EVM so that ordinary Windows and Linux
  operation no longer requires WSL2, `injectived`, or a local Kubo daemon.
- **Suite v3 / v4** are *protocol version* numbers, read from
  `SuiteDirectory.suiteVersion()` on chain.

So "EVM V2 generation" covers both Suite v3 and Suite v4. See
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md) and the
[suite version compatibility matrix](suite-version-compatibility.md) for the
authoritative statement.

### Version selection in one line each

- **Suite v4 is the current, latest path.** It is the storage-neutral successor
  delivered on 2026-10-05. Packs are uploaded to a bucket you own, the manifest
  commitment is written on chain, and the reader verifies a full read-back.
  → [Chapter 04](manual-suite-v4.md)
- **Suite v2 and v3 are the earlier EVM path.** Suite v3 is the first EVM
  release and is frozen by design; every ref points at one or more `ipfs://`
  CIDs. Suite v2 was never deployed as an EVM suite — a v2 repository, if one
  ever surfaced, would be read through the v3 shape. → [Chapter 05](manual-suite-v2-v3.md)
- **CosmWasm v1 is archived.** It is reachable only through an explicit,
  read-only archive viewer. It is never an automatic fallback. → [Chapter 06](manual-cosmwasm-v1-archive.md)

## The worked example used throughout

All worked examples in this manual use one real testnet account, so you can
compare your own output against something concrete:

| Field | Value |
|---|---|
| Injective (bech32) address | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` |
| EVM address | `0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8` |
| Local account label | `igit-dev` |
| Example repository | `demo-showcase` |
| Network | Injective Testnet (EVM chain ID `1439`) |

The repository exists in more than one generation at the same name. That is
exactly why the web application accepts a `?suite=` parameter — it is how a
reader says *which generation's copy of this name* they mean:

- Latest EVM, Suite v4 (BYOS):
  `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=4`
- Earlier EVM, Suite v3 (IPFS):
  `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3`
- Archived CosmWasm v1 (read-only):
  `https://www.igit.xyz/archive/cosmwasm-v1/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase`

The `?suite=` parameter is documented in full in
[Chapter 03](manual-web-guide.md#the-suite-query-parameter) and applied per
version in [Chapter 04](manual-suite-v4.md#step-6--browse-the-repository-in-the-web-application)
and [Chapter 05](manual-suite-v2-v3.md#step-6--browse-the-repository-in-the-web-application).

## Where the trust root lives

Ordinary clients — the CLI, `git-remote-igit`, and the web application — trust
exactly **one** configured `SuiteDirectory` address per generation. There are
no proxies, no diamonds, no `delegatecall`, and no implicit fallback path. The
client verifies, in order:

1. the RPC chain ID matches the configured profile,
2. `suiteVersion()` is a known version (3 or 4),
3. the Directory state is `active`,
4. the `configuredChainId` stored in the contract matches the live RPC,
5. every module address resolves and every module code hash matches,
6. every module is internally bound to the same Directory.

If any check fails, the client fails closed. It does not silently downgrade.
See [Chapter 07](manual-protocol-contracts.md#the-verification-chain).

The testnet profile currently lists **two** SuiteDirectory addresses, in this
order:

```text
0xf987396475d0a4c96b722e993a95d8720a6292ad   # Suite v4 (BYOS successor)
0xf8844F90887731FFd607E1f59e39a3918F6eAb35   # Suite v3 (IPFS)
```

The **first** address is the primary read target; repositories from all listed
suites are merged in listings. This is what makes `?suite=3` and `?suite=4`
meaningful on a single repository URL.

## Prerequisites

| Component | Requirement | Notes |
|---|---|---|
| Operating system | Native Windows or Linux | WSL2 is **not** required on the EVM path |
| Git | Any recent Git with a `PATH`-visible remote helper | `igit` forwards unknown subcommands to `git` |
| Node.js | ≥ 24 | Only for building the documentation site |
| Go | 1.22 or newer | Only for building the CLI from source |
| Injective account | Testnet key with a small INJ balance | Gas price floor is `160000000 wei` |
| Cloud bucket | Amazon S3 or Cloudflare R2 | **Suite v4 only**; required to push on v4 |
| Kubo | Local daemon | **Suite v3 only**; required to push on v3 |

`injectived` is **not** a prerequisite for ordinary EVM operation. That removal
is the entire point of the EVM V2 generation.

## Reading paths

**If you only want to use igit today**

1. [Chapter 01 — Getting started](manual-getting-started.md)
2. [Chapter 04 — Suite v4 walkthrough](manual-suite-v4.md)
3. [Chapter 03 — Web application guide](manual-web-guide.md)
4. [Chapter 10 — Troubleshooting](manual-troubleshooting.md)

**If you maintain a repository on the older IPFS generation**

1. [Chapter 01 — Getting started](manual-getting-started.md)
2. [Chapter 05 — Suite v2 + v3 walkthrough](manual-suite-v2-v3.md)
3. [Chapter 02 — CLI reference](manual-cli-reference.md)

**If you are integrating or auditing**

1. [Chapter 07 — Contracts and protocol](manual-protocol-contracts.md)
2. [Chapter 08 — BYOS storage and credentials](manual-byos-storage.md)
3. [Chapter 09 — Developer guide](manual-developer-guide.md)

## Status boundaries you must respect

This project publishes status with a bounded vocabulary — `PASS`, `FAIL`,
`BLOCKED`, `NOT PROVEN`, `HISTORICAL`. Those labels are used unchanged in both
languages and must not be softened. In particular:

- Suite v4 on Injective testnet, the CLI/Web version dispatch, and real
  Cloudflare R2 end-to-end Git flows are **delivered**.
- The real AWS S3 canary, Foundry gates, successor publication evidence,
  security review, and mainnet governance approval remain **open**.
- `SuiteDirectory` in published network profiles is intentionally held empty
  until real deployment and cutover evidence is approved. Do not treat a test
  address as an official address.

See [project status](project-status.md) and
[acceptance evidence](acceptance-evidence.md) for the authoritative record.

## Terminology

Check the [glossary](glossary.md) before translating or rephrasing anything.
Two rules from it are load-bearing across this whole manual:

- **Suite v3** means the legacy IPFS/Kubo path. **Suite v4** means the BYOS
  successor, whose production providers are **Amazon S3 and Cloudflare R2 only**.
- The six mainland-China S3-compatible providers tracked in the
  [BYOS provider roadmap](roadmap-byos-providers.md) are **roadmap candidates
  only**. They are not supported and not integrated, and their endpoints must
  not appear in any configuration example or production guidance.

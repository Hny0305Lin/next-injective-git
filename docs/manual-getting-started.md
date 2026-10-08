# 01 · Getting Started

Status: manual chapter. Audience: users. Last updated: 2026-10-05.

This chapter takes you from a bare machine to a pushed, cloned, and browsable
repository. It covers installation, configuration, the first on-chain
repository, and the verification steps that prove your setup is sound.

Everything here applies to the EVM generations (Suite v3 and Suite v4). The
archive generation has its own chapter: [Chapter 06](manual-cosmwasm-v1-archive.md).

## What you are about to build

```text
your terminal                     the chain                    your storage
─────────────                     ─────────                    ────────────
igit init demo-showcase  ───────► RepositoryCore.createRepository
igit push inj main       ───────► ref update (CAS)      ───────► S3 / R2 bucket   (Suite v4)
                                                             or IPFS pin        (Suite v3)
igit clone <owner>/<repo> ◄────── getRef + verified suite   ◄─── pack download
```

The chain stores **control plane** state: repository identity, metadata, refs,
collaborators, moderation, economics. Your bucket (v4) or IPFS (v3) stores the
**data plane**: Git packfiles. The ref is only updated after the bytes are
durable, and the reader verifies the bytes before handing them to Git.

## Step 1 — Verify your toolchain

Start by confirming what you already have. Do not skip this: a missing helper
binary is the single most common cause of a confusing failure.

```console
$ git --version
git version 2.43.0

$ go version
go version go1.22.5 windows/amd64

$ node -v
v24.11.1
```

Requirements:

| Tool | Needed for | Minimum |
|---|---|---|
| Git | everything | any recent release |
| `igit` | on-chain operations | matching release |
| `git-remote-igit` | clone/push/fetch | same release as `igit` |
| Go | building the CLI from source | 1.22 |
| Node.js | building the docs site only | 24 |

`injectived` is **not** required. WSL2 is **not** required.

## Step 2 — Install the CLI

### Option A — release binaries (recommended)

Download `igit` and `git-remote-igit` for your platform from the project's
release assets, then put **both** on your `PATH`. They must be the same
release: the helper speaks a versioned protocol with the CLI.

```console
# Linux / macOS
$ install -m 0755 igit-linux-amd64                ~/.local/bin/igit
$ install -m 0755 git-remote-igit-linux-amd64     ~/.local/bin/git-remote-igit
$ export PATH="$HOME/.local/bin:$PATH"
```

```powershell
# Windows (PowerShell)
PS> New-Item -ItemType Directory -Force "$env:USERPROFILE\.igit\bin" | Out-Null
PS> Copy-Item igit-windows-amd64.exe            "$env:USERPROFILE\.igit\bin\igit.exe"
PS> Copy-Item git-remote-igit-windows-amd64.exe "$env:USERPROFILE\.igit\bin\git-remote-igit.exe"
PS> $env:PATH = "$env:USERPROFILE\.igit\bin;$env:PATH"
```

The helper is discovered by Git through the `git-remote-<scheme>` naming
convention. If it is not on `PATH`, `igit clone` fails with a Git error about
an unknown remote helper — see [Chapter 10](manual-troubleshooting.md).

### Option B — build from source

```console
$ git clone https://github.com/Hny0305Lin/next-injective-git
$ cd next-injective-git/cli
$ go build -o igit             ./cmd/igit
$ go build -o git-remote-igit  ./cmd/git-remote-igit
```

Put both resulting binaries on `PATH`.

### Confirm the installation

```console
$ igit version
igit v0.x.y
```

A bare `igit` prints a compact quick start rather than the full reference:

```console
$ igit
igit - Next Injective Git (Injective EVM + suite-dispatched storage)

Quick start:
  igit setup                          prepare the complete push environment
  igit key import <name>              import an encrypted signing key
  igit init <name>                    create an on-chain repository
  igit clone <owner>/<repo> [dir]     clone a repository
  igit push [remote] [refspec...]     push on-chain (-q quiet, -v every step)
  igit pull [remote] [refspec...]     pull from chain

Explore:
  igit repos [owner]                  list on-chain repositories
  igit refs <owner> <repo>            list refs of a repository
  igit suite verify                   verify the chain and suite binding
  igit doctor                         diagnose tools, config and services

Anything else (add, commit, status, log, ...) is forwarded to git.
Full command reference: igit help
```

`igit help` prints the complete command reference — reproduced in
[Chapter 02](manual-cli-reference.md).

## Step 3 — Point the CLI at a network

Configuration lives in a single JSON file. On Windows and Linux alike:

```text
~/.igit/config.json
```

Override the location with the `IGIT_HOME` environment variable.

### Choose the profile

```console
$ igit config set network injective-testnet
network = injective-testnet
```

The profile supplies the chain ID, the LCD and RPC endpoints, the EVM RPC
endpoint, the block explorer, and the SuiteDirectory address.

| Profile | Cosmos chain | EVM chain ID | EVM RPC |
|---|---|---|---|
| `injective-testnet` | `injective-888` | `1439` | `https://k8s.testnet.json-rpc.injective.network` |
| `injective-mainnet` | `injective-1` | `1776` | `https://sentry.evm-rpc.injective.network/` |

### Select exactly one SuiteDirectory

This is the single trust root. Ordinary clients point at one address per
generation and never mix backends.

For the **latest EVM generation (Suite v4)** on testnet:

```console
$ igit config set evm_suite_directory_address 0xf987396475d0a4c96b722e993a95d8720a6292ad
evm_suite_directory_address = 0xf987396475d0a4c96b722e993a95d8720a6292ad
```

For the **earlier EVM generation (Suite v3)** on testnet:

```console
$ igit config set evm_suite_directory_address 0xf8844F90887731FFd607E1f59e39a3918F6eAb35
evm_suite_directory_address = 0xf8844F90887731FFd607E1f59e39a3918F6eAb35
```

The web application lists both addresses; the CLI takes exactly one. Switching
generations means changing this one value — the reader is then selected
automatically from the on-chain `suiteVersion()`.

> **Published profiles and empty directories.** The shipped network profiles
> deliberately leave `SuiteDirectory` empty until real deployment and cutover
> evidence is approved. Setting an address is your explicit, local choice. Never
> treat a test address as an official address, and never commit one into a
> published profile.

### Verify the binding

Never skip this. It is the cheapest possible failure.

```console
$ igit suite verify
suite verification passed
```

For machine-readable output:

```console
$ igit suite info --json
{
  "directory": "0xf987396475d0a4c96b722e993a95d8720a6292ad",
  "version": 4,
  "chain_id": 1439,
  "state": 1,
  "snapshot_root": "0x…",
  "bootstrap_coordinator": "0x…",
  "block_tag": "0x…",
  "modules": [ … ]
}
```

`version: 4` means you are on the BYOS successor. `version: 3` means the
frozen IPFS path. Any other value is rejected — the client accepts only 3 and 4
and fails closed otherwise.

## Step 4 — Create a signing key

Read-only commands (`repos`, `refs`, `collab list`, `suite info`, `suite verify`,
`doctor`, `archive …`) never need a key. Anything that writes to the chain does.

```console
$ igit key new dev
```

Or import an existing private key without echoing it to the terminal:

```console
$ igit key import dev
```

Then confirm the address the CLI will sign as:

```console
$ igit key show
```

The key is stored in an encrypted keystore under the configured keystore
directory. Fund that address with a small amount of testnet INJ — the client
enforces a gas price floor of `160000000 wei`.

To work with the account used throughout this manual:

| Field | Value |
|---|---|
| Injective address | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` |
| EVM address | `0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8` |

## Step 5 — Prepare the push environment

`igit setup` prepares everything a push needs and then proves it with the
diagnostic report.

```console
$ igit setup
igit will install pinned push dependencies under ~/.igit/deps.
Existing working Kubo installations will be preserved; EVM suite setup does not install injectived.
Continue? [y/N] y
```

Useful variants:

```console
$ igit setup push --yes                 # non-interactive
$ igit setup push --no-kubo             # Suite v4: no Kubo needed at all
$ igit setup push --create-key dev      # also create the signing key
$ igit setup push --force               # reinstall pinned dependencies
$ igit setup status                     # same as `igit doctor --push`
```

On **Suite v4** you want `--no-kubo`. The BYOS object-storage path does not
probe or require Kubo, the IPFS network, or the iGit gateway services. On
**Suite v3** Kubo is mandatory for push, because v3 pins packs through it.

### The diagnostic report

```console
$ igit doctor --push
igit doctor (push)
```

Each check reports `OK`, `WARN`, `FAIL`, or `SKIP`, and the report ends with a
`To fix:` block for anything that is not OK. The checks, in order:

| Check | What it proves |
|---|---|
| `git` | Git is on `PATH` |
| `git-remote-igit` | The remote helper is on `PATH` |
| `SuiteDirectory` | An address is configured |
| `chain backend` | The EVM v3/v4 immutable suite backend is selected |
| `LCD` | Always `SKIP` — ordinary runtime is EVM-only |
| `read gateway` | At least one configured read gateway answers `/healthz` |
| `key_name` | A signing key name is configured |
| `signing key` | The encrypted keystore unlocks |
| `key balance` | The signing address holds INJ (warns at zero) |
| `EVM RPC` | The RPC chain ID matches the profile |
| `Kubo CLI` | The pinned Kubo CLI resolves |
| `local Kubo API` | `POST <ipfs_api>/api/v0/version` answers |
| `upload authorization` | An explicit token exists, or the authorization endpoint issues one |

If the report ends with `environment is incomplete (N failed checks)`, the CLI
exits non-zero. Fix the listed items and re-run.

## Step 6 — Your first repository

Create the on-chain repository:

```console
$ igit init demo-showcase "igit demo repository"
repository created on chain.

add it as a git remote:
  igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
  igit push inj main
```

`igit init` with a plain name creates the repository on chain with default
branch `main`. With flags, or with `.`, it passes through to local
`git init` instead:

```console
$ igit init -b main .        # local git init, no chain transaction
```

Now wire up the working copy. `igit` forwards every subcommand it does not own
straight to Git, so the whole workflow stays inside one tool:

```console
$ mkdir demo-showcase && cd demo-showcase
$ igit init .                              # local git init
$ igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit add .
$ igit commit -m "first commit"
$ igit push inj main
```

Progress control:

```console
$ igit push inj main -q     # silence igit progress lines
$ igit push inj main -v     # show every step
$ export IGIT_QUIET=1       # environment equivalent
$ export IGIT_VERBOSE=1
```

Git flags must come **after** the repository argument for `igit clone`, because
the first positional argument selects the repository:

```console
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase -q
```

## Step 7 — Clone it back and confirm

```console
$ cd ..
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ cd demo-showcase
$ igit log --oneline -1
```

Cross-check against the chain:

```console
$ igit repos inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5
demo-showcase                    default:main         igit demo repository

$ igit refs inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
<commit-sha>  refs/heads/main    packfiles:1
```

The `packfiles:` count reports how many pack objects the ref points at. On
Suite v3 these are `ipfs://CID` URIs. On Suite v4 the ref carries a manifest
commitment instead, and pack locations are resolved from the signed manifest.

## Step 8 — Shorten the URL with a username

Addresses are long. Claim a username and the same repository becomes
reachable under it:

```console
$ igit username register haohanyh
```

Username registration locks a deposit. Once registered, `igit clone
<username>/<repo>` resolves through the on-chain username module.

```console
$ igit username show haohanyh
$ igit username show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5   # reverse lookup
$ igit username release                                           # give it back
```

> A username is **not** a SuiteDirectory. It changes how you *name* an owner;
> it does not change which suite you are talking to.

## Step 9 — Open it in the web application

The public web application at <https://www.igit.xyz> reads the same chain with
no installation at all.

Your repository, on the latest EVM generation (Suite v4):

```text
https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=4
```

The same name on the earlier EVM generation (Suite v3):

```text
https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3
```

And the archived CosmWasm v1 copy, read-only:

```text
https://www.igit.xyz/archive/cosmwasm-v1/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
```

[Chapter 03](manual-web-guide.md) walks every screen. The short version: the
repository page shows HEAD, branch/tag/packfile counts, a clone box that copies
the exact clone command, and tabs for Code, Commits, Refs, and Sponsors.

## Step 10 — Know what you just proved

At the end of this chapter you have evidence for all of the following:

- [x] The helper binary is on `PATH` and the CLI and helper versions match.
- [x] A network profile is selected and one SuiteDirectory is configured.
- [x] `igit suite verify` passed: chain ID, `suiteVersion()`, active state,
      `configuredChainId`, module addresses, and module code hashes all agreed.
- [x] A signing key exists, unlocks, and holds gas.
- [x] The push environment passed `igit doctor --push`.
- [x] A repository was created on chain and a ref was written.
- [x] The repository cloned back and the ref count matched.
- [x] The same repository rendered in the web application.

## A minimal command card

Keep this next to you; everything else is in
[Chapter 02](manual-cli-reference.md).

```console
# Inspect
igit suite verify                          # prove the trust root
igit suite info --json                     # machine-readable binding
igit doctor --push                         # full environment report
igit repos [owner]                         # list repositories
igit refs <owner> <repo>                   # list refs

# Prepare
igit config set network injective-testnet
igit config set evm_suite_directory_address <0x…>
igit key new dev
igit setup push --no-kubo                  # Suite v4
igit setup push                            # Suite v3 (needs Kubo)

# Use
igit init <name> "description"
igit remote add inj igit://<owner>/<name>
igit push inj main
igit clone <owner>/<name>
igit pull

# Administer
igit collab add <repo> <address> maintainer
igit fork <owner> <repo> [new-name]
igit transfer <repo> <new-owner>
igit mod <owner> <repo> <active|delisted|frozen> [reason-hash]
igit sponsor <owner> <repo> 0.5 "nice work"
```

## Next

- Full command surface → [Chapter 02 · CLI command reference](manual-cli-reference.md)
- The web application → [Chapter 03 · Web application guide](manual-web-guide.md)
- The latest generation end to end → [Chapter 04 · Suite v4](manual-suite-v4.md)
- The earlier generation end to end → [Chapter 05 · Suite v2 + v3](manual-suite-v2-v3.md)
- Something is broken → [Chapter 10 · Troubleshooting](manual-troubleshooting.md)

# 02 · CLI Command Reference

Status: manual chapter. Audience: users and operators. Last updated: 2026-10-05.

This chapter is the complete command surface of `igit`. Grammar is quoted from
the CLI's own usage text. Where a command has subcommands not listed in the
short usage, they are documented here as well.

## How to read this chapter

- **Syntax** uses `igit <literal> [optional] <required>...` notation.
- **Key** — `—` means the command is read-only and never loads a signer.
  `key` means it signs and broadcasts a transaction.
- **Who** — who is authorized on chain, independent of who *can* technically
  broadcast. The contract enforces authorization; the CLI only reports the
  resulting revert.

Two conventions apply everywhere:

1. **Anything not listed here is forwarded to Git.** `igit add`, `igit commit`,
   `igit status`, `igit log`, `igit branch`, `igit remote`, … all run real Git.
   Commands listed here shadow Git commands of the same name — use
   `git config` for Git's own `config`.
2. **Read-only commands use contract-only validation.** `igit repos`,
   `igit refs`, `igit collab list`, `igit suite info`, `igit suite verify`,
   `igit transfer show`, `igit guardians show`, `igit badge list`,
   `igit splits show`, `igit username show`, `igit release verify`,
   `igit doctor`, and every `igit archive` subcommand work with no key.

## Repository lifecycle

| Command | Syntax | Key | Who |
|---|---|---|---|
| Create on chain | `igit init <name> [description]` | key | anyone (becomes owner) |
| Local git init | `igit init [-b <branch>] [.]` | — | local only |
| Mirror a GitHub repo | `igit import <github-url> [name]` | key | anyone |
| Clone | `igit clone <owner>/<repo> [dir]` | — | anyone |
| Push | `igit push [remote] [refspec...]` | key | owner / maintainer |
| Pull | `igit pull [remote] [refspec...]` | — | anyone |
| Print your clone URL | `igit clone-url <name>` | — | anyone |
| List repositories | `igit repos [--all] [owner]` | — | anyone |
| List refs | `igit refs <owner> <repo>` | — | anyone |
| Edit metadata | `igit repo edit <repo> description <text...>` | key | owner |
| Edit default branch | `igit repo edit <repo> branch <name>` | key | owner |
| Fork | `igit fork <owner> <repo> [new-name]` | key | anyone |

### `igit init <name> [description]`

Creates the repository on chain with default branch `main`, then prints the
remote setup for you:

```console
$ igit init demo-showcase "igit demo repository"
repository created on chain.

add it as a git remote:
  igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
  igit push inj main
```

With no arguments, with a leading `-`, or with `.` it passes through to local
Git instead:

```console
$ igit init -b main .        # local git init
```

### `igit import <github-url> [name]`

Bare-clones a GitHub repository, creates the on-chain repository using the
source's own default branch, then pushes all branches and (best effort) tags.

Accepted source spellings: `github.com/user/repo`, `https://github.com/user/repo`,
`https://github.com/user/repo.git`, `git@github.com:user/repo`, `user/repo`.

```console
$ igit import github.com/Hny0305Lin/next-injective-git
cloning https://github.com/Hny0305Lin/next-injective-git.git ...
creating on-chain repo "next-injective-git" (default branch "dev") ...
pushing all branches to igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/next-injective-git ...
pushing tags ...

imported! your mirror is live:
  igit clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/next-injective-git
```

The default branch is read from the source with `git symbolic-ref --short HEAD`,
falling back to `main`.

### `igit clone <owner>/<repo> [dir]`

Wraps `git clone`. A bare `owner/repo` is expanded to `igit://owner/repo`.

```console
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase demo-local
```

Git flags must come **after** the repository argument, because the first
positional selects the repository:

```console
$ igit clone owner/repo -q          # correct
$ igit clone -q owner/repo          # error, with a hint telling you this
```

### `igit push` / `igit pull`

Thin wrappers over `git push` / `git pull` in the current repository.

```console
$ igit push inj main
$ igit push inj --all
$ igit push inj --tags
$ igit push inj +main               # force
$ igit pull
```

Suppress or expand igit's own progress lines:

```console
$ igit push -q          # or: export IGIT_QUIET=1
$ igit push -v          # or: export IGIT_VERBOSE=1
```

### `igit repos [--all] [owner]`

Lists active repositories. Without an owner it uses your signing key's address.
`--all` includes non-active repositories and marks them.

```console
$ igit repos inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5
demo-showcase                    default:main         igit demo repository

$ igit repos --all inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5
demo-showcase                    default:main         igit demo repository
old-experiment                   default:main         retired  [delisted]
```

### `igit refs <owner> <repo>`

```console
$ igit refs inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
<commit-sha>  refs/heads/main    packfiles:1
```

An empty repository prints `no refs (empty repository)`.

### `igit repo edit`

```console
$ igit repo edit demo-showcase description "igit demo repository, second take"
repo demo-showcase updated

$ igit repo edit demo-showcase branch trunk
repo demo-showcase updated
```

Fields are `description` (all remaining words joined) and `branch` (exactly one
word). Owner only.

### `igit clone-url <name>`

```console
$ igit clone-url demo-showcase
igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
```

### `igit fork <owner> <repo> [new-name]`

```console
$ igit fork alice awesome-lib my-awesome-lib
forked alice/awesome-lib
clone your fork: git clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/my-awesome-lib
```

Fork lineage is recorded on chain and rendered on the repository page as
`forked from <owner>/<repo>`.

## Collaboration and ownership

| Command | Syntax | Key | Who |
|---|---|---|---|
| Add collaborator | `igit collab add <repo> <address> [maintainer\|reader]` | key | owner |
| Remove collaborator | `igit collab remove <repo> <address>` | key | owner |
| List collaborators | `igit collab list <owner> <repo>` | — | anyone |
| Start transfer | `igit transfer <repo> <new-owner>` | key | owner |
| Accept | `igit transfer accept <owner> <repo>` | key | proposed new owner |
| Reject | `igit transfer reject <owner> <repo>` | key | proposed new owner |
| Clear expired | `igit transfer expire <owner> <repo>` | key | anyone |
| Cancel | `igit transfer cancel <repo>` | key | current owner |
| Show pending | `igit transfer show <owner> <repo>` | — | anyone |

### Collaborator roles

```console
$ igit collab add demo-showcase inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d maintainer
collaborator inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d added to demo-showcase as maintainer

$ igit collab add demo-showcase inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc reader

$ igit collab list inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
maintainer   inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d
reader       inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc

$ igit collab remove demo-showcase inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc
collaborator inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc removed from demo-showcase
```

The role defaults to `maintainer` when omitted. Only `maintainer` and `reader`
are accepted; anything else is rejected before a transaction is built.

### Ownership transfer — the 7-day rule

Ownership never moves in one step. The owner proposes, and the **new owner**
must accept after the maturation window.

```console
$ igit transfer demo-showcase inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j
ownership transfer for demo-showcase started; inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j must accept after 7 days

$ igit transfer show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
pending ownership transfer for inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase: inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j (proposed at 1770000000, execute after 1770604800, expires at 1771209600)
```

The new owner's address must be a valid `inj1…` bech32 address; the CLI rejects
anything else with `new owner %q must be an inj1... bech32 address`.

```console
$ igit transfer accept inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase   # as the new owner
$ igit transfer reject inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase   # as the new owner
$ igit transfer cancel demo-showcase                                             # as the current owner
$ igit transfer expire inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase   # after expiry
```

`igit transfer pending` is an accepted alias for `igit transfer show`.

## Guardian recovery

Recovery is the **only** ownership-recovery capability the Core contract
accepts. It is a separate, guardian-gated path — it is not an admin override.

| Command | Syntax | Key | Who |
|---|---|---|---|
| Configure | `igit guardians set <repo> <threshold> <address>...` | key | owner |
| Propose | `igit guardians propose <owner> <repo> <new-owner>` | key | guardian |
| Approve | `igit guardians approve <owner> <repo>` | key | guardian |
| Owner veto | `igit guardians cancel <repo>` | key | owner |
| Accept | `igit guardians accept <owner> <repo>` | key | proposed new owner |
| Show | `igit guardians show <owner> <repo>` | — | anyone |

```console
$ igit guardians set demo-showcase 2 inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j
guardians configured for demo-showcase (threshold 2)

$ igit guardians propose inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5
guardian recovery proposed; wait 7 days and collect approvals
```

`show` reports the configuration and any in-flight operations:

```console
$ igit guardians show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
threshold 2
guardians: inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d, inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j
recovery: inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 (2/2 approvals; execute after 1770604800)
```

Constraints enforced by the CLI before broadcast: the threshold parses as a
non-zero 8-bit integer, and every guardian address must start with `inj1`.

## Moderation

| Command | Syntax | Key | Who |
|---|---|---|---|
| Set status | `igit mod <owner> <repo> <active\|delisted\|frozen> [reason-hash]` | key | committee / admin |
| Submit report | `igit mod report <owner> <repo> <reason-hash>` | key | anyone |
| Read report | `igit mod report-show <report-id>` | — | anyone |
| Appeal | `igit mod appeal <report-id> <reason-hash>` | key | repo owner |
| Resolve report | `igit mod resolve <report-id> <active\|delisted\|frozen> <reason-hash>` | key | committee |
| Resolve appeal | `igit mod appeal-resolve <report-id> <active\|delisted\|frozen> <reason-hash>` | key | committee |

```console
$ igit mod inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase delisted 0xabc…
inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase moderation status set to delisted
```

Reasons are passed as **hashes**, not free text, so the chain stores no prose.
`igit mod report-show` prints the full record:

```console
$ igit mod report-show 7
#7 inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase status=delisted reporter=inj1kwq…
reason=0xabc…
resolution=delisted
resolution-reason=0xdef…
created-at=1770000000 updated-at=1770003600
```

Moderation hooks are mandatory for Core ref mutation and Economic sponsor
mutation. A `frozen` or `delisted` repository is still readable — moderation
changes visibility and write eligibility, not the immutability of published
pack bytes.

## Economics

| Command | Syntax | Key | Who |
|---|---|---|---|
| Sponsor | `igit sponsor <owner> <repo> <inj-amount> [message...]` | key | anyone |
| Set splits | `igit splits set <repo> [addr:bps]...` | key | owner |
| Show splits | `igit splits show <owner> <repo>` | — | anyone |

```console
$ igit sponsor inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase 0.5 "great tooling"
sponsored inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase with 0.5 INJ — thank you!
```

Amounts are decimal INJ, up to 18 decimal places, and must be positive. The CLI
converts them to base units (`inj`) before building the transaction.

Revenue splits use basis points (10000 bps = 100%):

```console
$ igit splits set demo-showcase inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d:2500 inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j:1500
revenue splits of demo-showcase updated (2 recipients)

$ igit splits show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
 25.0%  inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d
 15.0%  inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j
 60.0%  (owner remainder)
```

The owner receives the remainder after the platform fee. New sponsorship is
accepted **only in native INJ**; migrated historical totals remain queryable.

## Usernames, badges, and releases

### Usernames

```console
$ igit username register haohanyh      # claim, locks a deposit
$ igit username claim haohanyh         # reclaim a migrated V1 username
$ igit username release                # give it back
$ igit username show haohanyh          # resolve a name
$ igit username show inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5   # reverse lookup
```

### Badges

Badges are non-transferable trophies tied to a repository and a recipient.

```console
$ igit badge award demo-showcase inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d "first external pull request"
badge awarded to inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d for "first external pull request"

$ igit badge list inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d
#12   inj1sh4v00…/demo-showcase: "first external pull request"
```

`award` requires a configured `key_name`; the recipient may be given as an
address or a username.

### Releases

Release checksums are immutable once registered.

```console
$ igit release register v0.9.0 windows-amd64=<sha256> linux-amd64=<sha256>

$ igit release verify v0.9.0 windows-amd64 ./igit-windows-amd64.exe
```

Registration requires exactly 64 hex characters per digest, rejects duplicate
platforms, and normalizes to lowercase. Verification is read-only and fails
with `checksum mismatch for <platform>: got <a>, want <b>` on any difference.

## Suite inspection

| Command | Syntax | Key |
|---|---|---|
| Binding | `igit suite info [--json]` | — |
| Verification | `igit suite verify [--json]` | — |

```console
$ igit suite verify
suite verification passed
```

Both accept `--json` for machine-readable output. The command uses a five-minute
context, because a full verification is a sequence of dependent RPC calls.

The legacy `upgrade` subcommand **was removed** with the immutable EVM suite.
Invoking it now fails with a stable error code and tells you to use
`igit suite verify` instead. There is no
upgrade path by design: no proxy, no diamond, no `delegatecall`.

## Keys

| Command | Syntax | Key |
|---|---|---|
| Show address | `igit key show` | — |
| Create | `igit key new <name>` | — |
| Import | `igit key import <name>` | — |

```console
$ igit key show
$ igit key new dev
$ igit key import dev        # no terminal echo
```

`key new` and `key import` both persist the configured `key_name`.

## Gateways

| Command | Syntax | Key |
|---|---|---|
| Probe health | `igit gateway status` | — |
| Show order | `igit gateway select` | — |

```console
$ igit gateway status
hk       ok    https://igit-hk.haohanyh.ovh          42ms
us       ok    https://igit-us.haohanyh.ovh          180ms

$ igit gateway select
1  hk       https://igit-hk.haohanyh.ovh
2  us       https://igit-us.haohanyh.ovh
```

Both use a six-second budget. Gateways are probed on `/healthz` and ordered by
latency. These commands matter for the **Suite v3 / IPFS** read path; the Suite
v4 BYOS path does not use gateways.

## Storage (BYOS)

| Command | Syntax | Key |
|---|---|---|
| Register | `igit storage add <file>` | — |
| Validate | `igit storage doctor <file>` | — |
| Show | `igit storage show <file>` | — |

`storage` is handled before any chain or IPFS configuration is loaded, so it
works even when the rest of the config is incomplete.

```console
$ igit storage add ./storage-config.json
storage profile registered (path only; credentials are referenced, never stored)

$ igit storage doctor ./storage-config.json
PASS: local storage configuration (2 profiles, 1 repository bindings). Credentials not resolved; cloud access, CORS and Git integration tested separately.
```

Two things are worth stating plainly:

- `storage add` stores **only an absolute path** in your config. It does not
  copy, cache, or embed credentials.
- `storage doctor` does **no** cloud access. It validates the local file's
  shape only. Cloud reachability, CORS, and Git integration are separate tests.

Credentials are never values in the file. They are named environment variables,
resolved at the moment of use. See [Chapter 08](manual-byos-storage.md) for the
full profile format and the credential rules.

## Doctor and setup

| Command | Syntax | Key |
|---|---|---|
| Diagnose | `igit doctor [--clone\|--push] [--json]` | — |
| Prepare | `igit setup [options]` | — |
| Prepare (alias) | `igit setup push [options]` | — |
| Status | `igit setup status [--json]` | — |

`igit setup` with no arguments means `igit setup push`. `igit setup status` is
`igit doctor --push` with the same flags forwarded.

### `igit setup push` options

```text
usage: igit setup push [--yes] [--no-kubo] [--force] [--create-key NAME] [--wsl DISTRO]
```

| Option | Effect |
|---|---|
| `--yes`, `-y` | Skip the confirmation prompt |
| `--no-kubo` | Do not install Kubo (Suite v4; also skips the prompt) |
| `--force` | Reinstall pinned dependencies even if present |
| `--create-key NAME` | Create the signing key if none exists |
| `--wsl DISTRO` | Windows only: forward setup into a WSL distribution |

Step by step, `igit setup push`:

1. Applies the selected network profile.
2. Validates the EVM deployment profile (a missing SuiteDirectory fails here,
   before anything is installed).
3. Confirms with you — unless `--yes` or `--no-kubo` was given.
4. Creates the signing key if `--create-key` was passed and none exists.
5. Installs pinned dependencies under `~/.igit/deps` — unless `--no-kubo`.
   Existing working Kubo installations are preserved.
6. Saves the configuration.
7. Runs the push-mode doctor and prints the report.
8. Succeeds only if the required checks pass, then prints next steps.

The confirmation text is explicit about scope:

```text
igit will install pinned push dependencies under ~/.igit/deps.
Existing working Kubo installations will be preserved; EVM suite setup does not install injectived.
Continue? [y/N]
```

With `--wsl DISTRO` on Windows, the CLI first looks for an `igit` inside the
distribution whose `version` output matches exactly. If none matches, it
installs the released Linux CLI into `~/.local/bin` inside the distribution,
verifying `checksums.txt`, and appends the directory to `PATH` in `~/.profile`.

## Configuration

| Command | Syntax | Key |
|---|---|---|
| Public status | `igit config list` | — |
| Operator detail | `igit config list --internal` | — |
| Set | `igit config set <key> <value>` | — |
| Clear | `igit config unset <key>` | — |

```console
$ igit config list
{
  "network": "injective-testnet",
  "key_name": "dev",
  "local_ipfs": "configured",
  "upload_service": "configured"
}
```

The public view deliberately hides backend detail. `--internal` prints the
config file path and the full stored JSON, with `upload.authorization`
redacted:

```console
$ igit config list --internal
# /home/user/.igit/config.json
{
  "network": "injective-testnet",
  "evm_suite_directory_address": "0x…",
  "upload": { "authorization": "<redacted>", … },
  …
}
```

### Config keys

| Key | Controls |
|---|---|
| `network` | Selects the profile; atomically replaces the profile-owned fields below |
| `evm_suite_directory_address` | **The single trust root.** One `0x` + 40 hex address |
| `evm_rpc` | EVM JSON-RPC endpoint |
| `evm_chain_id` | Expected EVM chain ID |
| `evm_explorer` | Block explorer base URL |
| `evm_keystore_dir` | Encrypted keystore location |
| `storage_config` | Path to the BYOS storage reference file (Suite v4) |
| `key_name` | Which encrypted key signs |
| `ipfs_api` | Local Kubo API, default `http://127.0.0.1:5001` (Suite v3) |
| `ipfs_bin` | Kubo binary name or path (Suite v3) |
| `ipfs_gateway` | Preferred read gateway |
| `upload.endpoint` | Replication/upload service endpoint |
| `upload.authorization_endpoint` | Where to request a short-lived upload token |
| `upload.authorization` | Explicit token (redacted in output) |
| `upload.us_peer` | US Kubo swarm peer multiaddr |
| `upload.hk_peer` | HK Kubo swarm peer multiaddr |

`igit config unset <key>` clears an override and falls back to the built-in
default.

Validation is strict and fails early. A missing or malformed SuiteDirectory
produces a stable error code rather than a confusing RPC failure later:

```console
$ igit repos
igit: missing config: evm_suite_directory_address (run `igit config set evm_suite_directory_address 0x…`)
```

Read-only commands validate the contract selection only. Commands that write
additionally require `key_name`:

```console
igit: missing config: key_name (run `igit config set <key> <value>`)
```

## Archive (CosmWasm v1)

The archive tools are deliberately isolated: they are dispatched **before**
configuration is loaded, never load a signer, and never broadcast a V1
transaction.

```text
igit archive query --lcd URL --contract inj1... --height N '<smart-query-json>'
igit archive inventory --tx-search FILE --block-evidence FILE --chain-id ID --contract inj1... --height N --output FILE
igit archive verify --snapshot FILE --inventory FILE --tx-search FILE --block-evidence FILE
```

```console
$ igit archive query --lcd https://testnet.sentry.lcd.injective.network \
    --contract inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh --height 139852506 \
    '{"repo_info":{"owner":"inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5","repo":"demo-showcase"}}'
```

`inventory` and `verify` write evidence files. Evidence is never overwritten: a
second run against an existing file fails with `archive evidence <path> already
exists and is never overwritten`, and files are written `0600`.

Full walkthrough: [Chapter 06](manual-cosmwasm-v1-archive.md).

## Exit codes, output, and locale

- Success exits `0`.
- Any error is printed to stderr as `igit: <message>` and exits `1`.
- The remote helper prints `git-remote-igit: <message>` to stderr and exits `1`.
- `igit doctor` exits non-zero when required checks fail.

Error codes are stable identifiers independent of the message text, so scripts
can match on them rather than parsing prose.

### Locale

User-visible messages are bilingual. The CLI selects Chinese only for
`zh-CN`, `zh-HK`, `zh-MO`, and `zh-TW`, using the first non-empty value among
`LC_ALL`, `LC_MESSAGES`, `LANG`, and `LANGUAGE`. A bare `zh` — and every other
locale — renders **English**. On Windows, with no locale environment variables
set, the system default locale is consulted.

```console
$ LANG=zh_CN.UTF-8 igit repos
$ LANG=en_US.UTF-8 igit repos
```

Progress output is controlled independently:

| Variable | Effect |
|---|---|
| `IGIT_QUIET=1` | Silence igit progress lines |
| `IGIT_VERBOSE=1` | Print every step |

Values `1`, `true`, `yes`, and `on` are accepted, case-insensitively. The
environment variables are *pinned*: a later Git `option verbosity` cannot
override them, which is what makes `-q`/`-v` behave predictably inside scripts.

## Environment variables

| Variable | Effect |
|---|---|
| `IGIT_HOME` | Overrides the config directory (default `~/.igit`) |
| `IGIT_QUIET` | Silence progress lines |
| `IGIT_VERBOSE` | Verbose progress |
| `IGIT_EVM_KEY_PASSWORD` | Supplies the keystore passphrase non-interactively |
| `LC_ALL`, `LC_MESSAGES`, `LANG`, `LANGUAGE` | Locale selection |
| `GIT_DIR` | Git directory, honored by the helper |
| `IPFS_PATH` | Kubo repository location (setup and v3 push) |

Do not put a keystore passphrase in `IGIT_EVM_KEY_PASSWORD` in a shared or
logged environment. It exists for automation, not for convenience.

## URL forms

| Form | Meaning |
|---|---|
| `igit://<owner>/<repo>` | Canonical remote URL |
| `igit::<owner>/<repo>` | Accepted alias |
| `<owner>/<repo>` | Accepted by `igit clone`; expanded to `igit://` |
| `igit://<owner>/<repo>.git` | Trailing `.git` is stripped |

`<owner>` must be either an `inj1…` bech32 address or a registered username
(3–32 characters, lowercase letters, digits, and hyphens; no leading or
trailing hyphen; never starting with `inj1`). Anything else fails with:

```text
invalid remote URL "…" (expected igit://<owner>/<repo>)
invalid owner "…": expected an inj1... address or a registered username
```

## Version dispatch, seen from the CLI

You do not choose the storage path. The version you configured chooses it.

```console
$ igit suite info --json | grep '"version"'
  "version": 4,
```

- `version: 4` → the BYOS path. `igit push` uploads packs to your bucket,
  verifies a read-back, publishes a canonical JSON manifest, then updates the
  ref with revision CAS. No Kubo, no IPFS network.
- `version: 3` → the frozen IPFS path. `igit push` uploads the pack to your
  local Kubo, confirms the pin, registers replication, then updates the ref
  with `packUris`. Kubo is required.
- Any other version → rejected. The client supports 3 and 4 and fails closed.

The same rule governs cloning: `git-remote-igit` probes the suite once, then
routes every `list`, `fetch`, and `push` to the matching implementation.

## Next

- The web application → [Chapter 03](manual-web-guide.md)
- Suite v4 in practice → [Chapter 04](manual-suite-v4.md)
- Suite v2 + v3 in practice → [Chapter 05](manual-suite-v2-v3.md)
- Storage profiles → [Chapter 08](manual-byos-storage.md)

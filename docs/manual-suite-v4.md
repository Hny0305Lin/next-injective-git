# 04 · Suite v4 Walkthrough — The Latest EVM Generation (BYOS)

Status: manual chapter. Audience: users and developers. Last updated: 2026-10-05.

Suite v4 is the current generation. It is the **storage-neutral successor**: the
chain holds a commitment to your repository's pack data, and the bytes
themselves live in a cloud bucket **you own and administer**.

The worked example throughout this chapter is:

| Field | Value |
|---|---|
| Owner (Injective address) | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` |
| Owner (EVM address) | `0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8` |
| Account label | `igit-dev` |
| Repository | `demo-showcase` |
| SuiteDirectory (testnet) | `0xf987396475d0a4c96b722e993a95d8720a6292ad` |
| EVM chain ID | `1439` |
| Web URL | `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=4` |

> **Provider boundary.** Suite v4 production storage supports **Amazon S3 and
> Cloudflare R2 only**. MinIO, self-hosted object stores, generic S3-compatible
> write endpoints, and every other cloud are out of scope. The mainland-China
> providers in the [BYOS provider roadmap](roadmap-byos-providers.md) are
> **roadmap candidates only** — not supported, not integrated — and their
> endpoints must not appear in any configuration.

## What is different about v4

Three properties distinguish Suite v4 from the frozen v3 path. Each one changes
how you operate.

**1. The ref is a commitment, not a list of URLs.** On Suite v3 a ref stores
`commitSha` plus an array of `ipfs://CID` strings. On Suite v4 a ref stores four
fields:

```solidity
struct GitRef {
    bytes32 manifestDigest;    // SHA-256 of the canonical manifest bytes
    uint96  manifestSize;      // manifest length in bytes, capped at 65 536
    string  bootstrapLocator;  // where the manifest can be fetched, max 512 chars
    uint64  revision;          // monotonic counter for compare-and-swap
    // plus updatedAt, updatedBy, exists
}
```

The chain therefore knows *what* your data is (a digest) and *where to start
looking* (a locator). It does not know or care which buckets exist behind it.

**2. Every ref write is a compare-and-swap.** `updateRef` takes the expected
current state and reverts on mismatch. Concurrency is enforced by the chain, not
by politeness:

| Operation | Expected revision | Expected digest |
|---|---|---|
| Create | `0` | `bytes32(0)` |
| Update existing | current revision | current `manifestDigest` |
| Recreate after delete | the tombstone revision | `bytes32(0)` |

A mismatch reverts with `CommitmentMismatch`. **Force-pushing never waives this
check** — on v4, `force` changes which ref you may overwrite, never whether the
CAS was satisfied. This is the single most important behavioural difference from
v3.

**3. Every read is verified before it is trusted.** The reader fetches the
manifest, checks its digest and size against the on-chain commitment, then
fetches each pack and checks its raw SHA-256 and byte length before handing
anything to the Git object parser. Verification happens **before** Git is allowed
to repair or ingest bytes, because a `git index-pack --fix-thin` result is no
longer guaranteed to equal the uploaded raw pack.

The practical consequence: a user-owned bucket is safe to read from, because the
bucket can lie about contents but not about hashes.

## Prerequisites specific to v4

| Item | Requirement |
|---|---|
| Bucket | Amazon S3 or Cloudflare R2, in a commercial region |
| Public read | Anonymous HTTPS GET with CORS enabled, so browsers can read packs |
| Credentials | Supplied as **named environment variables**, never as file values |
| Kubo | **Not required.** No local daemon, no IPFS network, no gateway |
| `injectived` | **Not required** |
| WSL2 | **Not required** |

The R2 free tier is enough to complete this walkthrough.

## Step 1 — Confirm you are on a v4 Suite

Never assume. Read the version from the chain.

```console
$ igit config set network injective-testnet
network = injective-testnet

$ igit config set evm_suite_directory_address 0xf987396475d0a4c96b722e993a95d8720a6292ad
evm_suite_directory_address = 0xf987396475d0a4c96b722e993a95d8720a6292ad

$ igit suite verify
suite verification passed
```

Now confirm the version is 4:

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

`"version": 4` and `"state": 1` is the pair you need. Version 3 here would mean
you configured the legacy Directory; any other version is rejected outright,
because the client accepts only 3 and 4 and fails closed.

In the web application the same fact is rendered as a badge on the repository
page: **EVM V4** with the **BYOS** tag, and the suite badge reads
"Suite v4 · BYOS".

## Step 2 — Create the storage bucket

### Cloudflare R2

Create a bucket, then arrange two things:

1. **Anonymous read access.** Packs and manifests are fetched over plain HTTPS
   GET with no credentials, so the bucket (or the `packs/` and `manifests/`
   prefixes) must be publicly readable.
2. **CORS.** The web application reads packs from the browser, so the bucket must
   answer preflight requests from the application origin. A CLI-only workflow
   does not need CORS, but the web reader does.

Note the account ID from your R2 dashboard. It is 32 lowercase hex characters
and it is required in the profile.

### Amazon S3

Create a bucket in a commercial region and enable public read for the pack and
manifest prefixes. Note the region exactly as AWS reports it.

> **Region and account rules are enforced.** `aws-s3` profiles must carry a
> region from the commercial allowlist and must leave the account ID empty;
> `cloudflare-r2` profiles must use region `auto` and must supply a 32-hex
> account ID. A profile that violates either rule is rejected before any network
> call.

## Step 3 — Write the storage reference file

Credentials are **never values in this file**. The file *names* the environment
variables that hold them.

```json
{
  "version": 1,
  "profiles": {
    "r2-demo-writer": {
      "provider": "cloudflare-r2",
      "bucket": "igit-demo-showcase",
      "region": "auto",
      "accountId": "0123456789abcdef0123456789abcdef",
      "prefix": "igit",
      "credentialRef": {
        "kind": "env",
        "accessKeyEnv": "IGIT_R2_WRITER_ACCESS_KEY_ID",
        "secretKeyEnv": "IGIT_R2_WRITER_SECRET_ACCESS_KEY"
      }
    },
    "r2-demo-reader": {
      "provider": "cloudflare-r2",
      "bucket": "igit-demo-showcase",
      "region": "auto",
      "accountId": "0123456789abcdef0123456789abcdef",
      "prefix": "igit",
      "publicReadBase": "https://packs.example.com"
    }
  },
  "repositories": [
    {
      "chainId": "1439",
      "suiteDirectory": "0xf987396475d0a4c96b722e993a95d8720a6292ad",
      "repoId": "0x…",
      "writer": "r2-demo-writer",
      "reader": "r2-demo-reader"
    }
  ]
}
```

Field by field:

| Field | Meaning |
|---|---|
| profile map key | Profile identifier; `[A-Za-z][A-Za-z0-9_-]{0,63}` |
| `provider` | `aws-s3` or `cloudflare-r2` — nothing else is accepted |
| `bucket` | Bucket name, `[a-z0-9][a-z0-9-]{1,61}[a-z0-9]` |
| `region` | Commercial region for S3; `auto` for R2 |
| `accountId` | 32 lowercase hex, R2 only; must be empty for S3 |
| `prefix` | Optional key prefix |
| `publicReadBase` | Optional public base URL for anonymous reads |
| `credentialRef` | Names — not values — of the environment variables |

The environment variable names are validated too: `[A-Z][A-Z0-9_]{0,127}`.

### The writer/reader split

Every repository binding names a **writer** profile and a **reader** profile.
The two must be **different** profiles, and the rules are strict:

- The **writer** must carry a `credentialRef` (it uploads).
- The **reader** must carry either its own `credentialRef` or a
  `publicReadBase` (it downloads).
- Writer and reader must be different profiles.
- Writer and reader must **not** share an `accessKeyEnv` or a `secretKeyEnv`.

That last rule is what stops a compromise of the read path from becoming a
write path. Keep the two credential sets genuinely separate.

### Register and validate

```console
$ igit storage add ./storage-config.json
storage profile registered (path only; credentials are referenced, never stored)

$ igit storage doctor ./storage-config.json
PASS: local storage configuration (1 profiles, 1 repository bindings). Credentials not resolved; cloud access, CORS and Git integration tested separately.
```

Two things worth being explicit about:

- `storage add` stores **only an absolute path** in your config. It does not
  copy, cache, or embed credentials.
- `storage doctor` does **no** cloud access and resolves **no** credentials. It
  validates the local file's shape. Reachability, CORS, and Git integration are
  separate tests.

### Supply the credentials

Set the named environment variables in the shell that will run the push:

```console
$ export IGIT_R2_WRITER_ACCESS_KEY_ID=…
$ export IGIT_R2_WRITER_SECRET_ACCESS_KEY=…
```

```powershell
PS> $env:IGIT_R2_WRITER_ACCESS_KEY_ID = "…"
PS> $env:IGIT_R2_WRITER_SECRET_ACCESS_KEY = "…"
```

> **Never commit credentials.** Do not put them in the storage reference file,
> in a config file, in a script, or in a repository. The file references
> environment variables precisely so that it can be shared and committed while
> the secrets cannot. The CLI scans pushed refs for credential-shaped content
> and warns, but that warning is a safety net, not a licence.

## Step 4 — Prepare the push environment without Kubo

This is where v4 is dramatically simpler than v3.

```console
$ igit setup push --no-kubo
```

`--no-kubo` skips the Kubo install and the confirmation prompt. It is the
correct choice for every v4 workflow: the BYOS path does not probe Kubo, does
not contact the IPFS network, and does not use a gateway.

Then verify:

```console
$ igit doctor --push
igit doctor (push)
```

The Kubo checks report `SKIP`. What must be `OK`:

- `git` and `git-remote-igit` on `PATH`
- `SuiteDirectory` configured
- `chain backend` — the EVM v3/v4 immutable suite
- `EVM RPC` — chain ID matching the profile
- `key_name` and `signing key`
- `upload.endpoint` and `upload.authorization` (used for the legacy replication
  rail; the v4 BYOS push writes to your bucket)
- `read gateway` — harmless on v4, but must resolve

If the report ends with `environment is incomplete (N failed checks)`, the CLI
exits non-zero. Fix and re-run.

## Step 5 — Put the repository on chain

```console
$ igit init demo-showcase "igit demo repository — BYOS on Suite v4"
repository created on chain.

add it as a git remote:
  igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
  igit push inj main
```

Then wire up a working copy and make the first push:

```console
$ mkdir demo-showcase && cd demo-showcase
$ igit init .
$ igit remote add inj igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
$ igit add .
$ igit commit -m "first commit on Suite v4"
$ igit push inj main -v
```

`-v` is worth using on the first BYOS push. You will see the phases explicitly.

### What happens during a v4 push

1. **Probe the suite.** `git-remote-igit` verifies the Directory and reads
   `suiteVersion()`. It is 4, so the BYOS path is selected. The helper also
   requires that the owner is an address — a username must be resolved first —
   and that a storage profile is registered.
2. **Build the pack.** Git produces the pack; the helper decides what to send
   based on what the remote already has.
3. **Upload the packs.** Each pack is written to your bucket under a
   digest-derived key:
   ```text
   packs/sha256/<digest>.pack
   ```
4. **Verify by read-back.** The uploaded object is read back and its raw
   SHA-256 and byte length are checked. A mismatch fails the push. Metadata and
   ETags are never accepted as a substitute.
5. **Build and upload the manifest.** A canonical JSON manifest (RFC 8785 JCS)
   records every pack and its location:
   ```text
   manifests/sha256/<digest>.json
   ```
   The manifest digest is computed over the canonical bytes; the manifest does
   not hash itself. `manifestSize` is capped at 65 536 bytes.
6. **Update the ref with CAS.** The chain call supplies the expected revision and
   expected digest. On mismatch it reverts with `CommitmentMismatch` and the
   push fails — **even with `--force`**.

The ordering is deliberate and non-negotiable: **data is durable before the ref
points at it**. A ref never references bytes that have not been written and
verified.

### Reading a push failure

| Symptom | Meaning | What to do |
|---|---|---|
| `CommitmentMismatch` revert | Someone else updated the ref between your read and your write | Fetch, re-apply, push again. Do not force. |
| Digest or size mismatch on read-back | The object in the bucket is not what was uploaded | Check for a bucket lifecycle rule, a transform, or an intermediary rewriting objects |
| Missing storage profile | The helper needs a binding for this `chainId:suiteDirectory:repoId` | Add the repository binding to the reference file and re-run `igit storage add` |
| Address-owner required | You pushed to a username and the helper cannot bind storage to it | Resolve the username to its address and use `igit://<inj1…>/<repo>` |
| Credential resolution failure | A named environment variable is unset in this shell | Export it in the same shell, then retry |

## Step 6 — Browse the repository in the web application

Open the worked-example URL:

```text
https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=4
```

What you should see, and what it means:

| Element | Expected on v4 |
|---|---|
| Contract type badge | **EVM V4** with a **BYOS** tag |
| Suite badge | "Suite v4 · BYOS" |
| Stats row | HEAD SHA, branch count, tag count, packfile count |
| Clone box | `igit clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase` |
| Sponsors tab | Present (EVM repositories only) |
| IPFS explorer | Not applicable — v4 packs are in your bucket, not on IPFS |

The `?suite=4` parameter is what selects this copy. Drop it and the page uses the
first configured SuiteDirectory; change it to `?suite=3` and you get the v3 copy
of the same name, if one exists. See
[Chapter 03 · the suite query parameter](manual-web-guide.md#the-suite-query-parameter).

Compare against the earlier generation:

```text
https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3
```

That page shows **EVM V2 + V3** and "Suite v3 · IPFS" instead. Same name, same
owner, different generation — which is exactly what the parameter is for.

## Step 7 — Clone and verify

```console
$ igit clone inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
```

The clone needs no credentials at all. It:

1. verifies the Suite,
2. resolves the owner and repository,
3. reads the ref and gets the manifest commitment,
4. fetches the manifest by its locator and checks digest and size against chain,
5. fetches each pack from the recorded locations and checks raw SHA-256 and size,
6. hands verified bytes to Git.

Because step 5 precedes Git's ingestion, a corrupted or substituted object is
rejected before it can enter your object database.

Cross-check with the chain:

```console
$ igit refs inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase
<commit-sha>  refs/heads/main    packfiles:1
```

## Step 8 — Incremental packs (manifest schema 2)

Schema 2 is the incremental-pack improvement delivered with ADR 0005. It is
worth understanding because it explains why a v4 push is usually fast.

- **Schema 1** describes a complete pack set for a ref.
- **Schema 2** allows a ref to be satisfied by a base plus increments, so a push
  that adds a few commits does not re-upload the repository.

Three things to know:

1. **No contract change, no version change.** The chain is unaware of
   `schemaVersion`; the field lives in the manifest and is interpreted entirely
   client-side. The Suite stays at `suiteVersion == 4`.
2. **The CAS still applies.** Increments change the ref, and every ref write is
   still a compare-and-swap against the revision and digest.
3. **Read-back verification still applies, per object.** Incremental does not
   mean unchecked.

## Step 9 — Operate it

```console
# Read the current state
igit repos inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5
igit refs inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5 demo-showcase

# Read the on-chain commitment directly
# getRef(repoId, "refs/heads/main") returns
#   (manifestDigest, manifestSize, bootstrapLocator, revision, updatedAt, updatedBy, exists)

# Diagnose without touching the cloud
igit storage doctor ./storage-config.json
igit storage show ./storage-config.json

# Diagnose the full write environment
igit doctor --push
```

### What the chain does and does not prove

This distinction matters for anyone evaluating the guarantees.

The chain proves:
- which manifest was committed at a given revision,
- that the ref advanced monotonically under CAS,
- who signed the update.

The chain does **not** prove:
- that the bucket still exists,
- that the objects are still publicly readable,
- that the pack is still retrievable.

There is no on-chain proof of pack availability and no trusted storage-receipt
signer, by design. Availability is established at read time by fetching and
verifying, not by trusting a claim. Keep the bucket alive, keep public read
enabled, and keep object lifecycle rules from expiring your packs.

## Step 10 — Know what you just proved

- [x] The configured Directory reports `suiteVersion() == 4` and is active.
- [x] A storage profile validates locally with a writer/reader pair that does
      not share credentials.
- [x] Credentials resolve from named environment variables and are absent from
      every file you would commit.
- [x] `igit doctor --push` passes with the Kubo checks skipped.
- [x] A pack was uploaded, read back, and digest-verified before the ref moved.
- [x] The ref advanced under revision CAS.
- [x] A fresh clone verified the manifest and every pack before Git ingested
      them.
- [x] The web application renders the repository under `?suite=4` with the
      **EVM V4 / BYOS** badge.

## Operational checklist

| Concern | Practice |
|---|---|
| Credentials | Environment variables only. Rotate writer keys independently of reader keys. |
| Bucket lifecycle | Never let a lifecycle rule expire `packs/` or `manifests/`. |
| Public read | Must stay enabled for browsers and anonymous clones. |
| CORS | Must allow the application origin for the web reader. |
| Concurrency | Expect `CommitmentMismatch` under contention. Fetch and retry; do not force blindly. |
| Cost | You own the bucket and pay for storage and egress. |
| Backups | The chain stores commitments, not bytes. Back up the bucket. |

## Status boundaries

Suite v4 is **delivered**: the successor suite is deployed and active on
Injective testnet, CLI and Web dispatch by on-chain version, and real Cloudflare
R2 end-to-end Git flows pass without Kubo, WSL2, or `injectived`.

The following remain **open** and must not be described as complete: the real
AWS S3 canary, real force-push and concurrency races, Blockscout verification,
Foundry gates (R04), successor publication evidence, security review, and
mainnet governance approval. See [project status](project-status.md).

## Next

- The earlier EVM generation → [Chapter 05 · Suite v2 + v3](manual-suite-v2-v3.md)
- Storage profiles in depth → [Chapter 08 · BYOS storage](manual-byos-storage.md)
- The contract model → [Chapter 07 · Contracts and protocol](manual-protocol-contracts.md)
- Something is failing → [Chapter 10 · Troubleshooting](manual-troubleshooting.md)

# 08 · BYOS Storage and Credentials

Status: manual chapter. Audience: developers and operators. Last updated: 2026-10-05.

Suite v4 separates the **control plane** (repository identity, refs,
permissions — on chain) from the **data plane** (Git packfiles — in a bucket
**you own**). This chapter documents the storage configuration file, the
manifest protocol that binds bytes to the chain, the credential rules, and the
failure-and-recovery contract. Specification of record:
[BYOS implementation specification](storage-byos.md), [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md),
[ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md).

> **Provider boundary.** Production storage supports **Amazon S3 (`aws-s3`)
> and Cloudflare R2 (`cloudflare-r2`) only**. MinIO, self-hosted object
> stores, generic S3-compatible write endpoints, and every other cloud are
> out of scope. The six mainland-China providers in the
> [BYOS provider roadmap](roadmap-byos-providers.md) are **roadmap candidates
> only** — not supported, not integrated — and their endpoints must not appear
> in any configuration example, CLI flag, or production guidance.

## The model in one diagram

```text
git push ──► pack bytes ──► your bucket (packs/sha256/<digest>.pack)
                 │
                 └─ read-back verify (SHA-256 + size)
                 ▼
            manifest (canonical JSON, RFC 8785 JCS)
                 │  ──► your bucket (manifests/sha256/<digest>.json)
                 ▼
            on-chain ref commitment (digest, size, locator, revision)
```

Data is durable and verified **before** the ref points at it. The contract
cannot fetch cloud bytes: "data before ref" is a client publication policy,
and availability is established at read time by fetching and verifying, not by
trusting a claim.

## The storage reference file

`igit storage add <file>` registers one JSON file. The file holds **references
to credentials, never values**, and is parsed with a strict JSON decoder
(no duplicate keys, bounded to 64 KiB):

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

### Profile fields

| Field | Rule |
|---|---|
| profile map key | `[A-Za-z][A-Za-z0-9_-]{0,63}`; 1–32 profiles per file |
| `provider` | `aws-s3` or `cloudflare-r2` — anything else is rejected |
| `bucket` | `[a-z0-9][a-z0-9-]{1,61}[a-z0-9]`, with S3 alias suffixes disallowed |
| `region` | For `aws-s3`: one of the accepted commercial regions (e.g. `us-east-1`); for `cloudflare-r2`: exactly `auto` |
| `accountId` | R2 only: 32 lowercase hex characters; must be **empty** for S3 |
| `prefix` | Restricted key prefix; path traversal and non-canonical casing rejected |
| `publicReadBase` | Optional stable public HTTPS base for anonymous reads; validated URL |
| `credentialRef` | Names — never values — of environment variables |

Write endpoints are **derived**, never typed in: R2 resolves to
`https://<accountId>.r2.cloudflarestorage.com`, S3 to
`https://s3.<region>.amazonaws.com`. Arbitrary S3-compatible base URLs,
`localhost`, IP literals, and unknown providers are rejected before any
network call, and there is no `allow-any-endpoint` backdoor in production
configuration.

### Credential references

`credentialRef` supports `kind: "env"` with explicit environment-variable
**names**: `accessKeyEnv`, `secretKeyEnv`, and an optional `sessionTokenEnv`.
Names must match `[A-Z][A-Z0-9_]{0,127}` and be pairwise distinct. There is no
shared-file, IMDS, container, or instance-metadata credential discovery: if a
named variable is unset in the pushing shell, resolution fails loudly.

### Repository bindings

Each binding selects a writer/reader pair by **immutable context** —
`chainId` + `suiteDirectory` + `repoId` — so a rename or ownership transfer
can never silently select the wrong bucket, and a storage choice can never
change the network profile or Directory.

Validation is enforced locally, before any cloud access:

- `version` must be `1`; at most 128 bindings per file.
- Writer and reader must be **different** profiles.
- The writer must carry a `credentialRef`.
- The reader must carry a `credentialRef` **or** a `publicReadBase`.
- If the reader has a `credentialRef`, it must not share an `accessKeyEnv` or
  `secretKeyEnv` with the writer.

**Where does `repoId` come from?** It is the on-chain repository id returned by
`RepositoryCore.resolveRepository(owner, name)` — read it with any EVM client
(for example the *Read Contract* pane on the RepositoryCore address in
[testnet Blockscout](https://testnet.blockscout.injective.network/)). If a
binding is missing, a v4 push stops before uploading with:
`no storage profile bound to this repository (chainId/directory/repoId); run
igit storage add`.

### Register and validate

```console
$ igit storage add ./storage-config.json
storage profile registered (path only; credentials are referenced, never stored)

$ igit storage doctor ./storage-config.json
PASS: local storage configuration (2 profiles, 1 repository bindings). ...

$ igit storage show ./storage-config.json
```

`add` stores only the file's absolute path in `~/.igit/config.json`
(`storage_config`). `doctor` and `show` read the explicit file only — no
credential resolution, no cloud requests, no bucket-policy or CORS changes.
Cloud reachability, CORS, and Git integration are tested separately, by
design.

## The manifest protocol

### Canonical JSON

Manifest bytes are **RFC 8785 JCS** canonical JSON: UTF-8 without BOM, keys
JCS-sorted, arrays in protocol order, duplicate keys / invalid Unicode /
`NaN` / trailing data / unknown fields rejected. Values that can exceed the
JavaScript safe-integer range (`chainId`, byte `size`) are decimal strings;
small counters are bounded integers. Go's default marshaling and a hand-rolled
"sorted stringify" are *not* automatically JCS — the project pins audited
libraries and cross-checks Go and TypeScript byte-for-byte on shared vectors
(`protocol/packmanifest/vectors.json`, generated by the real implementations).

### Schema 1 (frozen)

```text
PackManifest {
  schema: "igit.pack-manifest", schemaVersion: 1,
  chainId, suiteDirectory, repoId, refName,
  commit: { algorithm: "sha1", oid },
  packs: [ PackEntry, … ]
}
PackEntry { sequence, sha256, size, format: "git-pack", packVersion: 2,
            thin: false, dependsOn: [], locations: [ PackLocation, … ] }
ManifestCommitment { sha256, size, bootstrapLocator }   // outside the hashed body
```

Every field is mandatory. `manifestDigest = SHA256(UTF8(JCS(body)))` — the
digest never hashes itself, and the manifest's own key/URL stays outside the
body. Object keys are digest-derived:

```text
<prefix>/packs/sha256/<64-lowercase-hex>.pack
<prefix>/manifests/sha256/<64-lowercase-hex>.json
```

A `PackLocation` is `{provider, url, reader}` with all three fields present
and exactly one of `url` / `reader` non-empty: public locations carry a stable
HTTPS URL (`provider` ∈ `aws-s3`, `cloudflare-r2`, `ipfs`); authenticated
locations carry only a local reader label (≤64 chars). The manifest stores no
private bucket names and no secrets. An IPFS CID is addressing information —
it can never substitute for the raw SHA-256.

### Schema 2 — incremental packs (ADR 0005)

A fast-forward push appends exactly one pack containing only objects absent
from the ref's own chain; each entry declares `dependsOn` referencing earlier
entries of the same manifest only, validated as backward-only, acyclic, and
bounded. Never git thin packs, never `--fix-thin` before digest verification.
Empty increments reuse the pack set with a new commit binding and
`revision + 1`. Anything that breaks the chain — non-fast-forward update,
missing or corrupt previous manifest, the 16-pack or total-size budget —
falls back to one fresh self-contained full-history pack. **Force push never
waives the revision CAS.** The chain is unaware of `schemaVersion`; schema 2
changed no contract and no `suiteVersion`.

### Limits

| Limit | Value |
|---|---|
| Manifest bytes / JSON depth | 64 KiB / 16 (mirrored on-chain by `MANIFEST_MAX_SIZE = 65_536`) |
| Packs per manifest / locations per pack | 16 / 4 |
| Pack bytes / manifest-declared total | 512 MiB / 2 GiB (web schema-2 budgets: 32 MiB per pack, 256 MiB total) |
| Single PUT | ≤ 16 MiB on both providers |
| Multipart | AWS only; 8 MiB parts, ≤ 512 MiB. R2 over-limit objects are rejected **before** any request or source-file IO — no multipart fallback |
| Retries | ≤ 3 attempts per single PUT/GET/UploadPart with 100/200 ms backoff; cancellation stops immediately |

## The read-back verification rule

The first-release writer policy is unconditional: after a conditional create,
the object is **read back in full (streaming GET)** and its raw SHA-256 and
byte length are verified — for packs and manifests alike, including objects
that already exist at the target key. Metadata SHA-256 and ETags may assist
diagnostics but are never accepted as proof; HEAD never replaces GET.
Verification happens **before** Git repairs or ingests bytes, and the public
path additionally verifies anonymous reads. A matched conditional-create
collision (412) is reused idempotently only if the read-back matches exactly;
a mismatch is treated as corruption, never overwritten.

## Public read and CORS

- The standard public path is **anonymous HTTPS GET** on stable URLs
  (`publicReadBase` or the bucket's public domain), with no cookies, no
  authorization headers, and no signed query parameters.
- The **web reader fetches packs from the browser**, so the bucket must answer
  CORS preflight from the application origin. A CLI-only workflow does not
  need CORS; the web reader does. `GET` success does not imply CORS success —
  the diagnostics report them separately.
- `r2.dev` domains are development conveniences, not a production SLA; use a
  configured public domain.
- If public URLs are not anonymously reachable, the web explains that public
  read configuration or an independent CLI reader is required — it never asks
  for the writer secret, and the first release ships no web-held secrets, no
  managed GET broker, and no automatic credential distribution.

## Credential and permission rules

- Cloud write credentials are supplied as **named environment variables** in
  the pushing shell; they never enter the storage file, `~/.igit/config.json`,
  chain state, manifests, Git remotes, logs, or browser storage.
- A maintainer needs **two independent authorizations**: on-chain ref
  permission, and cloud PUT/GET on the designated bucket/prefix. A
  collaborator needs only their own GET (and necessary HEAD) — never the
  uploader's long-lived keys.
- Default grants should not include `DeleteObject`, bucket-policy changes, or
  bucket listing; multipart permissions are scoped separately. Do not assume
  R2 token scope granularity equals AWS IAM prefix limits — isolate by
  bucket/prefix according to what the provider actually enforces.
- Rotate writer keys independently of reader keys. The CLI scans pushed refs
  for credential-shaped content and warns; treat the warning as a safety net,
  not a licence.

## Failure and recovery contract

| Failure | Required behavior |
|---|---|
| Conditional create returns 412 | Read the key back; exact match ⇒ idempotent reuse; mismatch ⇒ conflict/corruption, never overwrite |
| 409 / 429 / 5xx / timeout | Bounded backoff and retry budget; AWS `CompleteMultipartUpload` 409 rebuilds the session per official semantics instead of blind retries |
| Auth failure / reader not configured | Actionable configuration message; never degrade to public writes, never request the writer secret |
| Wrong digest/size, truncation, trailing bytes | Reject the location; try other declared locations of the same pack; if all fail, do not ingest and do not move the ref |
| Pack uploaded, manifest failed | Ref unchanged; objects retained and reused after re-verification on the next attempt |
| Upload succeeded, ref CAS conflict or revert | Objects retained; re-read the ref; repack/re-send is an explicit choice — **no automatic force** |
| Broadcast sent, receipt unknown or reorg | Retain tx hash/nonce/expected ref; query the receipt and canonical state; never re-broadcast with a new nonce; never mark success or GC |
| Cloud object deleted or overwritten out-of-band | Report unavailable/corrupt; hashing cannot resurrect missing bytes — this does not legitimize different chain-committed content |
| Ref delete / force push / rename / fork | Only authorized chain state changes; shared content is never auto-deleted; per-ref independence is a hard gate |

## Operational checklist

| Concern | Practice |
|---|---|
| Bucket lifecycle | Never expire `packs/` or `manifests/`; there is no automatic reaper, and none may be enabled |
| Public read | Must stay enabled for browsers and anonymous clones |
| CORS | Must allow the application origin |
| Backups | The chain stores commitments, not bytes — back up the bucket |
| Cost | You own the bucket: storage, requests, egress, and verification read-backs are yours |
| Tampering | Out-of-band overwrite is detectable (digest mismatch) but not preventable by the protocol; bucket-administrative discipline is an operational responsibility |

## Status boundaries

Delivered and **PASS**: the successor suite on Injective testnet, CLI/Web
version dispatch, canonical-JSON cross-vectors, and real Cloudflare R2
end-to-end Git flows (push / clone / fetch / ls-remote / tag / ref delete /
tombstone rebuild) with anonymous public GET + CORS verification, plus a real
testnet + R2 incremental (schema 2) push. **NOT PROVEN**: the real AWS S3
canary, real force-push staleness and concurrency races, R2 out-of-band
tamper detection, Blockscout source verification for the successor, and
Foundry gates. Do not soften these labels; see
[project status](project-status.md).

## Next

- The v4 walkthrough → [Chapter 04 · Suite v4](manual-suite-v4.md)
- The contract side of commitments → [Chapter 07](manual-protocol-contracts.md)
- Storage commands in the CLI reference → [Chapter 02](manual-cli-reference.md)
- Something is failing → [Chapter 10 · Troubleshooting](manual-troubleshooting.md)

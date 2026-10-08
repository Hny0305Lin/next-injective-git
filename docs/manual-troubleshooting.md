# 10 · Troubleshooting and FAQ

Status: manual chapter. Audience: users and developers. Last updated: 2026-10-05.

Every symptom below is stated as **symptom → cause → fix**. Run the two
diagnostics first; they resolve most situations on their own:

```console
$ igit doctor            # or: igit doctor --clone / --push / --json
$ igit suite verify      # or: igit suite info --json
```

## Installation and PATH

**`git clone igit://…` fails with "not a git command" / "unknown remote
helper".**
`git-remote-igit` is not on `PATH`. Git discovers remote helpers by the
`git-remote-<scheme>` naming convention. Install both binaries from the same
release and re-open the shell so `PATH` changes take effect.

**`igit` runs, but push/clone behave like plain Git.**
Unknown subcommands are forwarded to Git by design. Confirm you spelled the
subcommand exactly (`clone`, `push`, `pull`); `igit help` prints the full
command surface.

**Windows reports access denied under `~/.igit/`.**
The keystore and config are written under a protected DACL (current user and
SYSTEM only). Do not relax the permissions; run as the same user that created
the files, or move the keystore with `evm_keystore_dir` and re-protect.

## Suite verification

**`igit suite verify` fails with a chain-ID mismatch.**
The configured RPC belongs to a different network than `evm_chain_id`. Reset
the profile atomically — `igit config set network injective-testnet` — rather
than editing fields individually; a mixed-network configuration is exactly
what profile selection prevents.

**"SuiteDirectory has no code" / version predates the first EVM suite.**
The configured address is not a deployed Suite on this chain (wrong network,
typo, or a v1-era address). Use the documented testnet addresses —
`0xf987396475d0a4c96b722e993a95d8720a6292ad` (v4) or
`0xf8844F90887731FFd607E1f59e39a3918F6eAb35` (v3) — and remember the CLI
points at exactly one Directory at a time.

**Verification is slow against the public testnet RPC.**
Full verification is a long chain of dependent reads (about a minute against
the public RPC). Results are cached for 30 seconds. A first success after a
timeout is normal — retry once before investigating further.

**The web shows "EVM Suite is not configured" or "verification failed".**
On public deployments the Settings form is read-only by design; the built-in
profile supplies the addresses. A verification failure there means the
configured Directory is not a valid active Suite for the network — re-check
the address in Settings (local deployments only).

## Keys and gas

**"missing config: key_name".**
Create or import a key first: `igit key new <name>` or
`igit key import <name>`, or pass `--create-key <name>` to `igit setup push`.

**Transactions are rejected for insufficient funds / low gas price.**
Fund the signing address shown by `igit key show` with testnet INJ
([faucet](https://testnet.faucet.injective.network/)). All writes enforce a
minimum gas price of `160000000 wei`; the client applies the floor
automatically when the RPC quotes less.

**A write finished with "receipt was not confirmed".**
The transaction was broadcast but its receipt did not land within the ~2
minute window. The CLI retains the transaction hash and discards its cached
nonce. Look the hash up on
[testnet Blockscout](https://testnet.blockscout.injective.network/); if it
reverted or never landed, simply retry the command — do not attempt manual
nonce juggling.

## Suite v3 (IPFS) push and read

**`igit doctor --push` reports `Kubo CLI` / `local Kubo API` as FAIL.**
v3 push requires the pinned local Kubo. Re-run `igit setup push` (or
`--force`); the API must be loopback-only (`http://127.0.0.1:5001`). If you
are deliberately working on Suite v4, run `igit setup push --no-kubo` and
expect these checks to SKIP.

**Push fails while requesting upload authorization or replication.**
The v3 rail depends on the configured upload service endpoints and the
read/write gateways. Check `igit gateway status`, and verify
`upload.endpoint` / `upload.authorization_endpoint` appear in
`igit config list --internal` (defaults come with the network profile).

**Clone/fetch is slow or times out on a pack.**
The helper health-ranks the `hk`/`us` gateways and falls back to `ipfs.io`.
Inspect with `igit gateway status` / `igit gateway select`; a pack that is
only cold on the public fallback may need a retry while the hot tier fetches
it.

## Suite v4 (BYOS) push and read

**"no storage profile bound to this repository (chainId/directory/repoId);
run igit storage add".**
The storage reference file has no binding for this repository. Read the
repository's `repoId` from `RepositoryCore.resolveRepository(owner, name)`
(e.g. Blockscout's Read Contract pane), add a `repositories` entry with
`chainId` as a decimal **string**, and re-run `igit storage add`.

**`igit storage doctor` fails on a file you believe is correct.**
The checks are strict on purpose: profiles are a **map** (not a list), writer
and reader must be different profiles, the writer needs a `credentialRef`,
the reader needs a `credentialRef` or a `publicReadBase`, reader credentials
must not reuse the writer's variable names, S3 needs a commercial region and
no `accountId`, R2 needs `region: "auto"` and a 32-hex `accountId`. Compare
against the schema in [Chapter 08](manual-byos-storage.md).

**Push fails with a credential resolution error.**
A named environment variable is unset **in the shell running the push**.
Export both `accessKeyEnv` and `secretKeyEnv` (and `sessionTokenEnv` if
declared) in that shell and retry.

**Push reverts with `CommitmentMismatch`.**
The ref moved between your read and your write. Fetch, re-apply, push again.
Do not force blindly — on v4, force never waives the CAS.

**Read-back digest or size mismatch after upload.**
The object stored is not the object uploaded. Look for a bucket lifecycle
rule, a transforming proxy, or an intermediary rewriting objects. The client
refuses to publish a ref to bytes it could not verify.

**The web shows a repository but files/commits fail to load on v4.**
The manifest or packs are not anonymously reachable, or CORS blocks the
browser. Ensure `publicReadBase` (or bucket public read) answers plain GET
and the bucket's CORS allows the application origin. The CLI reader will
still work through its own reader profile.

**"Invalid manifest JSON, schema, context, commitment or limits" in the
web.**
The browser bundle is older than the manifest schema (schema 2 landed
2026-10-05). A hard refresh picks up the deployed front end; the CLI from the
same release reads both schemas.

## Cloning and URLs

**"invalid remote URL … expected igit://<owner>/<repo>".**
Owner must be an `inj1…` address or a registered username (3–32 chars,
`[a-z0-9-]`, not starting with `inj1`); Git flags go **after** the repository
argument (`igit clone owner/repo -q`), because the first positional selects
the repository.

**The same `<owner>/<repo>` opens a different repository than expected.**
The name exists in more than one generation. In the web, add `?suite=4`
(latest EVM) or `?suite=3` (earlier EVM), or use the archive path for V1. In
the CLI, point `evm_suite_directory_address` at the matching generation.

## Web and wallets

**No supported EVM wallet was detected.**
Install one of the supported EVM wallets (MetaMask, Rabby, OKX Wallet, Bitget,
Trust, Coinbase, Brave, Keplr's EVM provider, Compass) or use WalletConnect.
A Cosmos-only wallet cannot sign the EVM path.

**Connect succeeds but every write says wrong network.**
The session must be on Injective EVM chain `1439`. The app attempts to switch
or add the chain automatically; if the wallet refuses (`4902` path failed),
add the network manually: RPC `https://k8s.testnet.json-rpc.injective.network/`,
chain ID `1439`, symbol `INJ`, explorer
`https://testnet.blockscout.injective.network/`.

**WalletConnect QR cannot complete the EVM session.**
The paired wallet must advertise `eip155:1439`. Keplr Mobile currently does
not, and is therefore not offered as compatible on this path.

**Search finds nothing for a repository you just pushed.**
The browser builds its repository index from chain events and direct contract
reads; give it a moment or search `owner/name` directly. The contract read is
always authoritative — the index is navigation-only.

## CosmWasm v1 archive

**An archived repository shows "Migration to EVM V2 or newer required".**
Expected. The V1 archive is read-only; editing and current features require a
future migration that is not implemented. Browse, clone history out of it,
and continue work on an EVM generation.

**The archive page intermittently fails to load.**
Archive reads go through Cosmos LCD endpoints with bounded retries and a
community fallback; the official sentry drops a share of browser
connections. Retry, or verify the same fact yourself with
`igit archive query --lcd … --contract inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh
--height <N> '<query>'`.

## FAQ

**Is igit a GitHub replacement?**
It is decentralized code hosting that keeps the Git workflow: ordinary
`git push` / `git clone` / `git fetch` through the `igit://` remote helper,
plus repository features (collaborators, transfers, guardians, moderation,
sponsorship, splits, usernames, badges, release checksums) on Injective. It
does not have issues/CI/pull-request workflows today.

**What actually lives on chain?**
The control plane: repository identity, metadata, refs (commit pointers and
storage commitments), permissions, moderation, economics. Packfile **bytes**
live in the data plane — IPFS (Suite v3) or your own S3/R2 bucket (Suite v4).

**Which generation should I use?**
Suite v4 ([Chapter 04](manual-suite-v4.md)) for anything new — no Kubo, no
IPFS dependency, your own bucket. Suite v3 if you maintain an existing IPFS
repository. V1 is read-only history.

**Can the same repository name exist in several generations?**
Yes — they are distinct repositories with distinct repoIds. That is exactly
what the `?suite=` URL parameter disambiguates in the web application.

**Is mainnet supported?**
Not yet. A mainnet profile exists in the code, but published profiles keep
`SuiteDirectory` empty until deployment, migration verification, security
review, and cutover evidence are approved. Testnet is the live environment.

**Who pays for storage?**
On v4, you do — it is your bucket (storage, requests, egress, and
verification read-backs). On v3, packs ride the project's IPFS data plane.

**Is my private bucket a private repository?**
No. Private buckets are not anonymously readable, so the web cannot render
them; authenticated reads require your own CLI reader profile. Private
repositories and end-to-end encryption are future scope, not current
features.

**Can I use an S3-compatible provider other than AWS or Cloudflare R2?**
No. Production supports `aws-s3` and `cloudflare-r2` only. The mainland-China
providers in the roadmap document are roadmap candidates — not supported, not
integrated — and must not appear in any configuration.

**Is there gas sponsorship?**
No. Every write is paid by its signer (minimum gas price `160000000 wei`).
The `sponsor` command is repository revenue for maintainers, not transaction
fee sponsorship.

**What do PASS / FAIL / BLOCKED / NOT PROVEN / HISTORICAL mean?**
The project's bounded status vocabulary. `PASS` — verified with retained
evidence. `FAIL` — attempted and failed. `BLOCKED` — cannot be attempted in
the current environment. `NOT PROVEN` — not demonstrated yet, however likely
it seems. `HISTORICAL` — a frozen past record that is not current acceptance.
The labels are used verbatim and never softened; see
[project status](project-status.md).

## Next

- Full command surface → [Chapter 02 · CLI reference](manual-cli-reference.md)
- Web screens → [Chapter 03 · Web guide](manual-web-guide.md)
- Contract model → [Chapter 07](manual-protocol-contracts.md)
- Manual home → [manual.md](manual.md)

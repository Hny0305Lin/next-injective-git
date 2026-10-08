# Repository Guidance

Next Injective Git stores control-plane state in a non-upgradeable Injective EVM
Suite and stores packfiles through version-dispatched data planes: Suite v3
uses the legacy IPFS/Kubo adapter, and Suite v4 uses BYOS object storage
(Amazon S3 / Cloudflare R2 only). Ordinary clients trust only one configured
`SuiteDirectory`; never add a direct module address, compatibility backend,
legacy fallback, proxy, diamond, or `delegatecall` path.

**Current Project Status:** V4 BYOS delivered (2026-10-05): the successor Suite
(suiteVersion 4) is deployed and active on Injective testnet, the CLI and Web
dispatch by on-chain suite version, and real R2 end-to-end Git flows plus Web
browsing work without Kubo/WSL2/`injectived`. Remaining open items: real AWS
canary, Foundry gates, successor publication evidence, security review, and
mainnet governance approval. Incremental packs (S08) are delivered and
verified with a real testnet/R2 incremental push
([ADR 0005](docs/adr/0005-incremental-packs-via-manifest-schema-2.md);
manifest schema 2 on Suite v4, no contract change). See
[docs/project-status.md](docs/project-status.md)
(English) or [docs/project-status-zh.md](docs/project-status-zh.md) (中文) for
a comprehensive status overview.

## Why EVM V2

CosmWasm V1 Push ran natively on Linux, but the supported Windows workflow
required a WSL2 compatibility environment for the Linux CLI, `injectived`, and
Kubo. EVM V2 is specifically the second-generation control plane built on
Injective EVM so ordinary Windows operation can use the native Windows CLI,
encrypted EVM keystore, JSON-RPC, and native storage tooling. WSL2 and
`injectived` are not prerequisites for the EVM V2 product path.

The EVM V2 product-generation name is distinct from the on-chain Suite protocol
version. Suite v3 (IPFS) and Suite v4 (BYOS successor) coexist; clients select
the reader/writer path from the on-chain `suiteVersion()`. Completion requires
clean native Windows and Linux acceptance; source code or CI configuration
alone is not evidence. See
[ADR 0001](docs/adr/0001-evm-v2-runtime-and-migration-scope.md).

Storage is split by suite version and must stay split: v3 is the frozen
IPFS/Kubo legacy path, and v4 is the delivered BYOS successor whose cloud
providers are limited to Amazon S3 and Cloudflare R2 (see
[ADR 0002](docs/adr/0002-pluggable-pack-storage.md) and
[ADR 0004](docs/adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)).
MinIO, other clouds, arbitrary S3-compatible endpoints, and self-hosted object
stores are out of scope for v4 production configuration. Do not retrofit bucket
storage onto v3 or IPFS onto v4.

## Source Boundaries

- `contracts/evm-v2`: nine Solidity contracts, fixed `solc 0.8.24`, checked ABI
  and deployment artifacts (Suite v3, IPFS `packUris`).
- `contracts/evm-v2-successor`: Suite v4 (BYOS) — commitment-shaped
  `RepositoryCore` (manifest digest/size/bootstrap locator + revision CAS) and
  a `suiteVersion = 4` Directory; all other modules are byte-identical to v3.
- `cli`: Go CLI, Git remote helper (v3 IPFS / v4 BYOS dispatch), EVM
  transactor, deployment tooling, and deterministic offline migration tooling.
- `web`: React/Vite and viem. Wallet sends must specify legacy type, estimated
  gas, and gas price at least `160000000 wei`, then verify receipt status.
- `archive/cosmwasm-v1`: isolated read-only historical source and evidence
  tooling. Never import it into ordinary runtime, CI, or release paths.

Published profiles intentionally leave `SuiteDirectory` empty until real
deployment and cutover evidence is approved. Do not add an address from a test,
example, source artifact, or unreviewed deployment.

## Documentation System

Documentation lives in a two-repository layout. This repository is the
content source of truth (`docs/`, plus the canonical rules in this file);
the bilingual Docusaurus 3 site (Node >= 24) lives in the separate
`next-injective-git-docs` repository (`docs-site/` toolchain plus a
read-only mirror of this repo's `docs/`). Never move, rename, or rewrite
existing `docs/` files here (new files are allowed). English is the single
source of truth; Simplified Chinese is served through Docusaurus i18n under
`/zh/` and must never run ahead of the English status wording.

- Content flow: edit `docs/` in this repo → in the docs repo run
  `npm run sync:docs` (mirrors `docs/`; the docs repo's `docs/` folder is
  generated output, never hand-edited) → `npm run build` → commit and push
  to `next-injective-git-docs` `main`, which triggers its CI and Vercel
  deployment. This repo's `.gitignore` intentionally excludes `docs-site/`.
- Adding/changing a page: author the English page in `docs/`, register it in
  the docs repo's `docs-site/sidebars.js` (explicit list; no front matter on
  evidence files), run `npm run gen:zh-stubs`, then replace the stub with a
  real translation. Check `docs/glossary.md` before translating; if a term
  is missing, extend the glossary first. Untranslated pages must keep an
  explicit "Translation in progress" stub - never silently missing.
- Terminology boundaries: Suite v3 = legacy IPFS/Kubo; Suite v4 = BYOS
  successor (production configuration: Amazon S3 and Cloudflare R2 only).
  The six mainland-China S3-compatible providers tracked in
  `docs/roadmap-byos-providers.md` are **roadmap candidates only**: no page
  (any language) may describe them as supported/integrated, and their
  endpoints must not appear in configuration examples, CLI flags, or
  production guidance until implementation and approval evidence lands.
- Brand assets: docs repo `docs-site/static/img/` (copy of the read-only
  original `D:\inj\igit-image.png`). Site images stay local in `static/`; the
  only remote-image exception is contributor avatars from
  `avatars.githubusercontent.com`.
- Contributors: `docs-site/src/generated/contributors.json` (docs repo) is
  generated only by `npm run fetch:contributors` (build-time GitHub API
  against this repository, optional `GITHUB_TOKEN` env var; never hand-edit,
  never fetch at browser runtime; on failure fall back to the committed
  cache, else hide the homepage section).
- CI gate: the docs repo's `.github/workflows/docs.yml` builds the site on
  changes to `docs-site/**`, `docs/**` (the mirror), and itself. Do not
  modify this repo's `ci.yml`/`release.yml` or Required Checks scripts for
  documentation work.
- Deployment: user-operated Vercel (imports `next-injective-git-docs`) +
  Cloudflare DNS per the docs repo's `docs-site/DEPLOYMENT.md`. Agents
  produce static build output, commits, and guidance only - no Vercel/
  Cloudflare account operations, no deployment credentials.
  `archive/cosmwasm-v1` never enters site navigation, build dependencies,
  or runtime paths.

## Required Checks

```sh
(cd cli && go vet ./... && go test ./...)
(cd web && npm ci && npm run test:api && npm run typecheck && npm run build)
npm ci --prefix contracts/evm-v2
bash scripts/evm-v2-check.sh --required
bash scripts/suite-readiness.sh --required
bash scripts/migration-cutover-readiness-test.sh
```

CI also runs Go race tests, native Windows tests, source/profile guards, and
release asset verification. A missing compiler or Foundry executable must fail
required gates rather than become a passing skip.

## Safety Properties

- The client verifies chain ID, suite version (3 or 4), active state, every
  module code hash, and every module binding before reads or writes.
- One `EVMTransactor` owns nonce lookup, gas estimation, legacy signing,
  broadcast, and bounded receipt polling. An uncertain receipt returns the tx
  hash and invalidates cached nonce state.
- Push pins the pack durably before updating the ref: v3 pins via Kubo, v4
  uploads to the user's S3/R2 bucket and verifies a full read-back before the
  CAS ref update. A failed transaction must retain retryable content. A force
  push publishes a self-contained pack; on v4, force never waives revision CAS.
- Moderation hooks are mandatory for Core ref mutation and Economic sponsor
  mutation. Recovery is the only ownership-recovery capability.
- Snapshot imports are ordered, bounded, payload-hashed, rolling-committed, and
  finalized once. Directory activation occurs only after all modules verify.
- Username migration requires all historical deposits refunded and escrow
  balance zero. Pending temporal operations are re-created after cutover.
- Deployment, plans, manifests, journals, receipts, fixed-block state, and
  approval are immutable evidence. Never overwrite or manufacture them.

## Coding Conventions

User-visible CLI messages are bilingual through `i18n`. Read-only commands use
contract-only validation and must not require a key. Git-compatible unknown
commands may be forwarded to Git; inspect command dispatch before adding names.

Use checked-in ABIs through go-ethereum or viem. Do not hand-code selectors or
word decoders. Preserve the v3 IPFS gateway behavior and the v4 BYOS path
exactly as dispatched by suite version. V4 BYOS supports only Amazon S3 and
Cloudflare R2; do not add or claim MinIO, other clouds, or generic
S3-compatible/self-hosted endpoints from roadmap documentation alone.

Deployment is restricted to rotated encrypted testnet keys. Production needs a
separate multisig/timelock governance design. Never deploy or broadcast merely
to satisfy a test or documentation task.

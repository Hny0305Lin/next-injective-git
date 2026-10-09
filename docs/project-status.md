# Project status: reconciled evidence entry

Current status (2026-10-05): **Suite v4 (BYOS) delivered.** The successor
suite (`contracts/evm-v2-successor`, suiteVersion 4) is deployed and active on
Injective testnet (Directory `0xf987396475d0a4c96b722e993a95d8720a6292ad`,
evidence `local-only/successor-deploy/deployment5.json`); CLI and Web dispatch
by on-chain suite version (v3 → IPFS legacy, v4 → verified BYOS path); a real
Cloudflare R2 end-to-end Git flow (push/clone/fetch/ls-remote/tag/ref-delete/
tombstone rebuild, anonymous public GET+CORS, no Kubo/WSL2/`injectived`) and
Web repository browsing are working. Incremental packs (S08, manifest schema
2, no contract change) are delivered and verified with a real testnet/R2
incremental push ([ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md)).
BYOS cloud providers are limited to
AWS S3 and Cloudflare R2. Still open: real AWS S3 canary, real force-push/
concurrency races, Blockscout verification, Foundry gates (R04), successor
publication evidence, security review, and mainnet approval — see
[backlog](backlog.md) 2026-10-04/05 sections and
[suite version compatibility](suite-version-compatibility.md).

以下为 2026-09-13 审计的摘要入口（历史快照，反映当时的检查结果）。

复核日期：2026-09-13（Asia/Shanghai）；基线文件名按任务指定保留 2026-09-12。
源码：`dev` / `0ba06f436558f12d97625b393767440cdd0f9862`。

当前事实只以 [唯一事实基线](reconciliation-baseline-2026-09-12.md) 为准；本页是入口或摘要，不是另一份验收报告。
状态限定为 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL，定义和原始命令输出见基线。

The single current source of status is the linked baseline. Local checks, live read-only RPC, historical deployment evidence, and unproven product acceptance are explicitly separated.

| 核对对象 | 状态 | 已证实的范围 / 剩余问题 |
|---|---|---|
| 九合约源码、solc 0.8.24、ABI/artifact、生产尺寸 | PASS | 九个生产合约均存在；`npm run check` 通过。测试合约 SuiteProtocolParityTest 有 25537 > 24576 字节警告。 |
| Foundry build / test / gas | BLOCKED | 当前 PATH 无 forge；未执行 Foundry 验收，不能用 solc 代替。 |
| 测试网 Suite 固定区块只读核对 | PASS | chain 1439；区块 139852506 / 0x855fada；state=1(active)，version=3；七模块地址和 code hash 一致。 |
| Go 本地测试、vet | PASS | `go test -count=1 ./...`、`go vet ./...` 退出 0；包括单元和 mock/fixture，不能推导真实写入。 |
| CLI 限定代码只读 RPC | PASS | 既有 evm-demo-inspect 的 code-addresses 模式真实读取九个合约；latest 模式，不能冒充固定区块完整 VerifySuite。 |
| 普通 igit suite verify | BLOCKED | 隔离无密钥配置遇到 Windows DACL Access is denied；详细 inspect 也在 username progress 处超时。 |
| Web typecheck / API / build | PASS | 73 tests passed / 0 failed；构建改用全新 ignored 输出目录且关闭 emptyOutDir，有 Vite/PURE warning。 |
| Web Moderation UI 完成声明 | FAIL | 文档声称 Moderation UI 已实现，但对应文件实际不存在。三个指定文件均未创建；modules.ts API 不等于 UI。 |
| 四个 EVM 索引脚本 | FAIL | 仍为 untracked；错误 Topic、SHA-256 模块 ID、错误 selector、placeholder ABI、checkpoint/reorg/unpin 缺陷。 |
| 当前本机真实 Kubo / recursive pin | BLOCKED | PATH 无 ipfs、未观察到节点/监听，127.0.0.1:5001 拒绝连接；生命周期测试 SKIP。 |
| 真实 replication、gateway pack、Git 完整性、EVM 写入、push/fetch E2E | NOT PROVEN | 本轮没有写交易或真实 Kubo 上传；mock 不作为闭环证据。 |
| 过去部署、激活、Blockscout 记录 | HISTORICAL | 证据绑定 4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986；当期收据不等于当前产品验收。 |
| 九合约完整验收、产品切换、生产部署 | NOT PROVEN | 缺 Foundry、真实 E2E、finality、安全审查、批准及 checksum 门禁。 |

IPFS + EVM + CLI 完整闭环尚未验证。未取得可复现的全项目 91% coverage 证据；撤销该数字作为当前指标的效力。

公开内置 CLI/Web profile 的 Directory 为空；Web `loadConfig()` 在本地 origin 会使用 `0xf8844F90887731FFd607E1f59e39a3918F6eAb35`，并可载入已有 localStorage Suite 设置，因此“所有路径都为空”不成立。该地址不是本轮核对的 Directory，本轮未查询它。

## Confirmed First-Release Scope (2026-09-13, Not New Acceptance Evidence)

First mainnet directly targets a storage-neutral successor with user-owned
AWS S3 / Cloudflare R2 buckets, not an IPFS-only v3 intermediate launch.
Self-hosted object stores, MinIO, other clouds and arbitrary S3-compatible
write endpoints are excluded. Repositories are public; bucket owners pay
their cloud costs, manifests use canonical JSON, readers use independent
local configuration and dual replicas are not mandatory. Private repositories,
E2EE and managed brokers are later scope. The BYOS path must not depend on
Kubo, the IPFS network or iGit IPFS services; legacy v3 stays explicit.
Content edits publish new pack/manifest/ref commitments, never different bytes
at an existing digest-derived key.

Implementation status (updated 2026-10-05): the decision above is delivered as
Suite v4 — successor suite deployed on Injective testnet, real R2 end-to-end
Git flows PASS, Web reads working (see [backlog](backlog.md)). Still NOT
PROVEN: the real AWS S3 canary, remaining real-layer residuals, successor
publication evidence, security review, and mainnet acceptance.
See [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md),
[implementation specification](storage-byos.md), [S01–S08 backlog](backlog.md),
and [suite version compatibility](suite-version-compatibility.md).

以下文件保留原样并统一标记为：**“历史迁移草稿/未对齐报告，不作为当前项目状态依据。”**

- 根目录 MIGRATION-COMPLETE.md、MIGRATION-REPORT-P1P2.md、A01-BXX-MIGRATION-REPORT.md，以及其余 MIGRATION-/PRIORITY- 草稿。
- docs/a11-storage-indexer-v2.md、docs/PRIORITY-MAPPING.md。
- docs/evm-v2-handoff.md、docs/liveagent-evm-v2-context.md、docs/project-knowledge-base-zh.md 中的旧状态快照属于 HISTORICAL；架构描述需按当前代码逐条引用。

旧文档中的 P1.1/P1.2/P1.3 分别被用于“operator/部署/激活”和“Moderation/索引/ABI”两套含义，不能互换。旧问题使用 R01–R08；新增存储任务使用 S01–S08。

Current acceptance is NOT PROVEN. Foundry, native Kubo, and the standard CLI configuration path are BLOCKED in this environment. Real transactions and production deployment are outside this audit authorization. See [the independent follow-up task list](backlog.md).


---

<details>
<summary>HISTORICAL：本轮入场前原文（已失去当前状态依据效力，完整保留未提交内容）</summary>

以下为历史迁移草稿/未对齐报告，不作为当前项目状态依据。原文 SHA-256：`d0a5c36f792ca4a8a1487dcc05d3699e58f14c8ceb042eebb5475998f1f7014a`。下方所有旧状态、勾选、百分比、路径与执行指令仅作审计引用；正确状态以本页上方和唯一事实基线为准。

`````markdown
# Project Status Overview

- Status: P1.2 deployment evidence and P1.3 fresh-empty activation complete; P1.4-P1.6 acceptance in progress
- Last Updated: 2026-09-11
- Current Suite Evidence Baseline: `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`
- Latest Progress: 17 deployment/configuration transactions revalidated, 9/9 Blockscout verification complete, fresh-empty Suite activated
- Public Availability: None; checked-in Suite profiles remain intentionally empty

> [!IMPORTANT]
> This document provides a high-level status overview of the project. For detailed
> technical decisions, delivery sequencing, and evidence requirements, refer to
> the dedicated documents. Start with the [LiveAgent continuation context](liveagent-evm-v2-context.md),
> [Chinese project knowledge base](project-knowledge-base-zh.md), and [EVM V2 handoff](evm-v2-handoff.md).
> For Chinese status, see [项目状态总览 (中文)](project-status-zh.md).

## Project Overview

Next Injective Git (igit) is a decentralized Git storage system built on Injective EVM (EVM V2 generation):
- **Data Plane:** Git packfiles stored on IPFS (current) or S3/R2 (planned)
- **Control Plane:** Repository metadata, refs, permissions, moderation, economics stored in non-upgradeable Injective EVM smart contract suite
- **Platform Support:** Native Windows and Linux support without WSL2 or `injectived`

### Why EVM V2?

The V1 control plane used CosmWasm. It ran natively on Linux, but the Windows support path required WSL2 to host the Linux CLI, `injectived`, and Kubo. This successor is called **EVM V2** because moving the control plane to Injective EVM is the mechanism used to remove that Windows-only compatibility environment. The target path uses native Windows or native Linux tooling; ordinary EVM V2 users do not install WSL2 or `injectived`.

## Current Status Snapshot

### ✅ Completed Milestones

**P0: Windows and EVM Baseline Repair** (Completed 2026-08-19)

- **Reviewed Commit:** `f6dcee9aa67255bfdff1867785435022df7ec5e9`
- **CI Run:** [32215415044](https://github.com/Hny0305Lin/next-injective-git/actions/runs/32215415044)
- **Key Achievements:**
  - ✓ Native Windows support (no WSL2)
  - ✓ Native Linux support (no injectived)
  - ✓ Chinese Windows locale compatibility
  - ✓ Windows DACL file permission protection
  - ✓ Foundry test suite passing
  - ✓ Go race detector checks passing
  - ✓ Fixed `core.autocrlf=true` line-ending issues
  - ✓ Fixed mainnet RPC endpoint (updated to current official endpoint)
  - ✓ Implemented bounded Kubo download timeouts and failover

**P1.1: Operator Runner Implementation** (Completed 2026-08-22)

- **Location:** `cli/cmd/igit-suite-operator/`
- **Test Results:** 15/15 passing, 60.5% coverage
- **Code Quality:** go fmt/vet/build all passing
- **Key Components:**
  - ✓ Encrypted keystore with scrypt KDF (keystore.go)
  - ✓ Append-only signed journal with ECDSA signatures (journal.go)
  - ✓ Safe resume mechanism for uncertain receipts (main.go)
  - ✓ Manifest-driven calldata execution (manifest.go)
  - ✓ Fixed-block imported-state evidence emission
  - ✓ Comprehensive unit test coverage

### 🚧 In Progress

**P1: Suite V3 Testnet Deployment and Cutover** (In Progress - P1.1, P1.2, and P1.3 Complete)

**Blocking Factors:**
1. ✅ **Deployment Evidence Complete** - Nine creations and eight configuration calls are retained in no-clobber `deployment.json`
2. ✅ **Fresh-Suite Scope Complete** - V1 is archive-preview-only and all seven module import counts are zero
3. ❌ **Missing Product E2E And Wallet Receipts** - MetaMask, clean Windows/Linux Git, and finality evidence remain open
4. ❌ **Missing Independent Security Review And Approval** - `security-review.pdf` and `cutover-approval.txt` remain required

**Required Work (By Priority):**

#### P1.1 Operator Runner (✅ Complete - 2026-08-22)
- [x] Implement operator runner with encrypted keystore transactor
- [x] Implement append-only signed journal mechanism
- [x] Implement safe resume for uncertain receipts
- [x] Implement fixed-block imported-state evidence emission
- [x] Complete unit tests (15/15 passing, 60.5% coverage)
- [x] Pass code quality checks (go fmt, go vet, compilation)

#### P1.1b Migration Runner Validation (➖ Not Applicable To Current Cutover)
- The accepted `fresh-empty-suite` scope performs no V1 import and therefore does not require a migration dry run, manifest, or import journal
- The runner remains available for a separately approved future `cosmwasm-v1-migration`

#### P1.2 Deployment Execution (✅ Complete - 2026-08-29)
- [x] Deploy all nine contracts
- [x] Revalidate the ordered sender, nonce, initcode, and calldata for nine creation and eight configuration transactions
- [x] Revalidate historical runtime code, immutables, and Directory bindings
- [x] Verify all nine contracts on Blockscout
- [x] Generate `deployment.json` through the no-clobber historical-recovery mode
- [x] Generate independent `blockscout-verification.json`

#### P1.3 Fresh-Empty Activation (✅ Complete - 2026-08-29)
- [x] Bind `fresh-empty-suite` and the archive-preview-only V1 policy in `cutover-scope.json`
- [x] Verify zero expected/imported counts, batches, and sequences for all seven modules
- [x] Verify each expected root equals its deterministic empty rolling root
- [x] Record username escrow non-liability evidence
- [x] Atomically activate the Directory
- [x] Record final Directory state, code hashes, and module bindings at a fixed block

#### P1.4 E2E and Wallet Testing (Blocks Cutover)
- [ ] Execute MetaMask writes and record receipts
- [ ] Execute Git E2E on clean Linux environment
- [ ] Execute Git E2E on clean Windows environment
- [ ] Test uncertain receipt handling
- [ ] Verify the V1 archive preview remains read-only and isolated from ordinary EVM paths

#### P1.5 Security and Approval (Priority 1, Can Parallel - Blocks Cutover)
- [ ] Launch and complete deep source security review
- [ ] Resolve all security findings
- [ ] Obtain independent hash-bound cutover approval
- [ ] Generate `security-review.pdf`
- [ ] Generate `cutover-approval.txt`

#### P1.6 Evidence Collection and Gates (Final Step)
- [ ] Collect complete evidence directory
- [ ] Generate the final `cutover-evidence.sha256` checksum manifest
- [ ] Run `scripts/migration-cutover-readiness.sh`
- [ ] Verify all evidence passes gates
- [ ] **Only then** update testnet `SuiteDirectory` profiles

**Estimated Engineering Time:** P1.1-P1.3 are complete; P1.4-P1.6 now consist primarily of product E2E, finality, security review, independent approval, and the final checksum-bound gate.

### 📋 Planned Milestones

| Milestone | Est. Time | Dependencies | Primary Goal |
|---|---|---|---|
| **P2: Native Windows Product Acceptance** | 3-5 days | Depends on P1 | Release assets, installer, full Git workflow without dependencies |
| **P3: Verified Packstore Boundary** | 4-7 days | Can parallel P1 | Introduce `packstore` abstraction, preserve IPFS behavior, add streaming/verification |
| **P4: S3/R2 Successor Protocol** | 2-3 weeks | Depends on P3 | New Suite version, storage-neutral URI, AWS/R2 adapters |
| **P5: Historical Storage Migration** | 1 week | Depends on P4 | CID mapping, dual-read rollback window, full verification |
| **Z0: Isolated ZKP Testnet Prototype** | 3-5 days | Independent, can start after P0 | Membership proof, gnark Groth16, standalone experiment |
| **M0: Mainnet Candidate** | TBD | Depends on P1/P2, conditional P5, governance, finality, independent approval | Production deployment ready |

## Technical Architecture Status

### Core Components

```
contracts/evm-v2/          9 non-upgradeable Solidity contracts
├── SuiteDirectory         Directory and configuration coordinator
├── BootstrapCoordinator   Ordered batch import and activation
├── RepositoryCore         Stable repo IDs, refs, collaborators
├── RecoveryModule         Guardian proposals and ownership recovery
├── ModerationModule       Reports, appeals, mandatory policy hooks
├── EconomicModule         Native INJ sponsorship and historical totals
├── UsernameModule         Username claims and historical migration
├── BadgeModule            Non-transferable badges
└── ReleaseModule          Immutable release checksums

cli/                       Go CLI and Git remote helper
├── igit                   Main CLI
├── git-remote-igit        Git protocol adapter
├── igit-deploy-suite      Deployment tool (testnet keys only)
├── igit-suite-migrate     Deterministic migration plan generator
└── internal/              99 Go files

web/                       React/Vite + viem browser UI
archive/cosmwasm-v1/       Isolated V1 read-only historical viewer
scripts/                   Source, release, migration evidence, ops gates
```

### Technology Stack

| Layer | Technology | Current Version/Tool | Notes |
|---|---|---|---|
| **Smart Contracts** | Solidity | 0.8.24 (fixed) | Foundry v1.7.1, non-upgradeable |
| **CLI** | Go | 1.22 | go-ethereum, encrypted keystore |
| **Frontend** | JavaScript | React + Vite + viem | Legacy type-0 transactions |
| **Current Storage** | IPFS | Kubo | Local Kubo needed for push, HTTPS gateways for clone/fetch |
| **Planned Storage** | Object Storage | S3 + R2 | Requires successor protocol and new Suite version |
| **Blockchain** | Injective EVM | Testnet 1439, Mainnet 1776 | Min gas price 160000000 wei |

### Non-Negotiable Design Principles

- ✅ **Non-Upgradeable** - No proxy, no diamond pattern, no delegatecall
- ✅ **Single Trust Root** - Only `SuiteDirectory`, no mixed backends or fallback paths
- ✅ **Verification First** - Client verifies chain ID, suite version, active state, all module code hashes
- ✅ **Data Before Ref** - Pack must be durable before updating on-chain ref
- ✅ **Immutable Evidence** - Deployment, migration, receipts, fixed-block state must be immutable evidence, not fabricated
- ⚠️ **Storage Neutrality Requires New Protocol** - Current Suite only accepts `ipfs://` URIs, S3/R2 requires successor Suite

## Quantitative Metrics

| Metric | Current Value | Notes |
|---|---|---|
| **Source Code Scale** | 99 Go files, 9 Solidity contracts | Excludes node_modules and test files |
| **August Activity** | 85+ commits | Mostly CI/Web publishing fixes and operator tooling |
| **Test Coverage** | Comprehensive | CLI unit tests, race detection, Foundry unit/invariant, Web API tests |
| **Operator Tool Tests** | ✅ 15/15 passing | Coverage 60.5%, all core functionality tested (2026-08-22) |
| **Code Quality** | ✅ Passing | go fmt, go vet, compilation checks all passing (2026-08-22) |
| **P0 CI Status** | ✅ Green | Commit f6dcee9, all 5 jobs passing |
| **Current Branch** | `dev` | Main branch is `main` |
| **Public Deployment** | None | Awaiting P1 completion |
| **Security Review** | Pending | P1 critical blocker |

## Key Risks and Mitigations

| Risk | Current Control | Status |
|---|---|---|
| **Source readiness mistaken for availability** | Empty public profiles + evidence-gated release checks | ✅ In place |
| **Windows line-ending hash drift** | .gitattributes pinned LF + Windows artifact gates | ✅ Fixed (P0) |
| **Locale-dependent behavior tests** | Stable typed errors + independent translation tests | ✅ Fixed (P0) |
| **Windows key exposure** | DACL/credential provider, current-user and SYSTEM only | ✅ Implemented (P0) |
| **Immediate second immutable migration** | Testnet uses v3 IPFS; decide storage scope before mainnet | ⚠️ Decision pending |
| **Large pack memory exhaustion** | Temporary-file streaming, size limits, multipart tests | 📋 P3 scope |
| **Mutable or transformed object bytes** | Digest-derived keys, no transform, size/SHA-256 verification | 📋 P4 scope |
| **Provider API mismatch** | Separate AWS/R2 capability profiles and live canaries | 📋 P4 scope |
| **Thin-pack history corruption** | Preserve original bytes and URI order during migration | 📋 P5 scope |
| **Presigned URL leakage/reuse** | Short TTL, exact signed headers, one-time auth state, redaction | 📋 P4 design |
| **Premature garbage collection** | Finalized inventory, long grace, rollback window, retention-aware GC | 📋 P5 scope |
| **Mainland provider reachability** | Real network sampling and stable read-gateway/failover strategy | 📋 Ops scope |
| **ZKP replay or front-running** | Chain/contract/protocol/recipient/action/repo binding, spent-nullifier and epoch tests | 📋 Z0 scope |
| **Unreviewed trusted setup or circuit** | Hash-bound setup policy + independent circuit/verifier audit | 📋 Z0 gates |

## Timeline Visualization

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Now                         Future
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

P0 ████████ Complete (2026-08-19)
   └─ Windows/Linux baseline, CI green, Chinese support, DACL

P1 ▓▓▓▓░░░░ P1.1-P1.3 Complete ✅, P1.4-P1.6 In Progress
   ├─ Operator runner implementation ✅
   ├─ 9 contract deployments and Blockscout verification ✅
   ├─ Fresh-empty activation; no V1 import ✅
   ├─ Git, MetaMask, and finality E2E tests
   └─ Security review, evidence gate, and approval 🔒

P2 ░░░░ 3-5 days (Depends on P1)
   └─ Windows installer, release assets, clean VM tests

P3 ░░░░░░ 4-7 days (Can parallel P1)
   └─ packstore abstraction, streaming, verification

P4 ░░░░░░░░░░░░░░ 2-3 weeks (Depends on P3)
   └─ S3/R2 protocol, adapters, presigned flow

P5 ░░░░░░░ 1 week (Depends on P4)
   └─ Historical migration, CID mapping, rollback window

Z0 ░░░░ 3-5 days (Independent, can start after P0)
   └─ ZKP circuit, gnark, testnet deployment

M0 ⏸️ Not Scheduled (Depends on P1/P2 + conditional P5 + governance + approval)
   └─ Mainnet deployment

Critical Path: P0 ✅ → P1 ⚠️ → P2 → M0
```

## Security and Compliance Status

### Current Security Posture

**✅ Implemented:**
- Go race detector and vet checks passing
- Windows DACL key file permission isolation (current-user and LocalSystem only)
- Encrypted key storage (scrypt)
- Foundry invariant tests and gas ceilings
- Stable error codes (locale-independent)
- Bounded timeouts and failover (Kubo downloads)

**⚠️ Pending (P1 Blockers):**
- Independent deep source security review
- Security findings resolution
- Generate `security-review.pdf`

**⚠️ Pending (Mainnet Blockers):**
- Multisig/timelock governance design and deployment
- Operator key rotation and backup policy
- Key separation and offline backup
- ZKP circuit and verifier audit (if productized)

### Pre-Production Checklist

1. ✅ P0 source and CI baseline (Complete)
2. ⏳ Complete and document independent security review (P1 in progress)
3. ⏳ Design and deploy multisig/timelock governance (Before mainnet)
4. ⏳ Establish key rotation and disaster recovery procedures (Before mainnet)
5. ⏳ Resolve all 9 open decisions in `open-questions.md` (Before mainnet)
6. ⏳ Complete release and cutover evidence hash-bound approval (P1 in progress)

## Recommended Action Plan

### 🔴 Immediate Actions (Unblock P1)

**Priority 1: Security Review (Critical Path)**

1. **Launch Security Review Process** (Blocks Cutover)
   - Immediately contact independent security reviewers
   - Prepare review materials (architecture, threat model, critical paths)
   - Target: Start immediately, parallel with deployment execution

**Priority 2: Product E2E And Finality**

2. **Execute E2E Testing**
   - MetaMask writes and receipt recording
   - Linux/Windows clean environment Git E2E
   - V1 archive-preview read-only isolation
   - Uncertain-receipt and finality handling

3. **Collect Evidence and Pass Gates**
   - Run `migration-cutover-readiness.sh`
   - Obtain independent approval
   - Update testnet `SuiteDirectory` profiles
   - Target: 1-2 days after all tests pass

### 🟡 Short-Term Actions (After P1 Complete)

**P2: Windows Product Acceptance** (3-5 days)
- Build Windows installer
- Create release assets (igit.exe, git-remote-igit.exe)
- Test full Git workflow on clean VM
- Document WSL2 and injectived absence

**P3-P5: Storage Migration** (4-5 weeks total)
- P3: Introduce packstore abstraction (4-7 days)
- P4: Design and implement S3/R2 adapters (2-3 weeks, requires new ADR)
- P5: Execute historical migration (1 week)

**Documentation and Operations**
- Update user setup docs to reflect native Windows path
- Create troubleshooting guide
- Document operations runbook

### 🟢 Medium-to-Long-Term Actions (Mainnet Preparation)

**Governance and Policy Decisions**
- Determine multisig membership, quorum, emergency powers
- Approve platform fee and treasury policy
- Define username claim period and public communication
- Choose finality depth and reorg response thresholds
- Decide if ZKP is experiment or product requirement

**Operational Maturity**
- Establish evidence retention location and access policy
- Define independent reviewer authorization process
- Create key rotation and backup procedures
- Design S3/R2 bucket policies (versioning, retention, encryption)

## Related Documentation

| Document | Purpose |
|---|---|
| [项目状态总览 (中文)](project-status-zh.md) | Chinese version of this document |
| [Delivery Roadmap](delivery-roadmap.md) | Detailed milestones, dependencies, and exit criteria |
| [Backlog](backlog.md) | Granular engineering task inventory |
| [Architecture](architecture.md) | Immutable Suite and data-plane boundaries |
| [P0 Evidence](p0-evidence.md) | Commit-bound Windows/Linux CI and local verification |
| [Acceptance Evidence](acceptance-evidence.md) | Required real evidence and binding rules |
| [Open Questions](open-questions.md) | Decisions requiring explicit review |
| [ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md) | EVM V2 runtime and migration scope |
| [ADR 0002](adr/0002-pluggable-pack-storage.md) | Pluggable pack storage direction |
| [Release](release.md) | Release contents and cutover gates |
| [EVM V2 Migration](evm-v2-migration.md) | Migration workflow and completion definition |

## Frequently Asked Questions

### When will it be available?
**Testnet:** After P1 completion (est. 1-2 weeks), provided security review and all evidence gates pass.

**Mainnet:** Not yet scheduled. Requires P1/P2 completion, governance design, independent approval, and possibly P5 (if object storage is a launch requirement).

### Do I need WSL2 or injectived?
**Currently (after P0):** No. P0 has achieved native Windows and Linux support.

**Push operations:** Currently still require local Kubo daemon for IPFS push.

**Clone/fetch:** Use HTTPS IPFS gateways only, no local Kubo required.

**Future (after P4):** S3/R2 profiles will eliminate Kubo dependency entirely.

### Why can't the public cutover complete yet?
P1 is still blocked by:
1. Missing clean Windows/Linux Git, MetaMask, and finality E2E evidence
2. Independent security review and hash-bound approval are not complete
3. The final checksum-bound P1.4-P1.6 evidence gate has not passed

Deployment and fresh-empty activation are complete. CosmWasm V1 import is not
part of this cutover; V1 remains available only through the read-only archive
preview. The remaining acceptance work is still critical for a responsible
public cutover.

### Which networks are supported?
- **Injective Testnet:** EVM chain ID 1439
- **Injective Mainnet:** EVM chain ID 1776 (planned)

### Why are contracts non-upgradeable?
Security and trust minimization. Non-upgradeable means:
- No hidden governance backdoors
- Users can verify exact code at deployment time
- No proxy-related security risks
- Behavior is fixed and predictable

### When will S3/R2 be available?
**Current:** Direction accepted (ADR 0002), but not implemented. Current Suite only supports `ipfs://`.

**Timeline:** P3 (4-7 days) → P4 (2-3 weeks) → P5 (1 week), total ~4-5 weeks engineering time, starting after P1 completion.

**Requirements:** Requires new Suite version and successor protocol, since current Suite is non-upgradeable.

### What is the ZKP functionality?
**Z0:** Isolated testnet experiment for membership proofs (prove you belong to an authorized group without revealing who you are).

**Status:** Pure research, doesn't block any other milestones.

**Productization:** Requires separate design review, circuit audit, and governance decision.

### Can I help?
**Developers:**
- Review P0 code and CI configuration
- Test native Windows/Linux setup
- Assist with P1.4-P1.6 E2E, finality, and acceptance evidence

**Security Researchers:**
- Independent source code review
- Threat modeling
- Penetration testing (after P1 deployment)

**Users:**
- Prepare test environments
- Provide network reachability feedback
- Documentation review and translation

### How do I track progress?
1. **This Document** - Updated after each major progress
2. **[Delivery Roadmap](delivery-roadmap.md)** - Detailed milestone tracking
3. **[Backlog](backlog.md)** - Granular task checklist
4. **GitHub Commits** - Daily development activity
5. **CI Runs** - Automated test status

---

## Update History

| Date | Changes | Updated By |
|---|---|---|
| 2026-08-21 | Initial version - created based on P0 completion status | Project Assessment |
| 2026-08-22 | P1.1 operator tooling complete - updated metrics and blocking status | Project Assessment |
| 2026-08-31 | Confirmed P1.2/P1.3 complete; clarified P1.4-P1.6 and fresh-empty scope | Project Assessment |
| 2026-09-10 | Added code/evidence knowledge base and explicit EVM V2 continuation handoff; confirmed P1.4-P1.6 remain open | Project Assessment |
| 2026-09-11 | LiveAgent review reconciled client paths, required evidence gaps, and local gates; public SuiteDirectory remains empty | Project Assessment |

**Next Update:** After P1 completion or significant architectural changes

---

💡 **Tip:** This document provides a quick overview. For detailed technical decisions and implementation details, refer to the dedicated documents linked above.

`````

</details>

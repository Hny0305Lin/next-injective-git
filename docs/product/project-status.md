# Project Status Overview

- Status: P0 baseline complete, P1.1 operator runner complete (2026-08-22), P1.2 preparation phase
- Last Updated: 2026-08-22
- Current Assessment Baseline: `f6dcee9aa67255bfdff1867785435022df7ec5e9`
- Latest Progress: P1.1 operator runner implemented and tested (15/15 tests passing, 60.5% coverage)
- Public Availability: None; checked-in Suite profiles remain intentionally empty

> [!IMPORTANT]
> This document provides a high-level status overview of the project. For detailed
> technical decisions, delivery sequencing, and evidence requirements, refer to
> the dedicated documents. For Chinese version, see [项目状态总览 (中文)](project-status-zh.md).

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

**P1: Suite V3 Testnet Deployment and Cutover** (In Progress - P1.1 Complete)

**Blocking Factors:**
1. ❌ **No Public Deployment Evidence** - `SuiteDirectory` address remains empty in profiles
2. ✅ **Operator Tooling Complete** - Migration tool with encrypted keystore, signed journal, resume capability complete (2026-08-22)
3. ❌ **Missing Independent Security Review** - Currently skipped by operator decision, but cutover gate still requires `security-review.pdf`
4. ❌ **Missing Migration Evidence** - No deployment receipts, Blockscout verification, MetaMask receipts, or complete migration journal

**Required Work (By Priority):**

#### P1.1 Operator Runner (✅ Complete - 2026-08-22)
- [x] Implement operator runner with encrypted keystore transactor
- [x] Implement append-only signed journal mechanism
- [x] Implement safe resume for uncertain receipts
- [x] Implement fixed-block imported-state evidence emission
- [x] Complete unit tests (15/15 passing, 60.5% coverage)
- [x] Pass code quality checks (go fmt, go vet, compilation)

#### P1.1b Next Immediate Steps (Validation Phase)
- [ ] Execute dry-run end-to-end test with test manifest
- [ ] Verify journal writing and signature verification
- [ ] Test uncertain receipt recovery logic
- [ ] Confirm no transactions broadcast in dry-run mode

#### P1.2 Deployment Execution (Priority 2 - Blocks Cutover)
- [ ] Rotate and fund testnet operator key
- [ ] Fix V1 cutover height
- [ ] Generate complete inventory, snapshot, SHA-256 sidecar, username escrow release evidence
- [ ] Deploy all 9 contracts with no-clobber evidence
- [ ] Verify all contracts on Blockscout
- [ ] Collect all 9 deployment receipts

#### P1.3 Import and Activation (Blocks Verification)
- [ ] Import every bounded batch in order
- [ ] Verify counts and rolling commitments
- [ ] Atomically activate Directory
- [ ] Compare every migrated domain at one finalized block
- [ ] Record final Directory state, code hashes, module bindings

#### P1.4 E2E and Wallet Testing (Blocks Cutover)
- [ ] Execute MetaMask writes and record receipts
- [ ] Execute Git E2E on clean Linux environment
- [ ] Execute Git E2E on clean Windows environment
- [ ] Test historical alias resolution
- [ ] Test uncertain receipt handling

#### P1.5 Security and Approval (Priority 1, Can Parallel - Blocks Cutover)
- [ ] Launch and complete deep source security review
- [ ] Resolve all security findings
- [ ] Obtain independent hash-bound cutover approval
- [ ] Generate `security-review.pdf`
- [ ] Generate `cutover-approval.txt`

#### P1.6 Evidence Collection and Gates (Final Step)
- [ ] Collect complete evidence directory
- [ ] Run `scripts/migration-cutover-readiness.sh`
- [ ] Verify all evidence passes gates
- [ ] **Only then** update testnet `SuiteDirectory` profiles

**Estimated Engineering Time:** P1.1 complete ✅ (2026-08-22), remaining P1.2-P1.6 approximately 1-2 weeks (excludes external security review scheduling)

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

P1 ░░░░░░░░ 1-2 weeks (P1.1 Complete ✅, P1.2-P1.6 In Progress)
   ├─ Operator runner implementation ✅
   ├─ 9 contract deployments (Next)
   ├─ Blockscout verification (Next)
   ├─ Batch import and activation
   ├─ Git E2E tests
   └─ Security review and approval 🔒

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

**Priority 2: Deployment Execution** (P1.1 Complete ✅)

2. **Test Operator Runner in Protected Environment**
   - Execute dry-run end-to-end test with test manifest
   - Verify journal writing and signature verification
   - Test uncertain receipt resume logic
   - Target: 1-2 days

3. **Execute Testnet Deployment**
   - Rotate testnet operator key and fund
   - Deploy all 9 contracts
   - Verify all contracts on Blockscout
   - Collect all deployment receipts
   - Target: 3-5 days after dry-run complete

4. **Execute Batch Import and Activation**
   - Import all batches in order
   - Atomically activate Directory
   - Compare migrated state at fixed block
   - Target: Same week as deployment

5. **Execute E2E Testing**
   - MetaMask writes and receipt recording
   - Linux/Windows clean environment Git E2E
   - Historical alias resolution tests
   - Target: 2-3 days after activation

6. **Collect Evidence and Pass Gates**
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

### Why can't it be deployed yet?
P1 is blocked by:
1. Incomplete operator runner tooling
2. Independent security review not yet started
3. Missing complete deployment and migration evidence

These are non-code work items but critical for responsible public deployment.

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
- Assist with P1.2+ deployment and testing

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

**Next Update:** After P1 completion or significant architectural changes

---

💡 **Tip:** This document provides a quick overview. For detailed technical decisions and implementation details, refer to the dedicated documents linked above.

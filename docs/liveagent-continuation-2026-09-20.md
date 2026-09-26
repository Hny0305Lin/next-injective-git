# LiveAgent 接续入口：iGit EVM V2（2026-09-20）

> 本页是 LiveAgent 新客户端于 2026-09-20 接手本项目时的当前上下文入口，整合
> [事实基线](reconciliation-baseline-2026-09-12.md)、[ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)、
> [BYOS 规格第 9 节](storage-byos.md)和 [backlog](backlog.md) 的既定事实，并记录本轮本机复核结果。
> 它不修改、不取代任何 HISTORICAL 原文；状态仅使用 PASS / FAIL / BLOCKED / NOT PROVEN / HISTORICAL。

| 项目 | 当前值 |
|---|---|
| 本轮复核 | 2026-09-20 22:12–22:16（Asia/Shanghai），LiveAgent 新客户端 |
| 分支 / HEAD | `dev` / `0ba06f436558f12d97625b393767440cdd0f9862`（与 2026-09-13 基线一致，未变） |
| Remote | `https://github.com/Hny0305Lin/next-injective-git.git` |
| 工作树 | 不干净：2026-09-13 审计文档改动 + S01–S03 新源码 + 历史草稿均未提交（详见第 5 节） |
| 环境 | Windows amd64；Node v24.11.1、npm 11.6.2、Go 1.26.5、Git 2.45.1、锁定 solc 0.8.24 |
| 产品代号 / 链上协议 | iGit EVM V2（产品代际）/ Suite v3（链上版本，不可升级） |
| 测试网 | Injective Testnet EVM chain ID 1439；九合约已部署且实时可读（基线第 3 节） |

## 1. 先读什么（权威顺序）

1. [事实基线](reconciliation-baseline-2026-09-12.md)：唯一状态依据。九合约 PASS、索引器 FAIL、
   Moderation UI 缺失、Foundry/Kubo/DACL BLOCKED、真实 E2E NOT PROVEN；文件名保留 2026-09-12，实际审计 2026-09-13。
2. [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)（2026-09-13 用户确认）：
   主网直接采用 storage-neutral successor，首期支持用户自有 **AWS S3 / Cloudflare R2**（BYOS），
   不先发布 IPFS-only v3 主网；不接 MinIO/其他云/自建对象存储。
3. [BYOS 实施规格](storage-byos.md)：canonical JSON（RFC 8785 JCS）、PackManifest schema 1、
   provider 能力、凭据边界；第 9 节记录 S01–S03 本地实现与验证。
4. [backlog](backlog.md)：R01–R08（审计遗留问题）与 S01–S07（新存储工作流）及退出条件。
5. [下一窗口实施提示词](prompts/next-storage-implementation.md) 与 [BYOS 陪同实测提示词](prompts/storage-byos-hands-on-validation.md)。
6. 架构与术语参考 [architecture.md](architecture.md) 和
   [project-knowledge-base-zh.md](project-knowledge-base-zh.md)（后者为 HISTORICAL 快照，架构描述需按当前代码逐条引用）。

HISTORICAL（保留原样，不作状态依据）：根目录 MIGRATION-\*/PRIORITY-\* 草稿、
[docs/a11-storage-indexer-v2.md](a11-storage-indexer-v2.md)、[docs/PRIORITY-MAPPING.md](PRIORITY-MAPPING.md)、
[docs/evm-v2-handoff.md](evm-v2-handoff.md)、[docs/liveagent-evm-v2-context.md](liveagent-evm-v2-context.md)。
旧 P1.1/P1.2/P1.3 编号有两套含义，不能互换；后续任务只用 R/S 编号。

## 2. 项目一页模型

- **igit**：Git 控制面在 Injective EVM（九个不可升级 Solidity 合约，`SuiteDirectory` 是唯一链上信任根），
  Git packfile 走数据面。当前 v3 Suite 只接受 `ipfs://` pack URI。
- 现有 v3 测试网 Suite（九合约，chain ID 1439）已部署、激活 `fresh-empty-suite`，实时只读核对全部一致；
  但公开 CLI/Web profile 的 Directory 为空，**未完成公开切换**，真实产品 E2E NOT PROVEN。
- V1 CosmWasm 只保留固定高度 GET-only archive preview，不是 fallback。
- **主网方向已变更**（ADR 0004）：不再以 IPFS-only v3 为主网目标，而是实现 storage-neutral
  successor Suite + 用户自有 AWS S3 / R2 桶（BYOS），manifest 用 canonical JSON（JCS），
  digest-derived keys，公开仓库、用户付费、独立 reader 配置。
- 已部署 v3 不可升级：successor 需要新 Suite 版本、新 ABI/事件，与 Go/Web/indexer/evidence 同版本一起审查（S04）。

## 3. 当前任务状态（接手时）

**审计遗留 R01–R08**（详见基线/backlog）：

| 任务 | 状态 | 要点 |
|---|---|---|
| R01 索引 ABI/链身份修复 | FAIL | 四个旧 indexer 脚本 Topic0/module ID/selector 全错，未接当前 ABI 库 |
| R02 索引恢复与 unpin 安全 | FAIL | 无去重/reorg/原子状态；reaper 直接 `pin rm`，禁止运行 |
| R03 Windows CLI 配置读取 | BLOCKED | config.Load DACL `Access is denied`；不得关闭防护绕过 |
| R04 Foundry 验收 | BLOCKED | 本机无 `forge`（2026-09-20 复核仍未安装） |
| R05 Kubo 与存储实证 | BLOCKED | 本机无 `ipfs`（2026-09-20 复核仍未安装） |
| R06 Moderation UI | FAIL | 文档声称已实现，但 moderationModel.ts 等三个文件不存在 |
| R07 测试网产品 E2E | NOT PROVEN | 需 R01–R06 解除 + 明确写交易授权 |
| R08 切换证据与范围 | NOT PROVEN | reviewed commit 统一、安全审查、approval、checksum gate |

**新存储工作流 S01–S07**（2026-09-13 用户决策）：

| 任务 | 状态 | 要点 |
|---|---|---|
| S01 Canonical manifest/commitment | PASS（本地） | schema 1 冻结；Go/TS JCS 交叉向量 11 正向 + 55 反向 |
| S02 Verified packstore boundary | PASS（本地） | 流式写读、raw SHA-256、验证后摄取；IPFS adapter 封装 |
| S03 AWS/R2 BYOS adapters/config | PASS（本地） | 独立 capability、safehttp、storageconfig、恢复 checkpoint；无真实云 |
| S04 Successor Suite/versioned ABI | **NOT PROVEN（下一实现切片）** | 状态/查询/事件绑定 manifest，revision CAS/versioned dispatch |
| S05 CLI/Web 本地纵向接入 | NOT PROVEN | fake cloud/chain + 真实本地 Git 的 push/clone/fetch/pull |
| S06 真实云/successor 测试网 E2E | NOT PROVEN | 需明确桶/凭据/费用/写入授权 |
| S07 历史 v3 导入（条件性） | NOT PROVEN | 仅用户另选 import scope 时启动 |

## 4. 2026-09-20 本机复核结果

在当前工作区（含未提交 S01–S03 源码）实际执行，均一次性通过：

| 命令 | 结果 | 说明 |
|---|---|---|
| `git rev-parse HEAD` | `0ba06f4...` | 与基线一致，工作树未提交内容仍在 |
| `cd cli && go vet ./...` | PASS | 退出 0，无输出 |
| `cd cli && go test -count=1 ./...` | PASS | 全部有测试包 ok，含 S01–S03 新包（packmanifest、packstore、ipfsstore、s3store、safehttp、storageconfig、gitio、cmd/igit storage） |
| `cd web && npm run test:api` | PASS | 140 tests / 140 pass / 0 fail / 0 skip |
| `cd web && npm run typecheck` | PASS | tsc -b 通过 |
| `cd web && npm run test:storage-cross` | PASS | Go→TS / TS→Go canonical bytes/digest：11 vectors |
| `cd contracts/evm-v2 && npm run check` | PASS | `EVM SUITE SOLC CHECK: PASS`（锁定 solc 0.8.24） |
| `cd cli && go run ./cmd/igit storage doctor ../docs/examples/storage-byos.json` | PASS | 4 profiles / 2 repository bindings，本地无密钥诊断 |
| `which forge` / `which ipfs` | 均未安装 | R04、R05 维持 BLOCKED |

以上为本地检查，不构成云端、链上、Foundry、Kubo、真实 E2E 或部署验收。
与 2026-09-13 记录相比无回归；本轮未修改源码、未发送交易、未动云资源、未 commit/push。

## 5. 工作树未提交内容清单（接手时）

- **修改**：`cli/cmd/igit/main.go`、`cli/go.mod`/`go.sum`、`web/package.json`/`package-lock.json`
  （storage 子命令挂接与 SDK 依赖），以及 13 份 docs（audit 对齐版）。
- **新增源码（S01–S03，全部未跟踪）**：`cli/internal/packmanifest/`、`cli/internal/packstore/`（含
  `ipfsstore/`、`s3store/`）、`cli/internal/safehttp/`、`cli/internal/strictjson/`、
  `cli/internal/storageconfig/`、`cli/internal/gitio/verified.go`、`cli/internal/ipfs/stream.go`、
  `cli/cmd/igit/storage.go` + 测试、`protocol/packmanifest/`（共享向量）、
  `web/src/lib/packmanifest.ts`、`web/scripts/storage-*.mjs`、`web/test/packmanifest.test.mjs`、
  四个 `scripts/evm-*.sh`（R01/R02 所指旧索引脚本，未跟踪、有缺陷、勿运行主流程）。
- **新增文档**：`docs/reconciliation-baseline-2026-09-12.md`、`docs/storage-byos.md`、
  `docs/adr/0004-*.md`、`docs/backlog` 重写区、`docs/prompts/`、`docs/examples/`、本文件。
- **历史草稿（未跟踪，勿当状态依据）**：根目录 MIGRATION-\*/PRIORITY-\* 报告。
- **缓存目录（基线已登记，勿当业务源码、勿清理）**：`cli/null/`、`web/null/`、
  `contracts/evm-v2/null/`、`contracts/evm-v2/%SystemDrive%/`、`web/%SystemDrive%/`。

注意：S01–S03 源码只存在于本工作区，未跟踪、未提交。`git clone` 同一 HEAD 得不到这些实现；
换机器接续时必须先转移完整源码或先提交。

## 6. 接续下一步（按既定计划）

1. **S04：successor Suite / versioned ABI**——reviewed successor 状态/查询/事件绑定 manifest digest/size、
   revision CAS、force/delete/fork/bootstrap、versioned dispatch 与未知组合上传前拒绝；
   solc/Foundry/尺寸/parity 分层报告；保留 v3 ABI 与历史证据。同 commit 变更、fork 绑定、
   delete/recreate ABA、CAS/revert/reorg/不确定 receipt 的恢复语义需一并设计。
2. **S05：CLI/Web 本地纵向接入**——fake cloud/chain + 真实本地 Git 的 push/clone/fetch/pull/new-ref/force/delete；
   Web 公开 manifest/pack 先验摘要、CORS/大小上限提示，Web 不接触云 secret。
3. **BYOS 陪同实测**（见 [prompts/storage-byos-hands-on-validation.md](prompts/storage-byos-hands-on-validation.md)）：
   用户准备真实 AWS/R2 桶后分层 canary；真实云操作逐阶段授权。
4. 审计遗留 R01/R02（indexer 修复）、R06（Moderation UI）可与 S 线并行推进；
   R04/R05 需先安装 Forge/Kubo。
5. S01–S03 与本轮文档如需固化，应在用户明确授权后整理 commit；不要在未授权时自行提交或推送。

## 7. 硬边界（不要做）

- 不运行四个旧 `scripts/evm-*.sh` 主流程；reaper 直接 `pin rm`，自动 unpin 一律禁止。
- 不关闭/绕过 Windows DACL 配置防护（R03）；不读取用户真实密钥、不做云写入/删除/改 bucket 策略。
- 不发送链上交易、不部署、不改公开 profile、不把 SuiteDirectory 写入默认配置。
- 不覆盖 `evidence/` 或基线记录；不用 mock/fixture/本地检查冒充云、主网或 E2E 验收。
- 不清理缓存目录与 HISTORICAL 原文；不把旧 P1 编号当当前事实。
- 正常内容变更生成新 pack/manifest/ref，不覆盖既有 digest key。
- 不在 receipt 不确定时换 nonce 重发；不加第二 module address / proxy / V1 write fallback。

## 8. 交接记录

| 日期 | 记录 |
|---|---|
| 2026-09-13 | 事实基线审计；ADR 0004 产品决策；S01–S03 本地实现与验证（见基线与 BYOS 第 9 节） |
| 2026-09-14 | BYOS 陪同实测提示词成稿 |
| 2026-09-20 | LiveAgent 新客户端接手：复核 HEAD/工作树未变、无回归，全量本地检查通过（第 4 节）；确认下一步为 S04→S05 及 BYOS 实测；建立本接续入口 |

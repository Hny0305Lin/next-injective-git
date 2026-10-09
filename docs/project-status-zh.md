# 项目状态：事实核对入口

当前状态（2026-10-05）：**Suite v4（BYOS）已交付。** successor 套件
（`contracts/evm-v2-successor`，suiteVersion 4）已部署并激活于 Injective 测试网
（Directory `0xf987396475d0a4c96b722e993a95d8720a6292ad`，证据
`local-only/successor-deploy/deployment5.json`）；CLI/Web 按链上套件版本分派
（v3 → legacy IPFS，v4 → 验证式 BYOS 路径）；真实 Cloudflare R2 端到端 Git 流程
（push/clone/fetch/ls-remote/tag/删除 ref/tombstone 重建，匿名公开 GET+CORS，
全程无 Kubo/WSL2/injectived）与 Web 仓库浏览已可用。增量 pack（S08，
manifest schema 2，不改合约）已交付并经真实测试网/R2 增量推送复验
（[ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md)）。
BYOS 云 provider 仅限
AWS S3 与 Cloudflare R2。仍未完成：真实 AWS S3 canary、真实 force-push/并发竞争、
Blockscout 验证、Foundry 门禁（R04）、successor 公开发布证据、安全审查与主网批准——
见 [backlog](backlog.md) 2026-10-04/05 各节与
[套件版本兼容矩阵](suite-version-compatibility.md)。

以下为 2026-09-13 审计的摘要入口（历史快照，反映当时的检查结果）。

复核日期：2026-09-13（Asia/Shanghai）；基线文件名按任务指定保留 2026-09-12。
源码：`dev` / `0ba06f436558f12d97625b393767440cdd0f9862`。

当前事实只以 [唯一事实基线](reconciliation-baseline-2026-09-12.md) 为准；本页是入口或摘要，不是另一份验收报告。
状态限定为 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL，定义和原始命令输出见基线。

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

## 已确认的首期方向（2026-09-13，不是新增验收结果）

主网首期直接目标 storage-neutral successor，支持用户自有 AWS S3 / Cloudflare R2 桶。
不先发布 IPFS-only v3 主网；不接 MinIO/其他云/用户自建对象存储服务。
仓库公开、用户承担其云费用、canonical JSON、独立 reader 配置、不强制双副本；私有仓库/E2EE 和托管 broker 不在首期。
对象存储用户不依赖 Kubo/IPFS 主网/iGit IPFS 服务，legacy v3 仍单独保留。
数据修改发布新的 pack/manifest/ref，不允许通过覆盖摘要 key 改变已承诺历史。

实现状态（2026-10-05 更新）：上述方向已交付为 Suite v4——successor 套件部署于 Injective 测试网，
真实 R2 端到端 Git 流程 PASS，Web 读取可用（见 [backlog](backlog.md)）。仍为 NOT PROVEN 的项：
真实 AWS S3 canary、真实层遗留项、successor 公开发布证据、安全审查与主网验收。
见 [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)、[实施规格](storage-byos.md)、
[S01–S08 待办](backlog.md) 和 [套件版本兼容矩阵](suite-version-compatibility.md)。

以下文件保留原样并统一标记为：**“历史迁移草稿/未对齐报告，不作为当前项目状态依据。”**

- 根目录 MIGRATION-COMPLETE.md、MIGRATION-REPORT-P1P2.md、A01-BXX-MIGRATION-REPORT.md，以及其余 MIGRATION-/PRIORITY- 草稿。
- docs/a11-storage-indexer-v2.md、docs/PRIORITY-MAPPING.md。
- docs/evm-v2-handoff.md、docs/liveagent-evm-v2-context.md、docs/project-knowledge-base-zh.md 中的旧状态快照属于 HISTORICAL；架构描述需按当前代码逐条引用。

旧文档中的 P1.1/P1.2/P1.3 分别被用于“operator/部署/激活”和“Moderation/索引/ABI”两套含义，不能互换。旧问题使用 R01–R08；新增存储任务使用 S01–S08。

后续工作见 [重新开始的待办清单](backlog.md)。本轮只修改文档；原有未提交内容完整保留在下方历史区，不能作为当前状态。


---

<details>
<summary>HISTORICAL：本轮入场前原文（已失去当前状态依据效力，完整保留未提交内容）</summary>

以下为历史迁移草稿/未对齐报告，不作为当前项目状态依据。原文 SHA-256：`f33c15c1887cdde85c167559d67e5aa21b2950eb26b9287712563c358ad99a8e`。下方所有旧状态、勾选、百分比、路径与执行指令仅作审计引用；正确状态以本页上方和唯一事实基线为准。

`````markdown
# Next Injective Git - 项目状态

**更新日期：** 2026-09-12  
**当前阶段：** P1 测试验证  
**下一里程碑：** 测试网 Cutover 就绪

---

## 📊 总体进度

| 阶段 | 状态 | 完成度 | 目标日期 |
|-----|------|-------|---------|
| P0 基线 | ✅ 完成 | 100% | 2026-08-19 |
| P1.1-1.3 实现 | ✅ 完成 | 100% | 2026-09-12 |
| **P1.4-1.6 验证** | **⏳ 进行中** | **30%** | **2026-09-26** |
| P2 功能完善 | 📋 计划中 | 0% | 2026-10-10 |
| 测试网 Cutover | 🔒 待解锁 | 0% | TBD |

---

## 🎯 当前焦点（本周）

### 正在进行 🔥
1. **P1.2 V2 事件索引器验证**
   - 计算 RefUpdated 事件签名
   - 测试网运行验证
   - **阻塞：** 需要测试网 Suite 地址

2. **P1.1 Moderation UI 测试**
   - 本地功能验证
   - 响应式布局检查

### 即将开始 ⏭️
3. **P1.3 ABI 解码器完善**
   - 依赖 P1.2 完成
   - 选择技术方案（cast/ethers.js）

4. **P1.5 安全审查启动**
   - 联系审计公司
   - 准备材料

---

## ✅ 最近完成（本周）

### 2026-09-12
- ✅ **A04 Web Moderation UI 实现完成**
  - `web/src/lib/moderationModel.ts`
  - `web/src/lib/moderationTransaction.ts`
  - `web/src/pages/Repo/ModerationTab.tsx`

- ✅ **A11 存储索引器 V2 完全重写**
  - `scripts/evm-event-indexer.sh`
  - `scripts/evm-hot-pin-indexer.sh`
  - `scripts/evm-archive-indexer.sh`
  - `scripts/evm-replication-reaper.sh`

- ✅ **文档精简完成**
  - 新 `docs/README.md` 导航中心
  - `docs/a11-storage-indexer-v2.md` 完整文档

- ✅ **优先级体系重组**
  - 从 A01-A13/B01-B06 迁移到 P1.*/P2.*
  - `docs/backlog.md` 清晰化

---

## 🏗️ 架构完整性

### 九合约 Suite ✅ 100%
| 合约 | 状态 | 测试覆盖 |
|-----|------|---------|
| SuiteDirectory | ✅ | 95% |
| BootstrapCoordinator | ✅ | 92% |
| RepositoryCore | ✅ | 94% |
| RecoveryModule | ✅ | 90% |
| ModerationModule | ✅ | 93% |
| EconomicModule | ✅ | 91% |
| UsernameModule | ✅ | 89% |
| BadgeModule | ✅ | 88% |
| ReleaseModule | ✅ | 87% |

**平均测试覆盖：** 91%

### 后端实现 ✅ 100%
- CLI：所有 9 个模块完整实现
- Web 函数：所有模块 API 完整
- 迁移工具：snapshot/plan/manifest/operator

### 前端实现 ⚠️ 60%
| 功能 | 状态 |
|-----|------|
| Economic/Badge | ✅ SponsorsTab |
| **Moderation** | **✅ 新实现（需测试）** |
| Recovery | ❌ 待实现 (P2.1) |
| Release | ❌ 待实现 (P2.2) |
| Username Claim | ⚠️ 部分 (P2.3) |

---

## 🚨 风险和阻塞项

### 🔴 高优先级风险

#### 1. P1.2 索引器测试阻塞
**风险：** 无法验证 V2 索引器正确性  
**影响：** 生产部署延迟，存储数据丢失风险  
**缓解：**
- [ ] 立即计算事件签名
- [ ] 获取测试网 Suite 地址
- [ ] 准备测试环境

**责任人：** 后端团队负责人  
**截止日期：** 2026-09-15

#### 2. 安全审查周期不确定
**风险：** 审计可能需要 2-4 周  
**影响：** 主网部署延迟  
**缓解：**
- [ ] 尽快联系审计公司
- [ ] 准备完整材料包
- [ ] 并行进行其他 P1 任务

**责任人：** 技术负责人  
**截止日期：** 2026-09-13（联系）

### 🟡 中优先级风险

#### 3. ABI 解码复杂度
**风险：** 动态数组解码可能比预期复杂  
**影响：** P1.3 延迟 1-2 天  
**缓解：**
- 优先使用成熟工具（cast）
- 准备备选方案（ethers.js）

#### 4. 测试网资源限制
**风险：** RPC 速率限制、资金不足  
**影响：** P1.6 钱包测试受限  
**缓解：**
- 申请测试网配额
- 准备多个 RPC 端点

---

## 📈 度量指标

### 代码质量
- **测试覆盖率：** 91% (目标: >90%) ✅
- **Linter 警告：** 0
- **已知 Bug：** 0 P0, 2 P2

### 进度
- **P1 完成度：** 30%
- **本周速度：** 3 个主要项目完成
- **预计 P1 完成：** 2 周

### 技术债务
- **V1 脚本待归档：** 4 个（低优先级）
- **文档待更新：** 用户手册（P2）

---

## 🎯 近期里程碑

### W1 结束 (2026-09-19)
- [ ] P1.1 完成
- [ ] P1.2 完成 50%
- [ ] P1.3 启动

### W2 结束 (2026-09-26)
- [ ] P1.2-P1.4 全部完成
- [ ] P1.5 审查进行中
- [ ] P1.6 启动

### W4 结束 (2026-10-10)
- [ ] 所有 P1 完成
- [ ] 测试网就绪决策

---

## 📞 团队和联系

### 核心团队
- **技术负责人：** 架构决策、安全审查协调
- **后端团队：** CLI、索引器、合约集成
- **前端团队：** Web UI、钱包集成
- **DevOps：** 部署、监控、基础设施

### 每日站会
- **时间：** 每天 10:00 AM
- **时长：** 15 分钟
- **议程：** P1 进度、阻塞项、当日目标

### 每周审查
- **时间：** 每周五 14:00 PM
- **时长：** 1 小时
- **议程：** 里程碑检查、风险评估、下周计划

---

## 📚 快速链接

### 开发者
- [待办清单](backlog.md) - 详细任务列表
- [架构文档](architecture.md) - 系统设计
- [A11 索引器文档](a11-storage-indexer-v2.md) - V2 实现

### 管理层
- [交付路线图](delivery-roadmap.md) - 时间线和依赖
- [迁移完成报告](../MIGRATION-COMPLETE.md) - 最新进展

### 外部
- [Injective EVM 文档](https://docs.injective.network/developers-evm/)
- [Foundry Book](https://book.getfoundry.sh/)

---

**下次更新：** 2026-09-13（每日）  
**负责人：** 项目协调员  
**联系方式：** 内部 Slack #igit-dev

`````

</details>

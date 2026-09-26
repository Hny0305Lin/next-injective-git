# 项目文档入口

复核日期：2026-09-13（Asia/Shanghai）；基线文件名按任务指定保留 2026-09-12。
源码：`dev` / `0ba06f436558f12d97625b393767440cdd0f9862`。

原审计记录以 [事实基线](reconciliation-baseline-2026-09-12.md) 为准；其后 S01–S03 的新增源码与本地验证见 [BYOS 规格第 9 节](storage-byos.md)。本页仅作入口，不覆盖历史审计。
状态限定为 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL，定义和原始命令输出见基线。

| 入口 | 用途 | 状态边界 |
|---|---|---|
| [LiveAgent 接续入口 2026-09-20](liveagent-continuation-2026-09-20.md) | 新客户端接手时的当前上下文：权威顺序、R/S 状态、2026-09-20 本机复核、未提交清单与下一步 | 复核记录，不构成云/链上/E2E 验收 |
| [事实基线](reconciliation-baseline-2026-09-12.md) | 当前唯一状态依据；含命令、文件清单、RPC、冲突表和缺口 | 只按具体检查项赋予状态 |
| [中文状态入口](project-status-zh.md) / [English status](project-status.md) | 基线摘要 | 不维护第二套完成度 |
| [后续待办](backlog.md) | R01–R08 旧问题、S01–S07 新存储任务及退出条件 | 计划不代表实现或授权 |
| [架构](architecture.md)、[ADR](adr/) | 设计约束 | 不是实时验收证据 |
| [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md) | 2026-09-13 用户确认的主网 successor / AWS S3 / R2 BYOS 范围 | 产品选择已确定；S01–S03 本地 PASS，真实云/successor NOT PROVEN |
| [BYOS 实施规格](storage-byos.md) | canonical JSON、manifest、credentials、provider 能力与失败恢复 | 已测试 schema 1、本地 API/限额、命令与未验证边界 |
| [下一窗口实施提示词](prompts/next-storage-implementation.md) | 本轮已执行的 S01–S03 范围与后续约束 | 不含真实凭据/云写入/交易/部署授权 |
| [BYOS 陪同实测提示词](prompts/storage-byos-hands-on-validation.md) | 交给其他模型：本地复验、最小 canary、用户资源准备及真实 AWS/R2 分层测试 | 2026-09-14；真实云操作逐阶段授权，不冒充 successor E2E |
| [交付路线图](delivery-roadmap.md)、[开放问题](open-questions.md) | 首期依赖与真正未决事项 | 不再把 BYOS/successor/canonical JSON 列作待选方向 |
| [验收规则](acceptance-evidence.md)、[发布规则](release.md) | 将来验收和发布条件 | 本轮未通过最终 gate |
| [历史部署证据](../evidence/testnet-deployment-2026-08-25/) | 旧部署/激活资料 | HISTORICAL；本轮 RPC 记录位于基线 |

以下文件保留原样并统一标记为：**“历史迁移草稿/未对齐报告，不作为当前项目状态依据。”**

- 根目录 MIGRATION-COMPLETE.md、MIGRATION-REPORT-P1P2.md、A01-BXX-MIGRATION-REPORT.md，以及其余 MIGRATION-/PRIORITY- 草稿。
- docs/a11-storage-indexer-v2.md、docs/PRIORITY-MAPPING.md。
- docs/evm-v2-handoff.md、docs/liveagent-evm-v2-context.md、docs/project-knowledge-base-zh.md 中的旧状态快照属于 HISTORICAL；架构描述需按当前代码逐条引用。

旧文档中的 P1.1/P1.2/P1.3 分别被用于“operator/部署/激活”和“Moderation/索引/ABI”两套含义，不能互换。旧问题使用 R01–R08，新增存储任务使用 S01–S07。

原审计关键结论（不覆盖 BYOS 第 9 节新增记录）：九合约 solc、Go test/vet、Web 本地检查 PASS；Foundry/Kubo/普通 CLI 配置读取 BLOCKED；Moderation UI 完成声明及 EVM 索引器 FAIL；真实产品 E2E 和生产部署 NOT PROVEN。

### 2026-09-13 首期产品决策

主网直接采用存储中立 successor，支持用户自有 **AWS S3 / Cloudflare R2 云桶**，不先发布 IPFS-only v3 主网。
首期不接 MinIO/其他云/自建对象存储服务；仓库公开，用户承担费用，采用 canonical JSON，reader 由用户独立配置，双副本后续考虑。
AWS/R2 路径不应依赖 Kubo 或 iGit IPFS 服务；现有 v3 仍是独立 legacy IPFS 路径。
这些是已确认的实现目标，不是已部署功能。正常修改生成新 pack/manifest/ref，而不是覆盖既有摘要 key。

`D:/inj/next-injective-git/cli/README.md` 和 `D:/inj/next-injective-git/web/README.md` 文件不存在，旧入口不再作为有效文档链接。目录源码请直接查看 [CLI](../cli/)、[Web](../web/) 和 [九合约说明](../contracts/evm-v2/README.md)。

本轮测试辅助进程因环境变量缺失产生的五个缓存目录已在基线登记；自动审批拒绝清理，未删除或移动。不要把它们识别为业务源码。


---

<details>
<summary>HISTORICAL：本轮入场前原文（已失去当前状态依据效力，完整保留未提交内容）</summary>

以下为历史迁移草稿/未对齐报告，不作为当前项目状态依据。原文 SHA-256：`113e33da15b0a74a0f7a7b293189bdb8039b924765100860f808be9847a797bb`。下方所有旧状态、勾选、百分比、路径与执行指令仅作审计引用；正确状态以本页上方和唯一事实基线为准。

`````markdown
# Next Injective Git (iGit) 项目文档

本目录包含 iGit 项目的核心技术文档、架构决策和交付证据。

## 📖 核心文档

### 项目概述和状态
- **[项目状态总览](project-status-zh.md)** - 当前进度、里程碑、风险和行动计划
- **[交付路线图](delivery-roadmap.md)** - 详细时间线、依赖关系和退出标准
- **[待办清单](backlog.md)** - 工程任务跟踪

### 架构和设计
- **[系统架构](architecture.md)** - 不可变 EVM Suite 九合约架构说明
- **[ADR 目录](adr/)** - 所有架构决策记录
  - [ADR 0001: EVM V2 运行时和迁移范围](adr/0001-evm-v2-runtime-and-migration-scope.md)
  - [ADR 0002: 可插拔 Pack 存储](adr/0002-pluggable-pack-storage.md)
  - [ADR 0003: 空状态 Suite 和 V1 归档预览](adr/0003-fresh-evm-suite-and-v1-archive-preview.md)

### 证据和验证
- **[P0 证据记录](p0-evidence.md)** - P0 基线的提交绑定和验证证据
- **[验收证据要求](acceptance-evidence.md)** - 必需的真实证据和绑定规则
- **[发布和切换](release.md)** - 发布内容和切换门控

### 技术实现
- **[EVM V2 迁移指南](evm-v2-migration.md)** - 迁移工作流和完成定义
- **[A11 存储索引器 V2](a11-storage-indexer-v2.md)** - V2 EVM 事件索引器实现文档
- **[基础设施](infrastructure.md)** - 当前 IPFS 数据平面
- **[监控](monitor.md)** - 可观测性和告警

### AI 接续文档（供新会话快速上手）
- **[项目知识库](project-knowledge-base-zh.md)** - 完整的事实库、架构地图、数据流和安全不变量

## 🗂️ 文档分类

### 按状态分类
| 类别 | 状态 | 最后更新 |
|-----|------|---------|
| 项目状态和规划 | ✅ 最新 | 2026-09-12 |
| 架构和 ADR | ✅ 稳定 | 2026-08-31 |
| P0 证据 | ✅ 完整 | 2026-08-19 |
| A11 存储索引器 | ✅ 实现完成 | 2026-09-12 |
| P1.4-P1.6 验收 | ⏳ 进行中 | 2026-09-12 |
| 安全审查 | ⏳ 待启动 | - |

### 按受众分类
- **工程师** → architecture.md, evm-v2-migration.md, a11-storage-indexer-v2.md
- **项目经理** → project-status-zh.md, delivery-roadmap.md, backlog.md
- **安全审计员** → p0-evidence.md, acceptance-evidence.md, ADR 目录
- **AI 助手** → project-knowledge-base-zh.md

## 📋 快速导航

### 新工程师入职
1. 阅读 [项目状态总览](project-status-zh.md)
2. 理解 [系统架构](architecture.md)
3. 查看 [项目知识库](project-knowledge-base-zh.md)
4. 检查 [待办清单](backlog.md)

### 当前重点任务
- ✅ **A11 完成** - V2 EVM 事件索引器已实现（需测试）
- ⏳ **A04 测试** - Web Moderation UI 需要 E2E 验证
- ⏳ **B05 安全审查** - 九合约审计待启动
- ⏳ **B02/B03** - 清洁验收和钱包测试

## 🔧 合约和代码

### EVM V2 Suite（9个合约）
详见 `../contracts/evm-v2/README.md`

**基础设施：**
1. SuiteDirectory - 根注册表
2. BootstrapCoordinator - 顺序导入协调器

**生产模块：**
3. RepositoryCore - 仓库核心
4. RecoveryModule - 恢复模块
5. ModerationModule - 内容审核
6. EconomicModule - 经济模型
7. UsernameModule - 用户名系统
8. BadgeModule - 徽章系统
9. ReleaseModule - 版本发布

### CLI 工具
详见 `../cli/README.md`

### Web 应用
详见 `../web/README.md`

## 📚 历史和归档文档

以下文档已归档（仅供历史参考）：
- `archived-repositories.md` - 归档仓库信息
- `gogs-frontend-redesign.md` - 前端重新设计（历史）
- `evm-v2-repair-plan.md` - P0 修复计划（已完成）
- `keplr-acceptance.md` - Keplr 钱包测试（历史）
- `target-topology-migration.md` - 拓扑迁移（历史）

CosmWasm V1 相关文档已迁移至 `../archive/cosmwasm-v1/docs/`

## 🔗 外部资源

### Injective 文档
- [Injective EVM 文档](https://docs.injective.network/developers-evm/)
- [网络信息](https://docs.injective.network/developers-evm/network-information)
- [EVM 集成 FAQ](https://docs.injective.network/developers-evm/evm-integrations-faq)

### 工具和框架
- [Foundry](https://book.getfoundry.sh/)
- [Go-Ethereum](https://geth.ethereum.org/docs)

## 📝 文档维护

### 何时更新
- **里程碑完成** → 更新 project-status-zh.md, delivery-roadmap.md
- **架构变更** → 创建或更新 ADR
- **证据收集** → 更新相关证据文档
- **新风险** → 更新 delivery-roadmap.md 风险登记册

### 文档原则
1. **证据与计划分离** - CI 运行和收据是证据，计划不是
2. **提交绑定** - 所有证据绑定精确的 40-hex 提交 SHA
3. **不制造证据** - 测试输出不是部署证据
4. **保持同步** - 代码变更时同步更新文档

## 🚨 重要提示

### 安全注意事项
- 所有私钥和助记词必须保存在 `~/.igit/` 或加密的 keystore 中
- 切勿提交包含真实密钥的文件到 git
- 生产部署前必须完成独立安全审查

### 当前阻塞项
1. **A11 测试验证** - V2 索引器需要在测试网验证后才能用于生产
2. **安全审查** - 九合约需要独立审计
3. **钱包验收** - Web UI 需要真实钱包和测试网测试

---

**最后更新：** 2026-09-12  
**文档版本：** 2.0（精简版）  
**维护者：** Next Injective Git 团队

`````

</details>

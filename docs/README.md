# 文档索引

本目录包含 Next Injective Git (igit) 项目的所有技术文档、架构决策和证据记录。

## 快速导航

### 🚀 快速开始

- **[项目状态总览 (中文)](project-status-zh.md)** - 项目整体状态、里程碑和行动计划的快速概览
- **[Project Status (English)](project-status.md)** - Quick overview of project status, milestones, and action plan
- **[README.md](../README.md)** - 项目介绍和基本设置说明

### 📋 规划和跟踪

- **[Delivery Roadmap](delivery-roadmap.md)** - 详细的交付时间线、依赖关系和退出标准
- **[Backlog](backlog.md)** - 细粒度工程任务清单
- **[Open Questions](open-questions.md)** - 需要明确审查的决策（9 项待定）

### 🏗️ 架构和设计

- **[Architecture](architecture.md)** - 不可变 EVM Suite 架构和数据平面边界
- **[ADR 0001: EVM V2 Runtime and Migration Scope](adr/0001-evm-v2-runtime-and-migration-scope.md)** - EVM V2 运行时和迁移范围决策
- **[ADR 0002: Pluggable Pack Storage](adr/0002-pluggable-pack-storage.md)** - 可插拔 pack 存储方向和要求

### 🔒 证据和审查

- **[P0 Evidence Record](p0-evidence.md)** - P0 基线的提交绑定 CI 和本地验证证据
- **[Acceptance Evidence](acceptance-evidence.md)** - 必需的真实证据和绑定规则
- **[Release and Cutover](release.md)** - 发布内容和切换门控

### 🔧 技术实现

- **[EVM V2 Migration](evm-v2-migration.md)** - 迁移工作流和完成定义
- **[EVM V2 Repair Plan](evm-v2-repair-plan.md)** - P0 修复计划（历史）
- **[Infrastructure](infrastructure.md)** - 当前 IPFS 数据平面（非 EVM 或对象存储验收）
- **[CI Web Publishing](ci-web-publishing.md)** - 每树 CI 门控和 Web 生产发布路径
- **[Monitor](monitor.md)** - 监控和可观测性

### 🔐 安全和合规

- **[EVM Wallet Compatibility](evm-wallet-compatibility.md)** - 钱包兼容性测试和结果
- **Security Review** - 待完成（P1 阻塞器）

### 📚 其他文档

- **[Push Setup](push-setup.md)** - 推送设置说明
- **[EVM V2 Repo Identity](evm-v2-repo-identity.md)** - 仓库身份设计
- **[Gogs Frontend Redesign](gogs-frontend-redesign.md)** - 前端重新设计（历史）
- **[Archived Repositories](archived-repositories.md)** - 归档仓库信息
- **[Pinning Infrastructure](pinning-infrastructure.md)** - IPFS 固定基础设施
- **[Target Topology Migration](target-topology-migration.md)** - 目标拓扑迁移
- **[Feegrant Policy](feegrant-policy.md)** - Fee grant 策略
- **[Keplr Acceptance](keplr-acceptance.md)** - Keplr 钱包接受测试

## 文档状态矩阵

| 文档类别 | 当前状态 | 最后更新 |
|---|---|---|
| 项目状态和规划 | ✅ 最新 | 2026-08-21 |
| 架构和 ADR | ✅ 稳定 | 2026-08-18 |
| P0 证据 | ✅ 完整 | 2026-08-19 |
| P1 证据 | ⏳ 待收集 | - |
| 安全审查 | ⏳ 待启动 | - |
| 用户文档 | ⚠️ 需更新 | 2026-08-16 |

## 文档维护指南

### 何时更新

- **每次里程碑完成后** - 更新 Project Status、Delivery Roadmap 和 Backlog
- **重大架构变更时** - 创建或更新 ADR
- **证据收集后** - 更新相关证据文档
- **发现新风险时** - 更新 Delivery Roadmap 风险登记册
- **用户报告问题时** - 更新故障排除文档

### 文档原则

1. **证据与计划分离** - CI 运行、收据和部署清单是证据；计划和时间线不是
2. **提交绑定** - 所有证据必须绑定精确的 40-hex 提交 SHA
3. **不制造证据** - 测试输出、fixture 和本地探测不是部署或迁移证据
4. **保持同步** - 当代码变更影响文档时，在同一 PR 中更新文档
5. **双语支持** - 关键状态文档提供中英文版本

### 文档所有权

| 文档 | 主要维护者 | 审查者 |
|---|---|---|
| Project Status | 项目协调员 | 技术负责人 |
| Delivery Roadmap | 项目协调员 | 全体团队 |
| Backlog | 工程师 | 技术负责人 |
| ADR | 提案作者 | 架构师 + 团队 |
| 证据文档 | 运营工程师 | 安全审查员 |
| 技术文档 | 相关组件工程师 | 技术负责人 |

## 相关资源

### 外部文档

- [Injective EVM Documentation](https://docs.injective.network/developers-evm/)
- [Injective Network Information](https://docs.injective.network/developers-evm/network-information)
- [Injective EVM FAQ](https://docs.injective.network/developers-evm/evm-integrations-faq)
- [Foundry Book](https://book.getfoundry.sh/)
- [Go-Ethereum Documentation](https://geth.ethereum.org/docs)

### 代码仓库

- [Injective Core](https://github.com/InjectiveFoundation/injective-core)
- [Injective Solidity Contracts](https://github.com/InjectiveLabs/solidity-contracts)
- [Foundry](https://github.com/foundry-rs/foundry)

### 社区和支持

- Injective Discord
- Injective Telegram
- GitHub Issues

---

**最后更新：** 2026-08-21  
**文档版本：** 1.0  
**维护者：** Next Injective Git 团队

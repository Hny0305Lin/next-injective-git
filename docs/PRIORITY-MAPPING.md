# A0*/B0* → P1.*/P2.* 优先级映射表

本文档定义从旧标记到新优先级体系的映射关系。

## 映射规则

### P1 - 测试网就绪（关键路径，原 A01-A13 + B01-B06 关键项）

| 新编号 | 原编号 | 名称 | 状态 |
|-------|-------|------|------|
| P1.1 | A04 | Web Moderation UI | ✅ 实现完成 |
| P1.2 | A11 | V2 事件索引器 | ✅ 实现完成，需测试 |
| P1.3 | A11-补充 | ABI 解码器完善 | ⏳ 待实现 |
| P1.4 | A11-补充 | Reorg 检测 | ⏳ 待实现 |
| P1.5 | B05 | 安全审查 | ⏳ 待启动 |
| P1.6 | B02+B03 | 清洁验收测试 | ⏳ 待执行 |

### P2 - 功能完善（非阻塞，原 A0*/B0* 非关键项）

| 新编号 | 原编号 | 名称 | 状态 |
|-------|-------|------|------|
| P2.1 | - | Recovery UI | 📋 计划中 |
| P2.2 | - | Release UI | 📋 计划中 |
| P2.3 | - | Username Claim UI | 📋 计划中 |
| P2.4 | A11-补充 | RPC 错误处理 | 📋 计划中 |
| P2.5 | - | 监控告警 | 📋 计划中 |

### 已完成/验证存在（不需要独立项目）

| 原编号 | 状态 | 说明 |
|-------|------|------|
| A01 | `superseded` | 目标脚本已 Suite 化 |
| A02 | `superseded` | 单一 Directory 验证已存在 |
| A03 | `verified-existing` | Moderation hooks 已实现 |
| A05 | `verified-existing` | Fork 功能已完整 |
| A06 | `verified-existing` | Recovery 后端已完整 |
| A07 | `verified-existing` | Username 后端已完整 |
| A08 | `verified-existing` | Release 后端已完整 |
| A12 | `verified-existing` | Web activity 已正确 |

### 证据类/开放项（需外部授权）

| 原编号 | 状态 | 说明 |
|-------|------|------|
| A09 | `evidence-only` | 治理 ADR（需要时再做） |
| A10 | `implemented-needs-evidence` | 迁移工具完整，需真实执行 |
| A13 | `evidence-only` | 部署门禁已就位 |
| B01 | `evidence-only` | 部署 profile 准备 |
| B04 | `evidence-only` | 迁移执行（需授权） |
| B06 | `blocked-on-P1.2` | 存储生产（依赖 P1.2） |

---

## 关键说明

1. **P1 是关键路径** - 所有测试网 cutover 必需项
2. **P2 是功能完善** - 改善用户体验但不阻塞测试网
3. **已验证存在的不再独立列项** - 已在代码库中完整实现
4. **证据类项目视需求决定** - 不是代码实现任务

**生效日期：** 2026-09-12  
**后续文档必须使用 P1.*/P2.* 编号**

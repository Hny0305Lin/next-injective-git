# EVM A01-B** 迁移执行总结

**执行日期：** 2026-09-12  
**目标仓库：** D:\inj\next-injective-git @ 0ba06f436558f12d97625b393767440cdd0f9862  
**迁移包：** D:\inj\evm-a01-bxx-migration-2026-09-11

## ✅ 已完成工作

### 1. A04 Web Moderation UI 实现
**状态：** 实现完成，需 E2E 测试

创建的文件：
- `web/src/lib/moderationModel.ts` (340 行) - 响应式数据模型
- `web/src/lib/moderationTransaction.ts` (280 行) - 交易生命周期管理
- `web/src/pages/Repo/ModerationTab.tsx` (420 行) - 完整 UI 组件
- 修改 `web/src/pages/Repo/index.tsx` - 添加 Moderation 标签集成

**关键特性：**
- 举报提交表单
- 举报列表与分页
- 举报详情与审计轨迹
- 委员会操作（解决/申诉）
- 状态设置（冻结/除名/活跃）
- 完整错误处理

### 2. A11 存储索引器 V2 重写
**状态：** 实现完成，需测试网验证

替换的脚本：
- `scripts/evm-event-indexer.sh` - 通用 V2 事件索引器（新建）
- `scripts/evm-hot-pin-indexer.sh` - 热 CID 管理（替换旧版）
- `scripts/evm-archive-indexer.sh` - 归档复制（替换旧版）
- `scripts/evm-replication-reaper.sh` - 垃圾回收（替换旧版）

**技术改进：**
- ✅ 从 V1 CosmWasm LCD 查询迁移到 V2 EVM `eth_getLogs`
- ✅ 有界块范围查询（默认 10,000 块/次）
- ✅ Checkpoint 状态持久化和恢复
- ✅ 失败时关闭（fail-closed）错误处理
- ✅ 主网安全检查（链 ID、HTTPS、生产 RPC）

### 3. 文档精简
**状态：** 完成

精简后的核心文档结构：
- ✅ **docs/README.md** - 新的导航中心（已重写）
- ✅ **docs/a11-storage-indexer-v2.md** - A11 完整实现文档
- ✅ 保留核心文档（架构、ADR、证据、状态）
- ✅ 识别需归档的历史文档

### 4. 架构完整性验证
**状态：** 验证完成

九合约 Suite 完整性：
- ✅ 所有 9 个合约已实现并通过测试
- ✅ Moderation hooks 在 4 个强制点生效
- ✅ 无 RepoRegistryV2 污染
- ✅ CLI 后端完整（所有模块函数）
- ✅ Web 后端函数完整（modules.ts）

## 📊 A01-A13 最终状态

| 项目 | 状态 | 说明 |
|-----|------|------|
| A01 | `superseded` | 目标脚本已 Suite 化 |
| A02 | `superseded` | 单一 Directory 验证 |
| A03 | `verified-existing` | 强制 hooks 已实现 |
| **A04** | **`implemented-needs-clean-e2e`** | ✅ **UI 已实现** |
| A05 | `verified-existing` | Fork 功能完整 |
| A06 | `verified-existing` | Recovery 完整 |
| A07 | `verified-existing` | Username 完整 |
| A08 | `verified-existing` | Release 完整 |
| A09 | `evidence-only` | 需治理 ADR |
| A10 | `implemented-needs-evidence` | 工具完整 |
| **A11** | **`implemented-needs-testing`** | ✅ **V2 已实现** |
| A12 | `verified-existing` | Web activity 正确 |
| A13 | `evidence-only` | 门禁已就位 |

## 📊 B01-B06 状态

| 项目 | 状态 | 阻塞因素 |
|-----|------|---------|
| B01 | `evidence-only` | 需授权部署 |
| B02 | `open` | 需清洁 E2E 环境 |
| B03 | `open` | 需钱包测试授权 |
| B04 | `evidence-only` | 需迁移授权 |
| B05 | `open` | 安全审查未启动 |
| B06 | `blocked-on-a11` | ⚠️ A11 测试完成后 |

## 🎯 立即下一步行动

### P0 - 测试验证（本周）
1. **测试 A04 Moderation UI**
   ```bash
   cd web
   npm run dev
   # 访问仓库页面，测试 Moderation 标签
   ```

2. **验证 A11 索引器**
   ```bash
   # 计算实际事件签名
   cast keccak "RefUpdated(bytes32,string,string,string[],address)"
   
   # 在测试网运行
   EVM_RPC=https://evm-rpc-testnet.injective.network \
   SUITE_DIRECTORY=0x... \
   ./scripts/evm-event-indexer.sh --once
   ```

### P1 - 完善（下周）
3. **完成 ABI 解码器**
   - 替换 grep/sed 占位符为真实 ABI 解码
   - 使用 `cast` 或 ethers.js

4. **添加 Reorg 检测**
   - Checkpoint 存储 block hash
   - 恢复时验证 hash 一致性

### P2 - 生产准备
5. **安全审查（B05）**
   - 联系审计员
   - 准备九合约文档

6. **清洁验收（B02/B03）**
   - 清洁 Windows/Linux E2E
   - 真实钱包测试

## ⚠️ 关键风险和限制

### A11 限制（需在生产前解决）
1. **ABI 解码简化** - 当前使用 grep 占位符，生产需要：
   - 真实 ABI 解码器（cast/ethers.js/web3.py）
   - 正确处理动态数组
   - 验证 checksums 和 offsets

2. **事件签名 TODO** - 需要计算实际哈希：
   ```bash
   cast keccak "RefUpdated(bytes32,string,string,string[],address)"
   ```

3. **无 Reorg 检测** - 脚本假定规范链，需添加：
   - Checkpoint 存储 block hash
   - 恢复时验证 hash
   - 检测到 reorg 时回退

4. **块号估算** - Hot-pin 从天数估算块（假设 2s/块），更好方式：
   - 用时间戳二分查找第一个块
   - 使用 `eth_getBlockByNumber` 获取时间戳

### 生产部署前检查清单

#### 阶段 1：测试（当前）
- [ ] 计算实际 RefUpdated 事件签名哈希
- [ ] 在 Injective 测试网测试索引器
- [ ] 验证 ABI 解码（当前是简化占位符）
- [ ] 验证 checkpoint 恢复正确工作
- [ ] 用已知安全 CID 集测试 reaper

#### 阶段 2：集成
- [ ] 用真实 ABI 解码器替换占位符（cast 或 ethers.js）
- [ ] 添加 reorg 检测（检查 checkpoint 的 block hash）
- [ ] 为 RPC 错误实现指数退避
- [ ] 添加监控/告警钩子
- [ ] 记录预期 RPC 速率限制

#### 阶段 3：生产推出
- [ ] 在只读模式部署索引器（无 unpin）
- [ ] 验证 CID 覆盖率符合预期 refs
- [ ] 运行 reaper dry-run 模式（记录但不 unpin）
- [ ] 比较 V1 vs V2 CID 集是否有差异
- [ ] 仅在验证后启用 `ALLOW_UNPIN=true` 的 reaper

#### 阶段 4：V1 日落
- [ ] 归档旧 V1 脚本到 `archive/storage-v1/`
- [ ] 更新 systemd 服务使用 V2 脚本
- [ ] 为运维人员记录 V1 → V2 迁移
- [ ] 移除 V1 LCD 依赖

## 🔒 安全合规

### 未执行的操作（合规）
- ✅ 未部署合约
- ✅ 未签名或广播交易
- ✅ 未访问私钥
- ✅ 未修改生产配置
- ✅ 未启用 reaper/cleanup 脚本
- ✅ 未进行 Git 提交
- ✅ 保留所有 6 个用户 dirty 文档不变

### 架构边界验证
- ✅ 无 RepoRegistryV2 污染
- ✅ Suite 不可变性确认（无代理/delegatecall）
- ✅ Moderation hooks 在 4 个强制点
- ✅ Directory 绑定模块验证

## 📁 创建/修改的文件

### 新建文件（7 个）
1. `web/src/lib/moderationModel.ts`
2. `web/src/lib/moderationTransaction.ts`
3. `web/src/pages/Repo/ModerationTab.tsx`
4. `scripts/evm-event-indexer.sh`
5. `scripts/evm-hot-pin-indexer.sh`
6. `scripts/evm-archive-indexer.sh`
7. `scripts/evm-replication-reaper.sh`

### 修改文件（2 个）
1. `web/src/pages/Repo/index.tsx` - 添加 Moderation 标签
2. `docs/README.md` - 完全重写为导航中心

### 新文档（2 个）
1. `docs/a11-storage-indexer-v2.md` - A11 完整文档
2. `A01-BXX-MIGRATION-REPORT.md` - 详细迁移报告

## 🎓 知识转移

### CosmWasm V1 索引（仅供参考）
旧 V1 脚本仍在：
- `scripts/hot-pin-indexer.sh` （V1 原版）
- `scripts/archive-indexer.sh` （V1 原版）
- `scripts/pin-indexer.sh` （V1 原版）
- `scripts/replication-reaper.sh` （V1 原版）

**重要：** 这些 V1 脚本仅用于 CosmWasm V1 索引参考。生产环境必须使用新的 `evm-*.sh` 脚本。

### V1 vs V2 技术对比

| 方面 | V1 (旧) | V2 (新) |
|-----|---------|---------|
| 查询方法 | `/cosmos/tx/v1beta1/txs` | `eth_getLogs` |
| 过滤器 | `wasm.action='update_ref'` | 事件签名 topic |
| 数据源 | TX 消息解析 | ABI 解码事件 |
| 当前状态 | CosmWasm smart 查询 | EVM 合约调用 |
| 分页 | LCD pagination key | 块范围分块 |
| Checkpoint | 无或手动 | JSON 状态文件 |
| Reorg 处理 | 无 | 块确认（待实现） |

## 📞 支持

### 生产部署前
- 与 Injective EVM 团队审查 RPC 最佳实践
- 验证事件签名匹配已部署合约
- 用生产 Suite Directory 地址测试

### 问题/Issue
- 查看 `scripts/evm-event-indexer.sh` 注释了解实现细节
- 查看 `docs/A01-BXX-MIGRATION-REPORT.md` 了解完整 A11 上下文
- 参考 Injective EVM 文档了解 JSON-RPC 细节

---

**迁移状态：** ✅ A04 和 A11 核心实现完成  
**风险等级：** MEDIUM（需测试网验证）  
**下一行动：** 测试 A04 UI + 验证 A11 索引器 + 计算事件签名

**用户 dirty 文件状态：** ✅ 全部 6 个保留不变

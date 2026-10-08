# Web 全局仓库搜索设计草案

> 状态：DRAFT（草案）。仅为设计提案，不代表已实现或已授权；按 [docs/README.md](README.md) 的规则，计划不代表实现或授权。本文件不改变任何 PASS/FAIL/BLOCKED 状态，不构成验收证据。
>
> 提案日期：2026-10-09

## 1. 背景与问题

当前 Web 右上角全局搜索（`web/src/App.tsx` 的 `submitSearch` -> `web/src/lib/search.ts` 的 `buildSearchPath`）本质是“路径跳转”：

- 输入 `owner` -> 跳转 `/:owner`（Owner 页再经 `resolveOwner` 把用户名/地址解析为地址，列出仓库）；
- 输入 `owner/repo`、`igit://owner/repo`、`archive://owner/repo` -> 跳转对应仓库路由。

因此它只支持“地址 / 用户名 / owner（/repo）”这类以 owner 为先的导航；单独输入仓库名（不知道 owner）时，仓库名会被当成 owner，最终在 Owner 页报“找不到该 owner”。这与 [gogs-frontend-redesign.md](gogs-frontend-redesign.md)（历史参考）“快速搜索用户或仓库”的目标和 GitHub 的使用习惯有差距：**缺少按仓库名的全局检索**。

## 2. 目标 / 非目标

目标：

- 输入单个关键词时，按仓库名（及 `owner/repo` 前缀）检索已配置 Suite 的仓库，下拉展示结果，点击直达 `/:owner/:repo`；
- 完整保留现有快速路径：`inj1...`/`0x...` 地址、用户名、`owner/repo`、`igit://`、`archive://` 前缀；
- 严格复用现有信任模型：一切权威读取仍经 `verifySuite` + `readModule`（checked ABI），索引只作导航提示，不作为事实来源；
- 纯前端实现，不改合约、不部署、不引入任何后端服务。

非目标：

- 不新增合约全局枚举/搜索接口（见第 3 节与 [backlog.md](backlog.md) R01）；
- 不引入 A11 式外部索引器服务（[a11-storage-indexer-v2.md](a11-storage-indexer-v2.md) 属 HISTORICAL）；
- 不做代码全文检索、issue/PR 检索；
- 不改变 cosmwasm-v1 归档的访问方式（R5：EVM 与归档永不混读）。

## 3. 事实依据（现状核对）

| 事实 | 出处 |
|---|---|
| 合约没有全局仓库枚举，只有按 owner 分页 `listRepositoriesPage(address owner,uint256 cursor,uint256 limit)`；v3 与 v4 successor 的 core 均如此 | `contracts/evm-v2/src/RepositoryCore.sol`、`contracts/evm-v2-successor/src/RepositoryCore.sol`；backlog R01 亦明确“消除不存在的 repositoryCount/repositoryIdAt 依赖，明确仓库枚举来源” |
| 仓库创建/所有权事件：`RepositoryCreated(bytes32 indexed repoId, address indexed owner, string name, bytes32 indexed forkedFrom)`、`OwnershipTransferred(...)`；两代 core 均存在 | 同上 |
| 治理状态事件：`RepositoryStatusSet(bytes32 indexed repoId, RepoStatus status, ...)` | `contracts/evm-v2/src/ModerationModule.sol` |
| Web 已有事件扫描基建：`contractActivity`、范围二分重试（`EVM_LOG_RANGE_LIMIT=10_000`）、活动窗口（`EVM_ACTIVITY_BLOCK_WINDOW=100_000`） | `web/src/lib/activity.ts` |
| 权威解析已存在：`resolveRepo`（canonical owner/name）、`repoInfoById(repoId)` | `web/src/lib/registry.ts` |
| 多目录支持：`parseSuiteDirectories`（逗号分隔），内置 profile 含两个 SuiteDirectory | `web/src/lib/profile.ts` |
| 同名仓库跨代并存，dedupe 键为 `owner/name/suite_version` | `web/src/lib/registry.ts`（`listRepos` 合并逻辑） |
| Owner 页对仓库列表有本地过滤，但依赖已知 owner | `web/src/pages/Owner.tsx` |

## 4. 方案设计

### 4.1 索引来源（只读事件扫描）

对每个已配置 SuiteDirectory：经 `verifySuite` 得到 core 模块地址后，扫描其日志：

- `RepositoryCreated` -> 索引条目 `{ repoId, owner, name, suiteVersion, blockNumber, txHash, logIndex }`；
- `OwnershipTransferred` -> 更新条目 owner（避免索引 owner 过期）；
- `RepositoryStatusSet` -> 更新条目治理标记（active/frozen/delisted）；点击结果时仍以合约 `effectiveStatus` 读取为准。

事件扫描复用/抽取 `activity.ts` 的分段与二分逻辑，落到独立的 `web/src/lib/repo-index.ts`（扫描与解析为纯函数，便于单测）。

### 4.2 存储与增量

- localStorage 键：`igit.repo-index.v1.<chainId>.<hash(suiteDirList)>`，值为 `{ cursorBlock, entries[] }`；
- 首次构建两阶段：先“近窗口”（最近 `EVM_ACTIVITY_BLOCK_WINDOW` 块，倒序扫，立即可用），再后台“回填”（从部署块/创世正序扫，推进 cursorBlock）；
- 增量更新：从 `cursorBlock+1` 扫到 latest；为防 reorg，落 cursor 前回退若干确认块（如 64）重扫，并以 `(txHash, logIndex)` 去重；
- 容量上限：条目数与字节上限（如 50k 条 / 4MB），超限降级为“仅近窗口”并在 UI 提示；
- 索引可随时丢弃重建（清缓存即恢复），不承诺完整性：**best-effort 导航提示**。

### 4.3 触发与性能

- 输入至少 2 个字符且去抖（约 300ms）后才触发检索；索引构建在空闲时进行（`requestIdleCallback`/后台任务），不进首屏关键路径；
- 匹配规则：名字前缀优先 -> 名字包含 -> `owner/repo` 前缀；返回前 N（默认 20）条，高亮命中片段；
- 公共 RPC 压力：分段大小沿用 `EVM_LOG_RANGE_LIMIT`，扫描可取消（组件卸载/输入变更时 abort）。

### 4.4 多 SuiteDirectory 与版本

- 每个已配置 SuiteDirectory 独立建索引（同一存储键内分区）；
- 结果去重与排序沿用 `registry.ts` 规则（`owner/name/suite_version`），跨代同名仓库以版本徽标（V3/V4）区分，配置顺序优先；
- 不引入跨目录合并写入，也不做任何合约改动。

### 4.5 UI 交互（App.tsx 搜索框）

- 下拉复用现有 `search-history` 视觉体系，新增分组：Repositories（索引结果）/ Recent（历史）；
- 键盘上下选择、Enter 跳转、Esc 关闭；保留 `/` 聚焦快捷键与最近历史；
- 点击结果：先经 `repoInfoById(repoId)` 取 canonical owner（应对索引 owner 过期），再跳转 `/:owner/:repo`；
- 保留回退：索引未就绪/无结果时，Enter 仍按现行为 `buildSearchPath` 直接跳转；
- 占位文案更新为体现仓库检索（如 `Search repositories, owners, addresses...`）。

### 4.6 治理与安全过滤

- 默认隐藏 `delisted`；`frozen` 保留但显示徽标（与 `listRepos` 默认仅 active 的取向一致）；
- 仓库元数据（描述、更新时间）一律以点击后的合约读取为准，索引内不缓存描述。

### 4.7 归档隔离

- cosmwasm-v1 归档不并入本索引；`archive://` 前缀行为不变（遵守 [suite-version-compatibility.md](suite-version-compatibility.md) R5）。

## 5. 安全与合规检查表（对齐 CLAUDE.md）

- 不添加任何直接模块地址：日志只扫“已配置 SuiteDirectory 经 verifySuite 绑定得到的 core/moderation 模块地址”；
- 不引入兼容后端 / legacy fallback / 代理 / diamond / delegatecall；
- 索引不是事实来源：导航与页面渲染仍走合约读取（verifySuite -> readModule，checked ABI，不手写 selector）；
- 只读功能：不签名、不发交易、不需要密钥；
- 无部署动作、无合约改动、内置 profile 的 Suite 地址维持现状；
- 存储分层不混淆：索引仅含 EVM Suite 仓库元数据（repoId/owner/name/治理标记），不触碰 pack 存储路径（v3 IPFS / v4 BYOS 分发维持原样）。

## 6. 测试与验收（实现时执行）

- 单测（`npm run test:api`）：
  - 扩展 `web/test/search.test.mjs`：检索词归一化、与既有 `buildSearchPath` 行为不回归；
  - 新增 `web/test/repo-index.test.mjs`：ABI 解码 RepositoryCreated/OwnershipTransferred/RepositoryStatusSet、增量 cursor、reorg 回退、`(txHash, logIndex)` 去重、容量上限、匹配排序；
- `npm run typecheck`、`npm run build`；
- 浏览器手动验收（本地 dev + 内置测试网目录）：关键词命中 -> 跳转 `/:owner/:repo`；delisted 不出现；frozen 带徽标；索引未就绪时回退行为；清 localStorage 后可重建；
- 不涉及 Foundry、链上交易或部署 gate。

## 7. 与现有文档的一致性核对（结论）

| 文档 | 核对结论 |
|---|---|
| [gogs-frontend-redesign.md](gogs-frontend-redesign.md)（历史参考） | 其搜索框规格为 owner/owner-repo/igit://.../inj1...，本草案是其超集（新增仓库名检索），无冲突 |
| [suite-version-compatibility.md](suite-version-compatibility.md) R6 | R6 写“一次只指向一个 SuiteDirectory”，与现行 web 代码（`parseSuiteDirectories` 逗号分隔、内置两个地址）存在既有漂移；本草案按多目录现实设计，建议另行修订 R6 措辞（不在本草案内改动） |
| [backlog.md](backlog.md) R01 | 合约确无全局枚举（repositoryCount/repositoryIdAt 不存在）；本草案不假设任何新合约接口，枚举来源=事件+按 owner 分页，与 R01 的“明确仓库枚举来源”一致 |
| [a11-storage-indexer-v2.md](a11-storage-indexer-v2.md)（HISTORICAL） | 外部索引器属旧任务；本草案明确为浏览器内只读扫描，不引入后端服务，避免混淆 |
| [README.md](README.md) / [project-status.md](project-status.md) | 本文件为 DRAFT，不改变任何状态、不构成证据 |

## 8. 待决问题

1. 内置 SuiteDirectory 的部署块是否写入 profile（可显著减少首扫成本）；
2. 是否顺带索引 `UsernameRegistered` 提供用户名建议（v1 可不做）；
3. 结果排序权重：更新时间 vs 字典序 vs 前缀命中优先；
4. localStorage 容量预算的最终数值（浏览器约 5MB）。

## 9. 实现影响面（预计）

- 新增：`web/src/lib/repo-index.ts`、`web/test/repo-index.test.mjs`；
- 修改：`web/src/App.tsx`（搜索框交互）、`web/src/lib/activity.ts`（抽出可复用扫描）、`web/src/lib/search.ts`（如需匹配辅助）、`web/test/search.test.mjs`、相关样式；
- 不改：合约、CLI、内置 profile 的 Suite 地址。

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
| 公共测试网 RPC 的 `eth_getLogs` 对旧区块静默裁剪：对 -244k 块单块查询返回空且无报错（2026-10-09 实测，Blockscout 佐证事件存在） | 实测记录；催生 4.2 的吸收层设计 |

## 4. 方案设计

### 4.1 索引来源（只读事件扫描）

对每个已配置 SuiteDirectory：经 `verifySuite` 得到 core 模块地址后，扫描其日志：

- `RepositoryCreated` -> 索引条目 `{ repoId, owner, name, suiteVersion, blockNumber, txHash, logIndex }`；
- `OwnershipTransferred` -> 更新条目 owner（避免索引 owner 过期）；
- `RepositoryStatusSet` -> 更新条目治理标记（active/frozen/delisted）；点击结果时仍以合约 `effectiveStatus` 读取为准。

事件扫描复用/抽取 `activity.ts` 的分段与二分逻辑，落到独立的 `web/src/lib/repo-index.ts`（扫描与解析为纯函数，便于单测）。

### 4.2 存储与增量（按实测调整，2026-10-09）

- localStorage 键：`igit.repo-index.v1.<chainId>`，内容为各 SuiteDirectory 分片（事件索引 + 走查游标）与吸收层条目；
- 索引来源三路合并：
  1. 头窗口事件扫描：最近 `REPO_INDEX_RECENT_WINDOW`（100k）块的 `eth_getLogs`，结果立即可用；
  2. 历史走查：从头窗口下界向创世方向降序分块扫描（10k 块/查询，8 路并发，16 块/tick），条目按最新事件位置守卫，乱序合并正确；连续 4 轮全空或连续 3 次硬错误即自适应暂停（下个会话可续）；
  3. 吸收层：打开 Owner 页或连接钱包时，经 `listRepositoriesPage` 按 owner 合约枚举并入索引（不依赖日志，不受 RPC 日志裁剪影响），同 owner/name 跨代仓库按键含 suiteVersion 共存；
  4. 公共浏览器回填：搜索页首次使用时，向 profile 中已配置的 blockscout（evmExplorer）拉取全量 RepositoryCreated/OwnershipTransferred 事件（Etherscan 风格 logs API，带 12s 超时，失败即降级为前三路）；该源弥补公共 RPC 对旧区块日志的静默裁剪，仍是导航提示、不作为事实来源；
- 增量更新：头游标 `cursor+1` 起扫到 latest，回退 96 块 overlap 防 reorg；
- 已知限制：Injective 公共测试网 RPC 对旧区块日志存在静默裁剪（实测对 -244k 块的单块 `eth_getLogs` 也返回空），因此老仓库依赖吸收层覆盖；自建/归档 RPC 上走查可达全量历史；
- 容量上限：50k 条 / 4MB，超限停止扩充历史层；
- 索引可随时丢弃重建（清 localStorage 即恢复），不承诺完整性：best-effort 导航提示，点击结果仍以合约读取为准。

### 4.3 触发与性能

- 输入至少 2 个字符且去抖（约 300ms）后才触发检索；索引构建在空闲时进行（`requestIdleCallback`/后台任务），不进首屏关键路径；
- 匹配规则：名字前缀优先 -> 名字包含 -> `owner/repo` 前缀；返回前 N（默认 20）条，高亮命中片段；
- 公共 RPC 压力：分段大小沿用 `EVM_LOG_RANGE_LIMIT`，扫描可取消（组件卸载/输入变更时 abort）。

### 4.4 多 SuiteDirectory 与版本

- 每个已配置 SuiteDirectory 独立建索引（同一存储键内分区）；
- 结果去重与排序沿用 `registry.ts` 规则（`owner/name/suite_version`），跨代同名仓库以版本徽标（V3/V4）区分，配置顺序优先；
- 不引入跨目录合并写入，也不做任何合约改动。

### 4.5 UI 交互（App.tsx 搜索框 + /search 页，2026-10-09 调整）

- 搜索框回车一律进入独立路由 `/search?q=<词>&type=repositories`（GitHub 风格），不再把搜索词直接拼成 `/:owner` 或 `/:owner/:repo` 路径；`archive://` 前缀保留原有直达行为；
- 该设计消除了关键字与现有页面路由（/settings、/monitor、/explorer 等）的冲突：输入 Settings 回车停留在搜索页；
- 下拉新增固定动作项 "Search for <词>"；键盘方向键在仓库结果与最近历史间移动，选中仓库结果回车直达，未选中回车进 /search；
- /search 页结果来自客户端索引（三路事件 + 吸收层 + 浏览器回填），另做两类权威探测：q 形如 owner/name 时尝试 resolveRepo 给出 "Jump to" 卡片；q 能解析为地址/用户名时展示 owner 卡片并顺带吸收其仓库列表；
- 点击结果统一经 resolveEntryTarget（repoInfoById 或 resolveRepo）做链上权威解析后跳转；索引永远只是导航提示。

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

## 9. 实现影响面（已实施）
- 新增：`web/src/lib/repo-index.ts`、`web/test/repo-index.test.mjs`；
- 修改：`web/src/App.tsx`（搜索框交互与钱包吸收）、`web/src/pages/Owner.tsx`（页面吸收）、`web/src/lib/activity.ts`（导出扫描辅助并支持 topics 过滤）、`web/src/styles.css`（结果样式）；
- 未改：`web/src/lib/search.ts`（既有路径跳转行为保持不变）、合约、CLI、内置 profile 的 Suite 地址。


## 10. 实施结果（2026-10-09）

- 代码与测试落地：`npm run test:api` 163/163 通过（含 8 个 repo-index 单测）、`npm run typecheck` 与 `npm run build` 通过；
- 真实测试网端到端验证（Playwright + Chromium，本地 dev server）：访问 Owner 页吸收 3 个仓库（demo-showcase v3/v4、demo-showcase-byos v4）后，全局搜索输入 dem 下拉命中 3 条，方向键加回车跳转 `/:owner/:repo` 成功；截图存于会话 scratch 目录（1-owner-page.png / 2-search-dropdown.png / 3-repo-page.png）；
- 安全边界复核：索引仅为导航提示，点击后经 `repoInfoById`（有 repoId 时）或 `resolveRepo`（吸收条目）做权威解析；不新增模块地址、不发交易、不部署合约；delisted 条目不出现在结果中。

## 11. 演进方案对比（2026-10-09）

| 方案 | 说明 | 优点 | 取舍 | 建议 |
|---|---|---|---|---|
| A. 客户端多源索引（当前实现） | 链头事件 + 历史走查 + owner 枚举吸收 + 公共浏览器回填，/search 页承载 | 零后端、零合约改动、冷启动可用 | 公共浏览器可用性不受控；索引仅本浏览器；首次结果需数秒 | 已交付，作为基线 |
| B. 链上全局枚举（合约 v5 候选） | 给 core/directory 增加全局分页枚举（不做链上模糊匹配） | 真 direct-to-chain，CLI/外部工具同样受益 | 需要走 Suite v5 部署与迁移；枚举的存储/gas 成本；与非升级性原则的张力 | 记入 v5 路线图候选，只做枚举、匹配仍在客户端 |
| C. 自托管只读索引服务 | 独立 indexer 订阅事件入库，web 调只读 API（mini-A11 复活） | 全量、快、可排序分页联想、跨设备一致 | 引入运维组件，偏离浏览器直连定位；A11 历史 FAIL（漂移/reorg）教训；需重设安全边界 | 仓库量上万或需要代码搜索时再评估；默认关闭、自托管启用 |
| D. 公共浏览器为主后端 | 直接把 Blockscout API 当主搜索源 | 免自建、全量历史 | 停机/限流/改版即坏；测试网 explorer 曾观察到与 RPC 块高不同步；查询泄露第三方 | 仅作 best-effort 回退（当前定位），不做唯一来源 |
| E. 索引持久化增强 | localStorage 迁 IndexedDB/OPFS，支持导出导入索引包 | 弱网/离线可用，团队可分发缓存 | 只改善缓存不改善冷启动；需 TTL 与失效策略 | 低成本小步迭代（v1.6 候选） |

推荐路线：短期 A + E；中期按仓库规模在 B（链上枚举，v5）与 C（自托管索引器）之间二选一；D 永远只是回退源。用户名联想（UsernameRegistered）可作为后续补充源，与仓库索引同构。

## 12. 本轮修复记录（2026-10-09 第二轮）

- 缺陷：搜索页首次使用时浏览器回填源在事件扫描未建 shard 前执行而被静默跳过，且空结果被 Promise 永久缓存（表现为 Explorer source: ok (0 events)、结果为空）；已改为按需创建 shard；
- 缺陷：浏览器 fetch 无超时，explorer 挂起会拖住搜索页；已加 12s AbortSignal 超时；
- 回归验证：Playwright 全新会话中 /search?q=demo-showcase-byos 命中 1 条并直达仓库页；输入 Settings 停留在 /search 不再进入设置页；npm run test:api 165/165、typecheck、build 全部通过。
- 第三轮修复（线上反馈）：纯地址/用户名查询时，owner 探测结果除卡片外，其仓库列表直接并入结果（GitHub 搜索用户名即列出其仓库），消除误报的 No repositories found；搜索页移除索引状态脚注（状态仅保留在内部状态机）；探测进行中不再闪现空态；searchRepoEntries 支持裸地址按 owner 精确匹配（顶栏下拉同样受益）；
- 第四轮修复（线上反馈）：治理状态过期——浏览器回填源此前只拉 core 模块事件、缺少 ModerationModule 的 RepositoryStatusSet，且回填条目遮蔽了吸收层的新鲜 effectiveStatus；现回填源同时拉取状态事件并按区块位置排序折叠，mergeEntriesForSearch 让吸收层（合约直读）状态覆盖事件重放状态，两条路径（关键词/地址）均实时反映 frozen/delisted；
- 第五轮改进：加载指示改为结果区边框的 conic-gradient 旋转动画（@property 角度动画，去掉零散转圈符号）；结果非空时在下方显示 View other V1 Archive results 折叠链接（ChevronDown 旋转指示，参考 MapMonitor 折叠卡片交互），点击后原地展开加载 V1 归档结果（resolveCosmWasmV1Owner + listCosmWasmV1Repos，关键词查询无对应 owner 时行内提示），严禁跳转归档页面；归档结果行直达 /archive/cosmwasm-v1/:owner/:repo；
- 第五轮修订（用户反馈）：撤下结果区边框旋转动画（过于扎眼），改为搜索页右上角 Searching... 小号加载提示（探测/索引/解析期间显示）；V1 归档区去掉内衬边框容器，折叠按钮改为全宽头部样式并内嵌 CosmWasm V1 徽章，展开行与 EVM 结果行完全同宽同距对齐；
- 第六轮改进（用户反馈）：搜索页增加会话内快照缓存（searchSnapshots，按查询词键控，10 分钟信任期，上限 20 条）——从仓库页返回时立即恢复结果与归档展开态（实测 8ms），后台静默再验证且仅右上角提示、不影响布局，最新数据到达即刻替换；V1 归档折叠按钮移除 CosmWasm V1 徽章，徽章仅保留在仓库结果卡片上；
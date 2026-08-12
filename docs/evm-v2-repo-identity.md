# EVM V2 仓库身份、所有权转移与 Fork ADR

状态：**稳定身份和 ownership transfer 已完成源码实现；Foundry 运行、gas、
安全审查和有界 fork 仍待完成**

范围：`contracts/evm-v2`、统一 Chain API、CLI/remote helper、Web 和 V1
快照导入器。

本 ADR 只固定身份和状态语义；它不表示 EVM V2 已部署，也不把当前源码测试
描述为已通过 Foundry 或安全审计。

## 决策摘要

EVM V2 采用创建后永不改变的 `repoId` 作为仓库的内部身份。owner 是可变的
用户定位信息，name 在本 ADR 中保持 immutable；两者都不参与转移后的主存储键。合约维护当前定位
`(owner, name) -> repoId`，并保留历史定位别名 `(oldOwner, name) -> repoId`。

所有 refs、collaborators、经济状态、moderation、badge 和 report 都按稳定
`repoId` 归属。因此所有权转移只更新 owner、定位表和事件，不搬迁整个仓库的
映射。旧 URL 可以用于读取和 clone/fetch，但任何通过旧 URL 的写操作都必须
返回 `RepoMoved` 以及新的 canonical URL；不能静默把写入改发到新 owner。

Fork 创建一个新的稳定 `repoId`，只复制 V1 规定的 metadata 和 ref 快照，
不复制 collaborator、revenue split、sponsor、guardian、badge 或 moderation
report。小仓库可在单笔有界交易中完成；大仓库必须使用带 source snapshot
revision、隐藏目标和可恢复清理的批处理流程。生产实现不得包含无上限的 ref
复制循环。

这项决定避免了 `keccak(owner, name)` 作为永久主键时的 rekey/gas 和外部引用
失效问题，同时保留 `igit://owner/repo` 这一用户可见 URL 形状。

## 背景和现状

### CosmWasm V1 的实际身份行为

V1 的 `REPOS`、`REFS` 和多数 repo 扩展以 `(owner, repoName)` 为 key。`TransferOwnership`
使用 7 天 timelock，由新 owner 主动 `AcceptOwnership`；目标 owner 的同名仓库
会造成冲突，当前 owner 可以在接受前取消。转移和 guardian recovery 共用
`move_repo_ownership`，当前实现会：

- 移动 repo、refs、collaborators 和 sponsor totals，并保留 ref 的 SHA、URI、
  原始更新时间和 updater；
- 删除 revenue split；
- 删除 guardian 配置和 recovery proposal，让新 owner 重新设置 guardian；
- 不移动 `BADGES_BY_REPO` 或 moderation reports，因此记录中的旧 owner 会
  留下 stale association；
- 保留 metadata、moderation status、`forked_from`、created/updated 时间，且
  transfer 不推进 `updated_at`；
- 不调用 `ensure_not_frozen`，所以 Frozen/Delisted 仓库也可以转移所有权。

V1 fork 允许任意 sender fork 一个存在且 `Active` 的 source。Frozen 或 Delisted
source 都拒绝。fork 复制 description、default branch 和所有 refs；ref 的
SHA/URI 保留，但 `updated_at` 和 `updated_by` 改成 fork 时刻和 forker。它不
复制 collaborators、splits、sponsor totals、guardians、badges 或 reports，
并把 `forked_from` 保存为可读的 `owner/name` 字符串。IPFS URI 只是引用复制，
不重新上传对象。

这些行为是迁移的 V1 baseline，不代表所有 V1 的副作用都适合在 V2 复刻。
例如 revenue split 的删除和 badge/report 的 stale key 是已知历史行为；V2
应在稳定 ID 下保留经济和历史记录，并在 importer 中显式记录差异，而不是
无意中重现数据丢失。

### EVM V2 的当前身份行为

当前 `RepoRegistryV2.sol` 使用：

```solidity
keccak256(abi.encode(owner, repoName))
```

作为 `_repos`、`_refs`、`_refNames`、`_roles` 和 `_collaborators` 的主键计算。
`Repo` 目前包含 owner、name、metadata、timestamps、frozen 和 exists，但没有
transfer、fork 或 `forkedFrom`。refs 和 collaborators 以完整数组一次性返回，
数量上限分别是 1024 和 256。`MAX_PACK_URIS = 128` 现在同时限制单次
`updateRef` 输入和一个 ref 的最终累计 URI 数；普通非 force 更新在追加第 129
个唯一 URI 前原子回滚。结合单 URI 512 bytes 上限，单 ref 的 URI payload
已有硬边界，但整个仓库仍可能包含 `1024 * 128` 个 URI。

如果直接在这个模型上加入 transfer，owner 改变会改变 `repoId`，必须在一笔
交易中 rekey 所有 refs/collaborators 及未来扩展。即使 ref 数量上限存在，
1024 个 refs 加上最多 `1024 * 128` 个 URI 的字符串复制也可能超过 gas；不能
把输入上限当作单笔可行保证。如果未来扩大上限，原子迁移更不可行。
所有以旧 `(owner,name)` 保存的 badge、report、fork lineage 和 importer mapping
也容易失效。

## 方案比较

| 方案 | 优点 | 主要问题 | 结论 |
| --- | --- | --- | --- |
| A. 继续使用 `keccak(owner,name)`，转移时 rekey | 延续现有 storage；与 V1 key 直观对应 | owner 改变必然移动 refs/角色/扩展；单笔 gas 不可控；旧 URL 和外部引用失效；需要可恢复批迁移与复杂 pending 状态 | 仅可作为受限、过渡性的 fallback；不作为 V2 身份 |
| B. immutable `repoId` + current/historical locator | transfer O(1)；refs 和扩展无需搬迁；旧 URL 可读重定向；importer 可幂等 | 多两个定位表；客户端必须区分 canonical URL 和 alias；需要明确别名生命周期 | **采用** |
| C. immutable handle/token + successor/redirect tombstone | 身份与 URL 完全分离，可表达 rename/merge 历史 | 增加 handle、successor、redirect 多层状态；写入边界和查询复杂；容易混淆多种 ID | 保留为未来 rename/merge/governance 扩展 |

## 稳定 Repo ID 规范

### ID 生成

V2 native 创建使用域分离、无 packed 动态字段的确定性公式：

```text
repoId = keccak256(abi.encode(
  "igit:v2:repo",
  block.chainid,
  address(registry),
  initialOwner20,
  canonicalName
))
```

实现必须使用 `abi.encode` 或等价的长度明确编码，不能使用会产生动态字段
歧义的 `abi.encodePacked(owner, name, ...)`。`canonicalName` 在创建前按 V2
名称校验规范化；当前 ADR 不引入大小写不敏感名称，`Demo` 与 `demo` 仍是
不同名称，除非另一个命名 ADR 先改变 V1/V2 baseline。

同一 registry、owner 和 name 只能有一个 active 或 historical identity；
仓库本身没有 delete/recreate 操作，避免旧 alias 指向新的内容。`repoId` 一旦
写入不得改变，事件、扩展表和 importer 都以它作为主键。

### V1 导入 ID

V1 导入不能把 V1 的 `(owner,name)` 直接当作 V2 native ID，也不能依赖当前
owner 计算一个会随 transfer 改变的 ID。建议使用独立 V1 域：

```text
importedRepoId = keccak256(abi.encode(
  "igit:v1:repo",
  sourceChainId,
  v1Contract,
  canonicalOwner20,
  canonicalName
))
```

导入器必须把 `sourceChainId`、V1 contract、原始 owner/name、生成的 ID 和
快照 hash 写入可审计 mapping。若审计后决定由 importer 分配随机 ID，也必须
将完整 V1->V2 mapping 固化到快照，并保证重试得到同一结果；不能在每次重试时
重新生成 ID。V1 与 V2 域分离后，即使同一地址和名称存在，也不会与 native
V2 identity 静默碰撞。

### Storage 形状

实现阶段应把用户定位和内部身份拆开，等价于：

```text
repos[repoId]                  -> Repo { owner, name, metadata, ... }
activeLocator[hash(owner,name)] -> repoId
aliases[hash(owner,name)]       -> { repoId, ownerAtAlias, name }
pendingTransfers[repoId]       -> { newOwner, executeAfter, reservedLocator }
```

`activeLocator` 只保存当前 canonical owner/name。创建和转移都必须检查 active
locator、historical alias 和 pending reservation，防止 URL 重用。若实现使用
单一 `locators` mapping，也必须保留等价的 `isCanonical`/`isAlias` 语义，不能
让读取者从 `Repo.owner` 猜测 alias。

未来的 refs、roles、splits、sponsors、guardians、badges、reports 和 release
扩展均必须以 `repoId` 为 key。任何新扩展若仍以 owner/name 为 key，必须在合约
review 中说明迁移、转移和 alias 行为，否则不能进入 V2。

## Ownership transfer 语义

### API 和状态机

面向合约的写入口建议以 `repoId` 为主，避免调用者在 transfer 期间传入过时
owner：

```text
beginOwnershipTransfer(repoId, newOwner)
cancelOwnershipTransfer(repoId)
rejectOwnershipTransfer(repoId)
expireOwnershipTransfer(repoId)
acceptOwnership(repoId)
```

为了兼容现有 CLI/Web，可以保留 `(owner,name)` wrapper，但 wrapper 必须先解析
locator：canonical locator 可继续；historical alias 只允许读取，写入返回
`RepoMoved`。repoId-only 写入口无法知道调用者最初使用了哪个 URL；因此 CLI、
Web 和 remote helper 必须保留 `ResolveRepo` 返回的 `isCanonical/movedFrom`，在
签名之前拒绝来自 alias 的写入。显式使用 repoId 的高级调用属于 identity 写入，
不被解释为“通过旧 URL 写入”。pending transfer 状态机如下：

1. 当前 owner 发起；`newOwner != currentOwner`，repo 必须存在；
2. 发起时检查目标 `(newOwner,name)` 没有 active repo、属于其它 repoId 的
   historical alias 或其他 reservation，并立即保留该 locator，防止 timelock
   期间发生命名竞态；同一个 repoId 的 historical alias 可以保留读取能力并被
   reservation，accept 时重新提升为 canonical，因而 A -> B 后仍允许 B -> A，
   但 alias 永远不能改指另一个仓库；
3. `executeAfter = proposedAt + 7 days`，与 V1 对齐；pending transfer 与
   guardian recovery 互斥。`OWNERSHIP_TRANSFER_ACCEPTANCE_WINDOW = 30 days`，
   `expiresAt = executeAfter + 30 days`；
4. 只有指定 `newOwner` 可以在 timelock 后 accept；未成熟、非目标或不存在的
   pending 状态返回可区分的 custom error；超过 `expiresAt` 后 accept 返回
   `TransferExpired`；
5. 当前 owner 可以在 accept 前 cancel。cancel 必须释放目标 reservation，且
   不改变 repo、refs 或其它状态；指定 target 可以随时 reject，避免被动占用其
   namespace；超过 `expiresAt` 后任意 keeper 可以调用 `expireOwnershipTransfer`
   清理 reservation，避免永久占用；
6. accept 是 O(1) 状态更新：写入新 owner、移动 active locator、保留旧 locator
   alias、清除 pending state，并发出完整事件。任何一个检查失败都原子回滚。
   collaborator 使用 `indexPlusOne` 和 swap-pop，使移除“原本是 collaborator 的
   new owner”不需要线性扫描 refs 或其它 repo 状态；

建议的错误集合（名称可按项目 ABI 风格调整，但语义必须保留）：

```text
RepoNotFound(bytes32 repoId)
LocatorNotFound(address owner, string name)
RepoMoved(bytes32 repoId, address currentOwner, string name)
TransferAlreadyPending(bytes32 repoId)
TransferNotPending(bytes32 repoId)
TransferTooEarly(bytes32 repoId, uint64 executeAfter)
TransferExpired(bytes32 repoId, uint64 expiresAt)
TransferNotExpired(bytes32 repoId, uint64 expiresAt)
TransferUnauthorized(bytes32 repoId, address caller)
LocatorUnavailable(address owner, string name)
LocatorReservationMismatch(bytes32 locatorKey, bytes32 expectedSubject)
RecoveryPending(bytes32 repoId)
```

### 转移后的状态保留

下表是 V2 的规范行为，和 V1 的实际副作用有意区分：

| 状态 | V2 transfer 行为 |
| --- | --- |
| Repo metadata、moderation、frozen/delisted/active | 保留；transfer 不自动解冻，也不要求 source 为 Active |
| refs（SHA、URI、原始 timestamps/updater） | 按稳定 repoId 保留，不复制、不重写 |
| collaborators | 保留 role；若 new owner 之前是 collaborator，accept 时移除其 collaborator 记录 |
| revenue splits | 保留。V1 transfer 的删除是已知数据损失，不在 V2 重现 |
| sponsor totals | 保留并继续归属于同一 repoId |
| guardians/recovery proposal | 清除；新 owner 必须显式重设信任集合 |
| badges、moderation reports | 保留并按 repoId 归属；历史 award/report 时间和内容不改写 |
| forkedFrom/source snapshot | 保留；source identity 使用稳定 repoId，owner/name snapshot 仅供展示 |
| `createdAt`、ref timestamps | 保留原值 |
| `updatedAt` | transfer 不因兼容性自动推进；另用 transfer event/`transferredAt` 表达所有权变化 |

Frozen 或 Delisted 仓库允许 transfer，与 V1 不调用 ref 写入 guard 的实际行为
一致。transfer 不应绕过 owner-only、timelock、target collision 或 recovery
冲突检查。

### 事件

至少发出以下事件，字段必须能让 indexer 在没有再次猜测 URL 的情况下重建历史：

```text
OwnershipTransferStarted(
  bytes32 indexed repoId,
  address indexed oldOwner,
  address indexed newOwner,
  string name,
  uint64 proposedAt,
  uint64 executeAfter,
  uint64 expiresAt
)
OwnershipTransferCancelled(bytes32 indexed repoId, address indexed owner, address target)
OwnershipTransferRejected(bytes32 indexed repoId, address indexed owner, address indexed target)
OwnershipTransferExpired(bytes32 indexed repoId, address indexed owner, address indexed target)
OwnershipTransferred(
  bytes32 indexed repoId,
  address indexed oldOwner,
  address indexed newOwner,
  address acceptedBy,
  string name,
  uint64 executeAfter
)
```

事件中的 `repoId` 是唯一身份；不能只发 old/new URL，因为 URL 可能被客户端
缓存或别名解析改变。

## URL、别名和客户端行为

用户继续使用：

```text
igit://owner/repo
```

当前 owner/name 是 canonical URL。转移后的旧 URL 行为固定如下：

| 操作 | historical alias 行为 |
| --- | --- |
| `clone`、`fetch`、`pull`、repo/ref 查询 | 解析到同一 `repoId`，返回当前 owner/name 和 canonical URL |
| `push`、ref update/delete、metadata/collaborator 写入 | 拒绝并返回 `RepoMoved`，错误中带 canonical `igit://newOwner/repo` |
| Web 路由 | 显示 moved/redirect 状态，链接 canonical 页面；不自动提交交易 |
| `igit remote -v` / doctor | 显示用户 URL 和 moved 提示，不泄露 ABI/RPC 细节 |

alias 至少在整个迁移兼容窗口保留；推荐永久保留，因为 Git remote、issue、
badge 和外部文档可能长期引用旧 URL。若未来要回收 alias，必须单独制定版本化
兼容政策、离线重写工具和明确的 `AliasExpired` 错误，不能静默允许新仓库复用
旧 URL。

永久保留不等于禁止同一仓库转回历史 owner。同一 `repoId` 的 alias 可以在
transfer reservation 期间继续读取，并在 accept 时重新成为 canonical；此时
刚离开的 locator 变为 alias。跨 repoId 的 active locator、alias 或 reservation
仍然一律返回 `LocatorUnavailable`。

username 不是稳定身份。CLI/Web 先把 username 解析成地址，再执行 owner/name
locator 查询；username 变更、释放或退款不得改变任何 `repoId`。地址转换失败
必须 fail closed，不能映射为零地址或另一个用户名。

## Fork 语义

### 创建和 source 快照

合约内部建议使用：

```text
beginFork(sourceRepoId, targetName)
copyForkRefs(forkId, cursor, limit)
finalizeFork(forkId)
cancelFork(forkId)
```

CLI 的 `igit fork owner/repo [new-name]` 先解析 source URL（允许 historical
alias），然后传稳定 `sourceRepoId`。source 必须为 `Active`；Frozen 或 Delisted
均拒绝，保持 V1 的 moderation boundary。目标 owner 是交易 sender，target
locator 必须未被 active repo、alias 或 reservation 占用。

fork 创建新的 `repoId`，复制 source 的 description、default branch 和 source
ref 内容；新 repo 的 moderation 状态为 Active，`createdAt/updatedAt` 为 fork
完成时刻。每个复制 ref 保留 commit SHA 和 pack URI，但 `updatedAt` 为 fork
时刻、`updatedBy` 为新 owner。source 与 fork 完成后独立更新。

fork 记录两个来源字段：

```text
forkedFromRepoId       // 稳定、机器可追踪的来源身份
forkedFromOwnerSnapshot
forkedFromNameSnapshot
```

snapshot 仅用于 source 后续 transfer/rename 后的历史展示；查询必须优先以
`forkedFromRepoId` 解析当前 source，不能把旧字符串当成新的 repo identity。

下列状态不复制，和 V1 行为一致：collaborators、revenue splits、sponsor
totals、guardians/recovery、badges、moderation reports、pending transfer。
IPFS URI 是内容引用，不重新上传或重新 pin；客户端仍须验证 pack/CID。

### Gas、批处理和可见性

当前 V1 没有 ref 数量上限。当前 V2 已限制每个 ref 最多累计 128 个 URI、每个
URI 最多 512 bytes，也会在普通 push 超过最终累计上限前原子回滚；但 repo
级最坏 payload 仍可达到 1024 个 refs 和 `1024 * 128` 个 URI，不能在一笔交易
中复制。Fork 实现必须选择并审计以下一种存储策略：

1. 保留现有 per-ref 最终累计上限，再为每个 repo 增加可审计的聚合 payload
   上限，并在 append 前按最终存储量检查；或
2. 让 fork 引用不可变 source snapshot/revision，采用 copy-on-write，而不是
   复制所有 URI 字符串。引用模型必须证明 source 后续更新、删除和 transfer
   不会改变已完成 fork 的可重建历史。

无论选择哪种策略，都必须满足：

- Foundry gas benchmark 根据 ref 数、累计 URI 数、累计 URI bytes 和 storage
  writes 共同确定 atomic payload 上限；未有 benchmark 前不得把整个 1024 ref
  上限或单次 128 URI 上限当作单笔可行保证；
- source 的总 payload 不超过该复合上限时，`forkRepo` 可以单笔原子完成；超出
  时返回 `ForkRequiresBatch`，不能部分创建后报告成功；
- batch 路径必须在 `beginFork` 时记录 source revision/epoch，并锁定 source
  ref mutation 或以等价的 per-ref revision 保证 snapshot 不变；必须有过期
  cancel/cleanup，避免 pending fork 永久阻塞 source；
- pending target 的 repo、refs 和 URL 对普通查询不可见，只有 `finalizeFork`
  后一次性标记 visible；失败、cancel 或 timeout 先把 fork 标记为
  `Cancelling`，不能在一个调用中无界删除所有临时 refs；
- `cleanupFork(forkId, cursor, limit)` 必须按 ref/URI/bytes 复合上限分批清理，
  允许无权限 keeper 推进已经取消/超时的 cleanup，但不能改写内容。清理完成前
  target 继续不可见且 locator reservation 保留；最后一批删除 fork state 后才
  原子释放 reservation，避免确定性 target repoId 被新仓库复用并碰撞残留 refs；
- `copyForkRefs` 使用 cursor/limit，批次同时受 ref 数、URI 数和 URI bytes 硬
  上限约束，并在 gas 测试中覆盖空批、重复批、乱序 cursor、source revision
  变化、单个超大历史 ref 和最后一批；
- source 在 fork pending 期间若被 transfer，只要 repoId 不变，fork 仍指向同一
  source；source owner/name snapshot 保留，不能产生第二个 source repo。

建议事件：

```text
ForkStarted(
  bytes32 indexed sourceRepoId,
  bytes32 indexed targetRepoId,
  address indexed targetOwner,
  string sourceOwnerSnapshot,
  string sourceNameSnapshot,
  string targetName,
  uint256 sourceRevision,
  uint256 refCount
)
ForkFinalized(bytes32 indexed sourceRepoId, bytes32 indexed targetRepoId, uint256 refCount)
ForkCancelled(bytes32 indexed sourceRepoId, bytes32 indexed targetRepoId, address caller)
```

## 查询和 Chain API 契约

统一 backend 应把 locator 解析和稳定 ID 解析分成两个步骤：

```text
ResolveRepo(ownerOrUsername, name)
  -> { repoId, currentOwner, name, canonicalURL, movedFrom? }
GetRepo(repoId)
ListRefs(repoId, cursor, limit)
ResolveRef(repoId, refName)
ListCollaborators(repoId, cursor, limit)
```

现有 `(owner,name)` 查询 wrapper 可以继续存在，但实现必须先走
`activeLocator`/`aliases`，而不是重新计算 `keccak(owner,name)` 并直接访问
storage。所有列表查询采用 cursor/limit；refs 的 pack URI 也必须分页或由有界
snapshot descriptor 返回。不得为了兼容当前 Solidity 的完整数组返回而在
remote helper 中假设仓库一定小于 ref 上限或单次 URI 上限。

写操作优先使用 `repoId`：

```text
CreateRepo(name, metadata)
UpdateRef(repoId, refName, commitSha, packUris, expectedSha, force)
DeleteRef(repoId, refName)
UpdateRepoInfo(repoId, patch)
SetCollaborator(repoId, address, role)
```

若 CLI 只有 URL，backend 必须保留 locator resolution 结果，在构造/签名任何
repoId-only 写交易前拒绝 alias，并把 moved 状态转换为稳定的用户错误和
canonical URL。网络/RPC/receipt 错误、权限错误和 `RepoMoved` 不能触发
CosmWasm 写 fallback；V1 只允许明确的 legacy read 或迁移窗口写入。

## V1 快照导入规则

导入器必须在一个固定 LCD block height 导出 repositories、refs、collaborators、
metadata 和扩展，并对规范化 JSON 生成 snapshot hash。导入按 repoId 分批、可
重放、幂等；每批记录 tx hash、输入 hash、数量和比对结果。

导入前先建立 canonical identity 表：

```text
source (chain_id, v1_contract, owner20, name) -> imported repoId
```

最终状态快照只能证明导出高度上的 canonical `(owner,name)`，**不能单独推导**
仓库此前使用过哪些 locator。V1 repo state 没有 owner history；stale badge/report
owner 或 `forked_from` 字符串也可能重名、缺失或指向未纳入清单的仓库，不能
作为 transfer 历史的充分证据。历史 alias 只能来自固定区块范围内经核验的链上
transfer/recovery 事件、运营迁移台账，或与快照一起签名归档的显式 alias
manifest。manifest 至少包含 old owner/name、current owner/name、repoId、source
height/range 和证据 hash。

处理历史 V1 transfer 时：

- 以快照中最终 `(owner,name)` 的 repo 作为 canonical locator；
- 只有通过上述证据验证的旧 owner/name 才导入为 historical alias；只有最终
  快照而没有历史证据时，不生成猜测 alias，旧 URL 兼容性必须明确报告为未知；
- badge/report 的旧 owner 字段通过 repo identity 归并到同一 repoId，并保留原始
  字段用于审计；若无法唯一归并则进入 quarantine/fail-closed，不能按 stale
  locator 创建第二个 repo；
- 已转移 source 的 `forked_from` 字符串先尝试匹配 identity 表，歧义时 fail
  closed，不猜测；
- revenue split 若在 V1 transfer 中已丢失，只能报告为 snapshot 中的缺失，不能
  从 sponsor 或其它字段推导一个新 split；
- sponsor totals、refs、collaborator role 和 metadata 必须逐字段比对；
- 地址转换失败、重复 source key、同名 locator 冲突、缺失 ref/CID 或 hash 不
  一致都停止后续批次，不做双写补偿。

导入器不得逐个调用面向用户的 `createRepo` 来模拟迁移；必须使用受限的
`importRepo/importRefs` 批量入口，支持历史 metadata grandfather：原值可超过
普通 V2 新写入长度，但以后显式修改该字段仍须通过 V2 限制。

## 测试矩阵和发布门

### V1 对照测试

- transfer：7 天 timelock、accept、cancel、未授权、目标同名冲突、pending
  transfer/recovery 冲突、Frozen/Delisted transfer；
- transfer state：refs 保留、collaborator 行为、revenue split 实际丢失、
  sponsor totals 移动、guardian reset、badge/report stale association；
- fork：Active 成功、Frozen/Delisted 拒绝、metadata/ref timestamp/updater、
  extensions 不复制、name collision、响应 attributes、large-ref 边界；
- transfer 后 fork source：旧字符串与最终 owner/name 的解析结果。

### Foundry V2 测试

- repoId 在 transfer 前后不变，active locator 更新且 historical alias 可读；
- old URL read redirect、old URL write `RepoMoved`、alias 不跨 repoId 复用、
  same repoId alias 可重新成为 canonical；
- timelock、cancel、target reject、permissionless expire、accept、unauthorized、
  acceptance window、target reservation 和 recovery 冲突；
- refs/collaborators/splits/sponsor/guardian/badge/report 的保留或清除语义；
- Frozen/Delisted transfer、Active-only fork、fork source snapshot；
- atomic fork 可见性、batch cursor、revision lock、`Cancelling` 状态、分批
  cancel/timeout cleanup、cleanup 前 reservation 不释放、keeper 幂等重试；
- 事件 topics/data、custom error 解码、分页 cursor；
- fuzz/invariant：repoId 不变、locator 唯一、alias 不指向两个 repo、pending
  reservation 最终可释放、不可见 fork 不会出现在任何列表；
- gas/RPC report：最小/典型/最大 refs、累计 URI 数/bytes、单个历史大 ref、
  collaborators、每批 fork 上限和分页响应大小；重复普通 push 不得越过累计
  存储上限。

### Go、Web、remote helper 与 importer

- alias 解析、canonical URL 展示和 `RepoMoved` 分类；
- V2 写失败不回退 V1、不双写；receipt 失败和 custom error 保留；
- cursor/limit 不丢 ref，旧 wrapper 不依赖完整数组；
- Web moved route、fork source link、pending/finalized 状态；
- V1 transferred fixture、stale badge/report、fork-after-transfer、确定性
  repoId、带/不带 alias evidence 的最终快照、歧义 quarantine、snapshot hash、
  重复导入、批次重试/失败 fail-closed；
- clean Windows/Linux：无 WSL2、无 injectived、profile 自动选择、私钥不进入
  配置/日志、Kubo 独立生命周期。

任何一个身份/alias/gas/导入门未通过，都不能把 V2 标为 beta 或正式版。

## 发布顺序

1. **V2-alpha**：合约和 API 只在本地 EVM/testnet 评审；先落地稳定 ID、locator、
   transfer/fork Foundry 测试，禁止生产导入。
2. **V2-beta**：部署经审计的地址，启用 CLI/Web V2 读写、alias read redirect、
   有界 fork 和原生 Windows Kubo；V1 仅明确 legacy read。
3. **V2-rc**：固定 LCD 快照，执行一次性批量导入，完成数量/字段/CID/权限比对，
   在 Windows/Linux 清洁环境完成完整 Git 闭环和故障测试。
4. **V2 正式版**：新仓库永远写 V2，V1 合约只读；旧 alias 至少保留到所有活跃
   remote 都完成迁移，WSL2/injectived 仅作为 advanced legacy fallback。

### 当前 V2 alpha 的部署兼容

稳定 `repoId`、locator/alias 和 pending state 已进入当前 `RepoRegistryV2.sol`
源码和 checked-in ABI。当前实现包含 domain-separated native repoId、canonical/
historical locator、7 天 timelock、30 天 acceptance window、target reject、
permissionless expire、`RepoMoved` 和 O(1) ownership accept；Go remote helper 已在
pack/IPFS/sign 之前做 canonical preflight，Web 旧路由只读并链接 canonical route。

这些是源码和编译证据，不是部署结论。当前 alpha 没有审核后的 testnet 部署地址，
也没有可作为生产事实的数据，因此该变更采用**重新部署新的 V2 alpha 合约**，
不是对旧 alpha storage 做原地 upgrade，也不能用 storage layout cast/reinterpret
把 `keccak(owner,name)` mapping 变成稳定 ID mapping。

新部署后必须重新生成并校验 ABI/bytecode，network profile 只在审核、Foundry、
集成和安全门通过后更新 contract address。若某个环境已经存在需要保留的 alpha
测试数据，也必须先导出、按本 ADR 生成 identity mapping、再通过受限 importer
导入新合约；不能把未经验证的旧 storage slot 复制过去。旧 alpha address 在
profile 中标记 retired/read-only，不参与任何写 fallback 或双写。

## 未决但有边界的问题

- atomic fork 的复合 payload 上限必须由目标网络 gas limit、累计 URI 数/bytes
  分布和 Foundry 报告共同决定；在此之前不能承诺 1024 refs 或每 ref 128 URI
  可以单笔 fork。现有 per-ref 上限不能代替 repo 级 gas/payload 门禁。
- alias 是否永久存储会影响长期存储成本；默认永久保留，任何回收必须单独做
  版本化兼容 ADR。
- rename/merge/successor handle 不属于本 ADR；在它们设计完成前，V2 name
  视为 immutable，transfer 只改变 owner。
- V1 地址到 EVM 20-byte 地址的映射必须由网络 profile 和独立向量测试证明；
  失败时导入停止，不接受零地址或猜测映射。
- V1 历史 alias 数据源和覆盖区块范围必须在迁移 runbook 中列为独立交付物；
  当前最终状态 exporter 本身不包含 locator 历史。

## 相关规范

- [CosmWasm V1 协议基线](./cosmwasm-v1-protocol.md)
- [EVM V2 迁移规范](./evm-v2-migration.md)
- [系统架构](./architecture.md)
- [发布和验收门](./release.md)
- [V1 快照导出脚本](../scripts/v1-export-snapshot.sh)

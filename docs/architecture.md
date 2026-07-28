# Next Injective Git 架构设计

> 项目：**Next Injective Git**（CLI：`igit` / `git-remote-inj`，合约：`repo-registry`）。开放问题见 [open-questions.md](open-questions.md)。

## 1. 目标

让开发者用原生 Git 工作流（`push` / `clone` / `fetch`）把代码存储到去中心化基础设施上：

- **数据面**：Git packfile 存 IPFS（内容寻址、可验证）
- **控制面**：Injective 链上 CosmWasm 合约记录仓库元数据、refs 与权限
- **信任模型**：ref → commit SHA 的绑定由链上共识保证；packfile 内容由 CID 与 Git 自身的 SHA 链保证完整性

## 2. 数据流

### push

```
git push inj main
  └─ git 调起 git-remote-inj (remote helper 协议)
       1. list for-push        → 合约 list_refs 查询远端 tips
       2. push refs/heads/main:refs/heads/main
          a. git pack-objects --revs --thin --stdout
             （want = 本地 tip；exclude = 所有远端 tips，即增量 pack）
          b. POST Kubo /api/v0/add?pin=true   → CID
          c. injectived tx wasm execute {update_ref: {commit_sha, packfile_cids,
             expected_sha, force}}            → 链上确认
       3. 回报 ok/error 给 git
```

### clone / fetch

```
git clone inj://<owner>/<repo>
  └─ git-remote-inj
       1. list                 → 合约 list_refs + repo_info（HEAD symref）
       2. fetch <sha> <ref>    → 取该 ref 的 packfile_cids
          a. Kubo /api/v0/cat（失败则公共网关 GET /ipfs/<cid>）
          b. git index-pack --stdin --fix-thin  逐个注入对象库
       3. git 自行完成 checkout
```

## 3. 合约状态设计（contracts/repo-registry）

| 存储 | Key | Value |
|---|---|---|
| `REPOS` | `(owner, repo_name)` | `Repo { owner, name, description, default_branch, created_at, updated_at }` |
| `REFS` | `(owner, repo_name, ref_name)` | `RefEntry { commit_sha, packfile_cids[], updated_at, updated_by }` |
| `COLLABORATORS` | `(owner, repo_name, addr)` | `Role::Maintainer \| Role::Reader` |

关键决策：

- **`packfile_cids` 是有序追加列表**：普通 push 追加新 CID（增量历史），按顺序 `index-pack` 全部包即可重建完整历史。force push 替换整个列表（丢弃的历史由新 pack 全量覆盖）。
- **乐观并发（`expected_sha`）**：客户端声明它所认为的远端 tip；不匹配则拒绝，防止两个 maintainer 并发 push 互相覆盖。这是"链上 fast-forward 检查"的轻量替代——合约无法遍历 Git DAG 验证祖先关系，由客户端 Git 自身完成 FF 校验，链上只做防竞争。
- **权限收敛在合约**：只有 owner/maintainer 的 `update_ref` 交易能通过，签名即身份，无需额外账号体系。

## 4. CLI 设计（cli/）

| 模块 | 职责 |
|---|---|
| `cmd/git-remote-inj` | remote helper 入口（git 自动调用） |
| `cmd/igit` | 管理命令：init / repos / refs / key / config |
| `internal/remote` | helper 协议状态机（capabilities/list/fetch/push） |
| `internal/gitio` | 委托本地 `git` 做 pack-objects / index-pack |
| `internal/ipfs` | Kubo HTTP RPC（add/cat）+ 网关回退 |
| `internal/chain` | LCD smart query + injectived 交易签名广播 |
| `internal/config` | `~/.igit/config.json` |

依赖策略（MVP）：**零第三方 Go 依赖**。packfile 生成/注入委托本地 `git`（保证与所有仓库布局字节级兼容），签名委托 `injectived` keyring（私钥不经过 igit 进程）。后续可替换为 go-git + injective sdk-go 实现纯库内嵌。

## 5. 增量与大仓库策略

- 每次 push 只打包 `本地 tip − 所有远端 tips` 的增量对象，pack 体积 ≈ 本次变更
- clone 需下载该 ref 的全部历史 CID；CID 列表过长时的合并压缩（repack 成单一 pack 再 force 更新）留待里程碑 4 之后
- 浅克隆（`--depth`）暂不支持：helper 收到 git 的 depth 请求时按全量处理

## 6. 安全模型与已知限制

- 链上不验证 commit_sha 与 packfile 内容的一致性（成本过高）；恶意 maintainer 可写入不含对应对象的 CID，客户端 `index-pack` 会失败并报错——破坏可用性但不破坏完整性
- IPFS 数据持久性依赖 pin（见 open-questions）
- 仓库全公开；私有仓库需客户端加密（见 open-questions）

# Next Injective Git（`igit`）

**Next Injective Git** 是去中心化代码存储平台：Git 对象存 IPFS，仓库元数据与 refs 记录在 Injective 链上的 CosmWasm 合约中。配套 CLI **`igit`** + Git remote helper **`git-remote-igit`**，用 `igit push` / `igit clone`（或原生 `git`）即可上链协作。

```
igit init my-repo "hello chain"
igit push inj main
igit clone igit://alice/my-repo
```

## 架构一览

```
┌──────────┐  packfile   ┌───────┐   CID    ┌──────────────────────┐
│ git push │ ──────────► │ IPFS  │ ───────► │ Injective (CosmWasm)  │
│ (本地Git) │             │ Kubo  │          │ repo-registry 合约     │
└──────────┘             └───────┘          │  refs → sha + CIDs    │
      ▲                      ▲              └──────────────────────┘
      │      packfile        │      查询 refs / CIDs      │
      └──────────────────────┴──────────────◄─────────────┘
                       git clone / fetch
```

## 目录结构

| 路径 | 说明 |
|---|---|
| `contracts/repo-registry/` | CosmWasm 合约（Rust）：仓库注册、refs、协作者权限 |
| `cli/` | Go CLI：`git-remote-igit` 远程助手 + `igit` 管理命令 |
| `docs/` | 架构文档与开放问题 |

## 快速开始

### 依赖

- **Rust 1.81.0** + `wasm32-unknown-unknown` target（编译合约；链上 VM 不支持 bulk-memory / reference-types，**1.82+ 编出的 wasm 会被链拒收**，Cargo.lock 已钉住 1.81 兼容的依赖版本）
- Go 1.22+（编译 CLI）
- Git（CLI 依赖 `git pack-objects` / `git index-pack`）
- [Kubo](https://docs.ipfs.tech/install/command-line/)（仅 Push 时的本地临时上传组件；Clone/Fetch 不需要 Kubo）
- [injectived](https://docs.injective.network/)（签名与广播交易；无 Windows 版，Windows 用户请在 WSL2 中运行全套工具链）

### 1. 编译并部署合约（Injective testnet）

```bash
cd contracts/repo-registry
cargo test                                      # cw-multi-test 单元测试
cargo build --release --target wasm32-unknown-unknown
injectived tx wasm store target/wasm32-unknown-unknown/release/repo_registry.wasm \
  --from mykey --chain-id injective-888 \
  --node https://testnet.sentry.tm.injective.network:443 \
  --gas auto --gas-adjustment 1.4 --gas-prices 500000000inj --yes
# testnet 阶段保留单签 admin 以便快速升级（主网切多签 + 时间锁，见 docs/open-questions.md §7）
ADMIN=$(injectived keys show mykey -a)
injectived tx wasm instantiate <CODE_ID> '{"admin":"'$ADMIN'"}' \
  --label igit-repo-registry --admin $ADMIN --from mykey ... --yes
```

也可直接用 `scripts/testnet-deploy.sh <store交易hash>` 完成 instantiate + igit 配置；`scripts/testnet-e2e.sh` 可在真实 testnet + IPFS 上回归全部 push/clone 场景。

### 2. 编译并安装 CLI

```bash
cd cli
go build ./...
# igit + the remote helper must be on PATH. git resolves igit:// URLs by
# running git-remote-igit, so both binaries need to be installed.
go install ./cmd/igit ./cmd/git-remote-igit
```

### 3. 配置并使用

```bash
igit config set contract_address inj1...       # 部署得到的合约地址
igit key new dev                               # 或 igit config set key_name <已有key>
igit init hello "my first on-chain repo"

cd my-project
git remote add inj igit://$(injectived keys show dev -a)/hello
igit push inj main                             # packfile→IPFS, ref→链上
igit clone igit://alice/hello                  # 任何人可克隆（用户名或裸地址）
```

> `igit push` / `igit clone` / `igit pull` 是对 `git` 的轻包装——你只用 `igit` 一个命令即可；
> 原生 `git push` / `git clone igit://...` 同样有效（走 `git-remote-igit` 助手）。

从 GitHub 一键迁移现有仓库（全部分支 + tag 镜像上链）：

```bash
igit import github.com/user/repo          # 镜像到 igit://<你的地址>/repo
igit import github.com/user/repo my-name  # 自定义链上仓库名
```

## 工作原理

- **push**：`git-remote-igit` 生成增量 packfile → 本地 Kubo `add?pin=false` 临时加入并通过 Swarm 提供给 US → 受控复制服务确认 US Kubo 全量 Pin 且校验 pack SHA-256 → `injectived` 签名广播 `update_ref`。只有交易成功后才执行本地 IPFS GC；交易失败会保留临时块以便重试，US 未上链 Pin 按 TTL 回收。
- **clone/fetch**：LCD 查询合约 `list_refs` / `resolve_ref` → 先探测 HK/US `/healthz` 并按延迟排序 → 仅通过 HTTPS `GET /ipfs/<cid>` 下载（失败再回退公共网关）→ `git index-pack` 注入本地对象库；本地没有 Kubo 也能完成。
- **权限**：owner 可管理协作者（Maintainer 可推送、Reader 只读标记）、转移所有权；内容委员会（未设时为 admin）可设置 `moderation_status`，Frozen 状态下合约拒绝一切 ref 写入。

详见 [docs/architecture.md](docs/architecture.md)、[目标拓扑与迁移方案](docs/target-topology-migration.md)；开放问题见 [docs/open-questions.md](docs/open-questions.md)。

## 开发

```bash
# 合约
cd contracts/repo-registry && cargo test

# CLI
cd cli && go test ./... && go vet ./...
```

## IPFS 网关与临时上传

CLI 默认健康探测香港 `https://igit-hk.haohanyh.ovh` 与美国 `https://igit-us.haohanyh.ovh`。读取不启动、不依赖本地 Kubo，按 `/healthz` 延迟排序后使用只读 HTTPS 网关，并继续回退公共 IPFS 网关。

```bash
igit gateway status                 # 查看两地健康和延迟
igit gateway select                 # 查看当前自动选路顺序

# Push-only configuration: the API is a short-lived scoped authorization,
# never a remote Kubo credential. The US peer is a libp2p multiaddr.
igit config set upload.endpoint https://igit-us.haohanyh.ovh/v1/replications
igit config set upload.authorization '<identity-token>'
igit config set upload.us_peer '<US-Kubo-p2p-multiaddr>'
```

Kubo RPC `:5001` 始终只绑定本机 loopback，普通用户不能通过 SSH tunnel 控制 HK/US Kubo。Push 会以 identity token 换取 CID/ref/pack-SHA-256 绑定的一次性短期 ticket；没有本地 Kubo 的用户在 Push 时会得到明确安装提示，Clone/Fetch 无此依赖。

## License

Apache-2.0

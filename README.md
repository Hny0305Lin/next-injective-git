# Next Injective Git（`igit`）

**Next Injective Git** 是去中心化代码存储平台：Git 对象存 IPFS，仓库元数据与 refs 记录在 Injective 链上的 CosmWasm 合约中。配套 CLI **`igit`** + Git remote helper **`git-remote-inj`**，开发者用原生 `git push` / `git clone` 即可上链协作。

```
igit init my-repo "hello chain"
git clone inj://inj1abc.../my-repo
git push inj main
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
| `cli/` | Go CLI：`git-remote-inj` 远程助手 + `igit` 管理命令 |
| `docs/` | 架构文档与开放问题 |

## 快速开始

### 依赖

- Rust + `wasm32-unknown-unknown` target（编译合约）
- Go 1.22+（编译 CLI）
- Git（CLI 依赖 `git pack-objects` / `git index-pack`）
- [Kubo](https://docs.ipfs.tech/install/command-line/)（本地 IPFS 节点，`ipfs daemon`）
- [injectived](https://docs.injective.network/)（签名与广播交易）

### 1. 编译并部署合约（Injective testnet）

```bash
cd contracts/repo-registry
cargo test                                      # cw-multi-test 单元测试
cargo build --release --target wasm32-unknown-unknown
injectived tx wasm store target/wasm32-unknown-unknown/release/repo_registry.wasm \
  --from mykey --chain-id injective-888 \
  --node https://testnet.sentry.tm.injective.network:443 \
  --gas auto --gas-adjustment 1.4 --gas-prices 500000000inj --yes
injectived tx wasm instantiate <CODE_ID> '{}' \
  --label igit-repo-registry --no-admin --from mykey ... --yes
```

### 2. 编译并安装 CLI

```bash
cd cli
go build ./...
go install ./cmd/igit ./cmd/git-remote-inj    # 两个二进制都需在 PATH 中
```

### 3. 配置并使用

```bash
igit config set contract_address inj1...       # 部署得到的合约地址
igit key new dev                               # 或 igit config set key_name <已有key>
igit init hello "my first on-chain repo"

cd my-project
git remote add inj inj://$(injectived keys show dev -a)/hello
git push inj main                              # packfile→IPFS, ref→链上
git clone inj://inj1.../hello                  # 任何人可克隆
```

## 工作原理

- **push**：`git-remote-inj` 调 `git pack-objects` 生成增量 packfile → 上传本地 Kubo 并 pin（得到 CID）→ 通过 `injectived` 签名广播 `update_ref` 交易（携带 commit SHA + CID）。合约校验推送者为 owner 或 maintainer，并用 `expected_sha` 做乐观并发检查（force push 跳过）。
- **clone/fetch**：查询合约 `list_refs` / `resolve_ref` → 按 CID 从 IPFS（本地节点，失败则公共网关）下载 packfile → `git index-pack` 注入本地对象库。
- **权限**：owner 可管理协作者（Maintainer 可推送、Reader 只读标记）、转移所有权。

详见 [docs/architecture.md](docs/architecture.md)；开放问题见 [docs/open-questions.md](docs/open-questions.md)。

## 开发

```bash
# 合约
cd contracts/repo-registry && cargo test

# CLI
cd cli && go test ./... && go vet ./...
```

## License

Apache-2.0

# iGit EVM V2 项目信息库

> 本文档面向后续开发、审查和运维，记录代码和证据中已经确认的事实。路线图、示例配置和测试 fixture 不得被当成产品能力或上线证据。

| 项目 | 当前值 |
|---|---|
| 文档快照 | 2026-09-11（LiveAgent 源码复核；非链上实时复验） |
| 当前分支 | `dev` |
| 当前源码 HEAD | `0ba06f436558f12d97625b393767440cdd0f9862` |
| 产品代号 | iGit EVM V2 |
| 链上 Suite 协议版本 | `3` |
| 当前 pack 数据面 | IPFS/Kubo |
| 当前测试网 EVM chain ID | `1439` |
| 公开配置状态 | `SuiteDirectory` 仍为空，未完成公开切换 |

## 1. 一页结论

Next Injective Git（命令名 `igit`）把 Git 控制面放在 Injective EVM，把 Git packfile 放在当前 IPFS/Kubo 数据面：

```text
Git / igit CLI / Web
        |
        v
  SuiteDirectory  <--- 唯一链上信任根
        |
        +--> RepositoryCore
        +--> RecoveryModule
        +--> ModerationModule
        +--> EconomicModule
        +--> UsernameModule
        +--> BadgeModule
        +--> ReleaseModule

Git packfile ---> IPFS/Kubo ---> 受控复制与 Pin 服务 ---> HTTPS 网关读取
```

必须保持的理解：

1. **EVM V2 是产品代际名称，Suite v3 是链上协议版本。** 两者不是同一个版本号。
2. 普通 CLI、Git remote helper 和 Web EVM 路径只信任一个配置的 `SuiteDirectory`。模块地址必须从 Directory 发现，并在同一个区块完成代码哈希和绑定校验。
3. 当前 Suite 不可升级：没有 proxy、diamond、`delegatecall`、升级入口或旧 CosmWasm 写回退。
4. 测试网已经有九个合约的部署证据和 `fresh-empty-suite` 激活证据，但**公开产品切换仍被 P1.4-P1.6 阻塞**。
5. 当前 Suite 从空状态启动；CosmWasm V1 只保留显式的只读归档预览，不导入、不参与普通 EVM 读写，也不是失败时的 fallback。
6. 当前 pack URI 只支持 `ipfs://`。S3/R2 是后续 successor protocol 方向，不是当前实现。
7. 普通 EVM 运行路径不要求 WSL2 或 `injectived`。当前 Push 仍需要本机 Kubo；Clone/Fetch 通过 HTTPS 网关读取，不需要本机 Kubo。

## 2. 事实等级与术语

当文档与代码看起来冲突时，按以下优先级判断：

1. 当前源码、锁定 ABI、自动化测试和实际命令输出。
2. `evidence/` 下与精确源码提交绑定的链上证据。
3. 已接受的 ADR 和架构文档。
4. Roadmap、Backlog、旧 repair plan 和示例文件。

当前源码 HEAD 是完整的 `0ba06f436558f12d97625b393767440cdd0f9862`，而现有测试网部署/激活证据绑定的源码提交是 `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`。后者是当前 HEAD 的祖先，但不是同一提交；正式切换门禁要求证据、批准和 `expected_commit` 精确一致。不要把短 SHA 或当前源码检查当作历史部署证据的替代。代码中的 EVM runtime `codehash` 是 runtime bytecode 的 Keccak 哈希；`source_sha256`、bytecode 文件 SHA-256 和应用层 release digest 是不同字段，不能互称。`VerifySuite` 也只证明某个固定区块上的 Directory/module 内部一致性，不证明源码、审计、finality 或当前 HEAD 已验证。 

| 术语 | 含义 |
|---|---|
| iGit | 项目名称；CLI 为 `igit`，Git 协议 helper 为 `git-remote-igit` |
| EVM V2 | 从 CosmWasm V1 迁移到 Injective EVM 的第二代产品控制面 |
| Suite v3 | Solidity Suite 的链上协议版本，`SuiteDirectory.suiteVersion()` 返回 `3` |
| SuiteDirectory | Suite 唯一信任根；记录状态、chain ID、snapshot root、Coordinator、模块地址和 runtime code hash |
| BootstrapCoordinator | 按固定顺序验证/导入模块并最终激活 Directory 的协调器 |
| `repoId` | `RepositoryCore` 分配的稳定 `bytes32` 仓库身份 |
| locator | 当前 owner + repository name；旧 locator 可作为不可变 alias |
| control plane | 链上仓库身份、refs、权限、恢复、治理、赞助、用户名、徽章和发布校验和 |
| data plane | Git packfile 的存储和复制；当前实现是 IPFS/Kubo |
| `fresh-empty-suite` | Suite 激活时七个业务模块均为零导入记录的切换模式 |
| V1 archive preview | 独立的 CosmWasm V1 固定高度只读查询表面 |
| durable Pin | 受控复制服务确认 pack 已在持久节点固定 |

## 3. 仓库代码地图

| 路径 | 责任 | 接续注意事项 |
|---|---|---|
| [`contracts/evm-v2/src`](../contracts/evm-v2/src) | 九个不可升级 Solidity 合约和 Suite 接口 | Solidity 固定 `0.8.24`，optimizer runs=1，viaIR=true |
| [`contracts/evm-v2/test/SuiteArchitecture.t.sol`](../contracts/evm-v2/test/SuiteArchitecture.t.sol) | 架构、权限、导入校验、gas 和 invariant 测试 | 测试合约不是生产合约 |
| [`contracts/evm-v2/abi`](../contracts/evm-v2/abi) | Web/工具使用的 checked-in ABI | ABI 变更要和 Solidity 检查一起更新 |
| [`contracts/evm-v2/artifacts`](../contracts/evm-v2/artifacts) | 部署和 artifact 校验输入 | `igit-deploy-suite --check` 会检查 |
| [`cli/cmd/igit`](../cli/cmd/igit) | 用户 CLI：仓库、权限、经济、用户名、Suite、诊断和配置 | 未知子命令会转交 Git，修改 dispatch 前先读 `main.go` |
| [`cli/cmd/git-remote-igit`](../cli/cmd/git-remote-igit) | Git remote-helper 入口 | 协议实现位于 `cli/internal/remote` |
| [`cli/internal/chain`](../cli/internal/chain) | EVM RPC、ABI、Directory 验证、registry、transactor、keystore | 普通运行路径只能通过 `EVMSuiteRegistry` 使用模块 |
| [`cli/internal/remote`](../cli/internal/remote) | `list`、`fetch`、`push` remote-helper 协议和 pack 流程 | 链上 ref 更新必须晚于 durable Pin |
| [`cli/internal/ipfs`](../cli/internal/ipfs) | 本地 Kubo Add/GC、HTTPS 网关读取和健康排序 | Gateway 是只读下载面 |
| [`cli/internal/replication`](../cli/internal/replication) | CID/仓库/ref/pack hash 绑定的复制授权和 Pin 确认 | token 不能发链上交易 |
| [`cli/internal/suitemigration`](../cli/internal/suitemigration) | 从 snapshot 生成确定性 bootstrap plan/calldata manifest | 当前空状态切换不执行 V1 导入 |
| [`cli/cmd/igit-suite-operator`](../cli/cmd/igit-suite-operator) | 加密 keystore、manifest、广播、receipt 和 append-only journal | 保留供未来经批准的迁移 |
| [`cli/cmd/igit-deploy-suite`](../cli/cmd/igit-deploy-suite) | 九合约部署和 no-clobber deployment evidence | Broadcast 是显式运维操作 |
| [`web/src/lib`](../web/src/lib) | viem transport、Suite verification、registry、wallet、IPFS 和 V1 adapter | 写交易要显式 legacy、gas、gasPrice 并验 receipt |
| [`web/src/pages`](../web/src/pages) | Dashboard、Owner、Repo、Explorer、Monitor、IPFS、Archive、Settings | `/archive/cosmwasm-v1` 与 EVM 路径分离 |
| [`scripts`](../scripts) | 编译、源码边界、部署/切换证据、复制和运行门禁 | required gate 缺工具必须失败 |
| [`archive/cosmwasm-v1`](../archive/cosmwasm-v1) | 隔离的历史源码、协议材料和证据工具 | 不导入普通 runtime、CI 或 release |
| [`evidence`](../evidence) | 测试网部署和空状态激活证据 | 证据不可覆盖，fixture 不是 live evidence |

## 4. 链上 Suite 架构

### 4.1 Directory 信任链

`SuiteDirectory` 构造时绑定部署链的 `block.chainid`、非零 `snapshotRoot` 和一次性的 `bootstrapAuthority`。authority 设置 Coordinator 后，Directory 检查 Coordinator 的 runtime code hash、`suiteDirectory()` 和 `snapshotRoot()`。Coordinator 再按固定顺序注册七个模块；每个模块必须满足：

- runtime code hash 等于登记值；
- `suiteDirectory()` 指向同一个 Directory；
- `bootstrapCoordinator()` 指向同一个 Coordinator；
- `moduleId()` 等于预期模块 ID。

只有 Coordinator `readyForActivation()` 后 Directory 才能进入 `Active`，激活后配置冻结。客户端在一个 block tag 上读取并验证 chain ID、suite version、active state、Coordinator、七个模块地址、七个 runtime code hash 和模块反向绑定。

### 4.2 生产合约职责

| 部署顺序 | 合约 | 核心状态/能力 | 关键约束 |
|---:|---|---|---|
| 1 | `SuiteDirectory` | Suite 版本、状态、链 ID、snapshot root、模块绑定 | 一次配置、一次激活；无升级入口 |
| 2 | `BootstrapCoordinator` | 模块注册、批次导入、rolling root、激活 | 固定顺序；sequence/count/root 必须匹配 |
| 3 | `RepositoryCore` | `repoId`、locator/alias、metadata、refs、collaborator、ownership、fork | ref/经济/fork 等操作经过 Moderation policy；恢复只接受 Recovery capability |
| 4 | `RecoveryModule` | guardian 集合、阈值、延迟恢复 proposal | 与普通 ownership transfer 分离 |
| 5 | `ModerationModule` | active/delisted/frozen、report、appeal、decision trail | Core ref、Economic、fork、badge 使用强制 policy hook |
| 6 | `EconomicModule` | native INJ sponsor、收入分成、平台费、treasury、历史 denom totals | 最多 20 个 recipient；防重入；owner 不能给自己分成 |
| 7 | `UsernameModule` | username 正向/反向解析、保留名、原始 owner claim | V2 注册不锁 V1 escrow；原始 claim 有 90 天窗口 |
| 8 | `BadgeModule` | 不可转移贡献徽章和按 recipient/repository 索引 | owner 不能接收自己的 badge |
| 9 | `ReleaseModule` | `(version, platform) -> sha256` 不可变 artifact 记录 | release authority 写入；重复注册拒绝 |

跨层约束包括：RepositoryCore 每 ref 最多 128 个 pack URI、每仓最多 1024 refs、最多 256 collaborators、ownership transfer 延迟 7 天且有 30 天窗口；Economic 最多 20 个 split recipients、平台费上限 500 bps；分页上限通常为 64。修改时要同步 CLI/Web、ABI 和迁移协议。

### 4.3 Bootstrap 生命周期

```text
Directory
  -> BootstrapCoordinator
  -> RepositoryCore
  -> RecoveryModule
  -> ModerationModule
  -> EconomicModule
  -> UsernameModule
  -> BadgeModule
  -> ReleaseModule
```

每个模块的导入流程：

1. `beginBootstrap(expectedCount, expectedBatches, expectedRoot, snapshotRoot)`。
2. 按 sequence 发送有界 `importBatch(sequence, count, payloadHash, payload)`。
3. 合约折叠 payload hash 到 rolling commitment，并累计 count/batch。
4. `finalizeBootstrap` 要求数量、批次、root 全部相等。
5. 七个模块 finalize 后，Username 还需要 username escrow evidence；Coordinator 才能激活 Directory。

当前测试网的 `fresh-empty-suite` 使七个模块的 expected/imported count 都为 0，expected root 与 rolling root 匹配，并证明没有 V1 username escrow liability。

## 5. CLI、remote helper 与数据面

### 5.1 普通 CLI

`cli/cmd/igit/main.go` 的命令面分为：

| 类别 | 命令示例 | 作用 |
|---|---|---|
| Git/仓库 | `igit init`、`clone`、`import`、`repos`、`refs` | 创建、查看和导入仓库 |
| 远程同步 | `igit push`、`igit pull` | 转交给 Git，再由 remote helper 完成同步 |
| 权限/身份 | `collab`、`transfer`、`guardians`、`username` | collaborator、ownership、guardian recovery、用户名 |
| 内容/经济 | `repo edit`、`fork`、`mod`、`badge`、`sponsor`、`splits` | Suite 业务模块操作 |
| 发布 | `release register`、`release verify` | 注册和验证 release sha256 |
| Suite/诊断 | `suite info`、`suite verify`、`doctor`、`gateway` | 绑定验证、环境诊断、网关选择 |
| 密钥/配置 | `key new/import/show`、`config list/set`、`setup` | 加密 keystore 和本机设置 |
| 归档 | `archive query/inventory/verify` | 只读 V1 证据工具 |

配置默认位于 `~/.igit/config.json`。EVM key 使用 go-ethereum encrypted keystore；私钥输入不回显，正常 CLI 输出不打印 plaintext key。

### 5.2 EVM 读写路径

```text
igit command
  -> EVMSuiteRegistry
  -> ensureVerified / VerifySuite
  -> resolve module from verified Directory
  -> checked-in ABI encode/decode
  -> EVMTransactor (write only)
  -> JSON-RPC / receipt
```

`EVMSuiteRegistry` 是 lazy 构造，但第一次 read/write 前必须完成 Suite verification；模块地址不能从配置文件直接注入。ABI 必须来自 `cli/internal/chain/abi`，不要手写 selector、tuple word decoder 或事件 topic。

Go `EVMTransactor` 固定执行：查询 pending nonce；`eth_estimateGas`；legacy type-0 签名；gas price 至少 `160000000 wei`；广播后有界轮询 receipt；不确定时返回包含 tx hash 的 typed error 并使 nonce cache 失效；status `0x0` 视为明确失败。

### 5.3 Git remote-helper Push

远程 URL 是 `igit://<owner>/<repo>`。当前 `cli/internal/remote/url.go` 只接受 `inj1...` bech32 owner 或已注册 username；虽然链层、钱包和合约使用 `0x...` EVM 地址，但标准 Git remote URL 当前不直接接受 `0x` owner。Push 流程：

1. `list for-push` 解析 Suite 和仓库；旧 locator 只能读 canonical locator，写入旧 locator 返回 moved-repository error。
2. 对每个 ref 解析本地 SHA，非 force push 以远端 refs 为 exclude，调用 Git 生成 pack。
3. pack 为空且目标 ref 已存在时复用其 URI；新 ref/tag 则生成自包含 full pack，避免其它 ref 删除后不可取。
4. 非删除 push 将 pack 临时写入本地 Kubo，得到 CID，形成 `ipfs://CID`。
5. 连接 US durable peer，申请绑定 `CID + owner + repo + ref + pack sha256 + size + expiry` 的复制授权。
6. replication 服务确认 US Kubo durable Pin 后，才调用 `RepositoryCore.updateRef`。
7. 链上交易失败时保留临时内容供重试；成功后再做本地 Kubo GC。force push 不使用 exclude，因此发布自包含 pack。

删除 ref 直接调用链上 `deleteRef`。Fetch/Clone 读取链上 ref 的 pack URI，经健康排序的 HTTPS gateway 下载，再调用 Git `index-pack`；不依赖本机 Kubo。

### 5.4 Pack URI 边界

当前 Suite 和 helper 只接受 `ipfs://`（为历史 bare CID 保留受限读取）。其它 scheme 必须 fail closed。S3/R2 不能通过把 HTTP endpoint 写入 ref 来实现；需要 successor Suite、URI 协议、上传授权、完整性校验和迁移证据。

## 6. Web 应用

Web 是 React/Vite + viem，入口为 [`web/src/App.tsx`](../web/src/App.tsx)。路由包括 Dashboard、Owner、Repo、Explorer、Monitor、IPFS、Settings 和 `/archive/cosmwasm-v1`。

### 6.1 配置与 fail-closed

`web/src/lib/profile.ts` 当前只发布 `injective-testnet`：EVM chain ID `1439`、Injective EVM RPC、IPFS HK gateway，`suiteDirectory` 默认为空。公网 Settings 页面将 Directory 设为只读并禁用写操作；代码层的 localStorage override 和 local deployment fallback 仍应按本地开发配置理解，不能把它们当成公开测试网地址或正式 profile。代码内 local deployment fallback 只用于本地环境，不能当测试网地址发布。

`verifySuite` 在一个 block tag 上校验链、版本、Active、Coordinator、全部模块、代码哈希和反向绑定；成功缓存约 30 秒。普通 EVM verification 失败时，页面应停止链上操作，不能转入 V1。

### 6.2 Web 钱包写入

`web/src/lib/transport.ts` 的 `writeModule` 会确保钱包在 Injective EVM chain，使用 checked-in viem ABI，读取 `eth_gasPrice` 并保证不低于 `160000000 wei`，发送显式 `type: "legacy"`，使用估算 gas 的约 1.4 倍加 headroom。receipt 逻辑最多轮询 120 次、间隔 1 秒；底层 provider 请求没有严格的墙钟 timeout，因此不要把它描述成精确的 120 秒保证。status `0x0` 是 revert；RPC 不确定返回带 tx hash 的 `EVMReceiptUnconfirmedError`。成功后清理 Suite cache。

Web 通过 `web/src/lib/gitstore.ts` 从配置的单一 IPFS gateway 下载 ref pack，用 isomorphic-git/LightningFS 建立浏览器内只读仓库视图；它不具备 CLI 的多 gateway 排序/fallback。`web/api/upload-authorization.mjs` 只签发短期 identity token，复制授权仍由 CLI/replication 服务完成。钱包发现使用 EIP-6963 和 provider identity pinning；同品牌多 provider 时，silent restore 不应任意选择。

### 6.3 V1 归档预览

Web V1 adapter 使用 GET-only smart query、会话级 snapshot height 和 LCD failover。它不签名、不广播 CosmWasm execute，也不作为普通仓库页的 fallback。相关测试必须继续保持固定高度、只读、无写方法。

## 7. 仓库身份与业务语义

`RepositoryCore` 把稳定身份和可变位置分离：

- 创建时分配 `bytes32 repoId`；
- owner/name 是当前 canonical locator；
- 旧 locator 保留为不可变 alias；
- 通过旧 locator 读会返回 canonical locator；
- 通过旧 locator 写会失败，防止 split history；
- refs、collaborators、moderation、economic、badges、recovery 和 fork lineage 都按 `repoId` 关联；
- username 只用于展示/解析，授权最终以 EVM address 为准。

Ownership 有两个独立状态机：正常 ownership transfer（proposal、7 天后 accept、30 天窗口、cancel/reject/expire）和 RecoveryModule 的 guardian threshold/延迟恢复。两者不能互相静默替换；迁移 snapshot 不应带入 pending transfer/recovery。

## 8. 网络、地址和配置

| profile | Cosmos chain ID（归档工具） | EVM chain ID | EVM RPC | 当前 Directory |
|---|---|---:|---|---|
| `injective-testnet` | `injective-888` | `1439` | `https://k8s.testnet.json-rpc.injective.network` | 空 |
| `injective-mainnet` | `injective-1` | `1776` | `https://sentry.evm-rpc.injective.network/` | 空 |

CLI 的 legacy Cosmos 字段仅为配置兼容和 archive 工具保留；普通 EVM runtime 不应依赖 `injectived`、Cosmos keyring 或 legacy contract address。

Injective 账户有 EVM `0x...` 和 Cosmos `inj1...` 两种一对一表示：EVM RPC/钱包/合约使用 `0x`，Cosmos archive/LCD 和部分输入使用 `inj1`。用户输入应先规范化，权限、去重和限流使用规范化地址；Directory/module 配置只接受 EVM 20-byte address。

## 9. 必须保持的安全不变量

1. 普通运行时只从 SuiteDirectory 发现模块；不新增第二个信任根、旧写 fallback、proxy 或 delegatecall。
2. chain ID、Active、suite version、module address、runtime code hash 和反向 binding 必须在同一 block tag 验证。
3. 写交易必须显式 legacy type、估算 gas、最低 gas price、bounded receipt wait；不确定时保留 hash，不盲目重发。
4. Push 必须在 ref 变更前获得 durable Pin；失败时保留可重试内容。
5. Core ref、Economic sponsor、fork、badge 的 Moderation policy 由合约 hook 强制，不能只靠客户端。
6. Core ownership recovery 只接受 Directory 绑定的 RecoveryModule capability。
7. 导入的 sequence、count、batch count、payload hash、rolling root、finalize 和 snapshot root 必须全匹配；证据文件 no-clobber。
8. fresh-empty 切换必须有 username escrow liability 为 none 的证据。
9. V1 archive 不能签名、广播或成为 EVM verification 失败 fallback。
10. 私钥只进加密 keystore；replication token 只绑定有限的 CID/仓库/ref/hash/时间窗。
11. fixture、source-only CI、测试输出或本地探测不能代替 live receipt、finality、安全批准和 checksum manifest。

## 10. 测试网证据事实

主要证据目录：[`evidence/testnet-deployment-2026-08-25`](../evidence/testnet-deployment-2026-08-25)。

| 项目 | 值 |
|---|---|
| `SuiteDirectory` | `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334` |
| `BootstrapCoordinator` | `0x329921023FCf6E337E924686b92f7521549B5970` |
| `RepositoryCore` | `0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4` |
| `RecoveryModule` | `0xa7249cE20B54C4Be440838F0eF403d2a6a07E532` |
| `ModerationModule` | `0xb957B65634931dD0613abAC296D5a3837BF979Bd` |
| `EconomicModule` | `0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd` |
| `UsernameModule` | `0xc7C164E46788b37e98D27aCe908C489999c09853` |
| `BadgeModule` | `0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D` |
| `ReleaseModule` | `0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f` |
| 激活固定区块 | `0x83f6bc1` |
| 激活区块 hash | `0x2d305e00e907794eeb99b8a7b50056064fb702886f2c1630c23e7654c6598dc1` |
| snapshot root | `0x5e2eaab50320b54d85e45bdc9f59ce94ada216d440fac3086fb30da7f98a1cea` |
| 激活模式 | `fresh-empty-suite` |
| V1 runtime policy | `archive-preview-only` |
| V1 migration | `false` |
| 七模块 imported records | 全部 `0` |
| 证据绑定源码提交 | `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986` |

这些地址是 evidence fact，不代表 checked-in public profile 已经可以使用。未经最终门禁批准，不要把 Directory 写进 CLI/Web 默认 profile。

## 11. 当前状态矩阵

| 领域 | 当前判断 |
|---|---|
| Solidity Suite、ABI、架构测试 | 已实现 |
| CLI EVM registry/transactor、remote helper | 已实现并有测试 |
| IPFS 上传、gateway 下载、复制确认 | 已实现 |
| Web EVM runtime、wallet、业务页面 | 已实现并有 API/行为测试 |
| V1 archive preview | 已实现且与普通路径隔离 |
| P1.1 operator runner | 广播/receipt/resume 代码已实现并有测试；journal 重载时的签名校验仍是 TODO |
| P1.2 部署 evidence | 已完成（历史证据） |
| P1.3 fresh-empty 激活 | 已完成（历史证据） |
| P1.4 产品 E2E/finality | 未完成 |
| P1.5 独立安全审查/批准 | 未完成 |
| P1.6 最终切换 gate | 未完成 |
| S3/R2 | 仅规划 |
| Mainnet | 未排期 |

## 12. 本地验证基线

在当前工作区实际执行的结果：

| 检查 | 结果 | 备注 |
|---|---|---|
| `(cd cli && go vet ./... && go test ./...)` | 通过 | 全部 Go package test 通过 |
| `(cd web && npm run test:api)` | 通过 | 73 个测试通过 |
| `(cd web && npm run typecheck)` | 通过 | TypeScript 检查通过 |
| `(cd web && npm run build)` | 通过 | 有 Vite chunk size 和第三方 PURE 注释 warning |
| `(cd contracts/evm-v2 && npm run check)` | 通过 | solc `0.8.24`、ABI/artifact 和 bytecode gate 通过；测试合约有代码大小 warning |
| `scripts/migration-cutover-readiness-test.ps1` | 通过 | fail-closed fixture gate 行为测试通过 |
| `bash scripts/evm-v2-check.sh --required` | 当前主机未通过 | PowerShell 默认 `bash` 转入 WSL 时找不到 `node`；显式 Git Bash 可完成 solc 检查，但本机缺 `forge` |
| `bash scripts/suite-readiness.sh --required` | 当前主机未通过 | required gate 仍需完整 Node/Foundry toolchain；source boundary 本身通过 |
| Foundry `forge` | 未安装 | required gate 不应把缺失工具当成通过 |

本机工具版本：Node `v24.11.1`、npm `11.6.2`、Go `go1.26.5 windows/amd64`；默认 `bash` 为 WSL wrapper，显式 Git Bash 可调用 Windows Node，但 PATH 中没有 `forge`。完整 required gate 应在 CI/Linux 或具备 Node/Foundry 的 WSL/Git Bash toolchain 中执行。

### Operator runner 的已知边界

`cli/cmd/igit-suite-operator` 的广播、receipt 查询、超时标记、journal resume 和 keystore 流程已经存在，当前 Go 测试也覆盖了主要路径。但 [`journal.go`](../cli/cmd/igit-suite-operator/journal.go) 在重载既有 JSONL 时仍明确保留 `TODO: Verify signature`，目前会接受未验证的历史 entry。它不影响当前 `fresh-empty-suite`（当前切换不执行 V1 导入），但在未来使用 operator 做有价值的迁移或恢复前，必须补上签名验证、篡改检测和对应测试。

## 13. 接续入口

执行级别的证据清单、P1.4-P1.6 步骤、停止条件和切换顺序见 [`evm-v2-handoff.md`](evm-v2-handoff.md)。后续工作原则：

1. 先确认源码 commit、证据 commit 和 scope 一致。
2. 先完成真实 Linux/Windows/Web E2E 和 finality 证据，再做独立安全审查。
3. 审查修复后重新绑定 source-sensitive evidence，生成 `cutover-evidence.sha256`。
4. 用同一个 40-hex commit 运行切换 gate；通过前不修改公开 profile。
5. 切换后再更新 CLI/Web 默认 Directory，并重新跑 release profile gate。
6. P1 后处理 P2/P3；S3/R2 进入 P4/P5 前先完成 successor protocol 设计和 ADR。

## 14. 常见误区

- 把 EVM V2 写成链上 Suite v2；当前链上版本是 `3`。
- 把 evidence 中的 Directory 直接当成可以写入默认 profile 的地址。
- 把 V1 archive 当成 EVM verification 失败时的 fallback。
- 把 `injectived`、Cosmos `inj1` contract address 或旧 `ContractAddress` 带入普通 EVM runtime。
- 在 ref 更新前省略 durable Pin；gateway 可读取不等于 pack 已持久化。
- 对 uncertain receipt 直接换 nonce 重发；应先查询原 tx hash 并使 nonce cache 失效。
- 只跑 `npm run check` 就宣称 Foundry test 完成；两类检查是不同证据。
- 只跑 fixture gate 就宣称 testnet cutover 完成。
- 为支持 S3/R2 只把 `ipfs://` 换成 HTTP URL；当前不可升级 Suite 不允许这种替换。

## 15. 相关文档

- [LiveAgent 接续上下文](liveagent-evm-v2-context.md)
- [文档索引](README.md)
- [架构](architecture.md)
- [EVM V2 迁移范围](evm-v2-migration.md)
- [ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md)
- [ADR 0002](adr/0002-pluggable-pack-storage.md)
- [ADR 0003](adr/0003-fresh-evm-suite-and-v1-archive-preview.md)
- [项目状态（中文）](project-status-zh.md)
- [交付路线图](delivery-roadmap.md)
- [发布和切换](release.md)
- [验收证据规则](acceptance-evidence.md)
- [测试网部署证据](../evidence/testnet-deployment-2026-08-25/PROGRESS-SUMMARY.md)

## 更新规则

涉及合约 ABI、Suite verification、交易类型、pack URI、V1 边界、部署证据或切换状态的代码变更，都应同时更新本文件或交接文档，并写明源码 commit、验证命令、证据路径、仍属计划的内容和受影响的安全不变量。

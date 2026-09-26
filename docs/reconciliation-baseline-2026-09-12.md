# 项目事实基线

本文件是本轮唯一新增的事实基线，不是迁移完成或生产验收报告。文件名按用户要求保留 2026-09-12；实际复核发生于 **2026-09-13（Asia/Shanghai）**，UTC 原始记录为 2026-09-12。

项目源码、静态问题、历史部署、本地测试、真实只读 RPC 和未验证闭环分别列示。四份当前状态/导航文档只引用本文件，旧报告保留。

## 1. 工作区身份与审计边界

- 根目录：`D:/inj/next-injective-git`。未切换到其他项目。
- 分支：`dev`；HEAD：`0ba06f436558f12d97625b393767440cdd0f9862`。
- Remote：`https://github.com/Hny0305Lin/next-injective-git.git`。
- 入场 Git 状态：4 份文档已修改，15 个文件 untracked；原状态见附录 A。四个 EVM 脚本本来就是 untracked。
- 指定六目录实际文件清单见附录 A；包括现有 out/cache/bin。排除 node_modules、dist、.next、.git 依赖/构建内容；这些文件存在不代表测试通过。
- 未发现适用的 AGENTS.md；已读取任务全文和本项目 Injective EVM skill。
- 无源码/ABI/部署证据编辑，无交易、主网 RPC、签名、私钥读取、commit、push、reset、clean、force push、pin 或 unpin；没有成功删除或移动文件。
- 入场现有文件哈希与文档原文保存在审计内存中；落盘前 585 个受检查原有文件均未变。敏感路径按名称排除，未读取用户密钥/配置。
- 执行命令的环境：Windows amd64；Node v24.11.1，npm 11.6.2，Go go1.26.5 windows/amd64，Git 2.45.1.windows.1，锁定 solc 0.8.24，本地实际 viem 2.55.19。辅助 Windows PowerShell 5.1.26100.7019。

状态定义：PASS = 有本轮可重复证据且该限定检查无阻断；FAIL = 已证实不匹配；BLOCKED = 工具/权限/服务阻断；NOT PROVEN = 缺实证；HISTORICAL = 只描述过去。下文的 PASS 不扩展到未执行的验收层。

## 2. 九合约源码与本地检查

| 生产合约 | 源码 | initcode bytes | runtime bytes | solc/ABI/尺寸 |
|---|---|---|---|---|
| SuiteDirectory | contracts/evm-v2/src/SuiteDirectory.sol | 4059 | 3745 | PASS |
| BootstrapCoordinator | contracts/evm-v2/src/BootstrapCoordinator.sol | 4968 | 4478 | PASS |
| RepositoryCore | contracts/evm-v2/src/RepositoryCore.sol | 23925 | 23504 | PASS |
| RecoveryModule | contracts/evm-v2/src/RecoveryModule.sol | 9350 | 9006 | PASS |
| ModerationModule | contracts/evm-v2/src/ModerationModule.sol | 16862 | 16383 | PASS |
| EconomicModule | contracts/evm-v2/src/EconomicModule.sol | 10008 | 9476 | PASS |
| UsernameModule | contracts/evm-v2/src/UsernameModule.sol | 8800 | 8417 | PASS |
| BadgeModule | contracts/evm-v2/src/BadgeModule.sol | 6967 | 6653 | PASS |
| ReleaseModule | contracts/evm-v2/src/ReleaseModule.sol | 6589 | 6227 | PASS |

执行 `npm run check`，时间 2026-09-13 00:55:38 +08:00 至 2026-09-13 00:56:47 +08:00，exit=0，输出 `EVM SUITE SOLC CHECK: PASS`。脚本只校验现有 ABI/artifacts，未使用 --write-abi/--write-artifacts。九份 CLI 内嵌 ABI 与合约 ABI JSON 语义相同（PASS）。

生产 runtime 均小于 24576 bytes；RepositoryCore 为 23504 bytes，余量 1072。测试合约 SuiteProtocolParityTest runtime=25537 bytes，超过 24576 的警告真实存在；它不是生产合约。本轮未执行 Foundry，不能判断该警告在 Foundry 测试部署中的实际影响。

| 检查 | 状态 | 说明 |
|---|---|---|
| 九个生产源文件、锁定 solc/ABI/artifact | PASS | 本地编译与一致性检查 |
| forge build | BLOCKED | 当前 PATH 无 forge，未执行 |
| forge test -vvv | BLOCKED | 当前 PATH 无 forge，未执行 |
| forge test --gas-report | BLOCKED | 当前 PATH 无 forge，未执行 |
| required invariant/gas 验收 | BLOCKED | 原有 out/cache 和旧报告不能代替本轮输出 |
| 全项目平均覆盖率 91% / 逐合约百分比 | NOT PROVEN | 无对应可复现 coverage 命令+结果；撤销当前指标效力 |
| 九合约完整验收 | NOT PROVEN | 编译/实时哈希不能代替 Foundry、行为、安全和产品 E2E |

## 3. 测试网 Suite 实时只读状态

- RPC：`https://k8s.testnet.json-rpc.injective.network`。先执行 eth_chainId，返回 `0x59f`（1439），才继续其他读取。
- 固定区块：**139852506 / 0x855fada**；hash：`0x051d5b1577e22dc62d87a528d694cdd2391376f4ed844c154f476082a9f232cd`。
- 首次查询时间：2026-09-13 00:57:23 +08:00。每个请求的 UTC 时间、endpoint、区块参数和返回值见附录 D。
- Directory：`0x24124cb60f9ef02f7deb5bc868c028fb412f5334`；runtime 3745 bytes，Keccak-256 `0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd`，与 deployment.json 一致（PASS）。
- state()=1（SuiteDirectory.sol enum Active）、suiteVersion()=3、configuredChainId()=1439（均 PASS）。
- Coordinator：`0x329921023FCf6E337E924686b92f7521549B5970`；runtime 4478 bytes，code hash `0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7`，与 Directory 和 deployment.json 一致（PASS）。
- snapshotRoot()：`0x5e2eaab50320b54d85e45bdc9f59ce94ada216d440fac3086fb30da7f98a1cea`，与历史证据一致（PASS）。

| 模块 | 实时地址 | 实时 Keccak-256 = Directory moduleCodeHash = 历史 evidence | 状态 |
|---|---|---|---|
| RepositoryCore | `0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4` | `0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467` | PASS |
| RecoveryModule | `0xa7249cE20B54C4Be440838F0eF403d2a6a07E532` | `0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2` | PASS |
| ModerationModule | `0xb957B65634931dD0613abAC296D5a3837BF979Bd` | `0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3` | PASS |
| EconomicModule | `0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd` | `0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d` | PASS |
| UsernameModule | `0xc7C164E46788b37e98D27aCe908C489999c09853` | `0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e` | PASS |
| BadgeModule | `0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D` | `0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c` | PASS |
| ReleaseModule | `0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f` | `0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c` | PASS |

七个 moduleAddress、moduleCodeHash、eth_getCode 均在同一固定区块核对。七模块的 suiteDirectory/bootstrapCoordinator/moduleId 也与 Directory/源码要求一致（PASS）。所有哈希为代码 Keccak-256，不是 SHA-256。

历史 `deployment.json` 绑定提交 `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`，当前 HEAD 不同。git merge-base --is-ancestor 返回 0；两个提交间 contracts/evm-v2/src、abi、artifacts 的 git diff --stat 为空。九个源码 SHA-256、creation bytecode SHA-256、runtime template SHA-256 与历史证据相同；把证据的 immutable_values_by_solc_id 代入当前 artifact 的 immutable_references 后，九个重建 runtime 的 Keccak 均等于实时和历史哈希（PASS）。这证明被核对合约字节对应关系，不等于当前整个项目获批。

`suite-runtime-verification-2026-08-29.json`、`suite-verification.json`、部署/激活 receipts 和 Blockscout 文档保留为 HISTORICAL。本轮没有重放、重写或补造历史交易，也没有把历史 explorer 记录当作本轮安全验收。

## 4. CLI 与 Web：分层验证

| 检查 | 状态 | 时间（Asia/Shanghai） | 真实范围 |
|---|---|---|---|
| go test -count=1 ./... | PASS | 2026-09-13 00:58:09 +08:00 | 所有有测试包通过；10个包 no test files，不能算测试覆盖 |
| go vet ./... | PASS | 2026-09-13 00:58:09 +08:00 | 退出0，stdout/stderr为空 |
| npm run typecheck | PASS | 2026-09-13 00:55:38 +08:00 | TypeScript本地检查 |
| npm run test:api | PASS | 2026-09-13 00:55:41 +08:00 | 73 tests / 73 pass / 0 fail / 0 skip；mock fetch/provider及fixture |
| npm run build + 安全输出参数 | PASS | 2026-09-13 00:55:44 +08:00 | tsc -b && vite build；第三方PURE与chunk>500kB warning |
| igit suite verify --json | BLOCKED | 2026-09-13 00:59:02 +08:00 | Windows DACL Access is denied；无完整SuiteInfo |
| evm-demo-inspect 完整模式 | BLOCKED | 2026-09-13 01:01:40 +08:00 | 45秒context预算后在username progress超时；部分返回不是全量PASS |
| evm-demo-inspect -code-addresses | PASS | 2026-09-13 01:04:37 +08:00 | 真实测试网读九个runtime/hash；latest模式，非完整固定区块VerifySuite |
| 真实 EVM 写交易 | NOT PROVEN | 本轮未执行 | 没有使用测试或用户密钥发送交易 |
| 真实 push/fetch E2E | NOT PROVEN | 本轮未执行 | fakeChain/fakeIPFS/fakeAuthorizer不是真实闭环 |

Go 重点目录均已核对：`internal/chain` 的 RPC/Suite/交易测试使用测试服务器及依赖注入；`internal/remote/push_flow_test.go` 显式使用 fakeChain/fakeIPFS/fakeAuthorizer；`internal/ipfs` 的 HTTP 测试为 httptest gateway；`internal/replication` 的 confirmation/authorization 是 httptest；`internal/suitedeploy` 的部署/恢复是 fixture；`internal/suitemigration` 证明离线 plan/manifest/rolling-root/拒绝篡改语义；`cmd/igit` 和 `cmd/git-remote-igit` 的入口测试通过。都不能外推为真实链写入、远端 pin 或 clean Git E2E。

普通 CLI 只读命令使用本轮新建的无密钥隔离配置（仅 network/RPC/chain ID/Directory），但 config.Load 的 ProtectDirectory 失败：`igit: protect config directory before read: set sensitive-file DACL: Access is denied.` Node辅助环境及原生PowerShell均复现；保持 BLOCKED，不关闭 DACL 防护。既有 evm-demo-inspect 使用不存在的隔离配置目录并显式传入 RPC/Directory，绕过配置文件存在分支后可读代码；这是既有读命令的不同范围，不能覆盖 suite verify 的失败。完整模式初始 block=0x855fc13，但其合约字段调用用 latest，且最后超时。

Web构建命令实际为：

`npm run build -- --outDir node_modules/.cache/reconciliation-20260913-0054 --emptyOutDir false`

这是用户要求 build 的同一 package script，额外参数保护已有 dist，不执行清空；新产物保留在 ignored 目录。不是默认 dist 被重新验收。

**文档声称 Moderation UI 已实现，但对应文件实际不存在。**

- `D:/inj/next-injective-git/web/src/lib/moderationModel.ts`：文件不存在。
- `D:/inj/next-injective-git/web/src/lib/moderationTransaction.ts`：文件不存在。
- `D:/inj/next-injective-git/web/src/pages/Repo/ModerationTab.tsx`：文件不存在。

`web/src/pages/Repo/index.tsx:525` 仅有 code/commits/refs/sponsors 等 tab，没有 Moderation tab；`modules.ts:311` 起的业务 API 存在（静态 PASS），不能证明 UI 或真实钱包交互。未创建缺失文件。

公开内置 CLI/Web profile 的 Directory 为空；Web `loadConfig()` 在本地 origin 会使用 `0xf8844F90887731FFd607E1f59e39a3918F6eAb35`，并可载入已有 localStorage Suite 设置，因此“所有路径都为空”不成立。该地址不是本轮核对的 Directory，本轮未查询它。

`cli/cmd/igit-suite-operator/journal.go:61` 保留 TODO: Verify signature。operator 本地测试通过不能外推日志重载验签安全，相关“safe resume完成”声明为 FAIL/NOT PROVEN；本轮未执行迁移。

## 5. IPFS 与 EVM 事件索引器

四脚本仅静态审阅和本地 ABI fixture 分析，**未执行**其主流程；不做 pin/unpin、S3 写入或主网查询。

| 检查项 | 状态 | 源码与实际证据 |
|---|---|---|
| Topic0 | FAIL | event:15 / hot-pin:18 / archive:19 均为0xa7c1...占位值；当前ABI计算应为0x172cc57eb3f906b5e4bcf3a85861062c792de8d5d56754a3785d99057dec948a；reaper本身不使用事件Topic |
| module ID / selector | FAIL | 四脚本均sha256sum；event还先把文本转hex再做SHA-256。正确CORE=0x0f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8；moduleAddress(bytes32)正确selector=0xaa10e9f0，脚本0x40a3d246错误 |
| 当前ABI接入 | FAIL | 未导入contracts/evm-v2/abi或cli/internal/chain/abi；四脚本自己拼selector/word/address，动态结果cut/grep/sed占位 |
| indexed参数 | FAIL | repoId/refName/updatedBy是indexed；data只有commitSha与string[] packUris。indexed string只能得到hash，不能从data恢复refName |
| 动态string[] | FAIL | 本地合成fixture由viem编码/解码两个URI成功；对相同ABI hex应用脚本正则得到[]，不是两个CID。fixture不是实际IPFS上传 |
| reaper函数存在性 | FAIL | repositoryCount与repositoryIdAt不在当前ABI；实际有getRef、listRepositoriesPage(address owner,uint256,uint256)、listRefsPage(bytes32,uint256,uint256)。不能把按owner分页误当全局计数 |
| checkpoint | FAIL | event/archive只保存block数字+时间，event另有计数；hot-pin没有checkpoint。无链ID/Directory/ABI版本绑定 |
| blockHash / txHash / logIndex | FAIL | event仅输出transactionHash；不持久化blockHash/logIndex/removed，也无事件身份去重 |
| duplicate / 闭区间 | FAIL | eth_getLogs从checkpoint到next_end均包含端点；下次仍从next_end起，边界重复。无去重；event把JSON事件和count一起捕获进算术processed变量 |
| RPC range limit | FAIL | 声明MAX_BLOCKS_PER_QUERY=10000但闭区间实际可到10001块；未见限流拆段/退避及回归测试 |
| RPC/ABI失败是否推进 | FAIL | hot/archive/reaper以jq .result // empty吞JSON-RPC error，local var=$(...)掩盖失败；archive空logs返回0后推进checkpoint。event用cut占位且\|\| echo空，非严格decode拒绝 |
| pin/archive失败是否推进 | FAIL | archive:154/158仅echo失败，main:197/198继续保存checkpoint；pinned状态会让归档失败项后续跳过；不能称fail-closed |
| 原子状态写入 | NOT PROVEN | event/archive存在.tmp + mv rename模式；没有跨pin/archive状态一致性、fsync/锁/崩溃注入证据；并非完全没有原子写代码 |
| reorg实现 | FAIL | 未发现block hash恢复验证、确认深度、removed log处理或回退；不能称Block confirmations已实现 |
| reorg/重复/失败回归测试 | NOT PROVEN | scripts、cli、web/test中未找到这四个脚本对应测试；旧V1测试不能当V2证明 |
| unpin safety | FAIL | hot-pin默认ALLOW_UNPIN=false但true可pin rm；reaper:149直接pin rm，没有ALLOW_UNPIN或dry-run gate、宽限期/最终性证明；:13/28强制mainnet。本轮绝不运行这些脚本 |

真实事件声明位于 `contracts/evm-v2/src/RepositoryCore.sol:131`：

```solidity
event RefUpdated(bytes32 indexed repoId, string indexed refName, string commitSha, string[] packUris, address indexed updatedBy);
```
模块 ID 在 `src/suite/ISuite.sol:5` 使用 Keccak。当前 ABI 用 viem.toEventSelector/toFunctionSelector/keccak256(toHex(text)) 计算结果，不手写 selector 或动态word decoder。合成 fixture 与脚本不符的结果见附录 C；没有修改证据/脚本来让测试通过。

| 真实闭环环节 | 状态 | 范围 |
|---|---|---|
| IPFS client单元测试 | PASS | httptest模拟Kubo add/pin响应 |
| HTTP gateway mock测试 | PASS | httptest+假fetch测试fallback，不是真实gateway |
| 本地Kubo生命周期测试 | BLOCKED | IGIT_RUN_NATIVE_KUBO_INTEGRATION未启用，输出SKIP；无ipfs命令、API连接失败 |
| 当前Kubo运行 | BLOCKED | 未观察到ipfs/kubo进程和5001/8080监听；只探测默认localhost，不推断其他机器 |
| 真实上传/CID产生 | NOT PROVEN | 本轮未上传；fixture字符串不是真实CID产生证据 |
| 真实recursive pin | BLOCKED | /api/v0/pin/ls?type=recursive ECONNREFUSED |
| 远端replication confirmation | NOT PROVEN | 只有mock，未连接真实confirmation服务 |
| EVM update_ref / RefUpdated | NOT PROVEN | 本轮不发送updateRef；未进行事件历史全扫，没有本轮真实业务event样本 |
| indexer checkpoint实证 | FAIL | 代码存在上述明确缺陷；持久状态没有运行实证 |
| 真实gateway fetch | NOT PROVEN | 没有与本次真实pack/CID关联的HTTP下载记录 |
| git object integrity | NOT PROVEN | 没有真实pack下载后的git object验证链 |
| 完整IPFS+EVM+CLI闭环 | NOT PROVEN | 缺真实Kubo、真实测试网写入、真实gateway和Git完整性证据 |

**IPFS + EVM + CLI 完整闭环尚未验证。**

## 6. 文档冲突与当前效力

| 文档（项目根目录内） | 原表述 | 实际证据 | 正确状态 | 是否修改 |
|---|---|---|---|---|
| docs/project-status-zh.md | 九合约100%；平均覆盖91%；A04/A11完成；缺测试网地址 | 九源码solc PASS；coverage NOT PROVEN；UI缺失和indexer FAIL；实时Suite已读 | FAIL / NOT PROVEN | 是，新增准确当前区，原文保留为HISTORICAL |
| docs/project-status.md | Foundry通过；operator complete/60.5%；P1.2/1.3 complete | Foundry BLOCKED；journal TODO；旧部署/激活HISTORICAL；本地Go PASS不等于全部安全 | BLOCKED / HISTORICAL / NOT PROVEN | 是，当前入口重定向唯一基线 |
| docs/backlog.md | A04/A11核心实现完成；只需验证；需要Suite地址 | UI文件不存在；脚本代码不匹配；Suite地址已知 | FAIL | 是，R01–R08重新列任务 |
| docs/README.md | A11完成；知识库完整事实；cli/web README链接 | 脚本FAIL；知识库是旧快照；cli/README.md和web/README.md不存在 | FAIL / HISTORICAL | 是，唯一基线入口+历史分类 |
| docs/a11-storage-indexer-v2.md | Implemented；ABI-decoded；fail-closed；checkpoint/reorg；reaper safe/dry-run | placeholder ABI、无reorg、hot-pin无checkpoint、reaper无dry-run/ALLOW_UNPIN；函数不存在 | FAIL | 否，保留原文件；在当前入口标为历史迁移草稿 |
| docs/evm-v2-handoff.md | 已完成部署激活；2026-09-11本机通过；profile为空 | 历史证据与合约模型匹配；本轮工具/ACL和Web本地fallback需补充 | HISTORICAL | 否，保留；本基线覆盖当前状态 |
| docs/liveagent-evm-v2-context.md | P1.1/1.2/1.3历史实现或证据；过去本机测试 | 使用operator/部署/激活编号，与新草稿Moderation/索引器编号冲突 | HISTORICAL | 否，保留；任务改用R编号 |
| docs/project-knowledge-base-zh.md | IPFS上传/gateway/复制已实现；旧部署完成；所有公开配置为空 | 客户端代码存在、本地mock PASS；真实闭环NOT PROVEN；本地loadConfig fallback需限定 | HISTORICAL / NOT PROVEN | 否，保留；不是当前唯一事实来源 |
| MIGRATION-COMPLETE.md | Moderation完整实现；九合约100%；indexer生产就绪（需测试） | 3文件不存在；solc≠验收；错误Topic/hash/ABI和unpin | FAIL / NOT PROVEN | 否，历史迁移草稿/未对齐报告 |
| MIGRATION-REPORT-P1P2.md | P1.1 UI/P1.2索引已实现；CLI/Web后端100% | UI缺失；脚本FAIL；本地测试不证明完整功能/E2E | FAIL / NOT PROVEN | 否，保留历史草稿 |
| A01-BXX-MIGRATION-REPORT.md | 创建3个UI文件、修改Repo集成；后端完整 | 路径/Repo tab不匹配；实际API存在；不能推断完成 | FAIL / NOT PROVEN | 否，保留历史草稿 |

原文声明逐行清单见附录 E，包括事实断言、计划和警告上下文；行号针对本轮入场文本。没有命令/输出/环境/日期或真实范围证据的断言只能为 NOT PROVEN/HISTORICAL，不因为报告自身打勾而变成 PASS。中文91%与英文60.5%均未作为当前coverage保留。

以下文件保留原样并统一标记为：**“历史迁移草稿/未对齐报告，不作为当前项目状态依据。”**

- 根目录 MIGRATION-COMPLETE.md、MIGRATION-REPORT-P1P2.md、A01-BXX-MIGRATION-REPORT.md，以及其余 MIGRATION-/PRIORITY- 草稿。
- docs/a11-storage-indexer-v2.md、docs/PRIORITY-MAPPING.md。
- docs/evm-v2-handoff.md、docs/liveagent-evm-v2-context.md、docs/project-knowledge-base-zh.md 中的旧状态快照属于 HISTORICAL；架构描述需按当前代码逐条引用。

旧文档中的 P1.1/P1.2/P1.3 分别被用于“operator/部署/激活”和“Moderation/索引/ABI”两套含义，不能互换。后续任务统一采用本轮待办中的 R01–R08。


## 7. 本轮修改与保留情况

本轮未修改源码，只完成事实审计和文档状态对齐。

| 绝对路径 | 修改内容 |
|---|---|
| D:/inj/next-injective-git/docs/project-status-zh.md | 追加明确当前状态区；旧原文整个区块标为HISTORICAL；不覆盖原未提交文字 |
| D:/inj/next-injective-git/docs/project-status.md | 同一基线的英文入口与核对范围；旧状态原文保留 |
| D:/inj/next-injective-git/docs/backlog.md | R01–R08独立后续任务；旧任务完整保留为历史 |
| D:/inj/next-injective-git/docs/README.md | 唯一基线导航、旧报告失效分类、缺失README及缓存说明 |
| D:/inj/next-injective-git/docs/reconciliation-baseline-2026-09-12.md | 唯一新增事实基线；命令原始输出、实际文件清单、RPC/哈希、逐条声明和阻断 |

所有现有报告、四个untracked脚本、源码、ABI、部署和测试证据均保留。四份已有未提交改动的文档通过“新当前区 + 折叠历史原文”方式修改：原始字节序列连续保存在历史区，并保存入场SHA-256供核对；没有git恢复/回退用户内容。

测试构建在 ignored `web/node_modules/.cache/reconciliation-20260913-0054` 产生输出；只读 CLI 配置在两个新 ignored 目录 `reconciliation-readonly-config-20260913-0100` 与 `reconciliation-cli-native-20260913-0104`，只含公开测试网参数、没有密钥。

辅助进程最初缺少正确 USERPROFILE/SystemDrive，启动失败并生成以下缓存（不计为项目功能文件）：

| 本轮新增缓存目录 | 实际文件数 |
|---|---|
| D:/inj/next-injective-git/cli/null | 100 |
| D:/inj/next-injective-git/contracts/evm-v2/%SystemDrive% | 4 |
| D:/inj/next-injective-git/contracts/evm-v2/null | 2 |
| D:/inj/next-injective-git/web/%SystemDrive% | 4 |
| D:/inj/next-injective-git/web/null | 4 |

均未在入场清单中存在；内容为 Go 编译缓存、npm日志及 Windows shell cache，不打印其内容。自动审批策略拒绝清理本轮生成缓存的请求（`blocked by policy`，无更具体理由）。没有执行成功的删除/移动，未尝试通过另一工具清理。缓存仍为 untracked，应由后续明确范围的整理任务处理。因为本轮额外产生了这些缓存，不能声称工作树只新增一个Markdown文件或已经干净。

## 8. 阻断问题（按优先级）

- **P0 / FAIL**：事件Topic、module ID/hash算法、selector、动态ABI、reaper不存在的函数、checkpoint错误推进及unpin路径。阻断索引器使用与真实存储回收；本轮仅记录。
- **P1 / BLOCKED**：forge缺失；本机Kubo/API不可用；普通igit suite verify的Windows DACL失败；完整inspect公共RPC超时。
- **P1 / FAIL**：文档指定的Web Moderation UI文件不存在；不能把已有API当作UI验收。
- **P1 / NOT PROVEN**：真实CLI写入、clean Windows/Linux Git E2E、真实replication/gateway/Git完整性、钱包receipt/finality均缺实证。测试网资金本轮未检查，不能把“资金不足”冒认为已确认原因；写交易未获本轮授权。
- **P2 / NOT PROVEN**：当前reviewed commit绑定的安全审查、批准、checksum和切换证据；operator journal验签TODO；Web本地profile来源差异；本轮新增缓存未整理。

下列 required evidence 在现有部署证据目录中全部**文件不存在**，本轮未创建替代品：

- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/foundry-test.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/foundry-invariant.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/foundry-gas.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/windows-clean-e2e.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/linux-clean-e2e.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/web-receipt-e2e.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/security-review.pdf`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/finality-runbook.md`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/cutover-approval.txt`
- `D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/cutover-evidence.sha256`

## 9. 重新开始时的独立任务清单

| 任务 | 优先级 | 当前状态 | 独立范围与退出条件 |
|---|---|---|---|
| R01 索引 ABI 与链身份修复 | P0 | FAIL | 在后续源码任务中使用当前 ABI 库计算 Topic/module ID/selector，正确处理 indexed refName hash、commitSha 和 string[]；消除不存在的 repositoryCount/repositoryIdAt 依赖，明确仓库枚举来源。不得启用现有 reaper。 |
| R02 索引恢复与 unpin 安全 | P0 | FAIL | 验证 block hash + tx hash + log index 去重、闭区间边界、RPC 分段重试、reorg 回退、ABI/pin/archive 失败不推进 checkpoint、原子一致状态；先具备真正的 dry-run，自动 unpin 继续禁止。 |
| R03 Windows CLI 配置读取 | P1 | BLOCKED | 复现并定位 DACL Access is denied；后续授权源码修复后，使用无密钥测试网配置让完整 igit suite verify 在固定区块返回七模块绑定。不要关闭权限保护掩盖问题。 |
| R04 Foundry 验收 | P1 | BLOCKED | 配齐 forge 后运行 build、test -vvv、test --gas-report 及 required invariant gate；保留实际版本、commit、输出，并处理测试合约尺寸问题。 |
| R05 Kubo 与存储实证 | P1 | BLOCKED | 后续明确范围后配置本机 Kubo，真实生成 CID、recursive pin、远端 confirmation、gateway 读取与 Git object 校验；不能沿用 skipped 生命周期测试作为证据。 |
| R06 Moderation UI | P1 | FAIL | 后续单独实现当前缺失的 UI，并检查 Repo 集成；先建立可测的本地交互，再在明确交易授权后收集钱包收据。 |
| R07 测试网产品 E2E | P1 | NOT PROVEN | R01–R06 相关阻断解除后，取得明确写交易范围及测试网资金，在干净 Windows/Linux 跑真实 push/clone/fetch/pull/ref delete，收集完整因果链。资金本轮未检查，不能假定足够或不足。 |
| R08 切换证据与后续功能范围 | P2 | NOT PROVEN | 统一 reviewed commit，补齐安全审查/finality/approval/checksum；清点 Web local profile 差异、operator journal 验签 TODO 和新增缓存。Recovery/Release/Username 等后续 UI 另立有范围的任务。 |

R01–R08 只是后续任务清单，本轮没有实现这些功能或发送交易。静态修复任务可以开始；真实 E2E 需先解决对应环境/代码阻断并获得明确写交易授权；不得进入生产部署。


## 10. 结论

事实与当前文档效力已对齐到本基线：九合约源码/产物与部署证据分开核对；实时Suite记录已保存；本地/mock与真实只读RPC分开；索引错误和缺失UI明确；旧报告与旧文档原文完整保留且失去当前状态依据效力。文档层的“重新对齐”可以作为下一轮静态修复任务的起点，**不表示项目或工作树已经验收完成**。

完整功能/E2E验收为 NOT PROVEN。Foundry、Kubo、普通CLI配置读取仍 BLOCKED；索引器和UI声明仍 FAIL。可开始有范围的新源码修复/环境诊断任务；本轮不允许进入真实写交易E2E，未来需先解相应阻断并取得用户明确交易授权；**不允许生产部署或公开cutover**。清理缓存曾被自动审批拒绝，缓存仍保留且已登记，工作树不干净。

## 附录 A. 入场工作区原始命令与实际文件清单

### git rev-parse --show-toplevel

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.093Z; exit=0

```text
D:/inj/next-injective-git

```

### git status --short --branch

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.089Z; exit=0

```text
## dev...origin/dev
 M docs/README.md
 M docs/backlog.md
 M docs/project-status-zh.md
 M docs/project-status.md
?? A01-BXX-MIGRATION-REPORT.md
?? MIGRATION-COMPLETE.md
?? MIGRATION-EXECUTION-SUMMARY-ZH.md
?? MIGRATION-REPORT-P1P2.md
?? PRIORITY-MIGRATION-DONE.md
?? PRIORITY-UPDATE-COMPLETE.md
?? docs/PRIORITY-MAPPING.md
?? docs/a11-storage-indexer-v2.md
?? docs/evm-v2-handoff.md
?? docs/liveagent-evm-v2-context.md
?? docs/project-knowledge-base-zh.md
?? scripts/evm-archive-indexer.sh
?? scripts/evm-event-indexer.sh
?? scripts/evm-hot-pin-indexer.sh
?? scripts/evm-replication-reaper.sh

```

### git branch --show-current

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.096Z; exit=0

```text
dev

```

### git rev-parse HEAD

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.099Z; exit=0

```text
0ba06f436558f12d97625b393767440cdd0f9862

```

### git remote -v

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.102Z; exit=0

```text
origin	https://github.com/Hny0305Lin/next-injective-git.git (fetch)
origin	https://github.com/Hny0305Lin/next-injective-git.git (push)

```

### powershell.exe -NoProfile -Command Get-Location

cwd=D:/inj/next-injective-git; at=2026-09-12T16:53:45.081Z; exit=0

```text

Path                     
----                     
D:\inj\next-injective-git



```

<details>
<summary>contracts/evm-v2：入场实际文件 68 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/contracts/evm-v2/README.md
D:/inj/next-injective-git/contracts/evm-v2/abi/BadgeModule.json
D:/inj/next-injective-git/contracts/evm-v2/abi/BootstrapCoordinator.json
D:/inj/next-injective-git/contracts/evm-v2/abi/EconomicModule.json
D:/inj/next-injective-git/contracts/evm-v2/abi/ModerationModule.json
D:/inj/next-injective-git/contracts/evm-v2/abi/RecoveryModule.json
D:/inj/next-injective-git/contracts/evm-v2/abi/ReleaseModule.json
D:/inj/next-injective-git/contracts/evm-v2/abi/RepositoryCore.json
D:/inj/next-injective-git/contracts/evm-v2/abi/SuiteDirectory.json
D:/inj/next-injective-git/contracts/evm-v2/abi/UsernameModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/BadgeModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/BootstrapCoordinator.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/EconomicModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/ModerationModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/RecoveryModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/ReleaseModule.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/RepositoryCore.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/SuiteDirectory.json
D:/inj/next-injective-git/contracts/evm-v2/artifacts/UsernameModule.json
D:/inj/next-injective-git/contracts/evm-v2/cache/solidity-files-cache.json
D:/inj/next-injective-git/contracts/evm-v2/cache/test-failures
D:/inj/next-injective-git/contracts/evm-v2/foundry.toml
D:/inj/next-injective-git/contracts/evm-v2/out/BadgeModule.sol/BadgeModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/BootstrapCoordinator.sol/BootstrapCoordinator.json
D:/inj/next-injective-git/contracts/evm-v2/out/EconomicModule.sol/EconomicModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IBootstrapCoordinator.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IBootstrapModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IEconomicOwnershipHook.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IModerationPolicy.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IOwnershipTransferState.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IRecoveryOwnershipHook.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IRecoveryState.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/IRepositoryCore.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/ISuiteBound.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/ISuiteDirectory.json
D:/inj/next-injective-git/contracts/evm-v2/out/ISuite.sol/SuiteIds.json
D:/inj/next-injective-git/contracts/evm-v2/out/ModerationModule.sol/ModerationModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/RecoveryModule.sol/RecoveryModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/ReleaseModule.sol/ReleaseModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/RepositoryCore.sol/RepositoryCore.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/ReentrantSponsorReceiver.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/RevertingBindingModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteArchitectureSecurityTest.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteArchitectureTest.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteArchitectureTestBase.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteImportValidationTest.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteProtocolParityTest.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/SuiteVm.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteArchitecture.t.sol/WronglyBoundModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteDirectory.sol/SuiteDirectory.json
D:/inj/next-injective-git/contracts/evm-v2/out/SuiteModule.sol/SuiteModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/UsernameModule.sol/UsernameModule.json
D:/inj/next-injective-git/contracts/evm-v2/out/build-info/9840f531faea4b97.json
D:/inj/next-injective-git/contracts/evm-v2/out/build-info/dc75b0937621de7d.json
D:/inj/next-injective-git/contracts/evm-v2/package-lock.json
D:/inj/next-injective-git/contracts/evm-v2/package.json
D:/inj/next-injective-git/contracts/evm-v2/src/BadgeModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/BootstrapCoordinator.sol
D:/inj/next-injective-git/contracts/evm-v2/src/EconomicModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/ModerationModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/RecoveryModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/ReleaseModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/RepositoryCore.sol
D:/inj/next-injective-git/contracts/evm-v2/src/SuiteDirectory.sol
D:/inj/next-injective-git/contracts/evm-v2/src/UsernameModule.sol
D:/inj/next-injective-git/contracts/evm-v2/src/suite/ISuite.sol
D:/inj/next-injective-git/contracts/evm-v2/src/suite/SuiteModule.sol
D:/inj/next-injective-git/contracts/evm-v2/test/SuiteArchitecture.t.sol
```

</details>

<details>
<summary>cli：入场实际文件 138 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/cli/bin/archive-audit/main.go
D:/inj/next-injective-git/cli/bin/evm-activate-suite.exe
D:/inj/next-injective-git/cli/bin/igit
D:/inj/next-injective-git/cli/bootstrap-run.log
D:/inj/next-injective-git/cli/cmd/derive-address-demo/main.go
D:/inj/next-injective-git/cli/cmd/evm-activate-suite/main.go
D:/inj/next-injective-git/cli/cmd/evm-activate-suite/main_test.go
D:/inj/next-injective-git/cli/cmd/evm-demo-bootstrap/main.go
D:/inj/next-injective-git/cli/cmd/evm-demo-deploy-rest/main.go
D:/inj/next-injective-git/cli/cmd/evm-demo-inspect/main.go
D:/inj/next-injective-git/cli/cmd/evm-key-import-demo/import-hex.go
D:/inj/next-injective-git/cli/cmd/evm-key-import-demo/main.go
D:/inj/next-injective-git/cli/cmd/git-remote-igit/main.go
D:/inj/next-injective-git/cli/cmd/git-remote-igit/main_test.go
D:/inj/next-injective-git/cli/cmd/igit-deploy-suite/main.go
D:/inj/next-injective-git/cli/cmd/igit-deploy-suite/main_test.go
D:/inj/next-injective-git/cli/cmd/igit-release-profile-check/main.go
D:/inj/next-injective-git/cli/cmd/igit-replicationd/main.go
D:/inj/next-injective-git/cli/cmd/igit-replicationd/main_test.go
D:/inj/next-injective-git/cli/cmd/igit-suite-migrate/main.go
D:/inj/next-injective-git/cli/cmd/igit-suite-migrate/main_test.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/STATUS.md
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/broadcast.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/check-balance.exe
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/create-operator-key.exe
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/execute.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/igit-suite-operator.exe
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/journal.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/journal_test.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/keystore.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/main.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/main_test.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/manifest.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/manifest_test.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/receipt.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/recovery.go
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/setup-deploy-config.exe
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/test-manifest.json
D:/inj/next-injective-git/cli/cmd/igit-suite-operator/test-output.log
D:/inj/next-injective-git/cli/cmd/igit/archive.go
D:/inj/next-injective-git/cli/cmd/igit/doctor.go
D:/inj/next-injective-git/cli/cmd/igit/main.go
D:/inj/next-injective-git/cli/cmd/igit/main_test.go
D:/inj/next-injective-git/cli/cmd/igit/setup.go
D:/inj/next-injective-git/cli/cmd/igit/setup_test.go
D:/inj/next-injective-git/cli/cmd/simple-key-import/main.go
D:/inj/next-injective-git/cli/go.mod
D:/inj/next-injective-git/cli/go.sum
D:/inj/next-injective-git/cli/igit-cli.exe
D:/inj/next-injective-git/cli/igit.exe
D:/inj/next-injective-git/cli/internal/archivev1/client.go
D:/inj/next-injective-git/cli/internal/archivev1/inventory.go
D:/inj/next-injective-git/cli/internal/archivev1/inventory_test.go
D:/inj/next-injective-git/cli/internal/archivev1/verify.go
D:/inj/next-injective-git/cli/internal/bootstrap/bootstrap.go
D:/inj/next-injective-git/cli/internal/bootstrap/bootstrap_integration_test.go
D:/inj/next-injective-git/cli/internal/bootstrap/bootstrap_integration_unix_test.go
D:/inj/next-injective-git/cli/internal/bootstrap/bootstrap_test.go
D:/inj/next-injective-git/cli/internal/bootstrap/deps.json
D:/inj/next-injective-git/cli/internal/bootstrap/process_unix.go
D:/inj/next-injective-git/cli/internal/bootstrap/process_windows.go
D:/inj/next-injective-git/cli/internal/bootstrap/process_windows_test.go
D:/inj/next-injective-git/cli/internal/chain/abi/BadgeModule.json
D:/inj/next-injective-git/cli/internal/chain/abi/BootstrapCoordinator.json
D:/inj/next-injective-git/cli/internal/chain/abi/EconomicModule.json
D:/inj/next-injective-git/cli/internal/chain/abi/ModerationModule.json
D:/inj/next-injective-git/cli/internal/chain/abi/RecoveryModule.json
D:/inj/next-injective-git/cli/internal/chain/abi/ReleaseModule.json
D:/inj/next-injective-git/cli/internal/chain/abi/RepositoryCore.json
D:/inj/next-injective-git/cli/internal/chain/abi/SuiteDirectory.json
D:/inj/next-injective-git/cli/internal/chain/abi/UsernameModule.json
D:/inj/next-injective-git/cli/internal/chain/backend.go
D:/inj/next-injective-git/cli/internal/chain/backend_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_address.go
D:/inj/next-injective-git/cli/internal/chain/evm_codec.go
D:/inj/next-injective-git/cli/internal/chain/evm_errors.go
D:/inj/next-injective-git/cli/internal/chain/evm_gas.go
D:/inj/next-injective-git/cli/internal/chain/evm_gas_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_keystore.go
D:/inj/next-injective-git/cli/internal/chain/evm_keystore_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_nonce.go
D:/inj/next-injective-git/cli/internal/chain/evm_nonce_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_rpc.go
D:/inj/next-injective-git/cli/internal/chain/evm_rpc_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_suite_registry.go
D:/inj/next-injective-git/cli/internal/chain/evm_suite_registry_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_suite_values.go
D:/inj/next-injective-git/cli/internal/chain/evm_transactor.go
D:/inj/next-injective-git/cli/internal/chain/evm_transactor_test.go
D:/inj/next-injective-git/cli/internal/chain/evm_types.go
D:/inj/next-injective-git/cli/internal/chain/moderation_values.go
D:/inj/next-injective-git/cli/internal/chain/suite_abi.go
D:/inj/next-injective-git/cli/internal/chain/suite_directory.go
D:/inj/next-injective-git/cli/internal/chain/suite_directory_test.go
D:/inj/next-injective-git/cli/internal/chain/test_helpers_test.go
D:/inj/next-injective-git/cli/internal/chain/types.go
D:/inj/next-injective-git/cli/internal/config/config.go
D:/inj/next-injective-git/cli/internal/config/config_test.go
D:/inj/next-injective-git/cli/internal/environment/environment.go
D:/inj/next-injective-git/cli/internal/environment/environment_test.go
D:/inj/next-injective-git/cli/internal/fileprotection/protection_test.go
D:/inj/next-injective-git/cli/internal/fileprotection/protection_unix.go
D:/inj/next-injective-git/cli/internal/fileprotection/protection_windows.go
D:/inj/next-injective-git/cli/internal/fileprotection/write.go
D:/inj/next-injective-git/cli/internal/gitio/gitio.go
D:/inj/next-injective-git/cli/internal/i18n/i18n.go
D:/inj/next-injective-git/cli/internal/i18n/i18n_test.go
D:/inj/next-injective-git/cli/internal/i18n/locale_unix.go
D:/inj/next-injective-git/cli/internal/i18n/locale_windows.go
D:/inj/next-injective-git/cli/internal/ipfs/client.go
D:/inj/next-injective-git/cli/internal/ipfs/client_test.go
D:/inj/next-injective-git/cli/internal/ipfs/gateway.go
D:/inj/next-injective-git/cli/internal/ipfs/gateway_acceptance_test.go
D:/inj/next-injective-git/cli/internal/ipfs/gateway_test.go
D:/inj/next-injective-git/cli/internal/remote/helper.go
D:/inj/next-injective-git/cli/internal/remote/push_flow_test.go
D:/inj/next-injective-git/cli/internal/remote/url.go
D:/inj/next-injective-git/cli/internal/remote/url_test.go
D:/inj/next-injective-git/cli/internal/replication/client.go
D:/inj/next-injective-git/cli/internal/replication/client_test.go
D:/inj/next-injective-git/cli/internal/secrets/scan.go
D:/inj/next-injective-git/cli/internal/secrets/scan_test.go
D:/inj/next-injective-git/cli/internal/suitedeploy/artifact.go
D:/inj/next-injective-git/cli/internal/suitedeploy/deploy.go
D:/inj/next-injective-git/cli/internal/suitedeploy/deploy_test.go
D:/inj/next-injective-git/cli/internal/suitedeploy/history.go
D:/inj/next-injective-git/cli/internal/suitedeploy/recover.go
D:/inj/next-injective-git/cli/internal/suitedeploy/types.go
D:/inj/next-injective-git/cli/internal/suitedeploy/verify.go
D:/inj/next-injective-git/cli/internal/suitemigration/abi/BootstrapCoordinator.json
D:/inj/next-injective-git/cli/internal/suitemigration/codec.go
D:/inj/next-injective-git/cli/internal/suitemigration/manifest.go
D:/inj/next-injective-git/cli/internal/suitemigration/plan.go
D:/inj/next-injective-git/cli/internal/suitemigration/plan_test.go
D:/inj/next-injective-git/cli/internal/suitemigration/schema.go
D:/inj/next-injective-git/cli/internal/tunnel/process_unix.go
D:/inj/next-injective-git/cli/internal/tunnel/process_windows.go
D:/inj/next-injective-git/cli/internal/tunnel/tunnel.go
```

</details>

<details>
<summary>web：入场实际文件 99 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/web/.env.example
D:/inj/next-injective-git/web/.env.local
D:/inj/next-injective-git/web/.gitignore
D:/inj/next-injective-git/web/.vercel/node/package-manifest.json
D:/inj/next-injective-git/web/.vercel/output/builds.json
D:/inj/next-injective-git/web/.vercel/output/config.json
D:/inj/next-injective-git/web/.vercel/output/diagnostics/cli_traces.json
D:/inj/next-injective-git/web/.vercel/project.json
D:/inj/next-injective-git/web/api/ipfs-health.mjs
D:/inj/next-injective-git/web/api/upload-authorization.mjs
D:/inj/next-injective-git/web/components.json
D:/inj/next-injective-git/web/docs/frontend-improvements.md
D:/inj/next-injective-git/web/e2e/app.spec.ts
D:/inj/next-injective-git/web/e2e/app.spec.ts-snapshots/explorer-page-chromium-linux.png
D:/inj/next-injective-git/web/e2e/app.spec.ts-snapshots/home-page-chromium-linux.png
D:/inj/next-injective-git/web/e2e/app.spec.ts-snapshots/monitor-page-chromium-linux.png
D:/inj/next-injective-git/web/e2e/app.spec.ts-snapshots/settings-page-chromium-linux.png
D:/inj/next-injective-git/web/index.html
D:/inj/next-injective-git/web/package-lock.json
D:/inj/next-injective-git/web/package.json
D:/inj/next-injective-git/web/playwright.config.ts
D:/inj/next-injective-git/web/scripts/release-profile-check.mjs
D:/inj/next-injective-git/web/src/App.tsx
D:/inj/next-injective-git/web/src/components/AccountMenu.tsx
D:/inj/next-injective-git/web/src/components/CodeBlock.tsx
D:/inj/next-injective-git/web/src/components/ContractTypeBadge.tsx
D:/inj/next-injective-git/web/src/components/ErrorBoundary.tsx
D:/inj/next-injective-git/web/src/components/Markdown.tsx
D:/inj/next-injective-git/web/src/components/Toast.tsx
D:/inj/next-injective-git/web/src/components/WalletModal.tsx
D:/inj/next-injective-git/web/src/components/ui/alert.tsx
D:/inj/next-injective-git/web/src/components/ui/badge.tsx
D:/inj/next-injective-git/web/src/components/ui/button.tsx
D:/inj/next-injective-git/web/src/components/ui/card.tsx
D:/inj/next-injective-git/web/src/components/ui/separator.tsx
D:/inj/next-injective-git/web/src/components/ui/skeleton.tsx
D:/inj/next-injective-git/web/src/components/ui/table.tsx
D:/inj/next-injective-git/web/src/components/ui/tabs.tsx
D:/inj/next-injective-git/web/src/components/ui/tooltip.tsx
D:/inj/next-injective-git/web/src/index.css
D:/inj/next-injective-git/web/src/lib/WalletContext.tsx
D:/inj/next-injective-git/web/src/lib/abis.ts
D:/inj/next-injective-git/web/src/lib/activity.ts
D:/inj/next-injective-git/web/src/lib/address.ts
D:/inj/next-injective-git/web/src/lib/architecture-icons.ts
D:/inj/next-injective-git/web/src/lib/block-subscription.ts
D:/inj/next-injective-git/web/src/lib/chain.ts
D:/inj/next-injective-git/web/src/lib/cosmwasm-v1.ts
D:/inj/next-injective-git/web/src/lib/errors.ts
D:/inj/next-injective-git/web/src/lib/gitstore.ts
D:/inj/next-injective-git/web/src/lib/ipfs-probe.ts
D:/inj/next-injective-git/web/src/lib/modules.ts
D:/inj/next-injective-git/web/src/lib/profile.ts
D:/inj/next-injective-git/web/src/lib/registry.ts
D:/inj/next-injective-git/web/src/lib/search.ts
D:/inj/next-injective-git/web/src/lib/transport.ts
D:/inj/next-injective-git/web/src/lib/types.ts
D:/inj/next-injective-git/web/src/lib/utils.ts
D:/inj/next-injective-git/web/src/lib/wallet-icons.ts
D:/inj/next-injective-git/web/src/lib/wallet.ts
D:/inj/next-injective-git/web/src/lib/walletconnect.ts
D:/inj/next-injective-git/web/src/main.tsx
D:/inj/next-injective-git/web/src/pages/Archive.tsx
D:/inj/next-injective-git/web/src/pages/ArchiveOwner.tsx
D:/inj/next-injective-git/web/src/pages/Explorer.tsx
D:/inj/next-injective-git/web/src/pages/Home.tsx
D:/inj/next-injective-git/web/src/pages/IpfsExplorer.tsx
D:/inj/next-injective-git/web/src/pages/Monitor.tsx
D:/inj/next-injective-git/web/src/pages/Monitor/IpfsGatewayCard.tsx
D:/inj/next-injective-git/web/src/pages/Monitor/MetricCard.tsx
D:/inj/next-injective-git/web/src/pages/Monitor/PublicStorageProviderCard.tsx
D:/inj/next-injective-git/web/src/pages/Monitor/SourceCard.tsx
D:/inj/next-injective-git/web/src/pages/Monitor/badges.ts
D:/inj/next-injective-git/web/src/pages/Monitor/utils.ts
D:/inj/next-injective-git/web/src/pages/Owner.tsx
D:/inj/next-injective-git/web/src/pages/Repo/BlobView.tsx
D:/inj/next-injective-git/web/src/pages/Repo/CommitView.tsx
D:/inj/next-injective-git/web/src/pages/Repo/CommitsView.tsx
D:/inj/next-injective-git/web/src/pages/Repo/RefsTab.tsx
D:/inj/next-injective-git/web/src/pages/Repo/SponsorForm.tsx
D:/inj/next-injective-git/web/src/pages/Repo/SponsorsTab.tsx
D:/inj/next-injective-git/web/src/pages/Repo/TreeView.tsx
D:/inj/next-injective-git/web/src/pages/Repo/index.tsx
D:/inj/next-injective-git/web/src/pages/Repo/useRepoViews.ts
D:/inj/next-injective-git/web/src/pages/Settings.tsx
D:/inj/next-injective-git/web/src/styles.css
D:/inj/next-injective-git/web/src/vite-env.d.ts
D:/inj/next-injective-git/web/test/archive-v1.test.mjs
D:/inj/next-injective-git/web/test/evm-suite.test.mjs
D:/inj/next-injective-git/web/test/ipfs-browser-probe.test.mjs
D:/inj/next-injective-git/web/test/ipfs-health.test.mjs
D:/inj/next-injective-git/web/test/monitor.test.mjs
D:/inj/next-injective-git/web/test/release-profile-check.test.mjs
D:/inj/next-injective-git/web/test/search.test.mjs
D:/inj/next-injective-git/web/test/upload-authorization.test.mjs
D:/inj/next-injective-git/web/tsconfig.json
D:/inj/next-injective-git/web/tsconfig.tsbuildinfo
D:/inj/next-injective-git/web/vercel.json
D:/inj/next-injective-git/web/vite.config.ts
```

</details>

<details>
<summary>scripts：入场实际文件 66 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/scripts/archive-indexer-install.sh
D:/inj/next-injective-git/scripts/archive-indexer.sh
D:/inj/next-injective-git/scripts/archive-monitor-install.sh
D:/inj/next-injective-git/scripts/archive-monitor.env.example
D:/inj/next-injective-git/scripts/archive-monitor.sh
D:/inj/next-injective-git/scripts/bootstrap-push.ps1
D:/inj/next-injective-git/scripts/bootstrap-push.sh
D:/inj/next-injective-git/scripts/collect-deployment-metadata.ps1
D:/inj/next-injective-git/scripts/collect-deployment-metadata.sh
D:/inj/next-injective-git/scripts/durable-cid-receiver-install.sh
D:/inj/next-injective-git/scripts/durable-cid-sync-install.sh
D:/inj/next-injective-git/scripts/durable-cid-sync.sh
D:/inj/next-injective-git/scripts/evm-archive-indexer.sh
D:/inj/next-injective-git/scripts/evm-event-indexer.sh
D:/inj/next-injective-git/scripts/evm-hot-pin-indexer.sh
D:/inj/next-injective-git/scripts/evm-replication-reaper.sh
D:/inj/next-injective-git/scripts/evm-suite-solc-check.mjs
D:/inj/next-injective-git/scripts/evm-v2-check-test.sh
D:/inj/next-injective-git/scripts/evm-v2-check.sh
D:/inj/next-injective-git/scripts/evm-v2-foundry-abi-check.mjs
D:/inj/next-injective-git/scripts/evm-v2-solc-check.mjs
D:/inj/next-injective-git/scripts/feegrant-issue-test.sh
D:/inj/next-injective-git/scripts/feegrant-issue.sh
D:/inj/next-injective-git/scripts/feegrant-policy-gate-test.sh
D:/inj/next-injective-git/scripts/feegrant-policy-gate.sh
D:/inj/next-injective-git/scripts/feegrant-record-push-test.sh
D:/inj/next-injective-git/scripts/feegrant-record-push.sh
D:/inj/next-injective-git/scripts/feegrant-revoke.sh
D:/inj/next-injective-git/scripts/filebase-monitor.sh
D:/inj/next-injective-git/scripts/filone.env.example
D:/inj/next-injective-git/scripts/gateway-deploy.sh
D:/inj/next-injective-git/scripts/gateway-fallback-acceptance.sh
D:/inj/next-injective-git/scripts/gateway-ssh-harden.sh
D:/inj/next-injective-git/scripts/gateway-tls.sh
D:/inj/next-injective-git/scripts/gateway-us-deploy.sh
D:/inj/next-injective-git/scripts/hot-pin-indexer-install.sh
D:/inj/next-injective-git/scripts/hot-pin-indexer.sh
D:/inj/next-injective-git/scripts/identity-readiness.mjs
D:/inj/next-injective-git/scripts/migration-cutover-readiness-test.ps1
D:/inj/next-injective-git/scripts/migration-cutover-readiness-test.sh
D:/inj/next-injective-git/scripts/migration-cutover-readiness.ps1
D:/inj/next-injective-git/scripts/migration-cutover-readiness.sh
D:/inj/next-injective-git/scripts/monitor-install.sh
D:/inj/next-injective-git/scripts/pin-indexer-install.sh
D:/inj/next-injective-git/scripts/pin-indexer.sh
D:/inj/next-injective-git/scripts/race-check.sh
D:/inj/next-injective-git/scripts/readiness-file.ps1
D:/inj/next-injective-git/scripts/receive-durable-cids.sh
D:/inj/next-injective-git/scripts/replication-acceptance.sh
D:/inj/next-injective-git/scripts/replication-config-check-test.sh
D:/inj/next-injective-git/scripts/replication-config-check.sh
D:/inj/next-injective-git/scripts/replication-install.sh
D:/inj/next-injective-git/scripts/replication-monitor.sh
D:/inj/next-injective-git/scripts/replication-reaper-test.sh
D:/inj/next-injective-git/scripts/replication-reaper.sh
D:/inj/next-injective-git/scripts/replication.env.example
D:/inj/next-injective-git/scripts/semver-check.mjs
D:/inj/next-injective-git/scripts/semver-check.test.mjs
D:/inj/next-injective-git/scripts/suite-readiness.sh
D:/inj/next-injective-git/scripts/systemd/kubo-us.service
D:/inj/next-injective-git/scripts/testnet-demo-repo.sh
D:/inj/next-injective-git/scripts/testnet-e2e.sh
D:/inj/next-injective-git/scripts/validate-suite-cutover.mjs
D:/inj/next-injective-git/scripts/verify-metamask-tx.sh
D:/inj/next-injective-git/scripts/verify-release-assets.sh
D:/inj/next-injective-git/scripts/windows-suite-clean-check.ps1
```

</details>

<details>
<summary>docs：入场实际文件 35 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/docs/PRIORITY-MAPPING.md
D:/inj/next-injective-git/docs/README.md
D:/inj/next-injective-git/docs/a11-storage-indexer-v2.md
D:/inj/next-injective-git/docs/acceptance-evidence.md
D:/inj/next-injective-git/docs/adr/0001-evm-v2-runtime-and-migration-scope.md
D:/inj/next-injective-git/docs/adr/0002-pluggable-pack-storage.md
D:/inj/next-injective-git/docs/adr/0003-fresh-evm-suite-and-v1-archive-preview.md
D:/inj/next-injective-git/docs/architecture.md
D:/inj/next-injective-git/docs/archived-repositories.md
D:/inj/next-injective-git/docs/backlog.md
D:/inj/next-injective-git/docs/ci-web-publishing.md
D:/inj/next-injective-git/docs/delivery-roadmap.md
D:/inj/next-injective-git/docs/e2e-verification-summary.md
D:/inj/next-injective-git/docs/e2e-visual-verification-report.md
D:/inj/next-injective-git/docs/evm-v2-handoff.md
D:/inj/next-injective-git/docs/evm-v2-migration.md
D:/inj/next-injective-git/docs/evm-v2-repair-plan.md
D:/inj/next-injective-git/docs/evm-v2-repo-identity.md
D:/inj/next-injective-git/docs/evm-wallet-compatibility.md
D:/inj/next-injective-git/docs/feegrant-policy.md
D:/inj/next-injective-git/docs/frontend-improvement-analysis.md
D:/inj/next-injective-git/docs/gogs-frontend-redesign.md
D:/inj/next-injective-git/docs/infrastructure.md
D:/inj/next-injective-git/docs/keplr-acceptance.md
D:/inj/next-injective-git/docs/liveagent-evm-v2-context.md
D:/inj/next-injective-git/docs/monitor.md
D:/inj/next-injective-git/docs/open-questions.md
D:/inj/next-injective-git/docs/p0-evidence.md
D:/inj/next-injective-git/docs/pinning-infrastructure.md
D:/inj/next-injective-git/docs/project-knowledge-base-zh.md
D:/inj/next-injective-git/docs/project-status-zh.md
D:/inj/next-injective-git/docs/project-status.md
D:/inj/next-injective-git/docs/push-setup.md
D:/inj/next-injective-git/docs/release.md
D:/inj/next-injective-git/docs/target-topology-migration.md
```

</details>

<details>
<summary>evidence：入场实际文件 17 项（包含既有 artifacts/out/cache；排除依赖与dist）</summary>

```text
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/BATCH-IMPORT-GUIDE.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/LOCAL-TESTING.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/MANUAL-VERIFICATION-STEPS.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/NEXT-GOAL-REMINDER.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/PROGRESS-SUMMARY.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/TASK-COMPLETION-CHECKLIST.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/VERIFICATION-GUIDE.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/blockscout-verification.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/contract-addresses.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/cutover-scope.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/deployment-recovery-input.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/deployment-summary.md
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/deployment.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/empty-suite-activation-2026-08-29.log
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/empty-username-escrow-attestation.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/suite-runtime-verification-2026-08-29.json
D:/inj/next-injective-git/evidence/testnet-deployment-2026-08-25/suite-verification.json
```

</details>

文档入场字节哈希（用于证明原未提交内容完整保留）：

| 文件 | SHA-256 | 字节数 |
|---|---|---|
| docs/project-status-zh.md | f33c15c1887cdde85c167559d67e5aa21b2950eb26b9287712563c358ad99a8e | 5452 |
| docs/project-status.md | d0a5c36f792ca4a8a1487dcc05d3699e58f14c8ceb042eebb5475998f1f7014a | 22487 |
| docs/backlog.md | bc5ef40a14c0e1945bc22e84895e12a7151c807e9b6293c18fe65ae98d6e7932 | 7859 |
| docs/README.md | 113e33da15b0a74a0f7a7b293189bdb8039b924765100860f808be9847a797bb | 5257 |

## 附录 B. 本轮命令原始输出

<details>
<summary>solc：exit 0；2026-09-12T16:55:38.036Z → 2026-09-12T16:56:47.549Z</summary>

cwd=`D:/inj/next-injective-git/contracts/evm-v2`

```text
D:/Software/nodejs/node.exe D:/Software/nodejs/node_modules/npm/bin/npm-cli.js run check
```

```text
STDOUT:

> igit-evm-v2-contracts@0.0.0 check
> node ../../scripts/evm-suite-solc-check.mjs

Warning: Contract code size is 25537 bytes and exceeds 24576 bytes (a limit introduced in Spurious Dragon). This contract may not be deployable on Mainnet. Consider enabling the optimizer (with a low "runs" value!), turning off revert strings, or using libraries.
   --> test/SuiteArchitecture.t.sol:557:1:
    |
557 | contract SuiteProtocolParityTest is SuiteArchitectureTestBase {
    | ^ (Relevant source part starts here and spans across multiple lines).


SuiteDirectory: initcode=4059 runtime=3745
BootstrapCoordinator: initcode=4968 runtime=4478
RepositoryCore: initcode=23925 runtime=23504
RecoveryModule: initcode=9350 runtime=9006
ModerationModule: initcode=16862 runtime=16383
EconomicModule: initcode=10008 runtime=9476
UsernameModule: initcode=8800 runtime=8417
BadgeModule: initcode=6967 runtime=6653
ReleaseModule: initcode=6589 runtime=6227
SuiteArchitectureTest: initcode=22604 runtime=22576
SuiteArchitectureSecurityTest: initcode=13573 runtime=13546
EVM SUITE SOLC CHECK: PASS

STDERR:
npm notice
npm notice New minor version of npm available! 11.6.2 -> 11.19.1
npm notice Changelog: https://github.com/npm/cli/releases/tag/v11.19.1
npm notice To update run: npm install -g npm@11.19.1
npm notice

```

</details>

<details>
<summary>go：exit 0；2026-09-12T16:58:09.361Z → 2026-09-12T16:58:16.572Z</summary>

cwd=`D:/inj/next-injective-git/cli`

```text
C:/Program Files/Go/bin/go.exe test -count=1 ./...
```

```text
STDOUT:
?   	github.com/Hny0305Lin/next-injective-git/cli/bin/archive-audit	[no test files]
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/derive-address-demo	[no test files]
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/evm-activate-suite	0.353s
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/evm-demo-bootstrap	[no test files]
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/evm-demo-deploy-rest	[no test files]
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/evm-demo-inspect	[no test files]
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/evm-key-import-demo	[no test files]
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/git-remote-igit	0.393s
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit	1.118s
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit-deploy-suite	0.536s
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit-release-profile-check	[no test files]
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit-replicationd	0.579s
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit-suite-migrate	0.424s
ok  	github.com/Hny0305Lin/next-injective-git/cli/cmd/igit-suite-operator	0.860s
?   	github.com/Hny0305Lin/next-injective-git/cli/cmd/simple-key-import	[no test files]
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/archivev1	0.370s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/bootstrap	1.050s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/chain	5.380s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/config	0.355s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/environment	2.027s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/fileprotection	0.275s
?   	github.com/Hny0305Lin/next-injective-git/cli/internal/gitio	[no test files]
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/i18n	0.256s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/ipfs	0.586s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/remote	0.365s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/replication	0.821s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/secrets	0.228s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/suitedeploy	0.758s
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/suitemigration	0.315s
?   	github.com/Hny0305Lin/next-injective-git/cli/internal/tunnel	[no test files]

STDERR:

```

</details>

<details>
<summary>vet：exit 0；2026-09-12T16:58:09.363Z → 2026-09-12T16:58:10.379Z</summary>

cwd=`D:/inj/next-injective-git/cli`

```text
C:/Program Files/Go/bin/go.exe vet ./...
```

```text
STDOUT:

STDERR:

```

</details>

<details>
<summary>typecheck：exit 0；2026-09-12T16:55:38.047Z → 2026-09-12T16:55:41.482Z</summary>

cwd=`D:/inj/next-injective-git/web`

```text
D:/Software/nodejs/node.exe D:/Software/nodejs/node_modules/npm/bin/npm-cli.js run typecheck
```

```text
STDOUT:

> igit-web@0.1.0 typecheck
> tsc -b --pretty false


STDERR:

```

</details>

<details>
<summary>api：exit 0；2026-09-12T16:55:41.482Z → 2026-09-12T16:55:44.858Z</summary>

cwd=`D:/inj/next-injective-git/web`

```text
D:/Software/nodejs/node.exe D:/Software/nodejs/node_modules/npm/bin/npm-cli.js run test:api
```

```text
STDOUT:

> igit-web@0.1.0 test:api
> node --import tsx --test test/*.test.mjs

✔ repository breadcrumbs support both EVM and V1 archive route bases (0.7739ms)
✔ V1 smart queries use a frozen latest height and never a write method (1.3374ms)
✔ V1 smart queries retry one transient network failure (207.4204ms)
✔ V1 repository and ref queries map protocol fields and paginate (4.2908ms)
✔ V1 pagination fails closed when a full page does not advance (3.1389ms)
✔ V1 archive rejects oversized LCD responses before parsing (0.3305ms)
✔ V1 queries fail over to the community LCD when the sentry LCD drops requests (814.6497ms)
✔ V1 snapshot height falls back when every primary block query fails (0.4968ms)
✔ V1 queries report every tried LCD when all endpoints are unreachable (1626.0372ms)
✔ built-in profile has one empty SuiteDirectory and ignores legacy stored fields (0.9264ms)
✔ an explicit local SuiteDirectory override round-trips without restoring legacy module fields (0.1691ms)
✔ SuiteDirectory editing is limited to local deployment origins (0.3024ms)
✔ public Settings keeps the Directory copy-only and both write actions disabled (6.0414ms)
✔ wallet chain setup adds Injective testnet when the wallet does not know it (0.3968ms)
✔ native INJ balance does not depend on SuiteDirectory readiness (9.4378ms)
✔ unconfigured Directory fails closed before any network or wallet request (0.6417ms)
✔ Suite verification binds chain, active version, code hashes, and all seven modules at one block (13.9397ms)
✔ Suite verification rejects code hash and binding mismatches (11.6001ms)
✔ activity log reads never exceed the RPC [from, to] block distance cap (4.345ms)
✔ activity stops requesting older spans once the limit is filled (6.6448ms)
✔ activity halves a span when the provider still reports a range cap (6.0938ms)
✔ activity surfaces non-range RPC failures instead of splitting forever (4.0534ms)
✔ registry reads use viem tuple decoding and stable repo IDs (5.7167ms)
✔ wallet writes estimate gas and broadcast an explicit legacy transaction at Injective minimum gasPrice (6.5811ms)
✔ revenue split writes accept the V1 twenty-recipient limit and reject twenty-one (4.4474ms)
✔ metadata writes encode V3 repoId patch flags instead of a locator (4.0186ms)
✔ ownership-transfer rejection uses RepositoryCore cancellation (7.8237ms)
✔ a mined status zero receipt is a failure and never falls back (3.0089ms)
✔ receipt transport failures return a typed uncertain error carrying the tx hash (3.5409ms)
✔ ordinary Web runtime contains no V1 fallback or handwritten ABI selector table (3.6554ms)
✔ wallet connect is independent from Suite verification and keeps failures visible (0.7336ms)
✔ WalletConnect uses a custom URI surface and preserves the EIP-1193 session boundary (0.4492ms)
✔ WalletConnect chain incompatibility remains actionable and does not expose raw logger objects (0.3289ms)
✔ wallet chain IDs accept numeric WalletConnect responses and hex injected responses (0.096ms)
✔ WalletConnect QR entry is hidden at the existing mobile breakpoint (0.3689ms)
✔ WalletConnect brand icon is bundled for offline rendering (0.323ms)
✔ EIP-6963 announcements are matched by wallet RDNS (0.4049ms)
✔ late EIP-6963 announcements update availability without replacing a selected family (0.1358ms)
✔ provider error codes normalize numeric, string, and nested EIP-1193 shapes (0.14ms)
✔ wallet chain setup only adds a chain for normalized unknown-chain errors (0.1318ms)
✔ EIP-6963 resolution covers every announced EVM wallet and pins provider identity (0.3082ms)
✔ same-brand provider ambiguity is deterministic for connect but blocked for silent restore (4.5357ms)
✔ legacy fallback never chooses an arbitrary provider when a vendor flag is ambiguous (0.1793ms)
✔ wallet metadata uses the requested brand icons and keeps Compass unchanged (0.0918ms)
✔ Keplr EVM preparation uses only the documented provider enable hook (0.0659ms)
✔ wallet context pins events to the selected provider and marks wrong chains unwritable (1.1148ms)
✔ browser gateway probe uses the median of three sequential responses (1.8243ms)
✔ browser gateway probe degrades partial results without hiding the median (0.5028ms)
✔ browser gateway probe does not invent latency when every request fails (1.0841ms)
✔ median latency handles odd, even, and empty samples (0.1154ms)
✔ handler probes the default fixed gateway and returns JSON status (1.7654ms)
✔ handler falls back from HEAD 405 to a bounded GET (0.9846ms)
✔ profile and target inputs cannot select an arbitrary upstream (0.9645ms)
✔ the fixed US gateway is selected without accepting a URL (0.1403ms)
✔ probe aborts and reports a timeout without leaking upstream details (18.814ms)
✔ handler answers CORS preflight and rejects writes (0.3415ms)
✔ public Monitor is routed and suppresses the duplicate global Suite alert (3.3633ms)
✔ Monitor composes shadcn primitives and remains wallet independent (0.8136ms)
✔ Monitor exposes V1 status without enumerating or ranking V1 repositories (0.4912ms)
✔ Monitor activity retains an explicit bounded observation window (0.6722ms)
✔ Monitor keeps HK and US in one IPFS gateway surface (0.6199ms)
✔ Monitor keeps Filebase and Fil.one as provider metadata (0.5849ms)
✔ Monitor measures each gateway directly from the browser using three-sample medians (1.0277ms)
✔ Monitor automatically refreshes all probes every 90 seconds (0.4595ms)
✔ V1 archive source includes the public Injective contract explorer (0.137ms)
✔ checked-in Web SuiteDirectory profile passes the AST gate and CLI (223.1434ms)
✔ release profile gate rejects a missing default (0.7638ms)
✔ release profile gate rejects a Directory before evidence approval (0.2687ms)
✔ release profile gate requires a static Directory field (0.208ms)
✔ global search normalizes schemes and display-prefixed usernames (2.0415ms)
✔ global search encodes route segments and ignores extra path segments (0.2394ms)
✔ resource errors explain Suite configuration failures (0.4454ms)
✔ issues a short-lived Ed25519 identity token (3.9374ms)
ℹ tests 73
ℹ suites 0
ℹ pass 73
ℹ fail 0
ℹ cancelled 0
ℹ skipped 0
ℹ todo 0
ℹ duration_ms 3147.0882

STDERR:

```

</details>

<details>
<summary>build：exit 0；2026-09-12T16:55:44.858Z → 2026-09-12T16:55:54.423Z</summary>

cwd=`D:/inj/next-injective-git/web`

```text
D:/Software/nodejs/node.exe D:/Software/nodejs/node_modules/npm/bin/npm-cli.js run build -- --outDir node_modules/.cache/reconciliation-20260913-0054 --emptyOutDir false
```

```text
STDOUT:

> igit-web@0.1.0 build
> tsc -b && vite build --outDir node_modules/.cache/reconciliation-20260913-0054 --emptyOutDir false

vite v5.4.21 building for production...
transforming...
✓ 4222 modules transformed.
rendering chunks...
computing gzip size...
node_modules/.cache/reconciliation-20260913-0054/index.html                                              0.98 kB │ gzip:   0.53 kB
node_modules/.cache/reconciliation-20260913-0054/assets/geist-cyrillic-ext-wght-normal-DjL33-gN.woff2    7.42 kB
node_modules/.cache/reconciliation-20260913-0054/assets/geist-vietnamese-wght-normal-6IgcOCM7.woff2      8.00 kB
node_modules/.cache/reconciliation-20260913-0054/assets/geist-cyrillic-wght-normal-BEAKL7Jp.woff2       15.08 kB
node_modules/.cache/reconciliation-20260913-0054/assets/geist-latin-ext-wght-normal-DC-KSUi6.woff2      16.51 kB
node_modules/.cache/reconciliation-20260913-0054/assets/geist-latin-wght-normal-BgDaEnEv.woff2          29.40 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-CYLfohn-.css                             102.22 kB │ gzip:  18.40 kB
node_modules/.cache/reconciliation-20260913-0054/assets/info-DjHu9PL1.js                                 0.37 kB │ gzip:   0.28 kB
node_modules/.cache/reconciliation-20260913-0054/assets/award-B8oVgPIN.js                                0.45 kB │ gzip:   0.33 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCaretLeft-_aiyH7n1.js                          1.88 kB │ gzip:   0.92 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCaretDown-78IuKU3V.js                          1.94 kB │ gzip:   0.95 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCaretRight-8zJQEuDX.js                         1.96 kB │ gzip:   0.94 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCaretUp-CiCxKWwN.js                            2.02 kB │ gzip:   0.94 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowUpRight-BpJjRRRh.js                       2.06 kB │ gzip:   1.02 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowDown-C4hT9BVu.js                          2.10 kB │ gzip:   1.01 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowRight-Cp5ZME0F.js                         2.11 kB │ gzip:   1.01 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhPlus-BMjcHjsT.js                               2.11 kB │ gzip:   0.97 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCheck-CcADx8br.js                              2.11 kB │ gzip:   0.98 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowLeft-Way4Cu-r.js                          2.12 kB │ gzip:   1.00 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowUp-Dph6j8rm.js                            2.17 kB │ gzip:   1.01 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhMagnifyingGlass-Mygfryvz.js                    2.21 kB │ gzip:   1.07 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhBrowser-dUu2TrC1.js                            2.22 kB │ gzip:   0.97 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhDotsThree-BMXCQl-0.js                          2.25 kB │ gzip:   0.94 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhFunnelSimple-Ba7BbWTp.js                       2.36 kB │ gzip:   1.00 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCopy-C-PDgSlZ.js                               2.36 kB │ gzip:   1.01 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhClock-UItiXeJq.js                              2.37 kB │ gzip:   1.03 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhWarningCircle-BqjT745H.js                      2.43 kB │ gzip:   1.05 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhX-s-SmVExa.js                                  2.50 kB │ gzip:   1.06 kB
node_modules/.cache/reconciliation-20260913-0054/assets/ArchiveOwner-C8Ls_KYM.js                         2.55 kB │ gzip:   1.06 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhDeviceMobile-BdsPaATg.js                       2.57 kB │ gzip:   1.07 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowClockwise-CJPxQhr_.js                     2.59 kB │ gzip:   1.19 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhUser-CAgZSpGe.js                               2.68 kB │ gzip:   1.26 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhEnvelope-yUsis6yU.js                           2.68 kB │ gzip:   1.18 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCircleHalf-BYzSzd2h.js                         2.71 kB │ gzip:   1.21 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhSignOut-C5YOWlPT.js                            2.77 kB │ gzip:   1.16 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhInfo-Cr4DhVUE.js                               2.78 kB │ gzip:   1.21 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhPower-CGffGmdb.js                              2.78 kB │ gzip:   1.28 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhDesktop-BBIPrYLl.js                            2.79 kB │ gzip:   1.12 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowsLeftRight-CuLpbKRk.js                    2.80 kB │ gzip:   1.18 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowCircleDown-B305t-Sh.js                    2.81 kB │ gzip:   1.18 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhWallet-w25D16It.js                             2.83 kB │ gzip:   1.18 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhQuestionMark-BP9EMB6h.js                       2.84 kB │ gzip:   1.30 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowSquareOut-NnrK69c5.js                     2.93 kB │ gzip:   1.25 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowsDownUp-BjBI4ckC.js                       2.93 kB │ gzip:   1.18 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCreditCard-CWRaUT6F.js                         2.93 kB │ gzip:   1.16 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhBank-Dchr4XQJ.js                               2.94 kB │ gzip:   1.25 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhVault-l2PHF38a.js                              2.97 kB │ gzip:   1.20 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCompass-CUy3uA7t.js                            2.98 kB │ gzip:   1.28 kB
node_modules/.cache/reconciliation-20260913-0054/assets/ccip-hwxIgkK5.js                                 3.06 kB │ gzip:   1.40 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhPaperPlaneRight-BeuCA660.js                    3.10 kB │ gzip:   1.42 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhTrash-Biq6rmmL.js                              3.12 kB │ gzip:   1.19 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhCurrencyDollar-Cqla3AXT.js                     3.24 kB │ gzip:   1.33 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhQuestion-M09j2U0T.js                           3.32 kB │ gzip:   1.45 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhImage-BAJ_erz6.js                              3.44 kB │ gzip:   1.45 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhArrowsClockwise-CFUT8X7R.js                    3.87 kB │ gzip:   1.59 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhWarning-DPEydIPw.js                            3.91 kB │ gzip:   1.49 kB
node_modules/.cache/reconciliation-20260913-0054/assets/Owner-SRGMeW2u.js                                4.13 kB │ gzip:   1.51 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhIdentificationCard-Czjmq9Z3.js                 4.22 kB │ gzip:   1.65 kB
node_modules/.cache/reconciliation-20260913-0054/assets/Archive-BedOQ7jq.js                              4.34 kB │ gzip:   1.59 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhSpinner-BXooDYzU.js                            4.45 kB │ gzip:   1.65 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhLightbulb-BX_7duTr.js                          4.92 kB │ gzip:   1.86 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhQrCode-DRwRzXCH.js                             4.92 kB │ gzip:   1.47 kB
node_modules/.cache/reconciliation-20260913-0054/assets/IpfsExplorer-AzkvTfMp.js                         5.21 kB │ gzip:   2.04 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhPuzzlePiece-wm_GpVSi.js                        5.37 kB │ gzip:   1.96 kB
node_modules/.cache/reconciliation-20260913-0054/assets/Explorer-CKUYqBp9.js                             5.56 kB │ gzip:   2.12 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhGlobe-DgLxBk5U.js                              5.72 kB │ gzip:   1.92 kB
node_modules/.cache/reconciliation-20260913-0054/assets/features-Cki7Z7Xp.js                             5.97 kB │ gzip:   2.37 kB
node_modules/.cache/reconciliation-20260913-0054/assets/cosmwasm-v1-Dg8YGnCb.js                          6.85 kB │ gzip:   2.63 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-XV0DxkG5.js                                7.89 kB │ gzip:   3.22 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-COg4JOUG.js                                9.21 kB │ gzip:   3.53 kB
node_modules/.cache/reconciliation-20260913-0054/assets/PhSealCheck-BfG80yKn.js                         12.10 kB │ gzip:   4.02 kB
node_modules/.cache/reconciliation-20260913-0054/assets/property-C9JG5tag.js                            17.92 kB │ gzip:   6.53 kB
node_modules/.cache/reconciliation-20260913-0054/assets/Monitor-BeXX4fY9.js                             54.66 kB │ gzip:  16.56 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-DxMRRvEy.js                               98.59 kB │ gzip:  27.21 kB
node_modules/.cache/reconciliation-20260913-0054/assets/basic-BesZ6wb7.js                              108.35 kB │ gzip:  28.53 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-BNHftLFe.js                              109.98 kB │ gzip:  34.67 kB
node_modules/.cache/reconciliation-20260913-0054/assets/gitstore-C5XIKdon.js                           158.94 kB │ gzip:  52.13 kB
node_modules/.cache/reconciliation-20260913-0054/assets/w3m-modal-CvI7DPQw.js                          169.18 kB │ gzip:  35.64 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-CdM_Nu4G.js                              469.95 kB │ gzip: 144.21 kB
node_modules/.cache/reconciliation-20260913-0054/assets/core-BRPj90WC.js                               630.47 kB │ gzip: 186.43 kB
node_modules/.cache/reconciliation-20260913-0054/assets/index-CXN_Q3vR.js                              738.37 kB │ gzip: 235.72 kB
✓ built in 6.73s

STDERR:
node_modules/@walletconnect/utils/node_modules/ox/_esm/core/Base64.js (6:27): A comment

"/*#__PURE__*/"

in "node_modules/@walletconnect/utils/node_modules/ox/_esm/core/Base64.js" contains an annotation that Rollup cannot interpret due to the position of the comment. The comment will be removed to avoid issues.
node_modules/@reown/appkit/node_modules/ox/_esm/core/Base64.js (6:27): A comment

"/*#__PURE__*/"

in "node_modules/@reown/appkit/node_modules/ox/_esm/core/Base64.js" contains an annotation that Rollup cannot interpret due to the position of the comment. The comment will be removed to avoid issues.
node_modules/@reown/appkit-controllers/node_modules/ox/_esm/core/Base64.js (6:27): A comment

"/*#__PURE__*/"

in "node_modules/@reown/appkit-controllers/node_modules/ox/_esm/core/Base64.js" contains an annotation that Rollup cannot interpret due to the position of the comment. The comment will be removed to avoid issues.

(!) Some chunks are larger than 500 kB after minification. Consider:
- Using dynamic import() to code-split the application
- Use build.rollupOptions.output.manualChunks to improve chunking: https://rollupjs.org/configuration-options/#output-manualchunks
- Adjust chunk size limit for this warning via build.chunkSizeWarningLimit.

```

</details>

<details>
<summary>cliCodes：exit 0；2026-09-12T17:04:37.667Z → 2026-09-12T17:04:46.119Z</summary>

cwd=`D:/inj/next-injective-git/cli`

```text
C:/Program Files/Go/bin/go.exe run ./cmd/evm-demo-inspect -directory 0x24124cb60f9ef02f7deb5bc868c028fb412f5334 -rpc https://k8s.testnet.json-rpc.injective.network -artifacts D:/inj/next-injective-git/contracts/evm-v2/artifacts -code-addresses 0x24124cb60f9ef02f7deb5bc868c028fb412f5334,0x329921023FCf6E337E924686b92f7521549B5970,0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4,0xa7249cE20B54C4Be440838F0eF403d2a6a07E532,0xb957B65634931dD0613abAC296D5a3837BF979Bd,0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd,0xc7C164E46788b37e98D27aCe908C489999c09853,0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D,0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f
```

```text
STDOUT:
address=0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334
code_bytes=3745
code_hash=0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd
address=0x329921023FCf6E337E924686b92f7521549B5970
code_bytes=4478
code_hash=0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7
address=0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4
code_bytes=23504
code_hash=0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467
address=0xa7249cE20B54C4Be440838F0eF403d2a6a07E532
code_bytes=9006
code_hash=0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2
address=0xb957B65634931dD0613abAC296D5a3837BF979Bd
code_bytes=16383
code_hash=0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3
address=0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd
code_bytes=9476
code_hash=0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d
address=0xc7C164E46788b37e98D27aCe908C489999c09853
code_bytes=8417
code_hash=0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e
address=0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D
code_bytes=6653
code_hash=0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c
address=0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f
code_bytes=6227
code_hash=0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c

STDERR:

```

</details>

<details>
<summary>cliSuite：exit 1；2026-09-12T16:59:02.649Z → 2026-09-12T16:59:03.122Z</summary>

cwd=`D:/inj/next-injective-git/cli`

```text
go run ./cmd/igit suite verify --json
```

```text
STDOUT:

STDERR:
igit: protect config directory before read: set sensitive-file DACL: Access is denied.
exit status 1

```

</details>

<details>
<summary>kuboSkip：exit 0；2026-09-12T17:02:13.361Z → 2026-09-12T17:02:14.314Z</summary>

cwd=`D:/inj/next-injective-git/cli`

```text
C:/Program Files/Go/bin/go.exe test -count=1 -v ./internal/bootstrap -run ^TestNativeKuboLifecycleIntegration$
```

```text
STDOUT:
=== RUN   TestNativeKuboLifecycleIntegration
    bootstrap_integration_test.go:27: set IGIT_RUN_NATIVE_KUBO_INTEGRATION=1 to run the native Kubo lifecycle smoke test
--- SKIP: TestNativeKuboLifecycleIntegration (0.00s)
PASS
ok  	github.com/Hny0305Lin/next-injective-git/cli/internal/bootstrap	0.326s

STDERR:

```

</details>

### 启动失败与补充结果

最早辅助PowerShell未继承交互shell的Go PATH，npm.ps1因未设置LASTEXITCODE报错，即使exit=0也**未计PASS**。显式工具路径初次Go环境产生 `null\go\pkg\mod` 相对路径错误；补齐USERPROFILE/GOPATH/GOMODCACHE后，实际测试按上表完整运行通过。没有改测试、ABI或证据修复这些环境启动失败。

```text
forge: command not found in current PATH
ipfs: command not found in current PATH

```

```json
[
  {
    "at": "2026-09-12T17:02:12.475Z",
    "url": "http://127.0.0.1:5001/api/v0/version",
    "status": "BLOCKED",
    "error": "ECONNREFUSED"
  },
  {
    "at": "2026-09-12T17:02:12.476Z",
    "url": "http://127.0.0.1:5001/api/v0/pin/ls?type=recursive",
    "status": "BLOCKED",
    "error": "ECONNREFUSED"
  }
]
```

完整evm-demo-inspect于2026-09-13 01:01:40 +08:00启动，初始block=0x855fc13；Directory字段及core/recovery/moderation/economic/部分username查询返回，最后输出：

```text
coordinator.moduleProgress(username): EVM RPC eth_call transport: read response: context deadline exceeded
exit status 1
```

此命令包含latest调用且未全部完成，不计完整Suite验证通过。后续code-addresses限定模式退出0的完整输出已保存于上方。

## 附录 C. 静态 ABI 计算、fixture 和代码位置

```json
{
  "topic0": "0x172cc57eb3f906b5e4bcf3a85861062c792de8d5d56754a3785d99057dec948a",
  "event": {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "repoId",
        "type": "bytes32"
      },
      {
        "indexed": true,
        "internalType": "string",
        "name": "refName",
        "type": "string"
      },
      {
        "indexed": false,
        "internalType": "string",
        "name": "commitSha",
        "type": "string"
      },
      {
        "indexed": false,
        "internalType": "string[]",
        "name": "packUris",
        "type": "string[]"
      },
      {
        "indexed": true,
        "internalType": "address",
        "name": "updatedBy",
        "type": "address"
      }
    ],
    "name": "RefUpdated",
    "type": "event"
  },
  "coreModuleId": "0x0f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8",
  "moduleAddressSelector": "0xaa10e9f0",
  "functions": [
    {
      "name": "getRef",
      "exists": true,
      "abi": {
        "inputs": [
          {
            "internalType": "bytes32",
            "name": "repoId",
            "type": "bytes32"
          },
          {
            "internalType": "string",
            "name": "refName",
            "type": "string"
          }
        ],
        "name": "getRef",
        "outputs": [
          {
            "components": [
              {
                "internalType": "string",
                "name": "commitSha",
                "type": "string"
              },
              {
                "internalType": "string[]",
                "name": "packUris",
                "type": "string[]"
              },
              {
                "internalType": "uint64",
                "name": "updatedAt",
                "type": "uint64"
              },
              {
                "internalType": "address",
                "name": "updatedBy",
                "type": "address"
              },
              {
                "internalType": "bool",
                "name": "exists",
                "type": "bool"
              }
            ],
            "internalType": "struct RepositoryCore.GitRef",
            "name": "",
            "type": "tuple"
          }
        ],
        "stateMutability": "view",
        "type": "function"
      }
    },
    {
      "name": "listRepositoriesPage",
      "exists": true,
      "abi": {
        "inputs": [
          {
            "internalType": "address",
            "name": "owner",
            "type": "address"
          },
          {
            "internalType": "uint256",
            "name": "cursor",
            "type": "uint256"
          },
          {
            "internalType": "uint256",
            "name": "limit",
            "type": "uint256"
          }
        ],
        "name": "listRepositoriesPage",
        "outputs": [
          {
            "components": [
              {
                "internalType": "bytes32",
                "name": "id",
                "type": "bytes32"
              },
              {
                "internalType": "address",
                "name": "owner",
                "type": "address"
              },
              {
                "internalType": "string",
                "name": "name",
                "type": "string"
              },
              {
                "internalType": "string",
                "name": "description",
                "type": "string"
              },
              {
                "internalType": "string",
                "name": "defaultBranch",
                "type": "string"
              },
              {
                "internalType": "bytes32",
                "name": "forkedFrom",
                "type": "bytes32"
              },
              {
                "internalType": "uint64",
                "name": "createdAt",
                "type": "uint64"
              },
              {
                "internalType": "uint64",
                "name": "updatedAt",
                "type": "uint64"
              },
              {
                "internalType": "bool",
                "name": "exists",
                "type": "bool"
              }
            ],
            "internalType": "struct IRepositoryCore.Repository[]",
            "name": "page",
            "type": "tuple[]"
          },
          {
            "internalType": "uint256",
            "name": "nextCursor",
            "type": "uint256"
          }
        ],
        "stateMutability": "view",
        "type": "function"
      }
    },
    {
      "name": "listRefsPage",
      "exists": true,
      "abi": {
        "inputs": [
          {
            "internalType": "bytes32",
            "name": "repoId",
            "type": "bytes32"
          },
          {
            "internalType": "uint256",
            "name": "cursor",
            "type": "uint256"
          },
          {
            "internalType": "uint256",
            "name": "limit",
            "type": "uint256"
          }
        ],
        "name": "listRefsPage",
        "outputs": [
          {
            "internalType": "string[]",
            "name": "names",
            "type": "string[]"
          },
          {
            "components": [
              {
                "internalType": "string",
                "name": "commitSha",
                "type": "string"
              },
              {
                "internalType": "string[]",
                "name": "packUris",
                "type": "string[]"
              },
              {
                "internalType": "uint64",
                "name": "updatedAt",
                "type": "uint64"
              },
              {
                "internalType": "address",
                "name": "updatedBy",
                "type": "address"
              },
              {
                "internalType": "bool",
                "name": "exists",
                "type": "bool"
              }
            ],
            "internalType": "struct RepositoryCore.GitRef[]",
            "name": "refs",
            "type": "tuple[]"
          },
          {
            "internalType": "uint256",
            "name": "nextCursor",
            "type": "uint256"
          }
        ],
        "stateMutability": "view",
        "type": "function"
      }
    },
    {
      "name": "repositoryCount",
      "exists": false
    },
    {
      "name": "repositoryIdAt",
      "exists": false
    }
  ],
  "copiesMatch": [
    {
      "name": "SuiteDirectory",
      "matches": true
    },
    {
      "name": "BootstrapCoordinator",
      "matches": true
    },
    {
      "name": "RepositoryCore",
      "matches": true
    },
    {
      "name": "RecoveryModule",
      "matches": true
    },
    {
      "name": "ModerationModule",
      "matches": true
    },
    {
      "name": "EconomicModule",
      "matches": true
    },
    {
      "name": "UsernameModule",
      "matches": true
    },
    {
      "name": "BadgeModule",
      "matches": true
    },
    {
      "name": "ReleaseModule",
      "matches": true
    }
  ]
}
```

<details>
<summary>本地合成 ABI fixture：仅证明解码差异，不是真实CID/交易</summary>

```json
{
  "classification": "local synthetic ABI fixture, not real CID/transaction",
  "input": {
    "repoId": "0x1111111111111111111111111111111111111111111111111111111111111111",
    "refName": "refs/heads/audit-fixture",
    "commitSha": "abababababababababababababababababababab",
    "packUris": [
      "ipfs://bafybeiauditfixtureone",
      "ipfs://bafybeiauditfixturetwo"
    ],
    "updatedBy": "0x2222222222222222222222222222222222222222"
  },
  "topics": [
    "0x172cc57eb3f906b5e4bcf3a85861062c792de8d5d56754a3785d99057dec948a",
    "0x1111111111111111111111111111111111111111111111111111111111111111",
    "0x3d92ef4a7b2fde56e31f73e46880cd5967fdd7e2d313b7445028221a9ecd6b9a",
    "0x0000000000000000000000002222222222222222222222222222222222222222"
  ],
  "data": "0x000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000a0000000000000000000000000000000000000000000000000000000000000002861626162616261626162616261626162616261626162616261626162616261626162616261626162000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000001d697066733a2f2f626166796265696175646974666978747572656f6e65000000000000000000000000000000000000000000000000000000000000000000001d697066733a2f2f6261667962656961756469746669787475726574776f000000",
  "decoded": {
    "eventName": "RefUpdated",
    "args": {
      "repoId": "0x1111111111111111111111111111111111111111111111111111111111111111",
      "refName": "0x3d92ef4a7b2fde56e31f73e46880cd5967fdd7e2d313b7445028221a9ecd6b9a",
      "updatedBy": "0x2222222222222222222222222222222222222222",
      "commitSha": "abababababababababababababababababababab",
      "packUris": [
        "ipfs://bafybeiauditfixtureone",
        "ipfs://bafybeiauditfixturetwo"
      ]
    }
  },
  "scriptRegexMatches": [],
  "status": "FAIL (script parser); PASS (viem current ABI round-trip)"
}
```

</details>

```json
[
  {
    "path": "scripts/evm-event-indexer.sh",
    "matches": [
      {
        "line": 15,
        "text": "REF_UPDATED_TOPIC=\"0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9\""
      },
      {
        "line": 61,
        "text": "    core_id=\"0x$(echo -n \"$core_id\" | sha256sum | cut -d' ' -f1)\""
      },
      {
        "line": 63,
        "text": "    local calldata=\"0x40a3d246${core_id:2}\"  # moduleAddress(bytes32)"
      },
      {
        "line": 74,
        "text": "        jq -r '.last_indexed_block // \"0\"' \"$INDEXER_STATE\""
      },
      {
        "line": 81,
        "text": "save_checkpoint() {"
      },
      {
        "line": 89,
        "text": "        '{last_indexed_block: $block, total_processed: $processed, updated_at: $timestamp}' \\"
      },
      {
        "line": 132,
        "text": "    # This is a simplified placeholder - use proper ABI decoder in production"
      },
      {
        "line": 133,
        "text": "    ref_name=$(echo \"$data\" | cut -c67-130 | xxd -r -p 2>/dev/null || echo \"\")"
      },
      {
        "line": 182,
        "text": "    checkpoint=$(load_checkpoint)"
      },
      {
        "line": 186,
        "text": "        checkpoint=\"$from_block\""
      },
      {
        "line": 217,
        "text": "        checkpoint=\"$next_end\""
      },
      {
        "line": 221,
        "text": "            save_checkpoint \"$checkpoint\" \"$total_processed\""
      },
      {
        "line": 226,
        "text": "            save_checkpoint \"$checkpoint\" \"$total_processed\""
      }
    ]
  },
  {
    "path": "scripts/evm-hot-pin-indexer.sh",
    "matches": [
      {
        "line": 18,
        "text": "REF_UPDATED_TOPIC=\"0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9\""
      },
      {
        "line": 35,
        "text": "    local core_id=\"0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)\""
      },
      {
        "line": 36,
        "text": "    local calldata=\"0x40a3d246${core_id:2}\"  # moduleAddress(bytes32)"
      },
      {
        "line": 65,
        "text": "# Extract CIDs from event data (simplified - production needs proper ABI decoder)"
      },
      {
        "line": 73,
        "text": "    # This is simplified - use proper ABI decoder in production"
      },
      {
        "line": 74,
        "text": "    echo \"$data\" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||' | while read -r cid; do"
      },
      {
        "line": 109,
        "text": "        local logs=$(query_ref_events \"$current\" \"$next\" \"$core_address\")"
      },
      {
        "line": 164,
        "text": "        ipfs pin add --progress \"$cid\" || echo \"Failed to pin $cid\" >&2"
      },
      {
        "line": 173,
        "text": "                ipfs pin rm \"$cid\" || echo \"Failed to unpin $cid\" >&2"
      }
    ]
  },
  {
    "path": "scripts/evm-archive-indexer.sh",
    "matches": [
      {
        "line": 19,
        "text": "REF_UPDATED_TOPIC=\"0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9\""
      },
      {
        "line": 68,
        "text": "    local core_id=\"0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)\""
      },
      {
        "line": 69,
        "text": "    local calldata=\"0x40a3d246${core_id:2}\""
      },
      {
        "line": 81,
        "text": "        jq -r '.last_indexed_block // \"0\"' \"$CHECKPOINT_FILE\""
      },
      {
        "line": 90,
        "text": "save_checkpoint() {"
      },
      {
        "line": 93,
        "text": "        '{last_indexed_block: $block, updated_at: $timestamp}' > \"$CHECKPOINT_FILE.tmp\""
      },
      {
        "line": 113,
        "text": "# Extract CIDs from event (simplified - use proper ABI decoder in production)"
      },
      {
        "line": 118,
        "text": "    echo \"$data\" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||'"
      },
      {
        "line": 129,
        "text": "    local logs=$(query_ref_events \"$from\" \"$to\" \"$core_address\")"
      },
      {
        "line": 154,
        "text": "                            echo \"Failed to archive $cid\" >&2"
      },
      {
        "line": 158,
        "text": "                    echo \"Failed to pin $cid\" >&2"
      },
      {
        "line": 175,
        "text": "    local checkpoint=$(load_checkpoint)"
      },
      {
        "line": 197,
        "text": "        checkpoint=\"$next_end\""
      },
      {
        "line": 198,
        "text": "        save_checkpoint \"$checkpoint\""
      }
    ]
  },
  {
    "path": "scripts/evm-replication-reaper.sh",
    "matches": [
      {
        "line": 13,
        "text": "[[ \"$CHAIN_ID\" == injective-1 ]] || {"
      },
      {
        "line": 14,
        "text": "  echo \"replication reaper: CHAIN_ID must be injective-1\" >&2"
      },
      {
        "line": 28,
        "text": "    echo \"replication reaper: EVM_RPC must point to mainnet\" >&2"
      },
      {
        "line": 64,
        "text": "    local core_id=\"0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)\""
      },
      {
        "line": 65,
        "text": "    local calldata=\"0x40a3d246${core_id:2}\"  # moduleAddress(bytes32)"
      },
      {
        "line": 73,
        "text": "    local calldata=\"0x9d888e86\"  # repositoryCount()"
      },
      {
        "line": 83,
        "text": "    local calldata=\"0x0e832237${index_hex}\"  # repositoryIdAt(uint256)"
      },
      {
        "line": 102,
        "text": "        # Parse ABI-encoded response (simplified - use proper decoder in production)"
      },
      {
        "line": 106,
        "text": "        echo \"$result\" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||'"
      },
      {
        "line": 149,
        "text": "    if ipfs pin rm \"$cid\" 2>/dev/null; then"
      },
      {
        "line": 152,
        "text": "        echo \"Failed to unpin $cid (may already be unpinned)\" >&2"
      }
    ]
  }
]
```

<details>
<summary>九个本地源码/产物字段与部署证据对比</summary>

```json
[
  {
    "contract": "SuiteDirectory",
    "source_sha256": "f45d4a2dad277631499818672b01ace44e7650a6f0677b6a06c126107c82d0e7",
    "evidence_source_sha256": "f45d4a2dad277631499818672b01ace44e7650a6f0677b6a06c126107c82d0e7",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "BootstrapCoordinator",
    "source_sha256": "f1c9d9c52f6506bb5879cac6a90add3380821c8f7976ebbc065b89e8747d0534",
    "evidence_source_sha256": "f1c9d9c52f6506bb5879cac6a90add3380821c8f7976ebbc065b89e8747d0534",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "RepositoryCore",
    "source_sha256": "935965cbe3fa1d3e968d2f2a41608a9c6ef77fc68ec0dd59c8a8f8f79b5af069",
    "evidence_source_sha256": "935965cbe3fa1d3e968d2f2a41608a9c6ef77fc68ec0dd59c8a8f8f79b5af069",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "RecoveryModule",
    "source_sha256": "841876162a997be05de230e9c6d6bd59d5ad82217f6673690d2cd8589f3b2b54",
    "evidence_source_sha256": "841876162a997be05de230e9c6d6bd59d5ad82217f6673690d2cd8589f3b2b54",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "ModerationModule",
    "source_sha256": "82d86ec79a80ba672ab9303e0db3dbae96d22e8364d8677f71c723996669e891",
    "evidence_source_sha256": "82d86ec79a80ba672ab9303e0db3dbae96d22e8364d8677f71c723996669e891",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "EconomicModule",
    "source_sha256": "be27f25cf97c9e045fe0c1b43740a89638fbfd0c50b94782596f06e9d323d4bb",
    "evidence_source_sha256": "be27f25cf97c9e045fe0c1b43740a89638fbfd0c50b94782596f06e9d323d4bb",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "UsernameModule",
    "source_sha256": "f7526dea27c2646ba5b5afe4b1a5a71d75608295d0925dd835b2d6a7f5696102",
    "evidence_source_sha256": "f7526dea27c2646ba5b5afe4b1a5a71d75608295d0925dd835b2d6a7f5696102",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "BadgeModule",
    "source_sha256": "156aad191338155abb7a39da4c678feb282170f2d10f3afa06420e83dfb94740",
    "evidence_source_sha256": "156aad191338155abb7a39da4c678feb282170f2d10f3afa06420e83dfb94740",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  },
  {
    "contract": "ReleaseModule",
    "source_sha256": "301e63f27a07b71594ab53de9f8371fde043266c645b47d796437897e7a075b2",
    "evidence_source_sha256": "301e63f27a07b71594ab53de9f8371fde043266c645b47d796437897e7a075b2",
    "sourceMatches": true,
    "creationMatches": true,
    "templateMatches": true
  }
]
```
```json
[
  {
    "contract": "SuiteDirectory",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd",
    "liveHash": "0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd",
    "evidenceHash": "0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd",
    "status": "PASS"
  },
  {
    "contract": "BootstrapCoordinator",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "liveHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "evidenceHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "status": "PASS"
  },
  {
    "contract": "RepositoryCore",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
    "liveHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
    "evidenceHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
    "status": "PASS"
  },
  {
    "contract": "RecoveryModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
    "liveHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
    "evidenceHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
    "status": "PASS"
  },
  {
    "contract": "ModerationModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
    "liveHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
    "evidenceHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
    "status": "PASS"
  },
  {
    "contract": "EconomicModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
    "liveHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
    "evidenceHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
    "status": "PASS"
  },
  {
    "contract": "UsernameModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
    "liveHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
    "evidenceHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
    "status": "PASS"
  },
  {
    "contract": "BadgeModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
    "liveHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
    "evidenceHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
    "status": "PASS"
  },
  {
    "contract": "ReleaseModule",
    "immutableEvidenceComplete": true,
    "reconstructedHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
    "liveHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
    "evidenceHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
    "status": "PASS"
  }
]
```

</details>

### 可复现的只读 ABI / RPC 命令片段

代码仅在进程内执行，不创建测试/证据文件；fixture URI不是有效上传声明。固定区块历史读取未来可能受RPC保留策略限制，需据实际返回重新赋状态。

```javascript
// 在 D:/inj/next-injective-git 执行：node --input-type=module
import fs from 'node:fs';
import { keccak256, toHex, toEventSelector, toFunctionSelector, encodeEventTopics, encodeAbiParameters, decodeEventLog, encodeFunctionData, decodeFunctionResult } from './web/node_modules/viem/_esm/index.js';
const directoryAbi=JSON.parse(fs.readFileSync('contracts/evm-v2/abi/SuiteDirectory.json','utf8'));
const coreAbi=JSON.parse(fs.readFileSync('contracts/evm-v2/abi/RepositoryCore.json','utf8'));
const event=coreAbi.find(x=>x.type==='event'&&x.name==='RefUpdated');
console.log('Topic0',toEventSelector(event));
console.log('coreID',keccak256(toHex('igit.module.repository-core')));
console.log('moduleAddressSelector',toFunctionSelector(directoryAbi.find(x=>x.name==='moduleAddress')));
const payload={repoId:'0x'+'11'.repeat(32),refName:'refs/heads/audit-fixture',updatedBy:'0x'+'22'.repeat(20)};
const topics=encodeEventTopics({abi:coreAbi,eventName:'RefUpdated',args:payload});
const data=encodeAbiParameters(event.inputs.filter(x=>!x.indexed),['ab'.repeat(20),['ipfs://bafybeiauditfixtureone','ipfs://bafybeiauditfixturetwo']]);
console.log(decodeEventLog({abi:coreAbi,eventName:'RefUpdated',topics,data}));
console.log('scriptRegexMatches',data.match(new RegExp("ipfs://[a-zA-Z0-9]+", "g"))||[]);
const endpoint='https://k8s.testnet.json-rpc.injective.network', block='0x855fada', directory='0x24124cb60f9ef02f7deb5bc868c028fb412f5334';
let requestId=0;
async function rpc(method,params){
  if(!['eth_chainId','eth_getBlockByNumber','eth_call','eth_getCode'].includes(method))throw Error('read-only methods only');
  const queriedAt=new Date().toISOString();
  const response=await fetch(endpoint,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({jsonrpc:'2.0',id:++requestId,method,params}),signal:AbortSignal.timeout(25000)});
  const body=await response.json(); if(body.error)throw Error(JSON.stringify(body.error));
  console.log(JSON.stringify({queriedAt,endpoint,method,params,result:method==='eth_getCode'?{bytes:(body.result.length-2)/2,keccak256:keccak256(body.result)}:body.result}));
  return body.result;
}
if(BigInt(await rpc('eth_chainId',[]))!==1439n)throw Error('unexpected chain');
await rpc('eth_getBlockByNumber',[block,false]);
async function read(address,abi,functionName,args=[]){const result=await rpc('eth_call',[{to:address,data:encodeFunctionData({abi,functionName,args})},block]);return decodeFunctionResult({abi,functionName,data:result});}
for(const method of ['state','suiteVersion','configuredChainId','bootstrapCoordinator','bootstrapCoordinatorCodeHash','snapshotRoot'])console.log(method,await read(directory,directoryAbi,method));
const evidence=JSON.parse(fs.readFileSync('evidence/testnet-deployment-2026-08-25/suite-runtime-verification-2026-08-29.json','utf8'));
for(const m of evidence.modules){const address=await read(directory,directoryAbi,'moduleAddress',[m.id]);const hash=await read(directory,directoryAbi,'moduleCodeHash',[m.id]);const code=await rpc('eth_getCode',[address,block]);console.log(m.name,{address,hash,actual:keccak256(code),evidence:m.code_hash});}

```

## 附录 D. 实时只读 RPC 记录

Endpoint固定为 `https://k8s.testnet.json-rpc.injective.network`；时间为UTC（+8小时即Asia/Shanghai）。所有eth_call/eth_getCode在固定区块 `0x855fada` 查询。eth_getCode的完整字节在内存计算哈希，以下保存长度与Keccak返回摘要；可按相同区块重取原字节，未另写证据文件。此单区块记录不等于finality/reorg验收。

```json
{
  "endpoint": "https://k8s.testnet.json-rpc.injective.network",
  "blockTag": "0x855fada",
  "blockNumber": 139852506,
  "blockHash": "0x051d5b1577e22dc62d87a528d694cdd2391376f4ed844c154f476082a9f232cd",
  "chainIdRaw": "0x59f",
  "chainId": 1439,
  "dirLive": {
    "state": 1,
    "suiteVersion": "3",
    "configuredChainId": "1439",
    "bootstrapCoordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
    "bootstrapCoordinatorCodeHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "snapshotRoot": "0x5e2eaab50320b54d85e45bdc9f59ce94ada216d440fac3086fb30da7f98a1cea"
  },
  "directoryCodeHash": "0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd",
  "directoryBytes": 3745,
  "modules": [
    {
      "contract": "RepositoryCore",
      "id": "0x0f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8",
      "address": "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4",
      "storedHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
      "actualHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
      "evidenceHash": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
      "bytes": 23504,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "RecoveryModule",
      "id": "0x6cde1210dbeb58848fe7ff3cb286576a5f76ed31424ebc2117eff3f42e060631",
      "address": "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532",
      "storedHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
      "actualHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
      "evidenceHash": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
      "bytes": 9006,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "ModerationModule",
      "id": "0xa49a32b2d99126d93610cb8045423f52f62b8f3240b6c53899cbd19ca5eae7f3",
      "address": "0xb957B65634931dD0613abAC296D5a3837BF979Bd",
      "storedHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
      "actualHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
      "evidenceHash": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
      "bytes": 16383,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "EconomicModule",
      "id": "0xc9ab9ceae89076b0f126fd5329cb2bdf61760459991af4a5a599ad3a063c0c17",
      "address": "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd",
      "storedHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
      "actualHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
      "evidenceHash": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
      "bytes": 9476,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "UsernameModule",
      "id": "0xab916fa805962ec369f8b0815c08c95b3feab5da2e9c29cfdfacbbd9d7b3ee97",
      "address": "0xc7C164E46788b37e98D27aCe908C489999c09853",
      "storedHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
      "actualHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
      "evidenceHash": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
      "bytes": 8417,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "BadgeModule",
      "id": "0x129ffdcb675a98ccf83106de66261add937e83339ca1c3bcdea11a21d01137c2",
      "address": "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D",
      "storedHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
      "actualHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
      "evidenceHash": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
      "bytes": 6653,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    },
    {
      "contract": "ReleaseModule",
      "id": "0xe3b2d8c3973efdefb548d9f71c6e48d9cd24914c9f7df467aa34f6feac411993",
      "address": "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f",
      "storedHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
      "actualHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
      "evidenceHash": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
      "bytes": 6227,
      "addressMatches": true,
      "hashMatches": true,
      "status": "PASS"
    }
  ],
  "coordinator": {
    "address": "0x329921023FCf6E337E924686b92f7521549B5970",
    "bytes": 4478,
    "actualHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "storedHash": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "status": "PASS"
  },
  "bindings": [
    {
      "contract": "RepositoryCore",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0x0f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8",
      "status": "PASS"
    },
    {
      "contract": "RecoveryModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0x6cde1210dbeb58848fe7ff3cb286576a5f76ed31424ebc2117eff3f42e060631",
      "status": "PASS"
    },
    {
      "contract": "ModerationModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0xa49a32b2d99126d93610cb8045423f52f62b8f3240b6c53899cbd19ca5eae7f3",
      "status": "PASS"
    },
    {
      "contract": "EconomicModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0xc9ab9ceae89076b0f126fd5329cb2bdf61760459991af4a5a599ad3a063c0c17",
      "status": "PASS"
    },
    {
      "contract": "UsernameModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0xab916fa805962ec369f8b0815c08c95b3feab5da2e9c29cfdfacbbd9d7b3ee97",
      "status": "PASS"
    },
    {
      "contract": "BadgeModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0x129ffdcb675a98ccf83106de66261add937e83339ca1c3bcdea11a21d01137c2",
      "status": "PASS"
    },
    {
      "contract": "ReleaseModule",
      "directory": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
      "coordinator": "0x329921023FCf6E337E924686b92f7521549B5970",
      "id": "0xe3b2d8c3973efdefb548d9f71c6e48d9cd24914c9f7df467aa34f6feac411993",
      "status": "PASS"
    }
  ]
}
```

<details>
<summary>逐请求时间、method、params和返回值（code/block返回采用注明的摘要）</summary>

```json
[
  {
    "at": "2026-09-12T16:57:23.438Z",
    "method": "eth_chainId",
    "params": [],
    "result": "0x59f",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:57:24.755Z",
    "method": "eth_blockNumber",
    "params": [],
    "result": "0x855fada",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:57:26.716Z",
    "method": "eth_getBlockByNumber",
    "params": [
      "0x855fada",
      false
    ],
    "result": {
      "number": "0x855fada",
      "hash": "0x051d5b1577e22dc62d87a528d694cdd2391376f4ed844c154f476082a9f232cd",
      "timestamp": "0x6aa58474"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.367Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x47437d65"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000000000000000000000000000000000000000059f",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.367Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xb0f560c9"
      },
      "0x855fada"
    ],
    "result": "0x0000000000000000000000000000000000000000000000000000000000000003",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.368Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xbbb3f930"
      },
      "0x855fada"
    ],
    "result": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.368Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x5fb2266b"
      },
      "0x855fada"
    ],
    "result": "0x5e2eaab50320b54d85e45bdc9f59ce94ada216d440fac3086fb30da7f98a1cea",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.367Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:09.366Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xc19d93fb"
      },
      "0x855fada"
    ],
    "result": "0x0000000000000000000000000000000000000000000000000000000000000001",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:14.728Z",
    "method": "eth_getCode",
    "params": [
      "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
      "0x855fada"
    ],
    "result": {
      "byteLength": 3745,
      "keccak256": "0x48460f1f91897091148cfb4028956db320fcc7eb1f35e158a067cfe50269efcd"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.028Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f30f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8"
      },
      "0x855fada"
    ],
    "result": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.028Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f00f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000020269fb750d77e7c6c812290e7a7b99e0a3780d4",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.029Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f0a49a32b2d99126d93610cb8045423f52f62b8f3240b6c53899cbd19ca5eae7f3"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000b957b65634931dd0613abac296d5a3837bf979bd",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.028Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f06cde1210dbeb58848fe7ff3cb286576a5f76ed31424ebc2117eff3f42e060631"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000a7249ce20b54c4be440838f0ef403d2a6a07e532",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.029Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f36cde1210dbeb58848fe7ff3cb286576a5f76ed31424ebc2117eff3f42e060631"
      },
      "0x855fada"
    ],
    "result": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.029Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f0c9ab9ceae89076b0f126fd5329cb2bdf61760459991af4a5a599ad3a063c0c17"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000608380f355bf3d7cb0d3dcd58fc4dbe695daf3dd",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f0129ffdcb675a98ccf83106de66261add937e83339ca1c3bcdea11a21d01137c2"
      },
      "0x855fada"
    ],
    "result": "0x0000000000000000000000005e4ea68e31f89977bf056be9cb3d4f1c43ab995d",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f0ab916fa805962ec369f8b0815c08c95b3feab5da2e9c29cfdfacbbd9d7b3ee97"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000c7c164e46788b37e98d27ace908c489999c09853",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0xaa10e9f0e3b2d8c3973efdefb548d9f71c6e48d9cd24914c9f7df467aa34f6feac411993"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000aa0dd566eb2b21fed0ba9426c0e30887ce2de63f",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f3e3b2d8c3973efdefb548d9f71c6e48d9cd24914c9f7df467aa34f6feac411993"
      },
      "0x855fada"
    ],
    "result": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.338Z",
    "method": "eth_getCode",
    "params": [
      "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4",
      "0x855fada"
    ],
    "result": {
      "byteLength": 23504,
      "keccak256": "0x1c5e3ca62fa52b1b4ecef6da246b1b4dcfe5d027102165721c42ba3b5e21b467"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f3129ffdcb675a98ccf83106de66261add937e83339ca1c3bcdea11a21d01137c2"
      },
      "0x855fada"
    ],
    "result": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.361Z",
    "method": "eth_getCode",
    "params": [
      "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532",
      "0x855fada"
    ],
    "result": {
      "byteLength": 9006,
      "keccak256": "0x166e917cc005b551e15d840fb9fc92fb012048a715016304c7dd34047984b3c2"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.910Z",
    "method": "eth_getCode",
    "params": [
      "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f",
      "0x855fada"
    ],
    "result": {
      "byteLength": 6227,
      "keccak256": "0x3834e5dcdf1f26f1c30bd628c4a5117021eb70525e88d725e196f019a43c151c"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:16.846Z",
    "method": "eth_getCode",
    "params": [
      "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D",
      "0x855fada"
    ],
    "result": {
      "byteLength": 6653,
      "keccak256": "0x50300d8ae14280acd2f48939d9095f932ac3dc22b481af9e45e07e58f898a77c"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.029Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f3c9ab9ceae89076b0f126fd5329cb2bdf61760459991af4a5a599ad3a063c0c17"
      },
      "0x855fada"
    ],
    "result": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.030Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f3ab916fa805962ec369f8b0815c08c95b3feab5da2e9c29cfdfacbbd9d7b3ee97"
      },
      "0x855fada"
    ],
    "result": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:17.869Z",
    "method": "eth_getCode",
    "params": [
      "0xc7C164E46788b37e98D27aCe908C489999c09853",
      "0x855fada"
    ],
    "result": {
      "byteLength": 8417,
      "keccak256": "0xb7350b5e913d992a522ce2e05c3f3cb37ac50450486880bc36ab0c295292b49e"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:15.029Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x24124cb60f9ef02f7deb5bc868c028fb412f5334",
        "data": "0x64fff3f3a49a32b2d99126d93610cb8045423f52f62b8f3240b6c53899cbd19ca5eae7f3"
      },
      "0x855fada"
    ],
    "result": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:17.736Z",
    "method": "eth_getCode",
    "params": [
      "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd",
      "0x855fada"
    ],
    "result": {
      "byteLength": 9476,
      "keccak256": "0x907035110aeb8fa07f3a092ffd91b46b879ef54badcd4115459b79e6b9b0ad0d"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:58:19.975Z",
    "method": "eth_getCode",
    "params": [
      "0xb957B65634931dD0613abAC296D5a3837BF979Bd",
      "0x855fada"
    ],
    "result": {
      "byteLength": 16383,
      "keccak256": "0x3407b2f48bd761b0c241ba827960c84d95f6a100919f4a9863cf39c73c6e8de3"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T16:59:02.653Z",
    "method": "eth_getCode",
    "params": [
      "0x329921023FCf6E337E924686b92f7521549B5970",
      "0x855fada"
    ],
    "result": {
      "byteLength": 4478,
      "keccak256": "0xbbf6b60d9a90b2f87bfa15bca9733aa5851b2cbae05f1997dd34671b42cf27b7"
    },
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.950Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0xc9ab9ceae89076b0f126fd5329cb2bdf61760459991af4a5a599ad3a063c0c17",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.946Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.952Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xb957B65634931dD0613abAC296D5a3837BF979Bd",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.950Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.951Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.954Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.948Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.946Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0x0f2aa236426cdfb44f309719da4a1e7bf3c2f26dc5d3c28a474082cc7ad909b8",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.949Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.955Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xc7C164E46788b37e98D27aCe908C489999c09853",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0xab916fa805962ec369f8b0815c08c95b3feab5da2e9c29cfdfacbbd9d7b3ee97",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.955Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xc7C164E46788b37e98D27aCe908C489999c09853",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.945Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.951Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0x129ffdcb675a98ccf83106de66261add937e83339ca1c3bcdea11a21d01137c2",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.948Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0x6cde1210dbeb58848fe7ff3cb286576a5f76ed31424ebc2117eff3f42e060631",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.954Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0xe3b2d8c3973efdefb548d9f71c6e48d9cd24914c9f7df467aa34f6feac411993",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.955Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xc7C164E46788b37e98D27aCe908C489999c09853",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.952Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xb957B65634931dD0613abAC296D5a3837BF979Bd",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.951Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.954Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f",
        "data": "0x6a64ea95"
      },
      "0x855fada"
    ],
    "result": "0x00000000000000000000000024124cb60f9ef02f7deb5bc868c028fb412f5334",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.948Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532",
        "data": "0xcefdc2da"
      },
      "0x855fada"
    ],
    "result": "0x000000000000000000000000329921023fcf6e337e924686b92f7521549b5970",
    "status": "PASS"
  },
  {
    "at": "2026-09-12T17:13:37.953Z",
    "method": "eth_call",
    "params": [
      {
        "to": "0xb957B65634931dD0613abAC296D5a3837BF979Bd",
        "data": "0xa1308f27"
      },
      "0x855fada"
    ],
    "result": "0xa49a32b2d99126d93610cb8045423f52f62b8f3240b6c53899cbd19ca5eae7f3",
    "status": "PASS"
  }
]
```

</details>

## 附录 E. 重点文档逐条声明审计

涵盖用户指定11份文档中与完成、验证、覆盖率、索引安全有关的 283 个匹配行；包括相关标题、计划和警告以保留上下文。行号针对入场原文，四份可编辑文档的原文仍完整保留在其历史区；不是本轮新增文字。所有旧原文整体为HISTORICAL，下表仅对被检查的限定断言赋值。

PASS只证明具体文件存在、实现的静态路径或本地检查，不证明同一行隐含的全功能验收；NOT PROVEN也用于计划与缺乏实证的总括声明。不得把“需测试”当已完成。

C01–C14 的静态复核日期均为2026-09-13、环境见§1；精确命令、输出及执行时间见附录B。只有C04对应历史测试网交易，当前实时只读结果单列在§3/附录D；没有任何一条拥有本轮真实Kubo或写交易证据。

| 证据ID | 限定检查状态 | 实际文件/命令/输出依据 | mock / testnet / Kubo / transaction范围 |
|---|---|---|---|
| C01 | PASS | contracts/evm-v2/src + abi + artifacts；solc 原始输出 §2/附录B，只证明编译、ABI 和尺寸 | local / 非mock / 无测试网写入 / 无Kubo |
| C02 | BLOCKED | Get-Command forge 无结果；Foundry 三项未运行；原有 out/cache 不能证明本轮测试 | Windows环境 / 未执行Foundry |
| C03 | NOT PROVEN | 未见支撑91%或逐合约百分比的coverage命令+输出；本轮未运行coverage | 无当前coverage证据 |
| C04 | HISTORICAL | evidence/testnet-deployment-2026-08-25；source.commit=4fd6a07...；§3本轮RPC另列 | 历史测试网部署/激活；非本轮交易 |
| C05 | PASS | cli/internal/chain,remote,ipfs,replication,suitedeploy,suitemigration 和 cmd；go test -count=1 ./...、go vet ./... 退出0 | 单元+mock/fixture；无真实写入/Kubo |
| C06 | FAIL | 指定3个Moderation UI文件不存在；Repo/index.tsx没有该tab；modules.ts仅API | 文件/源码静态检查；Web本地测试不能证明UI |
| C07 | FAIL | 四个evm脚本错误Topic/hash/selector/动态ABI；§5及附录C | 静态+本地合成ABI fixture；无脚本运行 |
| C08 | FAIL | archive错误吞没并推进checkpoint；无blockHash/logIndex/reorg；hot-pin无checkpoint；reaper无dry-run/ALLOW_UNPIN | 静态检查；未运行pin/unpin |
| C09 | NOT PROVEN | Kubo API ECONNREFUSED，生命周期SKIP；远端confirmation、真实gateway/Git对象与写交易未采证 | 真实Kubo BLOCKED；E2E NOT PROVEN |
| C10 | HISTORICAL | 旧CI链接/过去测试/报告操作声明本轮未复验；仅保存历史来源 | 非本轮证据 |
| C11 | NOT PROVEN | 计划/标题/全称完成声明没有独立可复现实证；按§8-9重新分解任务 | 计划或不可证实的总括；不等于执行 |
| C12 | PASS | web typecheck/API73/build通过；modules.ts/transport.ts实现存在；§4 | 本地+mock/provider/fixture；无钱包写入 |
| C13 | FAIL | cli/cmd/igit-suite-operator/journal.go:61 TODO Verify signature；Go测试不能证明安全resume完成 | 静态；真实迁移NOT PROVEN |
| C14 | PASS | cli/internal/config/config.go + web/src/lib/profile.ts:29内置公开地址为空；:91/:100本地fallback另列 | 源码静态；未发布/切换 |

### docs/project-status-zh.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 13 | \| P0 基线 \| ✅ 完成 \| 100% \| 2026-08-19 \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 14 | \| P1.1-1.3 实现 \| ✅ 完成 \| 100% \| 2026-09-12 \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 44 | ## ✅ 最近完成（本周） | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 47 | - ✅ **A04 Web Moderation UI 实现完成** | C06 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 52 | - ✅ **A11 存储索引器 V2 完全重写** | C07 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 58 | - ✅ **文档精简完成** | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 62 | - ✅ **优先级体系重组** | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 70 | ### 九合约 Suite ✅ 100% | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 71 | \| 合约 \| 状态 \| 测试覆盖 \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 73 | \| SuiteDirectory \| ✅ \| 95% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 74 | \| BootstrapCoordinator \| ✅ \| 92% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 75 | \| RepositoryCore \| ✅ \| 94% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 76 | \| RecoveryModule \| ✅ \| 90% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 77 | \| ModerationModule \| ✅ \| 93% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 78 | \| EconomicModule \| ✅ \| 91% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 79 | \| UsernameModule \| ✅ \| 89% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 80 | \| BadgeModule \| ✅ \| 88% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 81 | \| ReleaseModule \| ✅ \| 87% \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 83 | **平均测试覆盖：** 91% | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 85 | ### 后端实现 ✅ 100% | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 86 | - CLI：所有 9 个模块完整实现 | C05 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 93 | \| Economic/Badge \| ✅ SponsorsTab \| | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 94 | \| **Moderation** \| **✅ 新实现（需测试）** \| | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 148 | - **测试覆盖率：** 91% (目标: >90%) ✅ | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 210 | - [迁移完成报告](../MIGRATION-COMPLETE.md) - 最新进展 | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |

### docs/project-status.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 3 | - Status: P1.2 deployment evidence and P1.3 fresh-empty activation complete; P1.4-P1.6 acceptance in progress | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 6 | - Latest Progress: 17 deployment/configuration transactions revalidated, 9/9 Blockscout verification complete, fresh-empty Suite activated | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 29 | ### ✅ Completed Milestones | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 31 | **P0: Windows and EVM Baseline Repair** (Completed 2026-08-19) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 36 |   - ✓ Native Windows support (no WSL2) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 37 |   - ✓ Native Linux support (no injectived) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 38 |   - ✓ Chinese Windows locale compatibility | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 39 |   - ✓ Windows DACL file permission protection | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 40 |   - ✓ Foundry test suite passing | C02 | BLOCKED | 当前区已更正；原文HISTORICAL保留 |
| 41 |   - ✓ Go race detector checks passing | C10 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 42 |   - ✓ Fixed `core.autocrlf=true` line-ending issues | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 43 |   - ✓ Fixed mainnet RPC endpoint (updated to current official endpoint) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 44 |   - ✓ Implemented bounded Kubo download timeouts and failover | C09 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 46 | **P1.1: Operator Runner Implementation** (Completed 2026-08-22) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 49 | - **Test Results:** 15/15 passing, 60.5% coverage | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 52 |   - ✓ Encrypted keystore with scrypt KDF (keystore.go) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 53 |   - ✓ Append-only signed journal with ECDSA signatures (journal.go) | C13 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 54 |   - ✓ Safe resume mechanism for uncertain receipts (main.go) | C13 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 55 |   - ✓ Manifest-driven calldata execution (manifest.go) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 56 |   - ✓ Fixed-block imported-state evidence emission | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 57 |   - ✓ Comprehensive unit test coverage | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 61 | **P1: Suite V3 Testnet Deployment and Cutover** (In Progress - P1.1, P1.2, and P1.3 Complete) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 64 | 1. ✅ **Deployment Evidence Complete** - Nine creations and eight configuration calls are retained in no-clobber `deployment.json` | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 65 | 2. ✅ **Fresh-Suite Scope Complete** - V1 is archive-preview-only and all seven module import counts are zero | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 71 | #### P1.1 Operator Runner (✅ Complete - 2026-08-22) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 76 | - [x] Complete unit tests (15/15 passing, 60.5% coverage) | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 83 | #### P1.2 Deployment Execution (✅ Complete - 2026-08-29) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 91 | #### P1.3 Fresh-Empty Activation (✅ Complete - 2026-08-29) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 107 | - [ ] Launch and complete deep source security review | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 114 | - [ ] Collect complete evidence directory | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 120 | **Estimated Engineering Time:** P1.1-P1.3 are complete; P1.4-P1.6 now consist primarily of product E2E, finality, security review, independent approval, and the final checksum-bound gate. | C09 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 174 | - ✅ **Non-Upgradeable** - No proxy, no diamond pattern, no delegatecall | C01 | PASS | 当前区已更正；原文HISTORICAL保留 |
| 175 | - ✅ **Single Trust Root** - Only `SuiteDirectory`, no mixed backends or fallback paths | C01 | PASS | 当前区已更正；原文HISTORICAL保留 |
| 176 | - ✅ **Verification First** - Client verifies chain ID, suite version, active state, all module code hashes | C05 | PASS | 当前区已更正；原文HISTORICAL保留 |
| 177 | - ✅ **Data Before Ref** - Pack must be durable before updating on-chain ref | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 178 | - ✅ **Immutable Evidence** - Deployment, migration, receipts, fixed-block state must be immutable evidence, not fabricated | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 187 | \| **Test Coverage** \| Comprehensive \| CLI unit tests, race detection, Foundry unit/invariant, Web API tests \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 188 | \| **Operator Tool Tests** \| ✅ 15/15 passing \| Coverage 60.5%, all core functionality tested (2026-08-22) \| | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 189 | \| **Code Quality** \| ✅ Passing \| go fmt, go vet, compilation checks all passing (2026-08-22) \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 190 | \| **P0 CI Status** \| ✅ Green \| Commit f6dcee9, all 5 jobs passing \| | C10 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 199 | \| **Source readiness mistaken for availability** \| Empty public profiles + evidence-gated release checks \| ✅ In place \| | C14 | PASS | 当前区已更正；原文HISTORICAL保留 |
| 200 | \| **Windows line-ending hash drift** \| .gitattributes pinned LF + Windows artifact gates \| ✅ Fixed (P0) \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 201 | \| **Locale-dependent behavior tests** \| Stable typed errors + independent translation tests \| ✅ Fixed (P0) \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 202 | \| **Windows key exposure** \| DACL/credential provider, current-user and SYSTEM only \| ✅ Implemented (P0) \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 221 | P0 ████████ Complete (2026-08-19) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 224 | P1 ▓▓▓▓░░░░ P1.1-P1.3 Complete ✅, P1.4-P1.6 In Progress | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 225 |    ├─ Operator runner implementation ✅ | C05 | PASS | 当前区已更正；原文HISTORICAL保留 |
| 226 |    ├─ 9 contract deployments and Blockscout verification ✅ | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 227 |    ├─ Fresh-empty activation; no V1 import ✅ | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 249 | Critical Path: P0 ✅ → P1 ⚠️ → P2 → M0 | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 256 | **✅ Implemented:** | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 277 | 1. ✅ P0 source and CI baseline (Complete) | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 278 | 2. ⏳ Complete and document independent security review (P1 in progress) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 282 | 6. ⏳ Complete release and cutover evidence hash-bound approval (P1 in progress) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 309 | ### 🟡 Short-Term Actions (After P1 Complete) | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 333 | - Choose finality depth and reorg response thresholds | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 374 | ### Why can't the public cutover complete yet? | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 377 | 2. Independent security review and hash-bound approval are not complete | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 380 | Deployment and fresh-empty activation are complete. CosmWasm V1 import is not | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 397 | **Current:** Direction accepted (ADR 0002), but not implemented. Current Suite only supports `ipfs://`. | C09 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 440 | \| 2026-08-22 \| P1.1 operator tooling complete - updated metrics and blocking status \| Project Assessment \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 441 | \| 2026-08-31 \| Confirmed P1.2/P1.3 complete; clarified P1.4-P1.6 and fresh-empty scope \| Project Assessment \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |

### docs/backlog.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 4 | **当前状态：** A04/A11 核心实现完成，进入测试验证阶段   | C06 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 47 | - [ ] 测试 checkpoint 保存/恢复 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 58 | **输出：** 索引器验证报告、checkpoint 文件 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 86 | **输出：** 生产就绪的解码器实现 | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 90 | ### P1.4 添加 Reorg 检测 ⏳ | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 95 | - [ ] 扩展 checkpoint 结构存储 block hash | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 97 | - [ ] 检测到 reorg 时回退逻辑 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 98 | - [ ] 测试 reorg 场景 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 100 | **Checkpoint 扩展：** | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 110 | **输出：** Reorg-safe 索引器 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 121 | - [ ] 整理测试覆盖率报告 | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 128 | - 测试覆盖报告 | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 237 | - [ ] CID 覆盖率告警 | C03 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 239 | - [ ] Checkpoint 延迟告警 | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 269 |   ├─ A04/A11 实现完成 ✅ | C06 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 272 |   ├─ P1.1 本地 UI 测试 ✅ | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 277 |   ├─ P1.4 Reorg 检测 ⏳ | C08 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 340 | - **执行总结：** `MIGRATION-COMPLETE.md` | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |

### docs/README.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 38 | \| 项目状态和规划 \| ✅ 最新 \| 2026-09-12 \| | C11 | NOT PROVEN | 当前区已更正；原文HISTORICAL保留 |
| 39 | \| 架构和 ADR \| ✅ 稳定 \| 2026-08-31 \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 40 | \| P0 证据 \| ✅ 完整 \| 2026-08-19 \| | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |
| 41 | \| A11 存储索引器 \| ✅ 实现完成 \| 2026-09-12 \| | C07 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 60 | - ✅ **A11 完成** - V2 EVM 事件索引器已实现（需测试） | C07 | FAIL | 当前区已更正；原文HISTORICAL保留 |
| 94 | - `evm-v2-repair-plan.md` - P0 修复计划（已完成） | C04 | HISTORICAL | 当前区已更正；原文HISTORICAL保留 |

### docs/a11-storage-indexer-v2.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 3 | **Status:** Implemented - Requires Testing and Production Validation | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 14 | **Purpose:** General-purpose V2 event indexer with checkpoint recovery | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 18 | - Checkpoint state persistence (JSON format) | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 21 | - Fail-closed error handling | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 36 | - Bounded block scanning with checkpointing | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 52 | - Checkpoint recovery for interrupted runs | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 65 | - Scans all repositories via `repositoryCount()` and `repositoryIdAt()` | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 97 | \| **Checkpoint** \| None or manual \| JSON state file \| | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 98 | \| **Reorg Handling** \| None \| Block confirmations \| | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 103 | 2. **Checkpoint Recovery:** Can resume from last processed block | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 104 | 3. **Fail-Closed:** Script exits on RPC error, malformed data, or invalid state | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 115 | - [ ] Verify checkpoint recovery works correctly | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 120 | - [ ] Add reorg detection (check block hashes at checkpoint) | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 127 | - [ ] Validate CID coverage matches expected refs | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 154 | 4. **No Reorg Detection Yet:** Scripts assume canonical chain. Add: | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 155 |    - Store block hash at checkpoint | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 157 |    - Rewind if reorg detected | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 165 | 2. CID coverage verified complete | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 200 | 2. **Investigate:** Collect RPC logs, event samples, checkpoint state | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 201 | 3. **Fix:** Address root cause (ABI decoding, reorg, rate limiting) | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |

### docs/evm-v2-handoff.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 13 | 已完成： | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 38 | \| 进度说明 \| `.../PROGRESS-SUMMARY.md` \| P1.2/P1.3 complete，P1.4-P1.6 open \| | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 78 | 5. 覆盖 receipt pending、RPC internal error、广播成功但 receipt 不确定、status `0x0` 和 finality/reorg 边界。 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |

### docs/liveagent-evm-v2-context.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|


### docs/project-knowledge-base-zh.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 56 | 当前源码 HEAD 是完整的 `0ba06f436558f12d97625b393767440cdd0f9862`，而现有测试网部署/激活证据绑定的源码提交是 `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`。后者是当前 HEAD 的祖先，但不是同一提交；正式切换门禁要求证据、批准和 `expected_commit` 精确一致。不要把短 SHA 或当前源码检查当作历史部署证据的替代。代码中的 EVM runtime `codehash` 是 runtime bytecode 的 Keccak 哈希；`source_sha256`、bytecode 文件 SHA-256 和应用层 release digest 是不同字段，不能互称。`VerifySuite` 也只证明某个固定区块上的 Directory/module 内部一致性，不证明源码、审计、finality 或当前 HEAD 已验证。  | C04 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 200 | 当前 Suite 和 helper 只接受 `ipfs://`（为历史 bare CID 保留受限读取）。其它 scheme 必须 fail closed。S3/R2 不能通过把 HTTP endpoint 写入 ref 来实现；需要 successor Suite、URI 协议、上传授权、完整性校验和迁移证据。 | C08 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 206 | ### 6.1 配置与 fail-closed | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 291 | \| Solidity Suite、ABI、架构测试 \| 已实现 \| | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 292 | \| CLI EVM registry/transactor、remote helper \| 已实现并有测试 \| | C05 | PASS | 原文件不改；当前入口标HISTORICAL |
| 293 | \| IPFS 上传、gateway 下载、复制确认 \| 已实现 \| | C09 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 294 | \| Web EVM runtime、wallet、业务页面 \| 已实现并有 API/行为测试 \| | C12 | PASS | 原文件不改；当前入口标HISTORICAL |
| 295 | \| V1 archive preview \| 已实现且与普通路径隔离 \| | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 296 | \| P1.1 operator runner \| 广播/receipt/resume 代码已实现并有测试；journal 重载时的签名校验仍是 TODO \| | C13 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 297 | \| P1.2 部署 evidence \| 已完成（历史证据） \| | C04 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 298 | \| P1.3 fresh-empty 激活 \| 已完成（历史证据） \| | C04 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 316 | \| `scripts/migration-cutover-readiness-test.ps1` \| 通过 \| fail-closed fixture gate 行为测试通过 \| | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |

### MIGRATION-COMPLETE.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 10 | ## ✅ 已完成的核心工作 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 12 | ### 1. P1.1 Web Moderation UI 完整实现 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 23 | - ✅ 举报提交表单（带reasonHash） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 24 | - ✅ 举报列表与分页显示 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 25 | - ✅ 举报详情与审计轨迹 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 26 | - ✅ 委员会操作（解决举报/处理申诉） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 27 | - ✅ 仓库状态设置（Active/Frozen/Delisted） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 28 | - ✅ 钱包状态管理与网络验证 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 29 | - ✅ 完整错误处理（拒签/revert/网络错误） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 30 | - ✅ Advisory vs Enforced 状态区分 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 40 | ### 2. P1.2 存储索引器V2完全重写 | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 49 | - JSON checkpoint状态持久化 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 52 | - 失败时关闭（fail-closed） | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 60 | - 有界块扫描与checkpoint | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 70 | - Checkpoint恢复中断运行 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 78 | - 扫描所有仓库（repositoryCount + repositoryIdAt） | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 92 | \| Checkpoint \| 无或手动 \| JSON状态文件 \| | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 93 | \| Reorg处理 \| 无 \| 块确认（待完善） \| | C08 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 100 | - `docs/README.md` - 完全重写为导航中心 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 101 | - `docs/a11-storage-indexer-v2.md` - A11完整实现文档 | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 109 | - ✅ project-status-zh.md（项目状态） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 110 | - ✅ architecture.md（架构说明） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 111 | - ✅ delivery-roadmap.md（交付路线） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 112 | - ✅ p0-evidence.md（P0证据） | C04 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 113 | - ✅ ADR目录（架构决策） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 114 | - ✅ project-knowledge-base-zh.md（AI接续用） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 119 | - evm-v2-repair-plan.md（已完成） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 126 | **九合约Suite完整性：** ✅ 100% | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 130 | \| SuiteDirectory \| ✅ \| 根注册表、模块绑定 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 131 | \| BootstrapCoordinator \| ✅ \| 顺序导入、滚动承诺 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 132 | \| RepositoryCore \| ✅ \| 仓库核心、refs、协作者 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 133 | \| RecoveryModule \| ✅ \| 守护者恢复 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 134 | \| ModerationModule \| ✅ \| 4个强制hooks \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 135 | \| EconomicModule \| ✅ \| 赞助、收益分成 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 136 | \| UsernameModule \| ✅ \| 用户名系统 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 137 | \| BadgeModule \| ✅ \| 徽章系统 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 138 | \| ReleaseModule \| ✅ \| 版本发布 \| | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 140 | **Moderation强制执行验证：** ✅ | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 148 | **旧架构清理验证：** ✅ | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 160 | \| **P1.1** \| **Web Moderation UI** \| **✅ 实现完成** \| 需 E2E 测试 \| | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 161 | \| **P1.2** \| **V2 事件索引器** \| **✅ 实现完成** \| 需测试网验证 \| | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 163 | \| P1.4 \| Reorg 检测 \| ⏳ 待实现 \| 依赖 P1.2/P1.3 \| | C08 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 167 | ### 已验证存在（后端完整） | C05 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 192 | **P1 完成度：** 2/6 已实现（33%）   | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 233 | - [ ] Checkpoint保存/恢复 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 251 | #### 4. 添加 P1.4 Reorg 检测 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 253 | # Checkpoint结构扩展 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 262 |   echo "Reorg detected! Rewinding..." | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 303 | #### 3. 无Reorg检测 🟡 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 306 | - Checkpoint存储block hash | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 308 | - 检测到reorg时回退 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 355 | - ✅ 未部署任何合约 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 356 | - ✅ 未签名或广播交易 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 357 | - ✅ 未访问私钥或助记词 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 358 | - ✅ 未修改生产配置文件 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 359 | - ✅ 未启用reaper或cleanup脚本 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 360 | - ✅ 未执行Git commit | C04 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 361 | - ✅ 所有6个用户dirty文档完整保留 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 364 | - ✅ 无RepoRegistryV2污染 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 365 | - ✅ Suite不可变性确认（无proxy/delegatecall） | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 366 | - ✅ Moderation hooks在4个强制点 | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 367 | - ✅ Directory绑定模块验证 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 376 | \| 仓库管理 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 377 | \| Fork功能 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 378 | \| Moderation强制 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 379 | \| 经济模型 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 380 | \| 恢复机制 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 381 | \| 用户名系统 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 382 | \| 徽章系统 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 383 | \| 版本发布 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 384 | \| 迁移工具 \| ✅ 完整 \| 100% \| | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 386 | **合约层：** 100% ✅   | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 387 | **CLI后端：** 100% ✅   | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 388 | **Web后端函数：** 100% ✅   | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 394 | \| 合约安全性 \| 不可升级 \| ✅ 长期稳定 \| 9/10 \| | C01 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 395 | \| 事件索引 \| ✅ V2实现 \| ✅ 生产就绪（需测试） \| 8/10 \| | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 396 | \| 模块化扩展 \| 独立模块 \| ✅ 可新增 \| 8/10 \| | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 468 | 1. ✅ **A04完整实现** - Web Moderation UI功能完备，适配当前Suite | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 469 | 2. ✅ **A11完全重写** - 4个存储索引器全部从V1迁移到V2 EVM事件模式 | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 470 | 3. ✅ **文档精简** - 建立清晰导航结构，减少冗余 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 471 | 4. ✅ **架构验证** - 九合约完整性100%，无旧架构污染 | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 481 | 当前位置: A04/A11实现完成 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 495 | - 🟢 **低风险：** 文档和架构验证已完成 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 499 | **迁移执行状态：** ✅ 核心实现完成   | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |

### MIGRATION-REPORT-P1P2.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 11 | ### 已完成实现项目 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 15 | \| **P1.1** \| A04 \| **Web Moderation UI** \| ✅ **已实现** \| 需 E2E 测试 \| | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 16 | \| **P1.2** \| A11 \| **V2 事件索引器** \| ✅ **已实现** \| 需测试网验证 \| | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 23 | \| P1.4 \| A11-补充 \| Reorg 检测 \| ⏳ 待实现 \| | C08 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 27 | ### 已验证存在（后端完整，无需独立项目） | C05 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 55 | \| A10 \| 迁移执行 \| `implemented-needs-evidence` \| | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 63 | ## ✅ P1.1 Web Moderation UI（原 A04） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 65 | **状态：** 实现完成，需 E2E 测试 | C09 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 74 | - ✅ 举报提交表单 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 75 | - ✅ 举报列表与分页 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 76 | - ✅ 举报详情与审计轨迹 | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 77 | - ✅ 委员会操作（解决/申诉） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 78 | - ✅ 状态设置（Active/Frozen/Delisted） | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 79 | - ✅ 完整错误处理 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 91 | ## ✅ P1.2 V2 事件索引器（原 A11） | C07 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 93 | **状态：** 实现完成，需测试网验证 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 102 | - ✅ V1 CosmWasm LCD → V2 EVM `eth_getLogs` | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 103 | - ✅ 有界块范围查询（10,000 块/次） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 104 | - ✅ Checkpoint 持久化 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 105 | - ✅ 失败时关闭（fail-closed） | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 106 | - ✅ 主网安全检查 | C08 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 112 | 4. 添加 Reorg 检测（P1.4） | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 130 | ## ⏳ P1.4 Reorg 检测（原 A11 补充） | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 135 | - Checkpoint 存储 block hash | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 137 | - 检测到 reorg 时回退逻辑 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 138 | - 测试 reorg 场景 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 151 | - 测试覆盖率报告 | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 195 | - ✅ 所有 9 个合约已实现 | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 196 | - ✅ Moderation hooks 在 4 个强制点 | C01 | PASS | 原文件不改；当前入口标HISTORICAL |
| 197 | - ✅ 无 RepoRegistryV2 污染 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 198 | - ✅ CLI 后端 100% 完整 | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 199 | - ✅ Web 后端函数 100% 完整 | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 202 | - ✅ Economic/Badge（SponsorsTab） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 203 | - ✅ Moderation（新实现 P1.1） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 229 | 11. `MIGRATION-COMPLETE.md` | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 250 | 5. **P1.4 实现** - Reorg 检测 | C08 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 258 | - ✅ 未部署合约 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 259 | - ✅ 未签名交易 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 260 | - ✅ 未访问私钥 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 261 | - ✅ 未修改生产配置 | C10 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 262 | - ✅ 未启用 reaper | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 263 | - ✅ 未进行 Git 提交 | C04 | HISTORICAL | 原文件不改；当前入口标HISTORICAL |
| 264 | - ✅ 所有 6 个用户 dirty 文档完整保留 | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |

### A01-BXX-MIGRATION-REPORT.md

| 入场行号 | 原表述（引用，不代表当前事实） | 对应证据ID | 限定断言状态 | 处理 |
|---|---|---|---|---|
| 13 | **Architecture Verdict:** The target Suite (SuiteDirectory + 7 modules) is structurally complete. Old RepoRegistryV2 contracts have been correctly removed and must not be reintroduced. | C01 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 25 | ### 已验证存在/已完成项目（不需要独立跟踪） | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 60 | **Verification:** web/src/lib/transport.ts implements complete Suite verification | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 71 | - CLI backend: cli/internal/chain/evm_suite_registry.go lines 934-1026 (complete moderation API) | C05 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 73 | - Test coverage in SuiteArchitecture.t.sol validates enforcement | C03 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 75 | **Action Taken:** None required (contracts and backend complete) | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 79 | ### A04-A13 and B01-B06 Analysis Complete | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 85 | Created complete Web Moderation UI adapted to current Suite architecture: | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |
| 92 | - ✅ **A01-A03, A05-A08, A12**: Verified existing or superseded | C11 | NOT PROVEN | 原文件不改；当前入口标HISTORICAL |
| 93 | - ✅ **A04**: Implemented Web Moderation UI (needs E2E testing) | C06 | FAIL | 原文件不改；当前入口标HISTORICAL |

## 附录 F. 落盘后核验与最终工作树

复核时间：2026-09-13 01:24:19 +08:00。

- 585 个入场受检查原文件中仅四份获准文档发生变化；其余581个文件哈希相同，无文件缺失。源码、ABI、artifacts、旧报告和部署证据未变。
- 四份文档都连续包含各自完整入场字节；原文保留检查4/4 PASS，历史折叠区/fence均闭合，当前入口链接全部存在。
- 文档中可复现代码经 node --check 通过；ABI/fixture前半段在真实Node中执行退出0，返回正确Topic、module ID、selector及空正则匹配。未在文档QA中再次发RPC。
- git diff --check 返回2，发现42处trailing whitespace；逐一定位全部位于原样保留的用户原文（Markdown双空格换行）。本轮不修改原文来制造检查通过；新当前区/复现示例自身未引入这些警告。
- git diff --name-only仍只列四份允许修改的文档；基线和五个本轮缓存目录为新增untracked。没有stage/commit/push。
- 文件全量新增检查未发现基线与登记缓存以外的非ignored新增文件。ignored构建和公开参数临时配置见§7。

最终git status --short --branch原始输出：

```text
## dev...origin/dev
 M docs/README.md
 M docs/backlog.md
 M docs/project-status-zh.md
 M docs/project-status.md
?? A01-BXX-MIGRATION-REPORT.md
?? MIGRATION-COMPLETE.md
?? MIGRATION-EXECUTION-SUMMARY-ZH.md
?? MIGRATION-REPORT-P1P2.md
?? PRIORITY-MIGRATION-DONE.md
?? PRIORITY-UPDATE-COMPLETE.md
?? cli/null/
?? contracts/evm-v2/%SystemDrive%/
?? contracts/evm-v2/null/
?? docs/PRIORITY-MAPPING.md
?? docs/a11-storage-indexer-v2.md
?? docs/evm-v2-handoff.md
?? docs/liveagent-evm-v2-context.md
?? docs/project-knowledge-base-zh.md
?? docs/reconciliation-baseline-2026-09-12.md
?? scripts/evm-archive-indexer.sh
?? scripts/evm-event-indexer.sh
?? scripts/evm-hot-pin-indexer.sh
?? scripts/evm-replication-reaper.sh
?? web/%SystemDrive%/
?? web/null/

```

文档原文保留检查：

```json
[
  {
    "path": "docs/project-status-zh.md",
    "byteExactOriginalRetained": true,
    "currentHeaderMatches": true,
    "opens": 1,
    "closes": 1
  },
  {
    "path": "docs/project-status.md",
    "byteExactOriginalRetained": true,
    "currentHeaderMatches": true,
    "opens": 1,
    "closes": 1
  },
  {
    "path": "docs/backlog.md",
    "byteExactOriginalRetained": true,
    "currentHeaderMatches": true,
    "opens": 1,
    "closes": 1
  },
  {
    "path": "docs/README.md",
    "byteExactOriginalRetained": true,
    "currentHeaderMatches": true,
    "opens": 1,
    "closes": 1
  }
]
```


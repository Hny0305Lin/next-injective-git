# 项目待办：从事实基线重新开始

复核日期：2026-09-13（Asia/Shanghai）；基线文件名按任务指定保留 2026-09-12。
源码：`dev` / `0ba06f436558f12d97625b393767440cdd0f9862`。

当前事实只以 [唯一事实基线](reconciliation-baseline-2026-09-12.md) 为准；本页是入口或摘要，不是另一份验收报告。
状态限定为 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL，定义和原始命令输出见基线。

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

## 新存储工作流 S01–S07（2026-09-13 用户决策）

范围见 [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md)；
技术细节见 [BYOS 实施规格](storage-byos.md)。下表状态已按 2026-10-05 交付情况同步；
各切片的原始执行记录见下方日期小节。

主网首期直接目标 storage-neutral successor；只接用户自有 AWS S3 / Cloudflare R2 桶，
不接 MinIO/其他云/自建对象存储服务。公开仓库、canonical JSON、用户独立 reader、用户付费，
不强制双副本，不以托管 broker 或私有仓库加密为首期前置条件。

| 任务 | 顺序 / 依赖 | 当前状态 | 实施范围与退出条件 |
|---|---|---|---|
| S01 Canonical manifest / commitment | 立即，本地协议起点 | PASS | 严格 schema、JCS bytes、外层 manifest digest、digest keys、Git/object algorithm 和安全限额；Go/TS 共用正反向向量，明确 pack 顺序与单 pack locations |
| S02 Verified packstore boundary | S01；先封装现有 IPFS | PASS | 临时文件流式写/读、raw SHA-256/size、验证后才摄取；明确 legacy 无链上 raw digest 的边界，初始 successor 用自包含非 thin pack；取消/错误/Windows 文件生命周期测试 |
| S03 AWS/R2 BYOS adapters / config | S01–S02；不等云账号 | PASS | 两个独立 provider capability、endpoint allowlist、writer/reader credential reference、条件写/回读/重复对象/有界重试/multipart 限额与恢复；无真实凭据的 SDK/HTTP contract tests，AWS/R2 不初始化 Kubo/replication |
| S04 Successor Suite / versioned ABI | S01 协议冻结后 | PASS（2026-10-04 本地；2026-10-04 深夜测试网部署） | 合约状态/查询/事件绑定 manifest，revision CAS/force/delete/fork/bootstrap 一起审查；Go/Web/indexer/evidence schema 同版本，保留 v3 ABI 和历史证据，未知组合上传前拒绝；solc/尺寸/parity 分层报告；Foundry unit/invariant/gas 仍 BLOCKED（R04） |
| S05 CLI/Web 本地纵向接入 | S02–S04 | PASS（2026-10-04） | fake cloud/chain + 真实本地 Git 的 push/clone/fetch/pull/new-ref/force/delete 与失败恢复；Web 公开 manifest/pack 先验摘要，CORS/大小上限/鉴权限制提示，无云 secret |
| S06 真实云 / successor 测试网 E2E | 对应本地切片通过，另取资源和写入授权 | PASS（R2 全链路 2026-10-04 深夜）；AWS canary NOT PROVEN | 分别证明 AWS/R2 存储及支持的 multipart、公共 GET/CORS、独立 reader、Windows/Linux no-Kubo Git；匹配 successor 收据/finality、源码/ABI/commit。R2 已真实通过；真实 AWS canary、force 陈旧/并发竞争、Blockscout 验证仍待补 |
| S07 历史 v3 导入（条件性） | 仅用户另选 import scope 时，依赖 S04–S06 | NOT PROVEN | 固定视图清点、CID→digest/size/位置 mapping、原 bytes/顺序与独立依赖闭包、目标 ref 上下文、保留 legacy 读取/回滚；新 Suite 不自动要求该步骤，V1 仍只读 archive |
| S08 增量 pack（manifest schema 2，不改 v4 合约） | 立即排期（2026-10-05 用户决策）；同日交付 | PASS（2026-10-05 本地切片 + 真实测试网/R2 增量推送复验） | fast-forward push 仅打包本 ref 相对上一 tip 的新对象，manifest 链显式 dependsOn 闭包；空增量复用包集；非 ff/旧 manifest 失效/16 包或 2 GiB 上限自动回退全量；不用 git thin pack；旧客户端 fail-closed；BYOS §2.3 门槛为验收标准，见 [ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md) |

### 2026-09-13 本地切片进展

S01–S03 的 PASS 限定为 [BYOS 规格第 9 节](storage-byos.md#9-s01s03-本地实现与可重复验证2026-09-13) 记录的源码和本地测试。
已实现 Go/TS JCS 交叉向量、流式 verified packstore、完整历史 Git fixture、AWS/R2 独立能力与配置、mock 冲突/回读/权限/恢复测试。
R2 限单 PUT 16 MiB；AWS multipart 仅本地测试。现有 helper 保留 v3；storage doctor/show 仅本地，不解析密钥。
S04–S07 仍 NOT PROVEN；下一切片为 successor ABI/CAS/version dispatch，再接 CLI/Web。真实云、真实 IPFS 和链上 Git E2E 未执行。

### 2026-10-04 S04 本地切片进展（合约+Go/mock 链；S05 CLI/Web 接入仍未开始）

- 新增 `contracts/evm-v2-successor/`：fresh successor 套件候选（suiteVersion=4）。`RepositoryCore` 为承诺形态（manifestDigest/manifestSize/bootstrapLocator/revision），revision CAS（force 不豁免）、delete 保 revision 的 tombstone（防 delete/recreate ABA）、fork 不复制 ref 承诺、RefUpdated/RefDeleted 事件 v2 携带完整 refName 与全部承诺字段；其余八模块与 v3 逐字节一致（检查脚本强制），SuiteDirectory 仅 suiteVersion 常量不同。v3 源码/ABI/artifacts/历史证据未改动。
- `scripts/evm-successor-solc-check.mjs`：锁定 solc 0.8.24（optimizer runs=1, viaIR）编译、EIP-170 限额、unchanged-from-v3 文件集校验、版本化 abi/artifacts（schema `igit.evm-successor.solc-artifact.v1`）。实测 successor RepositoryCore runtime **22637B**（v3 为 23504B），EIP-170 余量 **1939B**。
- 新增 `cli/internal/chain/successor`：checked-in ABI 加载、CAS 客户端（CommitmentMismatch/RefNotFound 类型化 revert 解码）、契约语义一致的 FakeChain（含 uncertain receipt 注入）；测试覆盖 create/update/CAS 冲突/force 不豁免/tombstone+ABA 重放拒绝/fork 无承诺/事件完整字段/无效承诺 fail closed，以及 fake cloud + 真实本地 Git 的 publish→CAS→冷克隆（manifest digest/size 预检→JCS Parse→逐包 ReadVerified→IndexVerified→fsck）、上传成功后 CAS 冲突的对象保留与定向恢复、uncertain receipt 解析、fork 上下文绑定拒绝复制源 manifest。
- 验证：`go vet ./...` PASS、`go test -count=1 ./...` 29 包全绿（含新包）；`npm run check --prefix contracts/evm-v2-successor` PASS（含幂等重跑）。Foundry unit/invariant/gas 仍 **BLOCKED**（无 forge，R04 口径不变）；真实测试网部署/交易、真实云写入、Web 解码器与 CLI helper 接入仍 **NOT PROVEN**（S05/S06 范围）。
### 2026-10-04 S05 本地切片进展（CLI/Web 接入；真实链/云待凭据注入）

- CLI 存储中立分派：`git-remote-igit` 经 `chain.ProbeSuiteInfo` 验证套件版本——v3 走原 IPFS 路径（行为不变），v4 走 `internal/byos`（push=Prepare+CAS、fetch=manifest+ReadVerified+IndexVerified、list=manifest 取 commit OID），BYOS 路径不初始化 Kubo/网关/replication。`chain.SuccessorRegistry` 复用 VerifySuite 信任链与 EVMTransactor 语义；`packmanifest.ParseRefManifest` 提供无预知 commit 的冷读取入口。`igit storage add` 登记 profile 路径（仅引用，不含凭据）。
- Web：`transport` 版本容错（3/4）+ successor core ABI；`registry` 按 suiteVersion 分派（`listSuccessorRefs`/`resolveSuccessorRefCommit`，commit OID 来自已验证 manifest）；`successorReader.ts` 受限 fetch+承诺对账+单包/总量内存上限+CORS 可操作提示（不索取 secret）；`gitstore.loadVerifiedRef` 先验证后 isomorphic-git 摄取；`useRepoViews` 自动分派。
- 本地验证：`go vet ./...` PASS、`go test -count=1 ./...` 30 包全绿（新增 byos 5 测试 + remote BYOS 全协议会话测试：push→list→冷 fetch→fsck）；`npm run test:api` PASS（新增 successor-reader 6 测试：验证/篡改/跨仓库替换/CORS 提示/超限/包预算）；`npm run typecheck` PASS；`npm run test:storage-cross` PASS；双套件 solc check 均 PASS。修复并回归：push 批在 BYOS 下误走 v3 resolveRepo 的缺口、delete-only push 的 Kubo preflight 语义回归。
- 测试网部署准备：`evm-successor-deploy` 命令（复用 suitedeploy no-clobber 证据流，schema 双白名单）；artifacts `--check` PASS（set sha256 d58df75b…）；operator key 0x3753…43b 余额约 0.63 INJ；storage profile 模板已备（R2 桶/prefix/publicBase 为已验证 canary 参数）。**BLOCKED（等待用户）**：`IGIT_EVM_KEY_PASSWORD` 与 `IGIT_CANARY_R2_WRITER/READER_KEY/SECRET` 需由用户通过环境变量注入（值不得经聊天/文件明文传递）；真实链/真实云读写与公开 profile 切换仍 NOT PROVEN。

### 2026-10-04 深夜 S06 真实层进展（successor 测试网部署 + 真实 R2 端到端）

- **fresh successor 套件已部署并激活于 Injective 测试网**（chainId 1439）：Directory `0xf987396475d0a4c96b722e993a95d8720a6292ad`，Coordinator `0x0360f499fda8d4cf8fba2d3f3c1e28871b8a76fc`，operator `0x85ea…4fa8`（igit-dev/successor-op key）。9/9 合约 + 8/8 配置交易全部确认，绑定验证 suiteVersion=4/state=active/7 模块 code hash 匹配；证据（no-clobber）：`local-only/successor-deploy/deployment5.json`（另有 deployment/2/3/attempt1/successor* 为失败与恢复尝试记录，保留不删）。激活 tx `0x8048fc61…`。
- **真实端到端全链路 PASS**：`git push`（自包含 pack → R2 条件 PUT+全量回读 → JCS manifest → CAS 上链）、匿名公开 GET+ACAO 验证（pub-…r2.dev，SHA-256 与链上承诺一致）、干净 Windows 冷 `git clone`（全程无 Kubo/WSL2/injectived，fsck --strict 通过）、增量 push（revision 2）、tag push（新 manifest 绑定 refs/tags）、`git ls-remote`（含 HEAD symref）、`git fetch`、删除 ref（tombstone，getRef 回退 RefNotFound）、tombstone 重建（revision 3 单调）。
- **真实链缺陷修复**：① RPC Client.Timeout 超时原被排除在重试外（evm_rpc.go 现按 "Client.Timeout exceeded" 语义重试）；② getRef/updateRef 的 revert payload 未解码为类型化错误（successorEVMBackend 读/写路径现接 DecodeRevert）；③ tombstone 重建期望 (0,0) 被 CAS 正确拒绝后客户端现按 mismatch.actualRevision 定向重试一次；④ suitedeploy 绑定验证硬编码 v3（Options.SuiteVersion 参数化，默认 3 不变）；⑤ evm-activate-suite 硬编码 v3 snapshot root（新增 --snapshot-root）；⑥ 测试网历史状态修剪窗口短于整套部署时长（Options.VerifyAtLatest 仅 successor 路径启用）。
- 全量回归：`go vet ./...` PASS、`go test -count=1 ./...` 30 包全绿。
- **NOT PROVEN（真实层遗留）**：force push 陈旧场景与真实并发双写竞争（CLI 时序无法制造；mock 层已覆盖）；R2 对象外部篡改检测（无覆盖权限且不应执行）；Blockscout 源码验证（该网络环境下 explorer 不可达）；真实浏览器 igit.xyz 浏览（本地 Settings 配 Directory `0xf98739…92ad` 后 commits/files/refs 即走 successor verified reader——待人工浏览确认）；公开 profile 切换未做（内置 profile Directory 仍为空，符合现行策略）。
- 当前 igit 全局配置已指向 successor Directory；恢复 legacy v3：`igit config set evm_suite_directory_address 0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334`。

### 2026-10-05 路径查询兼容层（web 集中版本分派 + 前向兼容）

- 新增 `web/src/lib/suite-compat.ts`：suite 协议版本的唯一分派入口。`refShapeForVersion()` 按链上版本映射 ref ABI 形态（v3-→ipfs-pack-uris，v4→manifest-commitment，v5+→unknown 回退 v4 ABI 尝试读取）。`transport.ts` 版本检查放宽为 v3+ 均通过验证（v3 以下无 EVM 读路径）；未知版本（v5+）走最新已知 ABI 的 best-effort 读取 + 页面显示 untested 警告标签，不再一刀切拒绝。`registry.ts`/`gitstore.ts` 的分派统一改为 `refShapeForVersion()` 判断，不再散落硬编码版本号比较。
- Repo 页面新增 SuiteVersionBadge：实时显示当前套件协议版本（如 "Suite v4 · BYOS"），未知版本显示 "(untested)"。
- 新增 `docs/suite-version-compatibility.md`：版本兼容矩阵，明确各版本路径规则、web/CLI 兼容范围、前向兼容策略与新增版本的扩展方法。
- CLI/合约零改动（v3 IPFS + v4 BYOS 现有分派已满足要求）。验证：typecheck PASS、test:api 146 PASS、go vet + go test 30 包全绿。

### 2026-10-05 S08 立项：增量 pack（manifest schema 2，不改 v4 合约）

用户当日决策：增量 pack 必须做、立即排期、走小改路线——不改 Suite v4 合约与 suiteVersion，以 manifest `schemaVersion 2` 表达同一 ref 的有序 pack 链。决策与验收门槛见 [ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md)；本节为排期记录，**实现未开始**，不构成任何交付证据。

- 写端（`cli/internal/byos` + `gitio`）：fast-forward push 仅打包本 ref 相对上一 tip 的新对象，唯一排除基是本 ref 旧 tip（`rev-list --objects <new> --not <old>`），绝不排除 sibling refs；新 pack 追加进 manifest 链并显式 `dependsOn`（向后引用、无环、有深度上界）；空增量（如 tag 指向已发布 commit）复用包集、仅换 commit 绑定（revision+1，不上传 pack）；非 fast-forward/上一 manifest 缺失或校验失败/链达 16 包/总量将超 2 GiB 时自动回退全量自包含 pack 并重置链（§2.1 已许可的重打包模式）；force 仍不豁免 revision CAS，ff/ancestry 是客户端检查，链上只保留 CAS 并发检查。
- 读端：CLI `FetchRef` 已按 `manifest.Packs` 顺序逐包 `ReadVerified`+摄取（含去重）；需把 `gitio.IndexVerified` 拆为逐包字节/index 校验 + 全链一次终检（最后一包后 `cat-file`/`fsck` 闭包检查），因为中间 pack 不含新 tip。Web `gitstore`/`successorReader` 保持逐包预算（32 MiB/包、256 MiB 总量）并新增 schema 2 解析。
- 兼容性：现有 Go/TS 解析器对 `schemaVersion != 1` 一律拒绝（fail-closed 已是现状）；schema 2 需新增 Go/TS 交叉向量（真实实现生成，不手写）；schema 1 manifest 与既有 digest-key 对象不变；旧客户端对 schema 2 必须给出可操作的双语升级提示，不得静默回退。
- 验收（硬门槛，出自 [BYOS §2.3](storage-byos.md)）：依赖闭包显式且无环（拓扑序校验）、数量/总量/深度有上界、缺失任一依赖 pack 时拒绝摄取且不改 ref、删除 sibling ref 后独立 clone 仍完整、空增量/force 重写/16 包回退/CAS 冲突中断后定向恢复各有本地测试；真实 R2 上复验一次增量 push 与冷 clone。
- 明确不做（S08 范围外）：链 compaction/合并、orphan GC、跨 ref/跨仓库依赖、git thin pack（`--thin`/`--fix-thin`）、合约或 suiteVersion 变更、私有仓库。

### 2026-10-05 S08 本地切片进展（同日立项后交付）

- **协议**：`packmanifest` schema 2 落地——Go/TS 解析器接受 `schemaVersion 1|2`；schema 1 仍要求 `dependsOn=[]`（自包含），schema 2 允许显式依赖闭包，且仅接受**向后引用**（自身/前向/未知/重复 digest 一律拒绝，无环由构造保证）。共享向量由 TS 实现重新生成：**13 正向（+schema2-chain/schema2-partial-deps）/ 60 反向（+5 个 schema2 依赖违规）**，`test:storage-cross` Go↔TS 字节/摘要双向 PASS。原 `schema-version=2` 负向向量语义变更为 `=3`（未知版本仍拒绝）。
- **Git 层**：`gitio` 新增 `IsAncestor`（merge-base --is-ancestor，客户端 ff 检查）、`PackIncremental`（`rev-list --objects --count <new> --not <old>` 计数 + `pack-objects --revs` 无 `--thin` 打包；计数 0 返回空信号；非 ff 基拒绝）、`IndexPackVerified`（逐包字节+结构校验，不要求 tip）与 `VerifyClosure`（全链一次 cat-file+fsck 终检）；`IndexVerified` 改为二者组合，行为顺序不变。真实 Git 测试覆盖：增量包显著小于全量包、链式冷摄取+fsck、**孤立增量包被 `index-pack --strict` 因悬空父引用直接拒绝**（缺失依赖在摄取层即 fail-closed，比 §2.3 门槛更强）、篡改字节在 Git 之前被拒。
- **BYOS 发布/读取**：`byos.PushRef` 经 `planPack` 决策——ref 存在且上一 manifest 可读、ff、链未达 16 包且总量未超限时，追加一个增量包并构建 schema 2 链 manifest（dependsOn=全部更早条目）；tip 已是发布 commit 时零上传直接成功（空增量）；任一条件不满足（非 ff force 重写/上一 manifest 缺失或校验失败/16 包回卷/预算超限）自动回退 schema 1 自包含全量包。增量上传仅 PutIfAbsent 新包 + `PublishManifest`（新拆分的 manifest 发布入口），**不重传/不覆盖既有包**。`FetchRef` 改为逐包 `IndexPackVerified` + 全链一次 `VerifyClosure`。
- **测试矩阵（全部本地真实 Git + fake chain/cloud）**：schema2 链构建与 dependsOn 断言；增量 push 仅新增 2 个对象（新包+新 manifest）；链冷克隆+`fsck --strict`；空增量零上传零 revision；force 重写回退 schema 1 单包；上一 manifest 篡改回退全量并仍可冷克隆；16 包回卷（第 17 次 push 重置为单包 schema 1，revision 单调）；删除链上基础包后 fetch 拒绝；删除 sibling ref 及其全部对象后主 ref 冷克隆完整（per-ref 独立）；remote helper 协议级增量会话（push→增量 push→list→冷 fetch→fsck）。
- **验证记录**：`go vet ./...` PASS；`go test -count=1 ./...` 30 包全绿（新增 gitio 增量测试、byos 6 个 S08 测试、remote 增量会话、packmanifest 闭包规则）；`npm run test:api` 155/155 PASS（+9）；`npm run typecheck`、`npm run build`、`npm run test:storage-cross` PASS。
- **未做/边界**：真实 R2 上的增量 push 复验 **已通过**（见下节）；合约/ABI/suiteVersion 零改动（双套件 solc 检查保持通过）；schema 1 manifest 与既有对象完全兼容；旧客户端对 schema 2 fail-closed 的真实二进制回归未执行（解析器拒绝逻辑由向量与单测覆盖）。

### 2026-10-05 S08 真实层复验（Injective 测试网 + 真实 Cloudflare R2 增量推送）

- **仓库**：`inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase-byos`（repoId `0xa52a01ecfabc8d179af3015bdd5d7d7f754ee5c745e95fc0434e0ac17edcdade`，绑定 r2-writer/r2-reader profile），operator `0x85ea…4fa8`（successor-op）。工具链为当日源码构建（含 S08）的 `igit`/`git-remote-igit`，安装于 `D:\igit-install\bin`。
- **前置**：链上 main@`52a37ad`（rev 3）。本地 `git pull` 追平后新建 1 个提交 `71dc6b6`，fast-forward `git push igit main` 成功（`52a37ad..71dc6b6`）。
- **增量证据（链上 + 公开 R2 双向核对）**：main → **revision 4**；bootstrap locator 公开 GET 返回 1172 字节 manifest，**SHA-256 与链上承诺一致**；`schemaVersion=2`、`packs=2`：基础包 `e55283e8…`（1743 B）+ 增量包 `fef531d9…`（**755 B**，`dependsOn=[基础包]` 显式闭包）。即本次推送仅上传 1 个新 pack + 1 个新 manifest，未重传既有对象。
- **读取复验**：二次冷克隆 HEAD=`71dc6b6`，`git fsck --strict` 通过，README 含 S08 标记内容完整；tag `refs/tags/v-successor` 同时可读。
- **凭据口径**：R2 writer 密钥与 keystore 密码由用户经环境变量/指定文件提供（密码未回显、未落盘）；测试所用 R2 token 已在对话中暴露过明文，**用户需在 Cloudflare 控制台轮换**。首次尝试因测试网 RPC 瞬时超时失败（suite verification eth_call），重试即成功，无代码影响。

### 2026-10-05 S08 兼容性事件与应急回退（www.igit.xyz 旧前端 × schema 2 manifest）

- **现象**：schema 2 manifest（rev 4）上链后，打开 `www.igit.xyz/.../demo-showcase-byos` 报 "Invalid manifest JSON, schema, context, commitment or limits"。
- **根因（取证）**：线上 bundle（`index-C8HlQA3X.js`）为 S08 之前的构建，反编译确认校验器含 `schemaVersion!==1` 与 `dependsOn.length!==0` 硬检查——遇到 schema 2 链式 manifest **按 ADR 0005 第 4 条设计 fail-closed**。manifest 本身经新版校验器对真实字节+链上承诺验证完全有效（schema=2 packs=2 deps=1，digest-match=true）。非 manifest 损坏，属部署滞后。
- **应急恢复（同日执行）**：`git commit --amend` + force push（非 ff）→ 客户端自动回退全量自包含包：main → **revision 5**（`ded55a8`，schema 1 单包 2237 B，digest-match=true，dependsOn=[]）。浏览器实测页面恢复：文件列表与 README（含 S08 段落）完整渲染，无报错。rev 4 的 schema 2 状态与 R2 对象保留为历史证据。
- **根修（同日完成，CI 自动部署）**：`721228e` 的 `web Vercel production deploy` CI job **SUCCESS**（secrets 已由用户配好），www.igit.xyz bundle 更新为 `index-_26HIkFf.js`，反编译确认校验器已接受 `schemaVersion 1|2`。随后对 demo 仓库做一次正常 fast-forward push（`ded55a8..c9a76b6`，rev 6），浏览器实测新前端渲染 schema 2 增量链完整（文件列表与 README 全部显示，无报错）。同轮推送暴露 `suite-readiness.sh --source-only` 的休眠缺陷：不可变审计基线 `reconciliation-baseline-2026-09-12.md` 记录"已移除的 V2 alpha 注册表合约"被 V1 路径 grep 扫中（该 gate 自基线入库起即休眠失败，`da6c148` 首次触发），`96c8be9` 以 `--exclude='reconciliation-baseline-*.md'` 修复并本地复验 PASS。
- **Windows CI 连锁排查（同日）**：S08 提交首次触发 `cli native Windows` job 后连续失败。经三轮定位（给 go test 步骤加 `::error::` 注解输出后匿名可读失败名）：`internal/chain/successor` 的 `TestEmbeddedSuccessorABIsMatchSolidityArtifacts` 失败——`cli/internal/chain/successor/abi/*.json`（嵌入 ABI 副本）**未在 `.gitattributes` pin `eol=lf`**，Windows runner 以 `core.autrlf=true` 重物化时被 CRLF 化，与 LF 的合约侧 ABI 字节不等（S04/S05 遗留，与 S08 无关；本地 LF checkout 与 Linux CI 均无法复现）。`ae0a145` 补 pin 并把 successor 路径加入 rematerialize 校验清单，该步骤及其后 Kubo smoke、native vet 全部转绿。

### 后续依赖（原实施顺序保留）

- S01–S03 本地实现已落盘并验证；后续扩展基于现有代码，不把 mock PASS 扩展为云或主网验收。
- S03 可以在 Forge/Kubo/云账号均不可用时完成本地切片；不能因旧 P1 全部未过而停工。
- R03 仅阻止受影响的真实配置路径，不能关闭 DACL 保护绕过；R04 只阻止 Foundry 验收，R05 只阻止真实 IPFS 验证。
- R01/R02 影响依赖它们的真实索引/清理，不能运行四个旧脚本主流程或启用 reaper；不阻止新协议与 adapter mock。
- R06 UI 缺失不阻止 S01–S03；完整产品/钱包验收时另处理。R07 的原 IPFS 验收不能替代 S06 BYOS 实证。
- successor 合约与客户端已贯通（S04–S06）；S03 时代的本地 adapter PASS 仍不能单独写成“已支持主网”，主网发布以 publication/mainnet 门禁为准。
- 私有仓库/E2EE、托管服务、额外 provider、双副本、自动 GC/清理和 compaction 另立范围，见 [开放问题](open-questions.md)；增量 pack 已于 2026-10-05 立项为 S08（[ADR 0005](adr/0005-incremental-packs-via-manifest-schema-2.md)）。

用户可修改自己的 Git 内容并发布新 pack/manifest/ref；不得用覆盖同一 digest key 的方式改变链上历史。
本任务不授权读取真实密钥、云写入、交易、部署、公开 profile 切换、commit/push 或清理现有文件。

以下文件保留原样并统一标记为：**“历史迁移草稿/未对齐报告，不作为当前项目状态依据。”**

- 根目录 MIGRATION-COMPLETE.md、MIGRATION-REPORT-P1P2.md、A01-BXX-MIGRATION-REPORT.md，以及其余 MIGRATION-/PRIORITY- 草稿。
- docs/a11-storage-indexer-v2.md、docs/PRIORITY-MAPPING.md。
- docs/evm-v2-handoff.md、docs/liveagent-evm-v2-context.md、docs/project-knowledge-base-zh.md 中的旧状态快照属于 HISTORICAL；架构描述需按当前代码逐条引用。

旧文档中的 P1.1/P1.2/P1.3 分别被用于“operator/部署/激活”和“Moderation/索引/ABI”两套含义，不能互换。旧问题继续使用 R01–R08；新增存储任务使用 S01–S07，不重新编号或覆盖旧问题。

不要继续旧草稿的“仅补测试即可完成”假设。Suite 测试网地址已知且实时可读；索引器的实际阻断是代码不匹配，Moderation 的实际阻断是文件不存在。


---

<details>
<summary>HISTORICAL：本轮入场前原文（已失去当前状态依据效力，完整保留未提交内容）</summary>

以下为历史迁移草稿/未对齐报告，不作为当前项目状态依据。原文 SHA-256：`bc5ef40a14c0e1945bc22e84895e12a7151c807e9b6293c18fe65ae98d6e7932`。下方所有旧状态、勾选、百分比、路径与执行指令仅作审计引用；正确状态以本页上方和唯一事实基线为准。

`````markdown
# iGit EVM V2 项目待办清单

**更新日期：** 2026-09-12  
**当前状态：** A04/A11 核心实现完成，进入测试验证阶段  
**目标：** 测试网 cutover 就绪

---

## 🔴 P1 - 测试网就绪（关键路径）

### P1.1 Web Moderation UI 本地测试 ⏳
**负责：** 前端工程师  
**预计：** 1-2 天

**任务：**
- [ ] 启动本地开发服务器测试 Moderation 标签
- [ ] 验证标签页切换正常
- [ ] 测试举报提交表单（模拟数据）
- [ ] 测试举报列表分页
- [ ] 测试状态设置 UI
- [ ] 检查响应式布局

**验证命令：**
```bash
cd web
npm run dev
# 访问任意仓库页面，点击"Moderation"标签
```

**阻塞项：** 无  
**输出：** 本地功能验证报告

---

### P1.2 V2 事件索引器验证 ⏳ 🔴 **高风险**
**负责：** 后端/存储工程师  
**预计：** 3-5 天

**任务：**
- [ ] **计算实际 RefUpdated 事件签名**
  ```bash
  cast keccak "RefUpdated(bytes32,string,string,string[],address)"
  ```
- [ ] 更新所有 4 个脚本的 `REF_UPDATED_TOPIC`
- [ ] 在测试网运行基础索引器
- [ ] 验证事件查询和解码
- [ ] 测试 checkpoint 保存/恢复
- [ ] 验证 CID 提取正确性

**验证命令：**
```bash
EVM_RPC=https://evm-rpc-testnet.injective.network \
SUITE_DIRECTORY=0x<testnet_address> \
./scripts/evm-event-indexer.sh --once --from-block 1000000
```

**阻塞项：** 需要测试网 Suite 部署地址  
**输出：** 索引器验证报告、checkpoint 文件

---

### P1.3 完善 ABI 解码器 ⏳
**负责：** 后端工程师  
**预计：** 2-3 天

**任务：**
- [ ] 选择 ABI 解码方案（cast/ethers.js/web3.py）
- [ ] 替换 4 个脚本中的 grep/sed 占位符
- [ ] 实现正确的动态数组解码
- [ ] 验证 checksums 和 offsets
- [ ] 添加解码错误处理

**技术选项：**
```bash
# 选项A: Foundry cast
cast abi-decode "RefUpdated(bytes32,string,string,string[],address)" $DATA

# 选项B: Node.js ethers
node -e "const {ethers} = require('ethers'); ..."

# 选项C: Python web3
python -c "from web3 import Web3; ..."
```

**阻塞项：** 依赖 P1.2  
**输出：** 生产就绪的解码器实现

---

### P1.4 添加 Reorg 检测 ⏳
**负责：** 后端工程师  
**预计：** 2 天

**任务：**
- [ ] 扩展 checkpoint 结构存储 block hash
- [ ] 实现恢复时 hash 验证
- [ ] 检测到 reorg 时回退逻辑
- [ ] 测试 reorg 场景

**Checkpoint 扩展：**
```json
{
  "last_indexed_block": "12345",
  "last_block_hash": "0xabc...",
  "updated_at": "2026-09-12T10:00:00Z"
}
```

**阻塞项：** 依赖 P1.2, P1.3  
**输出：** Reorg-safe 索引器

---

### P1.5 安全审查准备 ⏳
**负责：** 技术负责人 + 外部审计  
**预计：** 2-3 周

**任务：**
- [ ] 联系安全审计公司
- [ ] 准备九合约源码包
- [ ] 整理测试覆盖率报告
- [ ] 准备部署 evidence 材料
- [ ] 编写已知限制文档
- [ ] 攻击面分析文档

**交付物：**
- 审计合约包（9 个 Solidity 文件 + ABI）
- 测试覆盖报告
- 部署参数文档
- 已知风险清单

**阻塞项：** 无（可并行）  
**输出：** 安全审计报告

---

### P1.6 清洁验收测试 ⏳
**负责：** QA + DevOps  
**预计：** 1 周

**任务：**

**P1.6a Windows 清洁 E2E**
- [ ] 准备清洁 Windows 11 VM
- [ ] 克隆仓库、安装依赖
- [ ] 编译 Suite 合约
- [ ] 运行完整测试套件
- [ ] 测试 CLI 推送/拉取
- [ ] 记录所有步骤和输出

**P1.6b Linux 清洁 E2E**
- [ ] 准备清洁 Ubuntu 22.04 VM
- [ ] 执行相同测试流程
- [ ] 验证跨平台一致性

**P1.6c Web 钱包验收**
- [ ] 使用 MetaMask 连接测试网
- [ ] 测试所有交易类型（sponsor/moderation/badge）
- [ ] 验证收据确认
- [ ] 测试网络切换
- [ ] 测试拒签场景
- [ ] 记录所有交易哈希

**阻塞项：** 需要测试网授权和资金  
**输出：** E2E 验收报告 + 交易证据

---

## 🟡 P2 - 功能完善（非阻塞）

### P2.1 Recovery UI 实现 📋
**负责：** 前端工程师  
**预计：** 3-5 天

**任务：**
- [ ] 创建 `web/src/pages/Repo/RecoveryTab.tsx`
- [ ] Guardian 配置界面
- [ ] Recovery 提案界面
- [ ] 批准/取消/执行操作
- [ ] Timelock 倒计时显示

**参考：** 复用 ModerationTab 模式  
**阻塞项：** 无（后端函数已完整）

---

### P2.2 Release UI 实现 📋
**负责：** 前端工程师  
**预计：** 2-3 天

**任务：**
- [ ] 创建 `web/src/pages/Repo/ReleaseTab.tsx`
- [ ] Release 注册表单
- [ ] Release 列表显示（按版本）
- [ ] 平台/SHA256 显示
- [ ] 验证功能

**参考：** 简单的 CRUD 界面  
**阻塞项：** 无

---

### P2.3 Username Claim UI 📋
**负责：** 前端工程师  
**预计：** 2 天

**任务：**
- [ ] 添加 "Claim Original Username" 按钮
- [ ] 原所有者验证显示
- [ ] Claim 时间窗口倒计时

**集成位置：** 用户设置页面  
**阻塞项：** 无

---

### P2.4 RPC 错误处理完善 📋
**负责：** 后端工程师  
**预计：** 2 天

**任务：**
- [ ] 实现指数退避重试
- [ ] 速率限制处理
- [ ] 多 RPC 端点故障转移
- [ ] 详细错误日志

**阻塞项：** 依赖 P1.2-P1.4  

---

### P2.5 监控告警集成 📋
**负责：** DevOps  
**预计：** 3 天

**任务：**
- [ ] 索引器进度监控
- [ ] CID 覆盖率告警
- [ ] RPC 错误率监控
- [ ] Checkpoint 延迟告警

**工具：** Prometheus + Grafana 或类似  
**阻塞项：** 依赖 P1.2-P1.4

---

## 🟢 P3 - 长期演进（路线图）

### P3.1 治理去中心化
- 设计 multisig/timelock 层
- DAO 投票机制
- 权限逐步迁移

### P3.2 经济模型扩展
- 多币种支持评估
- ERC20 adapter 设计
- IBC 代币集成

### P3.3 跨链集成
- IBC 桥接可行性研究
- 跨链资产管理
- 统一身份方案

---

## 📊 里程碑时间线

```
当前 (W0)
  ├─ A04/A11 实现完成 ✅
  │
W1: 测试验证周
  ├─ P1.1 本地 UI 测试 ✅
  ├─ P1.2 索引器测试网验证 🔴
  └─ P1.3 ABI 解码器完善 ⏳
  │
W2-W3: 生产准备
  ├─ P1.4 Reorg 检测 ⏳
  ├─ P1.5 安全审查启动 ⏳
  └─ P1.6 清洁验收 ⏳
  │
W4: 测试网 Cutover 就绪检查
  └─ 所有 P1 门禁通过
```

---

## 🚨 当前阻塞项和风险

### 🔴 高风险
1. **P1.2 事件签名未计算** - 索引器无法工作
2. **P1.2 测试网地址缺失** - 无法验证
3. **P1.6 测试网资金** - 无法执行钱包测试

### 🟡 中风险
1. **P1.5 审计周期** - 可能 2-4 周
2. **P1.3 ABI 解码复杂度** - 动态数组处理

### 🟢 低风险
1. P2 所有项目（功能完善，非阻塞）

---

## 📋 每日站会检查清单

### 每日问题
1. P1.2 事件签名计算了吗？
2. 测试网 Suite 地址确定了吗？
3. 索引器能正确提取 CID 吗？
4. 今天有什么阻塞项？

### 每周问题
1. P1 进度是否符合预期？
2. 是否需要调整优先级？
3. 有新的风险吗？

---

## 🎯 Definition of Done

### P1 完成标准
- [ ] 所有 P1.1-P1.6 任务勾选完成
- [ ] 测试网索引器稳定运行 24 小时
- [ ] 清洁 E2E 通过（Windows + Linux）
- [ ] 真实钱包交易成功（至少 10 笔）
- [ ] 安全审计报告收到（无高危问题）
- [ ] 所有文档更新

### 测试网就绪标准
- [ ] 所有九合约部署并验证
- [ ] Suite Directory 激活
- [ ] 索引器运行无错误
- [ ] Web UI 所有核心功能可用
- [ ] 监控告警配置完成

---

## 📁 相关文档

- **迁移报告：** `A01-BXX-MIGRATION-REPORT.md`
- **执行总结：** `MIGRATION-COMPLETE.md`
- **A11 文档：** `docs/a11-storage-indexer-v2.md`
- **项目状态：** `docs/project-status-zh.md`

---

**最后更新：** 2026-09-12  
**下次审查：** 2026-09-13（每日站会）  
**负责人：** 项目协调员

`````

</details>

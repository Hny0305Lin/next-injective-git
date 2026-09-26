# 下一聊天窗口：AWS S3 / Cloudflare R2 BYOS 本地实施提示词

本文件可整份复制到新的聊天窗口。编写日期：2026-09-13（Asia/Shanghai）。
这是实施任务，不是完成报告；工作区状态需在新窗口重新核对。

---

你是本项目的“存储协议、AWS S3/R2 BYOS 与 EVM successor 客户端实施工程师”。

请完整阅读提示词后直接开始**有范围的本地源码和测试实现**，不要再次停下来询问是否开始，
不要只重复上一轮研究/规划，也不要生成另一份“迁移完成报告”。
用户已确认产品选择；你需要把它们落为可运行、可审查的最小实现切片。
本窗口不包含真实云资源、链上交易、公开切换或部署授权。

## 一、唯一工作区、入场核对与事实边界

唯一项目根目录：`D:\inj\next-injective-git`。
上一轮记录：分支 `dev`，HEAD `0ba06f436558f12d97625b393767440cdd0f9862`，
remote 为 `https://github.com/Hny0305Lin/next-injective-git.git`。
这些是 2026-09-13 的记录，不要假设新窗口仍相同。先执行：

```powershell
Get-Location
git status --short --branch
git rev-parse --show-toplevel
git branch --show-current
git rev-parse HEAD
git remote -v
```

检查适用 AGENTS.md 和本项目 Injective EVM skill。记录状态变化，不强行恢复旧分支/提交。
工作树已有未提交文档、历史报告、未跟踪脚本及缓存；它们不是本窗口可清理的临时文件。
四份文档的 HISTORICAL 区必须完整保留，不把旧完成度、旧 P1 编号当作当前事实。

先读下列文件（完整路径）：

1. `D:\inj\next-injective-git\docs\reconciliation-baseline-2026-09-12.md` 正文 1–10 节；具体证据需要时再查附录。文件名保留 2026-09-12，实际审计日期是 **2026-09-13**。
2. `D:\inj\next-injective-git\docs\adr\0002-pluggable-pack-storage.md`
3. `D:\inj\next-injective-git\docs\adr\0003-fresh-evm-suite-and-v1-archive-preview.md`
4. `D:\inj\next-injective-git\docs\adr\0004-mainnet-storage-neutral-successor-and-byos-scope.md`
5. `D:\inj\next-injective-git\docs\storage-byos.md`
6. `D:\inj\next-injective-git\docs\architecture.md`
7. `D:\inj\next-injective-git\docs\backlog.md`
8. `D:\inj\next-injective-git\docs\delivery-roadmap.md`
9. `D:\inj\next-injective-git\docs\open-questions.md`
10. `D:\inj\next-injective-git\docs\acceptance-evidence.md` 和 `D:\inj\next-injective-git\docs\release.md` 的 successor 边界。

事实基线是当次审计记录，不是永远不变的结论。新增实测分别记录命令、日期、代码状态及验证层，
不要覆盖基线或历史证据以制造“都通过”。新任务编号使用 S01–S07；旧问题保留 R01–R08。

## 二、用户已确定的产品选择，不再重复提问

1. **对象存储是主网首期目标要求，主网直接采用 storage-neutral successor。**
   不先发布 IPFS-only Suite v3 主网再立即迁移；当前已部署 v3 不可升级。
2. **首期只支持用户自有 Amazon S3 / Cloudflare R2 云桶。**
   用户在云厂商开设自己的桶属于 BYOS；用户自建对象存储服务不是首期目标。
   不实现 MinIO、OSS、COS、其他云或任意未知 S3-compatible 生产 endpoint。
3. **对象存储路径不能依赖 Kubo、IPFS 主网、iGit 配套 IPFS/replication/gateway 服务。**
   继续保留显式 legacy IPFS 路径，不把它强加给 AWS/R2 用户。
4. **当前 Git 仓库均为公开仓库。**私有仓库、E2EE、密钥分发/撤权是后续独立范围。
   私有 bucket 不等于私有 Git 系统，也不等于匿名 Web 可以读取。
5. **用户自行承担云存储、请求、流量及验证回读成本。**
   首期一个已验证位置即可，双副本只保留扩展设计，不强制 AWS+R2 或 IPFS 双写。
6. **需鉴权读取时使用用户独立 reader 配置。** 不引入平台 GET/upload broker，
   不把 writer 长期凭据给协作者，不要求 Web 保存云 secret。
   标准公开 Web 路径需要稳定匿名 HTTPS GET 和 CORS；不满足时提供清晰限制提示。
7. **Manifest 用 canonical JSON，不用 CBOR。** 以 RFC 8785 JCS 为实现基线，
   先做 Go/TypeScript 字节/摘要交叉测试，再冻结具体 wire schema 和限额。
8. 用户可以修改自己的 Git 数据并发布新 pack/manifest/ref；
   **不能覆盖同一 digest key 的不同字节来“修改”已承诺历史。**
   外部删除/覆盖应被检测；哈希不证明长期可用，也无法恢复丢失数据。

## 三、本窗口的首个交付：S01–S03 实际代码 + 本地测试

先简述可执行计划，然后立即改代码。默认优先顺序如下；不要在计划或空接口/stub 处停下。
S04/S05 属于后续同一产品链路，但必须在协议冻结后才贯通，不能为了展示 push 去绕过 v3。

### S01：协议、manifest 与跨语言 fixtures

- 将 `docs/storage-byos.md` 的候选 schema 转成严格类型/验证器，区分 PackManifest、PackEntry、PackLocation 和外层 ManifestCommitment。
- 包含 chainId/Directory/repoId/ref/commit 绑定、Git object algorithm、pack sequence、raw SHA-256、size、pack version、thin/依赖信息与位置。
- manifestDigest 不进入自己的 hashed body；canonical bytes 固定 UTF-8/JCS，无 BOM/额外换行。
- 严格处理重复 key、无效 Unicode、数组顺序、转义、大整数、溢出、未知字段/版本、大小/深度上限；
  不能用普通 encoding/json 或“排序 JSON.stringify”冒充 JCS。
- digest key 示例：`<prefix>/packs/sha256/<64-lowercase-hex>.pack`、
  `<prefix>/manifests/sha256/<64-lowercase-hex>.json`。
- Go/TS 使用同一正反向 fixture 验证精确 bytes 和 digest；用真实实现生成/比对向量，不手填假 golden hash。
- 初始 successor 新写入采用**自包含、非 thin 完整历史 pack**，不依赖其他 ref 的存活。
  第一个切片先证明 Git SHA-1/pack v2；其他格式只有实际支持后才能宣布通过。
- 只冻结已测试的 schema，不擅自宣告 successor 的链上版本号或地址。

### S02：存储接口、流式文件和 legacy IPFS 适配

- 实现 `cli/internal/packstore` 或符合项目风格的同等 storage boundary。
  先用 adapter 包装现有 IPFS 路径，再接 AWS/R2；chain 层不要直接承担 provider API。
- 明确 writer/reader/capabilities、PutIfAbsent、存储回读验证和 typed result/error。
  Open 返回的未验证数据不能直接交给 Git；默认接口不提供后台 Delete/GC。
- 大 pack 生成与下载写任务独占临时文件，流式算 SHA-256/size，明确取消/文件关闭/恢复与内存上限。
  Windows 上先关闭再重新打开给 Git；digest/size 验证后才运行 index-pack。
- 现有 v3 只有 IPFS URI，没有链上 raw SHA-256/size；
  不能用“下载后自己算一个 hash”声称完成链上完整性校验。保留 legacy 的真实保证边界。
- 保持旧 URI 顺序和兼容测试；当前 string[] 可能是多个不同 pack，不是同一 pack 的镜像。
- AWS/R2 provider selection/preflight 不初始化 Kubo/gateway/replication 依赖；已有 IPFS 测试不能回归。

### S03：AWS S3 / R2 adapters 与 BYOS 配置

- 分开实现 AWS/R2 capability profile，使用锁定且兼容当前 Go 基线的 SDK；查官方资料而非凭“S3-compatible”推断。
- 配置显式包含 provider、bucket、region/R2 accountId、prefix、公开读取位置和本地 credential reference。
  network/chain/Directory 配置与每 repo 的 writer/reader profile 分离；不要改变公开 profile。
- Writer 和 reader 凭据独立；普通 config 只存命名 profile/安全存储/环境变量名称等**引用**，不存秘密值。
- 测试只注入假凭据、假 transport/本地 HTTP server，阻止 SDK 自动读取共享凭据、IMDS/ECS 或真实 AWS/R2 endpoint。
  生产 endpoint 必须验证已知云服务，mock 不能导致生产开放任意 endpoint。
- 条件创建、已存在对象完整回读验证、错误分类、有界重试/取消、multipart 大小限额与恢复、
  SHA-256/实际长度校验及匿名 GET/CORS 诊断都要落为代码/测试，而非 TODO。
- AWS PutObject 与 CompleteMultipartUpload 分别验证；区分 412 幂等复用、409 冲突与 multipart 重建会话。
- R2 PutObject 条件写不能推出 multipart 条件完成也已验证。缺少实证时先提供受限单 PUT 并明确拒绝超限，
  不能 HEAD 后无条件覆盖、忽略条件 header 或悄悄退化为不安全写。
- ETag、用户 metadata/checksum 字段不替代 raw bytes 回读。先实现流式全量 GET 对照 digest/size；
  明确 provider receipt 是客户端观测，不是链上持久化证明，也不新建平台签名服务。
- 限制 HTTPS/redirect/userinfo/query/path，防 SSRF/DNS 重绑定/私网及云 metadata 地址，
  不把云 Authorization 头转发到公开自定义域名。Web 不持有 writer secret。
- 提供可测试的本地配置/doctor 入口和示例；明确尚未接入真实 successor 的功能限制。

若在一个窗口内不能完成全部 S01–S03，优先交付按此顺序已运行的完整切片，
明确剩余代码/测试，而不是把接口空壳、SKIP 或某一 adapter 的 PASS 扩大成整体完成。

## 四、必须阅读/考虑的源码落点

- `D:\inj\next-injective-git\contracts\evm-v2\src\RepositoryCore.sol`
- `D:\inj\next-injective-git\contracts\evm-v2\src\SuiteDirectory.sol`
- `D:\inj\next-injective-git\contracts\evm-v2\src\suite\ISuite.sol`
- `D:\inj\next-injective-git\cli\internal\remote\helper.go`
- `D:\inj\next-injective-git\cli\internal\gitio\gitio.go`
- `D:\inj\next-injective-git\cli\internal\ipfs\client.go`
- `D:\inj\next-injective-git\cli\internal\replication\client.go`
- `D:\inj\next-injective-git\cli\internal\config\config.go`
- `D:\inj\next-injective-git\cli\internal\chain\types.go`
- `D:\inj\next-injective-git\cli\internal\chain\backend.go`
- `D:\inj\next-injective-git\cli\internal\chain\evm_suite_registry.go`
- `D:\inj\next-injective-git\cli\cmd\git-remote-igit\main.go`
- `D:\inj\next-injective-git\web\src\lib\gitstore.ts`
- `D:\inj\next-injective-git\web\src\lib\profile.ts`
- `D:\inj\next-injective-git\web\src\lib\registry.ts`
- `D:\inj\next-injective-git\web\src\lib\transport.ts`

确认当前 updateRef 已有权限、expectedSha 和 RefUpdated，不要描述成“当前没有上链数据”。
当前 `_validatePackUri` 仅允许 ipfs://；helper 先 IPFS/replication 再 UpdateRef；
gitio 当前使用内存 []byte 和 --thin；Web 目前只从 IPFS gateway 下载，缺少新的 raw digest 检验。
以上是入场核对起点，不是禁止重构的新协议约束。

## 五、S04/S05 的不可绕过边界

- 当前 v3 Suite 不可升级，不允许 HTTPS 伪装为 ipfs:// 或 object reference 混入 legacy PackURIs。
- 在上传前按 chainId/Directory/明确 suiteVersion 选择协议；未知版本或不匹配的 BYOS+v3 组合 fail closed。
- successor 推荐当前 state 保存 manifest hash/size/稳定 bootstrap locator；无 indexer 的冷客户端也能 getRef 后读取，不能仅依赖事件或循环 profile lookup。
- 同 commit 的位置/manifest 变更也要 CAS；force 不豁免 successor 并发保护，delete/recreate 要避免 revision ABA。
  合约不能证明 Git ancestry 或云端可用；这些与客户端策略分开。
- fork/new-ref 的 context-bound manifest 必须绑定目标 repo/ref，不能原样复制源 commitment。
- 合约 state/query/event、ABI/artifacts、Go/Web/indexer/evidence gate 必须是同一 reviewed 版本。
  ABI 变更先完成协议向量，保留 legacy ABI 与历史部署证据；不能改旧证据消除 mismatch。
- 保持 Directory 唯一信任根、模块 code hash/绑定、owner/collaborator/moderation/recovery 约束，无 proxy/混合写回退。
- 新测试 Suite 推荐从空状态开始；是否导入现有 v3 测试数据是后续独立选择，V1 继续只读 archive preview。
- 本轮可做本地 mock-chain/Foundry 状态测试，但不得真实部署、签名、广播或改公开 Directory。

## 六、必须覆盖的失败/攻击用例

除正常上传/读取外，至少按当前切片覆盖：

- duplicate key + 相同/不同内容，412/409/429/5xx、鉴权失败、超时、取消、限额与早关响应。
- 错 digest、错 size、截断、额外尾随 bytes、内容变换、metadata/ETag 伪匹配、manifest 自引用/未知字段/跨 repo 替换。
- pack 上传后 manifest 失败、上传成功但 ref 失败、CAS 冲突、receipt 不确定；
  不更新 ref、不盲目新 nonce 重发、不立即 GC，保留定向恢复所需非秘密数据。
- 原对象被删除/覆盖、一个 location 失败、独立 reader 缺配置、公开 GET 能读但 CORS 失败。
- new branch/tag、零增量、force、其他 ref 删除、fork 上下文及目标 commit 可达性。
- 无 Kubo、无 iGit 服务的 AWS/R2 路径；生产拒绝 MinIO/未知 endpoint，但本地测试 transport 可注入。
- 日志、错误、普通配置、journal、snapshot、Git remote 和 Web localStorage 无秘密值或预签名 URL。

Multipart abort、对象删除和生命周期回收都不是本轮可执行的自动故障处理，
包括 SDK 默认隐式行为也须检查；记录会话/孤儿对象，后续另取精确授权。

## 七、验证分层，不把全部旧问题当成前置

- 可立即完成：源码、协议、mock/local HTTP provider tests、Go/TS 交叉向量、真实本地 Git fixture、文档。
- Forge 缺失只阻止对应 Foundry 验收；Kubo 缺失只阻止真实 IPFS 集成；它们不阻止对象存储本地实现。
- R03 DACL 阻塞只在相关 CLI 配置路径处理，不能禁用权限保护掩盖问题。
- R01/R02 indexer 缺陷阻止依赖它的真实索引/回收，不阻止新接口设计；四个旧脚本主流程禁止运行。
- R06 Moderation UI 缺失不阻止 S01–S03，完整产品/钱包验收再单独处理。
- 无云账号/真实凭据则完成 mock；真实 AWS、真实 R2 分别标 BLOCKED 或 NOT PROVEN，不能称已闭环。
- 旧 Suite 快照仅作背景：2026-09-13 固定区块 139852506，chainId 1439，version 3；
  Directory `0x24124cb60f9ef02f7deb5bc868c028fb412f5334`，Coordinator `0x329921023FCf6E337E924686b92f7521549B5970`。
  不当作新窗口实时状态，不向它们发送 successor payload。真实链读取需要时先验 chainId 再固定区块读取。

先检查测试脚本/环境，不意外触发真实服务。运行本切片新测试，再按改动范围运行：

- CLI：`go test -count=1 ./...`、`go vet ./...`；race/跨平台编译在对应环境可用时执行并分别记录。
- Web：`npm run test:api`、`npm run typecheck`；涉及构建时使用新任务独占的 ignored 输出目录，禁止自动清空已有 dist。
- Contracts：涉及变更时先 `npm run check`，并在 Forge 确实可用时运行 build/test/invariant/gas；否则报告对应 BLOCKED。
- ABI、源码、generated artifacts、依赖锁文件和文档保持一致；不伪造运行输出、不把 SKIP 记为 PASS。

验证前确认 TEMP/TMP/SystemRoot/SystemDrive 等运行环境路径有效，只检查必要非秘密字段，不打印整个环境。
不新建相对 `null` 或 `%SystemDrive%` 缓存；隔离测试配置，别借默认 SDK/profile 读取用户已有凭据。

## 八、安全与工作树规则

- 手工编辑用 apply_patch，只修改本任务需要的源码、测试、依赖和 docs；保留用户已有未提交内容。
- 不 git reset、git clean、force push，不自动 commit/push，不切换公共部署配置。
- 不删除、移动用户现有文件，不清理已登记的异常缓存。
- 不读取、打印、导出用户私钥、助记词、云密钥或已有秘密文件，不让用户粘贴秘密到聊天。
- 不真实建桶、上传、覆盖、删除、改 bucket policy/CORS/lifecycle；不能用 doctor 隐式写云。
- 不真实签名/发送交易/主网操作，不自动部署，不用 v3-only gate 宣称 successor 可以切换。
- 不运行 IPFS repo GC、自动 unpin 或以下旧索引脚本主流程：
  `D:\inj\next-injective-git\scripts\evm-event-indexer.sh`、
  `D:\inj\next-injective-git\scripts\evm-hot-pin-indexer.sh`、
  `D:\inj\next-injective-git\scripts\evm-archive-indexer.sh`、
  `D:\inj\next-injective-git\scripts\evm-replication-reaper.sh`。
- 搜索排除 node_modules/dist/.git 和已登记的 `cli/null`、`contracts/evm-v2/%SystemDrive%`、
  `contracts/evm-v2/null`、`web/%SystemDrive%`、`web/null`；不能换工具绕过此前被拒绝的清理。
- 涉及服务商 API、SDK、canonical JSON/Git 细节，核对官方原始文档并记录实际查询日期；
  `docs/storage-byos.md` 的 2026-09-13 引用不是永久能力保证。

## 九、最终交付格式

1. 本窗口实际完成的代码切片、工作树基线和完整绝对文件路径；区分已有修改与本窗口改动。
2. 协议/配置/API 的具体行为和已冻结/尚待冻结的细节，以及可运行的本地验证方式。
3. 按检查项列出命令、输出摘要和状态，仅使用 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL。
4. 分开列出本地/mock、真实本地 Git、真实 AWS、真实 R2、真实 IPFS、successor 合约/部署、Git E2E 的结果；未执行不写通过。
5. 失败恢复/秘密与 endpoint 防护的覆盖情况；哪些源码/测试仍未完成。
6. 同步 S01–S07 待办及相关 docs，不改原始事实基线/HISTORICAL 内容，不新造完成报告。
7. 下一切片和真正需要用户配合的资源/授权，说明缺少它仅阻止哪一步；不要提前索取云密钥。

本轮成功是得到可审查、真实运行测试的本地 AWS/R2 BYOS 基础，
不是宣布主网、云服务或迁移已经完成。请直接开始。

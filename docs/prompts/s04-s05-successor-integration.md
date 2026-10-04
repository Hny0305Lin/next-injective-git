# S04 链上承诺 + S05 CLI/Web 接入：实施提示词

编写日期：2026-10-04（Asia/Shanghai）。本文件可整份发送给接手模型。
这是工程实施任务提示词，**不是**部署、交易或公开 profile 切换授权；真实测试网写入与云写入仍按阶段单独取得用户明确批准。

---

你是本项目的「successor 协议与客户端接入工程师」。目标：实现 S04（链上 manifest 承诺）与
S05（CLI/Web 接入），让 BYOS 内容获得**链上可发现、链上可验证**的入口，最终替代
「公开 URL + 承诺值带外分发」的过渡方案。先本地/mock 实现，真实链与真实云逐阶段另行授权。

## 1. 协作方式

- 先读第 2 节文件核对现状，不要凭旧报告假设；引用代码给文件:行号。
- 状态只用 PASS / FAIL / BLOCKED / NOT PROVEN / HISTORICAL；不为绿灯降低完整性/权限/endpoint 校验。
- 每个阶段输出：改动文件清单、测试命令与实际输出、未覆盖层（NOT PROVEN 清单）。
- 不新增代理/线程；失败先定位并补最小回归测试；不重写历史证据或基线文档。

## 2. 必读文件（按序）

1. `docs/storage-byos.md` —— §2 链上/链下数据形状、§5 发布与读取流程（含故障恢复契约表）、§6 实现落点、§9 已冻结 schema 1 与已验证 API/限额。
2. `docs/adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md` —— BYOS 范围决策。
3. `docs/delivery-roadmap.md` —— S04/S05 交付边界与 P4 出口条件。
4. `docs/backlog.md` —— S01–S07 与 R01–R08 现状。
5. `docs/reconciliation-baseline-2026-09-12.md` —— 唯一事实基线（R01–R08、v3 合约核对、已知阻断）。
6. `docs/prompts/next-storage-implementation.md` —— 上一切片的安全边界。
7. 已实现源码：`cli/internal/packmanifest`（JCS schema 1）、`cli/internal/packstore`（含 `prepare.go`、`s3store`、`ipfsstore`）、`cli/internal/storageconfig`、`cli/internal/safehttp`、`cli/internal/gitio/verified.go`、`cli/cmd/igit-storage-canary`（真实云 canary 入口）。
8. 实测证据：`local-only/byos-canary/r2-01`、`r2-02`（真实 R2 全链路记录）。

## 3. 当前事实基线（2026-10-04，防旧状态混淆）

已完成并验证：
- S01–S03 本地 PASS：canonical manifest schema 1（Go/TS 交叉向量）、verified packstore、AWS/R2 adapter。
- **真实 Cloudflare R2 canary 全链路 PASS**（桶 `haohanyhak49`，公开域
  `https://pub-47dea1b44bd54384850b5555359c0289.r2.dev`，CORS 已配置，ACAO 验证通过）：
  条件 PUT（If-None-Match:*）+ 全量回读、同字节 412 幂等复用、manifest 上传、独立 reader 冷读、
  空 bare 仓库 `index-pack --strict` + fsck 全可达、tree/log 一致、匿名公开 GET 字节验证 + CORS。
- 可复用实测资产（非秘密）：prefix `igit-canary/r2-01`；pack
  `c06a84ce3e8e4cdf97173f6861e855e521837a4a85279fa9e5724b9b07393c63`（1502B）；manifest
  `f2d256a5446dfdc54ca7eb284a824ea64ddd089190332e5e6bfc2ac0e2f8a22b`（778B）；commit
  `1d299f0c944e5611d382ce86fced6a7d7516b407`（refs/heads/main，仓库 demo-showcase-byos）。
  凭据环境变量名：`IGIT_CANARY_R2_WRITER_KEY/SECRET`、`IGIT_CANARY_R2_READER_KEY/SECRET`（值永不出现在仓库/对话/文件）。

未完成/阻断（不得当作已解决）：
- S04/S05/S06/S07 全部 NOT PROVEN；successor 合约、版本化 ABI、CLI/Web 接入未实现。
- v3 现状：`RepositoryCore.GitRef` 只有 `commitSha + packUris(ipfs://)`，无 raw digest/size/CAS；
  生产尺寸余量仅 1072B（23504/24576）。
- R03 BLOCKED：本机 D: 盘目录 `fileprotection.ProtectFile` 报 DACL Access denied（TEMP 正常）——
  journal 类受保护文件放 TEMP 独占目录，**不得关闭保护**。
- R04 BLOCKED（无 forge）、R05 BLOCKED（无 Kubo）、R01/R02 FAIL（四个旧索引/reaper 脚本禁用）、
  R06 FAIL（Moderation UI 未实现）。
- 普通用户路径仍走 legacy v3 IPFS；`git-remote-igit` 与 Web `gitstore.ts` 尚无 BYOS 分派。

## 4. S04 任务目标：successor 链上承诺（新合约，不改 v3）

1. 设计 fresh successor 套件（延续 ADR 0003 的 fresh-suite 路线；suiteVersion 由审查确定，不在文档预编）：
   - ref 状态：`manifestDigest(bytes32) + manifestSize(uint96/有界) + bootstrapLocator(有界 HTTPS string) + revision(uint64) + updatedAt/By + exists`；
   - **revision CAS**：更新须携带 expected revision + expected 旧 commitment；`force` 不豁免 CAS；
     delete/recreate 用单调 revision 防 ABA；同 commit 换 manifest/位置走 CAS；
   - fork 必须重新生成绑定目标 repo/ref 的 manifest，不得复制源 commitment；
   - 未知协议版本在上传前拒绝；合约校验权限、长度、已知版本，**不读取链外字节**。
2. 事件 v2：可还原完整 refName（indexed string 只得 hash 的问题按 §2 事件行处理），绑定
   repo/ref/commit/manifest 承诺/revision/updatedBy。
3. ABI/artifacts、Go chain 类型/解码器、Web 解码器、evidence schema **同版本冻结**；v3 ABI 与
   历史部署证据原样保留；不把 HTTPS 伪装成 `ipfs://`，不向 v3 PackURIs 塞 manifest。
4. 尺寸与验收前置：新增状态/事件后重算生产合约尺寸余量（对照 24576B 上限）；
   R04 解除前 Foundry 层如实 BLOCKED，不得用 solc check 冒充。
5. mock 链垂直流：fake chain + fake cloud + 真实本地 Git fixture 驱动
   publish→CAS 成功/冲突→delete/recreate→fork→冷 clone→失败恢复（对齐 §5 故障契约表：
   上传成功但 ref 冲突、receipt 未知、reorg 假设等），全部先在测试层闭环。

## 5. S05 任务目标：CLI 与 Web 接入

CLI（`cli/internal/remote/helper.go`、`gitio`、`config`、helper main）：
1. 存储中立分派：按链上 ref 形态走 legacy v3 IPFS 或 successor BYOS；BYOS 路径不初始化
   Kubo/replication。
2. push：`packstore.Prepare`（先验证全部源→pack→manifest 条件上传→回读）→ 构造 CAS ref 更新
   （经既有 `EVMTransactor` 语义：nonce/legacy 签名/回执轮询/不确定回执返回 tx hash 并失效缓存）。
3. clone/fetch：验证 Suite → 读 ref commitment → bootstrapLocator/公开域取 manifest →
   digest/size 预检 → JCS Parse → 逐 pack `ReadVerified` → `IndexVerified`（先验证后摄取）。
4. `igit init/config` 集成 storage profile（沿用 storageconfig 的绑定与凭据引用边界）。

Web（`web/src/lib/gitstore.ts`、`profile.ts`、`registry.ts`、`transport.ts`）：
1. manifest verified reader：受限长度 fetch → 对照链上承诺 → TS `Parse` → 逐 pack 下载验证 →
   isomorphic-git 摄取；单包/总量内存上限，超限拒绝。
2. CORS/鉴权失败给可操作提示（提示配公开读或独立 reader），不索取/存储任何云 secret。
3. Suite Directory 配置路径沿用现有本地 Settings 机制；不改内置公开 profile 的空 Directory 策略。

## 6. 验收标准（分层；真实层逐项单独授权）

本地/mock（本提示词授权范围）：
- [ ] `go vet ./...`、`go test -count=1 ./...` 全绿；新增 successor 合约/解码/CAS/fork/ABA 测试。
- [ ] fake chain + fake cloud + 真实 Git 的 push/clone/fetch/failover fixture 全绿。
- [ ] Web：`npm run test:api`、`npm run typecheck` 全绿；manifest reader 单元与失败注入测试。
- [ ] 交叉回归：legacy v3 IPFS 测试不回退；`test:storage-cross` 向量不变。
- [ ] 尺寸余量报告 + R04 BLOCKED 如实记录（或 forge 可用后补 Foundry unit/invariant/gas）。

真实测试网（需用户届时明确授权交易与资金确认）：
- [ ] 部署 fresh successor 套件（轮换的加密测试网 key；no-clobber 证据；Blockscout 验证）。
- [ ] 发布 demo-showcase-byos 的 ref 承诺（manifest f2d256a5.../778B，bootstrap 指向公开域），
      链上 getRef 可读回全部承诺字段。

真实云与端到端（沿用既有桶/公开域/CORS；云写入范围仅 `igit-canary/` 约定 prefix 或另行批准）：
- [ ] igit.xyz：在本地 Settings 配 successor Directory 后，仓库 commits/files/refs 原生可见
      （无需公开 URL 带外分发）。
- [ ] 干净 Windows（及后续 Linux）上 `git clone igit://<owner>/<repo>` 冷克隆成功，全程无
      Kubo/WSL2/injectived；下载字节逐层验证。
- [ ] 回归：legacy v3 读写不受影响；四个旧索引/reaper 脚本保持禁用。

## 7. 安全红线（浓缩，全文以 CLAUDE.md 与基线为准）

- 不主网；不未授权广播；测试网交易与资金单独确认。凭据值只进执行进程环境，配置/manifest/日志/
  浏览器只存引用与摘要。不向 v3 Directory 发送 successor payload。
- 不自动 delete/GC/abort/lifecycle；无 broker；生产 endpoint 仅 aws-s3/cloudflare-r2 官方派生。
- journal/受保护文件在本机 D: 盘 DACL 失败时改用 TEMP 独占目录，不关保护（R03 口径）。
- 使用 checked-in ABI，不手写 selector/word decoder；CLI 用户可见消息走 i18n 双语。
- 历史部署/激活/迁移证据不可改写；本提示词不授权 commit/push 公开分支。

## 8. 起手步骤

1. 读第 2 节文件；`git status`/`git log` 核对 S01–S03 与 canary 源码确实在位（大量未提交文件，勿只看 HEAD）。
2. 产出 S04 合约设计草案（状态/事件/CAS/限额/尺寸估算，diff 级），交用户审查通过后再写实现。
3. 实现顺序：合约+测试 → Go 解码/mock 链流 → CLI helper → Web reader；每步跑第 6 节对应命令。
4. 真实层（部署/交易/云写/公开 profile）每阶段先向用户提交资源与范围确认表，批准后执行。

现在请从第 8 节第 1 步开始，先给出你对现状的核对结论与 S04 设计草案计划。

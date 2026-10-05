# AWS S3 / Cloudflare R2 BYOS：陪同用户实测提示词

> **状态（2026-10-05）：核心实测已完成，本文为历史任务书。** 真实 Cloudflare R2
> 全链路（条件 PUT + 回读、JCS manifest、CAS 上链、匿名公开 GET+CORS、冷 clone）
> 已于 2026-10-04 通过（记录见 `local-only/byos-canary/r2-01`、`r2-02` 与
> [backlog](../backlog.md) 2026-10-04 小节）；**真实 AWS S3 canary 仍未执行**。
> 不要再按本文重新启动 R2 实测；AWS canary 可参考本文的分层授权流程。

编写日期：2026-09-14（Asia/Shanghai）。本文件可以整份发送给接手模型。
这是实测协作任务，不是新的完成报告，也不是云资源、公开策略、删除或链上交易授权。

---

你是本项目的「BYOS 实测工程师与用户操作向导」。请在实际工作区核对代码，先复验已经实现的本地功能，再逐步配合我完成明确授权的真实 AWS/R2 小范围测试。
不要只复述计划，不要假装已有云上传 CLI，不要把 mock、编译或本地 Git fixture 写成主网/Git remote E2E 已完成。

## 1. 合作方式：先做不需要我的步骤，再逐步让我操作

- 你能执行的本地检查先直接执行；没有终端/文件权限时明确说明，给我可复制命令，等待真实输出。
- 每轮需要我配合时，只提出当前阶段的 1–3 个具体操作，按「做什么 → 命令/控制台位置 → 成功信号 → 我该返回哪些非秘密信息」组织。
- 不让我粘贴 access key、secret、session token、私钥、助记词、cookie、预签名 URL 或包含它们的截图。
- 不把服务缺失、账号未准备或某个 provider 阻塞扩大成整个任务无法继续。先完成可做的本地层，两个 provider 分别记录。
- 每个测试记录实际命令、运行日期/时区、源码状态、环境、退出码和验证层。状态仅使用 PASS、FAIL、BLOCKED、NOT PROVEN、HISTORICAL。
- 失败先定位，必要时补最小回归测试及修复；不为绿灯降低完整性/权限/endpoint 校验，不擅自更新 golden fixtures。
- 不新增线程/代理或委派工作，除非我另行要求。不要另造“迁移全部完成”之类报告。

## 2. 入场核对与事实边界

唯一项目根目录：D:/inj/next-injective-git。
上一轮记录：分支 dev，HEAD 0ba06f436558f12d97625b393767440cdd0f9862。
S01–S03 新源码、fixtures、测试和文档当时均有未提交内容；只 git clone 同一 HEAD 不会得到这些实现。
如果在另一台机器/另一模型工作区缺文件，先暂停相关测试，由我提供完整非秘密源码及未跟踪文件；不能凭 HEAD 宣称环境一致。

先执行以下只读核对，不切分支、不恢复文件：

~~~powershell
Set-Location D:/inj/next-injective-git
Get-Location
git status --short --branch
git rev-parse --show-toplevel
git branch --show-current
git rev-parse HEAD
git remote -v
Get-Command go,node,npm,git | Select-Object Name,Source
go version
node --version
npm --version
git --version
foreach ($name in @('TEMP','TMP','SystemRoot','SystemDrive')) {
    $value = [Environment]::GetEnvironmentVariable($name)
    [pscustomobject]@{ Name=$name; Value=$value; Exists=($value -and (Test-Path -LiteralPath $value)) }
}
~~~

检查适用 AGENTS.md；需要处理 Injective/EVM 客户端时读取项目 Injective EVM skill。
搜索排除 .git、node_modules、dist，以及既有 cli/null、contracts/evm-v2/null、contracts/evm-v2/%SystemDrive%、web/null、web/%SystemDrive%。
这些既有异常目录、历史报告、未跟踪脚本不是本次可清理临时文件。

至少阅读：

1. D:/inj/next-injective-git/docs/storage-byos.md，尤其第 9 节的实际 API、限额和验证记录。
2. D:/inj/next-injective-git/docs/adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md。
3. D:/inj/next-injective-git/docs/backlog.md 当前 S01–S07 区；保留其 HISTORICAL 区。
4. D:/inj/next-injective-git/docs/prompts/next-storage-implementation.md 的安全边界。
5. D:/inj/next-injective-git/docs/examples/storage-byos.json。
6. D:/inj/next-injective-git/docs/reconciliation-baseline-2026-09-12.md 的相关原审计记录，不改写它。

### 当前已实现 / 尚未实现

- 已实现 Go/TS canonical manifest、严格验证和共享向量；verified packstore；本地完整历史 Git pack；AWS/R2 adapter；独立配置；本地 journal 与恢复。
- 现有命令只有 igit storage doctor <file> 和 igit storage show <file>，两者都不访问云、不解析凭据、不上传。
- 当前没有现成的 igit storage upload/canary/clone 命令，也没有接通普通 git-remote-igit 的 BYOS push。
- 当前普通 helper / Web gitstore 仍使用 legacy Suite v3 IPFS。successor ABI、revision CAS 和客户端接入属于 S04/S05，不能借旧 v3 验证。
- 不真实签名、广播、部署或切换公开 Directory；不把 HTTPS 包装成 ipfs://，不向 v3 PackURIs 塞入 manifest。
- 首期是用户自有 AWS/R2 云桶、公开仓库、独立 reader、单个验证位置即可；不实现 MinIO/任意 S3-compatible，不要求双写或私有仓库加密。

## 3. 阶段 A：不需要云账号的本地复验

本阶段无需问我云密钥或桶信息。检查脚本没有新增默认联网/真实集成行为后执行。
如依赖缺失，先说明缺什么；依据锁文件安装，不盲目升级 SDK/依赖，不用 npm ci 清空现有依赖目录来掩盖问题。
每条命令失败立即保留输出并定位；不要只看同一 shell 最后一条命令的 exit code。

### A1. 本地配置入口

~~~powershell
Set-Location D:/inj/next-injective-git/cli
go run ./cmd/igit storage doctor ../docs/examples/storage-byos.json
if ($LASTEXITCODE -ne 0) { throw 'storage doctor failed' }
go run ./cmd/igit storage show ../docs/examples/storage-byos.json
if ($LASTEXITCODE -ne 0) { throw 'storage show failed' }
~~~

成功信号：示例是 4 profiles / 2 repository bindings；只显示环境变量名称等引用，不出现秘密。
示例中的 bucket、accountId、Directory、repoId、域名都是占位值，不能拿它直接上云。
本步骤不能证明任何云身份、IAM/R2 token 权限、GET、CORS 或 successor 可用。

### A2. 目标测试和真实本地 Git

~~~powershell
Set-Location D:/inj/next-injective-git/cli
$env:IGIT_RUN_NATIVE_KUBO_INTEGRATION = '0'
go test -count=1 ./internal/packmanifest ./internal/packstore/... ./internal/storageconfig ./internal/safehttp ./internal/gitio ./internal/ipfs ./cmd/igit
if ($LASTEXITCODE -ne 0) { throw 'storage tests failed' }
go test -count=1 -v ./internal/gitio -run '^TestRealGitIndependentFullHistory$'
if ($LASTEXITCODE -ne 0) { throw 'local Git fixture failed' }
go test -count=1 -v ./internal/packstore/s3store
if ($LASTEXITCODE -ne 0) { throw 'provider contract tests failed' }
~~~

检查实际 test 输出，至少确认以下场景：

- 完整历史、非 thin、SHA-1 / pack v2，新分支、annotated tag、零增量、回退提交，删除源仓库 sibling refs 后仍能在空仓库摄取。
- 错 digest/size、截断、尾随字节、内容变换、源文件被改、目标 commit 不可达均被拒绝。
- AWS/R2 条件写、412 同字节复用、不同字节拒绝；409/429/5xx、超时、取消、认证失败和有界重试。
- AWS multipart 完成 409 重建、失败会话保留、checkpoint 失败停止；R2 超过 16 MiB 在网络前拒绝。
- 独立 reader 缺配置失败；reader 不可写；公开 GET 不携带 writer secret；GET 可读但 CORS 不通过分开报告。
- 本地 protected receipt 写/读、先查最终对象的恢复、manifest 失败保留 pack，不自动 abort/Delete/GC。

这里 provider tests 使用 fake transport/httptest；只能记本地/mock PASS。
本地 Git fixture 确实使用 git 二进制，但不是 git clone igit:// 或链上 ref E2E。

### A3. Go/TypeScript 交叉验证与回归

~~~powershell
Set-Location D:/inj/next-injective-git/web
npm run test:storage-cross
if ($LASTEXITCODE -ne 0) { throw 'Go/TS parity failed' }
npm run test:api
if ($LASTEXITCODE -ne 0) { throw 'Web API tests failed' }
npm run typecheck
if ($LASTEXITCODE -ne 0) { throw 'Web typecheck failed' }

Set-Location D:/inj/next-injective-git/cli
$env:IGIT_RUN_NATIVE_KUBO_INTEGRATION = '0'
go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { throw 'Go regression failed' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
~~~

上一轮参考值为 11 个正向/55 个反向 shared vectors、140 个 Web tests、0 skipped；这些是 HISTORICAL 参考，不是本轮应填写的输出。
不能默认运行 npm run fixtures:storage：它会重生成期望向量，不是只读验收命令。确需协议修改时，先报告差异、理由和交叉结果。

### A4. 额外平台检查（分别报告，不扩大结论）

- go env CGO_ENABLED；只有本机 C 工具链确实可用且环境正确才执行 race。上一轮 CGO=0，race 为 BLOCKED，不应直接声称已解决。
- 可以在独立 shell 中设 GOOS=linux、GOARCH=amd64、CGO_ENABLED=0，执行：
  go build ./cmd/igit/... ./cmd/git-remote-igit/... ./internal/packstore/... ./internal/gitio/...
  这是多 package 交叉编译，不生成/覆盖已有二进制，不是 Linux 运行验收。
- 要证明 Linux Git 行为，必须在真实 Linux 用相同源码切片运行对应测试；不要用跨编译代替。
- 没有必要就不跑 Web build；若需 build，选本任务独占的 ignored 输出目录，显式不清空现有 dist。
- 原生 Kubo smoke、Foundry、钱包、链上验收仍单列；本阶段不启用它们。
- 包含旧文档的 git diff --check 可能报告 HISTORICAL Markdown 尾随空格；保留历史原文，单独检查本轮新增源码。

阶段 A 完成后，给我一张本轮结果表，再进入下一个有用步骤；不要直接接触真实云资源。

## 4. 阶段 B：准备最小云实测入口，而不是捏造现有命令

先检查是否已有后续新增的 canary 工具；有则审查并复用。
如果仍只有 doctor/show，可以补一个小范围、本地可审查的专用工具，例如放在：
D:/inj/next-injective-git/cli/cmd/igit-storage-canary/。
这个位置在 Go internal 导入允许的模块范围内；不要把工具放到仓库外再通过复制实现绕过 internal 限制。

这里只授权作为实测所需的最小本地源码/测试补充，不要求实现 S04/S05 或新生产服务。
以下是拟议工具需求，不是已经存在的 CLI 参数。实现并测试后，再根据真实 --help 给我最终命令。

### 工具必须满足

1. 默认 plan/dry-run：不解析真实凭据、不做 DNS、无网络。展示选定 provider/profile、桶和 prefix、本地源路径、digest/size、拟执行 API、对象清单、预计上传/回读量和最坏重试预算。
2. execute 需要显式开关，并绑定我批准的 provider、bucket、prefix、场景、对象字节上限和请求预算。配置变化/源码变化导致计划不一致时重新确认，不因传了开关就扩大授权。
3. 真实路径复用生产 API，不复制协议/签名/上传逻辑，不使用 httptest 注入入口：
   - storageconfig.Load / Select；
   - s3store.NewWriter / NewReader；
   - PutIfAbsent / PutRecoverable / VerifyStoredBytes；
   - packstore.Readers / ReadVerified；
   - packmanifest.Encode / Parse；
   - gitio.PackFullHistory / IndexVerified；
   - SaveReceipt / LoadReceipt / Recover、DiagnosePublic。
4. 使用受保护的本地 journal。每个运行固定对象和 session 的归属；journal 必须绑定 provider、bucket、region/account、prefix 与本次批准计划，不能只靠同名 key 在另一个桶误恢复。
   现有 Receipt 不是完整 plan/云配置绑定 envelope，工具必须检查外层运行上下文；不把本地 journal 当链上证明。
5. 用真正的本地 Git fixture 生成 pack，不用真实业务仓库或用户文件。不上传 protocol/vectors.json 里的“假 pack”当成真实 Git pack。
6. 小对象预算优先：建议每个 pack 不超过 1 MiB。先保留一份确定的源文件并计算 raw SHA-256/实际 size，后续重复上传必须使用完全相同字节，不能每次重新打包再说“同一 key”。
7. 读取时预期 digest/size 来自独立保留的本地计划/commitment，不来自 GET response metadata。冷读取不使用源仓库 object database、writer 内存或缓存。
8. 标准 go test ./... 仍只跑无云测试。canary 不因环境里有 AWS/R2 变量就自动触发；不启动 Kubo、replication、gateway 或旧索引脚本。
9. 输出仅保留允许的事实：操作名、状态码、字节数、digest、验证布尔值、局部恢复标识等；不输出 HTTP 全量 debug、Authorization、session token、SDK credentials 对象、签名 query。
   若要证明 If-None-Match:* 和 412/Complete 409，需要最小白名单诊断或已脱敏 provider 证据；不能通过替换生产安全 transport 来抓包。
10. 并发/字节/请求预算包括 SDK 重试和验证 GET。给每个操作 context deadline，失败停止，禁止脚本无限重试。确需重跑，先检查已有 receipt 和已消耗预算。
11. 没有 DeleteObject、AbortMultipartUpload、改 policy/CORS/lifecycle、建桶或发链交易子命令；故障也不能隐式执行它们。
12. 先用 fake/local tests 验证 plan 默认零请求、执行门控、凭据隔离、预算/配置绑定及秘密脱敏，再允许真实运行。

建议把新测试产物、非秘密运行计划、配置引用和证据放在新建的：
D:/inj/next-injective-git/local-only/byos-canary/<本次唯一run-id>/。
它在现有 ignored 规则内，但 ignored 不等于保密；journal 仍须执行 OS 权限保护，配置中绝不能保存秘密值。
目录必须由你创建并验证绝对路径，不覆盖/搬走/清空既有文件。不为了测试重建用户已有 ~/.igit 或云凭据目录。

暂时不能补工具时，明确标记真实 adapter canary BLOCKED，并给出缺少的最小入口；不要把 aws CLI 或控制台手动上传的结果当成本仓库 adapter 验收。


## 5. 阶段 C：由我准备云资源和独立凭据，分批确认权限

阶段 A 和工具 dry-run 可先做完。进入真实云前，每次只向我索取当前 provider 必需的非秘密信息：

- 本轮先测试 AWS 还是 R2（另一个可以稍后测试，不强制两个同时有账号）。
- 我自有的专用测试 bucket、AWS region 或 R2 accountId，准备使用的唯一 prefix。
- 是否只验证独立鉴权 CLI，还是还验证公开读取；公开域名/publicReadBase、真实浏览器 Origin。
- 允许的对象清单/数量、最大上传和回读字节数、最大请求次数、截止时间、费用上限。
- 凭据变量已在将执行 canary 的进程环境安全配置：只需“已配置/未配置”，不要值。

资源准备由我在云厂商控制台操作，或者在另行明确授权后由你操作；本提示词不授权自动建桶或改策略。
厂商 UI/API 可能变化，使用当次官方文档并记录日期；不凭“S3-compatible”推断 AWS/R2 能力相同。

### C1. AWS S3 专用测试资源

1. 使用我自有的普通 S3 general-purpose bucket，bucket 在当前代码支持的商业 region 内。
   当前配置校验不接受含点 bucket 名、directory bucket、access point ARN、未知 region/partition 或自建 endpoint；先本地 doctor 验证，不用降低校验迁就现有桶。
2. 使用独立测试 prefix，例如 igit-canary/<run-id>，不要指向真实仓库历史或其他应用目录。
3. writer 使用仅针对该 bucket/prefix 的专用身份。当前调用需要 s3:PutObject 和 s3:GetObject；AWS create/upload/complete multipart 的权限按当次官方 IAM 文档核对，不虚构单独的 IAM action 名称。
4. reader 使用另外的仅 GET 身份，不能复用 writer。需要临时凭据时独立配置 session token；KMS 加密桶还可能需要额外 KMS 权限，先解释缺失原因，不自动改加密方式或扩大策略。
5. 不授予或调用 DeleteObject、修改 bucket policy/lifecycle、全账号 ListAllMyBuckets。不因 SDK 示例包含 abort/list 权限就照抄。
6. 如果确需公开 GET，向我解释 Block Public Access、bucket policy、公开范围和泄露含义，由我单独决定是否配置。
   严禁关闭账户级 Block Public Access 来“跑通测试”。不能公开包含真实数据的整个桶。
7. 可采用稳定的区域性公开对象 URL；路径与选定 region/bucket 一致，避免自动区域重定向。publicReadBase 是 key 之前的根路径，计算完整 URL 时 prefix 只加一次。

### C2. Cloudflare R2 专用测试资源

1. 使用我自有的 R2 bucket。accountId 为账户 ID，不是 token；region 必须为 auto，写入 endpoint 由代码派生为官方 <accountId>.r2.cloudflarestorage.com。
2. 当前代码未单独接入 jurisdiction-specific 等其他 endpoint；如我的资源有此要求，记录兼容缺口，不手改为任意 BaseEndpoint。
3. 使用 R2 S3 凭据的 Access Key ID / Secret Access Key，不把普通 Cloudflare bearer API token 当成 S3 secret。
   writer 与 reader 使用独立、最小权限、限定桶的身份。不要把 AWS IAM prefix 限制粒度承诺成 R2 token 也具备；必要时用专用桶隔离。
4. 公开读使用我选择的 R2 公共域名/custom domain；它不等于签名用 S3 API endpoint。
   启用公开访问/custom domain/CORS 都必须先说明影响、由我单独确认；r2.dev 的开发用途及限制按官方文档说明。
5. R2 当前只测试不超过 16 MiB 的单 PUT。没有安全证明前不启用 multipart，不降低条件写要求、不退化成先 HEAD 再无条件 PUT。

### C3. 配置文件与秘密输入

- 从 D:/inj/next-injective-git/docs/examples/storage-byos.json 另建本次独占配置，保留原示例不改。
- 配置只保存 profile、bucket、region/accountId、prefix、publicReadBase 和 credentialRef 中的变量名称；不要加入 endpoint、accessKeyId、secretAccessKey、token 值等字段。
- writer/reader profile 分别选择；鉴权 reader 的环境变量引用必须不同，真实云身份也应不同。仅变量名不同不能证明身份隔离。
- 模型不要运行 Get-ChildItem Env:、打印 SDK credential 对象、读取 ~/.aws、~/.igit、现有 .env、keystore 或其他秘密文件。
- 由我在本地终端或安全输入界面配置进程级变量。不要让我把明文写入会话命令、PowerShell 历史、setx、仓库脚本或持久化 JSON。
  若需安全输入辅助脚本，必须使用交互式不回显输入、仅设置本次子进程环境，finally 清除；脚本本身不能含秘密字面值。默认由我在本地执行。
- 如果模型工具新开 shell 无法继承我配置的进程环境，明确说明，改由我在同一终端运行 canary；不要转而搜寻或导出已有凭据。
- 对实际配置先运行 doctor。示例链上字段不能被称为部署：纯存储 fixture 可以使用明确标记的合成 chainId/Directory/repoId，它们只作本地上下文绑定、不触发链读写。

### C4. 每个真实阶段的授权记录

在执行之前，给我一段填妥的计划供确认，至少包括：

| 字段 | 必须填明 |
|---|---|
| provider / account resource | AWS 或 R2、确切 bucket、region/accountId、prefix |
| source/code | 本轮代码状态、计划哈希、fixture 文件 digest/size |
| action | 小对象条件 PUT/GET、公开读诊断、multipart 或恢复，分别说明 |
| limits | 对象数、唯一对象存储上限、含重试的上传/回读字节和请求上限、超时 |
| cost | 费用由我承担；给出合理估计或计算方式；无可靠价格则不伪造金额 |
| prohibited | 不删除/覆盖损坏对象、不 abort、不改策略、不做链交易、不公开业务数据 |
| retention | 测试对象/会话默认保留；清理需要另取确切对象/会话范围授权 |

“准备好了”“看看能不能用”不能替代模糊范围的无限制写入授权。得到清晰的小范围确认后，只执行对应阶段；超预算或未知响应时停止。

## 6. 阶段 D：真实 AWS/R2 小对象条件写与独立读取

两个 provider 按相同用例分别做，证据不得互相替代。
以下每一项优先使用同一份小型合成 Git pack 和 digest-derived key，以减少请求/存储开销。

### D1. 小对象上传与完整回读

1. 工具在新建本地 Git fixture 中创建少量非秘密文件、至少两个 commit，生成非 thin 完整历史 pack，记录 Git commit OID、raw pack SHA-256 和真实 size。
2. dry-run 展示预计目标 key：<prefix>/packs/sha256/<digest>.pack，确认落在已批准 prefix 内且 size 不超过预算。
3. 经授权后用生产 NewWriter + PutRecoverable（或带可靠 checkpoint 的等价封装）执行条件 PUT。
4. SDK 必须发送 If-None-Match:*；成功后完整 GET 验 raw SHA-256/size，不能只看 ETag、metadata、HEAD 或 HTTP 200。
5. 记录 Verified、digest、size、provider 和 key；证据中不含签名 URL、Authorization、秘密和用户本地密钥路径。

成功只代表此时此对象在此 provider 可读且字节正确；不证明长期可用/链上承诺或 public Web 可读。

### D2. 同字节幂等复用

- 第二次发送同一源文件、同一 key 的条件 PUT，然后完整回读。
- 预期复用成功、Reused=true，所有 raw bytes/size 保持一致；实际服务端 412 需要白名单诊断/服务商证据确认。
- 若只看到 Reused=true 而没有服务端状态证据，只说明客户端走复用/不确定写后匹配路径，不冒称一定观测到了 412。
- 不创建“不同内容同摘要 key”的真实覆盖；损坏对象拒绝先用阶段 A 的 mock 证明。

### D3. 独立 reader，避免意外走匿名 URL

1. 单独 reader 进程只提供 reader 变量，不继承 writer 的长期秘密；使用冷临时目录，无源仓库对象或已下载 pack。
2. 读取目标 provider/bucket/prefix 应和本次批准对象一致，使用独立 NewReader。
3. 当前 Open 逻辑：PackLocation.url 非空时直接走公开 GET，即使 reader profile 带凭据也不会签名。
   要实测“鉴权 reader”，使用 url 为空、reader 为明确映射 label 的 location，通过 packstore.Readers.Authenticated 选择 reader，再用 ReadVerified。
4. 与独立保存的 expected digest/size 比对。删除 writer 变量后仍能成功，才能说明不依赖 writer 配置。
5. 如果 bucket 匿名可读，GET 成功本身不能证明请求用了 reader 身份。结合专用私有对象/桶或脱敏签名与服务商身份证据；不能为了证明它而自动收紧/放开已有桶策略。
6. NewReader 拒绝 PUT 只是客户端门控；不等于证明 IAM/R2 服务端只读权限。策略作用域由我确认，实际拒绝写请求需另有明确范围，默认只做本地门控测试。

### D4. 发布 manifest，再冷读取验证与 Git 摄取

1. 使用真实 fixture 的 commit 和 pack 生成 schema 1 manifest，locations 区分匿名 URL 与本地 reader label；不得混入秘密、私有凭据或预签名 URL。
2. 在上传前保存期望 manifest bytes/digest/size 和合成上下文；manifestDigest 不写回被哈希 body。
3. 用相同 provider 条件写 <prefix>/manifests/sha256/<digest>.json，完整回读验证。
   可复用 packstore.Prepare 做 pack→manifest 准备，但它自己不自动持久化每个上传 receipt；需要包装 Writer 使每个 Put 保存 checkpoint，不能丢失失败中间记录。
4. 关闭 writer 与源仓库访问，用独立 reader 或匿名公开路径下载 manifest，先对照本地固定 commitment 验 digest/size，再调用 Parse 验 JCS/schema/上下文。
5. 按有序 packs 逐项 ReadVerified，每个 pack 的 locations 才是可互换副本；不能把不同 packs 择一下载。
6. 创建完全空的 bare Git 目录，用 IndexVerified 显式传入固定 expected pack digest/size，再检查目标 commit、文件内容和可达历史。排除 alternates、replace objects、共享 object directory 等隐式来源。
7. 修改合成 fixture 文件并新建 commit，重新生成 pack/manifest；得到新 digest key 后，旧 key 和旧 bytes 仍可读取。仅在本阶段预算覆盖新对象时执行。
8. 不改任何链上 ref。结果名称应是“真实云数据平面 + 本地冷 Git 摄取”，不是链上 push/clone/pull、successor 发布或 fork E2E。

如只有鉴权 reader、没有公开域名，可以完成独立 reader 路径；外层 bootstrapLocator 可在纯协议 fixture 中明确使用占位值，但必须将公开 bootstrap/匿名读取标 NOT PROVEN，不能宣称链接可用或公开发布完成。

## 7. 阶段 E：匿名 HTTPS、CORS 与真实浏览器

必须单独获得对公开范围/Origin 的同意；私有 bucket 测试 PASS 不能推导本阶段 PASS。

### E1. 明确公开路径

- 实际 pack 和 manifest URL 应能稳定匿名 GET，不含鉴权 query、user/password、fragment 或显式端口。
- prefix 只出现一次，URL 不依赖本地配置文件或尚未读到的 manifest，不能造成循环解析。
- AWS 区域重定向、R2 S3 API 认证 endpoint 与公开域名混用，应定位配置错误；不能把客户端重定向限制关掉。
- 一次匿名 GET 成功也不等于浏览器允许跨域读；必须分别验证 pack 和 manifest。

### E2. Go 只读诊断

用生产 DiagnosePublic(ctx,url,origin,expected) 测完整 bytes 和 ACAO，两项分别记录：

- Verified=true，CORS=true：当前指定 Origin 的响应头允许读取，且字节匹配。
- Verified=true，CORS=false：服务可读，但该 Origin 未获允许；不是整体 PASS。
- 字节、状态或 Content-Encoding 不符合：拒绝，不能因 hash 恰好匹配就忽略 transport 问题。

当前 DiagnosePublic 的 origin 校验只接受无路径、无显式端口的 HTTPS Origin，例如 https://preview.example.com。
它目前不接受 http://localhost:5173。不要为了跑诊断冒填另一个 Origin，也不要称它已测了 Vite 本地页面。
如果需要验证实际 localhost 开发 origin，应分开跑浏览器测试并报告此诊断 API 的限制；必要的精确修复须有测试，不能扩大云写入 endpoint allowlist。

### E3. 真实浏览器页面

1. 先确认实际测试页面 Origin 及我的 CORS 授权；页面 Origin 必须与对象 URL 的 Origin 不同，才能证明跨域读取。仅在已有获准页面或本地测试页进行，不自动部署网站/绑定域名。
2. 用不持有任何云凭据的页面请求真实 URL，fetch 配置至少为 credentials:'omit'、redirect:'error'。不要 mode:'no-cors'，opaque response 不能证明字节可读。
3. 读取响应前设置最大长度，流式累计并在超限时取消。使用浏览器 SHA-256 对照预先固定 digest，manifest 使用当前 TS Parse 校验。
4. 输出请求 Origin、成功/失败、实际字节数、预期/实际 digest 和协议验证结果。Network/Console 截图仅保留非秘密内容；不发送全量 HAR。
5. 匿名环境无 writer/read secret、无已登录云控制台 cookie。控制台登录成功或直接地址栏下载不是跨域 fetch 验收。
6. CORS 配置变更需要我单独确认。建议只允许本次实际 Origin 与 GET；不要默认 '*'、带 credentials 或开放任意写方法。
   简单 GET 可不触发 preflight，不能编造一次 OPTIONS 记录；如果页面实际触发 preflight，再按真实 request 验证。
7. 当前 Web 产品 gitstore 尚未接入 BYOS。测试页 PASS 仅是“真实浏览器公共 manifest/pack verified reader”，不是产品 Web browse 全链路。


## 8. 阶段 F：AWS multipart 和中断恢复（单独授权的可选加强项）

先完成对应 provider 的小对象测试，再说明额外流量和残留 multipart 费用，得到独立确认后执行。
不因为已有上传授权就自动测试大对象、中断、并发冲突或损坏。

### F1. AWS multipart 正常完成

1. 创建一次性的本地合成 Git fixture，使用不可高度压缩的非秘密随机内容，让实际生成的 pack 大于 16 MiB、建议控制在 17–24 MiB 内。
   必须检查 pack 实际长度；仓库文件超过阈值不代表压缩后的 pack 也超过阈值。不要直接上传业务二进制或把随机 blob 冒称 Git pack。
2. 展示批准计划：实际 pack 大小、8 MiB 分片的数量、CreateMultipartUpload/UploadPart/CompleteMultipartUpload/验证 GET 的请求预算。
3. 执行生产 AWS adapter，确认 CompleteMultipartUpload 使用 If-None-Match:*，完成后全量 GET 对 raw SHA-256/size。
4. 检查完整 receipt；multipart ETag 不能当 SHA-256。记录实际成功会话与字节，不从本地 mock 推导真实结果。
5. 重复 multipart 对象可能先上传全部分片再在 Complete 返回 412，仍会消耗流量并留下待处理会话；不默认重跑，先单独估算并确认。
6. 不为制造 409 去并发写、覆盖或删除对象。409 重建语义已有 mock 测试；若真实场景自然出现，按 receipt/预算定向处理，未实际观察到则如实标 NOT PROVEN。

### F2. R2 超限只做本地拒绝证明

对 R2 配置输入一个超过 16 MiB 的本地 source/expected object，确认请求前返回 limit，且未产生云请求。
此项用无真实凭据的受控 transport 测试即可；不对 R2 发 CreateMultipartUpload 来“试试看”，不关闭条件创建。
拒绝超限是当前产品行为，不是 R2 multipart 已支持的证据。

### F3. 受控中断和恢复

1. 要做真实中断，先明确其可能留下 incomplete upload；确认专用 key/session 范围和残留费用，不把 abort 作为默认收尾。
2. 本地 source 文件必须在进程退出后仍然存在，且 digest/size 与最初计划相同。不要在测试 cleanup/defer 中删除恢复所需文件。
3. checkpoint 必须在云 IO 前记录目标 key、在 Create 成功后保存 upload ID、每次分片完成后保存序号；先确认受保护文件可读写。
4. 优先设计一次精确的测试工具停止点：首次 part checkpoint 持久化后取消 context，避免盲目 kill 造成无法辨识状态。
   停止点只属于显式 canary 场景，不能加入生产默认故障/自动清理路径。
5. 开新进程加载 journal，重新校验计划/云配置/源码/源文件；实际调用 Recover 前解释它可能继续写云并确认剩余预算。
6. Recover 首先完整 GET 最终对象：
   - 匹配：复用，不能再次创建会话；
   - 404：在获准预算内创建新会话、全分片重传，旧 upload ID 保留；
   - 403、网络失败、损坏或未知结果：停止并报告，不能当成“对象不存在”。
7. 若 Create 请求已被服务端接受但响应丢失，可能没有 upload ID；如实记录“会话身份未知”，不要宣称可自动恢复/清理所有孤儿。
8. 检查没有 DeleteObject/AbortMultipartUpload/改 lifecycle 请求，恢复完成也不代表长期可用。

### F4. 损坏/删除测试默认不在真实云执行

- 同 key 不同字节、对象消失、metadata 伪匹配、manifest 失败、网络中断、限流等，先使用已有本地 mock 覆盖。
- 若我明确要求真实损坏/删除验证，必须另给一份仅涉及一次性、无其他 ref/manifest 依赖的测试对象清单和恢复计划，让我确认后再做。
- 不能把“测试”作为绕过不可变摘要对象、覆盖真实仓库历史或清理未知 session 的理由。
- 凡需改 policy/CORS/lifecycle、禁用/撤销凭据或删除对象，也分别确认，不夹带在恢复命令里。

## 9. 任何阶段遇到这些情况时的处理

| 现象 | 应如何判断/行动 |
|---|---|
| doctor PASS，上传提示凭据缺失 | doctor 不解析凭据；检查执行进程是否有对应变量，只输出存在性，不能打印变量值 |
| 403 | 分开检查 writer/reader 选择、桶/前缀权限、account/region、KMS 或 token 范围；不自动扩大权限 |
| NoSuchKey/404 | 核对真实 key、prefix 是否重复、实际 bucket；只按可靠 404 判定缺失，权限不足导致的 403 不是缺失证据 |
| 412 后回读不匹配 | 完整性 FAIL，保留 key 和 receipt，停止；不无条件覆盖“修复” |
| 503/超时后结果 uncertain | 用固定 key 只读检查；没有完整匹配证据前不标成功、不盲目再上传，不自动清理 |
| AWS Complete 409 | 本地实现有限重建；真实阶段先检查预算与保留会话，不能无限重试 |
| 公开 GET 可读、CORS false | 限定为服务可读；只有我确认的实际 Origin 经浏览器成功才报告跨域通过 |
| localhost Origin 被 DiagnosePublic 拒绝 | 这是当前 origin API 限制；分开测真实本地浏览器，不伪造另一个 Origin |
| safehttp 拒绝 host/DNS/redirect | 查正式 endpoint、公网 DNS、自定义域名配置；不能关闭 SSRF 或启用 allow-any-endpoint |
| journal DACL/权限失败 | BLOCKED，仅该持久化路径；保持保护，使用新独占且获准的本地目录重试，不改用户目录 ACL 来绕过 |
| race 缺 CGO/编译器 | BLOCKED，继续其他验证；不把普通 go test 当 race PASS |
| 没 AWS/R2 账号或未给写入授权 | 对应真实 provider 项 BLOCKED/NOT PROVEN；继续本地/mock 或另一个 provider |
| ordinary git push/clone 仍走 IPFS | 当前代码边界，不用安装 Kubo来冒充 BYOS 测试；记录 S04/S05 仍待接入 |

## 10. 验证产物、状态表与提交边界

在本次独占 local-only 运行目录内保存可审查的非秘密证据，例如：

- run-plan.json：provider、资源范围、对象 digest/size、请求/字节预算、测试场景、我确认的授权范围，不存秘密。
- source-state.json：实际 40-hex HEAD、分支、dirty 状态；本轮相关源码/fixtures/锁文件路径及内容 SHA-256。
  当前有大量 untracked，实现不能只绑定 HEAD；不把用户秘密文件纳入哈希清单，不整包导出工作区。
- commands.txt / results.json：实际日期/时区、命令、exit code、测试层、结果摘要和证据文件位置；仅在脱敏后保存日志。
- 每个 provider 的 protected receipts/checkpoints：禁止公开，不能写入 manifests、Git remotes 或 Web localStorage。
- browser-result.json 或非秘密截图：实际 Origin、URL、对象 digest/size、fetch/协议结果；不保存 cookies、Authorization 或全 HAR。
- retained-objects.json：本次创建/复用的 key、已知 incomplete sessions 和归属，明确“尚未清理”。不能自动执行回收。

如后续要把证据提交到仓库，先单独审查泄露风险与审批；本提示词不授权 commit/push，也不允许重写既有部署/验收文件制造绿色 gate。

最终请给我这张分层表，每项附对应命令/证据，不用单一“都通过”总结：

| 检查层 | 本轮状态 | 必须说明 |
|---|---|---|
| Go/TS canonical manifest | 待实测 | bytes/digest 向量数量、正反向结果、协议差异 |
| packstore/mock AWS | 待实测 | 条件写、完整性、取消/重试、恢复 |
| packstore/mock R2 | 待实测 | 单 PUT、超限、独立配置 |
| 真实 Windows 本地 Git fixture | 待实测 | SHA-1/pack v2、历史与目标可达性 |
| Linux 交叉编译 / Linux 实际运行 | 分开记录 | 编译不能等同运行 |
| 真实 AWS 小对象/幂等复用 | 待资源/授权 | 实际请求、回读字节、身份/资源范围 |
| 真实 R2 小对象/幂等复用 | 待资源/授权 | 与 AWS 独立的证据 |
| 独立鉴权 reader | 待资源/授权 | 不依赖 writer、实际走签名 GET，而非匿名 URL |
| 匿名 GET / 实际浏览器 CORS | 分开记录 | 指定 Origin、manifest 与 pack 都检验 |
| 真实云 manifest→pack→空 Git 摄取 | 待资源/授权 | 本地固定 commitment，不是链上发布 |
| AWS multipart / 中断恢复 | 可选，单独授权 | 会话/预算/不自动 abort；没做就不通过 |
| R2 multipart | NOT PROVEN | 当前明确不支持，不能计作必需已通过功能 |
| 真实 IPFS/replication | NOT PROVEN | 本轮 BYOS 无需 Kubo，不借此宣布其通过 |
| successor 合约/部署/交易、Git remote/Web 产品 E2E | NOT PROVEN | S04/S05/S06 未贯通，不向 v3 发送新 payload |

“待实测/待资源”只是本提示词占位，不是允许的最终状态；执行时用 PASS/FAIL/BLOCKED/NOT PROVEN/HISTORICAL 替换。
发现缺陷时给出最小复现、影响范围、改动文件、重新运行结果和未覆盖层；只更新当前状态区，不改原审计或 HISTORICAL。

## 11. 安全红线与收尾

- 不真实部署、签名、广播、主网操作或修改公开 Directory。S04/S05 尚未接通时不要尝试“顺便完成”链上 canary。
- 不运行四个旧 EVM indexer/reaper 脚本主流程，不 pin/unpin/GC，不启用自动生命周期回收。
- 不 git reset/clean/force push，不删除或移动既有用户文件；新文件也不默认清理。Windows 文件操作使用原生 PowerShell LiteralPath，递归目标须先验证绝对路径，不跨 shell 拼接破坏性命令。
- 长期/真实凭据只在我控制的执行环境使用，模型不查看、不导出。不公开测试桶中的业务数据；本次 Git fixture 必须全部为新建合成内容。
- API/SDK/条件写/CORS 细节以当次官方原始资料为准：
  [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785)、
  [AWS conditional writes](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)、
  [R2 S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/)、
  [R2 Go SDK](https://developers.cloudflare.com/r2/examples/aws/aws-sdk-go/)、
  [Git pack-objects](https://git-scm.com/docs/git-pack-objects)。
- 测试结束给我本次资源/对象/会话清单、哪些需我后续撤销或清理、怎样申请精确清理授权。不自动删除、abort、撤销令牌或更改策略；即使我关心费用也先给出范围再确认。

现在请先从阶段 A 开始：核对实际文件和环境，运行无需账号的测试并给我真实结果。
随后补齐或核验 canary 工具的本地 plan/tests，确认无需我操作的部分完成后，再只问我当前 provider 的必要非秘密资源信息。

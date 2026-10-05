# AWS S3 / Cloudflare R2 BYOS：首期实施规格

决策日期及官方资料查询日期：**2026-09-13（Asia/Shanghai）**。
交付状态（2026-10-05 更新）：**已按本规格交付为 Suite v4（BYOS）**——successor 套件
（suiteVersion=4）已部署并激活于 Injective 测试网（Directory
`0xf987396475d0a4c96b722e993a95d8720a6292ad`）；CLI/Web 按链上版本分派，v4 走本规格的
BYOS 路径；真实 Cloudflare R2 端到端 Git 流程（push/clone/fetch/ls-remote/tag/删除/重建）
与匿名公开 GET+CORS 验证 PASS，全程无 Kubo/WSL2/injectived；Web 浏览可用。
**仍 NOT PROVEN**：真实 AWS S3 canary、force-push 陈旧/真实并发竞争、R2 外部篡改检测、
Blockscout 源码验证、Foundry 门禁、successor 公开发布与主网验收。交付记录见
[backlog](backlog.md) 2026-10-04/05 各节。云 provider 仍**仅限 AWS S3 / Cloudflare R2**。

产品范围以 [ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md) 为准；
原审计事实保留在 [事实基线](reconciliation-baseline-2026-09-12.md)；第 9 节记录 2026-09-13 的
S01–S03 本地切片（历史快照），其后的交付以上述状态为准。第 9 节冻结了已测试的 manifest
schema 1 和本地 API/限额；successor 已定为 suiteVersion 4 并已部署测试网；
**AWS/R2 BYOS、公开仓库、canonical JSON、用户独立 reader、不强制双副本不再是开放产品选项。**

## 1. 用户获得什么

- 使用自己在 AWS 或 Cloudflare 开设的桶，不需要安装 Kubo，也不需要连接 IPFS 主网或 iGit IPFS 服务。
- 写入凭据只用于自己的云账户；链上维护者权限与云写入权限分别配置，互不替代。
- 首期标准交付路径为公开仓库、私有写入、稳定匿名 HTTPS 读取；Web 需要正确的 CORS。
- 需要鉴权读取时，使用用户独立的只读 CLI reader 配置；不向协作者分发 writer secret，不引入平台 broker。
- 私有桶不等于私有 Git 仓库。鉴权 CLI 可读不等于匿名 Web 可读；Web 不收长期云密钥，配置不满足时明确报错。
- 用户承担存储、请求、流量及验证回读成本；只要求一个已验证位置。双副本、托管服务、私有仓库/E2EE 后续另立范围。
- 支持的是“用户自有云桶”，不是“自建存储服务”。MinIO、OSS、COS 和任意 S3-compatible endpoint 均不在首期生产配置中开放。

用户可以修改 Git 内容、管理云对象和保留策略，但客户端不会把不同字节覆盖到同一个摘要 key。
正常修改产生新 pack、新 manifest、授权的 ref 更新；修改位置或重新打包也可能产生新 manifest，而 commit 不变。
管理员在云控制台覆盖/删除对象会造成完整性失败/不可用，不会合法地改写链上已承诺的内容。

## 2. 链上承诺与链下模型

推荐混合方案：**链上保存当前 manifest 承诺及引导读取所需的有限信息，链下 manifest 保存 pack 集合及位置。**
不用逐 pack 链上数组承载无限历史，也不采用只有 hash、却必须依赖平台索引器才能定位 manifest 的方案。

| 层次 | 候选字段与责任 | 验证者 / 限制 |
|---|---|---|
| 当前合约状态 | repoId、refName 或可查询的完整名称、commitOid、Git hash algorithm、manifest SHA-256/size、稳定 bootstrap locator、ref revision、updatedAt/By、exists | 合约验证权限、长度、已知协议版本及 CAS；客户端验证内容，合约不能读取云对象 |
| 事件 | repo/ref、commit、manifest 承诺、revision、更新者；需可还原完整 ref 名称 | ABI 解码库和重组安全 indexer；不能仅存 indexed string 的 hash 就声称能还原原文 |
| PackManifest | schema/version、chainId、SuiteDirectory、repoId、refName、commit algorithm/OID、有序 packs | Go/Web 对照已验证的链状态检查上下文；避免跨仓库/链/Suite 替换 |
| PackEntry | sequence、raw SHA-256、size、pack format/version、thin、dependsOn、locations | 区分不同 pack 的顺序/依赖和同一 pack 的镜像 |
| PackLocation | provider、非秘密存储描述、稳定读取位置或用户本地 reader 映射标识 | URL 不是内容身份；每个成功读取都重新验证摘要和长度 |
| 本地配置/恢复记录 | writer/reader profile、credential reference、上传会话、provider receipt、ref 预期状态、tx hash | 不含秘密和预签名 URL；权限保护、可恢复、不得当作链上持久性证明 |

标准公开路径的 bootstrap locator 是非过期 HTTPS manifest 地址，由 ref 交易绑定并保存在**状态**中。
公开域名/路径可能披露桶名，这是公开路径的显式选择，不应混入私有 bucket 细节。
无 indexer、无本地缓存的新客户端也必须通过 Directory 验证和 getRef 取得该信息。
独立 reader 可在本地将已承诺的 digest 映射到自己的桶/key；该配置只能改变读取位置，不能改变期望 digest、size 或仓库上下文。
不得把解析 manifest 所必需的唯一 locator 放回尚未读取的 manifest，造成循环依赖。

### 2.1 数据形状（schema 1 已本地验证；不是现有 ABI）

```text
PackManifest {
  schema: "igit.pack-manifest", schemaVersion: 1,
  chainId: decimal-string, suiteDirectory: lower-case-0x-address,
  repoId: lower-case-0x-bytes32, refName: full-ref-name,
  commit: { algorithm: "sha1" | "sha256", oid: lower-case-hex },
  packs: [ PackEntry, ... ]
}
PackEntry {
  sequence: safe-integer, sha256: 64-lower-case-hex, size: decimal-string,
  format: "git-pack", packVersion: safe-integer,
  thin: boolean, dependsOn: [pack-sha256, ...],
  locations: [ PackLocation, ... ]
}
ManifestCommitment { sha256, size, bootstrapLocator } // outside hashed body
```

此处 `schemaVersion: 1` 是第 9 节冻结的新 manifest schema，不是 Suite v1，也不是宣告 successor 为 v4。
schema 可表达 Git SHA-256 不代表 Web/Git 库已经支持它；首个切片先证明 SHA-1 Git 仓库和 pack v2，
其他 object format 必须明确拒绝，直到对应客户端测试通过。

### 2.2 canonical JSON 与摘要

编码基线为 [RFC 8785 JCS](https://www.rfc-editor.org/rfc/rfc8785)。
UTF-8、无 BOM、无额外换行；对象键采用 JCS 排序，数组保持协议顺序。
禁止重复 key、非法 Unicode、NaN/Infinity、未定义字段/版本、尾随数据和超限嵌套。
对原始 UTF-8 字节限长后严格解析并检查规范字节，不能靠普通 JSON.parse/encoding/json 默默接受重复 key。
不做 Unicode 正规化来改变 ref/字符串含义。Go 默认 JSON 序列化及手写“排序 JSON.stringify”不自动等于 JCS。

chainId、字节 size 等可超过 JS 安全整数范围的值用无前导零的十进制字符串；
sequence/schemaVersion/packVersion 只接受协议限定的非负安全整数。字段缺省、空数组与 null 的规则必须固定。
接收器的 manifest 字节/pack 数量/location 数量/依赖深度/单 pack 大小/总下载预算均需有限上界。

```text
packDigest     = SHA256(exact uploaded .pack bytes)
manifestBytes  = UTF8(JCS(PackManifest))
manifestDigest = SHA256(manifestBytes)
pack key       = <user-prefix>/packs/sha256/<64-lowercase-hex>.pack
manifest key   = <user-prefix>/manifests/sha256/<64-lowercase-hex>.json
```

**manifestDigest 不放入被哈希的 manifest body**，否则出现自引用；它属于返回 envelope/链上承诺。
自己的 manifest key/URL 也留在外层；pack locations 可以写入 manifest，因为它们不依赖 manifestDigest。
临时凭据、验证回执、provider 时间戳不进入 canonical body。
key 由受限 prefix 与 digest 派生，拒绝路径穿越和非规范大小写。

必须提供 Go/TypeScript 同一 fixture 的精确 UTF-8 bytes 和 SHA-256 正反向测试，
覆盖键顺序、嵌套对象、数组、Unicode/转义、重复 key、大整数/溢出、未知版本、损坏 digest 与 size。
测试向量须由实现真实生成并交叉比对，不能在文档中填写假哈希作为通过证据。

Git OID、raw pack SHA-256、IPFS CID、S3 ETag/provider checksum、EVM Keccak topic 是不同概念。
[IPFS CID](https://docs.ipfs.tech/concepts/content-addressing/) 还受 DAG/编码影响，不能直接视作原始 pack 的 SHA-256。
现有 v3 没有链上 raw SHA-256/size：本地给下载结果再算一次 hash 并不能补出链上承诺。
legacy reader 必须标明验证边界；successor manifest 内的 IPFS location 才能用同一 raw-digest 验证流程。

### 2.3 pack 集合与兼容

首个 successor writer 每个 ref 发布自包含、非 thin 的完整可达历史 pack，`thin=false`、`dependsOn=[]`。
不复制当前 helper 排除所有远端 ref tip 的策略；否则其他 ref 删除后可能破坏独立 clone。
新分支/tag、空增量、force push 都必须独立可读；零新对象不代表可以绑定其他 ref 的隐式依赖。

legacy v3 的 packUris 顺序保持不变；它是多个 pack，而不是多个副本。
未来增量方案需要显式依赖闭包、无环拓扑排序、上界、缺失依赖拒绝和跨 ref 删除测试，不能悄悄开启。
Git 的 [thin pack](https://git-scm.com/docs/git-pack-objects) 可能引用 pack 外对象；
[index-pack --fix-thin](https://git-scm.com/docs/git-index-pack) 后的本地字节不再必然与上传 raw pack 相同。
验证上传 digest 必须在 Git 修复或摄取之前。

首期可以只有一个 location；换域名、加副本、重打包产生新 manifest，使用**同 commit、不同 revision** 的 CAS 更新。
不引入一个可悄悄改变内容身份的中心化 mutable profile registry。
fork/跨 ref 复制必须重新生成绑定目标 repo/ref 的 manifest；不能原样拷贝带源上下文的 commitment，
这也是 successor Core fork API 必须单独测试的变化。

## 3. BYOS 配置与凭据边界

将 network/Suite 信任配置与 storage profile 分开，storage 选择不得改变 Directory 或 chainId。
每仓库以 chainId + Directory + repoId 绑定本地 writer/reader，避免 rename/transfer 后按名字误选桶。
已实现 `igit storage <add|doctor|show>`：`add` 登记配置文件路径（仅引用，不含凭据），`show/doctor` 只读取显式的独立配置文件；show 仅显示无秘密引用，doctor 仅本地配置校验。
上传探针需单独显式开启并获资源授权，不能借 doctor 自动建桶、开放桶或写对象。

| 配置项 | AWS S3 | Cloudflare R2 |
|---|---|---|
| provider | `aws-s3` | `cloudflare-r2` |
| 服务定位 | 明确 bucket/region，SDK 解析经过允许的 AWS endpoint | 明确 bucket/accountId，派生官方 R2 S3 endpoint，region 为 `auto` |
| namespace | 用户授权的独立 prefix，digest-derived key | 同左 |
| writer credential reference | 命名 AWS profile/用户选择的 SDK 安全凭据来源 | OS 安全存储或用户显式配置的环境凭据引用 |
| reader | 匿名 HTTPS，或独立只读 identity | 匿名公开域名，或独立只读 identity |
| publicReadBase | 稳定无鉴权 HTTPS 路径，禁止动态签名参数 | 经用户配置的 R2 公开域名；不是带密钥的 S3 API URL |

普通 JSON 配置仅保存引用，例如 `credentialRef: {kind: "aws-profile", name: "igit-writer"}`
或环境变量**名称**；不保存 access key 值、secret、session token、Authorization header。
[AWS Go v2 凭据链](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html) 的真实凭据由用户在安全渠道配置，
不是让 agent 读取其已有凭据文件。测试必须注入假 credential provider，禁用共享文件、IMDS/ECS 等意外回退。

维护者需要两种独立授权：链上 ref 维护权限，以及指定 bucket/prefix 的云 PUT/GET 权限。
协作者只需要自己的 GET/必要 HEAD 权限，不得复用上传者长期密钥。默认不授予 DeleteObject、改 bucket policy 或列举所有桶权限；
multipart 所需权限单独说明。不要承诺 R2 token 与 AWS IAM 的 prefix 限制粒度一致，按服务商实际能力配置隔离桶/范围。

认证 reader 不得影响验证期望值，也不得把私有 bucket 参数传播到链、公开 manifest 或 Web localStorage。
如果公开 manifest/pack URL 不可匿名访问，Web 说明需要公开读配置或独立 CLI reader，而不是索取 writer secret。
首期不为 Web 提供长期 secret、托管 GET broker 或自动临时凭据分发。

### Endpoint 与网络安全

- 写 API endpoint 由已知 provider 字段派生/验证；生产配置拒绝任意 S3-compatible base URL、localhost、IP 字面量和未知 provider。
- R2 自定义**公开读取域名**可接受，不等于开放自建写入 endpoint；不得把云 Authorization 头发送给该域名。
- 不跟随不受控重定向；禁止 userinfo、签名 query、fragment、HTTP 降级、路径穿越及非预期端口。
- CLI 防 SSRF 需覆盖 DNS 解析/重绑定、私网/回环/link-local/云 metadata/IPv6 映射地址及每一跳重定向。
- Web 用不携带 credentials/cookies 的公开读取并拒绝不可信重定向；浏览器不能替代服务端 DNS/IP 防护。
- 只通过测试代码注入 loopback HTTP server/transport。不能为方便 mock 在正式配置留下 `allow-any-endpoint` 后门。
- 日志、错误、进度、恢复 journal 和测试 snapshot 都做秘密脱敏；不打印完整环境或密钥文件。

## 4. Provider 能力与实现策略

以下是 **2026-09-13 官方文档能力核对**，不是本仓库真实测试 PASS。每个 adapter 独立协商/限制能力；
不能因为 R2 使用 S3 API 就复用全部 AWS headers、错误恢复、加密或保留语义。

| 项目 | AWS S3 | Cloudflare R2 | 本项目规则 |
|---|---|---|---|
| 条件创建 | 官方说明 PutObject / CompleteMultipartUpload 的条件写 | S3 compatibility 表列出 PutObject 的 If-None-Match；不能据此推断 CompleteMultipartUpload 等价 | 重复 key 后验证已存在的真实 bytes；禁止 HEAD 后无条件 PUT |
| 校验和 | SDK/API 提供多种 checksum，multipart 还有 composite/full-object 区别 | 按 R2 checksum 兼容表逐操作选择 headers | raw SHA-256 + size 始终权威，metadata/ETag 不能代替回读验证 |
| 大对象 | 单独测试 multipart 条件完成与冲突重建会话 | 条件完成的可用性/语义需单独实证 | 未证明安全 multipart 前先限额单次 PUT，超限明确拒绝，不能降级覆盖 |
| 公开读取 | bucket policy/公开读路径与 CORS 分开配置 | 使用 R2 公开桶域名及 CORS；r2.dev 为开发用途，不作为生产 SLA | GET 成功不等于浏览器 CORS 正确，两个入口都测 |
| 保留保护 | Versioning / Object Lock 为独立可选能力 | R2 Bucket Locks 不是 AWS Object Lock API 等价实现 | 用户选择并承担费用；不把可选保护或双副本变成首期硬门槛 |

官方依据：
[AWS 条件写](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)、
[AWS upload checksum](https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity-upload.html)、
[R2 S3 API 兼容表](https://developers.cloudflare.com/r2/api/s3/api/)、
[R2 Go SDK](https://developers.cloudflare.com/r2/examples/aws/aws-sdk-go/)、
[AWS CORS](https://docs.aws.amazon.com/AmazonS3/latest/userguide/cors.html)、
[R2 public buckets](https://developers.cloudflare.com/r2/buckets/public-buckets/)、
[R2 CORS](https://developers.cloudflare.com/r2/buckets/cors/)、
[AWS Versioning](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Versioning.html)、
[AWS Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)、
[R2 Bucket Locks](https://developers.cloudflare.com/r2/buckets/bucket-locks/)。

选用 AWS SDK for Go v2 时锁定兼容仓库 Go 基线的版本；不得为“用最新版”悄悄升级整个 Go toolchain。
R2 文档没有证明的条件 multipart 行为保留为 NOT PROVEN，不写成已支持或确定不支持。

首个可靠 writer 的默认策略：条件创建后**全量流式 GET 回读**校验 digest/size，复用同一验证模块处理 pack/manifest。
metadata SHA-256 可用于诊断但不是独立证明；已有同 key 对象也需回读。公开路径还应验证匿名读取；
禁止传输/缓存层自动改写原始字节，正确处理内容编码、截断、额外尾随 bytes 和响应大小谎报。
在真实 canary 证明更低成本的校验方案前，不用 HEAD/ETag 取代此策略。

## 5. 发布与读取流程

### Push（successor 路径）

1. 验证所选 chain/Directory/版本/code hashes/绑定，读取 repo 和 ref 的一致快照；未知版本或 v3+BYOS 组合在上传前拒绝。
2. 读取明确的本地 writer 配置并做只针对该 provider 的 preflight；AWS/R2 路径不探测 Kubo、IPFS gateway 或 replication service。
3. 在当前任务独占临时目录生成自包含 pack，流式计算 raw SHA-256/size；禁止用无限增长的 `[]byte` 保存整个大 pack。
4. 生成 digest key，条件上传、回读验证；保留可恢复记录。生成 JCS manifest，计算外层 digest/size，上传并验证 manifest。
5. 确认标准公开读取路径可用，检查预期 ref revision；以 CAS 更新 commitment，必要时携带 force 语义。
6. 只有成功 receipt 和约定 finality 策略满足后，才将该次发布标为链上成功；结果未知则记录 tx hash 并进行定向恢复。
7. 不自动删除远端 pack、manifest、历史 pin 或不完整 multipart；本地临时文件仅在生命周期明确且失败可恢复时按安全路径处理。

`force` 允许替换历史，**不应免除 successor 的 CAS**。仅 expectedSha 不足以保护同 commit 的 manifest/location 更新；
使用显式 expected revision/旧 commitment，ref 删除/重建保持单调版本以防 ABA。
合约不能验证 Git ancestry；非 fast-forward 客户端检查与链上并发检查必须区分。
这些是 successor 要求，不冒称现有 v3 已如此实现。

### Clone / fetch / Web browse

1. 验证链/Suite，在同一链视图读取 ref commitment、manifest size 和 bootstrap locator；历史 v3 明确分派 legacy reader。
2. 从稳定 locator 或用户显式 reader 映射下载 manifest；先限长并核对链上 digest/size，再严格检查 schema/JCS/上下文。
3. 按 pack 顺序/已支持的依赖策略处理，每个 PackEntry 内尝试备用 location；不得把不同 packs 当镜像择一下载。
4. 写入受限临时文件，验证完整 raw bytes 的 SHA-256 和实际长度，验证完成后才交给 Git。Windows 先关文件再重新打开摄取。
5. 检查 pack 结构、Git object OID、目标 commit 及可达性；必要时在隔离 object database 验证后再合并。
6. Web 同样先验证再调用 isomorphic-git，设单包/总量上限；拒绝超限而非无界 arrayBuffer/浏览器内存消耗。
7. 缓存按可信 digest 和协议上下文隔离；chain ref 缓存包含 chainId、Directory、repo/ref、revision/区块标识，处理重组失效。

### 故障与恢复契约

| 故障 | 必须行为 / 最小测试 |
|---|---|
| 条件创建返回 412 | 回读同 key；完全匹配才作为幂等复用，不匹配视为冲突/损坏，禁止覆盖 |
| 409、429、5xx、超时 | 有界退避、取消、重试预算；区分操作。AWS CompleteMultipartUpload 的 409 按官方说明重建上传会话，不盲重试 Complete |
| 认证失败/reader 未配置 | 明确用户可执行的配置提示；不得降级公开写或索取 writer secret |
| 错 digest/size、截断、额外 bytes、内容变换 | 拒绝该位置；可尝试同 pack 的其他已声明位置，全部失败则不摄取、不改 ref |
| pack 上传成功、manifest 上传失败 | ref 不变，记录可复用对象；下次重试校验后复用，不立即清理 |
| 上传成功、ref CAS 冲突/交易 revert | 对象保留并记录；重新读取 ref，明确选择重新打包/重发，不自动 force |
| 广播已发、receipt 未知或发生 reorg | 保留 tx hash/nonce/ref 预期值；查 receipt 和规范链状态，不以新 nonce 盲重播，不标成功、不 GC |
| 云端对象/副本被删除或覆盖 | 报不可用/损坏；如有另一副本需逐份验证；哈希不能恢复不存在的内容 |
| ref 删除、force push、重命名、fork | 只改变授权的链状态；不自动删除共享内容，测试跨 ref 独立读取和上下文绑定 |
| multipart 中断或完成结果未知 | journal 保存会话/parts/最终 key；先查最终对象再恢复。真实 abort/清理需单独授权和作用域 |
| 事件重复、解码失败、checkpoint/reorg | 合约读取仍可独立工作；索引器不得在失败时推进 checkpoint，不启用现有 reaper |

AWS 冲突语义见本文件的[条件写来源](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)。
服务商 SDK 的默认重试、隐式 multipart、自动 abort 也须显式配置并测试，不能绕过以上契约。

## 6. 实现落点与版本隔离

| 当前文件/模块 | 后续改动边界 |
|---|---|
| [remote/helper.go](../cli/internal/remote/helper.go)、[gitio.go](../cli/internal/gitio/gitio.go) | 存储中立流程、流式临时文件、legacy/successor 分派、先验证后摄取、发布恢复 |
| [ipfs/client.go](../cli/internal/ipfs/client.go)、[replication/client.go](../cli/internal/replication/client.go) | 包在 legacy IPFS adapter 内；AWS/R2 初始化不依赖这些组件 |
| 新建 `cli/internal/packstore` 及 Go/TS manifest 模块 | 纯协议/校验、writer/reader/capabilities、provider adapters、mock transport；不要把具体 API 放入 chain 层 |
| [config.go](../cli/internal/config/config.go)、[helper main](../cli/cmd/git-remote-igit/main.go) | provider allowlist、本地 credential reference、独立 reader、provider-specific preflight |
| [chain/types.go](../cli/internal/chain/types.go)、[backend.go](../cli/internal/chain/backend.go)、[evm_suite_registry.go](../cli/internal/chain/evm_suite_registry.go) | 区分 legacy PackURIs 与 successor commitment，不用同一 string[] 偷渡新类型 |
| [RepositoryCore.sol](../contracts/evm-v2/src/RepositoryCore.sol)、[ISuite.sol](../contracts/evm-v2/src/suite/ISuite.sol)、[SuiteDirectory.sol](../contracts/evm-v2/src/SuiteDirectory.sol) | successor state/getRef/CAS/events/fork/bootstrap 与版本规则；保持不可升级及模块权限职责 |
| [gitstore.ts](../web/src/lib/gitstore.ts)、[profile.ts](../web/src/lib/profile.ts)、[registry.ts](../web/src/lib/registry.ts)、[transport.ts](../web/src/lib/transport.ts) | 公开 manifest/pack verified reader、内存上限、版本化解码、CORS/鉴权限制提示；无云 secret |
| ABI/artifacts、索引器与 evidence gates | 同一 reviewed protocol 的 parity 测试；旧 v3 ABI/历史证据保留，不修改以伪装 successor 可用 |

推荐 `PutIfAbsent(source{path,size,sha256})`、`VerifyStoredBytes(receipt)`、`Open(location)`、
公共 `ReadVerified(expectedDigest,size)` 与明确 capability 接口；不把未经验证的 Open 直接暴露给 Git。
receipt 只是本地流程结果，不新增托管签名者。默认接口不提供可被后台触发的 Delete/GC。

修改 successor ABI 前先冻结 S01 的 bytes/schema 与类型。合约、客户端、索引器、部署/验收 schema 必须版本一致；
仅改 `_validatePackUri` 或把普通 HTTPS 塞进 v3 PackURIs 不是实现方案。
如果新代码替换默认合约产物，须有明确的版本化来源和独立 legacy ABI，历史部署证据禁止重写。
当前 Core 尺寸余量有限，新增 fields/events/CAS 后重新检查生产尺寸及 Foundry unit/invariant/gas。

## 7. 分层验收与人工依赖

按 [S01–S07](backlog.md) 实施；先交付能在无账号环境运行的代码和测试，随后才申请真实资源。

| 验收层 | 可在下一窗口做什么 | 哪些结果仍不能推导 |
|---|---|---|
| 协议/JCS（S01） | Go/TS canonical bytes 与 digest 交叉向量、错误 schema/上下文/大小拒绝 | 不能证明云端存储或已部署 ABI |
| 存储边界（S02） | 流式临时文件、假 IPFS adapter、完整性/取消/资源释放、真实本地 Git fixture | 不能替代真实 Kubo pin/replication |
| AWS/R2 adapters（S03） | SDK 请求形状/签名输入、条件创建/读回/错误/配置/脱敏/SSRF 的独立 provider contract tests | 本地 HTTP fake 不是 AWS/R2 canary，也不是正式自建 provider |
| successor/客户端（S04–S05） | 版本化 ABI/Go/Web/mock-chain 的 ref 发布读取、CAS、fork 和 legacy 拒绝测试 | 编译/mock 不能证明真实交易/主网可用 |
| 真实云与测试网（S06） | 后续在明确授权下验证两个 provider 的小对象及已支持 multipart、公共浏览器 CORS、no-Kubo Git 流程、钱包收据/finality | 缺账号、服务、工具或写入授权时报告 BLOCKED 或 NOT PROVEN，不能补造证据 |
| 历史导入（条件性 S07） | 获批后固定区块清点、原字节 CID→digest mapping、独立 ref 依赖验证、回滚只读期 | 新建 successor 不自动要求搬迁 v3 或 V1；旧报告不证明迁移 |

真实阶段需要用户通过安全渠道配置测试 bucket/专用 prefix、权限、费用与上传范围；公开读域名/CORS/桶策略由用户明确决定。
所有对象删除、生命周期变更、multipart abort 和链上写入另需明确范围；本次提示词不包含这些授权。
没有账号时继续完成本地测试；没有 Kubo 只阻止真实 IPFS adapter 验证，不阻止 AWS/R2；
没有 Forge 只阻止对应 Foundry 验收，不阻止协议/Go/Web。R03 DACL 仅在相关配置路径阻塞时处理，不能关闭保护。
R01/R02 仍为 FAIL，四个现有脚本不能作为真实索引或清理基础；R06 Moderation 缺 UI 不成为 S01–S03 的前置。

## 8. 后续而非首期退出条件

- 双副本/IPFS+云镜像、平台托管 public profile、费用补贴与可用性服务承诺。
- 私有仓库、E2EE、撤权/密钥轮换、浏览器私有读取与元数据泄露模型。
- MinIO/其他云/任意兼容 endpoint；每种新增 provider 重新做能力与安全验收。
- 增量 thin pack、压缩/compaction、受保护 multipart 发布优化、自动 orphan/GC。
- 可选 versioning/retention/disaster-recovery 运维规范；不得自动启用会删除可达对象的策略。

这些选择保留在 [开放问题](open-questions.md)。不要为等待它们而推迟首期本地 AWS/R2 BYOS 实现。


## 9. S01–S03 本地实现与可重复验证（2026-09-13）

> **2026-10-05 注**：本节是 2026-09-13 S01–S03 切片的历史快照，保留原记录不作改写。
> 其后 S04（successor 套件/ABI）、S05（CLI/Web 接入）与 S06（测试网部署 + 真实 R2 E2E）
> 已交付，当前状态以本文件头部和 [backlog](backlog.md) 各日期小节为准；
> 本节中“下一步/尚未实现”的表述仅反映当时的进度。

工作树基线：Windows amd64；分支 `dev`；HEAD `0ba06f436558f12d97625b393767440cdd0f9862`，
加本轮未提交源码与既有未提交文档。此记录不绑定新提交，不作为云端/部署证据。
未改合约/ABI、普通网络 profile、既有事实基线或四份文档的 HISTORICAL 原文；未 commit/push。

### 已冻结的 wire schema 1

2.1 的 PackManifest / PackEntry 字段名保留，`commit.algorithm` 当前只接受 `sha1`，`packVersion=2`。
`schemaVersion=1` 仅指 manifest schema，不决定 successor Suite 版本。
所有字段必填；拒绝未知字段、缺失字段、null、重复 key（含转义后重复）、无效 UTF-8/Unicode、尾随数据、非 canonical bytes。
只有 `dependsOn=[]` / `thin=false` 可用；pack sequence 从 0 连续递增，摘要唯一，locations 内也不重复。
完整 ref 只接受 `refs/heads/*` / `refs/tags/*`，UTF-8 最多 255 字节，按 Git ref 规则拒绝危险字符；不做 Unicode 正规化。
chainId 是正的无前导零十进制字符串，最大 uint256；size 是正十进制字符串，应用下述资源限额。
Directory/repoId 分别为 lowercase 0x-address / 0x-bytes32，OID 为非零 lowercase SHA-1。

`PackLocation` 固定为 `{provider,url,reader}`，三个字段都存在，`url` / `reader` 恰有一个非空：

- `provider` 只接受 `aws-s3`、`cloudflare-r2`、`ipfs`。
- 云公开位置用稳定 HTTPS URL；独立鉴权位置用最多 64 字符的本地 reader label，manifest 不存私有 bucket/密钥。
- IPFS 使用 `ipfs://CID`，reader 为空；CID 是独立寻址信息，不能代替 raw SHA-256。
- HTTPS grammar 明确限制为 ASCII 域名与普通路径段，拒绝 userinfo/query/fragment/显式端口/percent encoding/IP 字面量/点路径。
  自定义公开域名可用，但不能把它作为任意写入 API endpoint。DNS 与已验证 IP 拨号由 Go transport 完成；浏览器只做 wire URL 校验。

`ManifestCommitment={sha256,size,bootstrapLocator}` 始终在 hash body 外；parse API 要求传入独立可信的上下文和 commitment。
`SHA256(UTF8(JCS(body)))` 是 manifest digest，接收器不接受额外换行/BOM或仅语义等价的非规范文本。

| 限额 | 当前实现 |
|---|---|
| manifest bytes / JSON depth | 64 KiB / 16 |
| packs / locations per pack | 16 / 4 |
| pack bytes / manifest 声明总量 | 512 MiB / 2 GiB |
| single PUT | AWS/R2 均最多 16 MiB |
| multipart | 仅 AWS，本地 SDK/HTTP 测试；8 MiB 分片，最多 512 MiB |
| R2 超限 | 请求和源文件 IO 前明确拒绝；无 multipart fallback |
| retries | 单 PUT / GET / UploadPart 最多 3 次，100/200ms 退避；取消即停 |
| AWS Complete 409 | 最多 2 个会话，重传所有分片；保存旧 ID，不自动 abort |

Go JCS 锁定 `cyberphone/json-canonicalization@19d51d7fe467`，加严格 token/Unicode/深度前置检查；
TS 锁定 `canonicalize@2.1.0`，加保留重复 key 信息的解析器。
共享 [vectors.json](../protocol/packmanifest/vectors.json) 含 11 个正向、55 个反向向量，由真实 TS 实现生成；Go 独立计算 bytes/digest，TS 再读取 Go 输出。
测试包含数值极值、UTF-16 排序、Unicode/转义、数组顺序、uint256、非法/超限/上下文/摘要/长度拒绝。

### 源码与本地 API

- [Go manifest](../cli/internal/packmanifest/manifest.go)、[TS manifest](../web/src/lib/packmanifest.ts)、[strictjson](../cli/internal/strictjson/json.go)。
- [packstore](../cli/internal/packstore/store.go)：`Source/Object/Reader/Writer/Capabilities/Receipt`、`Spool/Generate/ReadVerified`；64 KiB copy buffer、任务临时文件、失败清理、取消关闭响应。
- [完整历史 pack](../cli/internal/gitio/verified.go)：`PackFullHistory` 拒绝 shallow/非 SHA-1；无 remote exclusions/--thin；`IndexVerified` 要求显式 expected object，先验证再用 `index-pack --strict`，检查目标 commit 与可达历史。
  Windows 上文件已关闭后重新打开；临时文件内容变化会在 Git 摄取前再次检测。SDK 传输/签名可能再读源文件，但不把整个 pack 放入内存。
- [IPFS adapter](../cli/internal/packstore/ipfsstore/ipfs.go) / [流式 Kubo](../cli/internal/ipfs/stream.go)：保留现有客户端及 legacy 测试；新 wrapper 不自动 pin/GC，raw readback 不等于 pin/replication。
  现有 helper 尚未切换到新接口，旧 `PackURIs` 顺序语义保持不变；`OpenLegacy` 不声称存在 v3 链上 raw digest。
- [AWS/R2](../cli/internal/packstore/s3store/s3.go)：SDK S3 `v1.78.2` / core `v1.36.3` / smithy `v1.22.2`，仍兼容 `go 1.22`。
  AWS 与 R2 分别派生官方 endpoint、region、大小和 multipart 能力；无默认 credential discovery、共享文件/IMDS/ECS 或自建 endpoint。
- [配置](../cli/internal/storageconfig/config.go) 与 [绑定选择](../cli/internal/storageconfig/binding.go)：网络配置完全独立，repo 用 chainId+Directory+repoId 选择 writer/reader。
  首个凭据来源仅支持 `kind=env` 的显式变量名称，可选 session token 变量名称；不读取命名 AWS profile / OS vault，相关扩展未实现。
  鉴权 reader 必须独立引用，匿名 reader 使用 publicReadBase；`NewReader` 禁止写入。权限范围仍由 AWS IAM/R2 token 实际配置决定。
- [安全 transport](../cli/internal/safehttp/http.go)：禁止重定向、ambient proxy、私网/metadata/特殊用途 IPv4/IPv6；解析一次后拨已校验数字 IP；混合公网/私网 DNS 结果整体拒绝。
  公共 GET 不携带 Cookie/Authorization/Session token；签名请求只能发到 profile 派生的官方 host。
- [准备流程](../cli/internal/packstore/prepare.go)：先验证所有源，上传 pack 后再上传 canonical manifest；全部验证成功才返回 proposed commitment。
  manifest 失败仍返回已有 pack receipts。该 API 不写 ref，不提供 CAS/nonce/交易能力；公开 URL 可达性/CORS 仍需独立诊断，不能由私有 SDK GET 推导。
- [恢复](../cli/internal/packstore/s3store/recovery.go)：`PutRecoverable` 在 IO 前记录 key，再记录会话/完成分片；`SaveReceipt/LoadReceipt` 沿用现有 Windows DACL / Unix 权限保护。
  `Recover` 先完整 GET：匹配则复用、损坏则拒绝、确认 404 才重新上传，保留旧 ID。采用全分片重传，不信任本地 ETag，也不做自动 abort/Delete/GC。
  创建会话响应完全丢失时可能只知道最终 key、不知道 upload ID；不能声称能自动找回所有孤儿会话。
  checkpoint 是本地观测，不是链上或持久可用性证明；进程重启需用户/后续 S05 明确选择恢复。

### 无账号即可运行

[配置示例](examples/storage-byos.json) 仅含假 bucket/account/Directory 和环境变量名称，不要当成部署配置。
本轮实现 `doctor/show`；`storage add`、交互密钥存储、真实云 canary 和普通 Git remote 的 BYOS 发布入口仍未实现。
以下 doctor/show 不解析密钥、不访问云、不加载原有 network/IPFS 配置，也不修改任何 bucket policy/CORS：

```powershell
Set-Location D:/inj/next-injective-git/cli
go run ./cmd/igit storage doctor ../docs/examples/storage-byos.json
go run ./cmd/igit storage show ../docs/examples/storage-byos.json
go test -count=1 ./internal/packmanifest ./internal/packstore/... ./internal/storageconfig ./internal/safehttp ./internal/gitio ./internal/ipfs ./cmd/igit

Set-Location D:/inj/next-injective-git/web
npm run test:storage-cross
npm run fixtures:storage # only when intentionally regenerating reviewed vectors
```

公开读取 `DiagnosePublic` 必须显式调用，只做带 Origin 的完整 GET：分别返回 bytes verified 和 CORS 是否允许该 origin。
GET 成功但没有匹配 ACAO 时，verified=true、CORS=false；它不会修改云资源，也不能替代真实浏览器验收。
writer 至少需指定桶/前缀 PUT+GET；reader 为独立 GET 身份或匿名公开域名。
AWS multipart 只用 create/upload/complete 请求，不要求自动删除权限；R2 token 的作用域必须按厂商实际能力配置，不能把应用 prefix 当 IAM 强制边界。

### 本轮验证记录与剩余边界

运行日期：2026-09-13，Asia/Shanghai；Go `go1.26.5 windows/amd64`，Node `v24.11.1`。
TEMP/TMP/SystemRoot/SystemDrive 已检查为有效绝对环境位置；测试使用 task temp、fake credentials 和映射到 httptest 的受控 transport。

| 命令/检查 | 状态 | 实际验证层 |
|---|---|---|
| Go storage/manifest/config/IPFS/Git 新测试 | PASS | 本地协议/mock；真实本地 Git SHA-1 pack v2 |
| `npm run test:storage-cross` | PASS | Go→TS / TS→Go bytes+digest，使用共享向量 |
| `go test -count=1 ./...` | PASS | CLI 全量本地回归；显式 `IGIT_RUN_NATIVE_KUBO_INTEGRATION=0`，原 native Kubo smoke 未执行，不计 IPFS PASS |
| `go vet ./...` | PASS | 静态检查 |
| `npm run test:api` / `npm run typecheck` | PASS | 140 个 Web API 测试通过、0 skipped，TS 编译通过；不等于 Web BYOS browse |
| `go run ./cmd/igit storage doctor ../docs/examples/storage-byos.json` | PASS | 4 profiles / 2 repository bindings，本地无密钥诊断 |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/igit/... ./cmd/git-remote-igit/... ./internal/packstore/... ./internal/gitio/...` | PASS | Linux amd64 交叉编译，无真实 Linux 执行；多 package 模式不输出/覆盖现有二进制 |
| `go test -race -count=1 ./internal/packstore/...` | BLOCKED | 当前 CGO_ENABLED=0，Go 返回 `-race requires cgo`；PATH 未找到 gcc/clang |
| `git diff --check`（包含既有文档） | FAIL | 仅报告 README/backlog HISTORICAL 原文的已有 Markdown 尾随空格；按保留要求不清理 |
| 新 journal Windows DACL 写入/读取 | PASS | 临时目录及假 receipts；不覆盖基线 R03 对其他真实路径的 BLOCKED |
| 真实 AWS / 真实 R2 | NOT PROVEN | 无云 canary、费用/权限/CORS/厂商 multipart 实证 |
| 真实 IPFS / replication | NOT PROVEN | 新 wrapper 仅 fake/local HTTP；无 Kubo 写入/pin |
| successor 合约/部署/交易 | NOT PROVEN | S04 未实现；未改或调用现有 v3 ABI |
| Windows/Linux Git remote push/clone E2E | NOT PROVEN | S05–S06 未贯通；本地 pack/index fixture 不是链上 Git E2E |
| 真实浏览器公共 URL / CORS | NOT PROVEN | 仅 mock GET/ACAO 诊断 |

S04 下一步是 reviewed successor state/query/event 和 revision CAS/versioned dispatch；随后 S05 接入 CLI/Web。
同 commit 变更、fork 目标绑定、delete/recreate ABA、CAS/revert/reorg/不确定 tx receipt 的端到端恢复仍待 S04/S05，不能由上传测试推出。
S06 真实 AWS/R2 与测试网分别需要明确桶/前缀、费用与写入授权和安全渠道配置的专用凭据；本轮无需提供真实密钥。
S07 仅在单独选择历史导入时启动；V1 archive、四个旧 indexer 脚本和历史部署证据保持原边界。

官方原始资料本轮再次查询：2026-09-13 21:50（Asia/Shanghai），RFC 8785、AWS conditional writes、R2 S3 compatibility / Go SDK、Git pack-objects 均 HTTP 200。
链接沿用第 2/4 节。厂商能力记录不是 canary PASS；R2 multipart 仍明确不提供。

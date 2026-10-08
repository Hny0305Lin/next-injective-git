# Glossary / 术语表

EN↔ZH terminology for Next Injective Git documentation.
**Rule: check this table before translating anything. If a term is missing
here, add it here first, then use the table's wording everywhere.** Do not
invent alternative translations in either language (see `docs/AGENTS.md`).

**规则：翻译任何内容前必须先查本表；表里没有的术语，先补表、再使用表中译法。**
中英文档不得发明新译法。

## Core Terms / 核心术语

| English | 中文 | Notes / 备注 |
|---|---|---|
| Suite | 套件（Suite） | The non-upgradeable Injective EVM contract suite; keep "Suite" in code identifiers and addresses. 不可升级的 Injective EVM 合约套件。 |
| Suite v3 | Suite v3（v3 套件） | Legacy IPFS/Kubo path. Frozen by design. 旧路径：IPFS/Kubo，按设计冻结。 |
| Suite v4 | Suite v4（v4 套件） | Storage-neutral BYOS successor, suiteVersion 4. Production providers: Amazon S3 and Cloudflare R2 only. 存储中立后继（BYOS），生产配置仅支持 Amazon S3 与 Cloudflare R2。 |
| BYOS (bring-your-own-storage) | BYOS（自带存储） | Users own/administer their cloud buckets. 用户自备并管理云存储桶。 |
| CAS | CAS（比较并交换） | Compare-and-swap concurrency guard for ref updates. 引用更新的并发守卫。 |
| manifest schema | manifest 模式（manifest schema） | Versioned canonical-JSON pack manifest; schema 1 frozen, schema 2 = incremental packs (ADR 0005). 版本化的 canonical JSON pack 清单。 |
| canonical JSON | canonical JSON（规范化 JSON） | RFC 8785 JCS baseline (ADR 0004). 以 RFC 8785 JCS 为基线。 |
| cutover | 切换（cutover） | Evidence-gated activation/transition. Do not translate as "上线". 以证据为门禁的激活/过渡。 |
| canary | canary（金丝雀验证） | Small real-environment verification (e.g. real AWS S3 canary). 小规模真实环境验证。 |
| ADR | ADR（架构决策记录） | Architecture Decision Record; originals in `docs/adr/` are immutable evidence. 原件是不可变证据。 |
| control plane | 控制平面 | On-chain repository state. 链上仓库状态。 |
| data plane | 数据平面 | Pack transport/storage. pack 传输与存储。 |
| pack / packfile | pack / packfile（包文件） | Git pack objects. 不译。 |
| ref | ref（引用） | Git reference. |
| revision CAS | revision CAS | Suite v4 ref concurrency version; keep untranslated. |
| bootstrap locator | 引导定位符（bootstrap locator） | Stable manifest locator held on chain. 链上保存的稳定 manifest 定位信息。 |
| fresh-empty-suite | fresh-empty-suite（空状态全新套件） | Zero-state activation scope. 零状态激活范围。 |
| archive preview | 归档预览（archive preview） | Read-only CosmWasm V1 viewer. 只读 V1 查看器。 |
| verification readback | 验证回读（readback） | Full read-back check after upload. 上传后的完整回读校验。 |
| NOT PROVEN | NOT PROVEN（未证实） | Status bound for claims without evidence; do not soften in translation. 无证据时的状态用语，翻译不得弱化。 |
| BLOCKED / PASS / FAIL / HISTORICAL | BLOCKED / PASS / FAIL / HISTORICAL | Status bounds; keep English labels. 状态用语保留英文。 |

## Provider Terms / 厂商术语

Provider IDs are suggested values; the implemented code is the source of
truth. **All six are roadmap candidates only — not supported, not integrated**
(see [roadmap-byos-providers](roadmap-byos-providers.md)).
Provider ID 为建议值，实现以代码为准。六家均为 roadmap 候选，未实现、未接入。

| Provider ID (suggested) | 中文名 | English name | Product page / 产品页 |
|---|---|---|---|
| tencent-cos | 腾讯云 COS | Tencent Cloud COS | https://cloud.tencent.com/product/cos |
| aliyun-oss | 阿里云 OSS | Alibaba Cloud OSS | https://www.aliyun.com/product/oss |
| huawei-obs | 华为云 OBS | Huawei Cloud OBS | https://www.huaweicloud.com/product/obs.html |
| volcengine-tos | 火山引擎 TOS | Volcengine TOS | https://www.volcengine.com/product/TOS |
| bitiful-s4 | 缤纷云 S4 | Bitiful S4 | http://bitiful.com/s4 |
| ctyun-zos | 天翼云 ZOS | China Telecom ZOS | https://www.ctyun.cn/products/zos |

## Phrase Boundaries / 表述边界

| Must say | Must NOT say |
|---|---|
| "roadmap candidate / roadmap 候选" for the six mainland-China providers | "supported / 已支持 / 已接入" for any of them |
| "production: Amazon S3 and Cloudflare R2 only / 生产配置仅 Amazon S3 与 Cloudflare R2" | any other provider presented as available |
| "SuiteDirectory remains empty until approved / 审批前 SuiteDirectory 保持为空" | test addresses presented as official addresses |

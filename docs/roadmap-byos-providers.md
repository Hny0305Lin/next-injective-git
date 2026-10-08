# BYOS Provider Roadmap — Mainland China S3-Compatible Endpoints

Status: **Roadmap (planned follow-up). Not implemented. Not verified.**
Last reviewed: 2026-10-09 (Asia/Shanghai).

> [!IMPORTANT]
> This page is a **roadmap for future work on the Suite v4 BYOS data plane**.
> The currently delivered Suite v4 production configuration supports **Amazon
> S3 and Cloudflare R2 only** ([ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md),
> [BYOS specification](storage-byos.md)). Nothing on this page is implemented,
> integrated, or verified in any released CLI/Web build, and no endpoint below
> may appear in configuration examples, CLI flags, or production guidance
> until the corresponding implementation and review evidence is approved.
>
> "S3-compatible" here refers exclusively to the **Suite v4 BYOS data plane**
> (object-storage pack transport). It is unrelated to Suite v3, IPFS/Kubo, or
> the frozen legacy IPFS path.

## Provider Candidates

The table below records provider candidates contributed by the repository
owner. **Provider IDs are suggested values only; the implemented code is the
source of truth** and may assign different identifiers when this roadmap is
delivered.

| Provider ID (suggested) | 厂商 | 产品页 |
|---|---|---|
| tencent-cos | 腾讯云 COS | https://cloud.tencent.com/product/cos |
| aliyun-oss | 阿里云 OSS | https://www.aliyun.com/product/oss |
| huawei-obs | 华为云 OBS | https://www.huaweicloud.com/product/obs.html |
| volcengine-tos | 火山引擎 TOS | https://www.volcengine.com/product/TOS |
| bitiful-s4 | 缤纷云 S4 | http://bitiful.com/s4 |
| ctyun-zos | 天翼云 ZOS | https://www.ctyun.cn/products/zos |

Provider names are kept in their original Chinese; product pages open in a
new tab.

## Ground Rules For This Roadmap

1. **No support claims.** Until an implementation lands with passing gates
   (Go/TS cross vectors, capability profiles, clean-environment Windows/Linux
   E2E per [ADR 0002](adr/0002-pluggable-pack-storage.md)), every mention of
   these providers — in English or Chinese — must stay in roadmap wording.
   Pages must not say "supported", "已支持", or "已接入".
2. **No endpoint examples.** Documentation, CLI help, and configuration
   samples must not contain these vendors' endpoints or bucket-URL examples.
3. **Scope guard.** Any provider added here must go through the same BYOS
   capability-profile review as S3/R2 ([BYOS specification](storage-byos.md));
   self-hosted object stores and arbitrary S3-compatible endpoints remain out
   of scope for v4 production configuration unless separately decided.
4. **Terminology.** Suite v3 = IPFS/Kubo legacy path; Suite v4 = BYOS
   successor (production: Amazon S3 + Cloudflare R2 only). Check the
   [glossary](glossary.md) before writing provider-related content.

## Open Questions (non-binding)

- Capability profiles per provider (versioning, retention, CORS, readback
  behavior) are not yet drafted; each will require its own tested profile.
- Whether mainland-China endpoints require a distinct default public-read
  domain or mirror guidance is undecided.
- Priority order among the six candidates is not decided.

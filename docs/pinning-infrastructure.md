# Pinning 基建选型（open-questions §2 落地方案）

> 状态：调研稿（2026-07）。§2 已定案路径 A→B 并行：自建 pin 节点保活 + 托管 pinning 兜底，本文给出具体架构与实施清单。

## 1. 目标与约束

- **持久性**：push 者本地 Kubo 下线后，packfile 必须仍可被任何人 clone
- **大陆可用性**（硬约束）：ipfs.io / dweb.link / Cloudflare 网关在大陆均被阻断；官方 bootstrap 部分被阻断导致 DHT 寻址慢。CLI 默认网关必须换成自家网关
- **成本**：MVP 阶段 packfile 总量小（KB~MB 级/仓库），单节点即可起步

## 2. 推荐架构（Phase A：自建，上线前必做）

```
                    ┌─ 大陆用户 ────────────────┐
                    │  igit push/clone           │
                    │  (本地 Kubo + peers 直连)  │
                    └──────────┬─────────────────┘
                               │ swarm connect（绕开 DHT）
              ┌────────────────▼────────────────┐
              │  自建 pin 节点（香港/新加坡 VPS） │
              │  kubo daemon + nginx 反代网关     │
              │  gateway.igit.example (443, TLS) │
              └────────────────┬────────────────┘
                               │ pin 同步（Phase B: ipfs-cluster）
              ┌────────────────▼────────────────┐
              │  托管 pinning 兜底（Pinata 等）   │
              └─────────────────────────────────┘
```

### 节点选型

| 项 | 建议 | 理由 |
|---|---|---|
| 机房 | 香港 / 新加坡（大陆直连线路，如 CN2） | 大陆延迟 <80ms，无墙 |
| 规格 | 2C4G + 100GB SSD 起步 | MVP packfile 量级小 |
| 软件 | Kubo（官方，与 CLI 同栈）+ nginx TLS 反代 `:8080` 网关 | 最少组件 |
| 域名 | `gateway.<域名>`（网关）、`node.<域名>`（swarm 直连） | CLI 配置用 |
| 部署 | Docker（`ipfs/kubo` 官方镜像）+ compose | 可复制、可迁移 |

### 关键配置

- Kubo `Peering.Peers` 固定互联；对外公布 `/dns4/node.<域名>/tcp/4001` multiaddr
- 网关只开 **只读 path gateway**（`/ipfs/<cid>`），禁 API 端口外露（5001 仅内网）
- pin 策略：**监听合约事件自动 pin**——indexer 雏形：轮询 LCD 拉 `update_ref` 事件 → 提取 `pack_uris` → `ipfs pin add`（一个 50 行脚本即可起步，后续并入 §11 indexer）

### CLI 侧配套改动（待实施）

1. `config.json` 新增 `peers` 字段（multiaddr 列表），helper 启动时 `swarm connect` 直连自家节点，绕开 DHT 慢寻址
2. `Defaults()` 的 `ipfs_gateway` 从 `https://ipfs.io` 改为自家网关
3. push 成功后（可选）调用自家节点 pinning API 主动触发 pin，不等事件轮询

## 3. Phase B：托管 pinning 兜底（与 A 并行）

| 服务 | 免费额度 | 大陆可达性 | 备注 |
|---|---|---|---|
| Pinata | 1GB / 500 pins | API 可达性一般，网关被墙 | 生态最成熟，Pinning Service API 标准 |
| Filebase | 5GB | S3 兼容 API，可达性尚可 | 多地理冗余，企业向 |
| 4EVERLAND | 有免费档 | 亚洲节点较多 | 提供 IPFS Migrator 迁移工具 |

- 结论：**Filebase 或 4EVERLAND 优先**（亚洲/可达性更好），通过标准 [Pinning Service API](https://ipfs.github.io/pinning-services-api-spec/) 接入——Kubo 原生支持 `ipfs pin remote`，pin 脚本同一份逻辑双写即可
- 用途：自建节点故障时的兜底副本，不承担用户流量

## 4. Phase C/D（远期，不阻塞上线）

- Filecoin 存储交易：真正的持久化保证，适合 release 快照类冷数据（配合 §5.1 已实施的通用 URI，将来可写 `filecoin://` / `ar://`）
- 代币激励社区 pin 网络：依赖经济模型与代币决策（§3 待定项）

## 5. ipfs-cluster 演进（多节点时再上）

单节点起步不需要 cluster。节点 ≥2 时：
- 用 **ipfs-cluster**（CRDT 模式）做 pinset 编排，`replication_factor` 按节点数设
- 社区志愿节点可用 `ipfs-cluster-follow` 加入**协作集群**（follower 只读跟随官方 pinset，零信任要求）——这是 Phase D 激励网络的技术雏形，先行验证

## 6. Injective 端点大陆可达性（同属 §2 待验证项）

- 本轮实测（2026-07，WSL 环境）：`testnet.sentry.lcd.injective.network` / `testnet.sentry.tm.injective.network` 均可直连 ✅
- 上线前需在**纯大陆家宽/移动网络**复测主网端点；若不稳，LCD 反代可与网关同机部署（nginx 加一条 upstream 即可）

## 7. 实施清单（按序）

- [ ] 购置 HK/SG VPS + 域名 + TLS（1 天）
- [ ] Docker 部署 Kubo + nginx 只读网关（0.5 天）
- [ ] 事件轮询 pin 脚本（LCD → pack_uris → pin add）（1 天）
- [ ] CLI：`peers` 配置 + swarm connect + 默认网关切换（0.5 天）
- [ ] 注册 Filebase/4EVERLAND，pin 脚本双写远程 pinning（0.5 天）
- [ ] 大陆网络实测：家宽 clone 一个无本地节点的仓库（验收标准：60s 内完成）

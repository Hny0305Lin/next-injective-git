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

- 结论：**Filebase 或 4EVERLAND 优先**（亚洲/可达性更好）
- ⚠️ 实测（2026-07）：Filebase **免费档不开放 Pinning Service API**（`ipfs pin remote` 返回 403 "requires a paid account"）。改走 **S3 兼容 API + CAR 导入**：`ipfs dag export <cid>` 后 PUT 到 `s3.filebase.com/<bucket>/<cid>.car` 带 `x-amz-meta-import: car` 头，Filebase 以**原始 CID** pin（已验证 CIDv1 严格一致；注意 curl 需 `--http1.1`，HTTP/2 会挂起）
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

- [x] 购置 HK VPS（45.202.249.80，Debian 11，1C/1G/5G）；域名 + TLS 已完成（见下）
- [x] 部署 Kubo + nginx 只读网关——`scripts/gateway-deploy.sh`（直跑二进制而非 Docker，省磁盘；`lowpower,server` profile + 512M swap + StorageMax 1GB + systemd MemoryMax 750M）。已验收：外网 `/ipfs/<cid>` 可取回 packfile，启动后内存用 250M/磁盘 1.6G
- [x] 域名 + TLS——`scripts/gateway-tls.sh`（certbot --nginx，Let's Encrypt，certbot.timer 自动续期）。❗重要：原定 `ipfs-gateway-hk.haohanyh.com` 经实测 **`haohanyh.com` 全域被 GFW 按 SNI/Host 封锁**（大陆任意子域 HTTP/HTTPS 均被 RST，ICP 备案也无法绕过；诊断方法：`curl --resolve <域名>:443:<IP> https://<域名>/`）。已改用干净域名 **`igit-hk.haohanyh.ovh`**（不同注册域，已验证大陆 HTTPS 直达），Cloudflare 灰云/DNS-only 直指源站
- [x] 事件轮询 pin 脚本（LCD → pack_uris → pin add）——`scripts/pin-indexer.sh`：轮询合约 `update_ref` 交易，从 tx 消息体提取 `pack_uris`（事件不含该字段），本地 pin + Filebase CAR 双写，状态记录于 `~/.igit/pinned.list`（待迁到 HK 节点以 systemd timer 自动 pin）
- [x] CLI 默认网关切换：`~/.igit/config.json` 的 `ipfs_gateway` 改为 `https://igit-hk.haohanyh.ovh`（`peers`/swarm connect 优化待代码支持）
- [x] 注册 Filebase，pin 脚本双写远程 pinning（凭据在 `~/.igit/filebase.env`，不入库）；4EVERLAND 待注册接入
- [ ] 大陆网络实测：家宽 clone 一个无本地节点的仓库（验收标准：60s 内完成）

# igit 去中心化基建架构（as-built）

> 状态：已部署并验证（2026-07）。本文记录**当前实际跑着的**基建——HK 网关、域名/TLS、Filebase 托管 pinning、pin-indexer、用量告警、SSH 加固、swarm 直连，以及规划中的 CLI 加速。
> 选型/调研过程见 [pinning-infrastructure.md](./pinning-infrastructure.md)；本文是「建成后」的权威参考。

## 1. 拓扑总览

```
   ┌─────────────────── 写入路径 ───────────────────┐
   igit push ─▶ Injective 合约 repo-registry
                  update_ref(pack_uris = ipfs://<cid>)
                          │  (每 2 分钟轮询)
                 ┌────────▼─────────┐
                 │ HK: pin-indexer  │  从 tx 消息体取 pack_uris
                 │ (systemd timer)  │
                 └───┬──────────┬───┘
        本机 pin ────┘          └──── CAR 导入(保留原始 CID)
        ┌────────────┐              ┌──────────────────┐
        │ HK Kubo 节点│              │ Filebase(S3/IPFS)│  兜底副本
        └────────────┘              └──────────────────┘

   ┌─────────────────── 读取路径 ───────────────────┐
   大陆用户 ─HTTPS─▶ https://igit-hk.haohanyh.ovh/ipfs/<cid>
                        │ nginx 只读网关
                        ├─ 本地 Kubo 命中(pin + 全网可抓) ⚡
                        └─ miss/25s 超时 → 回退反代 ipfs.io
```

- **三处副本**：推送者本地 Kubo、HK 网关节点、Filebase。任一存活即可 clone。
- **大陆可达**：走自家 HK CN2 域名(HTTPS 直达，已实测)，不依赖任何被墙的公共网关。

## 2. 组件明细

### 2.1 HK 网关节点 `45.202.249.80`
- Debian 11，1C/1G/5G（唯一一台）。PeerID `12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W`。
- Kubo v0.42.0 二进制（非 Docker，省磁盘）；`ipfs init --profile=lowpower,server`、StorageMax 1GB、ConnMgr 32/96、512M swap；systemd `ipfs.service`（MemoryMax 750M）。
- Kubo 的 API(5001)/Gateway(8080) 只绑 `127.0.0.1`，一律由 nginx 前置。
- 部署脚本：`scripts/gateway-deploy.sh`（root 在服务器执行，幂等）。

### 2.2 nginx 只读网关 + 公共网关兜底
- 仅暴露 `/ipfs/<cid>`（只读 path 网关）与 `/healthz`，其余 403。
- `/ipfs/`：本地 Kubo 命中优先；miss 或 25s 超时 → `error_page` 回退到命名 location `@pubgw` 反代 `https://ipfs.io`。
- ⚠️ `cloudflare-ipfs.com` / `cf-ipfs.com` 已于 2024-08 停用（从 HK 都 000），故兜底用 `ipfs.io`。ipfs.io 对**网站/HTML 目录**会 301 到 `*.ipfs.dweb.link`（大陆被墙），但 **raw/文件内容（igit packfile）直接 200**，对 igit 无碍。
- 配置写在 `scripts/gateway-tls.sh` 的 nginx heredoc 中（单一来源）。

### 2.3 域名与 TLS `igit-hk.haohanyh.ovh`
- Cloudflare 灰云 / DNS-only，A 记录直指 `45.202.249.80`（不走 Cloudflare 代理——免费档大陆路由更差且不解封）。
- Let's Encrypt 证书，`certbot --nginx`，`certbot.timer` 自动续期（`--keep-until-expiring` 可幂等重跑复用证书）。脚本 `scripts/gateway-tls.sh`。
- **GFW 教训（重要）**：原定 `ipfs-gateway-hk.haohanyh.com` 经实测——**整个 `haohanyh.com` 注册域被 GFW 按 SNI/Host 封锁**，大陆任意子域 HTTP/HTTPS 均被 RST，ICP 备案也无法绕过。换到不同注册域 `haohanyh.ovh` 后干净可用。
- **诊断法**（选新域名前先测）：`curl --resolve <域名>:443:<IP> https://<域名>/`——被 RST=被封，能到 TLS(哪怕证书不匹配)=干净。ping 只打 IP、无域名，**不能**用来判断封锁。

### 2.4 Filebase 托管 pinning（兜底副本）
- bucket `a10101`，免费档 **5GB / 最多 1000 文件**（当前用量极小；真正的约束是文件数，非容量）。
- 免费档**不开放** Pinning Service API（`ipfs pin remote` 返回 403）。改用 **S3 兼容 API 的 CAR 导入**：`ipfs dag export <cid>` 出 CAR → 上传时带 `import=car` 元数据 → Filebase 以**原始 CID** pin（`x-amz-meta-cid` 与本地 CIDv1 一致、`pinning-status: pinned`，已验证）。
- 上传实现（`fb_upload_car`，优先 awscli 回退 curl）：
  - **awscli**：`aws s3api put-object --bucket <b> --key <cid>.car --body <car> --metadata import=car --endpoint-url https://s3.filebase.com`（Debian 11 的 curl 7.74 **不支持** `--aws-sigv4`，故 HK 上用 awscli）。
  - **curl**：`PUT --aws-sigv4 -H x-amz-meta-import:car --http1.1`（新版 curl；必须 `--http1.1`，HTTP/2 会挂起）。

### 2.5 pin-indexer（HK 常驻，自动双写）
- `scripts/pin-indexer.sh` + `scripts/pin-indexer-install.sh`。
- systemd `pin-indexer.timer`（每 2 分钟）触发 `pin-indexer.service`（oneshot，`--once`）。
- 逻辑：轮询 LCD 查合约 `update_ref` 交易 → 从 **tx 消息体**（事件不含 pack_uris）提取 `pack_uris` → 对每个新 CID：本机 Kubo `pin add` + Filebase CAR 上传。
- 去重状态 `/var/lib/igit/pinned.list`；凭据从 `/etc/igit/monitor.env` 注入；`IPFS_PATH=/var/lib/ipfs`。

### 2.6 Filebase 用量告警
- `scripts/filebase-monitor.sh` + `scripts/monitor-install.sh`；systemd `filebase-monitor.timer`（每小时检查）。
- 用 awscli 统计**文件数 vs 1000**、**存储 vs 5GB**；跨 **60/80/90/100%** 分档告警（`ALERT_BANDS` 可配置）；同档 **8 小时**冷却（`ALERT_INTERVAL_HOURS`），升档立即发。
- 发信走 **Resend API**（`onboarding@resend.dev` → `1553809191@qq.com`）。

### 2.7 SSH 加固 与 swarm/4001
- **仅密钥登录**：`scripts/gateway-ssh-harden.sh` 写 `/etc/ssh/sshd_config.d/99-igit-hardening.conf`（`PasswordAuthentication no`、`PermitRootLogin prohibit-password`）。登录密钥 `~/.ssh/igit_hk`（WSL 侧，唯一入口；应急走 VPS 控制台）。
- **4001（TCP+UDP/QUIC）已开放**：服务器无 ufw/iptables/nft 规则、云侧不限端口。大陆 `ipfs swarm connect` 到 HK 实测成功（TCP+QUIC），为下文 CLI 加速打好基础。

## 3. 脚本清单（均已入库、无密钥，root 在服务器执行）

| 脚本 | 作用 |
|---|---|
| `gateway-deploy.sh` | 装 Kubo + nginx 只读网关 + systemd |
| `gateway-tls.sh` | 签 Let's Encrypt 证书 + nginx(含公共网关兜底) |
| `gateway-ssh-harden.sh` | 关闭密码登录、仅密钥 |
| `pin-indexer.sh` | 轮询合约、本机 pin + Filebase CAR 双写 |
| `pin-indexer-install.sh` | 把 pin-indexer 装成 systemd timer(每 2 分钟) |
| `filebase-monitor.sh` | Filebase 用量分档检查 + Resend 告警 |
| `monitor-install.sh` | 把 monitor 装成 systemd timer(每小时) |

## 4. 凭据 / 密钥位置（**均不入库**；此处只记路径，不记明文）

| 位置 | 内容 | 权限 |
|---|---|---|
| HK `/etc/igit/monitor.env` | FILEBASE_*、RESEND_API_KEY、ALERT_TO/FROM | 600 |
| WSL `~/.igit/filebase.env` | FILEBASE_*（本地调试用） | 600 |
| WSL `~/.ssh/igit_hk` | HK 登录私钥 | 600 |
| HK `/var/lib/igit/pinned.list` | pin 去重状态（非密钥） | — |

`.gitignore` 已排除 `scripts/tmp-*.sh` 与暂存私钥。**Filebase/Resend 凭据曾在会话中明文出现，建议择机轮换。**

## 5. 客户端配置
- `~/.igit/config.json` 的 `ipfs_gateway = https://igit-hk.haohanyh.ovh`（CLI 读不到本地内容时的网关）。

## 6. 已实现：CLI `peers` + swarm connect（大陆 push/clone 加速）

**目的**：helper 启动时直连 HK 节点，绕过 DHT 慢寻址。已实测：编译通过、大陆 `/dns4` 与 `/ip4` swarm connect 均成功、helper 启动即连上并打印 `direct swarm peer connected`。

**为什么「N 仓库/一个用户」不需要担心互相干扰**：
- 本地只有一个 IPFS 节点、一个 swarm；`swarm connect` 是**节点级**，连一次 HK 覆盖**所有仓库**。
- 内容按 **CID 寻址**，取某仓库只传该 CID 的块，仓库间天然隔离，**零干扰**。
- 故现阶段设计极简：`config.json` 一个**全局 `peers` 列表 = [HK multiaddr]**，helper 启动时逐个 `swarm connect`。

**未来（用户各自跑 pin 节点时）**：把 owner 的 pin 节点 multiaddr **存上链**（如用户注册信息），helper 操作 `owner/repo` 时连「该 owner 节点 + 全局兜底 HK」。叠加式、非侵入，且契合隐私分层（要隐私者只连自己节点，取用不经共享 HK）。

**已落地代码**：
- `cli/internal/config/config.go`：`Config` 加 `Peers []string`；`Defaults()` 预置 HK 的 `/dns4`+`/ip4` 两个 multiaddr（同一节点，抗 IP 变更），并把默认 `ipfs_gateway` 改为 `https://igit-hk.haohanyh.ovh`。
- `cli/internal/ipfs/client.go`：新增 `SwarmConnect(multiaddr)`（POST /api/v0/swarm/connect，12s 超时，best-effort）。
- `cli/cmd/git-remote-igit/main.go`：入口处对 `cfg.Peers` 逐个 swarm connect（连上任一即提示；无 daemon/peer 不可达则静默回退网关）。

**未来演进（自托管）**待做：owner 节点 multiaddr 上链 + helper 按 `owner/repo` 连对应节点。

HK 直连地址（默认已内置）：
```
/dns4/igit-hk.haohanyh.ovh/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W
/ip4/45.202.249.80/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W
```

## 7. 运维速查

```bash
# SSH（仅密钥）
ssh -i ~/.ssh/igit_hk root@45.202.249.80

# 服务状态（HK 上）
systemctl status ipfs pin-indexer.timer filebase-monitor.timer certbot.timer

# 网关健康 / 取包
curl https://igit-hk.haohanyh.ovh/healthz
curl https://igit-hk.haohanyh.ovh/ipfs/<cid>

# 手动跑一次 pin-indexer / monitor
systemctl start pin-indexer.service
systemctl start filebase-monitor.service

# 证书 / 续期
certbot certificates
systemctl list-timers certbot.timer

# 新域名是否被 GFW 封锁（大陆执行）
curl --resolve <域名>:443:45.202.249.80 https://<域名>/
```

**应急**：私钥丢失 → VPS 服务商 VNC/网页控制台进系统，重新加公钥或临时开回密码。

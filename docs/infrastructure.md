# igit 去中心化基建架构（as-built）

> 状态：已部署并验证（2026-08）。本文记录**当前实际跑着的**基建——HK 热层网关、美国全量 Kubo、Fil.one CAR 归档、受限 CID 同步、域名/TLS、SSH 加固与 swarm 直连。
> 选型/调研过程见 [pinning-infrastructure.md](./pinning-infrastructure.md)；本文是「建成后」的权威参考。

## 1. 拓扑总览

```
   ┌─────────────────── 写入路径 ───────────────────┐
   igit push ─▶ Injective 合约 repo-registry
                  update_ref(pack_uris = ipfs://<cid>)
                          │  (每 2 分钟轮询)
                 ┌────────▼─────────┐
                  │ US: archive-indexer │  从 tx 消息体取全部历史 pack_uris
                  └───────┬─────────────┘
              全量 pin ───┘        └──── CAR 归档 + SHA-256 校验
              ┌──────────────┐          ┌─────────────────┐
              │ US Kubo 850GB│          │ Fil.one S3/Filecoin │
              └──────────────┘          └─────────────────┘
                       │
                       └── 受限 SSH 同步 durable CID 清单 ──▶ HK 热层

   ┌─────────────────── 读取路径 ───────────────────┐
   大陆用户 ─HTTPS─▶ https://igit-hk.haohanyh.ovh/ipfs/<cid>
                        │ nginx 只读网关
                        ├─ 本地 Kubo 命中(pin + 全网可抓) ⚡
                        └─ miss/25s 超时 → 回退反代 ipfs.io
```

- **持久化副本**：推送者本地 Kubo、美国全量 Kubo、Fil.one CAR 归档。香港仅负责热缓存。
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

### 2.4 美国全量 Kubo + Fil.one CAR 归档
- 美国节点 `162.35.187.224`，Debian 13，Kubo v0.42.0，`/var/lib/ipfs` StorageMax 850GB；PeerID `12D3KooWBGyxqNM3q6nHvacFfqnwoXP2uXxP36uSPab2p16ywfFS`。
- `igit-archive-indexer.timer` 每 5 分钟分页扫描所有历史 `update_ref`，将 CID Pin 到美国节点，再导出 CAR 到 Fil.one bucket `us101010`（`https://us-east-1.s3.fil.one`）。
- 对象键为 `cars/v1/<CID前两位>/<CID>.car`；上传后通过 `HeadObject` 校验 `x-amz-meta-sha256`，成功才写入 `/var/lib/igit-archive/archived.tsv`。
- `igit-durable-cid-sync.timer` 用受限 SSH 用户把已验证 CID 清单同步到香港；该账号仅能调用格式校验后的接收命令，不能取得 shell。
- `igit-archive-monitor.timer` 每小时检查 Kubo、归档/同步定时器、美国 Pin 与 Fil.one CAR 对象数量、以及 1TB 用量阈值；异常通过 Resend 发邮件。

### 2.5 Filebase（旧静态副本）
- bucket `a10101` 保留现有对象，但不再接收新 CID；`pin-indexer.timer` 和 `filebase-monitor.timer` 已停用。

### 2.6 香港热层 Pin
- `hot-pin-indexer.timer` 每 5 分钟运行，保留新内容（14 天）、近 30 天至少 3 次成功网关读取的热门内容，以及 `/etc/igit/important-cids.list` 中的手工重要 CID。
- 清理仅作用于 `/var/lib/igit/durable-cids.list` 中已确认美国 Pin 和 Fil.one CAR 成功的 CID；未确认 CID 会跳过。

### 2.7 SSH 加固 与 swarm/4001
- **仅密钥登录**：`scripts/gateway-ssh-harden.sh` 写 `/etc/ssh/sshd_config.d/99-igit-hardening.conf`（`PasswordAuthentication no`、`PermitRootLogin prohibit-password`）。登录密钥 `~/.ssh/igit_hk`（WSL 侧，唯一入口；应急走 VPS 控制台）。
- **4001（TCP+UDP/QUIC）已开放**：服务器无 ufw/iptables/nft 规则、云侧不限端口。大陆 `ipfs swarm connect` 到 HK 实测成功（TCP+QUIC），为下文 CLI 加速打好基础。

## 3. 脚本清单（均已入库、无密钥，root 在服务器执行）

| 脚本 | 作用 |
|---|---|
| `systemd/kubo-us.service` | 美国 Kubo 的 systemd unit 模板 |
| `gateway-deploy.sh` | 装 Kubo + nginx 只读网关 + systemd |
| `gateway-tls.sh` | 签 Let's Encrypt 证书 + nginx(含公共网关兜底) |
| `gateway-ssh-harden.sh` | 关闭密码登录、仅密钥 |
| `archive-indexer.sh` | 美国全量 Pin + Fil.one CAR 归档与哈希校验 |
| `archive-indexer-install.sh` | 安装美国归档定时器 |
| `hot-pin-indexer.sh` | 香港新/热门/重要 CID 热层策略 |
| `hot-pin-indexer-install.sh` | 安装香港热层定时器 |
| `durable-cid-sync.sh` | 美国向香港同步已验证 durable CID 清单 |
| `receive-durable-cids.sh` | 香港受限 SSH 接收端 |
| `archive-monitor.sh` | 美国 Kubo、Fil.one 与同步链路的邮件告警 |
| `archive-monitor-install.sh` | 安装美国每小时归档监控 |

## 4. 凭据 / 密钥位置（**均不入库**；此处只记路径，不记明文）

| 位置 | 内容 | 权限 |
|---|---|---|
| US `/etc/igit/filone.env` | FILONE_* S3 凭据 | 600 root:root |
| US `/etc/igit/archive-monitor.env` | Resend 邮件告警配置 | 600 root:root |
| US `/etc/igit/archive-sync.key` | 仅用于向香港同步 durable CID 的私钥 | 600 root:root |
| WSL `~/.ssh/igit_hk` | HK 登录私钥 | 600 |
| HK `/var/lib/igit/pinned.list` | pin 去重状态（非密钥） | — |

`.gitignore` 已排除 `scripts/tmp-*.sh` 与暂存私钥。任何曾在会话中明文出现的旧 Filebase/Resend 凭据都应轮换。

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
systemctl status ipfs hot-pin-indexer.timer certbot.timer

# 服务状态（US 上）
systemctl status kubo igit-archive-indexer.timer igit-durable-cid-sync.timer igit-archive-monitor.timer

# 网关健康 / 取包
curl https://igit-hk.haohanyh.ovh/healthz
curl https://igit-hk.haohanyh.ovh/ipfs/<cid>

# 手动跑一次香港热层或美国归档
systemctl start hot-pin-indexer.service
systemctl start igit-archive-indexer.service

# 证书 / 续期
certbot certificates
systemctl list-timers certbot.timer

# 新域名是否被 GFW 封锁（大陆执行）
curl --resolve <域名>:443:45.202.249.80 https://<域名>/
```

**应急**：私钥丢失 → VPS 服务商 VNC/网页控制台进系统，重新加公钥或临时开回密码。

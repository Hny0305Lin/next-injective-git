# MVP 与后续 Backlog

本文把 `open-questions.md` 中的方案决策转换成可执行工作项。状态只在有代码/部署、测试和运行证据时标记为完成。

## P0：发布前必须关闭

| 工作项 | 当前状态 | 负责人角色 | 验收证据 |
|---|---|---|---|
| US `igit-replicationd` 真实部署 | US 数据面已 staged 部署并完成 HTTPS 授权、Pin、SHA-256、JTI 重放、瞬态失败重试和监控验收；reaper 离线回归与主网配置 fail-closed fixture 已通过；主网合约/身份签发接入、TTL reaper 和正式迁移仍待执行 | Infra | [acceptance-evidence.md](acceptance-evidence.md)；仍需生产身份 token、成功 push、pin/hash 失败、TTL 回收日志和 Prometheus 指标 |
| 无本地 Kubo 的 Clone/Fetch 回退 | 隔离本地 `127.0.0.1:1` Kubo 的真实 testnet Clone 已通过（4.69s，US 优先）；仓库内 HK 故障/CID miss fixture 已通过；生产网关演练仍待执行 | CLI/Infra | [acceptance-evidence.md](acceptance-evidence.md)；关闭本地 Kubo，分别模拟 HK 故障和 CID miss，成功 checkout；补充真实 HK/CID 日志 |
| 中国大陆网络验收 | OrangeHome 家宽侧 testnet clone 已有部分证据；移动网络、重复采样和完整 checklist 仍未完成 | Infra/QA | [acceptance-evidence.md](acceptance-evidence.md)；纯家宽或移动网络 clone 无本地节点仓库，60 秒内完成；记录端点、时间和 CID |
| Injective 主网端点复测 | 腾讯上海、火山云主网 RPC 已返回 200；纯大陆移动网络和生产备用反代仍待执行 | Infra/QA | [acceptance-evidence.md](acceptance-evidence.md)；LCD/RPC 连接日志和备用反代方案 |
| 发布物链上登记 | 合约 v0.5.0、CLI `release register/verify`、登记脚本和测试已完成；首个生产发布登记待执行 | Release/Admin | `ReleaseArtifacts` 查询结果与 `checksums.txt` 一致，`igit release verify` 通过 |

## P1：主网上线安全与运营

| 工作项 | 当前状态 | 负责人角色 | 验收证据 |
|---|---|---|---|
| 内容委员会多签 | 合约支持独立 committee 地址；需部署并配置真实 3/5 多签 | Governance | 多签成员/阈值记录、委员会裁决交易 |
| 技术 admin 多签 + 14 天升级锁 | 合约已实现 `schedule_upgrade`/`cancel_upgrade`、精确 Wasm SHA-256 绑定和 14 天链上 timelock；主网 admin 多签配置与真实公告/执行交易仍待部署验收 | Contract/Governance | 主网多签地址、`upgrade_security` 查询、升级公告、14 天后执行记录 |
| 复制服务生产配额 | US staged 配置为单次 2 GiB、每用户/仓库 12 请求/分钟和 4 GiB 字节窗口；真实 201→429 配额探测已通过；容量告警和正式容量基线仍待观察 | Infra | [acceptance-evidence.md](acceptance-evidence.md)、配置文件、超限 429、监控告警 |
| Fee grant 代付 | [feegrant-policy.md](feegrant-policy.md) 与状态 gate、带锁的 `injectived` 发放/撤销/成功 `update_ref` 记账包装器及仓库回归 fixture 已实现（首 3 次、0.03 INJ、7 天 TTL、身份唯一性/冷却/日限额）；生产身份服务和真实 push 免 gas 仍待接入 | Platform/Infra | 策略 gate 测试、N 值、资格规则、反女巫策略、真实 grant/revoke/update_ref 和 gasless Push 记录 |
| 用户名策略 | 注册费、保留字、treasury 转账已实现；主网费率和保留字表待配置 | Governance | `Config` 查询、注册费入账和拒绝保留字的交易 |

### 治理配置自动门禁

代码级主网治理预检已加入 `scripts/mainnet-governance-check.sh`，并由
`scripts/mainnet-governance-check-test.sh` 和 CI fixture 覆盖。它只验证
主网节点/合约配置和可选部署 Wasm hash 符合安全门槛，不能替代真实多签部署、用户名策略交易、
升级公告或 14 天后的执行交易；这些运行证据仍保留在 P0/P1 待办中。

## P2：二期与长期能力

优先级按依赖和产品价值排序；P2-A 先于 P2-B，P2-B 先于 P2-C，P2-D
属于长期基础设施，不阻塞 MVP 发布。

| 优先级 | 能力 | 负责人角色 | 依赖/排序 | 下一步验收 |
|---|---|---|---|---|
| P2-A | 私有仓库与协作者密钥轮换 | Contract/Crypto | 加密格式先于成员权限 | 加密 pack、成员增删、历史密钥轮换测试 |
| P2-A | 链下 indexer、全网列表、热度和代码搜索 | Web/Indexer | 所有 Web 搜索能力的基础 | 可自部署 indexer 从链上事件重建索引 |
| P2-B | PR/Issue 混合协作流 | Contract/Web | 依赖 indexer 与 IPFS 正文 | 元数据上链、正文 IPFS、状态流转和 Badge 关联 |
| P2-B | Commit 作者身份绑定 | CLI/Supply-chain | 可与 indexer 并行 | 签名 commit、验证命令和失败用例 |
| P2-C | GitHub 双向镜像 | CLI/Infra | 依赖稳定 push/retry API | push 同步两端、冲突和断线重试 |
| P2-C | Repack、浅克隆、Git LFS | Git/Storage | 先定义 pack/对象兼容边界 | 50 CID/500MB 阈值 repack、`--depth` 和 LFS 指针 E2E |
| P2-C | Owner 节点 multiaddr 上链 | Contract/CLI | 依赖私有仓库访问控制 | 按 `owner/repo` 直连私有节点并回退 HK |
| P2-C | 版权补贴服务 | Platform/Legal | 依赖 indexer、AI 评估和实名流程 | 中国软著服务商、AI 评估指标、反刷分和实名流程 |
| P2-C | 经济模型与存储成本分摊 | Governance/Economics | 先于代币或收费实现 | INJ 收费/补贴、pin 成本归属和合规决策记录 |
| P2-D | 第二托管 pinning 供应商（4EVERLAND 等） | Storage/Infra | 自建 US/HK 持久化稳定后 | 双写或故障切换演练、CID 一致性和凭据轮换 |
| P2-D | Filecoin、ipfs-cluster、社区激励网络 | Storage/Economics | 依赖成本模型和多节点规模 | 多节点持久化、成本模型和代币治理决策 |

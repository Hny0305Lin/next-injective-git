# Remaining Work

This file is the granular task inventory. Phase ordering, dependency gates,
current milestone status, and shared exit criteria are maintained in the
[Delivery Roadmap](delivery-roadmap.md). For a high-level status overview, see
[Project Status](project-status.md).

**Last Updated:** 2026-08-21

The immutable Suite source path is implemented, but no checked-in profile may
claim a live deployment until all real evidence exists.

| Work | Required evidence |
|---|---|
| Testnet deployment | No-clobber manifest, nine successful receipts, runtime/template hashes, constructor arguments, and Blockscout results |
| Historical import | Complete fixed-height inventory, snapshot/hash, deterministic plan/calldata, signed journal, receipts, and module count/root parity |
| Activation | Fixed-block active Directory, version/chain checks, seven module code hashes/bindings, and imported-state comparison |
| Git acceptance | Clean native Windows without WSL2 or injectived, and clean Linux without injectived: init/push/clone/fetch/pull/delete plus historical alias resolution |
| Web acceptance | MetaMask receipts for supported writes with explicit legacy transaction parameters |
| Storage portability | Successor URI/digest contract and Linux/Windows E2E for Amazon S3 and Cloudflare R2 without Kubo |
| Isolated ZKP prototype | Approved statement/public inputs, pinned circuit/setup artifacts, native Windows proof generation, testnet valid/invalid/replay receipts, and gas/prover benchmarks; no Suite integration |
| Security | Deep source review, operator-runner review, resolved findings, and hash-bound approval |
| Production governance | New multisig/timelock design and deployment; the temporary testnet single EOA is not production-ready |

The public SuiteDirectory fields remain empty until the cutover gate passes.

## Engineering TODO

### P0 (完成)
- [x] Record and retain a passing, commit-bound immutable Suite CI run. The
  reviewed P0 commit `f6dcee9aa67255bfdff1867785435022df7ec5e9` is bound to CI
  run `32215415044`; the complete ledger is in [P0 Evidence
  Record](p0-evidence.md). This closes the P0 source/CI baseline only.
- [x] Record and retain a passing native Windows CI run for Go/Kubo tests and
  the PowerShell cutover fixture. The reviewed P0 Windows job passed these
  gates; clean release-asset Git E2E remains a separate P2 requirement.

### P1 (当前阻塞器 - 优先级 1)
- [ ] **P1.1 运营运行器**（阻塞部署）
  - [ ] 实现带加密密钥存储事务处理器的操作运行器
  - [ ] 实现仅追加签名日志（journal）
  - [ ] 实现安全恢复机制以处理不确定的收据
  - [ ] 实现固定区块导入状态证据发射
  - [ ] 在保护环境中测试日志和恢复逻辑
  
- [ ] **P1.2 部署执行**（阻塞切换）
  - [ ] 轮换并资助测试网操作员密钥
  - [ ] 修复 V1 切换高度
  - [ ] 生成完整清单、快照、SHA-256 侧车和用户名托管释放证据
  - [ ] 部署全部 9 个合约（使用 no-clobber 证据）
  - [ ] 在 Blockscout 上验证所有合约
  - [ ] 收集所有 9 个部署收据
  
- [ ] **P1.3 导入和激活**（阻塞验证）
  - [ ] 按顺序导入每个有界批次
  - [ ] 验证计数和滚动承诺
  - [ ] 原子激活 Directory
  - [ ] 在一个最终区块比较每个迁移的域
  - [ ] 记录最终 Directory 状态、代码哈希和模块绑定
  
- [ ] **P1.4 E2E 和钱包测试**（阻塞切换）
  - [ ] 执行 MetaMask 写入并记录收据
  - [ ] 在干净的 Linux 环境中执行 Git E2E（init/push/clone/fetch/pull/delete）
  - [ ] 在干净的 Windows 环境中执行 Git E2E
  - [ ] 测试历史别名解析
  - [ ] 测试不确定收据处理
  
- [ ] **P1.5 安全和批准**（阻塞切换）
  - [ ] 启动并完成深度源代码安全审查
  - [ ] 解决所有安全发现
  - [ ] 获得独立的哈希绑定切换批准
  - [ ] 生成 `security-review.pdf`
  - [ ] 生成 `cutover-approval.txt`
  
- [ ] **P1.6 证据收集**（阻塞配置文件更新）
  - [ ] 收集完整的证据目录
  - [ ] 运行 `scripts/migration-cutover-readiness.sh`
  - [ ] 验证所有证据通过门控
  - [ ] 仅在此之后更新测试网 `SuiteDirectory` 配置文件

### P2 (依赖 P1)
- [ ] 构建 Windows 安装程序（选择、重命名、安装、更新 PATH）
- [ ] 创建校验和验证的 Windows 发布资产（igit.exe、git-remote-igit.exe）
- [ ] 实现托管的原生 Kubo 生命周期（start、stop、status、按需重启）
- [ ] 将任何 WSL bootstrap 重命名为明确的旧版操作员路径
- [ ] 从干净的 Windows VM 和仅发布资产完成完整 Git 工作流
- [ ] 记录 WSL2 和 injectived 缺失

### P3-P5 (存储迁移)
- [ ] **P3: 验证的 packstore 边界**
  - [ ] 引入 `cli/internal/packstore` 抽象
  - [ ] 在添加新提供者之前通过它适配 IPFS
  - [ ] 将 pack 生成到原生临时文件并在流式传输时计算 size/SHA-256
  - [ ] 下载到临时文件，验证精确 size 和 SHA-256
  - [ ] 在 Windows 上关闭并重新打开临时文件后运行 `git index-pack`
  - [ ] 保留现有的持久性 saga 和 thin-pack 排序
  - [ ] 添加显式浏览器 pack-size 限制和 Web Crypto 摘要验证
  
- [ ] **P4: S3/R2 后继协议**（需要新 ADR）
  - [ ] 设计并审查后继 PackRef 格式（kind、logicalProfileId、sha256、size）
  - [ ] 决定托管与 BYO 存储桶策略
  - [ ] 实现 AWS SDK Go v2 S3 适配器（条件写入、ChecksumSHA256）
  - [ ] 实现 Cloudflare R2 适配器（Content-MD5、应用程序 SHA-256 元数据）
  - [ ] 实现托管上传流程（预签名 PUT、独立验证）
  - [ ] 单元和提供者合同测试（新上传、重复 412、可重试错误、错误 size/digest）
  - [ ] 保护的实时工作流（真实 S3 和 R2、小/多部分对象、CORS）
  - [ ] 原生 Windows 和 Linux 完整 Git 工作流（无 Kubo、无日志中的密钥）
  
- [ ] **P5: 历史存储迁移**
  - [ ] 在固定最终视图枚举每个历史 pack URI
  - [ ] 构建哈希绑定的 CID → sha256/size/object key 清单
  - [ ] 按摘要去重并独立读取/验证每个目标对象
  - [ ] 导入后继引用，同时保留历史别名和 ref pack 排序
  - [ ] 为审查的回滚窗口保留 IPFS 读取
  - [ ] 实现从最终链清单的孤立和不可达对象 GC
  - [ ] 定义压实/自包含 pack 策略

### Z0 (独立，可在 P0 后进行)
- [ ] 审查产品陈述和公共/私有输入
- [ ] 使用 gnark v0.15.0 和 Groth16 on BN254 实现成员资格证明电路
- [ ] 在原生 Windows 上编译固定电路并生成/验证证明
- [ ] 序列化精确约束系统和证明/验证密钥
- [ ] 从哈希绑定验证密钥确定性导出 Solidity
- [ ] 部署到 Injective 测试网并在 Blockscout 上验证
- [ ] 测试有效、无效、重放、收件人交换、repo 交换、操作交换、链交换、合约交换、协议交换和过期 epoch 证明
- [ ] 记录约束、设置类型、proof/calldata size、Windows 证明时间/内存、验证器 gas、交易哈希、收据

### 其他工程任务
- [ ] 替换或归档根 `scripts/testnet-e2e.sh`（仍然是明确门控的 V1 脚本）
- [ ] 在切换前拆分并审查此迁移提交
- [ ] 更新用户设置文档以反映原生 Windows 路径
- [ ] 创建故障排除指南
- [ ] 记录运营手册

## Testnet Cutover TODO

- Rotate and fund the testnet EOA, fix the V1 cutover height, and generate the
  complete inventory, snapshot, SHA-256 sidecar, and username escrow-release
  evidence.
- Deploy and verify all nine contracts, import every ordered batch, activate
  the Directory, and compare every migrated domain at one finalized block.
- Record Blockscout verification, MetaMask receipts, Linux/Windows Git E2E,
  finality handling, and an independent hash-bound cutover approval.
- Only after the evidence gate passes, set the single SuiteDirectory address in
  the CLI and Web testnet profiles.

The initial EVM V2 cutover may use the current IPFS adapter. S3/R2 are a
separate successor-protocol milestone and must not be implied by that release.

## Policy TODO

Deep security review is currently skipped by operator decision. The cutover
gate still requires `security-review.pdf`; therefore cutover remains blocked
until that evidence is supplied or an explicit reviewed policy change removes
the requirement from both Linux and PowerShell gates. Do not satisfy the gate
with placeholder evidence.

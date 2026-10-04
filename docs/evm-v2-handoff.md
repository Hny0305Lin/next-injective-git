# iGit EVM V2 接续交接

> 本文件让下一位工程师从当前仓库状态直接继续工作。它是执行清单，不替代 ADR、链上证据或独立安全审查。

**快照日期：** 2026-09-11（LiveAgent 复核）
**当前 HEAD：** `0ba06f436558f12d97625b393767440cdd0f9862`
**分支：** `dev`
**本轮接手时工作树：** 不干净；`docs/README.md`、两份 project-status 已修改，本文件与项目信息库已为未跟踪草稿。本轮保留原稿并增量修正，不把既有改动冒认成本轮新增。
**当前目标：** 完成 P1.4-P1.6 产品验收和受批准的 testnet cutover

## 1. 当前状态

已完成：

- 九个不可升级 EVM Suite 合约已部署到 Injective Testnet（EVM chain ID `1439`）。
- Directory、Coordinator、七个模块的地址、runtime code hash、配置交易和部署 receipt 已记录。
- `fresh-empty-suite` 已在固定区块激活；七个模块 expected/imported records 都为零，空 root 匹配。
- V1 不导入 Suite，只通过显式 archive preview 提供历史只读访问。
- Go CLI、remote helper、EVM registry/transactor、Web viem transport、IPFS gateway/replication 和 operator runner 已存在。

仍未完成：

- checked-in CLI/Web profiles 的 Directory 仍为空，不能宣称公开测试网产品已经 cutover。
- Linux/Windows clean Git E2E、Web receipt/finality、独立安全审查、hash-bound approval 和最终 checksum gate 尚未完成。
- 当前 Suite 只接受 `ipfs://`，不能宣称支持 S3/R2。

Operator runner 的广播、receipt、resume 和 keystore 路径已有实现和测试，但 `cli/cmd/igit-suite-operator/journal.go` 重载 JSONL 时的签名校验仍是 TODO。当前 fresh-empty cutover 不使用迁移 runner；未来执行真实迁移前必须先补齐这一安全边界。

## 2. 关键证据

| 证据 | 路径 | 当前绑定 |
|---|---|---|
| 切换 scope | `evidence/testnet-deployment-2026-08-25/cutover-scope.json` | `fresh-empty-suite`，源码 `4fd6a07...` |
| 激活验证 | `.../suite-verification.json` | 固定区块 `0x83f6bc1`，active，version 3 |
| 合约地址/部署 | `.../contract-addresses.json`、`.../deployment.json` | 测试网九合约 |
| Blockscout | `.../blockscout-verification.json` | 9/9 历史验证记录 |
| Username escrow | `.../empty-username-escrow-attestation.json` | V1 migration=false，liability none |
| 进度说明 | `.../PROGRESS-SUMMARY.md` | P1.2/P1.3 complete，P1.4-P1.6 open |
| 最终门禁 | `scripts/migration-cutover-readiness.sh`、`.ps1` | 要求完整 hash-bound evidence |

现有证据绑定 `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`，当前 HEAD 是 `0ba06f436558f12d97625b393767440cdd0f9862`。二者不能在最终 approval/gate 中混用。

## 3. 接手后的第一轮检查

```powershell
git status --short --branch
git rev-parse HEAD
git log -5 --oneline --decorate

Push-Location cli
go vet ./...
go test ./...
Pop-Location

Push-Location web
npm run test:api
npm run typecheck
npm run build
Pop-Location

Push-Location contracts/evm-v2
npm run check
Pop-Location

& .\scripts\migration-cutover-readiness-test.ps1
```

2026-09-11 的本机基线：Go `vet/test`、Web API 73 项、TypeScript、生产构建、Web release profile、locked-solc/ABI/artifact、Suite source-only、identity readiness 和 PowerShell 门禁 fixture 均通过。真实 cutover gate 按预期因当前证据目录缺少 `cutover-evidence.sha256` 而失败；这不是 cutover 通过证据。

## 4. P1.4：真实产品 E2E 和 finality

P1.4 要收集真实、可审计的产品证据：

1. 先选定 reviewed source commit。生产代码变化后，旧 `4fd6a07...` 证据不能自动覆盖新 commit。
2. 在干净 Linux 环境使用发布资产完成 `init -> push -> clone -> fetch -> pull -> ref delete`。
3. 在干净 Windows 环境重复工作流，确认不依赖 WSL2、`injectived` 或开发目录文件。
4. 用 MetaMask 或受支持钱包执行真实 Web 写入，保留 tx hash、receipt status、区块号、block hash 和 explorer 记录。
5. 覆盖 receipt pending、RPC internal error、广播成功但 receipt 不确定、status `0x0` 和 finality/reorg 边界。
6. 验证 `/archive/cosmwasm-v1` 仍为 GET-only，普通 EVM verification 失败时不会转入 V1。
7. 覆盖 ownership/alias、force push、空 pack 新 ref、moderation policy 和 wallet wrong-chain 场景。

当前 required evidence 状态（按 `scripts/migration-cutover-readiness.sh` 复核）：

已存在 6 项：`cutover-scope.json`、`deployment.json`、`suite-verification.json`、`blockscout-verification.json`、`empty-username-escrow-attestation.json` 和 scope 中声明的 `empty-suite-activation-2026-08-29.log`。存在不等于已经通过 hash、语义、独立审查或 finality 门禁。

仍缺 10 项：

- `foundry-test.txt`
- `foundry-invariant.txt`
- `foundry-gas.txt`
- `windows-clean-e2e.txt`
- `linux-clean-e2e.txt`
- `web-receipt-e2e.txt`
- `security-review.pdf`
- `finality-runbook.md`
- `cutover-approval.txt`
- `cutover-evidence.sha256`

Foundry、双平台 E2E、Web receipt/finality、安全审查、独立 approval 和 checksum 必须绑定同一个 reviewed commit；不能用 fixture 或本机源码检查补齐。

## 5. P1.5：独立安全审查

审查至少覆盖：Directory-first verification、code hash/binding、bootstrap 状态机和输入边界、ownership/recovery、Moderation policy、Economic settlement/reentrancy、Username escrow、durable Pin/ref 顺序、Go/Web nonce/gas/receipt、V1 只读隔离、keystore 权限和 evidence gate。

要求产物：

- `security-review.pdf`；
- finding 修复 commit 和复核记录；
- `cutover-approval.txt`，包含 `decision=approved`、reviewer、`reviewed_commit` 和 `reviewed_at`。

## 6. P1.6：最终切换

1. 把全部证据放入不可变 evidence directory。
2. 生成 `cutover-evidence.sha256`，每个 required file 有唯一 checksum，路径不逃逸目录且不包含 symlink。
3. 用与 approval 一致的 40-hex commit 执行：

   ```bash
   bash scripts/migration-cutover-readiness.sh EVIDENCE_DIR EXPECTED_COMMIT
   ```

   Windows 使用等价 PowerShell gate。

4. 核对 `validate-suite-cutover.mjs`、fresh-empty scope 和零导入状态。
5. Gate 通过后才更新：
   - `cli/internal/config/config.go` 的 testnet `EVMSuiteDirectory`；
   - `web/src/lib/profile.ts` 的 testnet `suiteDirectory`；
   - release/profile checks 和用户文档。
6. 更新后重跑 profile guards、全部测试、clean checkout 和 release asset 验证。

Suite 不可升级，也不能通过旧 V1 backend 做隐式回滚。切换后应另行保留 incident/finality runbook。

## 7. 当前验证环境限制

当前 Windows 主机有 Node `v24.11.1`、npm `11.6.2`、Go `1.26.5`，没有 `forge`。PowerShell 默认解析的 `bash.exe` 会启动 WSL，而 WSL 中找不到 Node；显式调用 Git Bash 时可以运行 locked-solc 检查，但仍会在 Foundry 阶段失败。因此本机不能提供 Foundry unit/invariant/gas 的 required 证据。

- `npm run check` 在 PowerShell 中通过；
- PowerShell 默认 `bash scripts/evm-v2-check.sh --required` 当前失败于 WSL `node not found`；
- 显式 Git Bash 的 `bash scripts/evm-v2-check.sh --required` 已通过 solc/ABI 阶段，但失败于 `forge not found`；
- `bash scripts/suite-readiness.sh --required` 仍需要完整 Node/Foundry toolchain；
- Foundry test/invariant/gas 未在本机验证。

Required gate 不能降级为 skip。应在 CI/Linux 或配置完整 Node/Foundry 的 WSL 中重跑。

## 8. 硬停止条件

出现以下需求时先重新审查设计：

- 增加第二个配置 contract address 或绕过 SuiteDirectory；
- EVM error 后调用 V1/CosmWasm backend；
- 让 `ipfs://` 之外的 URI 直接进入当前 Suite；
- receipt 不确定时自动换 nonce 重发；
- 覆盖或改写已有 `evidence/` 文件；
- 把 fixture、local deployment 或 explorer 截图当 final approval；
- 为满足测试临时广播真实交易；
- 没有 canary/receipt evidence 就改变 legacy tx/gas 规则；
- 把 pending transfer/recovery 直接写入迁移 snapshot；
- 用手写 selector/ABI 解码绕过 checked ABI。

## 9. 后续路线

| 阶段 | 目标 | 前置条件 |
|---|---|---|
| P1 | testnet product cutover | P1.4-P1.6 全部通过 |
| P2 | 原生 Windows/Linux 用户验收和发布安装资产 | P1 完成 |
| P3 | packstore boundary 和流式完整性校验 | 可与 P1 并行，不改变当前 IPFS 行为 |
| P4 | S3/R2 successor protocol 和 adapter | 新 ADR、Suite 版本和凭据/完整性设计 |
| P5 | 历史 pack storage 迁移 | P4、CID mapping、双读回滚和 provider E2E |
| M0 | Mainnet candidate | P1/P2、治理、finality、独立批准及条件性 P5 |

## 10. 交接记录

| 日期 | 记录 |
|---|---|
| 2026-09-11 | LiveAgent 复核完整 HEAD、客户端链路、required evidence 缺件和本机门禁；确认源码检查通过但 P1.4-P1.6 仍未完成。 |

下次更新至少补充：新的 reviewed commit、真实 E2E tx/block evidence、安全审查状态、finality 结果和 cutover gate 输出。

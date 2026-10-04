# LiveAgent 接续上下文：iGit EVM V2

> 本页是 LiveAgent 新客户端接手本项目时的最短上下文入口。它只记录当前源码、已签入证据和本机验证结果；不替代 ADR、链上证据、安全审查或最终 cutover gate。

**复核日期：** 2026-09-11  
**分支：** `dev`  
**当前源码 HEAD：** `0ba06f436558f12d97625b393767440cdd0f9862`  
**测试网证据绑定提交：** `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`  
**当前结论：** P1.1、P1.2、P1.3 已有实现或历史证据；P1.4、P1.5、P1.6 未完成，公开 SuiteDirectory 必须继续为空。

## 先读什么

1. [`project-knowledge-base-zh.md`](project-knowledge-base-zh.md)：架构、代码地图、数据流和安全不变量。
2. [`evm-v2-handoff.md`](evm-v2-handoff.md)：P1.4-P1.6 的执行清单、证据要求和硬停止条件。
3. [`architecture.md`](architecture.md) 与 [`adr/`](adr)：已接受的边界和设计决策。
4. [`evidence/testnet-deployment-2026-08-25/PROGRESS-SUMMARY.md`](../evidence/testnet-deployment-2026-08-25/PROGRESS-SUMMARY.md)：测试网部署和 fresh-empty 激活的历史状态。
5. 当前改代码前，先看 [`CLAUDE.md`](../CLAUDE.md) 和对应组件测试。

## 项目模型

- EVM V2 是产品代际名称；链上 Suite 协议版本是 `3`。
- `SuiteDirectory` 是普通 CLI、Git remote helper 和 Web EVM 路径的唯一链上信任根。模块地址由 Directory 发现，不能从第二份配置注入。
- Suite 不可升级：没有 proxy、diamond、`delegatecall`、CosmWasm 写回退或混合 backend。
- Git 控制面在 Injective EVM；Git packfile 当前通过 IPFS/Kubo 存储、复制和 HTTPS gateway 读取。
- V1 CosmWasm 只保留独立的固定高度、GET-only archive preview，不是 EVM verification 失败时的 fallback。
- 当前 Suite 只接受 `ipfs://` pack URI；S3/R2 需要新的存储协议、Suite 版本、完整性和迁移设计。

## 当前部署状态

测试网证据目录 `evidence/testnet-deployment-2026-08-25/` 已记录九个合约部署、八个配置调用、运行时代码/绑定复核、9/9 Blockscout 历史验证和 `fresh-empty-suite` 激活。fresh-empty 只表示本次七个业务模块的导入记录为零，并不表示全系统、V1 archive 或未来 Suite 状态为空。

证据 commit 是 `4fd6a07ad2f45eb4b0b09b58eeed1621ddc8f986`，当前源码 HEAD 是 `0ba06f436558f12d97625b393767440cdd0f9862`，二者不同。任何最终 approval、checksum manifest 和 cutover gate 必须绑定同一个、经过审查的 40-hex commit；不能把当前 HEAD 的源码检查冒充旧部署证据的验收。

因此，以下配置不得修改，直到 P1.4-P1.6 全部完成并通过门禁：

- `cli/internal/config/config.go` 的测试网 `EVMSuiteDirectory`；
- `web/src/lib/profile.ts` 的测试网 `suiteDirectory`；
- 发布 profile 和面向用户的公开可用性声明。

## 已核对的本机基线

2026-09-11 在当前工作区执行：

| 检查 | 结果 |
|---|---|
| `(cd cli && go vet ./... && go test ./...)` | 通过 |
| `(cd web && npm run test:api)` | 通过，73 项 |
| `(cd web && npm run typecheck && npm run build)` | 通过；Vite 仅报告 chunk size/PURE 注释 warning |
| `(cd web && npm run check:release-profile)` | 通过 |
| `(cd contracts/evm-v2 && npm run check)` | 通过；locked `solc 0.8.24`、ABI/artifact gate 通过 |
| `bash scripts/suite-readiness.sh --source-only` | 通过 |
| `node scripts/identity-readiness.mjs` | 通过 |
| `scripts/migration-cutover-readiness-test.ps1` | 通过 fixture 回归 |
| `bash scripts/migration-cutover-readiness.sh ...` | 按预期失败：当前证据目录缺 `cutover-evidence.sha256` |
| Foundry required gate | 当前 Windows 主机未完成：PATH 中无 `forge` |

这些结果证明源码边界和本机可执行检查，不证明真实双平台产品 E2E、钱包收据、交易 finality、独立安全审查或公开切换。

## 下一步只做这些

1. 选择一个经过审查的 reviewed commit，并让所有 source-sensitive evidence、approval 和 `expected_commit` 精确一致。
2. 在干净 Linux 和 Windows 环境生成真实 Git E2E 证据；覆盖 `init -> push -> clone -> fetch -> pull -> ref delete`、force push、空 pack 新 ref、alias/ownership、moderation 和失败重试。
3. 使用受支持钱包生成 Web receipt/finality 证据，保留 tx hash、receipt status、block number/hash 及不确定 receipt 处理记录。
4. 补齐 Foundry unit/invariant/gas 证据和独立安全审查，解决 finding 后生成哈希绑定 approval。
5. 生成 `cutover-evidence.sha256`，在同一 reviewed commit 上运行 shell/PowerShell cutover gate。
6. 只有 gate、approval 和 checksum 全部通过后，才填入 CLI/Web 默认 SuiteDirectory。

## 不要做

- 不要把 evidence 中的 Directory 地址直接写入默认 profile。
- 不要增加第二个 module address、V1 write fallback、proxy、diamond 或 `delegatecall`。
- 不要把 `VerifySuite` 描述为源码、审计、finality 或当前 HEAD 的证明；它只验证固定区块的 Suite 内部一致性。
- 不要把 Solidity runtime `codehash` 和文件 `sha256` 混称。
- 不要在 uncertain receipt 时换 nonce 自动重发。
- 不要覆盖 `evidence/` 中已有文件，也不要用 fixture、截图或本地探测代替 live evidence。
- 不要在 operator 的 `journal.go` 仍保留重载验签 TODO 时，把它用于有价值的真实迁移。

## 代码定位

| 目标 | 入口 |
|---|---|
| CLI 分发和配置 | `cli/cmd/igit/main.go`、`cli/internal/config/config.go` |
| Suite 验证和模块解析 | `cli/internal/chain/suite_directory.go`、`evm_suite_registry.go` |
| 交易 nonce/gas/receipt | `cli/internal/chain/evm_transactor.go` |
| Git push/fetch | `cli/internal/remote`、`cli/internal/ipfs`、`cli/internal/replication` |
| Web profile、registry、wallet、交易 | `web/src/lib/profile.ts`、`registry.ts`、`WalletContext.tsx`、`transport.ts` |
| Web pack 读取 | `web/src/lib/gitstore.ts` |
| Suite 合约 | `contracts/evm-v2/src` |
| source/evidence gate | `scripts/suite-readiness.sh`、`scripts/evm-v2-check.sh`、`scripts/migration-cutover-readiness.sh` |

## 权威性

事实冲突时按此顺序判断：当前源码和测试输出 > 与精确提交绑定的 `evidence/` > 已接受 ADR/架构文档 > roadmap、backlog、示例和历史 repair plan。文档改动要同时写明复核日期、源码 commit、验证命令和仍未完成的证据。

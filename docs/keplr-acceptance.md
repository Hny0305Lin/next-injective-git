# Keplr 网页赞助 — 手动验收指南

> Web 已接入 Keplr：可在浏览器里连接钱包、直接发赞助交易（合约即时拆分）。
> 自动化测试装不了钱包扩展，这一步需要人工完成。

## 技术选型说明
- 用**经过审计的 CosmJS**（`@cosmjs/cosmwasm-stargate`）+ Keplr 原生 signer，**不用** `@injectivelabs/sdk-ts`（该包 2026-07 曾被供应链投毒 v1.20.21 窃取私钥）。
- Injective 账户是 `ethsecp256k1`，Keplr 内建支持 injective-888，自动处理公钥类型。
- 查询走 LCD（REST），交易走 RPC（`k8s.testnet.tm.injective.network`）。

## 准备（约 3 分钟）
1. 浏览器安装 [Keplr 扩展](https://www.keplr.app/)。
2. 在 Keplr 里**新建一个账户**（不要用日常钱包；助记词妥善保存）。
3. 首次连接时若 Keplr 没有 injective-888，页面会自动请求添加该链 —— 在弹窗点 Approve。
4. 复制该账户的 `inj1…` 地址，去 [testnet faucet](https://testnet.faucet.injective.network/) 领测试 INJ（付 gas + 赞助金额）。

## 验收步骤
1. `cd web && npm run dev`，打开 http://localhost:5173 。
2. 顶栏点 **Connect Keplr** → Keplr 弹窗点 Approve。
   - ✅ 按钮变成 `<余额> INJ · inj1xxx…xxxx`（绿色等宽字体）。
3. 打开任意**不是你自己**的仓库的 sponsors 页，例如：
   `#/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase/sponsors`
   - 顶部绿框卡片出现金额输入框（默认 0.1）+ 留言框 + **Sponsor** 按钮。
   - （若连的是仓库 owner 本人，会显示"这是你自己的仓库"，属正常保护。）
4. 输入金额（如 `0.05`）、留言（如 `great work`），点 **Sponsor**。
5. Keplr 弹出签名窗 → 确认。
   - ✅ 出现 `✅ sponsored! tx <hash>… ` 提示。
   - ✅ 顶栏余额下降（赞助额 + gas）。
6. 刷新页面 → sponsor wall 出现你这条记录（金额 / 你的地址 / 留言）；Lifetime sponsorship 累加。
7. 去浏览器区块浏览器核对该 tx：一笔交易内含合约执行 + 3 笔 BankSend（金库 3% / 分成对象 / owner 余额）。

## 常见问题
- **"Keplr extension not found"**：扩展没装或没启用。
- **连接卡住/无弹窗**：确认 Keplr 已解锁；刷新页面重试。
- **签名报 insufficient funds**：faucet 领的 INJ 不够付 gas，再领一次。
- **RPC 连不上（大陆网络）**：Settings 目前只配了 LCD；RPC 端点硬编码在 `web/src/lib/wallet.ts` 的 `RPC` 常量，可改为自建节点（见 docs/pinning-infrastructure.md §6）。

## 反馈给我
把第 5 步的 tx hash（或任何报错文案）发我，我据此确认闭环或修问题。

---

# MetaMask 网页赞助 — 手动验收（EIP-712）

> MetaMask 走的是完全不同的路径（EIP-712，不是 Cosmos 签名），**这部分我无法自动验证**，
> 只能靠你手动测。用 `@injectivelabs/sdk-ts@1.20.27`（投毒事件后的干净版；恶意版 1.20.21 已被 npm 弃用）
> 生成 EIP-712 类型数据，直连 `window.ethereum` 的 `eth_signTypedData_v4` 签名。SDK 通过动态 import
> 拆成独立 chunk，不影响已验证的 Cosmos 钱包路径。

## 准备
1. 浏览器装 [MetaMask](https://metamask.io/)，新建/选一个账户。
2. 该账户对应的 Injective 地址：MetaMask 是 `0x...` 以太坊地址，Injective 会推导出对应的 `inj1...`。
   页面点 “Sponsor with MetaMask” 时会自动推导；你也可以先转点测试 INJ 到那个 inj 地址（付 gas）。
   - 拿 inj 地址：连接后控制台会显示，或用别的工具把 `0x` 转 bech32；实在不确定就先跑一次，报错会带出地址，再让我转测试币。

## 验收步骤
1. 打开某个**不是你自己**的仓库 sponsors 页，如 `#/inj1sh4v.../demo-showcase/sponsors`。
2. 绿框卡片里除了 Cosmos 的 Sponsor 按钮，应多一个 **“Sponsor with MetaMask”**（仅当检测到 `window.ethereum`）。
3. 填金额（如 `0.05`）+ 留言，点 **Sponsor with MetaMask**。
4. MetaMask 弹出 **签名请求（Signature request，EIP-712 类型数据）**，确认签名。
5. 期望：出现 `✅ sponsored! tx …`；刷新后赞助墙出现该笔。

## 最可能的坑（若失败，按顺序排查）
- **签名验证失败 / account not found**：该 inj 地址还没在链上激活（没余额）。先转点测试 INJ 过去再试。
- **evmChainId 不对**：代码里用的是 `EvmChainId.Injective`(888) 对应 injective-888。若链上报 EIP-712 domain 不匹配，把 `web/src/lib/metamask.ts` 里两处 `EvmChainId.Injective` 调整后重试——这是最可能需要微调的点。
- **broadcast 报错**：把 MetaMask 弹窗里的 EIP-712 内容 + 页面报错原文发我，我据此定位。

## 反馈给我
把 tx hash（成功）或完整报错文案（失败）发我。因为这条路径我没法自己验证，你的这次实测就是唯一的正确性判据。

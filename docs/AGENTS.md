# AGENTS.md — docs/ 目录级说明（Agent 必读）

本目录是 next-injective-git 文档站（`docs-site/`，Docusaurus 3）的**内容事实源**。
站点通过 docs 插件直接挂载 `../docs`，不复制、不移动本目录文件。

## 硬性红线（违反即返工）

1. **不可变证据不改不造**：`adr/`、部署工件、plans、manifests、journals、
   receipts、审批记录一律不改动、不重写、不制造。ADR 文件保持零改动——
   不注入 frontmatter，标题与分组由 `docs-site/sidebars.js` 解决。
2. **术语**：翻译/写作前必须查 [glossary.md](glossary.md)；表里没有的先补表。
   Suite v3 = IPFS/Kubo 旧路径；Suite v4 = BYOS 后继，**生产配置仅 Amazon S3 与
   Cloudflare R2**。
3. **六家中国大陆厂商只能以 roadmap 口径出现**（见
   [roadmap-byos-providers.md](roadmap-byos-providers.md)）：任何语言不得写
   "已支持/已接入/supported"；不得在任何配置示例、CLI 参数、生产指引中出现
   这些厂商的 endpoint。实现与审批落地后才能改口径。
4. **`archive/cosmwasm-v1`** 绝不进入站点导航、构建依赖或运行时路径。
5. **`SuiteDirectory` 保持为空**（已发布 profile），不得把测试地址写成正式地址。
6. **中文不得超前英文**：open items（真实 AWS canary、Foundry gates、后继发布
   证据、安全审计、主网治理审批）在两种语言里都必须保持"未完成"表述。
7. 本目录现有文件**不移动、不重命名、不改写内容**；允许**新增**文件。

## 新增/修改一个文档页的标准流程（双语）

1. 英文页放本目录（en 是唯一事实源），并在 `docs-site/sidebars.js` 登记分类。
2. 运行 `npm --prefix docs-site run gen:zh-stubs`（新文件若未翻译会生成 zh 占位页，
   显式标注"翻译进行中"）。
3. 完成中文翻译：在 `docs-site/i18n/zh/docusaurus-plugin-content-docs/current/`
   下放同名文件，替换占位页。翻译前先查 [glossary.md](glossary.md)。
4. `npm --prefix docs-site run build` 通过；检查 zh/en 页面一一对应。
5. 提交时包含：英文源、zh 翻译、sidebars.js 变更。

## sidebar 规则

- 顺序/分组/标签全部在 `docs-site/sidebars.js` 声明，**不要**给本目录文件加
  frontmatter（尤其 ADR）。
- 新文件必须登记进 sidebar（显式列表，未登记会告警）。

## 图片资产

- 站点图片一律本地放 `docs-site/static/`（如 `static/img/igit-image.png`），
  禁止外链图床。
- 唯一例外：贡献者头像（`avatars.githubusercontent.com`，构建时缓存于
  `docs-site/src/generated/contributors.json`，由脚本生成，**禁止手改**）。
- 品牌图原图 `D:\inj\igit-image.png` 只读，站点用副本。

## 相关入口

- 站点配置与脚本：`docs-site/`（见根目录 `AGENTS.md` 速查）
- 维护提示词模板：`docs-site/MAINTENANCE-PROMPT.md`
- 部署指引（用户本人操作）：`docs-site/DEPLOYMENT.md`
- 术语表：[glossary.md](glossary.md)

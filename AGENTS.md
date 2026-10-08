# AGENTS.md — 根目录速查（Agent 必读）

**规范唯一来源是 [CLAUDE.md](CLAUDE.md)。本文件只是文档系统操作速查，
不复制 CLAUDE.md 的项目规则，避免两份文档漂移；冲突时以 CLAUDE.md 为准。**

## Documentation System 速查（双仓库结构）

- **主仓库**（本仓库，`next-injective-git`）：文档内容事实源 `docs/` +
  规范文件。ADR/plans/evidence 等不可变证据零改动。
- **文档站仓库** `D:\inj\next-injective-git-docs` =
  https://github.com/Hny0305Lin/next-injective-git-docs （分支 `main`）：
  Docusaurus 3（Node ≥ 24）站点工具链 `docs-site/` + `docs/` **只读镜像**
  （由脚本同步，禁止手改）。主仓库 `.gitignore` 已忽略 `docs-site/`，
  站点工具链不进主仓库。
- 内容改动流程：改主仓库 `docs/`（en 唯一事实源）→ 在文档站仓库
  `docs-site/` 下 `npm run sync:docs` 镜像 → `npm run build` 验证 →
  `git add -A && git commit && git push origin main`（推送目标即
  next-injective-git-docs，触发其 CI 与 Vercel 部署）。
- 双语规则：en 是唯一事实源；zh 在
  `docs-site/i18n/zh/docusaurus-plugin-content-docs/current/` 下同名镜像，
  URL 前缀 `/zh/`。新英文页 → `npm run gen:zh-stubs` 生成占位
  （"翻译进行中"），再补译替换。翻译前必查 `docs/glossary.md`。
- 术语表：`docs/glossary.md`（含六家中国大陆 S3 兼容厂商的 roadmap 表）。
- 品牌图：原图 `D:\inj\igit-image.png`（只读）；站点副本
  `docs-site/static/img/igit-image.png` + `favicon.png`（在文档站仓库）。
  站点图片一律本地 `static/`，唯一外链例外是贡献者头像域
  `avatars.githubusercontent.com`。
- Contributors 缓存：文档站仓库 `docs-site/src/generated/contributors.json`
  **只能由脚本生成，禁止手改**（统计对象是主仓库）。刷新：
  `npm run fetch:contributors`（可选 `GITHUB_TOKEN` 环境变量应对限流，
  token 绝不落仓库）。拉取失败自动回退已提交缓存；无缓存则首页隐藏区块。
- CI：文档站仓库的 `.github/workflows/docs.yml`（路径过滤
  `docs-site/**`、`docs/**`、自身；Node 24；`npm ci` + `npm run build`）。
  不得修改主仓库现有 `ci.yml` / `release.yml`。
- 部署指引：文档站仓库 `docs-site/DEPLOYMENT.md`（Vercel 导入
  **next-injective-git-docs** + Cloudflare DNS，**由用户本人操作**；
  agent 不执行部署、不接触凭证）。
- 定时维护模板：文档站仓库 `docs-site/MAINTENANCE-PROMPT.md`
  （推送目标 = next-injective-git-docs）。
- 目录级规则：`docs/AGENTS.md`（新增页面流程、sidebar、红线复述）。

## 文档相关红线（详见 CLAUDE.md "Documentation System"）

- 六家中国大陆厂商只能以 roadmap 口径出现，禁止"已支持/已接入/supported"
  与任何 endpoint 配置示例。
- ADR/plans/evidence 等不可变证据零改动；`archive/cosmwasm-v1` 不入站点。
- 中文不得超前英文；open items 两语言均保持"未完成"。
- 文档站仓库的 `docs/` 是镜像产物：只能 `sync:docs` 生成，禁止手改。

# igit 前端 Gogs 风格改版建议

## 1. 改版目标

将当前偏展示型的前端改造成以仓库浏览和日常操作为核心的代码托管界面。

改版参考 Gogs 的信息架构、页面密度和交互习惯，但不直接复制 Gogs 的 Go 模板或品牌样式。现有 React、Injective、IPFS 和钱包功能继续保留。

目标体验：

- 用户进入网站后，能快速搜索用户或仓库。
- 进入仓库后，第一屏就能看到分支、最近提交、文件列表和克隆地址。
- 源码、提交、Refs、赞助等功能保持统一的仓库导航结构。
- 链上信息作为仓库能力呈现，不喧宾夺主。
- 桌面端信息紧凑，移动端仍可完成仓库浏览。

## 2. Gogs 参考重点

参考目录：<https://github.com/gogs/gogs/tree/main/web>

主要参考模板：

- `templates/base/head.tmpl`：全局导航。
- `templates/home.tmpl`：首页入口。
- `templates/user/dashboard/dashboard.tmpl`：用户仪表盘。
- `templates/repo/header.tmpl`：仓库标题和仓库级导航。
- `templates/repo/home.tmpl`：分支工具栏、克隆框和文件区域。
- `templates/repo/view_list.tmpl`：文件列表。
- `templates/repo/view_file.tmpl`：文件内容。
- `templates/repo/commits_table.tmpl`：提交列表。

应吸收的设计特点：

- 固定内容宽度，页面居中，减少无效留白。
- 顶部导航简单稳定，搜索是主要入口。
- 仓库名称和仓库操作独立成一层页头。
- 仓库功能使用横向标签栏组织。
- 分支选择、路径面包屑、克隆操作放在同一工具区。
- 文件列表采用紧凑表格，而不是卡片集合。
- README 是文件列表下方的独立内容面板。
- 页面状态通过小标签表达，不使用大面积装饰。

## 3. 保留的 igit 特性

以下内容不能因模仿 Gogs 而丢失：

- Injective 钱包连接和余额。
- Injective 地址与用户名解析。
- 链上仓库、Refs、协作者和治理状态。
- IPFS pack 下载和浏览器内 Git 对象解析。
- 仓库赞助、收入分配、贡献 Badge。
- Injective Explorer 和 IPFS Explorer。
- `igit://owner/repo` 克隆地址。

## 4. 全局页面结构

```text
+------------------------------------------------------------------+
| igit | Dashboard | Explore | IPFS | [ Search repositories... ]   |
|                                             [ Wallet / Sign in ]  |
+------------------------------------------------------------------+
|                                                                  |
|                     页面主体，最大宽度 1120px                     |
|                                                                  |
+------------------------------------------------------------------+
| Injective testnet | Contract | Documentation | Source            |
+------------------------------------------------------------------+
```

### 顶部导航

建议顺序：

1. igit 标志。
2. Dashboard 或首页。
3. Explore。
4. IPFS。
5. 居中的全局搜索。
6. Settings。
7. 钱包按钮。

调整建议：

- 将当前 `on Injective` 胶囊弱化为品牌副标题或环境标记。
- 顶栏高度控制在 52 至 56px。
- 搜索框支持 `owner`、`owner/repo`、`igit://owner/repo` 和 `inj1...`。
- CLI 链接移到底部或帮助菜单，避免占用主要导航位置。
- 钱包已连接时显示短地址和余额，点击打开账户菜单，而不是直接断开。

## 5. 首页方案

当前首页的大标题、命令展示和费率卡片适合产品介绍，但不适合作为代码托管产品的长期首页。建议改为 Gogs Dashboard 风格。

```text
+---------------------------+  +-----------------------------------+
| My repositories           |  | Recent on-chain activity          |
| [ Find a repository... ]  |  |                                   |
| owner/repo-a              |  | alice pushed main to repo-a       |
| owner/repo-b              |  | bob sponsored repo-b              |
| owner/repo-c              |  | carol created repo-c              |
|                           |  |                                   |
| [Explore repositories]    |  | [View all transactions]           |
+---------------------------+  +-----------------------------------+

+------------------------------------------------------------------+
| Network status: injective-888 | Contract | Gateway | Latest block|
+------------------------------------------------------------------+
```

未连接钱包时：

- 左栏显示快速开始和演示仓库。
- 右栏显示近期链上活动或热门仓库。
- 保留三条 CLI 示例，但放入紧凑的“Quick start”面板。

连接钱包后：

- 左栏展示该地址拥有或协作的仓库。
- 右栏展示与该地址相关的链上活动。
- 提供创建仓库、注册用户名和查看个人页入口。

当前测试钱包和合约地址表不应占据首页主体，建议移到 Settings 中的 `Network` 页面。

## 6. 仓库页面方案

仓库页是本次改版重点。

### 6.1 仓库标题区

```text
[repo icon] owner / repository                    [Fork] [Sponsor]
            Repository description
            Forked from source-owner/source-repo
```

规则：

- `owner` 使用普通字重，仓库名使用粗体。
- `active` 状态不必持续显示；只在 `frozen` 或 `delisted` 时显示醒目状态。
- Fork 来源放在标题下方。
- Sponsor 和 Fork 是仓库动作，放到标题右侧。
- 小屏幕下动作按钮换到第二行。

### 6.2 仓库导航

建议标签：

| 标签 | 对应现有功能 |
|---|---|
| Code | TreeView、BlobView |
| Commits | CommitsView、CommitView |
| Branches & Tags | RefsTab |
| Sponsors | SponsorsTab |
| Activity | 过滤后的 Explorer，可作为后续功能 |
| Settings | 仓库信息、协作者、收入分配，可作为后续功能 |

标签使用图标加文字。当前没有 Issues 和 Pull Requests 的数据模型，因此不要制作不可用的空标签。

### 6.3 Code 工具栏

```text
[ main v ]  repository / src / components       [ igit v ][ clone URL ][copy]
```

- 左侧是分支或 Tag 选择器。
- 中间是当前目录面包屑。
- 右侧是 Clone 方式和复制按钮。
- Clone 下拉菜单先提供 `igit`，以后可扩展 HTTPS 或归档下载。
- 使用图标按钮执行复制，悬停时显示说明。

### 6.4 仓库统计

参考 Gogs，在文件工具栏上方或下方增加紧凑统计栏：

```text
12 commits          3 branches          2 tags          8 packfiles
```

数据来源：

- Commits：浏览器加载默认分支后统计，加载前显示占位状态。
- Branches：`refs/heads/*` 数量。
- Tags：`refs/tags/*` 数量。
- Packfiles：当前默认 Ref 的 `pack_uris.length`。

### 6.5 文件列表

建议采用三列结构：

| Name | Last commit | Updated |
|---|---|---|
| `src/` | feat: add repository browser | 2 days ago |
| `README.md` | docs: update quick start | 3 days ago |

第一阶段如果尚不能高效计算每个文件最后一次提交，可先展示：

| Name | Type | Object |
|---|---|---|
| `src/` | Directory | `a6ce0f2` |

样式规则：

- 目录排在文件前面，各自按名称排序。
- 行高约 38 至 42px。
- 文件夹和文件使用统一线性图标，取消 emoji。
- 顶部提交摘要独立成浅色表头。
- 长提交信息截断，完整内容通过 title 或详情页查看。

### 6.6 README

- 作为文件列表下方的独立面板。
- 标题栏显示文件图标和 `README.md`。
- 正文宽度和字号接近 Gogs，不使用营销页的大标题字体。
- 表格、代码块、引用、链接和图片继续使用现有安全 Markdown 渲染。

## 7. 用户页面方案

参考 Gogs 的用户主页，改为左右布局：

```text
+----------------------+  +----------------------------------------+
| Avatar / identicon   |  | Repositories                           |
| @username            |  | [Search repositories...]              |
| inj1abc...xyz        |  |                                        |
| 8 repositories      |  | repo-a                    active       |
| 4 badges             |  | description                updated... |
|                      |  |                                        |
| Injective explorer   |  | repo-b                    fork         |
+----------------------+  +----------------------------------------+
```

- 没有头像系统时，可用地址生成稳定 identicon，或先使用仓库图标占位。
- 地址完整值放在可复制字段中。
- Badge 独立成用户页标签，不与仓库列表混排。
- 仓库列表继续使用列表，不使用每个仓库一个大卡片。

## 8. Explorer 和 IPFS 页面

这两个页面保留工具属性，不强行套用仓库页布局。

建议：

- 使用与 Gogs 管理页面相似的标题、筛选工具栏和表格。
- Explorer 的交易类型用小标签显示。
- IPFS 审计结果使用明确的成功、警告和失败状态。
- 搜索和筛选固定在结果表格上方。
- 移动端将表格切换为定义列表，避免横向内容全部挤压。

## 9. 视觉规范

### 颜色

保留深色主题，但降低当前紫色的占比：

| 用途 | 建议 |
|---|---|
| 页面背景 | 接近黑色的中性灰 |
| 面板背景 | 比页面背景高一个明度层级 |
| 边框 | 中性灰，保持清晰但不过亮 |
| 主链接 | 清晰蓝色 |
| 主操作 | Injective 紫色，仅用于钱包和主要交易 |
| 成功 | 绿色 |
| 警告 | 黄色 |
| 危险 | 红色 |

### 字体

- 全局使用系统无衬线字体。
- 代码、地址、SHA 和 CID 使用等宽字体。
- 取消仓库页和工具页的衬线展示字体。
- 不使用随视口宽度变化的字体尺寸。

### 圆角与阴影

- 按钮和输入框圆角建议 4 至 6px。
- 文件列表和 README 面板圆角不超过 6px。
- 页面区块不做浮动卡片。
- 主要依赖边框分层，不使用明显阴影和装饰光晕。

### 图标

- 引入统一的线性图标库，例如 `lucide-react`。
- 仓库、分支、Tag、文件夹、文件、历史、复制、钱包等使用图标。
- 移除 `📁`、`📄`、`🏆` 等 emoji。
- 纯图标按钮必须有 `title` 或 tooltip，并提供 `aria-label`。

## 10. 响应式规则

### 大于 960px

- 内容最大宽度 1120px。
- 仓库标题和操作按钮同一行。
- Code 工具栏尽量单行。
- 用户页和 Dashboard 使用两栏。

### 600px 至 960px

- 顶部搜索单独占一行。
- 仓库标题动作换行。
- Dashboard 改为单栏，仓库列表优先。

### 小于 600px

- 顶部导航收进菜单，保留品牌、搜索和钱包。
- 仓库标签栏允许横向滚动。
- 文件列表隐藏次要列，只保留名称和对象短 SHA。
- Clone 地址允许截断，复制按钮始终可见。
- 地址、CID 和 SHA 不得撑破容器。

## 11. 现有组件改造映射

| 当前文件 | 建议改造 |
|---|---|
| `web/src/App.tsx` | Gogs 风格全局导航、账户菜单、页面宽度结构 |
| `web/src/pages/Home.tsx` | 从宣传首页改为 Dashboard |
| `web/src/pages/Owner.tsx` | 用户资料侧栏和仓库列表 |
| `web/src/pages/Repo/index.tsx` | 仓库标题、动作区、仓库标签栏和统计 |
| `web/src/pages/Repo/TreeView.tsx` | 分支工具栏、文件表格、提交摘要、README |
| `web/src/pages/Repo/BlobView.tsx` | Gogs 文件标题栏和代码查看器 |
| `web/src/pages/Repo/CommitsView.tsx` | 紧凑提交列表和分页入口 |
| `web/src/pages/Repo/CommitView.tsx` | 提交摘要、父提交、文件变更区 |
| `web/src/pages/Repo/RefsTab.tsx` | 分成 Branches 和 Tags 两组 |
| `web/src/pages/Repo/SponsorsTab.tsx` | 保留链上功能，匹配仓库页视觉规范 |
| `web/src/styles.css` | 重建页面层级、颜色、表格和响应式规则 |

## 12. 建议实施顺序

### 第一阶段：建立页面骨架

- 重做全局导航和内容容器。
- 重做仓库标题区和标签栏。
- 重做 TreeView 文件列表和 README。
- 添加统一图标。
- 完成桌面端和移动端基础布局。

### 第二阶段：统一所有仓库视图

- 改造 Blob、Commits、Commit、Refs 和 Sponsors。
- 将 Branches 和 Tags 分组。
- 增加仓库统计栏。
- 统一加载、空数据和错误状态。

### 第三阶段：Dashboard 和用户页

- 首页改为 Dashboard。
- 测试网地址表迁移到 Settings。
- 用户页增加资料侧栏、仓库搜索和 Badge 标签。
- 钱包按钮改为账户菜单。

### 第四阶段：体验完善

- 增加每个文件的最近提交信息。
- 增加仓库 Activity 页面。
- 增加骨架屏、复制成功提示和工具提示。
- 对不同大小的真实仓库进行性能测试。

## 13. 验收标准

- 首屏不再是大面积宣传 Hero，用户可以立即进入仓库工作流。
- 仓库页第一屏能看到仓库名、导航、分支、克隆入口和文件列表。
- 所有当前已有功能仍能访问，没有仅用于装饰的空页面或按钮。
- 代码、提交、Refs、赞助页面使用一致的仓库级导航。
- 360px、768px、1280px 和 1440px 宽度下没有文本或控件重叠。
- 长地址、长仓库名、长路径、长 SHA 和 CID 不会撑破布局。
- 键盘可以操作搜索、标签、分支选择、复制和钱包入口。
- `npm run build` 通过。
- 使用桌面端和移动端截图进行最终视觉验收。

## 14. 不建议照搬的内容

- 不照搬 Gogs 的 Semantic UI 依赖，继续使用 React 和项目现有 CSS。
- 不添加当前合约不支持的 Issues、Pull Requests、Wiki 等假功能。
- 不复制 Gogs 品牌、Logo 或具体视觉资产。
- 不隐藏 Injective、IPFS、钱包和治理等项目差异化能力。
- 不把每个页面区块都包装成卡片。

## 15. 推荐结论

建议采用“Gogs 信息架构 + igit 链上能力”的方案。

第一版优先完成全局导航和仓库 Code 页面，因为这两部分最能改变整体观感，也能直接复用现有数据能力。首页 Dashboard 和用户页放在第二批，可以避免一次改动范围过大。

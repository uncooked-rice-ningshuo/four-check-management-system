# 周通报 Word 说明页与版式实施方案

> 2026-11-08 方案及实施记录。独立入口、权限、受限 Tiptap 正文编辑、设置版本快照与 Go DOCX 渲染已在本地实现。编辑器目前支持段落样式、白名单字体字号、加粗/斜体/下划线/删除线、四种对齐、撤销/重做，以及可编辑的单行页首和页尾；页面采用 A4 纸张和灰色工作区。它仍不具备真实分页或付费 Tiptap DOCX 模板的转换能力。[Word 实开记录](../reviews/2026-11-08-weekly-report-word-layout-check.md)确认虚构长表样本可在本机 Word 打开；WPS 实开、完整角色浏览器矩阵和双端逐页比对仍待完成。下文“现状与差距”“分步实施”保留为原方案形成时的历史记录，以本段和当前代码为准。

**目标：**有权负责人在“通报管理 → 通报文件设置”像编辑文档一样编排通报说明和默认输出选择，生成者能在生成前核对说明与版式；组合 Word 以接近第八周定稿样例的版式生成，单项 Word 不带说明页。下载的 DOCX 在 Word 与 WPS 中无修复提示、无截字/溢出，纵横页、字体、段落、表格和分页符合下述验收标准。

**架构：**前端用受限富文本编辑器录入说明页的结构化段落，不存任意 HTML 或 DOCX。后端校验结构，把它填入随代码发布的脱敏 DOCX 版式模板，再从冻结的通报版本生成附件表格。浏览器编辑区域提供近似分页效果；“下载预览 Word”与实际生成共用后端渲染器，DOCX 文件是最终版式依据。

**2026-11-08 交互调整：**用户确认采用单个配置面板，左侧切换 Word/Excel、右侧配置内容，并保留保存和预览按钮。Word 前文、页首、页尾由纸张编辑器维护；右侧用纸张缩略图回显，预览抽屉展示当前未保存配置的完整文件结构。页首、页尾是受限单行文本（各最多 120 字），保存于 `introductionDocument` 并写入 DOCX 页眉页脚；页尾文字与页码分行，避免编辑时重叠。行内允许加粗、斜体、下划线、删除线和受限字体/字号；段落间距支持段前、段后及单倍、1.5 倍、双倍、固定值行距，服务端按受控值映射至 OOXML。第八周定稿参考文件的附件默认采用逐段“附件1：……”格式，仍允许在编辑器中逐字修改。浏览器预览只表示内容和顺序，真实分页需以 DOCX 在 Word/WPS 打开的结果验收。

**2026-11-08 三项新反馈的待确认实施方案：**保持单面板。Word 首页把固定标题改为可编辑的标题块，默认模板中以受控字段插入“学期名”“周次”，用户可改文字、字段顺序和排版；附件目录改为正文中的专用插入块，可在标题前后或段落之间移动，内容始终从已选检查类型和顺序生成。服务端只接受已定义字段、最多一个标题块和一个附件目录块，生成前校验，预览与真实 DOCX 使用同一结构。Excel 预览改为工作簿视图，从现有 Excelize 生成规则读取真正的工作表标题、合并标题行、字段列和顺序，提供工作表标签、行列坐标及真实 XLSX 下载；不复用 Word 附件列定义。优先复用现有 Go Excelize，不增加前端运行时库。撤销/重做覆盖标题、正文、附件块和页首尾的编辑历史；按钮显示明确文字或悬停说明，历史为空时解释禁用原因。完成条件包括保存重开、未保存预览、生成文件内容/顺序一致、空历史与编辑后撤销重做、无权限及失败路径的测试，并在真实浏览器、Word/WPS 与 Excel/WPS 中验收。该段仅为方案，尚未实现。

**技术栈候选：**React 19、Ant Design、TanStack Query；Tiptap 可负责说明正文的编辑，不能单独负责整份 Word 的版式。候选组合是 [Tiptap React 编辑器](https://tiptap.dev/docs/editor/getting-started/install/react)的 `@tiptap/react`、`@tiptap/pm`、`@tiptap/starter-kit`，按需加入 [TextAlign](https://tiptap.dev/docs/editor/extensions/functionality/textalign) 和 [TextStyle/FontFamily](https://tiptap.dev/docs/editor/extensions/functionality/fontfamily)，以 [JSON 文档](https://tiptap.dev/docs/editor/core-concepts/persistence)保存内容；Go 1.25 使用现有 `archive/zip`、XML 处理与文件存储，把允许的段落与文字属性映射到 DOCX。继续沿用 PostgreSQL、MinIO 和当前权限/审计。新增前端运行时依赖实施前需按仓库规则确认。

## 现状与差距

- 当前配置页 `web/src/features/reporting/ui/ReportOutputSettingsPanel.tsx` 是 `Input.TextArea`；`report_output_settings.introduction` 是纯文本。
- 当前 `server/internal/reportoutput/render.go` 把每一行说明写成**居中、宋体、14 磅**的段落，没有正文样式、首行缩进、落款位置或行距控制。`render_test.go` 检查 ZIP/XML、字体和横向节，但没有在 Word/WPS 中打开的版式证据。
- 仓库[第八周定稿样例](<../../参考文件/学生会办公室材料参考/2025-2026下word/定/第八周日常管理通报   （定  稿） .docx>)的 OOXML 经只读检查：说明页为 A4 纵向（11906×16838 twip），页面边距约上/下 1440、左/右 1080 twip；首行标题约 30 磅华文新魏，副标题约 20 磅黑体，主要正文约 15 磅仿宋，正文含首行缩进和固定行距；附件页转 A4 横向。样例有页脚和多种段落对齐方式。这些是**基线测量**，仍要逐页核对版式，不代表所有段落都用同一规则。
- 现有[样稿与图片](2026-11-08-modular-weekly-report-word-excel-plan.md)是设计参照，尚不是本系统生成文件在 Word/WPS 的验收结果。

## 用户实际看到什么

1. “通报管理 → 通报文件设置”是独立页面，放通报说明与默认检查类型/顺序；“系统配置 → 组织与业务”移除现有“通报文件”页签，保留检查类型的“参与通报”业务开关。说明编辑区采用 A4 页面式布局。页眉标题、学期/周次、附件页、页码和表格样式由系统控制。工具栏只提供“正文、分节小标题、落款、日期”、加粗、撤销/重做及段落对齐；正文默认字体、字号、缩进、行距由所选样式决定。支持复制纯文本；粘贴的外部格式先清理，避免把网页或任意 Word 样式带进模板。配置人员可以直接看到换行、段落、强调与大致分页。
2. 配置页增加“下载预览 Word”。它使用**当前未保存的编辑内容**与虚构学期/周次、虚构表格生成真实 DOCX，不写正式输出记录，不带真实学生信息。预览与正式生成调用同一套模板与段落渲染代码；下载后可在 Word/WPS 观察精确分页。保存时仍按现有 `version` 乐观锁，冲突保留编辑内容并提示重载。
3. 工作台与归档仍是分别点击“生成 Word”/“生成 Excel”。Word 弹窗展示本次采用的说明设置版本和检查类型；配置已被其他人更新时，生成返回 409，重新加载后再确认。单项 Word 不显示说明页，组合 Word 显示说明页，生成文件仅读取已选冻结版本数据。

**编辑边界：**当前实现是受控说明页编辑，不是完整 Office 替代品。允许仿宋/宋体/黑体及 14/15/16/20 磅选择，前端 JSON、服务端白名单和 DOCX 属性映射已接通；实际跨端字体替换与逐页验收仍须核对。图片、自由表格、页眉页脚编辑和上传任意 DOCX 不在当前范围。若目标转为任意 DOCX 的原位编辑与高保真往返，须重新评估 DOCX 原生编辑器及其许可、部署和兼容成本。

## 编辑器选型调研结论（2026-11-08）

- [Tiptap Markdown](https://tiptap.dev/docs/editor/markdown)是可选的导入/导出扩展；Tiptap 本体基于 ProseMirror 的富文本模型。起步三个包不自动提供字体、字号或对齐控件。对齐需要 TextAlign 扩展；字体、字号需要 TextStyle 系列扩展，且这些属性必须在服务端被允许并转成 OOXML，才会出现在本项目生成的 Word 中。
- Tiptap 的 [DOCX Conversion](https://tiptap.dev/docs/conversion/getting-started/install)是独立的 Start 计划 Beta 能力，不随起步三个包提供。其[支持矩阵](https://tiptap.dev/docs/conversion/getting-started/feature-support-matrix)中分节编辑和导出尚不支持；本项目说明首页纵向、附件横向，需要后端模板/渲染器处理。其[导出说明](https://tiptap.dev/docs/conversion/export/docx/editor-extension)也明确不保证与 Word 像素级一致，且不从编辑器屏幕分页推断 DOCX 页边界。
- [Docmost](https://github.com/docmost/docmost)展示 Tiptap 适合知识文档内容编辑；[Canvas Editor](https://github.com/Hufe921/canvas-editor)使用 Canvas 自行控制字体、分页并以插件导出 DOCX，但[插件问题记录](https://github.com/Hufe921/canvas-editor-plugin/issues/5)说明浏览器与 Office 格式一致性仍需实测。[SuperDoc](https://github.com/superdoc/docx-editor)采用 OOXML 文档模型编辑 DOCX；[ONLYOFFICE Docs](https://github.com/ONLYOFFICE/DocumentServer)则提供完整文档服务。后两者的能力、AGPL/商业许可与集成成本均超出当前只编辑说明页的需求，若范围扩大再做样本验证和许可评估。
- 推荐先用受控富文本模型编辑说明正文，系统继续掌握标题、纵横分节、附件表格、页眉页脚与 DOCX 生成。要承诺“字体变更和左右/两端对齐保留”，必须将允许值、缺失字体时的处理、跨端映射和 Word/WPS 实际文件验收写成测试；浏览器预览只用于编辑辅助。

## 入口与权限

- 侧边栏“通报管理”下保留“通报工作台”“通报归档”，新增独立的“通报文件设置”页面（建议路由 `/reports/output-settings`）。这是用户本轮对原 PRD“仅两个入口”的明确调整；不新增一级导航，也不把文件设置塞进工作台步骤。
- 新增全局动作 `REPORT_OUTPUT_SETTINGS_MANAGE`：学生会办公室主部门且组织角色为部长/副部长者默认拥有；超级管理员按现有超级管理员语义拥有初始化与恢复能力；其他启用账号仅在超级管理员**单独授予**该动作后拥有。普通系统管理员、其他部门负责人、仅有 `REPORT_VIEW`/`REPORT_GENERATE` 的人均不自动获得。对普通账号，显式拒绝覆盖角色默认授予；超级管理员继续沿用现有不受普通用户拒绝规则限制的语义。权限或账号状态变化沿用现有会话失效机制。
- 该动作只用于编辑通报说明、默认勾选类型与顺序及下载虚构数据预览；**不授予**通报数据查看、生成、审核、定稿或归档权限。设置作用于全局，用户规则必须是空范围；带学年、学期、周次、检查类型或历史部门/班级/年级范围的授予与拒绝均返回输入校验错误，避免把某周限定误认为全局设置权限。
- 页面显隐与直达路由用有效权限控制，后端 `GET` 编辑数据、`PUT` 保存、`POST` 预览分别重新鉴权；只读的生成弹窗仍按原 `REPORT_GENERATE` 读取必要默认设置。不能只靠按钮隐藏保护编辑接口。编辑权限用户即使没有 `REPORT_VIEW` 也能进入该设置页，但看不到工作台和归档。
- 管理员的用户权限详情新增“编辑通报文件设置”动作与全局范围说明；审计记录保存前后配置版本、操作者和时间，不把完整说明正文写入普通请求日志。保存仍按版本乐观锁防互相覆盖。

设置中的说明文档采用受控结构，例如 `{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"justify"},"content":[{"type":"text","text":"示例说明"}]}]}`。只允许 `doc`、`paragraph`、`text` 正文节点；最多 200 个段落、合计 20000 字、请求体不超过 64 KiB。前后端使用同一份明确的约束，但服务端独立校验；不接受 HTML、CSS 或任意未列出的 Tiptap 扩展节点。**2026-11-08 后续用户反馈已覆盖原先固定标题与目录的决定：**首页标题与副标题都是可编辑段落，使用相同工具栏，支持换行、字体、缩进；段落样式扩展为主标题、副标题、正文和一至三级标题等白名单。`{semesterName}`、`{weekNo}` 在实际渲染时替换。附件名单以所选检查类型生成普通文本段落，用户可直接修改内容和格式；修改类型时只自动更新未改动的生成文本。旧 `titleTemplate` 与 `attachmentPosition` 仅作历史配置兼容，新的文档以段落内容为准。

## 版式实现与数据契约

### 1. 脱敏模板与渲染

- 从样例提取版式参数，**新建不含个人信息**的 `server/internal/reportoutput/templates/weekly-report-v2.docx`，保留必要的 `styles.xml`、页脚和节设置；不直接把历史业务文件作为运行模板。模板作为代码资源嵌入，附 `layoutVersion=WORD_V2`。不开放用户上传模板，不改变八类检查字段口径。
- 说明首页的自动标题使用独立样式；编辑器中的每个段落映射到白名单样式 ID（正文/小标题/落款/日期），加粗映射为文字 run。所有文本做 XML 转义；未知节点、未知样式、外链、图片、脚本、过深层级、超长段落及超长文档均在服务端拒绝。
- 首页保持纵向，附件从下一页进入横向；每项检查从新页开始。附件表格沿用已确认字段，按类型设置固定列宽、重复表头、禁止单行拆跨页；长文本允许换行，不能裁字。当前硬编码每 18 行一页的逻辑要改为按模板可用高度及实际换行估算分段，并用 Word/WPS 的多页样例校准；年级合并仅在同一页内，跨页重新显示年级。
- 生成后的 DOCX 先检查 ZIP 安全、必需部件、关系、XML 可解析、节/样式/表格约束和所选类型顺序；只有通过才标记 `AVAILABLE`。自动化可用 LibreOffice 作额外渲染检查，但它**不能替代** Word/WPS 的实际打开验收。[Microsoft 的 Open XML 样式说明](https://learn.microsoft.com/en-us/office/open-xml/word/how-to-create-and-add-a-paragraph-style-to-a-word-processing-document)用于校对样式映射。

### 2. 设置与历史文件

- 新增只追加迁移 `server/migrations/044_report_output_word_layout.sql`：给 `report_output_settings` 增加 `introduction_document_json`（JSONB）；给 `report_output_files` 增加 `settings_version`、`layout_version` 和 `introduction_document_snapshot_json`（仅组合 Word 使用）。原 `introduction` 保留为兼容字段。
- 面向有生成权限者的 `GET /api/v1/reports/output-settings` 继续提供只读默认值，增加 `introductionDocument` 与 `layoutVersion`；旧纯文本配置在读取时转换成正文段落。新增 `GET /api/v1/reports/output-settings/editor`，只向编辑权限者返回设置和参与通报的检查类型名称/编码/顺序，不复用超级管理员专用的 `/admin/catalogs`。新增 `PUT /api/v1/reports/output-settings` 接收受限 JSON，服务端校验并提取纯文本回填原字段，沿用版本冲突和审计。原 `/api/v1/admin/report-output/settings` 在过渡期只作超级管理员兼容入口，调用同一服务并核对新动作；新页面不再使用。未知字段或格式不合法返回稳定的 400 错误码；内部失败返回 500，不回显文档原文。
- 生成请求携带用户已看到的 `settingsVersion`；服务端在生成时锁定设置，版本不一致返回 409。生成尝试保存设置版本、模板版本和**实际使用的说明快照**。历史文件继续按原文件下载，不重写、不重新套模板；新生成失败保留 `FAILED` 与安全错误码，重试产生新尝试。
- `POST /api/v1/reports/output-settings/preview` 仅 `REPORT_OUTPUT_SETTINGS_MANAGE` 可用，接受未保存的受限文档，返回真实 DOCX 流；不使用业务数据、不落正式输出文件，限制请求大小与频率，审计预览动作但不记录说明正文。该接口不修改系统设置。

## 分步实施

| 顺序 | 改动位置 | 先写的可观察测试与完成条件 |
| --- | --- | --- |
| 0. 真实文件基线 | `docs/plans/previews/weekly-report-2026-11-08/`、`server/internal/reportoutput/render_test.go` | 用虚构数据从**现有渲染函数**导出短/长说明及多页附件 DOCX；记录 Word/WPS 的版本、截图、页数和缺陷。既有设计图只作目标参照。阶段 0 产出可供审查的实际文件及差距清单。 |
| 1. 模板与渲染 | 新增 `server/internal/reportoutput/templates/weekly-report-v2.docx`；修改 `server/internal/reportoutput/render.go`、`validate.go`；新增 `render_layout_test.go` | 用真实 DOCX 验证首页/附件页方向、样式 ID、字体/字号、缩进、行距、页脚、表头重复、18/19/40 行分页、长中文换行、年级跨页重复和零记录/来源未完成。失败文件不得可下载。 |
| 2. 设置模型、权限与版本 | 新增迁移 044；修改 `server/internal/permission/service.go`、`server/internal/reportoutput/model.go`、`service.go`、`server/internal/store/report_output.go`、`server/internal/httpapi/router.go`、`report_output_handlers.go`、`web/src/shared/api/reportOutputApi.ts` | 一次性 PostgreSQL 验证迁移向前执行与旧纯文本兼容；权限测试覆盖办公室部长/副部长默认允许、其他部门负责人拒绝、系统管理员无默认权、指定用户全局授权、显式拒绝优先、带范围规则拒绝、权限变更会话失效；API 测试覆盖 GET/PUT/预览越权、结构校验、409、审计、快照与失败重试；旧 043 文件照常下载。 |
| 3. 独立页面、编辑与预览 | 从 `web/src/pages/admin/OrganizationConfigPage.tsx` 移除旧页签；新增 `web/src/pages/report/ReportOutputSettingsPage.tsx`、`web/src/features/reporting/ui/ReportIntroductionEditor.tsx` 及测试；修改 `ReportOutputSettingsPanel.tsx`、`ReportOutputPanel.tsx`、`web/src/app/App.tsx`、`web/src/app/layouts/AppLayout.tsx`、`web/src/pages/admin/PermissionDetailPage.tsx` | 三个通报子入口按有效权限分别可见；只有设置权限者能直达新页面与编辑；编辑/保存/关闭重开、复制粘贴清理、撤销重做、错误重试、版本冲突保留输入；未保存内容下载预览，浏览器样式与实际 DOCX 样式名称一致。 |
| 4. 跨端联调 | `web/e2e/weekly-report-summary.js` 或独立 `web/e2e/weekly-report-word-layout.js`、`server/scripts/verify-browser.py` | 隔离 PostgreSQL/MinIO、真实 API/Vite、Chrome：办公室负责人及独立授权人进入设置→预览→保存→组合/单项生成→鉴权下载；设置专权账号不能读取通报数据，其他负责人/无权账号直达与 API 均拒绝；409/500/文件损坏路径可恢复，已定稿数据版本不变。 |
| 5. Office/WPS 验收 | `docs/reviews/` 新增版式验收记录与样本清单 | 使用**本轮代码实际生成**的虚构数据 DOCX 分别在 Microsoft Word 与 WPS 打开并导出 PDF/截图。逐页对照样例和下面的验收表；记录版本、字体环境、页数与差异。未通过前不声称“版式完成”，不发布目标环境。 |

适用完整检查：前端 `pnpm lint && pnpm test && pnpm build`；后端修改文件 `gofmt`、`go vet ./... && go test ./...`；迁移在一次性 PostgreSQL 向前执行；真实浏览器回归使用仓库隔离套件。新依赖获确认后才更新 `package.json`/锁文件，不自行切换包管理器。

## 可验收的效果

| 场景 | 在 Word 与 WPS 中逐项核对 |
| --- | --- |
| 说明首页 | A4 纵向；标题、副标题、正文、落款各自使用模板样式；中文字体、字号、首行缩进、行距和段前后间距接近样例；段落顺序与编辑器一致，无多余空白页。 |
| 附件 | A4 横向；所选检查类型及顺序准确，每项从新页开始；标题、边距、表头、列宽与样例对照；长中文不越界、不遮挡；分页后表头重现，年级在新页重新显示。 |
| 数据与状态 | 数字/列名/零记录与冻结版本一致；单项 Word 无说明页；修改默认说明后，旧文件下载内容和历史版本不变；生成失败无下载入口。 |
| 兼容性 | Word、WPS 均可直接打开，无修复或兼容提示；各自导出的 PDF 不出现缺字、截字、表格越页或异常空白页。对不同字体替代造成的轻微换行差异记录在验收表，不把浏览器截图当成这项证据。 |

**交付物：**脱敏模板、受限编辑器、可下载的真实预览文件、三组虚构数据实生成 DOCX（短说明/长说明/多页附件）、Word 和 WPS 的截图或 PDF、结构与端到端测试报告、迁移与回退说明、更新后的项目状态。若目标环境缺少样例所用字体，先明确批准可替代字体并复验；不能以 ZIP/XML 测试通过代替视觉通过。

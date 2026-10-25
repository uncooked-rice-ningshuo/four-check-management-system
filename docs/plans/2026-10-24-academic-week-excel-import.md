# 学期周次 Excel 导入实施记录

> 用户审查后已确认开始实现。本文件保留原设计依据，并记录已采用的决定、代码实现和隔离验证；不代表迁移或部署到目标环境。

**目标：** 在“学期与周次”页面内，为一个已存在的学期批量新增周次，并保留两阶段预览、整批事务、权限、审计及失败重试能力。

**架构：** 前端把导入作为 `AcademicCalendarPage` 的页面内操作；Go 端新增校历周次导入应用服务与 XLSX 解析器，沿用现有私有文件存储和安全解析约束。确认导入时由 PostgreSQL 单事务复核周次冲突、写入全部周次、追加审计并确认批次；所有校历写入继续使用超级管理员权限。

**技术栈：** React 19、TypeScript、Ant Design、TanStack Query；Go、Gin、PostgreSQL、现有 XLSX/MinIO/审计能力。

---

## 1. 现状与依据

- [基础数据导入方案](2026-10-24-master-data-excel-import.md)只覆盖 11 类基础资料，采用只新增、预览后整批确认，不支持覆盖、批量删除或批次回滚；学期周次不在该范围内。
- 学年、学期、周次由[校历页面](../../web/src/pages/admin/AcademicCalendarPage.tsx)维护。现有校历 API 路由位于超级管理员路由组；基础资料导入使用 `MASTER_DATA_MANAGE`，两者权限边界不同。
- `semester_weeks` 已有 `(semester_id, week_no)` 唯一约束、周次号 1–60 检查及 `start_date <= end_date` 检查。应用服务还会拒绝与其他启用周次重叠的日期区间。当前服务没有检查周次日期是否落在所属学期内；此处是现有校历校验缺口，不能把它误写成已存在约束。
- 当前日期与启用的学年、学期、周次相匹配时会自动形成系统工作周。导入覆盖今天的日期区间可能改变自动工作周；有效的临时工作周覆盖仍优先。
- 周次 `attendance_penalty_multiplier` 目前必须为 1 或 2，普通周为 1，明确的教学礼拜周为 2。倍率影响新记录口径，历史记录已有独立快照。
- 编写本方案时最高迁移为 022；`master_data_import_batches.kind` 明确枚举那 11 类资料。周次导入采用独立批次表并追加迁移 023，后续模板格式升级另追加迁移 025，不改写已应用迁移。

## 2. 建议范围

### 首期包含

1. 在“学期与周次”页面的周次区域增加“导入周次”操作，不新增一级导航。
2. 一个导入批次绑定一个目标学期；用户必须在抽屉中明确选择学年/学期，不从列表筛选条件或首个选项推断目标。
3. 上传版本化 `.xlsx` 模板，服务端解析并返回逐行预览、错误和影响提示。
4. 只新增目标学期中尚不存在的周次号；任一错误会阻止整批确认。
5. 确认时在 PostgreSQL 单事务内重新校验、插入所有周次、写逐条校历审计和批次审计、更新批次状态。
6. 相同幂等键重试返回已确认结果；不同幂等键重复确认返回冲突。
7. 复用现有私有对象存储、上传大小限制、文件签名/OOXML 校验、源文件补偿清理和审计脱敏机制。

### 首期不包含

- 在导入文件中创建或修改学年、学期；目标学年/学期必须先在现有页面配置。
- 修改、覆盖、启用/停用或删除已有周次；不提供导入批次撤回或批量回滚。
- 导入临时工作周覆盖设置、课程排课或检查业务记录。
- 导入无效行以外的有效行子集；错误批次不能确认。
- 将权限从当前超级管理员扩大到 `MASTER_DATA_MANAGE` 用户。
- 对周次日期连续性或“每周必须恰好 7 天”作新要求；允许学校校历中的空档和实际日期长度。

## 3. 用户流程

1. 管理员进入“学期与周次”页面，在周次区域点击“导入周次”。
2. 抽屉要求明确选择一个学期；选项显示学年、学期及启用状态，不预选首项。目标学期停用时仍允许查看和选择，以保持当前校历维护能力；页面提示其下新周次不会参与自动工作周识别，除非上级学年和学期启用。
3. 用户下载本次模板、填写并上传。上传按钮在文件和目标学期都有效前禁用。
4. 服务端返回批次 ID、版本、行数统计、逐行结果及冲突错误。预览注明目标学期；若新周次日期覆盖上海时区的今天，显示“确认后可能改变系统自动识别的当前工作周”。若当前存在有效临时工作周覆盖，同时说明导入不会修改该覆盖。
5. 仅当所有行有效时显示“确认导入”。二次确认摘要包含目标学期、周次范围、将新增数量、倍率 1/2 周次数，以及覆盖今天的影响提示。
6. 成功后显示新增周次和批次状态，并刷新校历、工作周、当前周、课程上下文、到课率筛选项及工作台缓存。
7. 失败时展示稳定中文提示；预览版本冲突或数据库状态变化时要求重新上传预览。网络响应丢失时，客户端保留批次 ID 和原幂等键以安全重试。

关闭抽屉再打开时清除未确认的本地选择与预览状态，延续现有导入页面的生命周期规则。服务端批次和源文件仍按审计与文件清理规则留存，不因关闭弹窗删除。

## 4. XLSX 模板与字段规则

模板规格：模板代码 `ACADEMIC_WEEKS`，当前版本 `2`，唯一工作表“周次导入”；第 4 行逐列展示数据类型、格式和校验要求，第 5 行提供带底色/斜体的示例，第 6 行起填写数据，最多 60 行。解析器忽略格式行和示例行，旧版第 5 行起数据的版本 1 文件会因版本不匹配被拒绝，避免漏掉首条记录。严格校验文件扩展名、签名、OOXML 安全边界、工作表名、模板版本、标题/字段顺序、隐藏内容和公式。目标学期 ID 通过受保护 API 请求提交，不写入用户可编辑的工作表单元格。

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| 周次 | 是 | ASCII 十进制整数，1–60；同文件和目标学期已存在数据不得重复。 |
| 开始日期 | 是 | 严格 `YYYY-MM-DD`，按日期处理，不按本地时间戳解析。 |
| 结束日期 | 是 | 严格 `YYYY-MM-DD`，不得早于开始日期。 |
| 备注 | 否 | 去除首尾空格；最多 255 个 Unicode 字符；空白按空字符串处理。 |
| 到课率违纪计入倍数 | 否 | 仅允许 `1` 或 `2`；空白默认 `1`。只有明确填 `2` 才表示教学礼拜周。 |

重复周次号、文件内重叠日期、与数据库中其他启用周次重叠、已存在周次号、日期格式错误或倍率非法均逐行报告。所有“重叠”继续采用现有闭区间语义：两个范围共用同一天也视为冲突。数据库中的停用周次仍占用 `(semester_id, week_no)` 唯一键；导入不尝试复用或启用它。

## 5. 已采用的校历规则与决定

### 建议沿用的规则

- 目标学期必须存在；周次仍由服务端生成 UUID 和审计字段，新增周次默认为启用。
- 周次号范围 1–60；日期起始不晚于结束；倍率为 1 或 2，默认 1。
- 周次日期不可与任何其他启用周次重叠，不限于同一学期；文件内的新周次互相之间也不能重叠。
- 日期之间可有空档，周次日期不强制连续，也不固定为 7 天。
- 目标学期可以停用，但在选择器和预览中明确标识；若其学年或学期停用，新增周次不会进入系统当前工作周解析。

### 已确认决定

**R1：周次日期必须位于所属学期范围内。** 将其作为统一校历不变量，覆盖手工新增、编辑和导入预览/确认；缩短学期范围时也拒绝越界既有周次。

**R2：采用独立 `academic_week_import_batches` 表。** 新迁移 023 直接绑定学期、源文件和创建人，不扩展基础资料导入的 11 类语义；就绪检查与源文件绑定校验同步覆盖新表。

首期权限继续限于超级管理员；目标为单个已存在学期；只新增、不覆盖、不批量回滚；停用学期可选择但给出提示。

## 6. 后端接口与批次模型

### API

均放在已有超级管理员 `/api/v1/admin` 路由组：

- `GET /academic/weeks/import-spec`：返回字段、规则、模板代码和版本。
- `GET /academic/weeks/import-template`：下载 XLSX 模板。
- `POST /academic/weeks/imports`：`multipart/form-data` 上传 `file` 与 `semesterId`，持久化源文件和预览，返回 `201`。
- `GET /academic/weeks/imports/:batchID`：仅创建人可读取该批次；无权和不存在使用同一安全响应。
- `POST /academic/weeks/imports/:batchID/confirm`：请求体只含 `version`，确认使用 `Idempotency-Key` 请求头。

预览响应至少包含：`batchId`、`status`、`version`、`templateVersion`、目标学期/学年显示信息、`totalRows/validRows/invalidRows`、逐行字段值和错误、潜在当前工作周影响标记、创建时间。API 不返回存储对象 key、私有路径或数据库诊断。

沿用现有稳定错误分层：上传体积超限为 `FILE_TOO_LARGE`；坏文件/模板结构错误为明确的输入错误码；行校验错误为 422 并附安全字段错误；批次版本、并发唯一键或日期冲突为 409；未授权/越权仍为 401/403；未知存储、数据库或审计错误为内部错误，不兜底映射成输入错误。

### 迁移 023（采用独立批次表时）

新增 `academic_week_import_batches`，至少包含：

- `id`、`semester_id`、`source_file_id`、`created_by`；
- `template_version`、`status`、行数统计、`version`、`preview_json`；
- `confirm_idempotency_key`、`created_at`、`confirmed_at`。

约束包括：学期与文件/账号外键、每个源文件最多绑定一个批次、模板版本为正整数、合法状态、计数一致、有效批次无错误行、确认状态必须有确认时间和幂等键。初始迁移 023 仅允许版本 1；追加迁移 025 将该约束放宽为正整数以支持模板升级，并保留历史版本批次。`semester_id` 使用 `ON DELETE RESTRICT`。`preview_json` 保存规范化行数据、服务端生成的周次 ID、校验结果和确认所需数据；不保存或回显工作簿原始 XML。

同时更新 `Postgres.Ready`，确保新版本 API 在迁移缺失时不报告 ready；更新源文件补偿删除逻辑，将新批次的 `source_file_id` 视为已绑定文件；所有上传和批次审计均不记录单元格全文。

### 确认事务与并发

1. 按批次 ID、目标学期和创建人读取批次并 `FOR UPDATE` 锁定。
2. 已确认且幂等键相同则返回原结果；已确认但幂等键不同则返回冲突；版本不匹配或预览有错误则拒绝。
3. 获取一个事务级校历周次写锁；锁定目标学期/学年行并重新读取其状态和日期。
4. 重新验证所有 week number、现存数据和全局启用日期区间。检查必须在写事务内进行，不能只依赖上传预览时的快照。
5. 在同一事务内插入所有 `semester_weeks`、逐周追加 `ACADEMIC_CALENDAR_CHANGED` 审计、追加批次确认审计，并将批次更新为 `CONFIRMED`。
6. 任一插入、唯一约束、审计或批次更新失败，整批回滚。相同幂等键重试返回相同批次结果。

手工新增/编辑周次的仓储写入也必须使用同一写锁并在事务内复核日期冲突，否则导入与手工操作并发时仍可能出现跨批次重叠。此锁只串行化校历周次变更，不引入新服务或数据库扩展。

## 7. 前端结构与交互状态

- 端点客户端放入 `web/src/shared/api/academicWeekImportApi.ts`；批次/预览响应类型放入 `web/src/entities/academic/model/`。
- Query/mutation Hook 放入 `web/src/features/academic/`；导入抽屉、模板下载和预览表放入 `web/src/features/academic/ui/`。
- `AcademicCalendarPage` 只负责提供入口和将抽屉接入周次区域，不内嵌 HTTP 请求、解析规则或权限判断。
- 抽屉本地状态包括目标学期、上传文件、批次、确认版本和幂等键；目标学期、文件或批次变更时清理旧预览及确认状态。
- 明确覆盖加载、空学期、模板下载失败、上传失败、行错误、预览过期、无权限、确认中、确认失败可重试及成功状态。
- 成功后刷新现有 `AcademicCalendarPage` 已使用的校历、`work-context`、当前周、课程上下文、到课率筛选和工作台缓存。若日期包含今天，刷新后页面展示新的当前工作周。
- 复用 Ant Design 现有样式；窄屏下预览表的横向滚动限制在表格容器内，确认按钮可见且可键盘操作。

## 8. 实施任务和文件范围

以下任务已按此顺序实施；不提交、不推送、不部署，不操作真实业务库。

### Task 1：定义导入规格与 XLSX 解析

**新增：** `server/internal/academicimport/model.go`、`spec.go`、`service.go` 及测试。当前模板为版本 2：行 4 字段说明、行 5 示例、行 6 起业务数据。

**修改：** `server/internal/importjob/xlsx_parser.go` 与测试，补充隐藏行/列安全检查；不改写基础资料或业务导入规则。

**验证：** `cd server && go test ./internal/academic ./internal/academicimport -count=1`。覆盖模板生成可回读、字段顺序/版本、坏签名、公式/隐藏内容、空文件、61 行、格式错误、重复周次和倍率默认值。

### Task 2：建立批次持久化与事务确认

**新增：** `server/migrations/023_academic_week_import_batches.sql`、`server/migrations/025_academic_week_import_template_versions.sql`、`server/internal/store/academic_week_import.go`、`server/internal/store/academic_week_import_integration_test.go`。

**修改：** `server/internal/store/postgres.go` 的 readiness 和源文件绑定校验；周次新增/编辑持久化路径加入共同写锁与事务内日期冲突复核。

**验证：** 新迁移只追加；隔离 PostgreSQL 16 上覆盖成功、整批回滚、审计失败回滚、预览后冲突、相同/不同幂等键、跨批次并发、并发手工维护及源文件不可被误清理。

### Task 3：实现学期周次导入应用服务与 HTTP API

**新增：** `server/internal/httpapi/academic_week_import_handlers.go`。

**修改：** `server/internal/httpapi/router.go`、`server/cmd/api/main.go`；如依赖字段增加，更新对应 `AdminServices` 和 handler 接口。

**验证：** `cd server && go test ./internal/academic ./internal/academicimport ./internal/httpapi -count=1`。验证状态码、稳定错误码、响应结构、401、非超级管理员 403、创建人隔离、重复确认和安全错误输出。

### Task 4：接入前端 API、Hook 与导入抽屉

**新增：** `web/src/shared/api/academicWeekImportApi.ts`、`web/src/entities/academic/model/academicWeekImport.ts`、`web/src/features/academic/hooks/useAcademicWeekImport.ts`、`web/src/features/academic/ui/AcademicWeekImportDrawer.tsx`、对应测试。

**修改：** `web/src/pages/admin/AcademicCalendarPage.tsx` 及必要的页面测试。

**验证：** 覆盖明确目标选择、无隐式首项、预览行错误、确认门禁、重复键重试、关闭重开清状态、确认成功后查询缓存刷新、失败保留文件并可重试、StrictMode。

### Task 5：真实浏览器回归、完整门禁和状态同步

**新增：** `web/e2e/academic-week-import.js`。

**修改：** `server/scripts/verify-browser.py` 注册 `BROWSER_SUITE=academic-week-import`；隔离数据库集成测试位于 `server/internal/store/academic_week_import_integration_test.go` 并复用现有验证脚本；按仓库规则更新 `docs/project-status.md`。

**验证：** 在一次性 PostgreSQL/独立 MinIO、真实 API/Vite 与 Chrome 下覆盖：模板下载、错误预览不可确认、预览后数据库冲突、成功新增、幂等重试、当前周变化、关闭重开和失败清理。最后按仓库要求运行完整前后端门禁。

## 9. 验收门禁

### 单元与后端

- `cd server && gofmt -w internal/academicimport/*.go internal/store/academic_week_import.go internal/httpapi/academic_week_import_handlers.go`
- `cd server && go vet ./...`
- `cd server && go test ./... -count=1`
- `cd server && python3 scripts/verify-integration.py`（使用脚本创建的一次性 PostgreSQL/MinIO，不接触业务库）

### 前端

执行前核对 Node、pnpm 版本、`web/package.json` 的 `packageManager` 和锁文件。随后运行：

- `cd web && pnpm lint`
- `cd web && pnpm test`
- `cd web && pnpm build`
- `cd web && BROWSER_SUITE=academic-week-import pnpm test:browser`

所有完整门禁命令预期退出码均为 0；聚焦集成与浏览器命令的报告须明确列出通过场景、浏览器版本、SKIP/未覆盖项及清理结果。

浏览器回归须使用隔离数据库、独立 Bucket、虚构学年/学期，不向本机业务库或目标环境写入。报告 Chrome 版本、真实 API 场景、错误/清理错误数量和未覆盖项。若任一命令因依赖、工具链或服务不可用而未执行，按仓库要求说明未验证，不报为通过。

### 完成状态分层

最终交付分别记录：迁移、导入 API、校历页面、模板、一次性真实服务验证、角色权限和目标环境加载状态。隔离测试通过不代表目标环境已加载；部署、目标迁移和真实业务数据验收须按既有授权另行执行。

## 10. 首版模板（版本 1）实施验证记录

- **后端**：`go vet ./...`、`go test ./... -count=1` 通过；集成验证脚本在一次性 PostgreSQL 16/MinIO 上通过新增周次导入事务、审计回滚、幂等、预览后冲突和并发不重叠场景。
- **前端**：Node 24.12.0 / pnpm 11.7.0 下 `pnpm lint`、`pnpm build` 通过；全量 `pnpm test` 通过，73 文件/392 项，用时约 342 秒。构建保留既有主 bundle 大于 500 kB 提示。
- **浏览器**：一次性 PostgreSQL、独立 MinIO、真实 API/Vite 和 Chrome 156.0.8078.48 回归通过，报告为 [`c28a4432f0`](../../output/playwright/c28a4432f0/summary.json)；8 个端到端场景，浏览器错误和清理错误均为 0。
- **真实环境**：本机业务库和目标环境均未应用迁移、部署或写入业务数据。

## 11. 决定记录

本轮审查后，用户确认开始实现；按 R1、R2 及第 2、5 节规则落地。没有扩展到目标环境部署、真实业务数据写入或非超级管理员授权。具体验证、浏览器版本和运行环境边界同步记录在 `docs/project-status.md`。

## 12. 2026-10-25 同步基础数据模板新版式

- 周次模板沿用基础数据导入模板的逐列格式说明和独立示例行：第 4 行格式说明、第 5 行示例、第 6 行起填写业务数据；示例值采用可读的周次、日期、备注和倍率组合，批注提醒按当前选择学期替换。
- 模板版本升为 2，解析器只接受新版布局，避免旧版版本 1 第 5 行首条业务记录被误跳过。下载响应设置 `Cache-Control: no-store`。
- 迁移 025 将 `academic_week_import_batches.template_version` 从固定值 1 放宽为正整数，以便保存版本 2 批次；确认阶段继续严格核对预览批次模板版本。尚未确认的版本 1 预览需用版本 2 文件重新预览后确认。
- 本次只改源码、测试和文档，不执行目标环境迁移或部署。版本 2 的测试、隔离浏览器回归与目标环境状态见项目状态文档最新条目；此前第 10 节记录是版本 1 的历史验证结果。

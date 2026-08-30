# 阶段 2 工作流 A：通用导入框架 + 到课率第一增量实施计划

> **范围说明：** 本计划是[完整阶段 2 实施主计划](./2026-08-30-phase-two-completion-implementation.md)的工作流 A，只交付通用导入底座与“到课率”第一增量。即使 Task 1–12 全部完成，也只能宣称“工作流 A 完成”，绝不能宣称“阶段 2 完成”。完整阶段 2 还必须通过角色化工作台、普通用户应用外壳、七类检查、其余六类导入、部门工作流、状态看板及跨角色验收门禁。

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 交付可复用的两阶段 Excel 导入框架，并以到课率实现模板下载、上传解析、错误预览、原子确认和草稿记录查询的端到端闭环。

**Architecture:** 后端新增 `inspection` 和 `importjob` 模块；解析阶段只持久化批次/行，确认阶段在 PostgreSQL 单事务内写检查记录、扩展表、批次结果和审计。前端在到课率详情页通过 TanStack Query 调用专用 API client，并以 Drawer 完成导入流程，不新增导入一级导航。

**Tech Stack:** Go 1.25、Gin、pgx/PostgreSQL、MinIO、React 19、TypeScript、Ant Design、TanStack Query、Vitest、`github.com/xuri/excelize/v2@v2.11.0`（用户已批准）。

---

## 前置门禁

- 当前工作分支必须是 `feat-0.1-2`，并保留工作区已有文档改动。
- Task 3 所需的 Go 1.25 基线和 `github.com/xuri/excelize/v2@v2.11.0` 已获用户明确许可；不得借此引入其他运行时依赖。
- 各 Task 的 `Commit` 步骤只是建议检查点。根据仓库规则，每次执行 `git add`、`git commit`、推送或创建 PR 前都要再次取得用户许可。
- 如测试数据库或 MinIO 未配置，先运行纯单元/API 测试；集成测试要准确标记为未运行，不得伪报通过。

### Task 1: 固化阶段 2 数据模型和权限动作

**Files:**
- Create: `server/migrations/005_phase_two_attendance_import.sql`
- Modify: `server/internal/store/postgres_test.go`
- Modify: `server/internal/permission/service.go`
- Modify: `server/internal/permission/service_test.go`

**Step 1: 写失败的迁移契约测试**

在 `postgres_test.go` 断言迁移包含 `import_templates`、`import_batches`、`import_rows`、`inspection_records`、`attendance_details`，状态约束、外键、批次行唯一键、当前模板唯一索引和未删除到课率重复唯一索引。

**Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/store -run TestPhaseTwoAttendanceImportMigrationDefinesRequiredSchema`

Expected: FAIL，提示 `005_phase_two_attendance_import.sql` 不存在或缺少结构。

**Step 3: 新增只追加迁移**

创建五张表、约束和必要索引；不得改写 `001`—`004`。为批次状态、记录状态、来源和模板状态增加数据库 CHECK；重复唯一索引只覆盖 `deleted_at IS NULL`。

**Step 4: 写权限动作失败测试并实现**

增加 `INSPECTION_VIEW`、`INSPECTION_ENTER` 领域常量和默认角色/部门映射计算测试，确保单独 `DENY` 仍优先。

Run: `cd server && go test ./internal/permission ./internal/store`

Expected: PASS。

**Step 5: Commit（执行前请求许可）**

```bash
git add server/migrations/005_phase_two_attendance_import.sql server/internal/store/postgres_test.go server/internal/permission/service.go server/internal/permission/service_test.go
git commit -m "feat: add attendance import schema"
```

### Task 2: 建立检查记录领域与范围查询

**Files:**
- Create: `server/internal/inspection/model.go`
- Create: `server/internal/inspection/service.go`
- Create: `server/internal/inspection/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/store/postgres_test.go`

**Step 1: 写失败的领域测试**

覆盖到课率记录必填上下文、正数旷课学时、发生日期位于周次、`DRAFT/EXCEL` 初始状态，以及跨检查类型输入被拒绝。

Run: `cd server && go test ./internal/inspection`

Expected: FAIL，模块尚未实现。

**Step 2: 实现最小领域模型和服务接口**

定义显式状态/来源常量、`AttendanceRecord`、分页筛选、范围条件和仓储接口。服务只接受已经由权限服务编译的范围，不允许先查全量再内存过滤。

**Step 3: 写失败的仓储集成契约测试**

验证查询 SQL 必含 semester/week/inspection type/department/class/grade 范围和白名单排序；列表返回 `items,total,page,pageSize`。

**Step 4: 实现 PostgreSQL 查询与扫描**

参数化全部值，只允许固定排序映射；返回空结果时保证数组非 `null`。

Run: `cd server && go test ./internal/inspection ./internal/store`

Expected: PASS。

**Step 5: Commit（执行前请求许可）**

```bash
git add server/internal/inspection server/internal/store/postgres.go server/internal/store/postgres_test.go
git commit -m "feat: add attendance inspection domain"
```

### Task 3: 实现安全 XLSX 解析器与黄金样例

**Files:**
- Modify: `server/go.mod`
- Modify: `server/go.sum`
- Create: `server/internal/importjob/xlsx_parser.go`
- Create: `server/internal/importjob/xlsx_parser_test.go`
- Create: `server/internal/importjob/testdata/attendance-v1-valid.xlsx`
- Create: `server/internal/importjob/testdata/attendance-v1-invalid.xlsx`

**Step 1: 获得新增 XLSX 依赖许可**

向用户说明候选库、锁定版本、维护状态和引入原因。只有用户批准后执行 `go get`；若不批准，回到设计评审选择标准库最小实现并重写本 Task。

**Step 2: 写解析器失败测试**

测试合法模板、非 ZIP 签名、`.xlsm`、宏条目、加密文件、多个业务 Sheet、错误表头、超过 1000 行、ZIP 条目/解压总量超限和空行归一化。

Run: `cd server && go test ./internal/importjob -run TestXLSXParser`

Expected: FAIL，解析器尚未实现。

**Step 3: 实现压缩包预检和工作簿解析**

先用 `archive/zip` 检查条目数、单条/总解压大小、宏和可疑路径，再交给 XLSX 库读取单个业务 Sheet。解析器只输出通用字符串行与模板元数据，不查询数据库。

**Step 4: 生成并锁定黄金样例**

样例必须包含模板版本、机器字段编码、中文表头、说明行与代表性数据；测试校验 Sheet 名、字段顺序、关键单元格和业务行数，而非只检查文件可打开。

Run: `cd server && go test ./internal/importjob -run 'TestXLSXParser|TestAttendanceGoldenWorkbook'`

Expected: PASS。

**Step 5: Commit（执行前请求许可）**

```bash
git add server/go.mod server/go.sum server/internal/importjob
git commit -m "feat: add secure xlsx parser"
```

### Task 4: 实现到课率导入适配器

**Files:**
- Create: `server/internal/importjob/attendance_adapter.go`
- Create: `server/internal/importjob/attendance_adapter_test.go`
- Modify: `server/internal/store/postgres.go`

**Step 1: 写失败的行校验测试**

覆盖学号/姓名/班级一致性、课程与教室存在、日期位于周次、时间/学时格式、备注长度、批内重复和库内重复。断言错误包含稳定 `code`、`field`、中文 `message`、`suggestion`，不泄露 SQL。

Run: `cd server && go test ./internal/importjob -run TestAttendanceAdapter`

Expected: FAIL。

**Step 2: 实现引用批量预取**

仓储按本批次出现的学号、班级、课程和教室编码批量查询，禁止逐行 N+1。适配器把合法行归一化为 ID、UTC/本地业务时间和受控数值。

**Step 3: 实现重复检查**

先生成规范化重复键标记批内重复，再一次查询数据库已有键并返回 `RECORD_ALREADY_EXISTS`。

Run: `cd server && go test ./internal/importjob`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add server/internal/importjob server/internal/store/postgres.go
git commit -m "feat: validate attendance import rows"
```

### Task 5: 实现导入批次状态机和解析用例

**Files:**
- Create: `server/internal/importjob/model.go`
- Create: `server/internal/importjob/service.go`
- Create: `server/internal/importjob/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/storage/service.go`
- Modify: `server/internal/storage/service_test.go`

**Step 1: 写失败的状态机与上传测试**

覆盖允许/拒绝的状态迁移、文件元数据必须 `AVAILABLE`、对象 SHA 校验、上传失败批次不可进入预览、重复解析、解析失败稳定码和取消边界。

Run: `cd server && go test ./internal/importjob ./internal/storage`

Expected: FAIL。

**Step 2: 扩展文件服务的受控读取能力**

增加只通过文件 ID 读取且重新校验状态/完整性的内部方法；不向 handler 暴露 object key 或预签名匿名地址。

**Step 3: 实现批次仓储和解析编排**

上传时生成服务端对象名，创建批次后显式进入 `PARSING`；解析结果以事务批量替换该批次行并进入 `PREVIEW_READY`，错误只保存稳定码和安全摘要。

**Step 4: 验证**

Run: `cd server && go test ./internal/importjob ./internal/storage ./internal/store`

Expected: PASS。

**Step 5: Commit（执行前请求许可）**

```bash
git add server/internal/importjob server/internal/storage server/internal/store/postgres.go
git commit -m "feat: add import batch parsing workflow"
```

### Task 6: 实现原子确认、幂等和审计

**Files:**
- Modify: `server/internal/importjob/service.go`
- Modify: `server/internal/importjob/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/store/postgres_test.go`
- Modify: `server/internal/audit/model.go`

**Step 1: 写失败的确认测试**

覆盖零错误批次确认、含错误批次拒绝、旧版本冲突、相同幂等键重试返回同一结果、不同键重复确认拒绝、插入扩展表失败时主表/行/批次/审计全部回滚。

Run: `cd server && go test ./internal/importjob ./internal/store -run 'Test.*Confirm|Test.*Rollback'`

Expected: FAIL。

**Step 2: 实现单事务确认**

使用 `SELECT ... FOR UPDATE` 锁定批次；检查状态、版本、计数和幂等键；批量写 `inspection_records` 与 `attendance_details`，回填 `import_rows.record_id`，更新批次为 `CONFIRMED` 并在同一事务追加审计。

**Step 3: 验证隐私和失败恢复**

审计仅保存批次 ID、上下文、计数、模板版本和结果，不保存 `raw_data_json` 或整行学生信息；所有失败保持可诊断且不产生部分业务数据。

Run: `cd server && go test ./internal/importjob ./internal/store ./internal/audit`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add server/internal/importjob server/internal/store/postgres.go server/internal/store/postgres_test.go server/internal/audit/model.go
git commit -m "feat: confirm attendance imports atomically"
```

### Task 7: 暴露受保护的 REST API

**Files:**
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`
- Modify: `server/cmd/api/main.go`
- Modify: `server/cmd/api/main_test.go`

**Step 1: 写失败的路由/API 测试**

覆盖模板下载、multipart 上传、批次摘要、分页预览、确认、取消和到课率列表；逐项断言 401、403、跨部门/班级范围拒绝、404 防 IDOR、409 版本冲突、稳定错误结构。

Run: `cd server && go test ./internal/httpapi -run 'Test.*Import|Test.*Attendance'`

Expected: FAIL。

**Step 2: 注入服务并注册路由**

扩展 `AdminServices`/运行时装配，handler 只做 HTTP 输入输出转换。上传限制同时设置 Gin 请求体上限和服务层校验；确认读取 `Idempotency-Key`。

**Step 3: 实现权限与范围适配**

在每个受保护接口解析当前账号、动作和资源上下文，通过权限服务生成范围，再调用应用服务；按 ID 查询先加载最小授权上下文，不直接返回是否存在敏感资源。

Run: `cd server && go test ./internal/httpapi ./cmd/api`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add server/internal/httpapi server/cmd/api
git commit -m "feat: expose attendance import api"
```

### Task 8: 建立前端领域模型、API client 与 Query hooks

**Files:**
- Create: `web/src/entities/inspection/model/inspection.ts`
- Create: `web/src/entities/import-job/model/importJob.ts`
- Create: `web/src/features/attendance/api/attendanceApi.ts`
- Create: `web/src/features/attendance/api/attendanceApi.test.ts`
- Create: `web/src/features/attendance/hooks/useAttendanceRecords.ts`
- Create: `web/src/features/import-job/api/importJobApi.ts`
- Create: `web/src/features/import-job/hooks/useImportBatch.ts`
- Create: `web/src/features/import-job/model/importErrorMessages.ts`
- Create: `web/src/features/import-job/model/importErrorMessages.test.ts`

**Step 1: 写失败的 client/错误映射测试**

断言所有请求通过 `shared/api/httpClient`，multipart 不手工伪造 boundary，分页参数正确，确认发送版本和幂等键，稳定错误码映射为可操作中文提示。

Run: `cd web && pnpm test -- src/features/attendance src/features/import-job`

Expected: FAIL。

**Step 2: 实现类型、client 和 hooks**

服务端状态全部交给 TanStack Query；query key 包含检查类型、学期、周次和筛选；确认成功精确失效批次与到课率列表缓存。

**Step 3: 验证**

Run: `cd web && pnpm test -- src/features/attendance src/features/import-job`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add web/src/entities/inspection web/src/entities/import-job web/src/features/attendance web/src/features/import-job
git commit -m "feat: add attendance import clients"
```

### Task 9: 实现到课率详情页

**Files:**
- Create: `web/src/pages/inspection/AttendancePage.tsx`
- Create: `web/src/pages/inspection/AttendancePage.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/shared/layouts/AdminLayout.tsx`
- Modify: `web/src/shared/styles/global.css`

**Step 1: 写失败的页面测试**

覆盖路由、上下文选择、加载/空/失败状态、分页、权限不足时动作隐藏或禁用、批量导入打开抽屉，以及记录状态和来源文本。

Run: `cd web && pnpm test -- src/pages/inspection/AttendancePage.test.tsx`

Expected: FAIL。

**Step 2: 实现页面编排**

新增“日常检查业务”导航和 `/inspections/attendance` 路由；页面只编排上下文、筛选、表格和导入 feature，不把请求或权限规则散落在展示组件中。

**Step 3: 可访问性与响应状态**

为上传、筛选、分页和抽屉提供标签、焦点顺序和非颜色状态文本；覆盖 403 与 409 的明确提示。

Run: `cd web && pnpm test -- src/pages/inspection/AttendancePage.test.tsx`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add web/src/pages/inspection web/src/app/App.tsx web/src/shared/layouts/AdminLayout.tsx web/src/shared/styles/global.css
git commit -m "feat: add attendance inspection page"
```

### Task 10: 实现导入 Drawer 与确认弹窗

**Files:**
- Create: `web/src/features/import-job/ui/AttendanceImportDrawer.tsx`
- Create: `web/src/features/import-job/ui/AttendanceImportDrawer.test.tsx`
- Create: `web/src/features/import-job/ui/ImportPreviewTable.tsx`
- Modify: `web/src/pages/inspection/AttendancePage.tsx`

**Step 1: 写失败的交互测试**

覆盖 `.xlsx` 客户端提示、上传中禁用、解析状态、摘要/筛选、行级错误与建议、存在错误时确认禁用、零错误确认弹窗、重复提交防护、409 冲突和成功刷新。

Run: `cd web && pnpm test -- src/features/import-job/ui`

Expected: FAIL。

**Step 2: 实现四步 Drawer**

沿用 Ant Design Steps、Upload、Table、Alert 和 Modal；关闭前提示未确认批次；批次创建后把 ID 保存在当前路由可恢复状态中，但不把敏感行写入 URL 或浏览器持久存储。

**Step 3: 实现确认弹窗**

展示检查类型、学期周次、部门、模板版本、合法行数和入库状态；生成每次用户确认动作唯一幂等键，并防止按钮连点。

Run: `cd web && pnpm test -- src/features/import-job/ui src/pages/inspection/AttendancePage.test.tsx`

Expected: PASS。

**Step 4: Commit（执行前请求许可）**

```bash
git add web/src/features/import-job/ui web/src/pages/inspection/AttendancePage.tsx
git commit -m "feat: add attendance import drawer"
```

### Task 11: PostgreSQL、MinIO 与 API 集成验证

**Files:**
- Create: `server/internal/importjob/import_integration_test.go`
- Modify: `server/internal/storage/minio_integration_test.go`
- Modify: `server/internal/store/postgres_test.go`
- Modify: `docker-compose.yml`（仅当现有测试编排确需增加初始化，不改变生产拓扑）

**Step 1: 写端到端后端集成测试**

在临时数据库应用全部迁移，上传黄金 XLSX 到私有 MinIO，创建预览并确认；断言对象 SHA、行计数、主/扩展记录、批次状态和审计一致。

**Step 2: 覆盖失败路径**

删除/篡改对象后解析必须失败；制造扩展表写入错误验证事务回滚；跨部门账号读取批次与文件均拒绝。

**Step 3: 运行验证**

Run: `cd server && go test ./internal/importjob ./internal/storage ./internal/store -run Integration -count=1`

Expected: PASS；若环境变量缺失，测试显式 SKIP 并在交付报告列为未完成验收项。

**Step 4: Commit（执行前请求许可）**

```bash
git add server/internal/importjob/import_integration_test.go server/internal/storage/minio_integration_test.go server/internal/store/postgres_test.go docker-compose.yml
git commit -m "test: verify attendance import integration"
```

### Task 12: 浏览器验收、文档同步与全量检查

**Files:**
- Create: `web/e2e/attendance-import.spec.ts`（若仓库尚无 E2E 目录，先按现有 Playwright skill 建立最小测试装配）
- Modify: `README.md`（如存在）
- Modify: `docs/plans/2026-08-30-attendance-import-design.md`

**Step 1: 启动隔离的本机服务**

使用测试数据库、私有测试 Bucket 和非生产账号；不得复用生产密钥或数据。记录实际 API/Web 地址和种子数据范围。

**Step 2: 执行 Playwright 验收**

验证模板下载、合法文件上传、错误预览、零错误确认弹窗、确认后列表出现 `DRAFT/EXCEL` 记录；再验证无权限账号和伪装文件被拒绝。

**Step 3: 运行后端完整检查**

Run: `cd server && gofmt -w <本次修改的.go文件> && go vet ./... && go test ./...`

Expected: PASS。

**Step 4: 运行前端完整检查**

Run: `cd web && pnpm lint && pnpm test && pnpm build`

Expected: PASS。

**Step 5: 核对迁移与文档**

在空测试数据库向前执行 `001`—`005`，检查模板版本、错误码、API、权限和实际实现一致；更新设计文档中的任何已批准偏差及其理由。

**Step 6: Commit（执行前请求许可）**

```bash
git add web/e2e README.md docs/plans/2026-08-30-attendance-import-design.md
git commit -m "test: verify attendance import workflow"
```

## 工作流 A 完成判定（不代表阶段 2 完成）

以下条件只判定本计划的 Task 1–12 是否完成。阶段 2 的总完成条件与角色验收矩阵以[完整阶段 2 实施主计划](./2026-08-30-phase-two-completion-implementation.md)为准；不得把本节检查结果外推为完整阶段 2 已完成。

- 到课率标准模板可鉴权下载，合法 `.xlsx` 最多 1000 行可以预览。
- 文件级和行级错误使用稳定错误码，前端提供明确修复建议。
- 有任一错误的批次不能确认；零错误批次一次性原子写入草稿记录。
- 相同幂等键不会重复入库，旧版本确认明确返回 409。
- 所有按 ID 的批次、行和文件访问均通过后端动作权限与数据范围验证。
- 原文件、模板版本、SHA-256、批次、导入行、业务记录与审计可相互追溯。
- 后端 `go vet ./...`、`go test ./...`，前端 `pnpm lint`、`pnpm test`、`pnpm build` 和计划内 Playwright 场景均实际通过；受环境限制的项目明确列出。
- 尚未交付角色化工作台、普通用户应用外壳、七类检查完整 CRUD、其余六类导入、部门提交/审核/退回/确认和状态看板时，阶段 2 必须保持“未完成”。

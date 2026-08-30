# Phase Two Completion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在工作流 A 的通用导入底座与到课率第一增量之上，完整交付阶段 2 的普通用户应用外壳、角色化工作台、七类检查、手工与 Excel 录入、部门提交审核状态机、状态看板及跨角色验收闭环。

**Architecture:** 保持模块化单体与 `Handler → Application Service → Domain/Repository → PostgreSQL/MinIO` 调用方向。统一检查主表承载公共状态、版本、数据范围和软删除信息，七张扩展表承载类型字段；所有列表、聚合和按 ID 访问把有效 RBAC + ABAC 范围编译进数据库查询。前端新增独立于 `/admin` 的普通用户应用外壳，通过 `UI → TanStack Query hook → API client → /api/v1` 获取服务端权威结果；工作台只编排阶段 2 检查与部门工作流，不伪实现阶段 3 通报能力。

**Tech Stack:** Go 1.25、Gin、pgx/PostgreSQL、MinIO、React 19、TypeScript、Vite、Ant Design、React Router、TanStack Query、Vitest、Playwright、`github.com/xuri/excelize/v2@v2.11.0`。

---

## 1. 计划地位、范围与执行规则

本文件是阶段 2 的唯一总完成计划。[工作流 A：通用导入框架 + 到课率第一增量](./2026-08-30-attendance-import-implementation.md)是本计划的强依赖；其 Task 1–12 保留原编号并在原计划中执行，不复制、不重编号。工作流 A 完成只开放后续门禁，不能单独支持“阶段 2 完成”的结论。

阶段 2 包含：

- 独立普通用户应用外壳、有效权限导航和角色化工作台。
- 当前学年、学期、周次、临时周覆盖、我的待办、本部门业务状态和有效权限快捷入口。
- 统一检查主表和到课率、无机课堂、教室卫生、早操早签、寝室卫生、晚归、公共寝室卫生七张扩展表。
- 七类检查的手工录入、查询、编辑、软删除、批量操作、乐观锁、审计和权限范围。
- 七类检查的模板、预览、确认；工作流 A 已实现到课率，其余六类复用通用导入框架。
- 部门提交、审核、退回、确认状态机和实时状态看板。
- PostgreSQL、MinIO、API、前端及 Playwright 的跨角色验收。

明确不属于阶段 2：通报生成、通报审核、通报定稿、发布、完整全院通报进度，以及“待生成通报”“待通报审核”“待定稿”等卡片或动作。这些均属于阶段 3；阶段 2 不创建假接口、假数据、禁用占位按钮或一级导航来伪实现。

执行规则：

- 严格按 Task 0 → Task 19 的依赖顺序执行；每批结束先反馈实际改动和验证结果。
- 每个 Task 先写测试并观察指定 RED，再写最小 GREEN；未观察 RED 不进入实现。
- 后端运行时代码改动后至少 `gofmt`、`go vet ./...`、`go test ./...`；前端运行时代码改动后至少 `pnpm lint`、`pnpm test`、`pnpm build`。
- 迁移只追加；不得重写已存在的 `001`—`005`。
- 本计划所有 Commit 步骤只是审查检查点。执行任何 `git add`、`git commit`、`git push` 或创建 PR 前，仍须取得用户明确许可。
- 若需要新增 Playwright 仓库依赖或其他未批准依赖，先暂停并取得用户许可。已批准的依赖仅包括 Go 1.25 基线和 `excelize/v2@v2.11.0`。

## 2. 依赖图与批次

```text
Task 0 工作流 A 总门禁
  └─ Batch B：Task 1–5 七类模型与手工检查
      ├─ Batch C：Task 6–7 普通用户外壳与工作上下文
      ├─ Batch D：Task 8–10 部门状态机与审核视图
      │   └─ Batch E：Task 11–12 状态看板与角色工作台
      └─ Batch F：Task 13–15 其余六类导入与安全回滚
          └─ Batch G：Task 16–19 集成、跨角色和总完成门禁
```

并行限制：同一工作树内不并行修改迁移、路由、共享类型或同一测试文件。Task 6–7 与 Task 8 的纯领域部分可在不同隔离工作树并行，但合并后必须先执行 Task 16 的契约矩阵。

## 3. 固定领域契约

### 3.1 七类检查代码与扩展表

| `typeCode` | 中文名 | 扩展表 | 工作流 A 状态 |
|---|---|---|---|
| `ATTENDANCE` | 到课率 | `attendance_details` | 已由工作流 A 创建 |
| `INORGANIC_CLASSROOM` | 无机课堂 | `inorganic_classroom_details` | 本计划新增 |
| `CLASSROOM_HYGIENE` | 教室卫生 | `classroom_hygiene_details` | 本计划新增 |
| `MORNING_EXERCISE` | 早操早签 | `morning_exercise_details` | 本计划新增 |
| `DORM_HYGIENE` | 宿舍卫生 | `dorm_hygiene_details` | 本计划新增 |
| `LATE_RETURN` | 晚归 | `late_return_details` | 本计划新增 |
| `PUBLIC_DORM_HYGIENE` | 公共宿舍卫生 | `public_dorm_hygiene_details` | 本计划新增 |

不得为了减少表数把两个检查类型静默合并到同一扩展表。公共字段进入 `inspection_records`；类型特有字段、唯一键和校验进入相应扩展模型。最终字段清单必须在 Task 1 的业务字典门禁确认后固化。

### 3.2 状态与稳定错误码

- 检查记录：`DRAFT`、`PENDING_REVIEW`、`RETURNED`、`CONFIRMED`、`LOCKED`。
- 部门汇总：`NOT_ENTERED`、`ENTERING`、`PENDING_REVIEW`、`RETURNED`、`COMPLETED`。
- 允许转换：`DRAFT/RETURNED → PENDING_REVIEW`，`PENDING_REVIEW → RETURNED/CONFIRMED`；`CONFIRMED → LOCKED` 属于后续通报锁定调用，不开放普通编辑接口。
- 关键错误码：`VALIDATION_FAILED`、`FORBIDDEN_SCOPE`、`VERSION_CONFLICT`、`INVALID_STATE_TRANSITION`、`DUPLICATE_RECORD`、`BATCH_HAS_ERRORS`、`BATCH_NOT_ROLLBACKABLE`、`RECORD_LOCKED`、`NOT_FOUND`。
- HTTP 映射：输入错误 400、未认证 401、无动作或范围 403、不存在 404、版本/状态/重复冲突 409、文件类型过大 413/415；响应不得暴露 SQL、对象 key、内部路径或敏感原文。

### 3.3 数据范围硬约束

所有检查列表、详情、批次、提交、状态聚合和工作台查询必须把学年/学期、周次、检查类型、部门、班级、年级及单独允许/拒绝编译进 Repository SQL。禁止先读取全院记录再在 Service、Handler 或浏览器过滤。按 ID 查询必须使用 `id + scope predicate`，无权访问统一返回安全的 403/404 策略，不泄露资源存在性。

## 4. 阶段 2 总体验收矩阵

“有效权限”指角色、组织、部门映射、指定授权和单独允许的并集，扣除单独拒绝，再与上下文范围取交集。前端显隐只改善体验，后端逐次权威鉴权。

| 角色 | 允许看到的阶段 2 工作台内容 | 允许动作 | 必须禁止 |
|---|---|---|---|
| `MEMBER` 普通成员 | 当前学年/学期/周次及临时覆盖提示；本人待补录/被退回任务；本人有效范围内的部门状态与检查快捷入口 | 在授权类型与范围内查看、手工录入、导入、修改自己的可编辑草稿、提交 | 审核/确认；查看其他部门或越范围个人数据；修改他人记录；系统配置；任何阶段 3 通报卡片或动作 |
| 部门管理员（部长/副部长或同等指定授权） | 普通成员内容；本部门、本检查类型待审核/已退回/已完成状态和待办 | 在有效部门/类型范围内录入、导入、编辑、批量处理、提交、审核、退回、确认 | 越部门/类型/学期范围；审核自己无授权的提交；直接修改已确认/锁定记录；阶段 3 通报动作 |
| 办公室/指定管理员 | 当前工作上下文；仅其显式检查授权覆盖的跨部门待办、状态与快捷入口 | 按有效授权查看或处理检查/部门工作流；有汇总动作时查看范围内汇总 | 因“办公室”身份自动获得全部敏感数据；无检查动作时录入/审核；通报生成/审核/定稿卡片 |
| 学院查看者 | 当前工作上下文；授权范围内的只读状态、计数和检查入口 | 只读查看 | 新建、导入、编辑、删除、批量操作、提交、审核、退回、确认、回滚；敏感明细越权；所有阶段 3 动作 |
| 系统管理员/超级管理员 | 管理后台入口；若有效检查权限允许，则显示相应阶段 2 工作台、全局或授权范围状态 | 系统管理员按显式检查权限；超级管理员按既有超级权限处理阶段 2 | 系统管理员仅凭系统配置角色读取全部敏感检查数据；绕过状态机/版本/审计；阶段 2 页面出现阶段 3 伪能力 |

矩阵还需覆盖：单独 `DENY` 覆盖角色允许、临时授权过期、账号禁用、跨部门 URL 猜测、按 ID 猜测、批量请求混入越权 ID、403/409 前端呈现及数据库查询确实包含范围谓词。

## 5. 实施任务

### Task 0: 完成并复核工作流 A 总门禁

**Depends on:** 无。

**Files:**
- Follow: `docs/plans/2026-08-30-attendance-import-implementation.md`
- Verify: `server/internal/importjob/**`
- Verify: `server/internal/inspection/**`
- Verify: `server/migrations/005_phase_two_attendance_import.sql`
- Verify: `web/src/features/attendance-import/**`

**RED:** 按工作流 A 尚未完成的 Task 继续 TDD；不得把已有文件视作自动通过。至少保留伪装 ZIP/OOXML、压缩炸弹、重复键、权限范围、幂等和版本冲突失败测试。

**Run:** 工作流 A 每个 Task 的聚焦命令。

**Expected RED:** 新行为对应测试先失败，原因与该 Task 缺口一致。

**GREEN:** 完成工作流 A Task 1–12，并逐项记录实际测试、PostgreSQL、MinIO 和 Playwright 结果。任何环境未验证项保持打开。

**Verify:** `cd server && go vet ./... && go test ./...`；`cd web && pnpm lint && pnpm test && pnpm build`；执行工作流 A 的数据库、对象存储和 E2E 命令。

**Checkpoint:** 只有工作流 A 自身完成；不得宣称阶段 2 完成。提交前仍须用户许可。

### Task 1: 固化七类字段字典、重复键与状态契约

**Depends on:** Task 0。

**Files:**
- Create: `server/internal/inspection/catalog.go`
- Create: `server/internal/inspection/catalog_test.go`
- Create: `web/src/entities/inspection/model/types.ts`
- Create: `web/src/entities/inspection/model/types.test.ts`
- Modify: `docs/plans/2026-08-30-phase-two-completion-implementation.md`（只记录已确认字段决议）

**RED:** 表驱动测试遍历七个 `typeCode`，断言中文名、扩展表、字段顺序、必填项、最大长度、重复键、支持动作和模板版本均存在；测试未知类型必须返回 `VALIDATION_FAILED`。前端镜像测试断言 API 代码到表单 schema 的映射完整且无阶段 3 类型。

**Run:** `cd server && go test ./internal/inspection -run 'TestInspectionCatalog|TestUnknownInspectionType'`；`cd web && pnpm test -- src/entities/inspection/model/types.test.ts`。

**Expected RED:** FAIL，提示目录或七类定义缺失。

**GREEN:** 仅实现不可变目录和显式类型，不写数据库/HTTP 逻辑。根据 PRD、技术方案和已确认业务材料固定字段；仍未确认的字段不得用猜测值进入迁移，暂停在本 Task 反馈。

**Verify:** 上述聚焦命令 PASS，`rg "REPORT|FINALIZE|PUBLISH" server/internal/inspection/catalog.go web/src/entities/inspection/model/types.ts` 无阶段 3 能力。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: define phase two inspection catalog`。

### Task 2: 追加其余六类扩展表迁移

**Depends on:** Task 1。

**Files:**
- Create: `server/migrations/006_phase_two_inspection_extensions.sql`
- Modify: `server/internal/store/postgres_test.go`
- Create: `server/internal/store/testdata/phase_two_extension_constraints.sql`

**RED:** 迁移契约测试断言六张扩展表、到 `inspection_records(id)` 的级联受限外键、字段 CHECK、有效记录 partial unique index、查询索引及无个人信息注释；临时 PostgreSQL 测试断言重复有效记录失败、软删除后可重建、孤儿扩展失败。

**Run:** `cd server && go test ./internal/store -run 'TestPhaseTwoExtensionMigration|TestPhaseTwoExtensionConstraints'`。

**Expected RED:** FAIL，提示 `006` 不存在或缺少表/约束。

**GREEN:** 新增 `006`，不修改 `005`。公共学期、周次、类型、部门、班级、来源、状态、版本、创建人和软删除继续只存主表；扩展表只存类型字段。

**Verify:** 聚焦测试 PASS；在空临时库顺序执行 `001`—`006` PASS；向下不提供破坏性回滚脚本。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add remaining inspection extension schema`。

### Task 3: 实现七类领域模型与校验适配器

**Depends on:** Task 2。

**Files:**
- Create: `server/internal/inspection/details.go`
- Create: `server/internal/inspection/validation.go`
- Create: `server/internal/inspection/validation_test.go`
- Modify: `server/internal/inspection/model.go`

**RED:** 每类至少覆盖合法、缺必填、边界值、未知字段、公式前缀文本、重复键组成和跨字段不变量；公共寝室卫生与寝室卫生必须走不同模型。断言错误包含稳定字段路径和错误码，不回显完整个人信息。

**Run:** `cd server && go test ./internal/inspection -run 'TestValidate.*Details|TestDuplicateKey'`。

**Expected RED:** FAIL，提示适配器或校验不存在。

**GREEN:** 用 `DetailAdapter` 注册表选择七类显式结构；禁止 `map[string]any` 直接穿透到 SQL。公式防护在文件输出边界处理，输入校验记录安全摘要。

**Verify:** 聚焦测试 PASS，`go test ./internal/inspection` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: validate seven inspection detail models`。

### Task 4: 实现手工录入、列表和详情 API

**Depends on:** Task 3。

**Files:**
- Create: `server/internal/store/inspection.go`
- Create: `server/internal/store/inspection_test.go`
- Modify: `server/internal/inspection/service.go`
- Modify: `server/internal/inspection/service_test.go`
- Create: `server/internal/httpapi/inspection_handlers.go`
- Create: `server/internal/httpapi/inspection_handlers_test.go`
- Modify: `server/internal/httpapi/router.go`

**RED:** API 测试覆盖 `POST /api/v1/inspections`、`GET /api/v1/inspections`、`GET /api/v1/inspections/:id` 的 200/201、400、401、403、404、409；Repository 测试用查询捕获或测试库证明范围谓词进入 SQL。分页响应固定为 `items,total,page,pageSize`。

**Run:** `cd server && go test ./internal/httpapi ./internal/inspection ./internal/store -run 'Test(Create|List|Get)Inspection|TestInspectionScopeQuery'`。

**Expected RED:** FAIL，端点 404 或 Repository 缺失。

**GREEN:** Handler 只转换输入输出；Service 校验动作、上下文、目录和状态；Repository 在一个事务写主表、对应扩展表和审计。创建幂等键或唯一约束阻止重复。

**Verify:** 聚焦测试 PASS；无权范围响应不泄露记录内容；`go vet ./... && go test ./...` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add scoped manual inspection APIs`。

### Task 5: 实现编辑、软删除和批量操作

**Depends on:** Task 4。

**Files:**
- Modify: `server/internal/store/inspection.go`
- Modify: `server/internal/store/inspection_test.go`
- Modify: `server/internal/inspection/service.go`
- Modify: `server/internal/inspection/service_test.go`
- Modify: `server/internal/httpapi/inspection_handlers.go`
- Modify: `server/internal/httpapi/inspection_handlers_test.go`

**RED:** 覆盖 `PATCH /api/v1/inspections/:id`、`DELETE /api/v1/inspections/:id`、`POST /api/v1/inspections/batch`；断言 `version` 必填，过期版本 409，已确认/锁定拒绝，批量混入一个越权或锁定 ID 时整批回滚，审计记录前值/后值/原因/人员/时间且敏感字段脱敏。

**Run:** `cd server && go test ./internal/httpapi ./internal/inspection ./internal/store -run 'Test(Update|Delete|Batch).*Inspection|TestInspectionAudit'`。

**Expected RED:** FAIL，端点或版本条件缺失。

**GREEN:** SQL 更新使用 `WHERE id=$1 AND version=$2 AND <scope> AND deleted_at IS NULL` 并原子递增版本；软删除不物理删除；批量操作单事务、全有或全无。

**Verify:** 聚焦测试及 `go vet ./... && go test ./...` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: protect inspection mutations with versions`。

### Task 6: 建立普通用户应用外壳和有效权限导航

**Depends on:** Task 4。

**Files:**
- Create: `web/src/shared/layouts/AppLayout.tsx`
- Create: `web/src/shared/layouts/AppLayout.test.tsx`
- Create: `web/src/app/router/EffectivePermissionRoute.tsx`
- Create: `web/src/app/router/EffectivePermissionRoute.test.tsx`
- Modify: `web/src/app/App.tsx`

**RED:** 路由测试断言普通成员能进入 `/workbench` 和授权的 `/inspections/:typeCode`，不能进入受管理员守卫的 `/admin`；菜单按有效动作与范围显示，单独 `DENY` 隐藏入口；直接 URL 越权显示 403，不重定向成空白页。断言无“导入”“审核”“统计”一级导航。

**Run:** `cd web && pnpm test -- src/shared/layouts/AppLayout.test.tsx src/app/router/EffectivePermissionRoute.test.tsx`。

**Expected RED:** FAIL，普通用户外壳/路由不存在。

**GREEN:** 保留 `/admin` 管理外壳；新增普通应用外壳，一级导航仅按原型包含工作台和日常检查等已交付范围。导入和审核是检查详情内动作。

**Verify:** 聚焦测试 PASS；`rg "path:.*(import|review|statistics)" web/src/app web/src/shared/layouts` 不产生一级导航命中；`pnpm lint && pnpm test && pnpm build` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add permission-aware member application shell`。

### Task 7: 实现学期周次工作上下文与临时覆盖

**Depends on:** Task 6。

**Files:**
- Create: `server/internal/academic/work_context.go`
- Create: `server/internal/academic/work_context_test.go`
- Create: `server/internal/httpapi/work_context_handlers.go`
- Modify: `server/internal/httpapi/router.go`
- Create: `web/src/entities/work-context/api/workContextClient.ts`
- Create: `web/src/entities/work-context/model/useWorkContext.ts`
- Create: `web/src/entities/work-context/ui/WorkContextBar.tsx`
- Create: `web/src/entities/work-context/ui/WorkContextBar.test.tsx`

**RED:** 后端测试覆盖教学周命中、非教学日、临时覆盖优先、覆盖过期、无活动学期和无权修改；前端覆盖加载、失败、无上下文、覆盖提示及键盘可达。修改覆盖必须有权限、版本与审计。

**Run:** `cd server && go test ./internal/academic ./internal/httpapi -run 'Test.*WorkContext|Test.*WeekOverride'`；`cd web && pnpm test -- src/entities/work-context/ui/WorkContextBar.test.tsx`。

**Expected RED:** FAIL，上下文 API/UI 不存在。

**GREEN:** 提供 `GET /api/v1/work-context`；如 PRD 已授权临时覆盖，提供受控更新端点并复用既有校历数据。前端只显示服务端计算结果，不在浏览器自行推导周次。

**Verify:** 聚焦测试与两端完整检查 PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: expose auditable academic work context`。

### Task 8: 追加部门提交模型并实现显式状态机

**Depends on:** Task 5。

**Files:**
- Create: `server/migrations/007_phase_two_department_workflow.sql`
- Create: `server/internal/workflow/model.go`
- Create: `server/internal/workflow/state_machine.go`
- Create: `server/internal/workflow/state_machine_test.go`
- Modify: `server/internal/store/postgres_test.go`

**RED:** 迁移测试断言 `department_submissions`、提交版本、范围唯一键、状态、退回原因、确认人/时间和审计事件；状态机表驱动测试覆盖全部允许转换及所有非法转换。确认后直接退回/编辑必须失败。

**Run:** `cd server && go test ./internal/workflow ./internal/store -run 'TestSubmissionStateMachine|TestDepartmentWorkflowMigration'`。

**Expected RED:** FAIL，迁移/状态机不存在。

**GREEN:** `007` 只追加；提交聚合唯一范围为学年、学期、周次、部门、检查类型和活动修订号。状态机是唯一转换入口，Repository 不接受任意状态字符串。

**Verify:** 聚焦测试 PASS；空临时库执行 `001`—`007` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add department submission state machine`。

### Task 9: 实现提交、审核、退回和确认 API

**Depends on:** Task 8。

**Files:**
- Create: `server/internal/store/workflow.go`
- Create: `server/internal/store/workflow_test.go`
- Create: `server/internal/workflow/service.go`
- Create: `server/internal/workflow/service_test.go`
- Create: `server/internal/httpapi/workflow_handlers.go`
- Create: `server/internal/httpapi/workflow_handlers_test.go`
- Modify: `server/internal/httpapi/router.go`

**RED:** 覆盖提交、审核视图、退回、确认端点的成功、401、动作 403、范围 403、空提交、混入越权记录、旧版本 409、非法状态 409、重复幂等键。退回原因必填；确认只包含当前范围内可确认记录。事务故障必须回滚提交、记录状态和审计。

**Run:** `cd server && go test ./internal/workflow ./internal/store ./internal/httpapi -run 'Test(Submit|Review|Return|Confirm)Department|TestWorkflowRollback'`。

**Expected RED:** FAIL，服务或端点不存在。

**GREEN:** Service 在单事务中锁定聚合版本、校验全部记录、转换记录与提交状态并写审计；Repository 查询下推范围。审核不能通过直接更新记录状态绕过聚合。

**Verify:** 聚焦测试及 `go vet ./... && go test ./...` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: implement auditable department workflow`。

### Task 10: 在检查详情内实现部门审核视图

**Depends on:** Task 9、Task 6。

**Files:**
- Create: `web/src/features/department-workflow/api/workflowClient.ts`
- Create: `web/src/features/department-workflow/model/useDepartmentWorkflow.ts`
- Create: `web/src/features/department-workflow/ui/DepartmentReviewPanel.tsx`
- Create: `web/src/features/department-workflow/ui/DepartmentReviewPanel.test.tsx`
- Create: `web/src/pages/inspection/InspectionDetailPage.tsx`
- Modify: `web/src/app/App.tsx`

**RED:** 测试覆盖审核列表加载/空/失败、只读视图、退回原因校验、确认二次确认与影响范围、403、409 后刷新、无动作权限时不显示按钮。断言路由中不存在 `/review` 一级页面。

**Run:** `cd web && pnpm test -- src/features/department-workflow/ui/DepartmentReviewPanel.test.tsx`。

**Expected RED:** FAIL，组件/接口不存在。

**GREEN:** Query hook 统一缓存键包含学期/周/部门/类型；mutation 成功或 409 后失效正确查询。审核面板嵌入检查详情，按钮显隐不替代后端鉴权。

**Verify:** 聚焦测试及 `pnpm lint && pnpm test && pnpm build` PASS；`rg "path:.*review" web/src/app/router` 无一级审核路由。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: embed department review in inspection detail`。

### Task 11: 实现部门实时状态聚合 API

**Depends on:** Task 9。

**Files:**
- Create: `server/internal/dashboard/service.go`
- Create: `server/internal/dashboard/service_test.go`
- Create: `server/internal/store/dashboard.go`
- Create: `server/internal/store/dashboard_test.go`
- Create: `server/internal/httpapi/dashboard_handlers.go`
- Create: `server/internal/httpapi/dashboard_handlers_test.go`
- Modify: `server/internal/httpapi/router.go`

**RED:** 用 PostgreSQL fixture 覆盖五个汇总状态：无记录 `NOT_ENTERED`、有可编辑草稿 `ENTERING`、已提交 `PENDING_REVIEW`、退回 `RETURNED`、全部确认 `COMPLETED`。覆盖跨部门/检查类型 `DENY`、空范围和按学期周次切换；断言 SQL 自带范围条件。

**Run:** `cd server && go test ./internal/dashboard ./internal/store ./internal/httpapi -run 'TestDepartmentStatus|TestDashboardScope'`。

**Expected RED:** FAIL，聚合服务或端点不存在。

**GREEN:** 提供 `GET /api/v1/dashboard/department-statuses`，基于记录与提交真实状态实时聚合，不持久化易漂移的假计数；必要缓存只能在权限和工作上下文键下短时使用并正确失效。

**Verify:** 聚焦测试 PASS；查询计划使用范围/状态索引；`go vet ./... && go test ./...` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add scoped department status dashboard`。

### Task 12: 实现跨角色阶段 2 工作台

**Depends on:** Task 7、Task 10、Task 11。

**Files:**
- Create: `server/internal/dashboard/workbench.go`
- Modify: `server/internal/dashboard/service_test.go`
- Create: `web/src/features/phase-two-dashboard/api/dashboardClient.ts`
- Create: `web/src/features/phase-two-dashboard/model/usePhaseTwoDashboard.ts`
- Create: `web/src/features/phase-two-dashboard/ui/PhaseTwoDashboard.tsx`
- Create: `web/src/features/phase-two-dashboard/ui/PhaseTwoDashboard.test.tsx`
- Create: `web/src/pages/workbench/WorkbenchPage.tsx`
- Modify: `web/src/app/App.tsx`

**RED:** 后端按五类角色矩阵返回当前上下文、我的待办、本部门/授权范围状态和有效快捷入口；前端逐角色覆盖加载、空、失败、403、过期授权和无快捷入口。负向断言页面没有待生成通报、待通报审核、待定稿、全院通报进度及其 API 请求。

**Run:** `cd server && go test ./internal/dashboard -run 'TestWorkbenchFor.*Role|TestWorkbenchExcludesPhaseThree'`；`cd web && pnpm test -- src/features/phase-two-dashboard/ui/PhaseTwoDashboard.test.tsx`。

**Expected RED:** FAIL，工作台聚合/UI 不存在。

**GREEN:** 快捷入口由服务端有效动作与范围生成，前端只做显示和路由验证；“我的待办”仅来自检查补录、退回和授权审核任务。办公室/指定管理员没有检查授权时不自动得到敏感卡片。

**Verify:** 两端聚焦和完整检查 PASS；`rg "待生成通报|待通报审核|待定稿|全院通报进度" web/src server/internal/dashboard` 只允许出现在负向测试。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add role-aware phase two workbench`。

### Task 13: 为其余六类实现模板与 XLSX 适配器

**Depends on:** Task 3、Task 0。

**Files:**
- Create: `server/internal/importjob/inspection_adapters.go`
- Create: `server/internal/importjob/inspection_adapters_test.go`
- Create: `server/internal/importjob/testdata/inorganic-classroom-golden.xlsx`
- Create: `server/internal/importjob/testdata/classroom-hygiene-golden.xlsx`
- Create: `server/internal/importjob/testdata/morning-exercise-golden.xlsx`
- Create: `server/internal/importjob/testdata/dorm-hygiene-golden.xlsx`
- Create: `server/internal/importjob/testdata/late-return-golden.xlsx`
- Create: `server/internal/importjob/testdata/public-dorm-hygiene-golden.xlsx`
- Modify: `server/internal/importjob/xlsx_parser.go`

**RED:** 表驱动测试逐类验证模板标题/版本/字段顺序、黄金样例解析、缺列、重复列、超长/非法值、公式单元格、隐藏 sheet、宏/伪装文件和 1000 行上限。Fixture 仅用虚构姓名、学号、班级、寝室号。

**Run:** `cd server && go test ./internal/importjob -run 'TestSixInspectionAdapters|TestSixInspectionGoldenFiles|TestRejectUnsafeWorkbook'`。

**Expected RED:** FAIL，六类适配器和样例不存在。

**GREEN:** 复用工作流 A 的 ZIP/OOXML 安全预检和通用 parser；类型差异只存在目录/适配器，不复制上传安全逻辑。模板输出的用户文本继续防公式注入。

**Verify:** 聚焦测试 PASS；扫描 fixtures/logs 确认无真实个人信息；`go test ./internal/importjob` PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add six inspection import adapters`。

### Task 14: 泛化七类模板、预览、确认 API 与前端

**Depends on:** Task 13、Task 5。

**Files:**
- Modify: `server/internal/importjob/service.go`
- Modify: `server/internal/importjob/service_test.go`
- Modify: `server/internal/httpapi/import_handlers.go`
- Modify: `server/internal/httpapi/import_handlers_test.go`
- Create: `web/src/features/inspection-import/api/inspectionImportClient.ts`
- Create: `web/src/features/inspection-import/model/useInspectionImport.ts`
- Create: `web/src/features/inspection-import/ui/InspectionImportDrawer.tsx`
- Create: `web/src/features/inspection-import/ui/InspectionImportDrawer.test.tsx`
- Modify: `web/src/pages/inspection/InspectionDetailPage.tsx`

**RED:** 后端表驱动覆盖七类模板/预览/确认、类型与模板版本不匹配、任一行错误禁止确认、幂等确认、旧版本 409、范围 403、确认事务回滚；前端覆盖每类动态列、错误下载、二次确认、加载/失败/403/409。断言无导入一级导航。

**Run:** `cd server && go test ./internal/importjob ./internal/httpapi -run 'Test.*InspectionImport'`；`cd web && pnpm test -- src/features/inspection-import/ui/InspectionImportDrawer.test.tsx`。

**Expected RED:** FAIL，通用端点只支持到课率或 UI 不存在。

**GREEN:** 端点以受控 `typeCode` 选择适配器；确认在单事务写主表、正确扩展表、批次、映射和审计。前端 Drawer 由字段目录驱动并嵌入检查详情。

**Verify:** 两端聚焦和完整检查 PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: generalize two-phase imports for seven checks`。

### Task 15: 实现遵守状态不变量的导入回滚

**Depends on:** Task 14、Task 9。

**Files:**
- Create: `server/internal/importjob/rollback.go`
- Create: `server/internal/importjob/rollback_test.go`
- Modify: `server/internal/store/importjob.go`
- Modify: `server/internal/httpapi/import_handlers.go`
- Modify: `server/internal/httpapi/import_handlers_test.go`
- Create: `web/src/features/inspection-import/ui/ImportRollbackButton.tsx`
- Create: `web/src/features/inspection-import/ui/ImportRollbackButton.test.tsx`

**RED:** 覆盖只有同一批次所有关联记录仍为未提交 `DRAFT`、未审核、未锁定且版本匹配时才能整批回滚；任一记录为 `PENDING_REVIEW/RETURNED/CONFIRMED/LOCKED`、已被其他编辑、越权或批次部分缺失时返回 409/403 且零改动。回滚写审计但不删除原文件与解析追踪。

**Run:** `cd server && go test ./internal/importjob ./internal/httpapi -run 'Test.*Rollback'`；`cd web && pnpm test -- src/features/inspection-import/ui/ImportRollbackButton.test.tsx`。

**Expected RED:** FAIL，回滚能力不存在。

**GREEN:** 单事务锁定批次与关联记录，重新鉴权并软删除业务记录；批次标记为已回滚，保留 SHA、原文件、行结果和审计。已审核或已锁定数据绝不通过此接口直接回滚。

**Verify:** 聚焦测试、故障注入和两端完整检查 PASS。

**Checkpoint:** 提交前请求用户许可；建议消息 `feat: add safe import batch rollback`。

### Task 16: 建立 API 契约、错误码与跨角色权限矩阵测试

**Depends on:** Task 12、Task 15。

**Files:**
- Create: `server/internal/httpapi/phase_two_contract_test.go`
- Create: `server/internal/httpapi/phase_two_role_matrix_test.go`
- Create: `web/src/shared/api/phaseTwoErrors.ts`
- Create: `web/src/shared/api/phaseTwoErrors.test.ts`
- Modify: `web/src/shared/api/httpClient.ts`

**RED:** 对本计划所有端点逐一断言状态码、稳定错误码、camelCase 结构、禁用账号、五类角色、单独 DENY、过期授权、跨范围 ID、批量混入越权项和 409；前端断言错误码转成明确中文操作建议，不解析原始消息。

**Run:** `cd server && go test ./internal/httpapi -run 'TestPhaseTwo(Contract|RoleMatrix)'`；`cd web && pnpm test -- src/shared/api/phaseTwoErrors.test.ts`。

**Expected RED:** FAIL，至少暴露一个不一致错误或范围缺口。

**GREEN:** 统一错误映射和响应 DTO；修复必须落在权威 Service/Repository，不通过前端隐藏或放宽后端校验让测试通过。

**Verify:** 聚焦与两端完整检查 PASS；查询日志/测试 spy 证明范围谓词下推。

**Checkpoint:** 提交前请求用户许可；建议消息 `test: enforce phase two API role matrix`。

### Task 17: 增加 PostgreSQL、MinIO 与失败恢复集成测试

**Depends on:** Task 16。

**Files:**
- Create: `server/internal/integration/phase_two_postgres_test.go`
- Create: `server/internal/integration/phase_two_minio_test.go`
- Create: `server/internal/integration/phase_two_recovery_test.go`
- Modify: `README.md`

**RED:** 在隔离测试库/Bucket 运行迁移、七类创建与导入、部门状态转换、回滚与对象下载鉴权；故障注入覆盖数据库提交失败、对象缺失、校验和不符和重试。无 MinIO/数据库时测试必须明确 skip 原因，不得假 PASS。

**Run:** `cd server && go test ./internal/integration -run TestPhaseTwo -count=1`（按 README 设置测试专用 PostgreSQL/MinIO 环境变量）。

**Expected RED:** 首次至少因集成装配或恢复路径缺口失败；若仅环境缺失，状态为明确 SKIP，并保持总门禁未完成。

**GREEN:** 使用非生产凭证、私有测试 Bucket 和自动唯一前缀；下载按关联批次/业务范围重新鉴权，不返回对象 key。补齐幂等重试与安全错误。

**Verify:** 集成测试实际 PASS；`go vet ./... && go test ./...` PASS。记录数据库/MinIO版本及命令，不记录密钥。

**Checkpoint:** 提交前请求用户许可；建议消息 `test: verify phase two storage integration`。

### Task 18: 执行 Playwright 跨角色与阶段边界验收

**Depends on:** Task 17。

**Files:**
- Create: `web/e2e/phase-two-role-workbench.spec.ts`
- Create: `web/e2e/phase-two-inspections.spec.ts`
- Create: `web/e2e/phase-two-department-workflow.spec.ts`
- Create: `web/e2e/phase-two-imports.spec.ts`
- Create: `web/e2e/phase-two-boundary.spec.ts`
- Create/Modify: `web/playwright.config.ts`（仅在依赖已存在或用户另行批准后）
- Modify: `README.md`

**RED:** 先运行角色/边界场景并观察缺页、缺动作或错误状态失败。至少覆盖 MEMBER、部门管理员、办公室/指定管理员、学院查看者、系统管理员/超级管理员，以及加载、空、失败、403、409、键盘操作和确认弹窗焦点。

**Run:** 使用仓库既有 Playwright 命令；若尚无仓库装配，先通过已批准的 Playwright skill 对本机临时服务执行同等场景。新增 `@playwright/test` 前必须取得用户许可。

**Expected RED:** 新场景首次因未实现行为失败；不得通过跳过关键断言获得 GREEN。

**GREEN:** 验证七类手工录入、至少一类六增量导入的每类代表路径、部门提交/退回/确认、实时状态联动和所有禁止动作。边界 spec 明确断言阶段 2 页面/网络均无通报生成、审核、定稿、发布和全院通报进度。

**Verify:** Playwright 场景实际 PASS；截图、录像、fixture 无真实个人信息；失败产物不进入提交。

**Checkpoint:** 提交前请求用户许可；建议消息 `test: accept phase two across roles`。

### Task 19: 执行阶段 2 总完成门禁并同步文档

**Depends on:** Task 0–18 全部实际完成。

**Files:**
- Modify: `README.md`
- Modify: `docs/plans/2026-07-11-four-check-management-system-technical-design.md`（只记录批准偏差）
- Verify: `docs/plans/2026-08-30-attendance-import-implementation.md`
- Verify: `docs/plans/2026-08-30-phase-two-completion-implementation.md`

**RED:** 先运行总检查清单；任何失败、SKIP 的 PostgreSQL/MinIO/Playwright、未确认字段、未验证角色或阶段 3 伪卡片都使总门禁失败。

**Run:**

```bash
cd server && gofmt -w <本阶段修改的.go文件> && go vet ./... && go test ./...
cd web && pnpm lint && pnpm test && pnpm build
git diff --check
```

另运行 Task 17 的实际 PostgreSQL/MinIO 集成命令和 Task 18 的实际 Playwright 命令。

**Expected RED:** 只要有一项未交付或环境未验证，总门禁保持 FAIL/OPEN，不得改写为完成。

**GREEN:** 更新 README 的开发/测试命令、角色入口、七类导入限制和阶段边界；只记录已经批准且实际发生的技术偏差。逐行复核第 4 节角色矩阵、数据库范围下推证据和阶段 3 负向断言。

**Verify:** 所有命令实际 PASS，`git diff --check` 无输出；内部链接可解析；Task 0–19 依赖无环且编号连续；无调试日志、真实个人信息或未解释 TODO。

**Checkpoint:** 只有此门禁全部通过才可宣称“阶段 2 完成”。任何提交、推送或 PR 仍须用户明确许可；建议最终消息 `docs: record completed phase two acceptance`。

## 6. 完成定义

阶段 2 只有同时满足以下条件才能完成：

- 工作流 A Task 1–12 与本计划 Task 0–19 均按 TDD 实际执行，RED/GREEN 证据可追溯。
- 普通用户使用独立应用外壳；`/admin` 仍是管理后台，不承担成员工作台。
- 七类主/扩展模型、手工录入、导入、编辑、软删除、批量、版本、审计和范围控制全部可用。
- 已审核或锁定数据不能直接回滚；确认/锁定数据不能被普通编辑、删除或批量覆盖。
- 部门状态机、事务、版本冲突、退回原因和实时五状态看板通过成功与失败路径验证。
- 第 4 节五类角色的允许与禁止路径均通过 API 和 Playwright；所有敏感查询在数据库层下推范围。
- 工作台不存在任何阶段 3 通报卡片、接口、占位动作或伪进度。
- 后端完整检查、前端完整检查、PostgreSQL、MinIO 和 Playwright 均实际通过；任何 SKIP/未运行都使完成门禁保持打开。
- 文档、迁移、API、共享类型和错误码一致，无真实个人信息、密钥、调试代码或未经批准的依赖。

## 7. 实施前仍需业务确认的门禁

这些问题不阻止本计划文档定稿，但会阻止对应生产 Task 进入 GREEN；执行时应在最早依赖 Task 的反馈检查点集中确认：

1. 七类检查各自的最终字段、必填规则、单位、合法区间、Excel 列顺序和“有效记录重复键”，尤其是无机课堂、早操早签、晚归与公共寝室卫生的历史口径。
2. 部门提交修订语义：退回后是在同一提交上递增版本再次提交，还是生成新修订；确认后如发现错误，阶段 2 是否只允许走后续更正流程而不重开原提交。
3. 导入回滚产品粒度：本计划采用更安全的“整批、全记录仍为未提交 DRAFT 才允许”基线；若需要选行回滚或提交后撤销，必须另行设计权限、审计与补偿流程。
4. 系统管理员与超级管理员的检查数据默认范围：本计划遵循最小权限，系统管理员不因配置权限自动获得敏感检查明细；超级管理员是否默认拥有全院阶段 2 数据范围需与既有权限基线最终核对。
5. 若仓库没有 Playwright 依赖，是否批准新增锁定版本的开发依赖与配置；未批准时可用既有 Playwright skill 验收，但不能把仓库内 E2E 装配写成已完成。
6. 扩展表物理拆分：现有技术方案允许到课率与无机课堂共用 `attendance_details`，本计划为满足“七类扩展模型/迁移”采用七个显式模型和七张扩展表。Task 2 执行前需确认是保留该显式拆分，还是采用“七个领域模型、六张物理表”的既有方案；不得静默偏离技术方案。

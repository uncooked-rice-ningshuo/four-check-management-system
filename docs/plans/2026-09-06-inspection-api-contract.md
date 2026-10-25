# 七类检查记录 API（Task 4 / Task 5）

> **2026-10-04 最新范围决定（优先于下文旧计划和历史记录）：** 当前后续开发及本批验收仅包含到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归五类。教室卫生、公共宿舍卫生列为待需求方确认，暂停后续开发；保留已有功能、数据及未来汇总通报接入能力。用户已确认成员按批次提交、部门按周汇总，同范围允许多个批次，退回在原批次重提。见[范围决定](2026-10-04-five-inspection-active-scope.md)。下文 Task 4/5 的草稿-only 说明是历史增量；当前接口行为以新增的部门提交流程为准。

> **2026-10-05 到课率学时补充：** 用户确认每节课折算 1 实际旷课学时；教学礼拜周倍数仅用于违纪计入学时。新建、修改的到课率 `details.absenceHours` 必须等于 `details.period` 表示的节数，通用检查 API 不一致时返回 422 `VALIDATION_FAILED`，字段问题为 `details.absenceHours / PERIOD_HOURS_MISMATCH`。旧手工入口与 Excel 导入预览同样校验；导入行以 `INVALID_HOURS` 提示。历史记录读取不自动回填或重算。教室卫生、公共宿舍卫生延期边界不变。

## 2026-10-04 五类部门提交流程增量

`POST /api/v1/department-submissions` 提交一个成员批次：请求为 `{scope:{semesterId,weekId,departmentId,inspectionTypeId},records:[{id,version}],idempotencyKey}`，记录必须同范围、未提交且为 DRAFT，最多 100 条；相同成员和键的同请求返回原批次，不同内容返回 `IDEMPOTENCY_CONFLICT`，重试仍检查当前记录授权。同范围允许多个批次。`GET /api/v1/department-submissions` 按学期、周、部门、类型、状态和分页查询；`GET /api/v1/department-submissions/:id` 返回完整批次。列表、详情只有批次内每条记录均在有效查看/录入/审核范围内才返回。响应含批次状态与版本、修订号、记录 ID 和当前记录版本、学期/周次/部门展示名；读取接口严格要求完整响应。

`POST /api/v1/department-submissions/:id/return` 与 `/confirm` 接收 `{version,reason}`（退回原因必填），只允许 PENDING_REVIEW，由审核权限覆盖全部记录，事务内更新记录、批次和审计。`POST /api/v1/department-submissions/:id/resubmit` 接收 `{version,records:[{id,version}]}`，只允许 RETURNED 且必须包含原批次全部记录及最新版本，重提修订号递增。退回记录可经已有编辑 API 在原提交范围内更正，不允许删除；已确认、待审核及锁定记录仍不可编辑。版本或非法状态分别返回稳定的 `VERSION_CONFLICT` / `INVALID_SUBMISSION_STATE`。详情与操作经后端鉴权，页面按钮显隐仅作体验提示。迁移 016、017 尚未加载目标业务库；早签隔离浏览器链路已通过，完整角色验收仍待完成。

> 2026-09-20 核对：本文描述已实现 API；其余六类页面、卫生区域管理及导入仍缺失，当前进度见[项目状态](../project-status.md)。09-20 已修正字段错误的顶层响应并补前端兼容；部署及浏览器验收另计。

## 交付范围

七类统一手工创建、分页列表、详情、草稿编辑、单条软删除与最多 100 条批量软删除。到课率与无机课堂共用课程扩展表，其他类型使用 012 迁移的明确扩展表。应用服务编排事务，主表、明细、责任快照及审计同事务提交。

本增量不包含其余六类页面、Excel 适配器、卫生区域管理页面或部门提交流程。RETURNED、PENDING_REVIEW、CONFIRMED、LOCKED，以及有关联提交/通报锁的记录都不能由本接口修改。退回记录修正仍依赖部门流程，不提前开放。

## HTTP 契约

| 动作 | 路径 | 成功响应 |
| --- | --- | --- |
| 创建 | POST /api/v1/inspections | 201，统一记录 |
| 列表 | GET /api/v1/inspections | 200，items,total,page,pageSize |
| 详情 | GET /api/v1/inspections/:id | 200，统一记录 |
| 编辑 | PATCH /api/v1/inspections/:id | 200，版本递增后的统一记录 |
| 单删 | DELETE /api/v1/inspections/:id | 204 |
| 批删 | POST /api/v1/inspections/batch | 204 |

创建体为 `semesterId,weekId,inspectionTypeId,departmentId,details`。四个标识为 UUID；details 按七类字段目录传入，不接受客户端提供快照、状态、来源或创建人。服务端根据 inspectionTypeId 解析类型，根据已启用的校历及部门类型映射解析工作上下文。请求体上限 64 KiB，未知字段和尾随 JSON 拒绝。

编辑体为 `version,reason,record`，record 与创建体相同。必须传当前正整数版本与非空原因（最多 255 字）；允许改业务内容及授权目标范围，类型不可更换。来源、导入批次、创建人、创建时间保持不变。删除体为 `version,reason`；批删体为 `action:"DELETE",items:[{id,version}],reason`，不能重复 ID，大小为 1–100。

统一记录包含 id、学期/周/类型/部门标识、typeCode、明确结构的 details、status、source、importBatchId（可选）、rowVersion、createdBy、createdAt、updatedAt，不暴露重复键和内部提交/锁标识。卫生记录的 details 返回责任 UUID，顶层 responsibleClasses / responsibleStudents 返回已保存的责任名称快照；只有整条记录通过全部责任范围鉴权后才读取。历史详情从保存的明细和快照读取，不使用当前基础数据覆盖历史字段。

列表支持 semesterId、weekId、inspectionTypeId、departmentId、classId、grade、status、source、page、pageSize、sort。默认 page=1、pageSize=20，最大 pageSize=100；排序为 occurredAtAsc / occurredAtDesc，日期/轮次记录按业务日排序，UUID 作为稳定次序。列表与详情采用一致数据库快照；total 与 items 使用相同权限谓词。

当前稳定错误码包括：400 INVALID_REQUEST；401 未认证；403 FORBIDDEN_SCOPE / PASSWORD_CHANGE_REQUIRED；404 INSPECTION_NOT_FOUND（不存在与范围外统一）；409 VERSION_CONFLICT / RECORD_NOT_EDITABLE / RECORD_ALREADY_EXISTS；422 VALIDATION_FAILED；500 INSPECTION_FAILED。字段校验 422 使用顶层 `{code,message,details}`，details 为安全的 path/code/message 数组；其他分支使用顶层 `{code,message}`。前端 ApiError 保留 details，并兼容旧服务 `{error:{code,message,details}}`；异常形状退回安全默认提示，不抛出二次解析异常。09-20 已有 Go/TypeScript 回归验证。

## 权限与关联

列表/详情使用 INSPECTION_VIEW。本人未提交草稿可用 INSPECTION_ENTER 修改，他人草稿需 INSPECTION_MANAGE；各动作 ALLOW/DENY 独立计算。创建始终要求 INSPECTION_ENTER，管理权不隐含录入权。

个人记录使用主表班级/年级快照；卫生使用责任班级关系。SQL 对卫生记录逐个责任班级检查有效授权，只返回全部责任范围均可见的完整记录，不返回部分授权记录的其他责任信息。修改同时检查原记录与新目标的全部责任范围。超级管理员使用已有全院权限语义。

事务内锁定并校验校历、部门类型映射、启用状态、班级/年级、学生班级归属、课程班级及该学期教室排课、宿舍层级、区域所属对象。晚归宿舍需匹配学生当前宿舍；公共区域责任寝室必须位于该区域宿舍楼。学生查询在 SQL 中限定到已授权责任班级。区域关联和责任快照写入失败时全部回滚。

卫生原因码使用现有字典配置能力：字典类型 code 为 `<inspectionTypeCode>_REASON`，例如 `DORMITORY_HYGIENE_REASON`；类型和条目均须启用。未配置的原因明确拒绝，不内置历史材料中的原因枚举。区域 UUID 来自 012 区域表，管理 UI 尚待后续实现，不伪造默认区域。

字段规则沿用 Task 3。课程学时额外限定最多两位小数，与 numeric(6,2) 存储一致，防止静默舍入。重复记录报错，结果/责任变更走版本编辑，同日复查通过时间或轮次区分。

批量按 UUID 固定顺序锁定，全部验证后修改；版本条件、原范围、DRAFT 及无提交/锁约束再次进入 UPDATE。审计记录动作、前后业务值、版本、操作人、原因、时间；个人姓名/学号不进入审计快照，自由文本字段脱敏。审计失败会撤销创建、编辑和删除。

## 到课率调用方迁移

此前 `/inspections/:id` 返回到课率平铺详情，本次改为统一结构。现有页面已切换到 `/inspections/attendance/:id` 的 GET/PATCH/DELETE，以及 `/inspections/attendance/batch`；这些专用接口保留原平铺结构与编辑权限规则。原有到课率创建/列表/导入路径不变。外部调用方如使用旧的平铺详情契约，需要同步切换专用路径。

字段目录的 availableActions 仍表示已有页面闭环能力；当前到课率、无机课堂、早锻炼/早签、晚归和宿舍卫生有页面，教室卫生与公共宿舍卫生页面尚未交付。这不表示通用 API 不可用。除到课率外的导入模板仍为 PLANNED，不能据 API 完成就开放导入。

## 验证

`INSPECTION_TEST_DATABASE_URL` 必须指向一次性空 PostgreSQL 库；测试发现 accounts 表时拒绝修改。运行：

```bash
cd server
GOTOOLCHAIN=go1.25.0 go test ./internal/inspection ./internal/httpapi ./internal/store
INSPECTION_TEST_DATABASE_URL='<一次性空库连接串>' GOTOOLCHAIN=go1.25.0 go test -race ./internal/store -run TestInspectionPostgresCRUD -count=1 -v
```

实际临时库验证包括七类 CRUD、重复键、软删除后重建、个人/卫生范围、原目标范围、独立管理权及 DENY、关联错误、双连接版本竞争、审计失败回滚和保护状态。未将本测试连接到业务数据库。

## 09-21 展示快照增量

通用创建、详情、列表及编辑响应新增顶层字符串字段 `studentNumber`、`studentName`、`className`、`grade`，来源为记录中已持久化的学生/班级快照。客户端不能写入这些字段；历史空快照及使用多责任对象的卫生记录返回空字符串，不以当前基础数据补写历史值。这些字段不代表课程/教室名称或卫生责任对象展示已交付。

权限过滤仍先于读取；未新增权限或迁移。现有前端尚无此通用 API 的页面调用方，到课率专用接口不变。普通成员关联选项、当前名称及无机课堂/晚归页面已补，详见后续契约与项目状态。真实 PostgreSQL 验证创建/详情/列表字段以及学生改名后快照保持，见[验收记录](../reviews/2026-09-21-inspection-display-snapshots.md)。

## 普通录入选项

2026-10-04：旧到课率 `filter-options` 的每类选项改为复用 SQL 授权的 `context-options purpose=view`，执行学年范围及 DENY；保持旧响应结构。`context-options` 班级项可返回可选字符串 `grade`，仅用于当前年级显示，不改变历史快照授权；旧客户端可忽略，客户端不得接受非字符串值。分页异常必须失败，不能显示正常空数据。详见[验收记录](../reviews/2026-10-04-attendance-filter-scope.md)。

`GET /api/v1/inspections/entry-options`：必填 `semesterId,weekId,inspectionTypeId,departmentId,kind`；到课率/无机课堂的 kind 为 classes/students/courses/classrooms；晚归为 classes/students/dormitories；早锻炼/早签为 classes/students；宿舍卫生为 classes/students/dormitory-objects/reason-codes；教室卫生为 classes/students/classroom-objects/classroom-areas/reason-codes；公共宿舍卫生为 classes/students/public-areas/responsible-dormitories/reason-codes。学生/课程另传 classId，普通教室另传 classId/courseId；晚归宿舍另传 classId/studentId 且仅返回该学生当前分配的启用宿舍。三类卫生的业务对象和原因码另传 classId，以授权责任班级限定选项；教室卫生区域另传 classroomId，公共宿舍卫生责任寝室另传 areaId，且只返回区域所属楼栋启用寝室。宿舍卫生寝室对象可与当前学生宿舍分配无关，仍须有启用的宿舍区/楼栋/寝室。编辑传 recordId。其他类型或类型与种类不匹配返回 400 INVALID_REQUEST。分页 page/pageSize 默认 1/20，pageSize 最大 200。响应 `{items:[{id,label}],total,page,pageSize}`，客户端须处理全部分页。

范围在 SQL 内执行 ENTER 的 ALLOW 扣 DENY；编辑先核验原草稿，再按其所有者评估 ENTER/MANAGE。管理权限不能作为新建权限。无匹配授权返回空页；原记录不可见返回 404，非草稿/已提交/已锁定返回 409；配置失效为 422，未知故障 500。详见[范围选项验收](../reviews/2026-09-21-inspection-entry-options.md)。此端点不替代列表查看范围筛选项。

### 当前课程/教室/宿舍名称（09-21 增量）

通用记录响应包含 `currentCourseName`、`currentClassroomName`、`currentDormitoryName`；三类卫生另含 `currentAreaName`（适用区域类型）和 `currentReasonLabel`，公共宿舍卫生可含 `responsibleDormitories:[{id,label}]`。创建/详情/列表/编辑一致。这些关联基础数据、区域或原因字典名称是当前名称，非历史快照，不用于定稿；无关联返回空串，引用对象停用不抹去名称。学生/班级及卫生责任班级/学生名称使用已保存快照；责任寝室只有当前名称，定稿须另行固化。

### 页面上下文选项（09-21）

`GET /api/v1/inspections/context-options`：详见[契约及验证](../reviews/2026-09-21-inspection-context-options.md)。支持七类检查。查看与录入目标分开，不提供当前周默认值，也不开放管理接口。三类卫生历史记录的可查看上下文按已保存的责任班级范围投影，不依赖主表单个 class_id。

通用记录另提供 `canEdit` 提示，由完整范围权限、所有权及草稿/无提交/无锁状态计算；前端按钮使用该提示，实际编辑/删除仍重新检查权限、状态和版本。上下文选项 `purpose=entry` 支持 `recordId`，仅在原记录可编辑且类型一致时提供其目标范围，新增仍不能使用 MANAGE。

## 2026-10-04 卫生区域维护 API 增量

区域管理权限与录入选项权限分开。`MASTER_DATA_MANAGE` 的全局授权仍可管理基础数据；仅有部门/检查类型范围授权的成员只能访问区域专用接口，不开放通用 `/admin/master-data`。查询与每次写入均重新核验启用的部门/检查类型映射及有效权限（DENY 优先）。

- `GET /api/v1/admin/hygiene-areas/options`：已登录、无需首次改密的用户可查询自身维护范围，返回 `{kind,departmentId,departmentName,inspectionTypeId,inspectionTypeName}[]`；无权时返回空数组，基础服务异常不能伪装为空数组。
- `GET /api/v1/admin/hygiene-areas/:kind/references?departmentId=…&page=1`：明确目标范围鉴权后返回启用教室或宿舍楼；响应 `{items,total,page,pageSize}`，每页 100，客户端加载全部分页并拒绝重复/不完整响应。
- `GET /api/v1/admin/hygiene-areas/:kind?departmentId=…&page=1&pageSize=20`：区域列表，复用 masterdata 分页模型。
- `POST /api/v1/admin/hygiene-areas/:kind?departmentId=…`：新增；`PUT /api/v1/admin/hygiene-areas/:kind/:id?departmentId=…`：编辑及启停。必填 `name`，教室区域必填 `classroomId`；公共宿舍区域必填 `dormitoryBuildingId,floorUnit`。UUID 由服务端生成，关联对象必须有效。
- `GET /api/v1/admin/hygiene-areas/:kind/:id/changes?departmentId=…`：区域变更历史。写入沿用 masterdata 的变更记录和审计事务。

`:kind` 仅允许 `classroom-hygiene-areas` 和 `public-dormitory-hygiene-areas`。无身份 401、越权 403、无效目标/分页 400；内部错误保持安全的稳定错误码。区域是供相应类型使用的共享基础配置，不新增未经确认的区域所有权字段。维护目标用于校验配置权限，不改变检查记录的部门归属。

首轮验证与剩余边界见[区域维护记录](../reviews/2026-10-04-hygiene-area-maintenance.md)，本节不代表业务环境已加载。


## 2026-10-04 整批导入回滚增量

`POST /api/v1/imports/:id/rollback` 请求 `{ "version": 4, "reason": "核对误导入" }`；成功 200 返回 `{ "batchId": "…", "status": "ROLLED_BACK", "rolledBackRows": 2, "version": 5 }`。

仅已确认批次可整批执行；每条记录必须存在、仍为 DRAFT、未关联提交/通报锁、未删除且 rowVersion=1（确认后未经编辑）。批次和全部记录按固定顺序加锁，按同现有记录修改的本人 ENTER / 范围 MANAGE 权限重新校验，全部责任范围均须授权，DENY 优先。软删除记录、递增版本、批次状态和审计为同一事务；保留 importedRows、原文件、解析行及业务详情。不提供选行回滚，不将取消批次混同回滚。

输入错误 400 `INVALID_IMPORT_REQUEST`；未认证 401；无可访问批次 404 `IMPORT_BATCH_NOT_FOUND`；部分记录越权 403 `FORBIDDEN_SCOPE`；版本冲突 409 `VERSION_CONFLICT`；非确认状态 409 `INVALID_BATCH_STATE`；记录不再满足整批条件 409 `IMPORT_ROLLBACK_UNSAFE`；内部异常 500 `IMPORT_SERVICE_UNAVAILABLE`。重复旧版本请求明确冲突，不会再次删除或追加成功审计。014 迁移在一次性 PostgreSQL 验证，目标环境未应用。

## 2026-10-04 无机课堂导入

`GET /api/v1/imports/course-records/template` 与 `POST /api/v1/imports/course-records` 使用既有到课率模板下载/上传的范围、模板版本及 multipart 契约，仅支持 `ATTENDANCE_RATE`、`UNATTENDED_CLASSROOM`。既有 `/imports/attendance` 入口维持到课率类型边界。无机课堂模板标识为 `UNATTENDED_CLASSROOM_IMPORT`，sheet 为“无机课堂导入”；范围取数据库批次，不能通过文件伪装其他检查类型。模板代码与入口不匹配返回 422 `TEMPLATE_VERSION_MISMATCH`，错误业务工作表返回 422 `INVALID_WORKBOOK`。XLSX 公式、隐藏业务工作表和额外非空数据列被拒绝。

批次读取、预览行、确认、取消和回滚继续使用 `/imports/:id/...`，继承服务端范围权限、版本、幂等和审计校验。仅两类课程模板已实现；实际模板是否发布仍由目标环境配置决定，不能把字段目录 ACTIVE 作为业务库已初始化的证据。无机课堂记录 DTO 可选 `importBatchId` 用于批次重开，未提供时不展示入口。

### 确认时基础数据变化

`POST /imports/:id/confirm` 在批次事务中重新检查并锁定当前有效基础数据、课程关联和录入范围；预览后的停用、学生跨班、年级或周次范围变化导致 409 `IMPORT_PREVIEW_STALE`。失败保留预览/批次版本且无部分写入。前端提示“基础数据或检查范围已变化，请重新上传并校验后确认”。恢复原关联后可重试；已确认的同幂等键请求继续返回原结果。

预览后出现重复的有效检查记录时，确认返回 409 `RECORD_ALREADY_EXISTS`，整批不产生部分记录；只有已知去重约束映射为此错误，未知数据库故障仍返回内部错误。前端提示修正文件并重新上传校验。

## 导入取消审计补强（2026-10-04）

`POST /imports/:id/cancel` 请求/响应格式不变。服务端身份元数据传入取消应用服务；版本/允许状态复验、取消状态和 `IMPORT_CANCELLED` 审计同事务提交。审计失败不改变批次，可原版本重试；并发同版本只有一个成功，其余 `409 VERSION_CONFLICT`。保留原文件、解析预览与批次。必须改密账号在访问批次前被拒绝，真实内部读取错误返回安全500，非超级管理员按服务端学期所属学年补全权限范围。

## 宿舍卫生导入增量（2026-10-04）

通用 `/imports/inspections` 接受 `DORMITORY_HYGIENE_IMPORT` v1；到课率/课程入口仍拒绝。十列对象模板使用完整名称、多责任分号列表、日期+时间或轮次、互斥结果与字典原因；预览沿用 `normalizedInspection` schemaVersion1，确认按多责任关系重校验，空学生合法而空班级不合法。可选 importBatchId 用于页面重开原批次。具体字段与实际验证见[宿舍卫生导入验收](../reviews/2026-10-04-dormitory-hygiene-import.md)。目标环境模板是否发布需单独核对。

### 2026-10-04 导入目标班级、年级

`GET /api/v1/inspections/context-options` 新增 `kind=grades`，返回年级名称作为 `id/label`；可选 `grade`（去除首尾空白后最多64字符）约束班级或其他上下文候选。`purpose=entry` 的年级、班级选项要求学期和周次，与既有选项一样在SQL按有效录入权限过滤，停用年级不参与新目标。

导入批次响应含可选 `classId`、`grade`。前端新导入独立选择目标，不能把列表班级/年级默认带入；模板和上传请求必须携带选中的范围。恢复和重传沿用批次范围，服务端权限仍权威校验，不扩大限定角色权限。

### 2026-10-04 教室卫生对象导入

`CLASSROOM_HYGIENE` 使用 `/api/v1/imports/inspections`，模板编码 `CLASSROOM_HYGIENE_IMPORT`、版本1、工作表“教室卫生导入”。字段顺序为 `classroom_name,area_name,inspection_date,inspection_time,inspection_round,result,responsible_class_names,responsible_student_nos,reason_name,description,remark`。完整教室名称由教学楼与教室号组成，区域按该教室解析；重复名称或非所属区域拒绝，不能使用另一教室的同名区域代替。

结果、日期、时间/轮次及责任口径沿用D3和既有领域校验。预览与确认检查所有责任班级，确认锁定区域/教室/原因/责任引用；区域停用或改归属返回409 `IMPORT_PREVIEW_STALE`，不得部分入库。安全回滚仍要求整批全部为未变更、未提交、未锁定草稿。目标业务环境模板未发布。

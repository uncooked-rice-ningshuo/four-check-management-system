# 阶段 2 到课率与通用 Excel 导入设计

**日期：** 2026-08-30  
**状态：** 已确认  
**阶段范围：** 阶段 2 第一增量——通用导入框架 + 到课率检查业务

## 1. 目标与边界

本增量交付一个可复用于七类检查业务的 Excel 两阶段导入框架，并以“到课率”完成首个端到端业务适配：模板下载、上传解析、错误预览、确认入库、批次查询和到课率记录列表。

本增量不交付部门提交/审核、导入后回滚、其余六类检查、统计与通报。这些能力仍属于阶段 2 后续增量或阶段 3。导入成功的数据统一进入 `DRAFT`，不会自动提交部门审核。

## 2. 关键决策

- Excel 导入仍位于到课率详情页的抽屉中，不新增一级“导入”导航。
- 采用“解析预览”和“确认入库”两阶段流程。解析阶段只写导入批次与行，不写检查业务表。
- 模板使用稳定的业务编码和显式版本号；中文表头用于用户阅读，后端以模板版本和预期表头共同校验。
- 首期仅接受无宏 `.xlsx`，拒绝 `.xls`、`.xlsm` 和其他压缩包；限制文件大小、ZIP 解压规模、工作表数量和 1000 行业务数据。
- 原始 Excel 先写入私有 MinIO，数据库只保存文件元数据、SHA-256 与关联关系。下载必须按关联模板/批次重新鉴权。
- 到课率保存个人旷课明细，不在单条记录中保存应到人数、实到人数或到课率等统计结果。
- 预览中任一行有错误时不允许确认整个批次。本增量不实现“仅导入通过行”，避免同一文件被部分落库后难以核对；后续如启用部分导入，需作为显式产品规则另行确认。
- 新 Excel 处理库属于运行时依赖，开始对应实现任务前必须单独获得引入许可。

## 3. 架构与数据流

后端新增两个模块：

- `inspection`：检查主记录、到课率扩展、列表查询和业务不变量。
- `importjob`：模板定义、批次状态机、XLSX 安全解析、通用行校验编排与确认幂等。

调用方向保持：

```text
HTTP Handler
  ├─ inspection.Application Service → inspection.Repository
  └─ importjob.Application Service
       ├─ XLSX Parser
       ├─ Attendance Adapter → master data/reference lookup
       ├─ ObjectStorage/File metadata service
       └─ importjob.Repository → transaction → inspection tables + audit log
```

导入状态由后端显式控制：

```text
UPLOADED → PARSING → PREVIEW_READY → CONFIRMING → CONFIRMED
              └──→ PARSE_FAILED        └──→ CONFIRM_FAILED
UPLOADED/PREVIEW_READY/PARSE_FAILED → CANCELLED
```

状态改变同时校验当前状态和 `version`。确认接口还校验幂等键；重复请求返回同一确认结果，版本冲突返回 `BATCH_VERSION_CONFLICT`，不会重复写业务记录。

## 4. 数据模型

新增只追加迁移 `005_phase_two_attendance_import.sql`。

### 4.1 模板与批次

`import_templates`

- `id`, `code`, `name`, `inspection_type_id`
- `version_no`, `file_id`, `field_definition_json`
- `status`：`DRAFT/PUBLISHED/RETIRED`
- `published_at`, `created_by`, `created_at`, `updated_at`
- 同一检查类型和版本唯一；同一检查类型最多一个 `PUBLISHED` 当前版本

`import_batches`

- `id`, `inspection_type_id`, `semester_id`, `week_id`, `department_id`
- `original_file_id`, `template_id`, `template_version`
- `status`, `total_rows`, `valid_rows`, `invalid_rows`, `imported_rows`
- `version`, `confirm_idempotency_key`, `failure_code`, `failure_message`
- `created_by`, `created_at`, `updated_at`, `confirmed_at`, `cancelled_at`

`import_rows`

- `id`, `batch_id`, `row_no`
- `raw_data_json`, `normalized_data_json`
- `validation_status`：`VALID/INVALID`
- `errors_json`, `record_id`, `created_at`
- `(batch_id, row_no)` 唯一

### 4.2 检查业务

`inspection_records`

- 公共上下文：`semester_id`, `week_id`, `inspection_type_id`, `department_id`
- 对象与组织：`student_id`, `class_id`
- 业务时间：`occurred_at`
- 追踪字段：`source`, `status`, `import_batch_id`, `created_by`, `created_at`, `updated_at`
- 流程保护：`submission_id`, `locked_by_report_id`, `row_version`, `deleted_at`

`attendance_details`

- `record_id`（PK/FK）
- `course_id`, `absence_hours`
- `period_text`, `classroom_id`

到课率首期重复键为：同一学期、周次、检查类型、学生、课程、发生时间及节次。应用层先返回行级 `ROW_DUPLICATED`/`RECORD_ALREADY_EXISTS`，数据库对未删除记录使用唯一索引兜底。

## 5. 模板与行规则

到课率模板编码 `ATTENDANCE_IMPORT`，首版字段：

| 字段编码 | 中文表头 | 必填 | 规则 |
| --- | --- | --- | --- |
| `student_no` | 学号 | 是 | 学生存在且启用 |
| `student_name` | 姓名 | 是 | 与学号对应姓名一致 |
| `class_name` | 班级 | 是 | 班级存在且与学生当前班级一致 |
| `course_name` | 课程 | 是 | 课程存在且启用 |
| `absence_date` | 旷课日期 | 是 | 合法日期且位于所选周次 |
| `absence_time` | 旷课时间 | 是 | 合法时间 |
| `period` | 节次 | 是 | 非空、长度受限 |
| `absence_hours` | 旷课学时 | 是 | 大于 0 的有限小数 |
| `classroom_code` | 上课地点 | 否 | 填写时教室必须存在且启用 |
| `remark` | 备注 | 否 | 长度受限，展示时按普通文本处理 |

通用解析器校验扩展名、ZIP 文件签名、MIME、加密/宏、压缩条目与解压总量、工作表数量、模板版本、表头、空行和行数。业务适配器校验字段格式、引用一致性、批内重复与库内重复。

稳定错误码至少包括：

- 文件级：`INVALID_FILE_TYPE`、`INVALID_XLSX_SIGNATURE`、`FILE_TOO_LARGE`、`XLSX_ARCHIVE_LIMIT_EXCEEDED`、`TEMPLATE_VERSION_MISMATCH`、`INVALID_HEADER`、`ROW_LIMIT_EXCEEDED`
- 行级：`REQUIRED_FIELD_MISSING`、`INVALID_FIELD_FORMAT`、`REFERENCE_NOT_FOUND`、`REFERENCE_MISMATCH`、`ROW_DUPLICATED`、`RECORD_ALREADY_EXISTS`
- 并发/状态：`INVALID_BATCH_STATE`、`BATCH_VERSION_CONFLICT`、`BATCH_NOT_CONFIRMABLE`

## 6. API 与权限

接口前缀 `/api/v1`：

- `GET /inspection-types/:inspectionTypeId/import-template`：下载当前模板
- `POST /inspection-types/:inspectionTypeId/import-batches`：上传、保存原文件并解析
- `GET /import-batches/:batchId`：批次摘要
- `GET /import-batches/:batchId/rows`：分页预览，支持 `validationStatus`
- `POST /import-batches/:batchId/confirm`：携带 `version` 和 `Idempotency-Key` 确认
- `POST /import-batches/:batchId/cancel`：取消未确认批次
- `GET /inspection-records/attendance`：按学期、周次、班级、关键词分页查询到课率记录

权限在后端重复执行：

- 模板、批次、预览和记录查看/下载：`INSPECTION_VIEW`
- 上传、解析、确认和取消：`INSPECTION_ENTER`
- 数据范围必须同时与学期、周次、检查类型、部门、班级和年级约束取交集，并下推到仓储查询。

批次 ID 和文件 ID 都不是授权凭证。每次按 ID 读取、预览、确认、取消或下载时，重新读取关联上下文并做动作权限与范围校验。

## 7. 前端体验

新增“日常检查业务”入口和到课率详情页。页面顶部提供学年/学期/周次上下文，操作区包含“批量导入”和“下载导入模板”，记录列表包含加载、空数据、失败、无权限和分页状态。

导入使用右侧 Drawer：

1. 下载模板与选择上下文。
2. 上传 `.xlsx`，显示文件名和安全限制。
3. 展示解析进度与预览摘要。
4. 按全部/通过/失败筛选行，错误显示行号、字段、稳定错误文案和修复建议。
5. 零错误时进入确认；确认弹窗明确显示批次、周次、记录数和“进入录入草稿”。
6. 成功后刷新到课率列表并保留批次结果；刷新页面后可以通过批次 ID 恢复预览。

前端只负责操作提示和按钮显隐，不自行推断权限或解析后端错误字符串；稳定错误码统一映射为中文可操作提示。

## 8. 事务、审计与恢复

- 上传成功/失败、解析结果、确认成功/失败、取消和文件读取均记录审计；审计不保存完整原始行、上传文件内容或敏感请求体。
- 确认在一个 PostgreSQL 事务中完成：锁定批次与版本 → 再校验状态/幂等 → 插入检查主表与扩展表 → 回填行记录 ID → 更新批次 → 写成功审计 → 提交。
- 任一业务记录或审计写入失败时整笔回滚，批次保持可诊断状态；不会出现部分导入成功。
- MinIO 上传与数据库不能形成分布式事务，沿用阶段 1 的 `PENDING/AVAILABLE/FAILED/MISSING` 文件恢复策略；只有 `AVAILABLE` 且完整性校验通过的原文件可以解析。

## 9. 测试与验收

- 解析器单元测试：合法黄金样例、伪装扩展名、宏、损坏 ZIP、超限压缩、错误模板/表头、空行与 1000 行边界。
- 到课率适配器单元测试：引用匹配、周次日期、数字边界、批内/库内重复。
- 服务测试：状态机、权限/范围拒绝、版本冲突、幂等确认、事务回滚、审计脱敏。
- API 测试：HTTP 状态、稳定错误码、分页结构、未登录/越权/跨部门访问。
- 前端测试：权限动作、上传限制、解析状态、错误筛选、确认禁用、冲突与失败提示、成功刷新。
- PostgreSQL/MinIO 集成测试：迁移、原文件关联、确认原子性、缺失对象拒绝。
- Playwright 验收：模板下载 → 上传 → 预览 → 确认弹窗 → 列表出现草稿记录，并覆盖无权限和错误文件。

## 10. 后续衔接

本增量稳定后，阶段 2 后续工作按同一主表和导入框架增加其余六类适配器、手工录入、批量删除/回滚、部门提交与审核状态机。统计分母和通报快照留到已确认口径后的阶段 3，不提前写入明细模型。

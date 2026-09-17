# 七类检查记录 API（Task 4 / Task 5）

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

统一记录包含 id、学期/周/类型/部门标识、typeCode、明确结构的 details、status、source、importBatchId（可选）、rowVersion、createdBy、createdAt、updatedAt，不暴露重复键和内部提交/锁标识。返回责任人的 UUID，不额外返回姓名或学号。历史详情从保存的明细读取，不使用当前基础数据覆盖历史字段。

列表支持 semesterId、weekId、inspectionTypeId、departmentId、classId、grade、status、source、page、pageSize、sort。默认 page=1、pageSize=20，最大 pageSize=100；排序为 occurredAtAsc / occurredAtDesc，日期/轮次记录按业务日排序，UUID 作为稳定次序。列表与详情采用一致数据库快照；total 与 items 使用相同权限谓词。

错误使用统一 error 对象：400 INVALID_REQUEST；401 未认证；403 FORBIDDEN_SCOPE / PASSWORD_CHANGE_REQUIRED；404 INSPECTION_NOT_FOUND（不存在与范围外统一）；409 VERSION_CONFLICT / RECORD_NOT_EDITABLE / RECORD_ALREADY_EXISTS；422 VALIDATION_FAILED；500 INSPECTION_FAILED。领域校验的 422 附带 error.details，包含稳定 path、code、中文 message，不回显个人原始输入。

## 权限与关联

列表/详情使用 INSPECTION_VIEW。本人未提交草稿可用 INSPECTION_ENTER 修改，他人草稿需 INSPECTION_MANAGE；各动作 ALLOW/DENY 独立计算。创建始终要求 INSPECTION_ENTER，管理权不隐含录入权。

个人记录使用主表班级/年级快照；卫生使用责任班级关系。SQL 对卫生记录逐个责任班级检查有效授权，只返回全部责任范围均可见的完整记录，不返回部分授权记录的其他责任信息。修改同时检查原记录与新目标的全部责任范围。超级管理员使用已有全院权限语义。

事务内锁定并校验校历、部门类型映射、启用状态、班级/年级、学生班级归属、课程班级及该学期教室排课、宿舍层级、区域所属对象。晚归宿舍需匹配学生当前宿舍；公共区域责任寝室必须位于该区域宿舍楼。学生查询在 SQL 中限定到已授权责任班级。区域关联和责任快照写入失败时全部回滚。

卫生原因码使用现有字典配置能力：字典类型 code 为 `<inspectionTypeCode>_REASON`，例如 `DORMITORY_HYGIENE_REASON`；类型和条目均须启用。未配置的原因明确拒绝，不内置历史材料中的原因枚举。区域 UUID 来自 012 区域表，管理 UI 尚待后续实现，不伪造默认区域。

字段规则沿用 Task 3。课程学时额外限定最多两位小数，与 numeric(6,2) 存储一致，防止静默舍入。重复记录报错，结果/责任变更走版本编辑，同日复查通过时间或轮次区分。

批量按 UUID 固定顺序锁定，全部验证后修改；版本条件、原范围、DRAFT 及无提交/锁约束再次进入 UPDATE。审计记录动作、前后业务值、版本、操作人、原因、时间；个人姓名/学号不进入审计快照，自由文本字段脱敏。审计失败会撤销创建、编辑和删除。

## 到课率调用方迁移

此前 `/inspections/:id` 返回到课率平铺详情，本次改为统一结构。现有页面已切换到 `/inspections/attendance/:id` 的 GET/PATCH/DELETE，以及 `/inspections/attendance/batch`；这些专用接口保留原平铺结构与编辑权限规则。原有到课率创建/列表/导入路径不变。外部调用方如使用旧的平铺详情契约，需要同步切换专用路径。

字段目录的 availableActions 仍表示已有页面闭环能力，其余六类保持空；这不表示本次通用 API 不可用。导入模板仍为 PLANNED，不能据 API 完成就开放其余类型导入。

## 验证

`INSPECTION_TEST_DATABASE_URL` 必须指向一次性空 PostgreSQL 库；测试发现 accounts 表时拒绝修改。运行：

```bash
cd server
GOTOOLCHAIN=go1.25.0 go test ./internal/inspection ./internal/httpapi ./internal/store
INSPECTION_TEST_DATABASE_URL='<一次性空库连接串>' GOTOOLCHAIN=go1.25.0 go test -race ./internal/store -run TestInspectionPostgresCRUD -count=1 -v
```

实际临时库验证包括七类 CRUD、重复键、软删除后重建、个人/卫生范围、原目标范围、独立管理权及 DENY、关联错误、双连接版本竞争、审计失败回滚和保护状态。未将本测试连接到业务数据库。

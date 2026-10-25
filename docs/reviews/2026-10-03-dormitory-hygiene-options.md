# 宿舍卫生页面前置契约验收（2026-10-03）

## 代码范围

- 允许宿舍卫生使用检查上下文选项。录入目标与查看范围分开；历史对象记录按其已保存的全部责任班级投影查看上下文。
- 录入选项新增 `dormitory-objects` 和 `reason-codes`，均要求明确的学期、周次、检查类型、部门和一个授权责任班级。前者分页返回完整层级寝室名称，后者只返回该检查类型启用的原因字典条目。原晚归 `dormitories` 仍要求学生和班级，限定为学生当前宿舍。
- 通用卫生记录创建、详情、列表和编辑返回已保存的责任班级及学生名称快照；原有 `currentDormitoryName` 提供寝室当前层级名称。所有责任明细只在整条记录通过后端范围鉴权后读取，前端显隐不作为安全边界。

## 验证

- `go vet ./...`、`go test ./...` 通过（默认外部服务用例可能 SKIP）。
- `python3 server/scripts/verify-integration.py` 在一次性 PostgreSQL 和独立 MinIO 上四包均退出 0，报告位于 `/var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-3x4pm150`；其中 `TestInspectionPostgresCRUD/dormitory_hygiene_scoped_options_and_display` 实际运行并通过。场景覆盖寝室/原因/学生选项、七维 DENY、无关班级、MANAGE 不能新建但可编辑、真实详情的 VIEW 与 DENY、责任快照。
- `git diff --check` 通过。无迁移或依赖变更，未连接业务库。

## 下一项

基于此契约实现宿舍卫生独立页面：结果互斥、检查时间或轮次二选一、不合格原因来自字典、至少一个责任班级、责任学生可空；列表/详情展示寝室和责任名称。需要组件与真实浏览器验证。教室及公共宿舍卫生仍依赖卫生区域配置归属和选项交付。

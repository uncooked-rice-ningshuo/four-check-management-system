# 2026-09-21 早锻炼/早签授权选项

既有单一 MORNING_ATTENDANCE 类型新增页面上下文及录入选项入口；只允许 classes/students，不开放课程、教室和宿舍。复用七维范围SQL、ENTER创建及原记录所有者/草稿/版本的MANAGE编辑边界，无迁移、权限扩张或新业务字段，缺勤次数仍由既有后端固定1。

先添加回归：应用服务测试因类型不支持失败；真实PG `four-check-integration-results-oqg90f76/store.json` 因 morning student options 返回 invalid inspection request 失败。实现后聚焦通过；真实PG/MinIO四包全部退出0，报告 `four-check-integration-results-znxwpxia`（位于 `/var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/`）。覆盖班级/学生有权查询、七维DENY分别扣除、无关班级、禁止其他类型选项、MANAGE不能创建但能提供原记录编辑选项。

命令 `GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py`。修改Go文件已gofmt；`go vet ./... && go test ./...` 通过，日志 `/tmp/four-check-morning-options-go.log`。默认外部用例跳过由本轮隔离测试补验，未影响包有缓存。前端本增量未修改，不重复前一晚归增量全量检查。

模型/迁移：无新增。API：授权选项已验。页面：早锻炼/早签尚未实现。模板/初始配置：未变。真实环境：仅一次性服务，业务环境未替换。角色验收：仓储范围已验，页面矩阵待开发。下一项早锻炼/早签独立手工页面及浏览器回归；阶段总门禁保持OPEN。

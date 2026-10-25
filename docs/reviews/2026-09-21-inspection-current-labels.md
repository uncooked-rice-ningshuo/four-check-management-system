# 2026-09-21 检查记录当前课程与教室名称

新增响应 `currentCourseName`、`currentClassroomName`，明确为当前基础数据名称，不能用于历史快照或定稿。记录鉴权/写入校验后，在同一事务读取名称；没有关联时返回空串，停用对象仍可展示。创建、详情、列表、编辑响应一致；不新增迁移，不改变审计快照。

验证：真实 PostgreSQL 测试先因字段缺失失败（gr2n6r7f），实现后四包通过（acavt0yd）；补课程改名/停用读取后最终四包退出 0（yjxg8mff）。报告均位于临时目录的 `four-check-integration-results-<编号>`。命令为 `GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py`。修改 Go 文件已格式化，`go vet ./...` 和 `go test ./...` 通过；默认外部用例的跳过由隔离套件补验，未影响包有缓存。

模型/迁移：仅响应字段，无迁移。API：已实现。页面：尚未接入。模板/初始配置：未变。真实环境：一次性 PG/MinIO 通过，未更新业务服务。角色验收：已有仓储范围回归通过，页面角色验收待开发。前端运行时代码未变，未重复既有前端全量验证。

下一项是普通成员页面上下文，随后接入无机课堂页面及真实浏览器验收；阶段二总门禁仍 OPEN。

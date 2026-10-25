# 2026-09-21 晚归授权关联选项

## 实现

context-options 开放 LATE_RETURN 的既有查看/录入上下文。entry-options 开放晚归 classes/students/dormitories；宿舍要求 classId/studentId，在授权班级 SQL 内连接学生当前分配宿舍，并要求学生、宿舍、楼栋、区域启用。标签包含区域、楼栋与房间号。晚归不能读取课程/教室选项，其他类型不能读取晚归宿舍选项。原记录编辑鉴权、状态与原所有者范围规则不变，不新增管理权限。

## 验证

- 聚焦单元先红：晚归上下文返回 invalid inspection request；实现后 `go test ./internal/inspection -run 'TestEntryOptions|TestContextOptions' -count=1` 通过。
- 真实 PostgreSQL 先红：`four-check-integration-results-6byrvhjr/store.json` 中 student dormitory 返回 invalid inspection request。
- 隔离脚本 `GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py` 四包退出 0，报告 `four-check-integration-results-kcul_614`。覆盖晚归班级上下文、学生宿舍关联、七维 DENY、无关学生、停用宿舍及类型/选项不匹配。资源为一次性 PostgreSQL 和独立 MinIO，执行后清理。
- 全部修改 Go 文件已 gofmt；`go vet ./... && go test ./...` 通过，日志 `/tmp/four-check-late-options-go.log`；未影响包有缓存，外部用例另由上述一次性环境补验。`git diff --check` 通过。

## 边界

无迁移；API 已实现。晚归页面、宿舍当前名称响应、模板/导入尚未交付；业务环境未加载，晚归浏览器角色验收待页面实现。前端运行时代码本增量无变化，上一个认证缓存增量的 180 项测试证据不能当作晚归页面验收。

下一项补授权读取后的宿舍当前名称展示，再接晚归页面及真实浏览器回归。

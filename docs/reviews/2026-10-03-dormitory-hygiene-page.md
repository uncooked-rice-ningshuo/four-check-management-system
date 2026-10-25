# 宿舍卫生页面增量验收（2026-10-03）

## 实现边界

独立 `/inspections/dormitory-hygiene` 入口、授权侧边栏、ProTable 查询、详情、新增、编辑和批量删除已接入现有七类检查 API。录入按学期、周次、部门、责任班级选范围；寝室对象和不合格原因由后端授权选项提供，责任学生可选。检查时间与轮次二选一；不合格时要求原因字典值。列表与详情展示当前寝室、责任班级/学生和原因名称。服务端同时返回责任名称快照及当前原因标签，权限和数据范围仍由服务端判定。

## 实际验证

- 前端 `pnpm lint`、`pnpm test`（48 文件、220 项）、`pnpm build` 通过；构建仍有大包提示。后端 `go vet ./...`、`go test ./...` 通过；无外部服务配置的默认集成用例可能 SKIP。
- 使用临时本机 PostgreSQL 16 运行 `TestInspectionPostgresCRUD/dormitory_hygiene_scoped_options_and_display`，通过。此前同一组选项在一次性 Docker PostgreSQL 通过；本轮复验包含原因标签。
- `BROWSER_DB_MODE=native BROWSER_SUITE=dormitory-hygiene python3 server/scripts/verify-browser.py` 使用一次性 PostgreSQL、MinIO、真实 API、Vite 和 Chrome 154.0.8037.97，16 组场景通过，浏览器错误 0、清理错误 0。包含创建时间/轮次两种记录、原因必填与字典、详情、编辑生命周期、注入 500 重试、真实 409、422、权限角色、跨部门 404、批删及 390px 表格滚动。报告见 `output/playwright/a45a9bfc32/summary.json`。
- 本机 Docker 后端在 `docker info` 超过 10 秒未响应，故本轮浏览器使用隔离本机 PostgreSQL。首次两轮脚本因精确展示文案断言失败，已按服务端实际组合标签修正；最终完整回归通过。

## 未完成

这只是宿舍卫生的手工草稿页面。目标业务环境仍未加载当前代码，也未执行目标账号验收；该类型的模板/导入、部门审核流程、回滚及通报均未交付。教室卫生和公共宿舍卫生仍无页面；二阶段总门禁保持开放。

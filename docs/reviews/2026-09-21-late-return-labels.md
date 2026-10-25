# 2026-09-21 晚归当前宿舍名称

记录响应新增 `currentDormitoryName`：宿舍区＋宿舍楼＋房间号。已有晚归与寝室卫生直接宿舍引用均适用；无关联返回空串。授权读取或合法写入后在同一事务解析，停用/改名仍展示当前名称，明确非历史快照，不进入审计快照，无迁移。

真实 PostgreSQL 红阶段因字段缺失失败（`four-check-integration-results-_ulszuwr/store.json`）；实现后四包全部退出 0（`four-check-integration-results-zstxpech`），覆盖创建/详情/列表/编辑、改名停用、跨班级拒绝及审计不含名称。报告根目录为 `/var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/`。命令：`GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py`。已 gofmt；`go vet ./... && go test ./...` 通过（`/tmp/four-check-late-labels-go.log`，未影响包有缓存，默认外部用例跳过由隔离套件补验）。

模型/迁移：仅响应字段，无迁移。API：已实现。页面：晚归待实现。模板/初始配置：未变。真实环境：一次性 PG/MinIO 已验，业务服务未替换。角色验收：仓储范围回归已验，晚归浏览器待开发。前端代码未变。阶段总门禁仍 OPEN。

# 2026-09-21 Compose 就绪门禁

Compose API healthcheck 从 /health/live 改为 /health/ready，保留 web depends_on.api.condition=service_healthy。这样启动依赖不再把 API 进程存活当作 PostgreSQL/MinIO 已就绪。README 同步说明：已启动的 web 不会因后续 unhealthy 自动停止；模板与业务初始化仍需单独验收。

验证：修改前断言 readiness 端点存在失败；修改后 `docker compose --env-file /dev/null config --format json` 成功解析，内部断言 API ready 地址及 web service_healthy 通过。命令使用临时虚构环境值满足必填配置，未读取业务 .env、未输出展开配置或启动服务。

`GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache go test ./internal/httpapi -run 'Test(Readiness|Liveness)' -count=1 -v` 实际命中 3 项并全部通过。此前本轮真实 MinIO 暂停/恢复的 ready=503/200 证据见[角色/存储矩阵](2026-09-21-role-storage-matrix.md)。`git diff --check` 通过。未构建/启动 Compose 镜像，不能据此声称目标环境已加载配置。

仅配置和文档变化，不重跑无关完整代码套件。目标环境模板发布/服务更新仍需要具体授权。其余六类页面中，无机课堂已有 D1 确认且不依赖卫生区域配置，可独立推进；卫生区域管理归属、退回修订和回滚语义继续保持待确认。

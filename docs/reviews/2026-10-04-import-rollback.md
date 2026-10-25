# 整批导入安全回滚（2026-10-04）

分支 `feat-0.1-2`，沿用用户确认的整批规则。新增 014 迁移、回滚应用服务/领域校验/事务仓储、`POST /api/v1/imports/:id/rollback`、到课率列表“导入批次”入口和批次抽屉的整批二次确认。

## 不变量

仅 CONFIRMED 批次且全部导入记录仍为 DRAFT、未提交、未锁定、未删除、确认后未经编辑（rowVersion=1）才允许回滚。请求要求批次版本和原因；按本人 ENTER 或范围 MANAGE 在 SQL 下推权限，全部责任班级均须有权，DENY 优先。批次加锁后按记录 ID 顺序加锁，检查记录数和解析行关联，软删除、批次递增版本及逐记录/批次审计同事务。失败不产生部分删除，原文件、批次、解析行、业务详情保留；并发旧版本请求返回冲突。

## 验证

- 领域与 HTTP 聚焦测试通过，覆盖必填/超长原因、非法 UUID、状态、版本、缺记录及 401/403/404/409/500 稳定响应；内部错误不暴露诊断原文。
- 一次性 PostgreSQL 16 执行全部迁移至 014，`go test ./internal/importjob -run TestImportRollbackPostgres -count=1 -race -v` 通过。包含两行真实确认入库、部分班级 DENY、已编辑/提交/锁定/解析关联缺失的零改动拒绝、审计失败事务回滚及两个并发请求仅一次成功；验证删除数、解析链接、原文件元数据、版本及唯一成功审计。测试日志 `/private/tmp/four-check-rollback-result.log`；临时集群自动销毁。
- 后端 gofmt、`go vet ./...`、`go test ./...`、`go test -race ./...` 通过；默认全量包含缓存及外部服务 SKIP，上述 PostgreSQL 单独执行不依赖默认跳过。最终 UUID 规范化后 vet/full 再通过。
- Node 24.12.0 / pnpm 11.7.0：前端聚焦 22 项及缓存聚焦 1 项通过；完整 57 文件/258 项、lint、build 通过。响应数值范围校验补强后客户端聚焦 1 项及 lint 再通过，最终构建包含补强。保留已知 jsdom 样式及分包体积警告。
- `BROWSER_DB_MODE=native BROWSER_SUITE=import-rollback python3 server/scripts/verify-browser.py`：`output/playwright/77cee652d0/summary.json` 12 组通过，Chrome 154.0.8037.97，一次性 PostgreSQL/MinIO/API/Vite。涵盖真实文件上传→预览→确认→从列表重开批次→整批回滚；500 保留原因并重试、成功后列表/批次缓存刷新、重新导入、真实编辑后 409 零删除、401、关闭重开清空原因。browserErrors/cleanupErrors 均为空。

## 边界

当前完整页面链路为到课率；回滚后端以公共主表和责任范围规则实现，其余六类导入尚未交付，不能宣称逐类回滚总门禁通过。014 仅在一次性库应用，目标业务环境未迁移、未加载。未修改业务原始文件，未提交、推送、发布或部署。后续先扩展无机课堂导入，同时等待部门提交分组粒度确认。

# 导入取消审计事务闭环（2026-10-04）

依据导入设计的取消审计要求，实际 PostgreSQL RED 复现取消成功后审计数为 0。现由应用服务接收服务端当前操作者元数据，取消仓储事务锁定批次，重校验版本和允许状态，原子更新取消时间/状态/版本并写入 `IMPORT_CANCELLED` 审计。审计仅含前后状态、版本及标准操作者元数据，保留原文件、解析行及批次。

## 实际验证

- 一次性 PostgreSQL 16、Go 1.25.0：`TestLateReturnImportPostgres -count=1 -race -v` 通过，0 SKIP。
- 注入审计触发器失败：返回错误，状态/版本未变，数据库 cancelled_at 保持 NULL；移除故障后使用原版本可重试。
- 同一批次版本两个 goroutine 并行取消：一项成功、一项版本冲突，仅一条归属正确操作者的审计。再次使用旧版本也冲突。
- 取消后两行解析预览和源文件关联保留。既有晚归确认、重校验、幂等、回滚以及年级预览回归同时通过。
- `go test ./internal/importjob -run TestCancelBatch -count=1`、`go test ./internal/httpapi -run TestImportBatch -count=1` 通过，覆盖状态边界、缺少审计操作者拒绝、HTTP 元数据传递。
- `go vet ./...`、`go test ./...`、`go test -race ./...` 全部通过。默认套件包含缓存及未配置外部环境的 SKIP，实际 PG 回归单列如上。

无迁移或前端运行时代码变更，无业务库写入、源文件删除或部署。目标环境未加载；没有将本项称为全七类、全角色或浏览器总门禁通过。

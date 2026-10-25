# 必须改密账号的导入批次访问边界（2026-10-04）

`authorizedBatch` 原先先加载批次并放行超级管理员，后检查必须改密。HTTP 红绿回归确认超级管理员的批次详情、预览行、确认和取消均返回 200，后两项调用了副作用服务；普通成员虽然被拒绝，仍先调用了批次读取。

修复将必须改密限制放在业务读取与超级管理员快捷放行之前。保持原有隐藏资源语义：404 `IMPORT_BATCH_NOT_FOUND`。八种请求（成员/超级管理员 × 四路由）均验证稳定错误码及读取、预览、确认、取消服务零调用。

实际命令（Go 1.25.0）：

- `go test ./internal/httpapi -run 'TestImportBatchRequiresPasswordChangeBeforeSuperAdminBypass|TestImportBatchByIDReauthorizesAndMapsVersionIdempotencyErrors' -count=1` 通过。
- `go vet ./...`、`go test ./...` 通过；部分默认测试缓存命中、外部环境测试未配置而跳过。

仅服务端鉴权顺序修复，无迁移、前端修改或业务库写入。本项 HTTP 使用真实认证与路由、业务服务计数替身验证不可到达；未宣称真实浏览器或全角色总门禁通过，目标环境未加载。

## 批次读取内部故障分类

另用 `TestImportBatchReadFailureIsNotReportedAsMissing -count=1` 复现底层内部错误原先返回 404。修复复用既有 `writeImportError`：真正不存在仍为 404 `IMPORT_BATCH_NOT_FOUND`，内部错误为 500 `IMPORT_SERVICE_UNAVAILABLE`，响应不包含底层诊断。日志只保留方法、路由模板及错误类型，不输出 SQL、凭据或业务行。聚焦红绿回归与最终 `go vet ./...`、`go test ./...` 通过，缓存/外部 SKIP 边界同上。

# 刷新会话数据库失败恢复（2026-10-04）

原实现先提交旧令牌撤销，再生成并保存新会话。新增回归已复现保存故障后旧令牌无法重试。现改为先无副作用准备凭据，再在一个事务内撤销旧会话、保存替换会话；保存失败整体回滚。

事务先锁账号，再消费会话，与改密、禁用和权限变更保持锁顺序。锁内复核账号 ACTIVE、会话版本、旧会话所有者、有效期及未撤销状态；同一旧令牌仍严格单次消费。查询不存在与内部故障分开：失效令牌返回 `401 INVALID_SESSION` 并清 Cookie，内部故障返回 `500 AUTH_SERVICE_UNAVAILABLE`，保留 Cookie；日志仅记路由及错误类型，不输出凭据或数据库原文。

没有新增迁移、运行时依赖或前端运行时代码。刷新成功后的响应丢失、提交结果未知、跨标签页协调均不在此修复范围；不能把数据库失败回滚等同于透明恢复所有刷新中断。

## 验证记录

- `TestRefreshPersistenceFailureLeavesOldSessionRetryable` 先红后绿。HTTP `TestRefreshInfrastructureFailurePreservesCookieAndRetry` 覆盖读取/保存故障500、安全错误码、无Set-Cookie、重试200及重放401清Cookie。
- 一次性实际 PostgreSQL：`go test ./internal/auth -run TestRefreshAtomicPostgres -count=1 -race -v` 通过（7.103秒）。实际 INSERT 触发器故障证明旧会话保留，解除后重试成功；双并发仅一次成功；确认实际账号行锁等待后提交失效，拒绝旧版本；过期、撤销、禁用无新会话，查询故障不误报无效令牌。
- 实际 `ChangePassword` 与暂停在持久化前的刷新交错，改密提交后拒绝旧版本轮换、全部旧刷新会话失效、旧Access Token被拒绝。
- 隔离脚本 `/private/tmp/four-check-refresh-atomic-test.py`，日志 `/private/tmp/four-check-refresh-atomic-result.log`。默认完整测试的外部SKIP不能替代这组证据。

最终 gofmt、go vet ./...、go test ./...、go test -race ./... 均通过；默认缓存及外部SKIP不充实际集成。前端 lint、63文件304项测试（270.81秒）、build 通过，保留既有CSS/chunk警告。Chrome154.0.8037.97真实数据库故障/恢复/退出报告 [54c63b6d4f](../../output/playwright/54c63b6d4f/summary.json) 共11组通过，浏览器异常及清理错误0。两次早期脚本失败分别为错用嵌套错误码、未展开账户菜单，修正断言/操作后重跑。额度剩余35%。

浏览器初始化故障目前仍回到登录页，Cookie保留，故障解除后重新导航可以恢复。随后[明确提示和原位重试](2026-10-04-login-recovery-ui.md)已完成独立增量，以上“重新导航”保留为当时验收方式。目标业务服务未重启加载，目标环境和阶段总门禁仍待验收。

# 认证错误与账号会话原子性（2026-10-04）

本轮从认证内部故障误报401扩展到其失败恢复根因。登录账号读取/会话保存、Access Token账号读取、改密写入及退出撤销故障，原来分别误报401、400或204。五项HTTP回归全部先失败；现明确的无效凭据/会话仍为401，输入或旧密码错误仍为400，内部故障统一500 `AUTH_SERVICE_UNAVAILABLE`，安全提示且不清Cookie。

实际 PostgreSQL 故障进一步复现：改密、管理员重置密码、禁用和账号/角色修改先提交账号与审计，再另行撤销会话；撤销失败会部分提交。现在账号修改、会话撤销和审计同事务，锁顺序保持账号→会话；返回账号也在事务内读取，避免提交后额外查询失败产生含糊结果。服务层不再另发撤销操作。没有新增迁移。

退出失败时前端保留当前页面，显示安全提示和“重试退出”，不产生未处理Promise拒绝。两处改密页面通过统一认证动作完成改密和本地会话/查询清理；成功接口本已撤销所有会话并清Cookie，不再额外发送退出请求，以免把成功改密显示为失败。

## 验证证据

- `TestAuthInfrastructureErrorsAreRetryableAndDoNotClearCookie` 五种故障先红后绿，恢复后原会话可用，错误原文不泄露；既有刷新、登录、管理及改密聚焦测试通过。
- 实际 PostgreSQL/race：`TestAccountMutationAtomicPostgres` 四类操作先全部复现部分提交，修复后四类全部通过（9.533秒）。触发器拒绝refresh_sessions撤销UPDATE，证明账号字段、密码、版本、审计和会话均回滚；解除后重试仅一条审计，旧刷新/访问令牌均失效。
- 刷新兼容实际 PostgreSQL/race `TestRefreshAtomicPostgres` 再次通过（6.937秒），包括双并发、锁等待及真实改密交错。
- 两处改密组件回归先失败，修复后与退出、Provider聚焦共3文件20项通过，覆盖失败输入保留、成功清缓存及无第二次退出请求。
- 脚本 `/private/tmp/four-check-account-mutation-test.py`；实际PG日志 `/private/tmp/four-check-account-mutation-result.log`、`/private/tmp/auth-errors-refresh-compat.log`。

最终 gofmt、go vet ./...、go test ./...、go test -race ./... 通过（默认外部SKIP/缓存不充集成）；前端lint、65文件311项全量测试（273.13秒）、build通过，保留既有CSS/chunk警告。Chrome154.0.8037.97真实API/数据库报告 [9fbfeacb6b](../../output/playwright/9fbfeacb6b/summary.json) 共12组通过，覆盖登录保存500、改密撤销故障回滚、退出失败/重试、375px、恢复后旧密码仍可登录、成功改密无第二次退出、新密码成功及旧密码401；浏览器异常/清理错误0。早期窄屏断言在侧栏动画中读取布局，改为等待可观察的最终可达状态后通过，未裁剪页面。最新额度剩余30%。

下一项核验普通登录凭据签发与账号变更并发：当前Save不核对读取时的账号版本，先用实际PG交错测试确认是否会遗留旧凭据会话。仅一次性数据库与虚构账号，未加载目标业务服务；成功刷新响应丢失、全角色及阶段总门禁仍未关闭。

# 2026-10-25 认证刷新响应丢失当前实现复验

## 结论

当前工作区仍能在受控“服务端轮换成功、浏览器未收到响应”的情况下复现刷新会话丢失；此前记录的自然整页跳转失败在本轮 12 次中未复现。2026-10-04 的原子轮换修复覆盖数据库持久化失败，不覆盖提交成功后的网络响应丢失。

## 隔离浏览器证据

- 基线：分支 `feat-0.1-2`、HEAD `37c0413`；一次性 PostgreSQL、MinIO、当前工作区 API/Vite 和合成账号；Chrome `156.0.8078.48`。
- 自然导航：12 轮整页进入工作台后再打开到课率页面，均没有返回登录页；每轮记录的刷新请求为 HTTP 200。本轮不能据历史 5/12、7/12 结果断言该自然竞态仍常见。
- 受控故障：独立请求上下文收到真实刷新响应 HTTP 200，浏览器页面请求被中止；浏览器仍保留原 `refresh_token`，随后重试返回 `401 INVALID_SESSION`。该路径再次证明成功提交后的响应丢失会使浏览器失去替换凭据。
- 报告：[隔离认证导航诊断](../../output/playwright/fd62719e4f/summary.json)。诊断只使用合成账号/临时服务，测试结束后环境已清理。

## 根因链路

1. `web/src/shared/api/httpClient.ts` 的 `refreshAccessToken` 只有收到并解析成功响应后才更新内存 Access Token。网络失败会保留现有会话并允许重试。
2. `server/internal/auth/service.go` 先准备替换凭据，再调用 `Rotate`；`server/internal/store/postgres.go` 在一个事务中撤销旧 refresh 会话并保存新会话。
3. `server/internal/httpapi/router.go` 仅在事务成功返回后通过 `Set-Cookie` 把新 refresh token 发给浏览器。
4. 若事务已提交而响应未到达浏览器，数据库中的旧 token 已撤销，浏览器仍只有旧 Cookie；下一次刷新无法恢复替换 token，服务端按当前严格单次消费规则返回 `401 INVALID_SESSION` 并清 Cookie。

## 边界与后续设计门槛

本轮没有改认证代码、迁移或安全语义。不能为解决响应丢失而无条件接受旧 refresh token 重放。透明恢复需要设计可跨同标签页整页跳转保存的请求标识、服务端对已提交轮换结果的安全重取、结果保留/加密和过期策略，以及跨标签页并发处理；须明确重放威胁模型并补真实 PostgreSQL/浏览器故障矩阵后再实现。

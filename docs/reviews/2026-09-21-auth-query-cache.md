# 2026-09-21 同浏览器账号切换缓存隔离

## 根因与修改

AuthProvider 登录、退出仅更新 user，跨会话共用 QueryClient 中的记录、权限与关联选项仍存在。新账号进入同一业务页面时，TanStack Query 会先显示旧缓存再刷新，造成短暂跨账号展示。

登录/退出在认证 API 成功后、更新 user 前，先 await cancelQueries 再 clear。旧查询被取消、查询和 mutation 缓存移除，迟到响应不会恢复已移除的查询。失败的认证请求不进入身份切换步骤。没有改变服务端鉴权、Cookie、Token 或单次刷新消费规则；认证整页导航响应丢失问题仍独立未修复。

## 证据

- AuthProvider 回归先红：退出后仍有上一账号学生记录；成功登录另一个账号后仍有上一账号权限缓存。实现后3项聚焦测试通过，含 StrictMode 仅恢复一次、退出取消未完成查询及成功登录清理。
- 隔离真实浏览器红报告 `output/playwright/bb0b3ba561/summary.json`：上一账号已有两条记录，退出并登录不同部门的查看成员，延迟真实列表响应，断言失败 `Previous account record visible while new list is pending`。
- 此前 `849c4146cc` 被路由清理异常遮蔽，`f2f6a10290` 错误等待工作台（实际按 from 返回原页）；这些测试脚本失败不计为业务复现。
- 修复后真实服务浏览器通过：`output/playwright/2ba0f1aa09/summary.json`，15 组场景，Chrome 153.0.8010.50，无浏览器错误和清理错误。
- `pnpm lint && pnpm test && pnpm build` 全部通过：33 文件/180 项测试，196.54 秒；输出 `/tmp/four-check-auth-cache-final.log`。既有 jsdom 样式提示与构建大包警告保留。

本缺陷已在隔离环境闭环；下一项推进晚归页面；业务环境未加载源码，没有部署、提交或推送。

# 2026-10-06 长页面会话续期修复

## 根因与范围

分支 `feat-0.1-2`，保留工作区原有未提交改动。用户确认长时间停留后出现“读取工作周次失败”，按钮重试失败，但整页刷新恢复。

API 入口的 Access Token 有效期为 15 分钟；`useWorkContext` 每 30 秒请求工作周；原 `httpClient` 在 401 时仅抛错；原 `AuthProvider` 仅在挂载时调用 Cookie 续期。于是过期后每次重试均携带原凭证。诊断时以现有传输代码模拟过期，三次重试均 401，续期请求为零；本机 `/health/ready` 为 200。本轮不修改后端 TTL、权限或数据库。

## 实现与决定

- `httpClient` 统一处理受保护请求的 401，续期后只重试一次；启动恢复与业务请求共用进行中的续期 Promise。
- 并发 401 不重复消费 Refresh Cookie；已完成续期后到达的旧凭证失败直接使用新凭证。
- 保留原请求方法、Body、幂等键与下载响应头；403、500、登录/注册/退出/续期自身不触发递归续期。
- 登录、退出和改密建立会话代次；旧请求/续期不能覆盖新会话，也不能以新账号重发旧写入。已取消的请求不重发。
- 明确失效清令牌、用户与私有缓存，登录页显示失效提示；暂时网络/5xx 保留原页面和输入，允许重试。
- 保持 Refresh Cookie 单次消费规则，不恢复已消费 Cookie，不持久化访问令牌。

## 验证记录

工具链：默认 Node 22.23.0/pnpm 10.26.0 与仓库 pnpm 11.7.0 不一致；已定位并使用现有 Node 24.12.0 与 pnpm 11.7.0，不安装依赖、不改锁文件。初次诊断测试曾误用已装 pnpm 10.26.0，纠正后重新运行。

- RED：新增回归与 Provider 测试共 21 项中 12 项失败、9 项通过，失败原因是缺少续期/失效同步；没有未处理 Promise rejection。
- 聚焦 GREEN：`pnpm --dir web exec vitest run src/shared/api/sessionRenewal.test.ts src/shared/api/httpClient.test.ts src/app/providers/AuthProvider.test.tsx src/pages/auth/LoginRecovery.test.tsx`，4 文件 35 项通过。
- `pnpm --dir web lint` 通过。
- 隔离浏览器：`BROWSER_DB_MODE=native BROWSER_SUITE=session-renewal pnpm --dir web test:browser`（显式 Node 24/pnpm 11、`GOCACHE=/tmp/four-check-progress-go-cache`、现有 `PLAYWRIGHT_CLI_JS`），PostgreSQL 16.13 / MinIO / 真实 Go API / Chrome 154.0.8037.98，11 组通过，未捕获浏览器异常及清理错误均为 0。报告：`output/playwright/306d8000e4/summary.json`。
- 续期浏览器场景使用一次性库签名密钥为虚构账号构造真正过期 JWT，仅在测试拦截中替换发往真实 API 的 Authorization；后端返回真实 401 `UNAUTHORIZED`，之后真实刷新 Cookie 并重发获得 200。浏览器时钟推进用于触发 30 秒轮询，并非真的等待 15 分钟。另验证并发两请求、未保存弹窗内容保留、续期 500 后原位重试、清 Cookie 后真实 401 跳转与重新登录返回。
- 浏览器前两轮测试脚本失败单列：`ea321aeb83` 因 CLI 沙箱无 URL 构造器，在新增场景前中断；`9c17c12554` 已执行续期/失效场景，在登录按钮正则同时匹配“重试恢复登录”时中断。分别改为字符串解析及锚定按钮名称后，完整重跑 `306d8000e4` 通过；未改产品代码或放宽断言。失败快照现统一经过既有敏感值脱敏器。
- 只读目标探测：本机 5173 的 `httpClient.ts`、`AuthProvider.tsx` 编译模块 HTTP 200 且包含本修复；8080 `/health/ready` HTTP 200。不是业务账号登录态验收，未改动业务库或重启服务。
- 前端完整验证：`pnpm --dir web test`，71 文件/383 项通过（317.94 秒）；`pnpm --dir web build` 成功（TypeScript + Vite），保留大于 500 kB 分包警告。最终 `pnpm --dir web lint` 再次通过；测试辅助函数去除非空断言后聚焦 20 项再次通过，运行时代码仅改注释。
- 后端兼容检查：`GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache go vet ./...` 与 `go test -json ./...` 均退出 0，17 包通过；默认 30 项因未配置一次性 PostgreSQL/MinIO 等外部条件 SKIP，不能将其算作通过。与本轮有关的真实 Cookie 轮换和工作周读取另由上述隔离浏览器验证；未重新执行全部 Go 外部集成矩阵。

保留已存在的 jsdom CSS/伪元素诊断及 Ant Design 静态 message、useForm 警告，未隐藏或将其声称为无警告。

## 交付边界

| 分类 | 本轮状态 |
| --- | --- |
| 模型/迁移 | 无变更 |
| API | 无服务端契约或权限变更；修复前端统一请求与认证客户端 |
| 页面 | 长会话续期、会话失效后的用户/缓存同步 |
| 模板/初始配置 | 无变更 |
| 真实环境 | 隔离真实服务 11 组通过；本机 Vite 已提供新模块，目标业务账号长会话未验；云端未部署 |
| 角色验收 | 认证增量验证；不是全项目/五类业务总门禁 |

跨标签页的独立 Cookie 轮换竞争、刷新提交后页面整页跳转丢失响应，仍属于原有独立问题；本轮仅保障单页面并发续期。云端旧静态资源需要重新构建部署，本轮不部署或创建新版镜像。修复不会把有效期到达当成永久失效；Refresh 会话真正到期、被撤销或账号失效仍要求登录。

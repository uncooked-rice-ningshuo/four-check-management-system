# 2026-09-20 Playwright 校历增量回归

## 范围和结论

上一批交付只运行 Vitest、Go 和 PostgreSQL/MinIO 集成检查，没有运行 Playwright。本次已补入可重复执行的浏览器用例与隔离启动入口，并实际运行通过。它属于 Task 18 的局部增量，不关闭 Task 0、18、19，也不证明现有业务环境已经恢复。

- 分支 `feat-0.1-2`，基线 `8be1aca` 加当前未提交修改；没有切换分支或提交。
- Playwright 官方 CLI `0.1.21`，Google Chrome `153.0.8010.50`，headless，1440×1000。
- Node `24.12.0`、pnpm `11.7.0`、Go `1.25.0`、PostgreSQL `16-alpine`、本机 MinIO/mc。
- 当前 Go 源码临时构建 API；真实 Vite 前端，React StrictMode；全新临时数据库应用全部现有迁移。
- MinIO 为独立临时进程，私有且启用版本控制的 Bucket；API 仅接收受限应用账号。随机管理员走真实登录和首次强制改密，没有跳过认证或修改账号状态绕过流程。
- 不读取业务 `.env`，不写业务库、不使用已有 Bucket，不重启现有 8080 API；结束后清理本次服务。没有新增生产依赖。

## 已运行的场景

| 场景组 | 操作与断言 | 后端是否真实 |
| --- | --- | --- |
| 登录流程 | 登录 → 首次强制改密 → 返回登录 → 新密码登录 → 工作上下文可见 | 是 |
| 弹窗生命周期 | 学年、学期、周次分别执行新建关闭后首次编辑、关闭再编辑；日期必须正确回显 | 是 |
| A/B 切换 | 分别打开两条周次，日期与备注不得串值；按记录定位，不假设排序 | 是 |
| 冲突与重试 | 将 B 日期改为与 A 重叠；断言 409、ACADEMIC_DATE_OVERLAP、中文提示、输入保留；修正后同弹窗重试成功 | 是 |
| 保存后联动 | 把当前周次从 1 改为 3；5 秒内顶部工作周更新，临时周选择项更新，不等待 30 秒轮询 | 是 |
| 服务失败与异常响应 | 分别注入标准 500 和 null 响应；断言安全提示、备注保留、保存按钮恢复可重试 | 仅这两次 PUT 做故障注入 |
| 持久化 | 重载页面后周次仍为 3、B 的修正备注存在；失败操作中的备注没有入库 | 是 |

以上为 **7 组场景**，不是独立测试框架统计的 7 个 test。脚本收到实际执行返回的 JSON 完成标记才判定成功；CLI 非零退出、工具错误、断言失败、未处理页面异常或非预期控制台错误都会使入口失败。

## 控制台与已知问题

未捕获到未处理的页面异常。预期的未登录 refresh 401、主动制造的 409、注入的 500，以及 favicon 404 被明确区分，不作未知故障。

另外观察到两个 Ant Design 警告：静态 message 不能消费主题上下文，以及 useForm 实例未连接 Form。这两个警告保留在输出并写入 summary 的 knownWarnings，不宣称“控制台无错误/警告”。当前行为断言通过；其治理列入后续界面任务，本轮未改动产品运行时代码来隐藏警告。

## 复跑方式与证据

从 `web/` 运行：

```bash
pnpm test:browser
```

从根目录直接运行等价命令：

```bash
GOTOOLCHAIN=go1.25.0 python3 server/scripts/verify-browser.py
```

前置条件：Docker、本地 `postgres:16-alpine` 镜像、Go、Node 24、Python 3、MinIO、mc、Google Chrome、已安装的前端依赖。默认 npx 使用固定 `@playwright/cli@0.1.21`，首次需联网；可用 `PLAYWRIGHT_CLI_JS` 指定已有官方 CLI 的绝对路径。无须为生产工程增加 `@playwright/test`。

- 启动与清理：[verify-browser.py](../../server/scripts/verify-browser.py)。
- 浏览器操作及断言：[calendar-regression.js](../../web/e2e/calendar-regression.js)。
- 最终复验：`pnpm test:browser` 退出码 0；运行编号 `9285c66f8b`。报告见 [summary.json](../../output/playwright/9285c66f8b/summary.json)，截图见 [保存后页面](../../output/playwright/9285c66f8b/calendar-passed.png) 和 [真实冲突提示](../../output/playwright/9285c66f8b/calendar-conflict.png)。这些是本机忽略提交的产物，其他检出应重新运行生成。
- 脚本打印 `output/playwright/<run-id>/`：其中 `summary.json` 是状态及范围，`calendar-result.txt` 是实际 JSON 返回，截图和快照用于复核。目录由 Git 忽略。
- 不保存认证期间的 trace、storageState、请求 Cookie 或令牌。临时密码不写入仓库；错误诊断脱敏，测试结果不包含凭据。
- 本轮还执行 `pnpm lint`、JS/Python 语法检查及 `git diff --check`。未修改产品运行时代码，未重复上一批的 151 项组件测试和 Go 全量测试，也不把旧结果冒充本轮重跑。

## 未覆盖与下一步

- 仅覆盖超级管理员登录和校历；非管理员越权、部门组合、所有导航守卫还需完整角色矩阵。
- 到课率录入/编辑/删除/导入，其他六类页面、部门审核、退回重提与回滚尚未纳入本浏览器入口。
- 422 字段明细解析仍由已有 Go/TypeScript 回归覆盖，本次没有将它算作页面级浏览器验证。
- 500 是响应故障注入，不是 PostgreSQL 真故障恢复测试。历史业务 MinIO、模板发布以及目标业务环境登录闭环仍待完成。
- Firefox、WebKit、移动端和无障碍全量审计未执行。

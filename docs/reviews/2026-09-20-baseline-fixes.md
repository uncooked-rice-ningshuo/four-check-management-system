# 2026-09-20 第一批基线修复与验证

> 后续补充：本记录描述第一批交付时的验证边界。随后已执行隔离真实服务的 Playwright 登录/校历回归，见[浏览器回归记录](2026-09-20-playwright-regression.md)。业务环境恢复和完整阶段二浏览器矩阵仍未完成。


分支：`feat-0.1-2`，基线 `8be1aca`，本次改动未提交。保留此前文档整理，未创建分支/worktree，未修改业务数据库或重启旧 API。

## 完成内容

- 通用 HTTP 错误出口显式区分 400 校验、403 权限、404 不存在、409 冲突和未知 500；日志只记方法、路由模板及类型，不记录原始仓储错误。
- 七类字段校验统一顶层 `{code,message,details}`；前端兼容旧嵌套格式，保留合法字段明细，过滤异常结构。
- 校历保存失败可见提示、保留输入并可重试；成功刷新校历、工作上下文、当前周、临时周选择、工作台、到课率选项缓存。错误回调不返回 Ant Design 提示的 thenable，避免按钮等待消息消失。
- 隔离集成发现历史夹具固定课程名、时钟取模年级名及关联清理顺序导致冲突，修正后真正执行到事务失败回滚检查。未删除失败用例，未放宽数据库约束。
- 新增 `server/scripts/verify-integration.py`，创建并销毁本次临时 PostgreSQL/MinIO，用随机凭据与独立空库执行真实集成，任一失败返回非零；不读取业务 `.env`。

## 实际验证

首先观察 HTTP、错误解析、校历新回归失败，再实现修复。校历重试用例进一步暴露了提示 thenable 导致的 pending 延迟，修复后复验。

| 命令（对应目录） | 结果 |
| --- | --- |
| `go test ./internal/httpapi -run 'TestDomain\|TestInspectionValidationUses' -count=1`（server） | 通过 |
| `pnpm exec vitest run src/shared/api/httpClient.test.ts`（web） | 7 项通过 |
| `pnpm exec vitest run src/pages/admin/AcademicCalendarPage.test.tsx -t '保存失败'`（web） | 最终通过 |
| `pnpm lint`、`pnpm test`、`pnpm build`（web） | 全部通过；27 文件、151 项测试 |
| `go vet ./...`、`go test ./...`（server） | 通过；默认运行外部服务用例有 SKIP，由下项补验 |
| `python3 server/scripts/verify-integration.py`（仓库根目录） | store 50、storage 21、importjob 49 项通过；0 失败、0 SKIP |

本机验证显式使用 Node 24.12.0、pnpm 11.7.0、`GOTOOLCHAIN=go1.25.0`、`GOCACHE=/tmp/four-check-progress-go-cache`。最终前端套件约 227 秒，已正常结束；有 jsdom CSS/伪元素提示和 Vite 大于 500 kB 分包警告，未掩盖或关闭。未改并发运行时代码，本批未额外运行 race。

## 运行环境与未完成边界

- 只读检查实际 API 进程配置确定数据库为原生 PostgreSQL；连接及区域表/工作周配置检查成功，`import_templates` 数量为 0。
- API 的 MinIO 地址为 `127.0.0.1:9000`，没有监听；未找到可确认的原对象数据目录。不能用新空 Bucket 冒充恢复成功。
- 尚未重启 API、启动业务页面、执行登录浏览器验收或发布模板。隔离导入成功不等于业务环境可用。
- 第一批尚未关闭 Task 0：余项为原存储恢复、可重复模板交付及浏览器闭环。卫生区域归属、退回修订语义、整批回滚决策已询问，尚无回复，不据超时自行视为批准。
- 本次尚未实现其余六类页面、部门工作流和六类导入；继续按[项目状态中的顺序](../project-status.md)推进。

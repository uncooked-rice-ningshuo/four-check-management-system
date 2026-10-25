# 2026-10-05 四类列表故障与页面布局修复

## 结论

用户在 `localhost:5173` 报告无机课堂、早锻炼/早签、晚归、宿舍卫生列表返回 `INSPECTION_FAILED`。四类使用同一 `/api/v1/inspections` 查询，列表读取非空记录时会调用 `Read`，其可编辑条件引用 `department_submissions`。用户按本机启动说明运行的 `four_check` 数据库迁移账本只到 013；016 创建该表。一次性 PostgreSQL 在应用到 015 后，通过真实 `GetInspection` 复现 SQLSTATE `42P01`。这解释了四类共用失败，而到课率使用另一列表接口。

先使用 `pg_dump --format=custom` 备份宿主机目标库，备份保存在权限受限的 `/private/tmp/four-check-target-backup.W8AVqR/four_check_before_014_017.dump`，大小 193859 字节。随后按仓库 `server/scripts/migrate.sh` 应用 014、015、016、017；每个文件以事务提交，原有 001–013 均跳过。迁移后 `schema_migrations` 包含 014–017，`/health/ready` 返回成功。目标库四类各有 1 条记录；当前仓储代码在目标库分别执行 `ListInspections`，四次均得到 `items=1,total=1`。用户在已登录的 5173 页面刷新四类后确认“已经无问题”。没有在业务库制造违规记录或新增测试账号。

原 API 是用户终端中启动的 `go run ./cmd/api` 进程，迁移后继续运行。本轮新增的就绪诊断和安全 SQLSTATE 日志代码尚未由该进程重新加载；数据库修复已对它生效。仓库初始管理员密码与当前密码不同，只尝试一次登录得到 401，未重试猜测或输出凭据；因此没有用自动化凭据重放登录态 HTTP。用户页面确认与仓储只读查询是当前目标环境证据，完整角色验收仍开放。

## 代码与界面

- `/health/ready` 现在要求 016 的批次及修订表、017 的不可变触发器；一次性 PostgreSQL 验证缺迁移不就绪、完整迁移后就绪。
- 检查接口遇到未知数据库错误时，响应继续使用安全的 `INSPECTION_FAILED`，服务端只记录路由、SQLSTATE 和缺失关系信息；测试确认 SQL 内部信息不会进入响应。
- 以到课率为基准，六个其他检查页面统一容器、标题、展开筛选、记录卡片、总数标签、工具栏、分页、新增/导入顺序及批量删除显隐。教室卫生、公共宿舍卫生只调整已有页面布局，不推进待定业务规则。
- 六页浏览器用例改用 ProTable 的刷新控件，并适应批量删除计数文案。

## 验证

- 后端：`go vet ./...`、`go test ./...`；一次性 PostgreSQL `TestReadyRequiresCurrentSchema`，包括 015 状态下真实 `GetInspection` 的 `42P01` 回归。
- 前端：`pnpm lint`、`pnpm test`（69 文件、348 项）、`pnpm build`。构建保留已有大包提示，jsdom 保留样式模拟提示。
- Chrome 154 + 一次性 PostgreSQL/MinIO：六页独立回归分别见 `output/playwright/54a57bdbd0`、`dbfcd01951`、`a62d7f597c`、`ca95862ed0`、`f3e1b037f4`、`0abb10ba78`；最终工具栏调整后六页复验均通过，报告见项目状态。
- 五类批次跨角色矩阵：`output/playwright/199c793c2e/summary.json`，录入/审核分权、跨部门隐藏与拒绝、原样重提修订、确认后状态约束均通过。增补阶段三负向边界后再次通过 `output/playwright/7f87bd6b18/summary.json`，工作台无通报待办/伪进度，页面没有请求阶段三报告 API。
- 当前源码五类导入/回滚/部门提交联合浏览器回归全部通过：到课率 `56fd16f1f2`、无机课堂 `8d0afa5402`、早锻炼 `8e8d2215f8`、晚归 `e0412ebba5`、宿舍卫生 `e7240ee200`（均为 `output/playwright/<编号>/summary.json`，使用一次性 PostgreSQL/MinIO）。这些是隔离环境结果，不代替目标库写入验收。

## 后续

用户授权后已在本机业务库和私有 Bucket 发布五类 v1 导入模板。数据库逐类为 `PUBLISHED`，关联文件均为 `AVAILABLE/TEMPLATE`，五个对象从 MinIO 读回后的 SHA-256 与预览一致，发布审计新增 5 条。目标下载路由的未登录请求逐类返回 401；已登录页面下载待用户反馈，不能据 401 推断下载成功。原 API 进程在方便时重启后，新增的就绪与日志诊断才能生效。阶段二总门禁仍须完成目标环境登录态 HTTP、完整角色与导入验收。

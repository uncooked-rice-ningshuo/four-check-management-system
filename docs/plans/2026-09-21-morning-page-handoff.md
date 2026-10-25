# 早锻炼/早签页面开发交接

直接在现有 `feat-0.1-2` 分支及当前工作区接续，核对分支并保留全部未提交改动；不新建分支或 worktree，不使用子代理。原任务已停止修改。

先读 `AGENTS.md`、`docs/project-status.md`、`docs/plans/2026-09-21-next-implementation.md` 和相关 PRD/技术方案/字段核对。立即实施早锻炼/早签页面及验证，不只提供计划。

## 已完成

- 到课率、无机课堂、晚归三类页面已实现。晚归详见 `docs/reviews/2026-09-21-late-return-page.md`。
- 晚归当前宿舍名称已实现：`currentDormitoryName` 为当前主数据名称，非历史快照，不进审计，停用/改名仍显示。详见 `docs/reviews/2026-09-21-late-return-labels.md`。
- 早签授权前置选项已完成，不能重复安排开发：上下文支持 `MORNING_ATTENDANCE`，录入选项仅 `classes/students`。七维 DENY、无关班级、禁止其他种类、MANAGE 不能创建但能原记录编辑均经过真实 PostgreSQL 回归。详见 `docs/reviews/2026-09-21-morning-options.md`。
- 最新前端 lint、38文件194项测试、build通过，日志 `/tmp/four-check-late-page-tests.log`；后端 vet/test 通过 `/tmp/four-check-morning-options-go.log`。
- 晚归真实浏览器最终 `output/playwright/8e04eefefa/summary.json`，17组通过，Chrome153.0.8010.50，浏览器/清理错误为空。真实 PG 最新四包报告目录 `/var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-znxwpxia`。

## 下一项

实现早锻炼/早签独立页面。现有 `MorningAttendanceDetails` 字段为 studentId/classId/date/absenceCount/remark；保持单一类型，不加子类型或时间；缺勤次数由后端固定1，不输入累计次数。

参考 `LateReturnPage`、`features/late-return`、`shared/api/lateReturnApi.ts` 和对应测试。选项分页/响应校验已抽到 `shared/api/inspectionOptionsApi.ts`。保持 UI→hook→API 和 entities/shared 依赖方向，避免无关重构。

ProTable 原生搜索/工具栏；独立新增目标与列表筛选；只默认有权当前周，无当前周不选首项；不手填UUID。表单使用挂载快照 initialValues、onChange清依赖，失败保留输入，409关闭重开核对。canEdit只作提示，后端权威。接类型导航、路由及前后端catalog手工动作，IMPORT不开放。

先回归再实现，前端运行代码改动必须lint/test/build；后端改动gofmt/vet/test。浏览器覆盖首次编辑、新增关闭再编辑、A/B切换、失败重试、重开、StrictMode、权限角色、真实409、批删及窄屏。可适配 `web/e2e/late-return.js` 去掉宿舍/时间，保留角色矩阵，在 `verify-browser.py` 增加聚焦和默认套件入口。体育部已有早签映射，应用不得写死部门关系。

## 工具与边界

- PATH前缀：`/Users/ningshuo/Library/pnpm/.tools/pnpm/11.7.0/bin:/Users/ningshuo/.nvm/versions/node/v24.12.0/bin`。Node24.12.0/pnpm11.7.0，执行前核对锁文件。
- Go：`GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache`。
- 隔离集成：根目录 `python3 server/scripts/verify-integration.py`，一次性PG/独立MinIO自动清理。
- 浏览器：根目录 `PLAYWRIGHT_CLI_JS=/Users/ningshuo/.npm/_npx/81fb41e6b6793dc6/node_modules/@playwright/cli/playwright-cli.js BROWSER_SUITE=late-return GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-browser.py`，使用上述PATH。新页需增加新的BROWSER_SUITE。现有聚焦先跑公共登录/校历7组。
- 隔离开发测试已有授权，按工具要求申请沙箱升级。禁止输出凭据/环境全集，不改dist/node_modules。
- 每个完整验证增量后查用量；开始下一项前任一可读核心窗口剩余<=40%停止总结，缺失窗口不是0，不用重置额度。最近周剩余67%，需重新读取。
- 提交、推送、PR、部署、替换业务服务、发布业务模板仍须具体授权。

## 未关闭

整页跳转丢失refresh响应导致401仍未修复，不能放宽单次token消费；切号缓存隔离已另行修复。业务服务未更新、模板未发布；历史ready不代表当前。四类页面、六类导入、卫生区域配置、部门流程、回滚及阶段总门禁仍有缺口。卫生配置归属、退回修订语义和回滚需确认，不重复询问D1–D4。早签完成后同步状态、查用量并重新评估下一项。

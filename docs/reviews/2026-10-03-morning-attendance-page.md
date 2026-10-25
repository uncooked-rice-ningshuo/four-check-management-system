# 早锻炼／早签页面增量验收（2026-10-03）

## 本轮交付

- 在现有 `feat-0.1-2` 工作区接通独立路由、有效权限导航、ProTable 列表与筛选、详情、手工新增、草稿修改和批删。列表范围与新增目标分开；无当前授权工作周时不自动选择首个学期。
- 复用已验证的早签上下文和 `classes/students` 授权选项。学生、班级使用关联选择，不输入 UUID；学期、周次、部门、班级变化时清除下游值。编辑表单按打开时快照挂载，失败保留输入，409 提示关闭重开核对。
- 服务端继续固定每个学生每天的 `absenceCount=1`。页面不提供累计次数、锻炼／早签子类型或时间输入；客户端把服务端缺少必需字段、非 1 次数视为错误响应。前后端目录只开放手工操作，不开放尚未实现的 Excel 导入。
- 隔离浏览器首次发现详情面板仍使用 `LATE_RETURN` 类型守卫，已修正为 `MORNING_ATTENDANCE` 并补回归。

## 实际验证

- Node 24.12.0、pnpm 11.7.0：聚焦 Vitest 8 文件／28 项通过；`pnpm lint`、`pnpm test`（43 文件／207 项）与 `pnpm build` 通过。jsdom 样式提示和构建体积警告仍存在。
- Go 1.25.0：`go test ./internal/inspection -run TestInspectionCatalog -count=1`、`go vet ./...`、`go test ./...` 通过。默认 Go 测试可能跳过需要外部服务的用例，不能替代隔离集成。
- `BROWSER_SUITE=morning-attendance python3 server/scripts/verify-browser.py` 使用一次性 PostgreSQL、独立 MinIO、真实 API/Vite 与 Chrome 154.0.8037.97，通过 17 组记录（公共校历 7、早签 10），浏览器运行错误与清理错误均为 0。[结果](../../output/playwright/8ff87dab41/summary.json)覆盖首次编辑、新增关闭再编辑、A/B 切换、500 保留输入重试、409 关闭重开、422、角色越权、批删及 390px 窄屏。已知 Ant Design useForm/message 警告仍存在。
- 10-03 本地 8080 与 5173 均拒绝连接。本轮浏览器使用隔离服务；目标业务环境未加载新代码，也未发布模板或写入业务数据。

## 仍未交付

早签 Excel 模板与导入、部门提交审核流程、确认后修正、导入回滚及阶段二总门禁均未关闭。下一项先核对宿舍卫生对象和责任关联的授权选项及展示契约，再交付宿舍卫生页面；卫生区域配置归属仍待确认。

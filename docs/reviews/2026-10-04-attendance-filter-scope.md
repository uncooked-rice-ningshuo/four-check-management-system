# 到课率旧筛选选项授权修复（2026-10-04）

旧 `GET /api/v1/inspections/attendance/filter-options` 仅投影 ALLOW，遗漏学年与 DENY，且先读取全部班级。已用 HTTP 回归复现其他学年授权、当前学年拒绝仍返回当前选项，再改为复用 SQL 授权的 `context-options purpose=view`。旧响应结构保持，部门映射仅在已授权部门内展示；选项分页不完整、重复或类型不一致均明确失败，不返回正常空态。

班级上下文选项新增可选 `grade` 显示元数据。该值来自班级当前年级，历史权限仍使用原有快照谓词；前端兼容旧响应省略字段，拒绝非字符串字段。没有迁移、模板发布或业务流程变更。

## 实际验证

- 后端聚焦 HTTP 学年/DENY、分页失败关闭测试通过；修改 Go 文件格式化，`go vet ./...`、`go test ./...` 通过。默认外部数据库用例跳过或缓存不作为集成证据。
- 一次性 PostgreSQL：`go test ./internal/importjob -run TestAttendanceContextOptionsPostgres -count=1 -race -v` 通过，覆盖五种学年/班级授权组合、五类选项、201 个班级分页和当前年级显示。脚本 `/private/tmp/four-check-context-options-test.py`，日志 `/private/tmp/four-check-context-options-result.log`。
- Node 24.12.0、pnpm 11.7.0：`pnpm --dir web lint`、`pnpm --dir web test`（63 文件、304 项，265.55 秒）、`pnpm --dir web build` 通过。保留既有 jsdom CSS 提示及大 chunk 警告，无依赖或锁文件改动。
- 真实 Chrome 154.0.8037.97、一次性 PostgreSQL/MinIO、真实 API/Vite：`BROWSER_SUITE=attendance-filter-scope` 报告 [cf2010e275](../../output/playwright/cf2010e275/summary.json) 通过。含原有 7 组日历回归与 5 个受限账号场景，匿名请求 401；完整学年拒绝显示 403，部分范围剩余保留入口。浏览器异常、清理错误均 0。
- 两次早期浏览器失败分别为夹具缺其他学年与完整拒绝场景错误期待业务标题，均修正测试后重跑，不调整权限规则。

旧导入兼容 `BROWSER_SUITE=import-scoped-targets` 报告 [695c3e44f3](../../output/playwright/695c3e44f3/summary.json) 的12组通过，浏览器异常及清理错误0。最新额度剩余38%。目标业务环境未加载；上述结果不代表跨角色总门禁或阶段二完成。公共宿舍卫生责任跨楼规则、部门提交分组仍待用户确认。

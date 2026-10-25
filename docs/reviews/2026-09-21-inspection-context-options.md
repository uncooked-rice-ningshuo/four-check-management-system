# 2026-09-21 普通成员页面上下文

新增 `GET /api/v1/inspections/context-options`。`typeCode` 目前限到课率、无机课堂；`purpose=view|entry`，`kind=semesters|weeks|departments|classes`。周次要求 semesterId；录入部门/班级要求 semesterId、weekId；可选 departmentId 缩小范围。分页默认 1/20，上限 200，响应 items/total/page/pageSize/inspectionTypeId，客户端必须取全分页。接口不选择默认周。

Application Service 校验输入并解析有效权限，Repository 在 SQL 中复用七维 ALLOW 扣 DENY。查看仅用 VIEW；新增仅用 ENTER，不允许 MANAGE 替代；编辑可传 recordId，先校验原记录修改范围、DRAFT、无提交/锁和类型一致，再按原所有者使用 ENTER/MANAGE 的既有规则，其他动作不会进入候选权限。候选录入范围要求启用校历、部门映射和班级；查看额外合并已有个人记录的历史范围，配置停用后仍可查询其原记录。返回最少 ID 和名称，不提供管理权限。无可见范围返回空页，数据库故障返回失败。

输入测试先红后绿；HTTP 路由先被详情误接、缺少 items，装配后通过 401/400/403/404/409/500 契约。真实 PostgreSQL 先红（yy1e8xlr），实现后通过（bhxp8_mm）；最终 ap9cag95 补验分页第二页、历史停用校历、历史 DENY、VIEW 不可录入及七维录入 DENY，store/storage/importjob/template-cli 四包退出 0。命令为 `GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py`，报告在临时目录 `four-check-integration-results-<编号>`。`gofmt`、`go vet ./... && go test ./...` 通过，缓存/默认跳过外部测试另由隔离套件补验。最初错误目录导致的 no tests to run 不计通过。

模型/迁移：请求响应类型，无迁移。API：已装配。页面：未接入。模板/初始配置：未变。真实环境：一次性环境通过，业务服务未更新。角色验收：后端范围通过，浏览器待页面实现。当前无前端调用方，未重复无变化前端检查。下一项无机课堂页面；阶段二总门禁 OPEN。

# 导入与到课率列表的权威学年范围（2026-10-04）

问题：HTTP 授权构造的范围缺少学年，导致只限定当前学年的 ALLOW 被拒绝，其他学年的 DENY 误伤当前请求。16 个初始 HTTP 场景实际复现其中 8 个失败。

修复：三个授权入口在普通账号权限判断前，通过既有校历应用服务查找学期所属学年，覆盖权限范围的学年值。模板/上传使用 `authorizeInspectionScope`，详情/预览/确认/取消使用 `authorizedBatch`，到课率列表使用 `authorizeAttendanceList`。不信任客户端 `academicYearId`；不改变超级管理员原有全局权限，必须改密限制仍优先。

验证：

- 四种规则 × 六个入口（模板、详情、预览、确认、取消、列表）共 24 个 HTTP 场景通过：本学年 ALLOW/本学年 DENY/其他学年 DENY/其他学年 ALLOW，请求均附伪造学年。
- 校历未配置、内部读取故障、缺失学期、批次所属学期缺失四个失败路径通过；失败时不继续授权，错误不泄露内部诊断。
- `go test ./internal/httpapi -run 'TestImportAcademicYearScopeUsesAuthoritativeSemester|TestInspectionAcademicYearLookupFailsClosed' -count=1`、`go vet ./...`、`go test ./...` 通过。完整默认套件有缓存及外部环境 SKIP。

无数据库结构或查询改动、无前端运行时代码修改。本轮未单独执行上传学年专项浏览器或全角色浏览器矩阵；上传共用授权函数已接入，但其真实上传验收保留到总门禁。目标服务未加载，未写业务库。

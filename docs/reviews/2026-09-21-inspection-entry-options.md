# 2026-09-21 普通录入关联选项

## 已交付范围

新增 GET /api/v1/inspections/entry-options，仅支持到课率、无机课堂的 classes、students、courses、classrooms。调用方显式传学期、周次、检查类型及录入部门，不从列表空筛选推断录入目标。学生和课程要求 classId，教室另要求 courseId。应用服务校验 UUID、种类及分页；每页默认 20、最大 200，返回 items/total/page/pageSize。客户端仍须覆盖分页，不能只取第一页。

仓储先核验启用的校历及部门类型映射，再用既有七维 ALLOW 扣 DENY SQL 在班级层过滤。学生限已授权班级；课程必须属于班级并有本学期启用教室排课；教室来自已选课程的本学期排课。标签只含选择所需名称、学号或楼栋房间，不暴露管理级数据。无可用授权返回正常空选项；原记录无权编辑返回 404，不泄漏其存在。

新增只允许 INSPECTION_ENTER。传 recordId 时先校验原记录修改权限、DRAFT、无提交和无通报锁，再按原所有者判断目标范围的 ENTER/MANAGE；不允许更换检查类型。该接口不提供基础数据管理能力。

## 验证

- 输入校验回归先失败（invalid query accepted），实现后 TestEntryOptions 两项通过。早期错误工作目录导致“no tests to run”和模块查找失败不计通过。
- HTTP 路由先失败：entry-options 被详情路由接收，响应缺少 items；注册后 TestInspectionHTTPContracts 通过，包含 401、400、403、404、409、500 稳定码及安全错误文本。
- 真实 PostgreSQL 分页先失败（返回空页）；实现后分页、学生/课程/教室关联、七维 DENY、仅管理权不可新增、管理编辑及原范围拒绝通过。
- 最终隔离命令：GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py。四包退出 0，报告目录 /var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-yn9o8ond。
- 修改 Go 文件 gofmt；go vet ./...、go test ./... 通过（未影响包存在缓存；外部用例另由上述隔离套件补验）。
- 前端本轮 lint、28 文件/163 测试、build 已通过；选项接口尚无前端调用方，未重复无变化的前端检查。保留样式和包体警告。
- git diff --check 通过；没有重启业务服务、发布模板、提交或推送。

## 交付边界与下一项

模型/迁移：新增请求/响应类型，无迁移。API：应用装配和路由已接通。页面：尚未接入。模板/初始配置：未改变。真实环境：仅一次性 PostgreSQL/MinIO 验证，未更新业务环境。角色验收：后端范围回归通过，浏览器角色验收待页面实现。

下一项补课程/教室展示名称及普通成员可读取的学期/周次/部门/类型上下文（须先核对既有工作上下文接口能否满足），然后落地无机课堂页面，保持无当前周不自动选首项。不能因为关联选项可用而宣称无机课堂页面或阶段二完成。

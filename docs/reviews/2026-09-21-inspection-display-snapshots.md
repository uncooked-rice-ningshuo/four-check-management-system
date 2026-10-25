# 2026-09-21 通用检查学生班级展示快照

在现有 feat-0.1-2 工作区增加响应 studentNumber、studentName、className、grade。读取数据库已有快照，创建/编辑沿用事务内校验后的快照，不新增迁移、不修改权限或历史数据。卫生多责任对象及历史空快照的这些字段为空；课程/教室名称、普通录入选项和无机课堂页面仍未交付。

## 验证证据

- 先红：一次性 PostgreSQL 的 TestInspectionPostgresCRUD/UNATTENDED_CLASSROOM 因创建响应缺少展示字段失败；报告目录 /var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-rh3nidqv。
- 后绿：创建、详情、列表均返回学生和班级快照；将虚构学生改名后再次查询仍返回原快照。最终隔离报告目录 /var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-sdsr792g，store/storage/importjob/template-cli 四个包退出 0。命令为 GOTOOLCHAIN=go1.25.0 GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py。
- 修改 Go 文件已 gofmt；go vet ./... 与 go test ./... 退出 0，部分未受影响包命中缓存；外部服务用例由上述隔离套件补验。
- 前端 pnpm lint、pnpm test（28 文件/163 项，202.38 秒）、pnpm build 均退出 0；既有 jsdom 样式提示和构建包体警告保留。本增量未修改前端运行时代码。
- git diff --check 通过。业务库、业务服务、模板发布均未操作；没有提交或推送。

## 后续

先补范围过滤关联选项和课程/教室展示，再交付无机课堂页面及真实浏览器回归。现有学生/班级字段增量不能作为页面验收或阶段二总门禁通过。

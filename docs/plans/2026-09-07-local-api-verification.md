# 本地 8080 API 联调记录

日期：2026-09-07。用户明确批准先备份本地数据库，再应用 011–013 迁移。本轮暂停 Task 8/9。

## 数据库定位与执行结果

本机原生 PostgreSQL 16.13 和 Docker PostgreSQL 同时监听 5432。最初通过 docker exec 核对的是 Docker 库，仅到 010；其备份 `/tmp/four-check-before-011-013.dump` 校验后成功执行 011–013。但 API 使用 `127.0.0.1:5432/four_check`，实际命中原生 PostgreSQL，因此这次 Docker 迁移没有解决页面错误。

随后通过 API 进程配置的数据库地址和本机监听来源核实实际库，原生库已到 011。先生成 `/tmp/four-check-native-before-012-013.dump` 并用 pg_restore -l 校验目录，再以逐个事务执行缺失的 012、013；不重复执行 011。原生迁移台账最终包含 001–013。两份备份以仅当前用户可读写的权限保留，尚未做完整恢复演练；/tmp 不是长期备份位置。

API 保留原运行配置、会话密钥与存储配置，从当前代码重新构建并重启，包含数据库就绪检查修复。`/health/ready` 返回 200。前一轮“本地库仅到 010”的结论应限定为 Docker 库，不能用于描述 API 实际使用的原生库。

## 实际验证

通过内置浏览器已有登录会话，使用现有 authApi/httpClient 请求经 Vite 代理到 8080 的接口；未输出或持久化访问令牌。临时验证页面已移除。

| 接口/场景 | 结果 |
| --- | --- |
| GET /work-context | 正常返回 week/source/version |
| GET /academic/current-week | 正常返回 week |
| GET /dashboard/workbench | 正常返回 context/myTodos/departmentStatuses |
| GET /permissions/effective | 正常返回权限结构 |
| GET /inspections/attendance/filter-options | 正常返回筛选配置 |
| GET /inspections/attendance，完整学期/周/类型/部门条件 | 正常返回分页，total=0 |
| GET /inspections，分别筛选七种 inspectionTypeId | 七类均正常返回分页，total=0 |
| GET /admin/master-data/teaching-buildings | 正常返回分页 |
| GET /admin/academic/calendar | 正常返回校历 |
| PUT /work-context，故意使用不匹配版本 | 409 VERSION_CONFLICT，未修改配置 |
| GET /imports/attendance/template，完整上下文 | 404 IMPORT_TEMPLATE_NOT_FOUND，未解决 |

不带必要上下文的旧到课率列表返回 403 FORBIDDEN_SCOPE，补全上下文后正常，不能将该请求当作服务不可用。

浏览器实际访问工作台与到课率页面：当前工作周正常显示为第 1 周（2026-09-06 至 2026-09-12）；工作台加载失败、工作周读取失败提示消失；到课率显示正常空态。

## 验证边界与待办

实际原生库 import_templates 数量为 0，故模板下载及后续导入仍无法使用。需要补齐模板文件、版本及发布配置后再验证下载/上传/预览/确认/重复行提示，不能把该链路记为通过。

库内 inspection_records 数量为 0。本轮没有在真实学生上制造测试违规记录，因此真实 8080 新增、编辑、删除、详情成功路径及跨角色写入验证未完成。既有临时库集成测试不替代这部分现场验收。

实际运行 `GOTOOLCHAIN=go1.25.0 go vet ./...` 与 `go test ./...` 均通过；未配置外部服务的集成用例仍跳过。前端未保留运行时代码改动，本轮未重跑前端全量构建/测试。没有提交、推送、发布模板或执行后续业务任务。

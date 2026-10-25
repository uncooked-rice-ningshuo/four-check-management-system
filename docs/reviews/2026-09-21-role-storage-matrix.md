# 2026-09-21 到课率角色与存储故障矩阵

## 已完成范围

在一次性 PostgreSQL、独立 MinIO、当前 API/Vite 与 Chrome 153.0.8010.50 上增加真实角色/故障验收。没有修改运行时代码、发布业务模板或替换业务服务。

- 学习部成员在部门留空时看到四条本部门记录；体育部成员仅获本部门到课率 ALLOW，查询返回零条，显式指定学习部返回 403 FORBIDDEN_SCOPE。
- 学习部成员可下载真实 XLSX 模板；跨部门下载返回 403。跨部门批次详情、预览行及确认均为 404 IMPORT_BATCH_NOT_FOUND，隐藏资源存在性。
- 个人 DENY 覆盖学习部默认授权。权限变更使旧 Access Token 立即返回 401 UNAUTHORIZED；重新登录后列表、下载和确认仍被拒绝。浏览器侧到课率导航隐藏，直达路由显示无权访问。
- 未携带 Access Token 的模板下载返回 401 UNAUTHORIZED。
- 暂停本轮一次性 MinIO 进程，API readiness 真实返回 503，模板接口返回 500 IMPORT_SERVICE_UNAVAILABLE；页面显示失败并禁止下载/上传。恢复同一进程后 readiness=200，重新打开页面可再次下载模板。未用 mock 替换这些响应。

## 实际验证

使用显式 Node 24.12.0/pnpm 11.7.0 PATH、GOCACHE=/tmp/four-check-progress-go-cache 和已安装 PLAYWRIGHT_CLI_JS，运行 `python3 server/scripts/verify-browser.py`：退出 0，42 组场景通过，cleanupErrors=[]。见[最终报告](../../output/playwright/2651bca5ec/summary.json)。既有校历、到课率重传、代表性管理操作和账号禁用回归一并通过。

首次矩阵 f59b83eca8 失败于测试仍使用权限变更前的 Access Token（预期 200，实际 401）。修正测试为重新登录，并单独断言旧 Token 失效；产品没有为测试降低会话失效要求。

脚本语法检查 `PYTHONPYCACHEPREFIX=/tmp/four-check-python-cache python3 -m py_compile server/scripts/verify-browser.py` 与 `git diff --check` 通过。默认 Python 缓存路径曾受沙箱阻止，改为临时目录后通过。此增量仅修改测试/文档，未重新执行前后端完整单元套件；上一轮 163 项前端测试属于历史证据。

## 未覆盖与后续

本项验证现有模板下载，不代表尚不存在的导入源文件、通报或归档下载。对象暂停覆盖操作超时及恢复，未扩展为对象删除、内容篡改、生产灾备或全浏览器兼容性验收。其他六类页面、部门工作流及七类导入仍未交付，阶段二总门禁 OPEN。认证连续整页跳转问题见[独立诊断](2026-09-21-auth-navigation.md)，没有因矩阵通过而宣称修复。

真实故障再次证明 live 不能代替 ready；当前 Compose API healthcheck 仍请求 live，下一项在配置层修正门禁并验证解析，不启动或替换业务 Compose 服务。

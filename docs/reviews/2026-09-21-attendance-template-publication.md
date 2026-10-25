# 2026-09-21 到课率模板生成与发布

## 当前结论

已新增可重复的到课率 v1 模板生成及本地运维发布命令，在一次性 PostgreSQL/MinIO 中完成真实发布、故障恢复、并发重试和导入验证。未执行目标业务库发布，未重启现有 API；最近业务库已发布模板数量仍为 0。Task 0 和二阶段总门禁保持 OPEN。

本次没有新增依赖、数据库迁移、公开发布 API 或前端运行时代码。保留既有工作区改动，未提交或推送。

## 交付内容

- `server/internal/importjob/attendance_template.go`：单工作表、前两行隐藏元数据、第三行中文表头、第四行说明、第五行起录入；十列与现有解析器共用字段定义，输入区采用文本格式，保留学号前导零。固定 ZIP 元数据使相同代码生成相同文件。
- `server/internal/importjob/template_publisher.go`：校验启用中的超级管理员、生成文件、通过现有文件服务存储及读取校验，复用可验证的文件；重复执行仍检查实际对象。
- `server/internal/store/template_publication.go`：事务内串行化发布，重验账号/检查类型/文件状态，模板与发布审计一并提交。已有不同编码、归属、版本或未发布状态拒绝覆盖。
- `server/cmd/publish-attendance-template`：默认预览；只有显式 `--apply` 才连接并写入目标服务，要求账号与原因。

## 使用方式

从 `server/` 运行，先使用只预览命令核对模板：

```bash
GOTOOLCHAIN=go1.25.0 go run ./cmd/publish-attendance-template
GOTOOLCHAIN=go1.25.0 go run ./cmd/publish-attendance-template --output /tmp/attendance-v1.xlsx
```

输出路径的父目录须已存在；已有文件拒绝覆盖，新文件权限为 0600。已在 `output/templates/attendance-v1.xlsx` 生成可检查样本（忽略提交的生成产物，不是模板发布证据）。两次独立进程预览一致：26886 字节，SHA-256 为 `4acf90bcc8862189ac22e3827cead869e5597255b763400fbb82bb44f8a72386`。

发布前核对 API 实际连接的数据库和 Bucket，确认迁移、检查类型及操作账号，取得该目标环境的发布授权。通过受控环境注入 `DATABASE_URL`、`MINIO_ENDPOINT`、`MINIO_ACCESS_KEY`、`MINIO_SECRET_KEY`、`MINIO_BUCKET`、`MINIO_USE_TLS`，可选 `MINIO_REGION`；命令不会自动读取 `.env`、执行迁移、创建账号或 Bucket。MinIO 使用应用账号，不使用根凭据。

```bash
GOTOOLCHAIN=go1.25.0 go run ./cmd/publish-attendance-template \
  --apply --actor-id '<启用中的超级管理员 UUID>' --reason '初始化到课率导入模板 v1'
```

这是持有受信数据库凭据的运维命令；`--actor-id` 用于审计归属和账号状态检查，不是新的登录认证接口。不要在终端历史、文档或输出中粘贴凭据。

发布后仍须通过业务 API 和实际角色验证模板下载、上传、预览、确认入库及列表结果。命令成功不代表浏览器总验收成功。

## 实际验证

- `gofmt`、`go vet ./...`、`go test ./...` 通过。默认 Go 套件会跳过缺少外部服务配置的集成测试；部分未变更包使用缓存，不把它们计作本次真实集成证据。
- `GOFLAGS=-race GOCACHE=/tmp/four-check-progress-go-cache python3 server/scripts/verify-integration.py`：四包退出码均为 0；store 50、storage 21、importjob 50、template-cli 3 项通过，0 FAIL、0 SKIP，包含子测试计数。
- 最终 JSON 报告位于临时目录 `/var/folders/mz/h5hgqzbx0y51n9rr4gs_hqv00000gn/T/four-check-integration-results-_olq91mu`，可能被系统清理；脚本可重复执行并创建新的隔离资源。
- 模板生成测试覆盖重复字节一致、空模板解析、十列契约、第五行数据和前导零。
- 真实 PostgreSQL/MinIO 回归覆盖未授权账号、发布审计故障导致模板事务回滚、保留文件重试、四路并发重试、重复发布、错误编码/检查类型拒绝、对象篡改拒绝，以及两条虚构记录上传→预览→确认入库。
- 实际 CLI 覆盖预览无需数据库、输出不覆盖、参数拒绝、真实重复发布只保留同一模板/文件/发布审计，以及停用账号拒绝。

检查类型归属的负向测试曾真实失败：已有模板错误绑定其他检查类型仍被接受；现已在仓储读取时核验类型及启用状态，最终隔离回归通过。

## 边界与后续

模板数据库事务与对象存储并非跨系统原子事务。发布失败后保留已上传文件和审计以支持重试；首次发布同时启动多个进程，可能留下未引用的同内容文件，但只有一个发布模板及对应发布审计。本轮并发证据针对已有保留文件的失败重试，不能宣称首次并发上传不会产生额外文件。不自动删除文件、审计或修复已损坏的历史对象。

本记录对应的发布命令实现批次未跑 Playwright。后续已补隔离环境模板下载→上传→预览→确认、记录维护与连续导入的真实浏览器验证，见[09-21 E2E 补验](2026-09-21-expanded-e2e.md)。目标环境交付及角色总验收仍未完成。

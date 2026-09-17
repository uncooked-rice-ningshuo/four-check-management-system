# 计算机学院日常管理系统

当前已完成阶段 1 本轮确认范围：React/Ant Design 前端、Go API、PostgreSQL、私有 MinIO 对象存储和 Docker Compose 本地环境，以及账号登录与注册审核、用户全生命周期、权限计算与来源解释、部门与角色、七类检查类型映射、学年学期周次、十二类基础数据（含年级、宿舍区、宿舍楼和教学楼）、字典管理和追加式审计基础设施。

基础数据当前支持人工新增、编辑、启停、按名称/状态/关联字段筛选和变更记录。院系、年级、专业、班级、宿舍层级、教室和课程统一使用后端生成的 UUID，管理员不再填写人工编码；学号、工号、宿舍号和教室位置仍作为真实业务信息维护。课程支持多个班级共同上课，并按“学期 + 星期 + 第几节 + 教室”维护结构化排课，每天最多 10 节、每节 45 分钟。Excel 的“上传解析预览 → 确认入库”两阶段导入属于阶段 2。学生身份证号在采集必要性和明文查看权限定稿前不采集。

## 当前实施状态（2026-09-06）

阶段 2 在 `feat-0.1-2` 的现有工作区继续，尚未整体完成。已实现到课率查询与两阶段导入、统一侧边栏和真实数据工作台；已补到课率手工新增、未提交草稿编辑及单条/批量软删除；其余六类检查、退回记录修正、部门提交审核流程及完整跨角色验收仍待补齐。

工作台统一位于 `/`；管理员访问 `/admin` 经管理守卫后重定向到 `/`，具体管理页面保留独立鉴权。“我的待办”统计本人创建的草稿和退回记录；部门状态展示完整授权范围内的部门整体数据。最新决策、任务状态、验证边界及下一步顺序见[交接记录](docs/plans/2026-09-06-project-handover.md)。到课率 CRUD 与工作台的最新[真实浏览器增量验收](docs/plans/2026-09-06-attendance-browser-verification.md)已补齐，完整 MinIO 与跨角色总门禁仍开放。

## 本地运行

```bash
cp .env.example .env
# 编辑 .env，为管理员、MinIO 根账号和 MinIO 应用账号分别设置独立强凭证
docker compose up --build
```

打开 <http://localhost:8088>。首次启动会创建数据库表、私有 Bucket、应用账号和最小权限策略。Compose 通过 `schema_migrations` 记录已执行的 SQL 文件，避免重启时重复执行结构迁移。迁移 `009_server_managed_master_data.sql` 会原位回填年级 UUID 和课程班级关系；无法可靠自动解析的旧排课文本会保存在 `course_schedule_migration_backlog`，等待管理员确认后再录入结构化排课，不会清空既有业务或变更历史。MinIO API 与 Console 不映射到宿主机端口，浏览器不能绕过 API 直接访问对象。

Compose 从已包含安全修复的 `RELEASE.2025-10-15T17-29-55Z` 源码构建固定版本 MinIO Server；这是社区版停止发布预构建镜像后的最终安全发行版。生产环境可以连接独立或分布式 MinIO，但必须使用相同环境变量模型、TLS、私有 Bucket 和独立应用账号，不能使用根账号运行 API。

首次启动会幂等创建 `admin` 系统账号，不要求学号且不会在后续启动时覆盖密码。首次登录必须修改初始密码。普通用户注册后进入 `PENDING`，由超级管理员审核通过后启用。

## 测试

```bash
docker run --rm -v "$PWD/server:/app" -w /app golang:1.25-alpine go test ./...
cd web && pnpm test && pnpm build
docker compose config
```

到课率导入的 PostgreSQL + MinIO 集成测试默认跳过；执行时必须连接可清空的临时数据库和测试专用 MinIO 账号，不得指向生产或共享数据环境。测试会执行仓库全部迁移、写入测试数据，并创建后自动清理独立 Bucket：

```bash
cd server
TEST_DATABASE_URL='postgres://<user>:<password>@127.0.0.1:15432/<temporary_database>?sslmode=disable' \
TEST_MINIO_ENDPOINT='127.0.0.1:19090' \
TEST_MINIO_ACCESS_KEY='<test_access_key>' \
TEST_MINIO_SECRET_KEY='<test_secret_key>' \
TEST_MINIO_USE_TLS='false' \
go test ./internal/importjob ./internal/storage \
  -run 'Integration|TestMinIOContractAgainstDisposableServer' -count=1 -v
```

该命令覆盖真实 XLSX 解析与预览、范围不匹配拒绝、确认入库和审计、对象完整性异常，以及 MinIO 凭据错误和服务不可用等失败路径。

## MinIO 配置与安全边界

所有环境只使用 MinIO，不提供本地目录存储分支。`.env` 中必须为以下变量注入独立凭据或配置：

- `MINIO_ROOT_USER`、`MINIO_ROOT_PASSWORD`：只供 `minio-init` 创建 Bucket、策略和应用账号，API 不接收根凭据。
- `MINIO_ACCESS_KEY`、`MINIO_SECRET_KEY`：API 使用的最小权限应用账号，不得与根账号相同。
- `MINIO_ENDPOINT`、`MINIO_USE_TLS`、`MINIO_BUCKET`、`MINIO_REGION`：生产必须使用 TLS 和受信任证书。
- `MINIO_MAX_FILE_SIZE_BYTES`、`MINIO_CONNECT_TIMEOUT`、`MINIO_OPERATION_TIMEOUT`：必须为正数并在启动时校验。

生产环境不得公开 MinIO API 或 Console，也不得启用匿名访问或让浏览器直连。Bucket 必须开启版本控制；对象 key 由服务端生成，原始文件名不参与路径生成。

## 审计动作

审计日志为数据库追加写，普通业务接口不能更新或删除。当前稳定动作包括：

- `REGISTRATION_REVIEWED`
- `ACCOUNT_CREATED`、`ACCOUNT_UPDATED`、`ACCOUNT_STATUS_CHANGED`、`ACCOUNT_PASSWORD_RESET`
- `ORGANIZATION_UPDATED`、`PERMISSION_RULE_CHANGED`
- `ACADEMIC_CALENDAR_CHANGED`、`MASTER_DATA_CHANGED`、`DICTIONARY_CHANGED`
- `INSPECTION_RECORD_CREATED`、`INSPECTION_RECORD_UPDATED`、`INSPECTION_RECORD_DELETED`
- `FILE_UPLOAD_SUCCEEDED`、`FILE_UPLOAD_FAILED`、`FILE_READ`、`FILE_INTEGRITY_ANOMALY`

审计记录可包含操作者、请求 ID、可信客户端 IP、截断后的 User-Agent、原因以及脱敏后的前后值。禁止记录密码、密码哈希、Token、Cookie、MinIO Secret、请求体、上传文件原文、完整身份证号或其他不必要的敏感内容。

## 联合备份与恢复

PostgreSQL 元数据与 MinIO 对象必须使用同一备份批次标识，并把清单、数据库 dump 和对象副本保存在同一受控备份目录。以下命令中的凭据只从环境或 Secret 注入，不写入脚本和仓库。

```bash
backup_id="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "backups/$backup_id/minio"

# PostgreSQL 逻辑备份（宿主机需安装 PostgreSQL 客户端）
PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h 127.0.0.1 -U four_check -d four_check \
  --format=custom --file="backups/$backup_id/postgres.dump"

# MinIO 当前对象逻辑备份；版本历史另行使用版本感知复制或卷快照保护
docker compose run --rm -v "$PWD/backups/$backup_id:/backup" --entrypoint /bin/sh minio-init -c '
  mc alias set source http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
  mc mirror --overwrite "source/$MINIO_BUCKET" /backup/minio
  mc find "source/$MINIO_BUCKET" --exec "mc stat --json {}" > /backup/minio-manifest.jsonl
'
```

恢复必须先在空 PostgreSQL 数据库和空、私有且已启用版本控制的 Bucket 中演练：

```bash
PGPASSWORD="$POSTGRES_PASSWORD" createdb -h 127.0.0.1 -U four_check four_check_restore
PGPASSWORD="$POSTGRES_PASSWORD" pg_restore -h 127.0.0.1 -U four_check \
  -d four_check_restore --clean --if-exists "backups/$backup_id/postgres.dump"

docker compose run --rm -v "$PWD/backups/$backup_id:/backup:ro" --entrypoint /bin/sh minio-init -c '
  mc alias set target http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
  mc mb --ignore-existing "target/$MINIO_BUCKET"
  mc version enable "target/$MINIO_BUCKET"
  mc anonymous set none "target/$MINIO_BUCKET"
  mc mirror --overwrite /backup/minio "target/$MINIO_BUCKET"
'
```

恢复后必须抽样读取 `files.status='AVAILABLE'` 的对象，重新计算 SHA-256，并与数据库 `files.sha256` 对比；任何缺失或哈希不一致都视为恢复失败。普通 `mc mirror` 只恢复当前对象，不代替版本历史复制或底层卷快照。

## API

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET|POST|PUT /api/v1/admin/master-data/:kind`
- `GET|POST|PUT /api/v1/admin/dictionaries`
- `GET|POST|PUT|DELETE /api/v1/admin/users/:id/permission-rules`
- `GET /api/v1/dashboard/workbench`
- `GET /api/v1/inspections/attendance`
- `POST /api/v1/inspections/attendance`（手工新增草稿）
- `GET|PATCH|DELETE /api/v1/inspections/:id`（到课率编辑详情、编辑、软删除）
- `POST /api/v1/inspections/batch`（到课率事务批量软删除）
- `GET /health/live`
- `GET /health/ready`

Access Token 只由前端保存在内存；Refresh Token 使用 HttpOnly、SameSite=Lax Cookie，并在刷新时轮换。

MinIO 根凭证只供 `minio-init` 初始化使用，API 仅接收 `MINIO_ACCESS_KEY` 和 `MINIO_SECRET_KEY`。Bucket 启用版本控制并关闭匿名访问；对象 key 和底层存储错误不会作为公共 API 字段返回。

### 工作台 PostgreSQL 聚合验证

`TEST_DATABASE_URL` 指向测试专用 PostgreSQL 后执行：

```bash
cd server
GOTOOLCHAIN=go1.25.0 go test ./internal/store -run TestDashboardPostgresIntegration -count=1 -v
```

该测试使用单连接的临时表，不执行迁移；覆盖本人草稿/退回计数、他人记录排除、局部允许与拒绝、部门整体计数、无授权和跨周隔离。未设置测试数据库时明确 SKIP。它不替代完整迁移、工作台 API/角色组合及浏览器端到端验收。

### 到课率手工录入

到课率页面的“新增记录”在抽屉中独立选择学期、周次、目标部门和班级，填写学号、姓名、课程名称、教室位置、检查日期/时间、节次、旷课学时和备注。课程名称及教室位置应与基础数据一致，服务端复用 XLSX 导入校验。外层部门筛选不作为新增门禁。

`POST /api/v1/inspections/attendance` 接收 `semesterId,weekId,inspectionTypeId,departmentId,classId,grade,className,studentNo,studentName,courseName,classroom,date,time,period,absenceHours,remark`；日期为 `YYYY-MM-DD`，时间为北京时间 `HH:MM`。学时大于 0 且不超过 24，节次最多 64 字，备注最多 500 字。服务端解析并核对快照，强制来源 `MANUAL`、状态 `DRAFT`、版本 1，记录与 `INSPECTION_RECORD_CREATED` 审计同事务写入。

成功返回 201；无权限返回 403 `FORBIDDEN_SCOPE`；重复返回 409 `RECORD_ALREADY_EXISTS`；基础数据/日期校验失败返回 422 `VALIDATION_FAILED`。唯一约束沿用导入去重键，重复请求不会重复写入。保存成功刷新列表和工作台；409 刷新列表并保留表单输入。

真实数据库测试必须使用**一次性空库**，会应用全部迁移并写入虚构数据，不得指向共享/业务数据库：

```bash
cd server
MANUAL_TEST_DATABASE_URL='<一次性空库连接串>' GOTOOLCHAIN=go1.25.0 \
  go test ./internal/importjob -run TestManualAttendancePostgresIntegration -count=1 -v
```

该测试不依赖 MinIO，覆盖手工新增、去重、权限拒绝、姓名/日期校验和审计失败回滚。未配置时明确 SKIP。

### 到课率编辑与软删除

`INSPECTION_ENTER` 允许处理有效范围内本人草稿，独立 `INSPECTION_MANAGE` 允许处理有效范围内他人草稿。部长/副部长在允许录入的部门类型映射下默认取得管理权，也可通过权限规则单独授予或拒绝。动作分别应用拒绝规则；管理权不作为新增记录的录入权。

编辑使用 `PATCH /api/v1/inspections/:id`，请求 `{version,reason,record}`，其中 `record` 与手工新增字段一致。单条删除请求 `{version,reason}`；批量请求 `{action:"DELETE",items:[{id,version}],reason}`，最多 100 条。原因必填且最多 255 字。详情接口用于编辑授权，不是通用只读详情接口。

只允许未提交且无通报锁定关联的 DRAFT；保留创建人、来源和导入关联。版本冲突返回 409 `VERSION_CONFLICT`，状态不允许返回 409 `RECORD_NOT_EDITABLE`，不存在或原范围无权返回 404 `INSPECTION_NOT_FOUND`，目标范围无权返回 403 `FORBIDDEN_SCOPE`。批量操作任一失败全部回滚，软删除保留明细和审计。

真实事务验证需要独立的一次性空库（应用全部迁移并写入虚构数据），不可复用业务库：

```bash
cd server
MUTATION_TEST_DATABASE_URL='<一次性空库连接串>' GOTOOLCHAIN=go1.25.0 \
  go test ./internal/importjob -run TestAttendanceMutationsPostgresIntegration -count=1 -v
```

未配置时明确 SKIP；测试不依赖 MinIO。

### 七类字段目录与导入重复反馈

七类字段决策已确认并实现前后端一致的元数据目录，详见[字段契约](docs/plans/2026-09-06-seven-inspection-field-review.md)。到课率/无机课堂共享扩展表，早锻炼/早签保持单一类型，卫生按对象检查记录并关联责任人/班级；其余六类的迁移、表单和接口仍待实施。

到课率预览对批内重复提示来源行与首条冲突行，对库内重复提示当前来源行，并展示本行学生、课程、日期时间及节次以便核对。重复不覆盖已有数据；如需修改，使用已有记录的版本校验编辑流程。不会返回无权查看的旧记录详情。

### 012 检查扩展迁移

最新追加迁移为 `012_phase_two_inspection_extensions.sql`，已补齐五张业务扩展表、卫生区域和责任关联结构。Task 3 领域校验及 Task 4/5 七类通用 API、未提交草稿编辑/软删除/批删已实现。其余类型页面、Excel 适配器及退回修正仍待后续任务。[API 契约与调用方迁移](docs/plans/2026-09-06-inspection-api-contract.md)说明了范围规则、请求格式和到课率专用详情路径。

迁移验证：将 `EXTENSION_TEST_DATABASE_URL` 指向一次性空 PostgreSQL 库，在 `server/` 执行 `GOTOOLCHAIN=go1.25.0 go test ./internal/store -run TestPhaseTwoExtension -count=1 -v`。测试拒绝已有 accounts 表的数据库。已在独立临时库通过迁移、约束和历史数据保留验证。

### 工作上下文（2026-09-07）

最新迁移为 `013_work_context_settings.sql`。统一外壳展示服务端工作周次；超级管理员可设置带到期时间的全局临时周次或恢复校历。版本冲突明确拒绝、变更写审计，到期自动回退，既有记录不改变。工作台与默认周次共用该结果。导航已补齐完全 DENY 的处理。[说明与验证](docs/plans/2026-09-07-navigation-work-context.md)。下一步为 Task 8/9 部门提交状态机与 API，其迁移编号暂排 014。

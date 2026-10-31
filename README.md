# 计算机学院日常管理系统

阶段 1 基础平台已有实现：React/Ant Design 前端、Go API、PostgreSQL、私有 MinIO 对象存储和 Docker Compose 配置，以及账号登录与注册审核、用户全生命周期、权限计算与来源解释、部门与角色、七类检查类型映射、学年学期周次、十二类基础数据（含年级、宿舍区、宿舍楼和教学楼）、字典管理和追加式审计基础设施。代码实现不等于当前运行环境或上线验收已通过。

基础数据当前支持人工新增、编辑、启停、按名称/状态/关联字段筛选和变更记录。院系、年级、专业、班级、宿舍层级、教室和课程统一使用后端生成的 UUID，管理员不再填写人工编码；学号、工号、宿舍号和教室位置仍作为真实业务信息维护。课程支持多个班级共同上课，并按“学期 + 星期 + 第几节 + 教室”维护结构化排课，每天最多 10 节、每节 45 分钟。Excel 的“上传解析预览 → 确认入库”两阶段导入属于阶段 2。学生身份证号在采集必要性和明文查看权限定稿前不采集。

## 当前实施状态（2026-10-05）

阶段 2 在 `feat-0.1-2` 的现有工作区继续，五类代码与隔离交付已完成，总门禁等待用户验收。七类检查已有字段、统一 API 和手工草稿页面；到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归五类是当前开发与验收范围，教室卫生和公共宿舍卫生后续规则待需求方确认，已有能力保留以便未来接入。五类导入、整批回滚、按成员批次提交、部门按周汇总、退回更正、重提、确认及工作台批次统计已有代码和隔离回归证据。本机业务库迁移已到 `021`，校级无部门记录进入独立校级批次，到课率按每节 1 实际学时、周配置 1/2 倍计算违纪计入学时；本机 API 已重载并返回就绪。Task 17/18 的五类隔离验收已有成套证据，目标环境登录态业务验收和 Task 19 已执行，总门禁等待目标登录态业务验收。具体状态和下一步见[项目状态](docs/project-status.md)。

工作台统一位于 `/`；管理员访问 `/admin` 经管理守卫后重定向到 `/`，具体管理页面保留独立鉴权。“我的待办”统计本人创建且有权查看的草稿和退回记录；部门状态展示完整授权范围内的记录数与批次数。[09-06 浏览器验收](docs/plans/2026-09-06-attendance-browser-verification.md)及[09-21 补验](docs/reviews/2026-09-21-expanded-e2e.md)是历史增量证据。最新五类部门流程的隔离证据见[验收记录](docs/reviews/2026-10-04-department-submission-foundation.md)。

**目标运行证据边界：** 10-05 在本机联调权限下确认业务 API 8080、前端 5173 与 PostgreSQL 5432 正在运行；备份后补齐 014–021，本机 API 已重载，`/health/ready` 返回 200。普通沙箱下的端口探测误报拒绝连接，不能据此判断服务停机。浏览器通过的是一次性 PostgreSQL/MinIO 和当前源码构成的隔离环境；目标库四类只读仓储查询通过，用户已确认四类已登录页面刷新后无错误，五类模板已登录下载成功。最终代码的目标登录态业务写入和完整角色验收仍待用户自主执行。

## 本地运行

以下为 Compose 启动方式，不是当前机器正在正常运行的证明。已有 `.env` 不要覆盖；该命令会启动服务并应用目标 Compose 数据库的待执行迁移，运行前按 AGENTS.md 核对授权、数据及连接目标。

```bash
test -f .env || cp .env.example .env
# 编辑 .env，为管理员、MinIO 根账号和 MinIO 应用账号分别设置独立强凭证
docker compose up --build
```

打开 <http://localhost:8088>。首次启动会创建数据库表、私有 Bucket、应用账号和最小权限策略。Compose 通过 `schema_migrations` 记录已执行的 SQL 文件，避免重启时重复执行结构迁移。迁移 `009_server_managed_master_data.sql` 会原位回填年级 UUID 和课程班级关系；无法可靠自动解析的旧排课文本会保存在 `course_schedule_migration_backlog`，等待管理员确认后再录入结构化排课，不会清空既有业务或变更历史。MinIO API 与 Console 不映射到宿主机端口，浏览器不能绕过 API 直接访问对象。

Compose 从已包含安全修复的 `RELEASE.2025-10-15T17-29-55Z` 源码构建固定版本 MinIO Server；这是社区版停止发布预构建镜像后的最终安全发行版。生产环境可以连接独立或分布式 MinIO，但必须使用相同环境变量模型、TLS、私有 Bucket 和独立应用账号，不能使用根账号运行 API。

首次启动会幂等创建 `admin` 系统账号，不要求学号且不会在后续启动时覆盖密码。首次登录必须修改初始密码。普通用户注册后进入 `PENDING`，由超级管理员审核通过后启用。

首次启动**不会自动交付已发布的到课率模板**。模板需要文件对象、元数据、版本与发布配置齐全；已新增默认只预览、显式 `--apply` 才发布的运维命令。10-05 已按用户授权在这台机器的 `four_check` 业务库发布当前五类 v1 模板，并核对文件与对象摘要；其他环境仍须单独发布和验收。见[模板交付说明](docs/reviews/2026-09-21-attendance-template-publication.md)及[本机修复记录](docs/reviews/2026-10-05-four-list-layout-and-runtime-fix.md)。

本地开发的 Vite 代理配置为 `5173 → localhost:8080`；Compose 对外入口为 `8088`，内部 API 为 `api:8080`，内部数据库为 `postgres:5432`。宿主机原生 PostgreSQL 与 Compose 数据库可能并存，迁移前必须核对 API 实际目标，不能只看端口或容器名。存活检查不等于就绪；还需 `/health/ready` 和已登录业务验证。不要为排障输出 `.env` 全文、Token 或完整数据库连接串。

## 前端静态素材

前端公共静态文件位于 `web/public/`。`logo.png` 是页面主标识和浏览器标签图标；`assets/backgrounds/` 保存登录、注册场景背景，`assets/brand/` 保存其他品牌版本，`assets/characters/` 保存人物和头像。页面通过以 `/` 开头的路径引用这些文件，Vite 和 Web 镜像构建从该目录复制素材。根目录中文素材文件夹保留为临时来源，不参与前端构建。

## 测试

先核对 `node --version`、`pnpm --version` 与 `web/package.json`。当前指定 pnpm@11.7.0；09-20 已用本机现有 Node 24.12.0 与 pnpm 11.7.0 完成验证，默认 PATH 仍可能指向旧版，执行前须自行核对。外部服务 SKIP、超时或中断必须单列。

```bash
(cd server && GOTOOLCHAIN=go1.25.0 go vet ./... && GOTOOLCHAIN=go1.25.0 go test ./...)
(cd web && pnpm lint && pnpm test && pnpm build)
docker compose config --quiet
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
- `POST /api/v1/admin/users`：请求包含学号、姓名、系统角色；服务端生成临时密码，201 响应额外返回 `temporaryPassword`，仅展示一次。
- `POST /api/v1/admin/users/:id/reset-password`：无需传入密码；200 响应返回新生成的 `temporaryPassword`。两种响应均设置 `Cache-Control: no-store`，不得记录或长期缓存密码。
- `GET|POST|PUT /api/v1/admin/master-data/:kind`
- `GET|POST|PUT /api/v1/admin/dictionaries`
- `GET|POST|PUT|DELETE /api/v1/admin/users/:id/permission-rules`
- `GET /api/v1/dashboard/workbench`
- `GET /api/v1/inspections/attendance`
- `POST /api/v1/inspections/attendance`（手工新增草稿）
- `GET|POST /api/v1/inspections`（七类统一列表、创建）
- `GET|PATCH|DELETE /api/v1/inspections/:id`（七类统一详情、草稿编辑、软删除）
- `POST /api/v1/inspections/batch`（七类草稿事务批量软删除）
- `GET|PATCH|DELETE /api/v1/inspections/attendance/:id`（现有到课率页面专用平铺契约）
- `POST /api/v1/inspections/attendance/batch`（现有到课率页面批量删除）
- `GET|PUT /api/v1/work-context`（工作周读取、受权临时覆盖/恢复）
- `GET /health/live`
- `GET /health/ready`

Compose 的 API 健康检查使用 `/health/ready`，前端启动依赖 API 的 `service_healthy`：PostgreSQL 或对象存储未就绪时不放行启动。`/health/live` 只用于判断进程存活。Compose 不会因运行后的依赖失效自动停止已启动的前端；模板发布和业务初始化仍需单独验收。

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

现有到课率页面编辑使用 `PATCH /api/v1/inspections/attendance/:id`，请求 `{version,reason,record}`，其中 `record` 与手工新增字段一致。单条删除用同一专用路径，请求 `{version,reason}`；批量使用 `/api/v1/inspections/attendance/batch`，请求 `{action:"DELETE",items:[{id,version}],reason}`，最多 100 条。原因必填且最多 255 字。该专用详情用于编辑授权；通用 `/inspections/:id` 则使用 INSPECTION_VIEW 并返回统一 details 结构，不能混用。见[API 契约](docs/plans/2026-09-06-inspection-api-contract.md)。

只允许未提交且无通报锁定关联的 DRAFT；保留创建人、来源和导入关联。版本冲突返回 409 `VERSION_CONFLICT`，状态不允许返回 409 `RECORD_NOT_EDITABLE`，不存在或原范围无权返回 404 `INSPECTION_NOT_FOUND`，目标范围无权返回 403 `FORBIDDEN_SCOPE`。批量操作任一失败全部回滚，软删除保留明细和审计。

真实事务验证需要独立的一次性空库（应用全部迁移并写入虚构数据），不可复用业务库：

```bash
cd server
MUTATION_TEST_DATABASE_URL='<一次性空库连接串>' GOTOOLCHAIN=go1.25.0 \
  go test ./internal/importjob -run TestAttendanceMutationsPostgresIntegration -count=1 -v
```

未配置时明确 SKIP；测试不依赖 MinIO。

### 七类字段目录与导入重复反馈

七类字段目录与通用 API 已建立，详见[字段契约](docs/plans/2026-09-06-seven-inspection-field-review.md)。到课率/无机课堂共享扩展表，早锻炼/早签保持单一类型，卫生按对象检查记录并关联责任人/班级。当前后续开发只推进到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归五类；教室卫生和公共宿舍卫生的后续方案待需求方确认，既有代码与未来通报接入能力保留。当前完成度与目标环境证据见[项目状态](docs/project-status.md)，代码目录的 ACTIVE 不证明目标库已有发布模板。

到课率预览对批内重复提示来源行与首条冲突行，对库内重复提示当前来源行，并展示本行学生、课程、日期时间及节次以便核对。重复不覆盖已有数据；如需修改，使用已有记录的版本校验编辑流程。不会返回无权查看的旧记录详情。

### 012 检查扩展迁移

`012_phase_two_inspection_extensions.sql` 补齐五张业务扩展表、卫生区域和责任关联结构；016/017 提供五类部门多批次提交与历史修订不可变，018/019 保存周处罚倍数与到课率记录快照，020 设置未标记周默认 1 倍，021 支持独立校级记录及批次。通用 API、七类手工草稿页面、五类当前范围的导入及部门流程代码已接入；本机业务库账本已到 021，完整角色总验收仍未完成。[API 契约与调用方迁移](docs/plans/2026-09-06-inspection-api-contract.md)说明了范围规则与请求格式。

迁移验证：将 `EXTENSION_TEST_DATABASE_URL` 指向一次性空 PostgreSQL 库，在 `server/` 执行 `GOTOOLCHAIN=go1.25.0 go test ./internal/store -run TestPhaseTwoExtension -count=1 -v`。测试拒绝已有 accounts 表的数据库。已在独立临时库通过迁移、约束和历史数据保留验证。

### 工作上下文（2026-09-07）

`013` 建立服务端工作周次，`014` 处理整批回滚，`015` 记录失败文件对象清理进度，当前最新的 `016/017` 建立部门提交及不可变修订。统一外壳展示服务端工作周次；超级管理员可设置带到期时间的全局临时周次或恢复校历。版本冲突明确拒绝、变更写审计，到期自动回退，既有记录不改变。工作台按当前周分别显示记录和提交批次数。导航已补齐完全 DENY 的处理。[工作上下文历史验证](docs/plans/2026-09-07-navigation-work-context.md)与[当前后续顺序](docs/project-status.md)分别记录证据和待办。

### 隔离数据库与对象存储检查

本机已有 Docker、`postgres:16-alpine` 镜像、MinIO 二进制、Python 3 和 Go 时，从仓库根目录运行：

```bash
GOTOOLCHAIN=go1.25.0 python3 server/scripts/verify-integration.py
```

脚本使用随机凭据、仅绑定回环地址的临时 PostgreSQL 容器和 MinIO 进程，不读取 `.env`，不使用业务数据库或已有 Bucket。各迁移测试使用独立空库，覆盖 store/storage/importjob 和模板发布 CLI 的真实集成检查；任一包失败返回非零。结束后清理本次资源，JSON 报告保留在启动时打印的临时目录。它不启动业务 API，也不发布业务模板。

### Playwright 浏览器回归

已加入校历、到课率及基础平台回归，可从 `web/` 执行：

```bash
pnpm test:browser
```

入口是 `server/scripts/verify-browser.py`，用例位于 `web/e2e/`（校历、模板缺失、到课率及重传恢复、基础管理及账号流程）。需要 Node 24、Python 3、Go 1.25、Docker 及本地 `postgres:16-alpine` 镜像、`minio`、`mc`、Google Chrome，以及已经安装好的前端依赖。默认通过 npx 获取固定版本 `@playwright/cli@0.1.21`，首次需要网络；已有 CLI 时可通过 `PLAYWRIGHT_CLI_JS=/绝对路径/playwright-cli.js` 指定。没有增加生产依赖，也不要求安装 `@playwright/test`。

脚本构建当前 API，启动全新 PostgreSQL/MinIO 和独立 Vite，使用随机测试账号完成真实登录与首次改密；通过发布命令交付隔离模板，验证下载字节、上传/预览/确认、草稿维护、连续导入、错误批次重传、真实 409 与确认响应丢失后的幂等恢复、基础管理代表性写操作、工作周覆盖、注册审核和禁用会话。API 仅使用受限 MinIO 应用账号。测试结束后清理本次服务，退出码反映成功或失败；报告、截图和快照在忽略提交的 `output/playwright/<run-id>/`。脚本不读取业务 `.env`，不连接业务数据库，不记录密码、Cookie 或令牌，不保存认证阶段的 trace。

覆盖弹窗回显、真实日期冲突与重试、工作周即时刷新、失败输入保留和重载持久化；500/异常响应是明确的故障注入。到课率和代表性管理流程已有增量 E2E，不代表阶段二全部角色或七类页面验收完成。结果与边界见[最新 E2E 记录](docs/reviews/2026-09-21-import-recovery.md)，原校历结果见[历史记录](docs/reviews/2026-09-20-playwright-regression.md)。

当前补验范围、发现及下一步见 [09-21 E2E 记录](docs/reviews/2026-09-21-expanded-e2e.md) 和 [实际实施顺序](docs/plans/2026-09-21-next-implementation.md)。十二类列表加载不等于十二类全部管理动作验收，隔离通过不等于目标业务环境通过。

无机课堂隔离浏览器聚焦验证：在原 `verify-browser.py` 环境配置前加 `BROWSER_SUITE=unattended-classroom`，仍使用一次性 PostgreSQL/MinIO/API/Vite，并保留登录与校历前置回归。验证边界见[页面记录](docs/reviews/2026-09-21-unattended-classroom-page.md)。

晚自习专属浏览器验收：从 `web/` 执行 `BROWSER_SUITE=evening-self-study pnpm test:browser`。该用例覆盖页面录入、提交、部门审核、录入/审核/越权边界以及进入周通报计数和明细；Excel 模板映射和导入事务另由晚自习 PostgreSQL 集成测试覆盖，尚不代表浏览器上传 UI 或目标环境模板已验收。

### 有界文件恢复命令

数据库须已应用015。已配置目标`DATABASE_URL`与`MINIO_BUCKET`后，在`server/`运行只读预览：

```bash
go run ./cmd/reconcile-files --older-than 1h --limit 100
```

预览仅查询当前Bucket下过期的PENDING/FAILED文件，输出文件ID、状态、大小和更新时间，不连接MinIO、不改状态或审计。后续执行会重新查询候选；预览不是锁定清单。

明确核对目标并取得对应环境操作授权后，配置API使用的MinIO凭据、TLS、Bucket和`MINIO_MAX_FILE_SIZE_BYTES`，显式执行一个批次：

```bash
go run ./cmd/reconcile-files --apply --actor-id '<启用中且已完成改密的超级管理员UUID>' --reason '恢复原因' --older-than 1h --limit 100 --timeout 60s
```

`older-than`至少5分钟，`limit`为1–1000，`timeout`为1秒–5分钟。PENDING完整对象恢复为AVAILABLE；确定缺失、长度异常、超限才进入FAILED；临时读故障保留并延后。FAILED删除成功留下清理标记与审计，不再重复入选。每次只处理一个批次，有失败时输出计数并退出1；修复原因后再次预览/执行。命令不会扫描整个Bucket、处理AVAILABLE孤立文件或自动建立定时任务。[设计](docs/plans/2026-10-04-file-reconciliation-cli-design.md)与[验收](docs/reviews/2026-10-04-file-reconciliation-cli.md)。

# 计算机学院日常管理系统

当前已完成阶段 1 本轮确认范围：React/Ant Design 前端、Go API、PostgreSQL、私有 MinIO 对象存储和 Docker Compose 本地环境，以及账号登录与注册审核、用户全生命周期、权限计算与来源解释、部门与角色、七类检查类型映射、学年学期周次、八类基础数据、字典管理和追加式审计基础设施。

基础数据当前支持人工新增、编辑、启停、查询和变更记录；Excel 的“上传解析预览 → 确认入库”两阶段导入属于阶段 2。学生身份证号在采集必要性和明文查看权限定稿前不采集。

## 本地运行

```bash
cp .env.example .env
# 编辑 .env，为管理员、MinIO 根账号和 MinIO 应用账号分别设置独立强凭证
docker compose up --build
```

打开 <http://localhost:8088>。首次启动会创建数据库表、私有 Bucket、应用账号和最小权限策略。MinIO API 与 Console 不映射到宿主机端口，浏览器不能绕过 API 直接访问对象。

Compose 从已包含安全修复的 `RELEASE.2025-10-15T17-29-55Z` 源码构建固定版本 MinIO Server；这是社区版停止发布预构建镜像后的最终安全发行版。生产环境可以连接独立或分布式 MinIO，但必须使用相同环境变量模型、TLS、私有 Bucket 和独立应用账号，不能使用根账号运行 API。

首次启动会幂等创建 `admin` 系统账号，不要求学号且不会在后续启动时覆盖密码。首次登录必须修改初始密码。普通用户注册后进入 `PENDING`，由超级管理员审核通过后启用。

## 测试

```bash
docker run --rm -v "$PWD/server:/app" -w /app golang:1.24-alpine go test ./...
cd web && pnpm test && pnpm build
docker compose config
```

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
- `GET /health/live`
- `GET /health/ready`

Access Token 只由前端保存在内存；Refresh Token 使用 HttpOnly、SameSite=Lax Cookie，并在刷新时轮换。

MinIO 根凭证只供 `minio-init` 初始化使用，API 仅接收 `MINIO_ACCESS_KEY` 和 `MINIO_SECRET_KEY`。Bucket 启用版本控制并关闭匿名访问；对象 key 和底层存储错误不会作为公共 API 字段返回。

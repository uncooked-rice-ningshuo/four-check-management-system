# 2026-10-06 腾讯云 IP 试运行升级

本次仅升级 `four-check-api`、`four-check-web` 为 `ip-trial-20261006-r1`，应用数据库迁移 014–021。沿用现有 PostgreSQL 16、MinIO、mc、`.env`、公网 IP 和 8088 端口。保留服务器上的其他应用。

## 已确认的目标

2026-10-06 用户提供的云端输出确认：项目 `four-check-ip-trial`，目录 `/opt/four-check/ip-trial`；旧应用镜像 `ip-trial-20260922`，四个运行容器健康；迁移账本 001–013；数据库 10 MB、MinIO 376 KB；根分区空闲 5.8 GB。卷名是 `four-check-ip-trial_postgres_data` 和 `four-check-ip-trial_minio_data`。这不是本地开发数据库。

新版从当前未提交工作区快照构建，不能仅用 Git HEAD 表示全部来源。包内附源文件摘要与镜像 ID。正式生产 HTTPS 及完整角色总验收不在本次 IP 试运行升级的完成声明内。

## 1. 上传与预检（不停服务）

在 1Panel 文件管理创建 `/opt/four-check/updates/20261006-r1`，上传本机生成的 `four-check-update-20261006-r1.tar` 和同名 `.tar.sha256`。不要上传本地 `.env`，不要覆盖原部署目录。

在云服务器执行：

```bash
(
  set -e
  umask 077
  cd /opt/four-check/updates/20261006-r1
  sha256sum -c four-check-update-20261006-r1.tar.sha256
  mkdir release
  tar -xf four-check-update-20261006-r1.tar -C release
  cd release
  sha256sum -c SHA256SUMS
  docker load -i images.tar.gz
  python3 upgrade.py
)
```

如果 `release` 已存在，先核对上一次执行结果，不要盲目覆盖。脚本校验原容器健康、项目身份、真实数据卷、旧镜像标签、历史迁移文件与账本、新镜像 ID/架构和剩余磁盘。任何一项不符即停止，默认模式不修改运行服务。

## 2. 进入维护窗口并升级

确认无人继续录入，保持终端连接，在云服务器执行：

```bash
cd /opt/four-check/updates/20261006-r1/release
python3 upgrade.py --apply
```

该命令会暂时中断 8088 的应用访问，按顺序：

1. 重新预检，保存原配置和迁移文件到独立私有备份目录。
2. 停 Web/API，使用 `pg_dump -Fc` 备份数据库，并检查备份目录。
3. 暂停 MinIO，把整个原数据卷只读打包，逐项读取归档校验后恢复 MinIO。
4. 补充迁移文件，通过新建一次性 migrate 容器执行缺失 SQL；每个迁移与账本写入处于同一事务。
5. 确认账本为 001–021，仅替换原 compose.yaml 的两个应用镜像标签。
6. 使用原项目执行 `up -d --no-deps --pull never api web`，等待四个容器健康及 `/health/ready` 返回 200。

备份路径会打印在终端，位于 `/opt/four-check/backups/before-ip-trial-20261006-r1-日期时间`。内含凭据与业务数据，不得公开上传或粘贴到聊天。保留旧镜像、备份、原发布包，暂不执行镜像清理。

失败时脚本保留 API/Web 停止状态并报告备份位置。迁移是逐文件原子执行，前面的文件可能已成功；不要反复执行 `--apply`，也不要只换回旧镜像。先核对迁移账本和私有 `migration.log` 再制定恢复步骤。不要执行 `down -v`、初始化 `.env`、清库或重建数据卷。

## 3. 验证云端

```bash
cd /opt/four-check/ip-trial
docker compose -p four-check-ip-trial ps -a
curl --noproxy '*' --fail http://127.0.0.1:8088/health/ready
docker compose -p four-check-ip-trial exec -T postgres \
  psql -U four_check -d four_check -v ON_ERROR_STOP=1 \
  -c 'SELECT version FROM schema_migrations ORDER BY version;'
```

浏览器仍访问原 `http://公网IP:8088`，强制刷新后用原账号和现用密码登录。依次核对原有基础数据、五类检查列表、原文件下载；在隔离测试数据范围验证新增录入、学时、导入/回滚和提交审核。脚本不创建测试账号、不修改用户密码、不自动给真实学生写入检查记录。

迁移 020 把尚未标记的教学周设为 1 倍，管理员应把教学礼拜周明确配置为 2 倍。历史记录缺少的处罚快照不会被猜测补齐。

## 4. 云端模板单独交付

本地已发布模板不随镜像自动复制到云端，升级脚本也不覆盖历史模板或自动执行文件清理。先核对云端五类模板下载；缺失时使用新版镜像的 `publish-attendance-template`，先默认预览，再指定实际操作管理员的 UUID 和发布原因执行 `--apply`。支持本批五类的代码为 `ATTENDANCE_RATE`、`UNATTENDED_CLASSROOM`、`MORNING_ATTENDANCE`、`LATE_RETURN`、`DORMITORY_HYGIENE`。

示例预览（只读，不发布）：

```bash
cd /opt/four-check/ip-trial
docker compose exec -T api publish-attendance-template --type-code MORNING_ATTENDANCE
```

模板版本冲突时先核对现有模板内容与使用情况，不删除旧文件、不改写已发布版本。云端模板、登录态业务和角色验收须另行记录，不能用启动健康替代。

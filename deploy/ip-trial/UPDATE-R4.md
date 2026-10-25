# r4 离线更新包操作说明

适用目标：腾讯云测试机 `/opt/four-check/ip-trial`，Compose 项目 `four-check-ip-trial`，当前 API/Web 必须都是 `ip-trial-20261025-r3`，迁移账本必须精确到 `028_hygiene_reason_dictionaries.sql`。本包只更新 API、Web 和三个后续迁移，不改 MySQL、1Panel、OpenResty、PostgreSQL/MinIO 镜像或数据卷。

## 1. 上传并校验

在 1Panel 文件管理创建 `/opt/four-check/updates/20261025-r4`，上传外层归档 `four-check-update-20261025-r4.tar` 与 `four-check-update-20261025-r4.tar.sha256`。不要上传 `.env`。在服务器执行：

```bash
cd /opt/four-check/updates/20261025-r4
sha256sum -c four-check-update-20261025-r4.tar.sha256
mkdir release
tar -xf four-check-update-20261025-r4.tar -C release
cd release
sha256sum -c SHA256SUMS
docker load -i images.tar
```

所有校验都应显示 `OK`；镜像载入应出现 API/Web 的 r4 标签。

## 2. 只读预检

```bash
cd /opt/four-check/updates/20261025-r4/release
python3 upgrade-r4.py
```

只有输出“只读预检通过”才继续。它会核对 r3 容器与 Compose 标签、服务健康、数据卷、迁移账本、r4 镜像架构和磁盘余量。预检退出时不要尝试旧 r3/r3a 脚本。

## 3. 备份并更新

```bash
python3 upgrade-r4.py --apply
```

脚本会在 `/opt/four-check/backups/before-ip-trial-20261025-r4-*` 保存 `.env`、Compose/迁移文件、PostgreSQL dump 和 MinIO 数据归档；校验备份后应用 `028_login_device_sessions.sql`、`029_simplify_permission_scope.sql`、`030_evening_self_study.sql`，再切换 API/Web 到 r4 并等待健康检查。不要重复执行旧更新包，也不要运行 `docker compose down -v`。

这三个迁移会限制账号有效设备会话数、调整已有个人授权范围规则并增加“晚自习”检查类型。当前目标是测试环境；已确认的测试数据影响按本地部署记录处理。部分用户可能需要重新登录。

## 4. 验收

浏览器访问 `http://腾讯云公网IP:8088`，确认可登录；核对“晚自习”页面可访问；再由有权限账号实际创建一条虚构晚自习检查记录并查看详情。服务器上检查四个服务 healthy、`/health/ready` 返回 200，以及迁移账本末项为 `030_evening_self_study.sql`。真实页面/角色验收仍需在目标服务器完成。

更新脚本遇到任何预检错误会停止；迁移开始后的失败会保持 API/Web 停止并保留备份目录，先把终端输出和 `migration.log` 发回排查，不要直接重启旧镜像。

### Web 容器停在 `Created` 时

`docker compose ps -q web` 只列出运行中的容器；`Created` 状态会返回空，旧版更新脚本可能因此显示不准确的“web 服务不是唯一容器”。不要重跑整包或迁移。先只读核对迁移账本、API/Web 镜像和备份目录；若迁移已到 `030_evening_self_study.sql`、Compose 指向 r4 且只有 Web 停在 `Created`，可单独启动 Web：

```bash
BASE=/opt/four-check/ip-trial
docker compose --env-file "$BASE/.env" -p four-check-ip-trial \
  -f "$BASE/compose.yaml" up -d --no-deps --pull never web
```

随后确认 Web 为 `healthy` 且 `http://127.0.0.1:8088/health/ready` 返回 200。2026-10-25 目标测试机按此恢复成功；此处记录的回执不代替晚自习页面和角色验收。

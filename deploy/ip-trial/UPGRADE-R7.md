# 腾讯云 IP 试运行：r7 更新与业务数据清理

本包用于当前已有的腾讯云服务器 `/opt/four-check/ip-trial`，Compose 项目 `four-check-ip-trial`，不是新服务器。基线按 r6a：API/Web 镜像 `four-check-api/web:ip-trial-20261102-r6`，数据库迁移 001–037。目标版本为包内 `release.json` 所列提交、镜像和迁移。

## 变更范围

- 应用迁移 038–049，更新 API/Web 离线镜像。
- 清理日常检查、导入、提交、周报版本及文件输出尝试等测试业务数据；保留账号权限、主数据、学期周次、检查类型、应交配置、审计日志和通报输出配置。
- 清理后由新 API 重新发布八类检查导入模板。

## 生成更新包

在项目 Mac 上执行：

```bash
bash deploy/ip-trial/build-update-r7.sh
```

该命令构建并打包 `linux/amd64` API/Web 镜像，但不运行应用测试。用户已确认本次测试通过。成功后产物位于 `output/releases/four-check-update-20261109-r7a.tar.gz` 及其 `.sha256` 文件；不要把 `.env` 放入包内。

## 上传、校验和加载镜像

把两个产物上传到服务器目录 `/opt/four-check/updates/20261109-r7a/`，再执行：

```bash
set -eu
cd /opt/four-check/updates/20261109-r7a
sha256sum -c four-check-update-20261109-r7a.tar.gz.sha256
test ! -e release || { echo 'release 已存在，请核对，不覆盖'; exit 1; }
mkdir release
tar -xzf four-check-update-20261109-r7a.tar.gz -C release
cd release
sha256sum -c SHA256SUMS
docker load -i images.tar.gz
```

解压后 `SHA256SUMS`、`images.tar.gz` 和 Python 脚本都应直接位于 `/opt/four-check/updates/20261109-r7a/release/`，不再有第二层 `release/` 子目录。

## 先预览，再清理

确认服务器仍是 r6a、迁移账本为 001–037，并逐项核对清理预览中的业务行数、文件对象数和对象容量：

```bash
cd /opt/four-check/updates/20261109-r7a/release
python3 clean_test_business_data.py
```

只有预览目标与预期完全一致后，才执行实际清理。脚本会先停止 API/Web，备份并校验 PostgreSQL 和 MinIO，再移除对象及对应业务行；清理范围及保留范围见同目录 `test-business-data-cleanup.md`。执行命令需要一名启用中的超级管理员 UUID：

```bash
python3 clean_test_business_data.py \
  --apply \
  --confirm '清理日常检查和通报测试数据' \
  --actor-id '<启用中的超级管理员 UUID>'
```

备份位于 `/opt/four-check/backups/before-inspection-report-cleanup-*`。命令成功后 API/Web 会恢复运行。若预览出现未说明的新表、非零待清理配置或预期外对象，停止并先维护清理清单。

## 只读预检与更新

清理完成后先执行只读预检：

```bash
python3 upgrade-r7.py --actor-id '<启用中的超级管理员 UUID>'
```

确认预检显示当前 API/Web 为 r6 镜像、迁移基线 001–037、清理计数为零、目标迁移为 038–049、磁盘空间满足备份要求；再在维护窗口执行：

```bash
python3 upgrade-r7.py --actor-id '<启用中的超级管理员 UUID>' --apply
```

更新器会再次备份数据库和 MinIO、应用迁移、切换 API/Web、等待健康检查，再通过新 API 发布八类检查模板。

## 成功核验

- API、Web、PostgreSQL、MinIO 均为 `healthy`。
- `http://127.0.0.1:8088/health/ready` 返回 HTTP 200。
- `schema_migrations` 与包内 `allMigrations` 清单完全一致，末项为 `049_report_output_week_dates.sql`。
- 八类检查模板均为 `PUBLISHED`，关联文件均为 `AVAILABLE/TEMPLATE`。
- 清理预览确认检查/导入/提交/通报业务记录为零；通报输出设置、周日期、学期通报年级范围等配置仍保留。

部署成功后按仓库固定流程更新飞书 Wiki，并向“日常管理系统 leader”群发送实际部署功能说明；只同步已部署功能和仍待验收事项。

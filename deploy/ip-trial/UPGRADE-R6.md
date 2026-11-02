# 腾讯云 IP 试运行：2026-11-02 更新与测试数据清理

本说明对应当前代码提交 `76df81f`（日常检查规则、导入范围和周次页面调整，及 2026-11-02 页面一致性/可访问性修正）。目标是本对话已有的腾讯云服务器：Ubuntu 22.04、`/opt/four-check/ip-trial`、Compose 项目 `four-check-ip-trial`、Web 端口 8088。当前服务器基线为 API/Web r5、数据库迁移 001–035；本包应用迁移 036–037。

应用测试已由用户确认通过，本流程不重跑应用测试。Dockerfile 只构建前端生产文件和 Go 二进制，不在镜像构建期间重复跑前端 lint/Vitest。

## 数据清理范围

长期维护与表范围变更规则见 [`test-business-data-cleanup.md`](test-business-data-cleanup.md)。

执行 `clean_test_business_data.py` 清理以下事务数据：日常检查记录及明细/责任关联、检查 Excel 导入批次与行、部门提交及其不可变修订、周报版本/记录快照/零回执快照/审核/更正/调整/导出文件引用、旧检查导入模板及对应 MinIO 对象。MinIO 的对象版本也一并删除。

保留账号、会话、角色和权限、院系/学生/课程/宿舍等主数据、学期/周次、检查类型、卫生情况字典、卫生区域、周报应交配置、早签周计划、主数据与校历导入批次/文件、系统设置和 `audit_logs`。清理在审计日志中新增一条清理事件；不会删除既有审计。

清理不是 `DROP DATABASE`。脚本有明确表清单和外键依赖检查；新增相关表或文件外键时会拒绝执行，维护者应先更新 `PURGE_ORDER`、文件选择逻辑和保留列表。脚本会删除旧导入模板，因此清理后须使用当前 API 重新发布八类模板；本次升级器会自动完成，日后单独运行清理脚本时需手工执行发布命令。

## 本地生成离线更新包

在 Mac 项目根目录使用已配置的 Docker Desktop：

```bash
BUILD_PROXY=http://host.docker.internal:7890 bash deploy/ip-trial/build-update-r6.sh
```

如果本机 Docker Desktop 已配置代理，可不传 `BUILD_PROXY`。脚本构建 `linux/amd64` API/Web，导出镜像，并把迁移、更新器、清理器和哈希清单装入 `output/releases/four-check-update-20261102-r6a.tar.gz`。

## 服务器操作

把外层 `.tar.gz` 和 `.sha256` 文件上传到 `/opt/four-check/updates/20261102-r6a/`。在服务器执行：

```bash
set -eu
cd /opt/four-check/updates/20261102-r6a
sha256sum -c four-check-update-20261102-r6a.tar.gz.sha256
test ! -e release || { echo 'release 已存在，请核对，不覆盖'; exit 1; }
mkdir release
tar -xzf four-check-update-20261102-r6a.tar.gz -C release
cd release
sha256sum -c SHA256SUMS
docker load -i images.tar.gz
```

先只读预览清理计数：

```bash
python3 clean_test_business_data.py
```

核对它报告的检查、导入、提交、周报和文件对象数量。用户已明确授权本次清理；实际执行还需提供启用中的超级管理员 UUID：

```bash
python3 clean_test_business_data.py \
  --apply \
  --confirm '清理日常检查和通报测试数据' \
  --actor-id '<启用中的超级管理员 UUID>'
```

脚本会停止 API/Web，生成 `pg_dump` 和完整 MinIO 卷归档，验证 PostgreSQL dump 可列出且 MinIO tar 可完整读取，再删除对象及数据库事务行，最后恢复 r5 服务。备份位于 `/opt/four-check/backups/before-inspection-report-cleanup-*`。清理日志、审计 marker 和 `SHA256SUMS` 留在备份目录。

清理成功后，r5 数据库仍处于迁移 001–035。运行只读更新预检：

```bash
python3 upgrade-r6.py --actor-id '<启用中的超级管理员 UUID>'
```

核对它报告清理计数为零、当前镜像 r5、迁移基线 001–035，再在维护窗口执行：

```bash
python3 upgrade-r6.py --actor-id '<启用中的超级管理员 UUID>' --apply
```

更新器会再备份数据库和对象存储，应用迁移 036–037，切换 API/Web 到 `ip-trial-20261102-r6`，等待四个服务 healthy 和 `http://127.0.0.1:8088/health/ready` 返回 200，然后用新版 API 重新发布八类检查模板。成功标准是迁移账本严格等于 001–037、八类模板均为 `PUBLISHED` 且文件 `AVAILABLE`。

部署后仍需通过浏览器检查主要角色的列表、手工录入、Excel 预览/确认、部门提交和通报查看。部署成功后按 `docs/deployment/post-deployment-feishu-sync.md` 更新指定 Wiki，并在“日常管理系统 leader”群发送实际部署内容；不要重跑部署来重试飞书同步。

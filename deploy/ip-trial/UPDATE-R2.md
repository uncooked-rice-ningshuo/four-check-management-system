# r2 会话续期修复更新

> **历史文件（2026-10-06）**：本文仅说明当时的 Web 会话续期补丁（应用迁移账本保持 001–021），不是 2026-10-25 r2 候选包的说明，也不是当前工作区的更新步骤。运行前请先核对仓库中的[部署记录](../../docs/deployment/ip-trial-deployment-log.md)。

r1 包之后修复了长时间停留页面、访问凭证过期后工作周读取持续失败的问题。本次只重新构建前端镜像：`four-check-web:ip-trial-20261006-r2`。后端复用已验证的 `four-check-api:ip-trial-20261006-r1`，数据库仍为 001–021，没有新增迁移。不要期待 API 标签也变成 r2。

此包包含两张应用镜像，可直接替代尚未使用的 r1 整包；也可用于已经成功升级 r1 的服务器。原 r1 包保留。

## 上传并预检

在 1Panel 文件管理创建 `/opt/four-check/updates/20261006-r2`，上传 `four-check-update-20261006-r2.tar` 和对应 `.tar.sha256`。在云端执行：

```bash
(
  set -e
  umask 077
  cd /opt/four-check/updates/20261006-r2
  sha256sum -c four-check-update-20261006-r2.tar.sha256
  mkdir release
  tar -xf four-check-update-20261006-r2.tar -C release
  cd release
  sha256sum -c SHA256SUMS
  docker load -i images.tar.gz
  python3 update.py
)
```

默认只读预检，不停止服务。目录已存在或校验失败时停止，核对上次执行情况后再继续。不要覆盖原 `/opt/four-check/ip-trial/.env`。

## 执行更新

预检通过后，在无人录入的时间执行：

```bash
python3 /opt/four-check/updates/20261006-r2/release/update.py --apply
```

脚本根据两个正在运行的应用镜像自动选择路径，随后再次检查账本与健康状态：

| 当前版本 | 执行范围 |
| --- | --- |
| API/Web 都是 20260922，账本 001–013 | 停写，备份数据库/文件/配置，应用 014–021；API 更新到 r1，Web 更新到 r2 |
| API/Web 都是 r1，账本 001–021 | 备份原 compose.yaml，仅重建 Web；不运行迁移、不重启 API/PostgreSQL/MinIO |
| 其他组合，包括上次升级未完成或已到 r2 | 停止，要求核对，不推断或重复执行 |

备份位于原目录旁的 `/opt/four-check/backups`，实际路径由脚本输出。任何错误请保留终端输出和备份，先核对再处理；不要清库、删除卷或盲目运行旧升级脚本。仅前端更新失败时，原配置与旧 Web 镜像仍保留；数据库没有被补丁修改。

## 更新后验证

```bash
cd /opt/four-check/ip-trial
docker compose -p four-check-ip-trial ps -a
curl --noproxy '*' --fail http://127.0.0.1:8088/health/ready
```

预期 API 镜像为 `ip-trial-20261006-r1`，Web 为 `ip-trial-20261006-r2`，四个容器健康。原账号、密码、IP:8088 和数据沿用。所有已打开页面都需强制刷新，以加载新前端：Mac `Command+Shift+R`，Windows `Ctrl+F5`。

登录后在校历等业务页面停留超过 15 分钟，再操作或等待工作周轮询，检查是否自动续期，且未保存输入仍保留。Cookie 真正失效时应提示重新登录；临时网络失败应允许原位重试。不要把健康检查成功当作长会话验收通过。

云端五类模板是否齐全仍按原升级说明核对，此补丁不发布或覆盖模板。正式 HTTPS、完整角色矩阵和目标长会话验收仍需单独记录。

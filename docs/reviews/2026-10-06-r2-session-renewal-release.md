# 2026-10-06 r2 会话续期修复交付

## 范围与来源

相对 `upgrade-20261006-mpzstbsa/source` 的 r1 快照，业务运行代码只变更前端 `AuthProvider.tsx`、`authApi.ts`、`httpClient.ts`，另有相应测试、浏览器用例及验证脚本。后端 Go、数据库迁移和模板没有新增变化。数据库仍为 001–021。

构建新 Web 镜像 `four-check-web:ip-trial-20261006-r2`；API 复用已经验证的 `four-check-api:ip-trial-20261006-r1`。分支 `feat-0.1-2`，未提交、推送或切换分支；包内记录工作区快照摘要，不能以 HEAD 代替源码快照。r1 镜像和更新包保留。

包：`output/releases/four-check-update-20261006-r2.tar` 及对应 `.tar.sha256`。说明：[r2 更新](../../deploy/ip-trial/UPDATE-R2.md)。阶段目录：`output/releases/webfix-20261006-tebh08t3`。

## 本轮实际验证

- 新镜像 Dockerfile 内执行原 `pnpm lint && pnpm test && pnpm build`，Node 24.12.0 / pnpm 11.7.0，71 个文件、383 项测试及生产构建通过，没有放宽断言/超时。日志 `web-build.log`。
- 新归档实际 `docker load` 后执行完整旧版升级演练，11 组检查通过，见 `upgrade-verification.json`。
- 再从旧版构造成功升级到 r1 的独立环境，通过统一 `update.py` 入口执行 Web-only 补丁，12 组检查通过，见 `web-patch-verification.json`；验证 API、PostgreSQL、MinIO 容器 ID 没有变化。
- 两条路径均验证原账号/现用密码、校历和模板字节保留，五类列表、工作台、会话刷新/未授权拒绝、模板发布、文件 CLI 预览以及数据库备份恢复。实际 HTTP 返回的生产 JS 包包含 `SESSION_CHANGED` 修复标记，排除仅构建了源码但仍运行旧前端的情况。
- 两次演练均清理一次性容器、网络和数据卷，不连接业务数据库。打包前源快照内 server/web 文件与对应工作区内容一致。
- 本轮没有重新修改认证逻辑或重新运行全量 Go 集成矩阵。真实浏览器续期行为沿用该修复已有的 11 组隔离证据 `output/playwright/306d8000e4/summary.json`，详见[修复记录](2026-10-06-session-renewal.md)；本轮新增的是容器构建、实际归档和两种部署路径验证。目标业务账号长会话及完整角色验收仍待执行。

## 更新选择与保护

`update.py` 从真实运行的 API/Web 镜像选择路径，默认只读预检：

- 两个应用均为 20260922：进入完整 `upgrade.py`，原账本须为 001–013，备份后迁移至 021，使用 API r1 / Web r2。
- 两个应用均为 r1：进入 `update-web.py`，账本须为 001–021；备份原 compose.yaml，只重建 Web，不执行数据库迁移。
- 其他组合或不健康/账本不符：停止；不推断上次操作状态，不自动重放迁移。

仅 `--apply` 执行更新；两种写入路径共用原部署目录的升级锁。镜像校验兼容平台 manifest/config 两种身份；保留 Docker 27 目标环境实际执行待验证的边界。云端本轮未操作，也没有收到此前 r1 是否已正式执行的新回执；不能沿用此前 013 输出断言云端当前版本。

下一步：用户上传 r2 两个文件，执行统一入口预检，再执行 `--apply`。更新后强制刷新所有已打开的业务页面，检查原账号及停留超过 15 分钟的续期行为；模板完整性按既有计划核对。

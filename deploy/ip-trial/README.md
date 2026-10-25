# 腾讯云 IP 试运行部署

适用：Ubuntu 22.04 x86_64、Docker 27 / Compose 2.32、直接访问 HTTP 8088。
这是虚构数据试运行配置，不是生产 HTTPS 配置；Cookie Secure 关闭，MinIO 仅在容器内部使用 HTTP。
保留已有 MySQL、PostgreSQL、OpenResty，不绑定它们的宿主机端口。

## 更新已有部署

**本目录的已有部署升级文件是历史说明，不是当前工作区的更新指引。** [r1 升级说明](UPGRADE.md)记录 2026-10-06 的迁移 014–021；[r2 更新说明](UPDATE-R2.md)记录 2026-10-06 的 Web 会话续期补丁，数据库仍为 001–021。文件名中的 r2 不等于 2026-10-25 的 r2 候选包。

当前工作区已包含周通报迁移 026 和 API/页面；本地 2026-10-25 r2 包仅含迁移 022–025，不能交付当前工作区。`build-release.sh`、`update.py` 和 `upgrade.py` 仍是更早版本的发布路径，尚未更新为可复现当前工作区的包。请先核对[部署记录](../../docs/deployment/ip-trial-deployment-log.md)，不要把本目录的历史脚本用于当前目标机更新。

## 2026-09-22 首次交付历史证据

部署配置已准备并通过 Compose 静态解析、凭据生成权限及拒绝覆盖检查。
五个 Linux amd64 镜像已构建。前端 lint、38 文件/194 项测试和 build 通过；后端 go vet 与默认测试通过（含子测试 348 PASS、17 个外部服务测试 SKIP）。
最终镜像已在本机一次性 PostgreSQL/MinIO 中验证启动、迁移、管理员改密、刷新/退出、模板真实发布与重复发布、存储暂停/恢复。
当时云服务器尚未部署；10-06 用户已提供其部署成功的运行证据。云端真实浏览器及完整业务/角色验收仍须另行记录，本段历史本地验证不能代替这些验收。
历史构建故障：加速源 EOF 已通过本次进程代理和官方源解决；验证容器缺少的仓库夹具已补齐。x86 模拟测试曾失败并中止，浮动 Node 24 的原生测试有两项超时；固定到此前验证的 Node 24.12.0 并补齐字段目录后全部通过，未改测试断言或超时。不能将环境差异归因为单一因素。
复现构建请使用下文代理示例，脚本仅在所有镜像构建成功后导出，不应在目标云服务器执行。
不要在仅有 6.9 GB 磁盘余量和 673 MiB 可用内存的目标机上构建镜像。
Swap 仅缓冲瞬时内存压力，不证明容量满足要求。

## 发布包应包含

- compose.yaml、nginx.conf、init-env.py、preflight.py、本说明。
- migrations/（全部 SQL）、migrate.sh、app-policy.json。
- 配套的 Linux amd64 镜像归档及 SHA256 校验文件，另行构建和验证。

镜像名称固定为 four-check-api:ip-trial-20260922、four-check-web:ip-trial-20260922，
以及 Compose 指定的 PostgreSQL、MinIO、mc 镜像。MinIO 和 mc 均从官方固定版本源码构建，未使用无法取得的 mc 预构建镜像。后续更新须使用新版本标签，不能覆盖此版本。
所有镜像使用本目录 Dockerfile.*，从仓库根目录构建；前端构建阶段使用构建机原生架构，运行镜像仍为 linux/amd64。
本次构建机为 Docker 29，导出脚本要求支持 `docker image save --platform`；目标机仅执行 `docker load`，无需构建工具。
本地 FlClash 构建示例（代理仅作用于构建，不写入运行环境）：

```bash
CLIENT_PROXY=http://127.0.0.1:7890 BUILD_PROXY=http://host.docker.internal:7890 bash deploy/ip-trial/build-release.sh
```

nginx.conf 在运行时只读挂载，避免改动开发配置。

## 镜像交付后的操作顺序

1. 在 1Panel 文件管理将完整发布包放到 /opt/four-check/ip-trial。不要上传本地 .env。
2. 核对磁盘：解压包、导入镜像、数据库和对象存储都需要空间。以最终包大小估算，保留运行余量；不足先扩容。
3. 执行 `python3 preflight.py`，只读检查架构、8088、磁盘及网段，确认 172.30.88.0/24 不与现有 Docker、VPC 或 VPN 路由冲突。
   冲突时同步更改 Compose 子网、web 固定地址及 API 的 TRUSTED_PROXY_CIDRS 后再启动。
4. 在发布目录执行 `python3 init-env.py`；只在服务器上生成凭据，勿粘贴至聊天。
5. 执行 `sha256sum -c SHA256SUMS`，全部 OK 后执行 `docker load -i images.tar.gz`。
6. 执行 `docker compose --env-file .env -f compose.yaml config --quiet`。
7. 在 1Panel「容器 → 编排 → 创建编排 → 路径选择」选择本目录 compose.yaml。
   确认工作目录及 .env 加载正确，缺少必需变量时停止；不要把凭据粘贴到公开模板。
8. migrate、minio-init 应以 0 退出，其余服务运行，API 与 web 应 healthy。所有服务均设置 pull_policy=never，缺少镜像时应先核对导入结果。
9. 在服务器运行 `curl --noproxy '*' -fsS http://127.0.0.1:8088/health/ready`。
10. 腾讯云安全组仅对管理员公网 IP 放行 TCP 8088，浏览器访问 `http://服务器公网IP:8088`。
11. 私下查看 .env 中的初始管理员密码，登录后改密。初始化基础数据和模板。

首次启动之外的后续升级必须单独执行并核对迁移/初始化任务；不要假定已退出容器自动重跑。
常驻服务 restart=unless-stopped；不自动删除卷，不执行 down -v。

## 模板发布

先预览（不发布）：

```bash
docker compose run --rm --no-deps --entrypoint publish-attendance-template api
```

核对目标数据库、Bucket 和启用中的超级管理员 UUID 后显式发布：

```bash
docker compose run --rm --no-deps --entrypoint publish-attendance-template api \
  --apply --actor-id '<超级管理员UUID>' --reason '初始化到课率导入模板 v1'
```

发布后用虚构业务对象验收到课率下载→上传→预览→确认入库。ready=200 不代表业务初始化完成。

## 运行观察

观察 `docker stats --no-stream`、`free -h`、`df -h /` 和容器 OOM 状态。
本配置限制 API Go 堆目标及数据库连接数，但未证明 2 GB 主机可稳定承载现有服务与完整项目。
若持续换页、OOM、数据库或文件操作超时，停止试运行并扩容，不删除现有业务数据腾空间。
正式上线前还需 HTTPS、内部存储 TLS、联合备份恢复和完整角色验收。

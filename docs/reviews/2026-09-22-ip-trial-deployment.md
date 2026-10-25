# 2026-09-22 腾讯云 IP 试运行部署包验证

## 范围与结论

在用户指定的 feat-0.1-2 现有工作区准备离线 Docker 部署包；未创建分支、提交、推送或连接云服务器。镜像包含当前工作区实现，不能用远程分支 HEAD 代替本次构建输入。仅新增 deploy/ip-trial 下的部署文件及文档，不改开发 Compose 和业务运行时代码。

目标机信息来自用户终端输出：Ubuntu 22.04 x86_64、2 核/1.9 GiB、1Panel 1.10.32-lts、Docker 27.5.1、Compose 2.32.4。Halo 已停止并保留数据，MySQL 保留；可用内存 673 MiB、Swap 1 GiB、磁盘余量 6.9 GB。这些是用户提供的证据，非本轮远程探测。

五个 linux/amd64 镜像已经生成：API（含模板 CLI）、web、PostgreSQL 16、固定版本 MinIO、同版本官方源码构建的 mc。离线配置只发布 8088，数据库和对象存储独立持久化，镜像 pull_policy=never，带日志轮转、常驻服务重启策略及前后端就绪检查。初始凭据仅在目标服务器生成；发布包无 .env。

## 实际验证

- `docker compose ... config --quiet` 通过；仅 web 映射端口，部署编排无 build 项。
- `init-env.py` 在临时目录生成权限 0600 的配置，二次执行拒绝覆盖，未输出凭据。
- 前端 Docker 构建阶段使用原生架构 Node 24.12.0、pnpm 11.7.0、原锁文件，执行 `pnpm lint && pnpm test && pnpm build`，38 文件/194 项测试通过，构建成功。jsdom CSS/伪元素及 Ant Design 警告、大包警告保留。最终 Nginx 镜像仍为 amd64。
- API 构建镜像内执行 `go vet ./... && go test -v ./...`，348 个 PASS 条目（含子测试）、17 个外部服务测试 SKIP、0 FAIL。仓库根 Compose 作为只读夹具挂载；未提供业务数据库或业务存储凭据。
- 最终镜像在独立随机 Compose 项目、新卷、一次性 PostgreSQL/MinIO、随机本机回环端口上通过 12 项部署检查，结束后只删除该次临时容器、网络和卷。没有写入现有业务环境。
- 12 项检查：全新启动与 ready=200、前端 /login HTML、匿名访问拒绝及稳定错误码、初始管理员登录与 HTTP Cookie 属性、改密后工作台 API、Cookie 刷新、全部 13 个迁移、模板预览/真实发布/重复发布、暂停 MinIO 时 ready=503、恢复后 ready=200、退出使刷新会话失效、web 容器健康检查。
- 验证镜像归档 manifest 的 5 个条目均为 linux/amd64，运行配置没有构建代理或部署凭据变量；SHA256 清单核对通过。
- `bash -n deploy/ip-trial/build-release.sh`、Python 脚本语法检查、`git diff --check` 通过。

验证日志位于忽略提交的 output/releases/：frontend-build.log、frontend-native-first.log、backend-check.log、deployment-smoke.json。部署包内附结构化隔离检查报告及本文副本，未包含测试环境凭据。

## 已处理的构建/验证问题

1. 本机镜像加速源 EOF，官方认证端点直连超时。使用用户给出的 FlClash 7890 作为本次客户端/构建进程代理，并显式访问官方 registry 地址；未改 Docker 全局配置。
2. mc 固定版本预构建镜像获取失败。使用相同官方发布标签 RELEASE.2025-08-13T08-35-41Z 源码构建独立镜像，真实初始化和模板发布已通过。
3. 后端验证容器没有仓库根 compose.yaml，导致夹具读取失败；补只读挂载后完整检查通过。
4. 前端 x86 模拟测试出现失败后中止；浮动 Node 24 原生检查出现两项超时及缺少跨端 catalog.json。补齐目录夹具、固定此前验证的 Node 24.12.0 后完整套件通过。两项用例在本机独立对照也通过；未修改断言或超时。环境变量和负载并非全部独立控制，不能认定唯一原因是 Node 版本。
5. Compose Started 不代表 Nginx 已监听；验证脚本等待实际 HTTP 就绪。故障恢复后 Docker 周期健康状态也可能滞后，验证等待下一次检查并确认 healthy；没有把中间未就绪状态写成通过。

## 仍未验证与下一步

本轮不是云服务器部署完成证明，也不是浏览器/跨角色/并发/全业务验收。Go 中 17 项外部集成 SKIP 没有被这 12 项部署检查全部覆盖。

一次空闲采样中四个常驻容器合计约 324 MiB，但宿主 Docker 有 7.65 GiB 内存；这不是 2 GiB 云服务器的负载容量结论。目标机仍须预检网段、端口、磁盘，再导入启动并观察内存、Swap、磁盘与 OOM。不得清理现有 MySQL 或其他业务数据来绕过容量问题。

IP HTTP 仅用于虚构数据试运行，Cookie Secure=false，内部 MinIO 为 HTTP；正式上线需按既有约束补齐 TLS、联合备份恢复及完整角色验收。云端管理员改密、基础数据配置、模板发布和到课率真实浏览器导入仍待现场执行。

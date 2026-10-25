# 腾讯云 IP 试运行部署与迁移记录

本文件记录腾讯云 IP 试运行的部署版本、迁移账本、操作结果和待解决问题。目标环境的输出由用户从服务器提供；没有服务器终端证据的状态明确标为未核验。不要在这里保存 .env、密码、Token 或个人数据。

服务器部署通过技术核验后，固定执行[部署成功后的飞书同步流程](post-deployment-feishu-sync.md)：追加指定 Wiki 记录，向“日常管理系统 leader”群发送《功能更新说明》，并在本日志留下同步回执。尚未完成的业务/角色验收必须如实注明。

## 当前状态

更新时间：2026-10-25。腾讯云事实仅依据用户粘贴的终端输出；本轮未连接服务器。下文“当前状态”覆盖相互冲突的早期结论；后面的按日操作记录保留为历史证据。

已建立每周跟进，检查关联任务中的新迁移证据并同步本日志与项目状态；只做记录和状态跟进，不会自动连接或修改生产环境。状态无变化时不通知。

- **服务入口**：公网 IP 的 TCP 8088，Compose 项目 `four-check-ip-trial`，目录 `/opt/four-check/ip-trial`。该更新只涉及此项目的 API/Web 与迁移；不操作 1Panel MySQL、独立 PostgreSQL、其他容器或卷。
- **云端当前状态**：用户 2026-10-25 最新回执确认 API/Web 已切至 `four-check-api/web:ip-trial-20261025-r4`；API、Web、Postgres、MinIO 均报告 `running/healthy`，`http://127.0.0.1:8088/health/ready` 返回 HTTP 200。迁移账本包含 `028_login_device_sessions.sql`、`029_simplify_permission_scope.sql`、`030_evening_self_study.sql`，末项为 030。
- **模板问题**：到课率、无机课堂、早锻炼/早签、晚归、宿舍卫生 v1 已发布，文件元数据均为 `AVAILABLE/TEMPLATE`；用户确认页面使用已无问题。教室卫生和公共宿舍卫生模板本次暂不补。
- **已应用更新包**：[r4 离线更新包](../../output/releases/upgrade-20261025-r4/four-check-update-20261025-r4.tar)，校验文件为同目录 `.tar.sha256`。用户在服务器验证外层及包内 SHA256、导入 API/Web `ip-trial-20261025-r4` Linux amd64 镜像，并应用迁移 `028_login_device_sessions.sql`/029/030。该归档是已部署的历史构件；本地后续诊断修正不会改写该归档或其校验值。
- **飞书同步回执（r4）**：2026-10-25 已在[发布更新记录 Wiki](https://acna9bso0eml.feishu.cn/docx/PlUAdsBiLobNpaxDyR8cDK1Wn0b)追加 r4 部署与功能说明，并由宁硕的飞书 CLI 应用机器人向“日常管理系统 leader”群发送《功能更新说明》（消息 ID `om_x100b62d5e0a544a0ceaca1822bcca28`）。内容明确记录服务/迁移核验结果，以及晚自习实际记录和角色权限仍待验收。
- **当前源码迁移影响**：026 将教室卫生、公共宿舍卫生排除出通报范围；027 增加逐周应交与零记录回执；028 卫生原因迁移预置字典并要求新写入/修改的 `OTHER` 原因带说明；另一个同编号 028 建立登录设备会话；029 简化权限范围，可能转换已有 ALLOW、停用带范围 DENY，并撤销相关账号的刷新会话；030 增加并启用晚自习检查类型。云端只读统计显示：028 预计淘汰一个账号的 3 个超额有效会话；029 涉及 1 条启用的有范围 ALLOW、0 条 DENY、1 个账号和 2 个有效刷新会话。用户已确认目标为测试环境并接受必要时清理测试数据，因此这些影响作为测试部署预期行为记录，部署后验证即可。两个 `028_*.sql` 是不同文件，必须都纳入迁移账本核对。
- **迁移器静态核对**：当前仓库的 `server/scripts/migrate.sh` 按 `migrations/*.sql` 文件名排序，且 `schema_migrations.version` 记录完整文件名；每个 SQL 文件单独包在事务中。因此 `028_hygiene_reason_dictionaries.sql` 与 `028_login_device_sessions.sql` 会分别执行和登记，不会因数字前缀相同而互相覆盖。后续更新包必须同时携带这两个文件，并按完整文件名核对账本。
- **资源观察**：最新服务器回执根分区可用 4.4 GiB（91% 已用）、RAM 可用约 1.1 GiB、swap 已用约 707 MiB；这是当时观测值，执行升级前仍须重查。r3a 预检门槛曾通过，但不代表当前资源与新包的备份空间仍满足要求；不能在云服务器构建镜像。
- **本地发布验证状态**：本轮工作区在 `feat-0.1-3`，以未提交工作区源码构建。`go test ./... -count=1`、`go vet ./...`、`go build ./...` 通过；API r4 镜像构建通过。Web lint、76 个 Vitest 文件/414 项及 Vite 生产构建通过；前端 Docker 构建的全量测试耗时约 447 秒。Docker Hub BuildKit 元数据请求超时，因此 Web dist 在固定 Node 24.12.0 容器中完成同样的 lint/test/build，再装入 Nginx 1.27 amd64 镜像；Nginx 配置检查与首页 HTTP 冒烟（200）通过。两个目标镜像架构均核对为 `linux/amd64`；外层包和包内 SHA256 均已复核。Vite 仍提示主 JS chunk 超 500 kB，测试输出含 CSS/Ant Design 既有警告。
- **部署恢复记录**：升级后 Web 一度为 `Created`，`docker compose ps -q web` 因只查询运行容器返回空，旧脚本报出误导性的“web 服务不是唯一容器”。用户确认迁移账本已到 030、API 已是健康 r4，并在唯一 Web 容器上执行 `docker compose ... up -d --no-deps --pull never web`；Web 随后 `running/healthy`，ready 返回 200。r4 备份目录 `/opt/four-check/backups/before-ip-trial-20261025-r4-20261025-224857` 存在，已看到 `original-images.json` 与 `migration.log`；用户尚未提供 dump/MinIO 归档的文件清单及备份校验结果，因此备份完整性不记为已核验。晚自习真实页面/角色验收仍待完成；原 records 500 的用户登录态重放仍未完成。

## 操作记录

### 2026-10-25：20261025-r1 更新包与预检

- 从本地分支 feat-0.1-2 的当前工作区构建 API/Web 的 linux/amd64 镜像；Web lint、TypeScript 检查和生产构建通过。该次打包没有重新运行完整测试套件。
- 更新包包含迁移 022、023，不包含后来新增的迁移 024。用户已将 images.tar.gz 导入服务器；Docker 回显两张镜像均为 linux/amd64。
- 原 upgrade.sh 的前置条件仍要求 API/Web 为 20261006 标签、迁移账本精确到 021。用户实际环境已是 20261024-r1、账本 022，因此脚本在镜像标签预检处退出。
- 用户随后核对到容器仍健康运行在 20261024-r1、账本仍为 022、8088 健康检查 HTTP 200，且没有生成 before-20261025-r1 备份目录。这证明该次脚本没有停止服务或执行迁移。
- 已在本地准备针对 20261024-r1 / 022 的修正版脚本，但没有用户回执证明该脚本在云端运行。之后关联任务的数据库查询显示 023 已登记；其执行来源和当时应用镜像标签尚未核实。

### 2026-10-25：学生导入 500 与迁移 024

- 关联任务中，用户查询到数据库已有 022、023，但没有 024；检查约束仍是 master_data_import_batches_template_version_check，定义为 template_version = 1。该输出之后未同步核对运行镜像和服务端错误日志。
- 本地新增 server/migrations/024_master_data_import_template_versions.sql，以追加迁移方式删除固定为 1 的约束并建立大于 0 的约束；不会改写已应用的 022。
- 迁移 024 在一次性 PostgreSQL 中完成验证。该结果只证明隔离环境，不代表云端已经应用。
- 关联任务曾单独讨论只应用 024；现在 r2 候选包将 API 更新与 024、025 一并预检/备份/迁移。若服务器仍运行旧代码，单独应用迁移并不能加载新功能；若 r2 之后仍报 500，应核对新 API 标签和服务端日志。

### 2026-10-25：r2 候选更新包

- 当前分支 feat-0.1-2、HEAD 474943472e24545695546ae8039ea49455bca35f；包镜像从该 HEAD 和未提交工作区构建，工作区清单随包保存。
- 后端 go vet ./... 与 go test ./... -count=1 通过；前端 pnpm lint、pnpm test（74 文件/396 项）、pnpm build 通过。构建有 Vite 主 JS chunk 超过 500 kB 提示。
- API/Web 镜像均为 linux/amd64。Web 静态文件经过本地隔离 Nginx 容器冒烟检查；云端应用、迁移和真实业务流程尚未验证。
- 包含迁移 022–025，迁移执行器会跳过已登记文件并按序应用缺失迁移。更新脚本先只读核对基线；--apply 时创建数据库、MinIO 和配置备份，再更新 API/Web。包文件 SHA256 校验通过；没有包含 .env。

### 2026-10-25：r3 离线更新包构建与复核

- **源码与镜像**：基于分支 `feat-0.1-2`、提交 `b78a5df` 构建 API/Web 镜像，标签为 `ip-trial-20261025-r3`，均为 `linux/amd64`。旧 r2 包缺少迁移 026–028，已标为不可用于本轮。
- **验证**：Go `go vet ./...`、`go test ./... -count=1` 通过；前端 lint、76 文件/415 项 Vitest 与 build 通过。Docker Web 镜像内部重复执行 lint/test/build 通过；Nginx 配置检查通过。Vite 有主 chunk 体积提示。外层包使用 USTAR 并含独立校验清单。
- **脚本安全检查**：脚本只接受已知同标签 API/Web 基线，检查 PostgreSQL/MinIO 卷与连续迁移前缀；默认运行只读预检。执行模式先备份配置、pg_dump 和 MinIO 卷，再应用缺失迁移与切换 API/Web。修正了备份目录标签，确保记录为 r3。
- **迁移边界**：预检读取三类卫生检查中 `OTHER` 空说明的旧数据数量。数量非零时须先评估，避免业务人员编辑旧记录时被新约束拒绝；脚本本身不会自动替业务方作此决定。
- **目标环境**：没有连接云服务器、上传包、执行生产迁移、停止服务或改镜像。本机业务数据库未运行这组迁移。需要用户上传后提供新一轮只读预检输出，再确认空间和数据门槛。

### 2026-10-25：r3 首版预检安全退出与 r3a 基线修订

- **用户提供的运行结果**：外层与包内 SHA256 均通过，r3 API/Web 镜像成功载入；服务器根分区当时可用 4.6 GiB，RAM 可用约 1.0 GiB、swap 已用约 720 MiB。
- **预检结果**：容器列表显示 API 与 Web 均运行 `20261025-r2`；脚本前面的容器状态和健康检查通过，但首版允许的基线只有 20261024-r1/20261025-r1，故在镜像组合检查处安全停止。用户没有执行 `--apply`。按脚本顺序，停止发生在创建备份、停 API/Web、停 MinIO、应用迁移和更新 Compose 镜像之前；本次没有已知生产副作用。
- **修订包**：本地制作 r3a，镜像层与迁移 022–028 保持不变，仅增加已观测的 `20261025-r2` API/Web 同标签组合为允许起始基线。包上传到独立目录 `/opt/four-check/updates/20261025-r3a` 后，重新做只读状态/迁移/空间/历史数据预检；当前仍未获准或执行 `--apply`。

### 2026-10-25：r3a 只读预检通过

- **包与镜像**：用户上传的 r3a 外层归档和包内文件校验均通过，API/Web r3 镜像成功导入。
- **运行基线**：API/Web 当前为匹配的 `20261025-r2`；升级脚本检查的容器状态、健康和 Compose 项目身份均通过。迁移账本是 001–025 连续前缀，末项为 `025_academic_week_import_template_versions.sql`。
- **数据与容量门槛**：宿舍、教室、公共宿舍三类 `OTHER` 空说明记录计数均为 0；脚本磁盘门槛通过。观测到根分区可用 4.5 GiB、RAM 可用约 1.0 GiB、swap 已用约 725 MiB，维护窗口开始前仍须复查资源。
- **当时结论**：只读预检输出“只读预检通过”，没有停止容器、创建备份或写数据库。该记录之后没有收到执行 `--apply` 的结果，因此不能据此判断当前状态。

### 2026-10-25：r3 镜像与迁移账本复核

- **用户提供的最新回执**：API/Web 镜像均为 `four-check-api:ip-trial-20261025-r3` / `four-check-web:ip-trial-20261025-r3`，状态 `running/healthy`；Postgres 与 MinIO 也为 `running/healthy`。
- **迁移状态**：账本从 001 连续到 `028_hygiene_reason_dictionaries.sql`。因此线上已包含 026 周报基线、027 应交/零回执和卫生原因 028；没有 `028_login_device_sessions.sql`、029 或 030。
- **资源快照**：根分区 50 GiB、已用 43 GiB、可用 4.4 GiB；内存可用约 1.1 GiB；1 GiB swap 已用约 707 MiB。
- **边界**：这组输出确认目标服务与 schema 当前版本，但没有给出升级备份目录、迁移日志或新功能验收结果。尚不能据此宣称完整部署验收通过。

### 2026-10-25：待应用迁移影响只读统计

- **用户提供的查询回执**：`028_login_device_sessions.sql` 将有 1 个账号超过三台设备限制，预计淘汰 3 个有效会话；该迁移尚未执行。
- **迁移 029 影响**：有范围权限规则 1 条，为启用的 ALLOW；没有 DENY 规则。将有 1 个账号的 `session_version` 递增，并撤销该账号 2 个未撤销刷新会话。迁移会移除此 ALLOW 的部门/班级/年级范围，使授权范围扩大。用户已确认这是可清理数据的测试环境，该规则变化可随测试版本一起应用并在部署后验收，不作为生产审批门槛。
- **账号关系推断**：028 超额会话账号与 029 受影响账号不是同一账号：前者至少有 4 个未撤销有效会话，后者只有 2 个未撤销会话。这里只记录聚合推断，不读取或保存账号 ID。
- **副作用边界**：以上 SQL 为只读。应用 028/029 会使相关用户的登录会话失效；029 还会扩大一条具体授权规则。

### 2026-10-25：目标环境性质确认

- 用户明确说明当前腾讯云部署仅作测试，尚非正式线上；如测试出问题可以清理测试数据。后续按测试环境更新推进，不再以生产数据保护审批作为迁移 029 的阻塞项。
- 此确认不代表已经执行登录会话 028、029 或 030。r4 包现已在本地生成；目标服务器仍待用户上传、只读预检并执行更新。

### 2026-10-25：r4 Docker 离线更新包已生成

- **下载文件**：[four-check-update-20261025-r4.tar](../../output/releases/upgrade-20261025-r4/four-check-update-20261025-r4.tar)（约 67 MiB）与 [SHA256 校验文件](../../output/releases/upgrade-20261025-r4/four-check-update-20261025-r4.tar.sha256)。包内有 API/Web `four-check-*:ip-trial-20261025-r4` 的 `linux/amd64` 镜像、迁移 `028_login_device_sessions.sql`、`029_simplify_permission_scope.sql`、`030_evening_self_study.sql`、r3 完整迁移账本基线、操作说明与 `upgrade-r4.py`；不含 `.env`。
- **源码状态**：本地 `feat-0.1-3` 工作区，HEAD `d9c5eb5516e5dcdd21566a039cbde4dc3509720a`，镜像包含未提交的当前工作区源码。包内 `source-worktree-status.txt` 留有打包时的工作区清单。
- **验证结果**：Go `go test ./... -count=1`、`go vet ./...`、`go build ./...` 通过；Web `pnpm lint`、Vitest 76/76 文件、414/414 项、`pnpm build` 通过。API/Web 均为 amd64；Nginx 配置测试与直接请求首页 HTTP 200 通过；包内及外层 SHA256 均通过。生产构建仍有 Vite 主 JS chunk 超 500 kB 提示；测试有 CSS 解析和 Ant Design 弃用提示。
- **构建方式说明**：BuildKit 请求 Docker Hub 基础镜像元数据时遇到超时，固定的 Node 24.12.0 镜像通过 Docker 拉取后，在一次性 Node 容器中跑完前端 lint/test/build；静态产物装入 Nginx 1.27 amd64 基础镜像并做网络别名条件下的配置/HTTP 冒烟。API 通过当前 Dockerfile 构建。`Dockerfile.web` 测试命令固定为 `pnpm test --maxWorkers=2`，这次全量测试耗时约 447 秒。
- **更新脚本行为**：默认只读检查 API/Web 容器和 Compose r3 标签、四服务健康、Postgres/MinIO 卷身份、迁移账本精确基线、目标镜像 ID/amd64 架构及磁盘余量。`--apply` 才会停止 API/Web，备份 `.env`/配置/全部既有迁移、PostgreSQL dump 与 MinIO 数据，再添加三条迁移并更新 API/Web 镜像；迁移开始后的失败保持 API/Web 停止并保留备份。
- **云端边界**：没有上传包、连接服务器、停止云端服务、执行云端迁移或改容器。本包应先上传至 `/opt/four-check/updates/20261025-r4/`，核验归档 SHA256、导入镜像、运行 `python3 upgrade-r4.py` 只读预检，通过后再运行 `python3 upgrade-r4.py --apply`。服务器完成后的健康状态与晚自习角色验收仍待用户回执。

### 2026-10-25：r4 已应用与 Web 容器恢复

- **用户上传/校验回执**：外层 r4 归档、包内 `SHA256SUMS` 均通过；API/Web r4 镜像载入成功。`mkdir release` 提示目录已存在，但随后解包与文件校验成功。
- **应用结果证据**：用户后续查询显示 Compose 已指向 API/Web r4；迁移账本已包含 `028_login_device_sessions.sql`、029、030。API 为 `running/healthy`；Web 容器曾停在 `Created`，其日志为空；Postgres、MinIO 当时均为健康状态。r4 备份目录已创建，用户查询到了 `original-images.json` 与 `migration.log`。
- **原因与恢复**：`docker compose ps -q web` 不包含 `Created` 容器，旧脚本将 0 个运行容器误报为“服务不是唯一容器”。用户执行 `docker compose --env-file ... up -d --no-deps --pull never web` 后容器成功启动，状态 `running/healthy`，`/health/ready` 返回 HTTP 200。没有重跑迁移或 `--apply`。
- **备份边界**：已确认备份目录及部分文件存在；尚未确认 `database.dump`、`minio-data.tar.gz` 的大小和校验清单，因此不宣称备份已完整校验。
- **剩余验收**：用户尚未回报浏览器登录、晚自习页面/虚构记录、三设备登录限制及权限范围专项验收。更新脚本源码已改为查询所有状态的容器并显示具体状态；已部署归档保持原始哈希，不重写历史构件。

## 待办与完成条件

1. **云端 r4 已部署**：不要重跑 r4 或旧版本更新脚本。API/Web 均为 r4，迁移账本到 030；用户已报告四服务健康和 ready=200。
2. **剩余验收**：浏览器确认登录、晚自习页面，并由有权限账号完成一条虚构记录的创建/查看；验证登录设备上限和权限范围变化。部署健康不自动代表这些业务验收已完成。
3. **备份完整性**：r4 备份目录存在，但 `database.dump`、`minio-data.tar.gz` 的存在/大小及 `SHA256SUMS` 尚未由用户回执确认；核对前不记录备份已完整校验。
4. 原 records 500 的用户登录态重放仍是独立未解决问题。

**完成标准**：r4 已在测试服务器应用，迁移和 API/Web 健康已确认；浏览器功能/角色验收及备份归档完整性仍待分别确认。

### 当前云端状态只读核验命令（已执行，留作复核）

下面只读取容器配置/健康状态、磁盘内存和迁移账本；不重启、不迁移、不修改数据。整段从任意目录粘贴执行：

```bash
set -eu
cd /opt/four-check/ip-trial
docker inspect --format '{{.Name}} image={{.Config.Image}} status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' four-check-ip-trial-api-1 four-check-ip-trial-web-1 four-check-ip-trial-postgres-1 four-check-ip-trial-minio-1
docker compose --env-file /opt/four-check/ip-trial/.env -p four-check-ip-trial -f /opt/four-check/ip-trial/compose.yaml exec -T postgres psql -U four_check -d four_check -Atqc 'SELECT version FROM schema_migrations ORDER BY version;'
df -h /
free -h
```

该命令此前得到 r3 基线回执（见“r3 镜像与迁移账本复核”）；后续用户又单独提供了 r4 容器、迁移、Web ready 状态。部署后复核可重用此只读命令；不要附 `.env` 内容、密码或 Token。

### 待应用迁移影响只读盘点（已执行；仅作测试变更记录）

下面只统计 `028_login_device_sessions.sql` 与 `029_simplify_permission_scope.sql` 按当前数据将影响的记录数，不会修改数据。两个类别的刷新会话数可能重叠，因为 028 先淘汰超出三设备限制的会话，029 再撤销受影响账号剩余的有效刷新会话：

```bash
cd /opt/four-check/ip-trial
docker compose --env-file /opt/four-check/ip-trial/.env -p four-check-ip-trial -f /opt/four-check/ip-trial/compose.yaml exec -T postgres psql -U four_check -d four_check -P pager=off -c "
WITH scoped_rules AS (
  SELECT id, account_id, effect, enabled
  FROM user_permission_rules
  WHERE department_id IS NOT NULL OR class_id IS NOT NULL OR grade IS NOT NULL
), active_sessions AS (
  SELECT id, account_id,
         row_number() OVER (PARTITION BY account_id ORDER BY created_at DESC, id DESC) AS session_rank
  FROM refresh_sessions
  WHERE revoked_at IS NULL AND expires_at > now()
), metrics AS (
  SELECT '029 有范围权限规则' AS item, count(*)::bigint AS affected FROM scoped_rules
  UNION ALL SELECT '029 有范围 ALLOW 规则总数', count(*) FROM scoped_rules WHERE effect='ALLOW'
  UNION ALL SELECT '029 将放宽的启用 ALLOW 规则', count(*) FROM scoped_rules WHERE effect='ALLOW' AND enabled
  UNION ALL SELECT '029 有范围 DENY 规则总数', count(*) FROM scoped_rules WHERE effect='DENY'
  UNION ALL SELECT '029 将停用的启用 DENY 规则', count(*) FROM scoped_rules WHERE effect='DENY' AND enabled
  UNION ALL SELECT '029 session_version 将递增的账号', count(DISTINCT account_id) FROM scoped_rules
  UNION ALL SELECT '029 将撤销的有效刷新会话', count(*) FROM refresh_sessions r
    WHERE r.revoked_at IS NULL AND r.account_id IN (SELECT account_id FROM scoped_rules)
  UNION ALL SELECT '028 超过三台设备限制将淘汰的有效会话', count(*) FROM active_sessions WHERE session_rank > 3
  UNION ALL SELECT '028 存在超额有效会话的账号', count(*) FROM (
    SELECT account_id FROM active_sessions GROUP BY account_id HAVING count(*) > 3
  ) AS over_limit_accounts
)
SELECT item, affected FROM metrics ORDER BY item;
"
```

用户随后确认目标为测试环境并接受必要时清理数据，因此不再安排额外的权限影响只读查询；本统计留作应用前后对照。

### 2026-10-25：导入模板下载接口 404 诊断

- 用户截图中的请求是 `GET /api/v1/imports/attendance/template`，状态 404，响应头为 `Content-Type: application/json`、`Content-Length: 91`；响应正文没有展开。远端地址显示本机 FlClash `127.0.0.1:7890`，因此它是浏览器代理地址，不是云服务器地址；`Server: nginx/1.27.5` 也不能单独证明 404 由 Nginx 生成。
- 已核对已部署基线包记录的源码提交 `12a2794`：该 API 已注册 `/api/v1/imports/attendance/template` 路由，并将 `ErrTemplateNotFound` 映射为 `{"code":"IMPORT_TEMPLATE_NOT_FOUND","message":"当前检查类型尚未发布导入模板"}`。按当前序列化方式，这个错误体正好 91 字节，与截图长度吻合。随后用户的只读查询确认截图所用 ID 在 `inspection_types` 中不存在；因此当前要先追查页面为何送出云端不存在的 ID，而不能仅当作缺少模板发布记录处理。
- 用户随后对截图里的 `inspectionTypeId=30000000-0000-0000-0000-000000000014` 执行查询，关联结果为 0 行；筛选接口返回 `ATTENDANCE_RATE` 的真实 ID `30000000-0000-0000-0000-000000000001`。按业务代码再次查模板，结果为 `NO_TEMPLATE`。前端到课率页的 ID 来自筛选接口响应中的 `inspectionType.id`（见 `web/src/pages/inspection/AttendancePage.tsx`），并非硬编码；因此 `...0014` 是旧页面/旧查询状态或不同目标数据留下的请求值。对这个 404 的直接修复是使用已部署 API 镜像内的发布工具，为 `ATTENDANCE_RATE` 生成并发布模板；发布前先预览，发布需要启用中的超级管理员 UUID 和审计原因。
- 只读核对结果：截图 ID 在 `inspection_types` 中无记录；按业务代码查询得到 `ATTENDANCE_RATE` 的云端 ID 为 `30000000-0000-0000-0000-000000000001`，模板状态为 `NO_TEMPLATE`。因此模板发布是下载成功的必要条件。使用部署 API 容器内的 CLI 做只读预览：

  ```bash
  cd /opt/four-check/ip-trial
  docker compose --env-file .env -p four-check-ip-trial -f compose.yaml exec -T api \
    publish-attendance-template --type-code ATTENDANCE_RATE
  ```

  用户已在目标服务器实际执行上述预览命令，输出 `{"code":"ATTENDANCE_IMPORT","mode":"preview","sha256":"4acf90bcc8862189ac22e3827cead869e5597255b763400fbb82bb44f8a72386","sizeBytes":26886,"version":1}`。这证明当前 API 容器可生成并校验模板；`mode=preview` 表示没有写数据库或 MinIO。

  预览确认后，把下方命令中的占位值替换为实际启用中的超级管理员 UUID，再执行发布；该操作会写入模板文件和数据库发布记录，并留下审计原因：

  ```bash
  docker compose --env-file .env -p four-check-ip-trial -f compose.yaml exec -T api \
    publish-attendance-template --type-code ATTENDANCE_RATE --apply \
    --actor-id '替换为启用中的超级管理员UUID' \
    --reason '云端初始化到课率导入模板'
  ```

  之后按 `ATTENDANCE_RATE` 再查应显示 `PUBLISHED`，强制刷新浏览器后模板请求应使用 `...0001`。如果预览或发布报告版本冲突，停止并核对，不要手工改数据库。若强制刷新后的筛选接口仍返回不同 ID，则先检查公网 API 与本 Compose 数据库是否一致。
- `context-options` 路由也存在于该部署基线，但截图没有展示这些请求各自的状态和响应体；分别打开 Response 后再诊断。本任务环境没有直接连接或修改云服务器；以下云端变更与核查均来自用户提供的服务器回执。

### 2026-10-25：检查类型模板全量核查范围

- 这里的“模板”按本次 404 对应的 Excel 下载模板理解；服务端 API/代码称为“导入模板”，路径为 `/api/v1/imports/*/template`。它与通报 Word/Excel 导出文件不是同一功能。
- 部署升级说明列出的五类是 `ATTENDANCE_RATE`、`UNATTENDED_CLASSROOM`、`MORNING_ATTENDANCE`、`LATE_RETURN`、`DORMITORY_HYGIENE`；但服务器当前 20261024-r1 镜像的源码提交 `12a2794` 已有教室卫生批量导入按钮及相应 API/模板实现。故模板链路应覆盖这六类，部署升级说明中的五类清单不完整。
- `PUBLIC_DORMITORY_HYGIENE` 当前有独立手工录入页面，但页面没有批量导入按钮；模板发布定义与 `NewInspectionImportService` 也未接入该类型。因此当前没有已实现的导入模板链路，不能手工写 `import_templates` 或发布一个无法配套解析的模板。
- 云端只读盘点命令如下，返回全部检查类型及各版本模板、文件元数据；`AVAILABLE` 只表示数据库元数据状态，仍需下载读取来证明 MinIO 对象实际存在且内容可校验：

  ```bash
  cd /opt/four-check/ip-trial
  docker compose --env-file .env -p four-check-ip-trial -f compose.yaml exec -T postgres \
    psql -U four_check -d four_check -P pager=off -c "
  SELECT i.id, i.code, i.enabled, i.participates_in_report,
         t.id AS template_id, t.code AS template_code, t.version_no, t.status AS template_status,
         t.published_at, f.status AS file_status, f.retention_type,
         f.original_name, f.size_bytes
  FROM inspection_types AS i
  LEFT JOIN import_templates AS t ON t.inspection_type_id = i.id
  LEFT JOIN files AS f ON f.id = t.file_id
  ORDER BY i.sort_order, t.version_no;
  "
  ```

- 之后对所有目标 UI 实际提供导入入口的类型逐一从已登录浏览器下载：确认筛选/上下文返回的 ID 与同一数据库 `inspection_types` ID 一致、响应为 HTTP 200、响应含模板 ID/版本头且文件可作为 XLSX 打开。只有缺失发布记录且预览成功的类型才逐类用 CLI 发布；发现 `FAILED`/`MISSING` 文件状态、对象读取错误、重复/异常版本、ID 不匹配或模板契约不匹配时先停止发布并记录证据。
- **用户 2026-10-25 云端盘点回执**：目标库共列出 7 类，全部 `enabled=true`、`participates_in_report=true`。`ATTENDANCE_RATE` 已有 v1 `PUBLISHED`，关联文件 `AVAILABLE/TEMPLATE`、26,886 字节；其余六类没有任何模板记录。到课率 `published_at=2026-10-25 05:13:57+00`，说明用户已在预览后完成发布。
- **同次生成预览回执**：目标 API 容器对 `UNATTENDED_CLASSROOM`、`MORNING_ATTENDANCE`、`LATE_RETURN`、`DORMITORY_HYGIENE`、`CLASSROOM_HYGIENE` 均返回 `mode=preview`、版本 1；大小分别为 26,903、18,632、21,867、27,052、28,871 字节。五份生成和模板契约校验成功，预览命令没有发布或写入数据库/MinIO。教室卫生 SHA-256 为 `a8127af10d627e7502947496fd927ba173528cfc3cd17726775f2160eea56288`。
- **当前差异**：数据库中除已发布的到课率外，其余六类均无模板记录；公共宿舍卫生不提供批量导入，不要求发布导入模板。其余五类均有导入功能，模板生成和契约校验已确认成功，且目标 Web 基线提交包含教室卫生导入入口；因此这五类都是待补模板项。目标库七类均启用且参与通报；本地迁移 026 将教室卫生与公共宿舍卫生排除出通报类型，云端状态与该迁移后的配置不同，需与部署版本区分。
- **用户范围决定（2026-10-25）**：本轮补齐无机课堂、早锻炼/早签、晚归、宿舍卫生四类；教室卫生、公共宿舍卫生暂不处理。到课率已完成发布，不重复创建。
- **用户执行回执（2026-10-25）**：第一条含 `set -eu` 的循环发布命令导致交互终端断开；用户随后从 `/root` 使用相对 `.env` 查询，命令因找不到 `/root/.env` 而没有执行。之后用户使用绝对路径运行只读 SQL，四类均返回空模板字段。四类随后均由 CLI 单条发布成功：`UNATTENDED_CLASSROOM_IMPORT` v1、`PUBLISHED`，SHA-256 `8a3532c5ef6e866b96bf84428e2359c49f25f26de15f21040d6e2a8535889093`；`MORNING_ATTENDANCE_IMPORT` v1、`PUBLISHED`，SHA-256 `293673caa92bd15e50e5e8bba70f0b9e3f5d0e2be07fd63477baf4597fb2d0f6`；`LATE_RETURN_IMPORT` v1、`PUBLISHED`，SHA-256 `8bf6d44c3a0ec55a6f6d2d473ba30702e1c7936c92c12e2d8de77868f7af81de`；`DORMITORY_HYGIENE_IMPORT` v1、`PUBLISHED`，SHA-256 `387dc2a19e59caf4e8fa0907c98ce1ae4ec3e3e90f22404170092cee5e37cb69`。四个哈希均与各自发布前预览一致。
- **数据库状态核对回执（2026-10-25）**：用户使用绝对 Compose 与 env 路径查询四类模板，均为 v1 `PUBLISHED`，关联文件均为 `AVAILABLE/TEMPLATE`。数据库文件名和大小分别为：`unattended-classroom-v1.xlsx`（26,903 字节）、`morning-attendance-v1.xlsx`（18,632 字节）、`late-return-v1.xlsx`（21,867 字节）、`dormitory-hygiene-v1.xlsx`（27,052 字节）。
- **浏览器验收回执（2026-10-25）**：用户确认页面使用已无问题，视为本次四类模板功能问题已解决。未记录逐类 HTTP 状态、响应头或 XLSX 打开截图，因此这里只记录用户对功能结果的确认，不补写未提供的底层验收细节。源码路由映射为：无机课堂 `GET /api/v1/imports/course-records/template`；早锻炼/早签、晚归、宿舍卫生 `GET /api/v1/imports/inspections/template`；到课率走 `GET /api/v1/imports/attendance/template`。

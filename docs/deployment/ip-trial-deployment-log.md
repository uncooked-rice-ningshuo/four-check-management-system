# 腾讯云 IP 试运行部署与迁移记录

本文件记录腾讯云 IP 试运行的部署版本、迁移账本、操作结果和待解决问题。目标环境的输出由用户从服务器提供；没有服务器终端证据的状态明确标为未核验。不要在这里保存 .env、密码、Token 或个人数据。

## 当前状态

更新时间：2026-10-25。腾讯云事实仅依据用户粘贴的终端输出；本轮未连接服务器或发起生产变更。

已建立每周跟进，检查关联任务中的新迁移证据并同步本日志与项目状态；只做记录和状态跟进，不会自动连接或修改生产环境。状态无变化时不通知。

- **服务入口**：公网 IP 的 TCP 8088，Compose 项目 `four-check-ip-trial`，目录 `/opt/four-check/ip-trial`。该更新只涉及此项目的 API/Web 与迁移；不操作 1Panel MySQL、独立 PostgreSQL、其他容器或卷。
- **云端最近基线**：r3a 只读预检于 2026-10-25 通过，API/Web 均为 `20261025-r2`，四服务健康，迁移账本连续至 `025_academic_week_import_template_versions.sql`；历史 `OTHER` 空说明三类计数均为 0。脚本没有修改服务或数据库，`--apply` 尚未执行。
- **模板问题**：到课率、无机课堂、早锻炼/早签、晚归、宿舍卫生 v1 已发布，文件元数据均为 `AVAILABLE/TEMPLATE`；用户确认页面使用已无问题。教室卫生和公共宿舍卫生模板本次暂不补。
- **当前候选包**：[four-check-update-20261025-r3a.tar](../../output/releases/four-check-update-20261025-r3a.tar) 及 `.sha256` 已在本机生成。包基于提交 `b78a5df`，有 API/Web `linux/amd64` 镜像及迁移 022–028；应用镜像仍为 r3，并支持实际观测到的 r2/r2 起始基线。r3 首版包已经安全退出，不要复用它的解包目录；r3a 需上传到新的 `/opt/four-check/updates/20261025-r3a`。旧 r2 包只含 022–025，不能用于当前代码；`deploy/ip-trial/build-release.sh` 仍是更早的历史构建路径。
- **迁移影响**：026 将教室卫生、公共宿舍卫生排除出通报范围；027 增加逐周应交与零记录回执；028 预置三类卫生原因，并要求新写入/修改的 `OTHER` 原因带说明。028 用 `NOT VALID` 保留旧记录，但未填写说明的旧 `OTHER` 行在编辑时需补充说明。脚本会在只读预检中报告数量；若非零，先评估接受方式，不要直接应用。
- **资源观察**：r3a 只读预检时根分区可用 4.5 GiB、RAM 可用约 1.0 GiB、swap 已用约 725 MiB；脚本要求至少保留 3 GiB 加数据库与 MinIO 数据量的两倍空间，已报告预检通过。资源随时间变化，真正执行前仍须再次核对 `df -h /`、`free -h`，不能在云服务器构建镜像。
- **当前部署状态**：r3 镜像已通过 `docker load` 导入；首版脚本在镜像基线校验处安全退出。r3a 外层/内层校验与镜像导入均成功，只读预检通过；`--apply` 尚未执行，生产服务和迁移账本没有因此改变。原 records 500 的用户登录态重放仍未完成，单独作为未解决问题跟进。

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
- **结论**：只读预检输出“只读预检通过”，没有停止容器、创建备份或写数据库。待用户确认维护窗口后才执行 `bash /opt/four-check/updates/20261025-r3a/release/upgrade.sh --apply`；执行完成后再检查迁移到 028、四服务健康、ready 200 与业务页面。

## 待办与完成条件

1. r3a 上传、完整性校验、镜像导入和只读预检已完成；本次预检基线为 r2/r2、账本到 025、旧 `OTHER` 空说明计数为 0，磁盘门槛通过。不要重跑旧 r3 解包目录。
2. 用户确认维护窗口后，按包内说明再次确认资源并执行 `nohup ... upgrade.sh --apply`。若 SSH 断开或结果不明，不要重复运行；先查看日志和备份目录。
3. 更新后核对 API/Web r3、迁移账本到 028、四容器健康及 `8088/health/ready=200`，再在浏览器强制刷新检查页面和已发布模板下载。不得在生产库做真实导入确认写入。

**完成标准**：云端 API/Web 均为 r3，账本连续到 028，迁移后置条件和四容器健康检查通过，ready 返回 200，页面与模板下载通过只读/非破坏性验收。迁移状态、代码加载、业务页面验收分别记录；r3 包本地构建完成不代表云端已更新。

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

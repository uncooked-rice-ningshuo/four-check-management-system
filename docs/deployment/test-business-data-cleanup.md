# 日常检查与通报测试业务数据清理

本说明维护本机 PostgreSQL、本地 Compose 与腾讯云 IP 试运行环境的可重复清理流程。脚本源码在 [`deploy/ip-trial/clean_test_business_data.py`](../../deploy/ip-trial/clean_test_business_data.py)。所有模式都会清理日常检查/导入/提交/通报业务数据；每次执行前都要先预览并备份数据库。本机 PostgreSQL 模式只清理数据库，不访问对象存储；Compose 和 IP 试运行模式另外清理关联 MinIO 对象并备份对象存储。

这里的“清理测试业务数据”不是重建数据库：账号、权限、组织/学生基础数据、学期周次、检查类型、应交配置、字典和既有审计日志都会保留。若要重建这些基础配置，应另行使用明确的本地数据库重建流程。

## 清理范围

脚本清理检查记录及类型明细/责任关联、检查导入批次和行、部门提交及修订、周报版本与记录/零回执快照、审核、更正、调整、通报 Word/Excel 输出尝试、检查导入模板及相关文件记录和对象。

脚本保留账号及会话、角色/权限、组织和学生等主数据、学年学期周次、检查类型、卫生原因字典与区域、周报应交配置、早签周计划、通报文件版式设置、各周通报日期、学期通报年级范围、主数据/校历导入批次及源文件、系统设置和既有审计日志。执行后在审计日志新增一条清理事件。清理不是删库或重建数据库。

脚本兼容 r6a 的迁移 001–037 基线：迁移 043 之后才创建的通报文件尝试表会在表存在时清理，在旧库中缺失时按 0 处理；通报输出配置和周次/学期设置始终保留。预览会显示这些配置表的保留行数。

## 执行方式

### 本机 PostgreSQL（宿主机服务）

适用于 PostgreSQL 直接运行在开发机上的情况，不依赖 Docker。目标固定为仓库根目录当前机器的 `/tmp` Unix socket、端口 `5432` 和数据库 `four_check`，并校验数据库 owner 仍为 `four_check`。脚本以当前 macOS 用户对应的本地 PostgreSQL 超级管理员角色连接，以便完整备份并清理由不同本地角色创建的表；它拒绝其他数据库、owner 或网络连接目标。执行前必须停止本机 API/其他数据库客户端；若检测到其他 `four_check` 会话，脚本会拒绝清理。

先预览并核对迁移账本和清理表行数，再执行：

```bash
python3 deploy/ip-trial/clean_test_business_data.py --target local-postgres
python3 deploy/ip-trial/clean_test_business_data.py \
  --target local-postgres \
  --apply \
  --confirm '清理日常检查和通报测试数据' \
  --actor-id '<本地启用中的超级管理员 UUID>'
```

此模式备份并验证 PostgreSQL dump，清理事务数据、模板/文件元数据并新增一条审计记录；账号、权限、主数据、学期周次、检查类型、应交配置、系统设置和已有审计均保留。备份默认位于 `/private/tmp/four-check-local-cleanups`。由于此模式不连接 MinIO，报告/导入/模板对象文件本身不会删除；预览和清理结果会报告留下的对象数与字节数。清空数据库文件元数据后，这些对象可能成为孤立对象，可在对象存储恢复运行后另行盘点清理。

清理成功后，从仓库根目录将当前迁移应用到本机数据库：

```bash
PGHOST=/tmp PGPORT=5432 PGUSER="$(id -un)" PGDATABASE=four_check \
  MIGRATIONS_DIR="$PWD/server/migrations" sh server/scripts/migrate.sh
```

然后按当前本机 API 启动方式重启 API；模板记录和对象都已清理，数据库迁移后还须通过 `server/cmd/publish-attendance-template` 按八类检查发布当前代码对应的模板版本，并核验每类均恰好一个 `PUBLISHED` 版本。

### 本地 Compose 环境

本地目标仅允许当前仓库根目录、Compose 项目名 `four-check-management-system` 和本机 Unix socket Docker context。清理必须在应用迁移前完成：先停止 API/Web，再只启动 PostgreSQL 和 MinIO；不要先启动全栈，以免迁移先于清理运行。

```bash
docker compose stop api web
docker compose up -d postgres minio
```

等待 PostgreSQL 与 MinIO 健康后，先预览。预览列出迁移账本、每张清理表的行数、待删文件对象数和容量；核对目标后再执行。`<本地超级管理员 UUID>` 必须是本地数据库中启用的超级管理员账号。

```bash
python3 deploy/ip-trial/clean_test_business_data.py --target local-compose
python3 deploy/ip-trial/clean_test_business_data.py \
  --target local-compose \
  --apply \
  --confirm '清理日常检查和通报测试数据' \
  --actor-id '<本地超级管理员 UUID>'
```

脚本备份数据库和 MinIO 数据卷，校验备份后才删除对象及业务行。备份默认位于 `/private/tmp/four-check-local-cleanups`。执行成功后 API/Web 保持停止；此时再运行 `docker compose up --build`，由 `migrate` 服务应用尚未执行的迁移并启动新代码。清理会删除导入模板记录与对象，数据库升级后还须通过 `server/cmd/publish-attendance-template` 按八类检查发布当前代码对应的模板版本，再核验每类均恰好一个 `PUBLISHED` 版本。

如果本地 Docker daemon、PostgreSQL 或 MinIO 未就绪，`local-compose` 模式会停止而不清理；若本机 PostgreSQL 是宿主机服务，使用上方 `local-postgres` 模式。不要将其他数据库或网络主机作为本地目标。

### 腾讯云 IP 试运行环境

在更新包 `release` 目录先做只读预览：

```bash
python3 clean_test_business_data.py
```

确认目标确为当前腾讯云 IP 试运行库，并逐项核对输出的迁移账本、表计数、待删文件数和对象大小。涉及删除时，先把本次预览计数和保留项给用户核对；确认后才运行 `--apply`。执行需要启用中的超级管理员 UUID：

```bash
python3 clean_test_business_data.py \
  --apply \
  --confirm '清理日常检查和通报测试数据' \
  --actor-id '<启用中的超级管理员 UUID>'
```

脚本会停止 API/Web，生成数据库 dump 和 MinIO 数据卷归档，验证两份备份后才删对象和数据库行；数据库行在事务内清理。MinIO 数据卷快照完成后会重新启动原 MinIO 容器，脚本等待其 Docker 健康状态变为 `healthy` 再删除对象，避免启动窗口内请求失败。备份目录位于 `/opt/four-check/backups/before-inspection-report-cleanup-*`。失败时保留备份并尝试恢复已删的 MinIO 对象和重启服务；若数据库清理已提交，则以备份为准人工恢复，不能直接重复执行未确认的恢复命令。

## 安全停止条件

- 仅允许目标目录 `/opt/four-check/ip-trial`、Compose 项目 `four-check-ip-trial` 和数据库 `four_check`。
- 必须有 PostgreSQL、MinIO、API、Web 四个 healthy 服务，数据库与 MinIO 必须挂载当前部署的命名卷。
- 表清单外出现 `inspection_`、`import_`、`department_submission_`、`weekly_report_`、`report_output_`、`semester_report_` 新表，或清理表出现未批准的入向外键时，脚本拒绝执行。
- 备份空间不足、镜像缺失、服务未就绪、管理员 UUID 不有效、确认短语不一致时停止。
- 不禁用审计、不删除审计行；清理范围外的配置和主数据不因“测试环境”而一起删除。

## 相关表维护

从 r6a 基线升级时，清理器在迁移 038–049 之前执行。因此迁移 043 创建的 `report_output_files` 必须按“存在则清理、缺失则计数为 0”处理；不能把它设为 r6a 预检的必需表。清理完成后迁移才会创建通报输出配置表。

每次增加/删除日常检查、导入或通报相关数据结构时，先检查所有迁移中的 `CREATE TABLE`、外键、触发器及 MinIO 文件引用，再同步维护：

1. `PURGE_ORDER`：按子表到父表顺序列出要删除的数据表。
2. `REQUIRED_TABLES` 与未知表前缀规则：让脚本识别当前受支持的 schema；新表未评估前应令预检失败。
3. `PRESERVED_FILE_REFERENCES` 和 `file_candidates()`：决定哪些对象随业务数据删除，哪些导入源文件必须保留。
4. `purge_sql()`：维护锁表、外键顺序、触发器恢复和清理审计事件。
5. 对应版本的升级器：若清理后需要重建字典、模板、发布状态或其他初始配置，在迁移后通过当前版本服务写入并审计，再增加相应只读验收。
6. 本文、当前版本部署说明与包生成器：保持操作说明、保护范围和实际脚本一致；旧版本的 `UPGRADE-R6.md` 作为 r6 历史记录保留，不用来指导后续包。

任何新的业务表或文件类别都要先在一次性数据库验证清理顺序及恢复逻辑；不要在目标业务库试跑迁移或用宽泛 `TRUNCATE ... CASCADE` 代替显式表清单。用户已确认应用测试通过时，不要重复运行应用测试；Docker 镜像构建只负责生成本次 API/Web 离线部署镜像，不代替也不触发应用测试。

清理完成后，检查/通报初始化逻辑由对应版本的更新器负责执行。r6 更新器曾应用迁移 031–037 并发布八类检查导入模板；后续更新器应按包内迁移清单增量应用迁移，并重新发布被清理的八类模板。新增需重建的配置时，同步更新当前更新器及其验证清单。

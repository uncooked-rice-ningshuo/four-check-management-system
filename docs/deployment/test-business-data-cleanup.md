# 日常检查与通报测试数据清理

本说明维护腾讯云 IP 试运行库的可重复清理流程。脚本源码在 [`deploy/ip-trial/clean_test_business_data.py`](../../deploy/ip-trial/clean_test_business_data.py)，本次随 r6 更新包发布。执行清理会删除业务测试数据并清除关联 MinIO 对象版本；每次执行前都要先预览，并保留可验证的数据库和对象存储备份。

## 清理范围

脚本清理检查记录及类型明细/责任关联、检查导入批次和行、部门提交及修订、周报版本与记录/零回执快照、审核、更正、调整、导出文件关联、检查导入模板及相关文件记录和对象。

脚本保留账号及会话、角色/权限、组织和学生等主数据、学年学期周次、检查类型、卫生原因字典与区域、周报应交配置、早签周计划、主数据/校历导入批次及源文件、系统设置和既有审计日志。执行后在审计日志新增一条清理事件。清理不是删库或重建数据库。

## 执行方式

在更新包 `release` 目录先做只读预览：

```bash
python3 clean_test_business_data.py
```

确认目标确为当前腾讯云 IP 试运行库，并逐项核对输出的表计数、待删文件数和对象大小。执行需要启用中的超级管理员 UUID：

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
- 表清单外出现 `inspection_`、`import_`、`department_submission_`、`weekly_report_` 新表，或清理表出现未批准的入向外键时，脚本拒绝执行。
- 备份空间不足、镜像缺失、服务未就绪、管理员 UUID 不有效、确认短语不一致时停止。
- 不禁用审计、不删除审计行；清理范围外的配置和主数据不因“测试环境”而一起删除。

## 相关表维护

每次增加/删除日常检查、导入或通报相关数据结构时，先检查所有迁移中的 `CREATE TABLE`、外键、触发器及 MinIO 文件引用，再同步维护：

1. `PURGE_ORDER`：按子表到父表顺序列出要删除的数据表。
2. `REQUIRED_TABLES` 与未知表前缀规则：让脚本识别当前受支持的 schema；新表未评估前应令预检失败。
3. `PRESERVED_FILE_REFERENCES` 和 `file_candidates()`：决定哪些对象随业务数据删除，哪些导入源文件必须保留。
4. `purge_sql()`：维护锁表、外键顺序、触发器恢复和清理审计事件。
5. [`deploy/ip-trial/upgrade-r6.py`](../../deploy/ip-trial/upgrade-r6.py)：若清理后需要重建字典、模板、发布状态或其他初始配置，在迁移后通过当前版本服务写入并审计，再增加相应只读验收。
6. 本文、`deploy/ip-trial/UPGRADE-R6.md` 与包生成器：保持操作说明、保护范围和实际脚本一致。

任何新的业务表或文件类别都要先在一次性数据库验证清理顺序及恢复逻辑；不要在目标业务库试跑迁移或用宽泛 `TRUNCATE ... CASCADE` 代替显式表清单。测试已单独通过时，构建更新包不应把应用测试嵌入 Docker image build。

清理完成后，检查/通报初始化逻辑由对应版本的更新器负责执行。本次 r6 更新器应用迁移 031–037、发布八类检查导入模板，并验证模板状态；日后新增需重建的配置时，应更新该更新器及其验证清单。

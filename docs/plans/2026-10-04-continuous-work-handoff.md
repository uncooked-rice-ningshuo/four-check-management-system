# 持续实施交接（2026-10-04）

> **2026-10-04 最新范围决定（优先于下文旧计划和历史记录）：** 当前后续开发及本批验收仅包含到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归五类。教室卫生、公共宿舍卫生列为待需求方确认，暂停后续开发；保留已有功能、数据及未来汇总通报接入能力，不删除、不据此隐藏入口。具体方案经用户确认并同步文档后再实施。完整七类规划保留，当前五类验收与两类延期分开记录。见[范围决定](2026-10-04-five-inspection-active-scope.md)。部门提交分组建议尚未批准。

## 用户授权与停止规则

用户要求持续完成项目相关后续任务，每完成一步检查 Codex 账户额度；启动下一项前，若最近一次任何适用额度剩余 ≤5%，停止并汇总。额度高于阈值时继续有意义的已授权工作，不以普通阶段汇报结束。用户明确允许因上下文把任务交给其他会话，接续会话同样执行本规则。不购买额度，不使用重置券，不自行创建定时自动化。

继承授权：同一提交退回重提、递增版本并保留每次修订；导入仅整批回滚，全部记录须未提交、未锁定草稿，保留原文件、批次、审计。不要再次询问。未授权提交/推送/PR/部署/业务库写入，不得新分支或 worktree。保留当前大量未提交改动。当前 `feat-0.1-2`、基线 `8be1aca`，工作目录 `/Users/ningshuo/code/my-projects/four-check-management-system`。

部门分组仍未获答复：计划“每周部门类型唯一提交”与普通成员提交本人记录有冲突。已经向用户询问“按成员提交、部门聚合”还是“经理统一提交全部门”；不要自行固定聚合迁移/API（015已用于文件清理，部门暂排016）。独立导入、认证缺陷、矩阵验收可继续。

## 本会话接续增量（优先于下方历史交接）

最新完成admin-read-contract：shared/api/adminReadResponses.ts校验getUsers/getCatalogs必需字段、集合、bool/角色、重复ID，异常502 INVALID_RESPONSE；19项客户端红绿通过。organization.Service.ListUsers成功nil→[]，JSON单测先红后绿，gofmt/organization聚焦/vet/full通过，无SQL变更。前端lint/66文件336项293.06秒/build通过；Chrome154报告2afa2cc3e4的10场景通过，异常/清理错误0；py_compile通过，无运行验证任务。

最新完成用户读取恢复：用户列表/组织目录各自失败提示和重试，不伪装正常空态或停用；目录错误时保留编辑输入、按钮及提交路径阻止保存。新增3组件红绿，聚焦5项及直接提交保护通过。最终前端lint/65文件317项302.36秒/build通过，Chrome154报告6f68a17be0的13场景通过，异常/清理错误0；无运行验证任务。前一次eec9455240仅visibilitychange因目录仍新鲜没请求而失败，不计通过。详见2026-10-04-user-read-recovery.md。

当前已恢复：续行指令后重新查询额度剩余100%，goal为active；上次5%停止为历史节点。已完成用户编辑Select标签与筛选表单ID冲突修复：Form name独立前缀，314项前端全量299.51秒、最后聚焦2项/lint/build、Chrome154报告92bdbf97b0的11场景通过，无运行验证任务。浏览器中一次首次新增请求超时未复现，保留记录，不将其计通过。目标8080/live、5173为200，8080/ready503，未核代码版本或迁移，未操作业务服务。分支仍为feat-0.1-2，所有未提交改动保留，未提交/推送/部署。

最新完成有效引用：Postgres.AssignUserRole在事务内检查引用UUID、部门/角色enabled，固定顺序FOR SHARE保护至提交；无效400，未知数据库错仍500。六种新增实际PG场景包含服务读完后停用、缺失/非法UUID，账号/审计/会话不变；最终24项40.786秒通过。Chrome154报告3d0569a756的11兼容场景通过，异常/清理错误0；后端gofmt/vet/full/race、前端lint/65文件314项286.08秒/build通过。详见2026-10-04-assignment-reference-validation.md。

恢复工作时先重新核对额度、分支/工作区和当前代码。候选独立缺口是客户端编辑时版本未随请求携带（现仅服务执行期间CAS）、用户编辑Select标签关联、AVAILABLE孤立文件发现；本轮未开始这些改动。公共宿舍卫生跨楼责任和部门提交分组仍待用户确认。目标环境未加载、真实角色总验收与阶段总门禁仍未完成。

历史增量组织角色完整性：organization.UserAssignment新增SessionVersion json:-，Postgres.ListUsers加载；服务仅采纳请求dept/role，其余资料及版本来自已存账号，缺目标ErrUserNotFound→404。AssignUserRole SQL先CAS再关联/撤会话/审计，冲突409。单测伪造审计/JSON隔离、HTTP401/403/404/409通过；实际PG新增两竞态先红，18项26.612秒通过；backend聚焦/vet/full/race通过。frontend lint/65文件314项280.95秒/build通过，Chrome154报告4f25a2b5d3的11场景通过，异常/清理错误0。之前304b9c1b31因Select无accessible name失败不计通过，标签关联列独立缺口。当前无运行验证任务，最新自行查额度6%；≤5%不新开任务。下一候选后端部门/角色有效引用验证：前端只列enabled，服务/仓储当前未校验；先复核再定范围。

历史增量普通账号并发：UpdateAccount WHERE session_version=account.SessionVersion，SetStatus新增expectedVersion由service读取传入；4实际PG交错先红后绿，现16项25.600秒，HTTP409和backendvet/default/racefull通过。浏览器发现未分配账号undefined/空串比较误判，UserManagementPage归属比较统一??''；新增PhaseOnePages1项红绿，聚焦2项通过。最终Chrome154 276b27992310组含StrictMode首次/新增取消/A-B/保存重开/409重试通过。前端最终lint/65文件314项275.21秒/build通过，无运行任务。之前前端313项277.99秒是空值修复前历史证据。最新自行查额度9%。下一项组织角色分配审计After目前直接采用客户端完整UserAssignment，且AssignUserRole SQL无读取版本条件；准备伪造审计字段及真实PG交错回归。客户端编辑时版本仍是独立缺口，当前仅服务执行期间CAS，不能声称端到端乐观锁完成。

历史增量：最新完成恢复CLI：cmd/reconcile-files默认预览，只查询Bucket/stale/limit候选，无MinIO访问或写入；apply要求ACTIVE superadmin且mustChange=false和reason。limit1–1000、older-than>=5m、timeout1s–5m，复用Reconcile；部分失败JSON统计+exit1。实际PG/MinIO/race最终3.655秒含真实二进制、缺015拒绝、三类actor拒绝、审计故障/重试/分批/保护范围/输出脱敏；backendvet/default/racefull通过。额度15%，无运行任务，下一项只读复核普通账号更新和状态变更并发。使用brainstorming技能完成最小设计，沿既有持续授权实现，无新分支/提交/部署。

历史增量：PENDING恢复大小/状态冲突/暂时读失败保护已代码完成。11组主场景实际PG/MinIO/race4.556秒通过；最终backendvet/default/racefull通过，无运行任务。详见2026-10-04-file-recovery-safety.md；额度17%。下一项cmd/reconcile-files默认只预览DB候选，显式apply需要启用中已完成改密的superadmin+reason，Bucket/older-than>=5m/limit<=1000/timeout约束；真实PG/MinIO验证预览零写入、权限拒绝、执行和部分失败。

历史增量：Bucket隔离。Open/ReadVerified/Discard在对象操作前核对非空Bucket一致；Reconcile SQL按Bucket选候选且保留服务guard。真实PG/MinIO五路径及limit1旧Bucket阻塞当前Bucket均先红后绿，最终8组（含6Bucket子场景）2.616秒；storage聚焦、backendvet/default/racefull通过，额度18%，无运行任务。下一项先修recoverPending无字节上限和Store/failStore状态更新失败仍删除对象的竞态，再实现恢复CLI。

历史增量：文件恢复队列持续推进，015新增object_removed_at和部分索引，成功出队/失败更新updated_at让位，清理结果与审计同事务，Ready要求015。实际PG/MinIO/race7组2.573秒、迁移保留/缺015就绪2.538秒、模板CLI5.437秒以及backendvet/default/racefull通过。额度20%，无运行任务。准备CLI时发现Service读取/Discard/Reconcile未核file.Bucket与objects.Bucket，先加跨Bucket实际回归和保护，再实现恢复入口；部门迁移暂排016。

历史增量：导入源文件补偿已修复。5组实际PG/MinIO/race、旧导入/CLI兼容、backendvet/default/racefull通过；Chrome154最终7c787a9207的11组及frontend lint/65文件313项276.98秒/build通过；三次早期浏览器测试脚本文案/mc错误分类断言失败已记录不计通过。详见2026-10-04-import-source-compensation.md。额度22%，无运行任务；下一项实际PG/MinIO复现FAILED队列饥饿，设计持久化清理完成标记使有限批次推进，运行入口后续接入。

以下为历史增量，当前进展以上方为准。

最新完成密码并发保护（2026-10-04-password-concurrency.md）：AccountRepository.UpdatePassword新增expectedVersion，self/adminreset传读到版本，SQL CAS失败ErrVersionConflict→409 VERSION_CONFLICT；自行改密在FindByID后再核ACTIVE和actor版本，旧actor401，禁用目标管理员重置不变。两表单useRef+submitting防重复，失败保留。实际PG新增3旧写入和2旧actor先红后绿，现12项21.390秒，刷新兼容6.929秒；Chrome154 5eee07737f13组含延迟409通过；backendvet/default/racefull、frontendlint/65文件313项274.09秒/build通过。一次前端全量误在聚焦失败后启动已精确停止，不计通过；后续最终全量通过。额度剩余25%，无运行任务。下一项只读发现storage.Service.Discard把AVAILABLE源文件传给MarkFileFailed，但store只接受PENDING，API批次创建失败补偿实质失效。准备实际PG/MinIO红绿，并加本人/未引用/IMPORT_SOURCE保护与创建批次竞争；目前未改storage运行时。


最新完成登录账号版本保护（2026-10-04-login-account-version.md）：SessionRepository.Save新增expectedVersion，Login校验时账号版本传入；Postgres.Save账号FOR UPDATE核ACTIVE/版本再INSERT，否则ErrInvalidCredentials无会话。TestAccountMutationAtomicPostgres新增改密/禁用/角色调整后延迟登录3项实际PG先红后绿，现共7项14.746秒；刷新PG兼容6.874秒。Chrome154 be48eddf5a12组通过；backendvet/default/racefull、frontendlint/65文件311项274.12秒/build通过。额度剩余28%，无运行任务，分支再次确认为feat-0.1-2。下一项核验UpdatePassword缺expectedVersion导致两个校验完旧密码的请求覆盖新状态；准备实际PG暂停UpdatePassword交错测试，再补409并发保护。当前仅只读定位未改。


最新完成认证错误和账号原子性，见2026-10-04-auth-errors-and-account-atomicity.md。Login/AuthenticateAccessToken/ChangePassword保留内部错误，handler统一500 AUTH_SERVICE_UNAVAILABLE，Logout失败不清Cookie；UI退出失败Alert重试，两处改密改用AuthProvider.changePassword，成功清本地会话/缓存而不再次logout。进一步实际PG复现四类账号操作后RevokeAll失败导致部分提交，已把UpdatePassword/UpdateAccount/SetStatus和会话撤销/审计同事务，取消service二次RevokeAll，更新账号在事务内读取返回。实际PG/race四项红绿，刷新兼容也通过；Chrome154 9fbfeacb6b 12组通过；backendvet/default/racefull、frontendlint/65文件311项273.13秒/build通过。额度剩余30%，无运行任务。下一项核验Login读旧密码后延迟Save与改密/禁用交错：store.Save当前仅INSERT、无expectedVersion，准备实际PG红绿证明后修复。临时脚本/private/tmp/four-check-account-mutation-test.py，日志同前缀-result.log。


最新完成登录恢复UI，见2026-10-04-login-recovery-ui.md。AuthProvider提供recoveryError/recoverSession，StrictMode/快速点击共用promise；500/network提示原位重试，401无恢复按钮；恢复时禁用登录并拦键盘提交，失败保留输入，成功清缓存回原路由。authApi刷新失败清内存token。Chrome154 b6ca83cb1d共12组含375px通过，前端lint/64文件308项263.54秒/build通过。额度剩余33%，无运行任务。下一项只读发现auth.Login和AuthenticateAccessToken吞掉账号仓储内部错误，handler也全映射401，准备补红绿测试和安全500分类；不改变无效凭据401语义。


最新完成刷新原子轮换，见2026-10-04-refresh-atomic-rotation.md：FindRefreshSession不消费，prepareTokens无副作用，Rotate按accounts→sessions锁并复核状态/版本，撤销旧与保存新同事务。内部500 AUTH_SERVICE_UNAVAILABLE保留Cookie，明确失效401清Cookie。实际PG/race覆盖保存故障、双刷新、实际锁等待、过期/撤销/禁用、真实改密交错；Chrome154报告54c63b6d4f共11组通过。backendvet/default/racefull、frontendlint/63文件304项270.81秒/build通过。额度剩余35%，无运行任务。下一项补登录页初始化故障提示及原位重试，保持失败回登录页；成功响应丢失仍未修复。临时脚本/private/tmp/four-check-refresh-atomic-test.py，日志同前缀-result.log。


最新完成到课率旧filter-options授权修复，见2026-10-04-attendance-filter-scope.md。新handler复用SQL context-options view，返回原结构，新增可选班级grade显示元数据；完整分页失败关闭。实际PG/race五种授权及201班级分页通过；Chrome154报告cf2010e275、695c3e44f3各12场景通过；前端lint/63文件304项265.55秒/build、backendvet/full通过。额度剩余38%，没有运行中测试。下一项只读发现auth.Refresh先Consume提交后issueTokens.Save，保存失败会丢旧会话；准备先红绿复现再做原子轮换及实际PG并发/失败回滚验证，不放宽旧令牌重放，也不把数据库原子修复声称解决响应丢失。下方旧“尚未修改”及额度为历史节点。

公共宿舍卫生进展：仅新建 public_dormitory_hygiene_template.go 及测试（11字段、对象区域/责任宿舍独立），刻意不注册 InspectionTemplate/CLI/API/目录能力；gofmt/vet/full通过。发现字段评审第50行允许跨楼而API/仓储限制同楼，已用异步问题请用户选择，必须等答案才实现相关业务逻辑；不能以超时视为同意。额度剩余44%。等待期间继续独立缺口：当前只读核对到课率旧 filter-options 忽略 AcademicYearID 和 DENY，尚未修改该实现。认证刷新响应丢失仍待设计，不放宽单次消费。

最新完成教室卫生导入：11字段模板、按教室解析区域、共享卫生适配器抽到hygiene_adapter.go，实际PG/race确认关联失效零写入与时间/轮次精确存储；CLI/通用入口/页面/回滚已验。Chrome154报告f5cbfdad44共17组通过；前端lint/62文件300项（260.51秒）/build、后端vet/full及宿舍卫生实际PG/race兼容通过。当前无运行任务，额度剩余45%，下一项公共宿舍卫生导入。隔离脚本 /private/tmp/four-check-classroom-hygiene-import-test.py。下方数字为历史证据。

最新完成：受限导入班级/年级目标选择，见 `docs/reviews/2026-10-04-import-scoped-targets.md`。新导入与列表班级/年级独立；恢复重传保留原学期、周次、部门、班级、年级；到课率列表导航保留批次引用；失效目标阻止下载/上传。context-options增加grades与grade筛选；两类上传客户端校验返回范围。实际PG/race、Chrome154报告 `11321d27ab` 的12组通过，最终lint、62文件/298项（265.06秒）、build和backend vet/full通过。当前无测试在运行，最新额度已用50%、剩余50%，下一项教室卫生导入。下方旧数字为历史证据。

当前继续在原工作区，无新分支/worktree。公共年级引用、必须改密超级管理员批次访问、批次读取内部错误误报404、学年权限补全、取消审计同事务均已修复，见10-04各自验收记录。取消实际PG覆盖审计失败零状态变化、两个goroutine同版本仅一次成功及一条审计；后端完整race已通过。

宿舍卫生导入已完成模板/适配器/仓储/多责任确认/页面/回滚，实际PG/race与Chrome154.0.8037.97浏览器17组通过（`642a5c2fd0`），异常/清理错误0。最终前端lint、62文件/288项（257.41秒）、build通过；后端vet/full通过；晚归/早签实际PG/race兼容也通过。当前没有运行中的测试或隔离服务。新脚本 `/private/tmp/four-check-dormitory-hygiene-import-test.py`，固定短临时目录 `/private/tmp/fc-hygiene-*` 避免Unix socket路径过长。构造函数NewInspectionImportService现需dormitoryHygiene适配器参数。两类其余卫生手工动作元数据已同步。

下一项先补共用导入独立班级/年级目标选择：目前InspectionImportScopePanel只选学期周次、AttendanceImportDrawer只选部门，批次缺classId/grade，受限ALLOW可能看见按钮却无法获取模板。先真实角色复现，不能放宽后端授权或借用列表筛选。随后教室卫生、公共宿舍卫生导入。宿舍卫生多责任行受限批次拒绝已实际PG验证；其浏览器为超级管理员路径，不代表受限角色总验收。另可核查旧到课率filter-options学年选项过滤。部门分组仍待答复。

## 原交接完成及证据（历史）

按 `docs/project-status.md`、`docs/plans/2026-10-04-code-based-roadmap.md` 与 `docs/reviews/` 阅读，不仅看旧计划。两类区域维护、七类手工页面、四类导入（到课率、无机课堂、早签、晚归）、通用整批安全回滚和确认重校验已完成代码及各自隔离增量。纯工作流状态机已验，但无提交聚合和审核 API/UI。最新迁移014是回滚；部门迁移顺延015。

最近早签验证：后端 vet/default full tests、前端 lint/61文件279项/build，通过；浏览器 `7851cef450` 16组通过。晚归浏览器 `5c4f54ffa9` 17组通过，实际学生停用、宿舍调换、第二行并发重复、恢复重试、回滚500和编辑后拒绝均验；错误与清理错误0。Chrome154.0.8037.97。晚归实际 `TestLateReturnImportPostgres -count=1 -race` 通过。晚归最终前端 lint、62 文件/285 项测试、build 全部通过（256秒），后端 vet/full 同样通过。当前没有运行中的测试或隔离服务。最新额度 used32%、remaining68%，不要重置额度。

早签复用日期记录（时刻NULL、absenceCount=1）；晚归为学生单次时间事件（violationCount=1），宿舍当前关系匹配，宿舍不是事件身份。模板五列/七列，支持 `publish-attendance-template --type-code`，默认仅预览，隔离测试才`--apply`。支持通用 `/imports/inspections`；原到课率及课程入口类型隔离。类型化预览 `InspectionImportRecord` schemaVersion1，不再挤入课程结构；旧格式兼容。

目标业务8080/5173本次交接前重新只读探测仍拒绝连接（HTTP000），未重启加载代码/迁移/发布模板。所有最近浏览器、PG、MinIO写入均一次性服务和虚构数据。完整角色矩阵、MinIO故障总门禁及阶段二未完成。

## 原交接建议（已由上方接续状态更新）

1. 共用导入引用预览：`store.LoadAttendanceReferences` 的学生与班级SQL过滤s.enabled/c.enabled但漏g.enabled，确认阶段有年级启用检查。先实际PG复现“停用年级仍预览有效”，补回归再修；不要只做SQL静态检查。
2. 三类卫生导入：宿舍卫生→教室卫生→公共宿舍卫生。先读字段评审D3及领域ValidateDetails，按对象结果、责任班级/学生/宿舍、原因字典、日期+时刻/轮次要求实现，不能凭旧Excel统计列发明新口径。复用已验证通用模板/预览/确认/回滚，但逐类做字段黄金样例、权限范围、事务、实际PG与浏览器失败恢复。新增生产依赖须另确认；优先现有excelize。
3. 用户若回答分组问题，则工作流聚合/迁移/API/审核UI/工作台联动可推进；否则继续独立缺口。认证整页跳转丢失刷新响应的缺陷在`docs/reviews/2026-09-21-auth-navigation.md`，尚未设计恢复，不能降低单次消费安全性。
4. 完整跨角色/七类/故障门禁，随后统计通报按已确认流程；缺少实质业务决定时尽早异步问并做独立任务。

## 工具链与隔离命令

显式 PATH：`/Users/ningshuo/Library/pnpm/.tools/pnpm/11.7.0/bin:/Users/ningshuo/.nvm/versions/node/v24.12.0/bin:$PATH`。Node24.12.0/pnpm11.7.0，锁文件未变；默认命令可能旧版。默认Go1.27，浏览器 harness GOTOOLCHAINgo1.25.0，缓存 `/private/tmp/four-check-go-cache`。

前端运行时代码改动交付前 lint+全部test+build；测试约255秒，**全量Vitest期间不要编辑前端源码/测试**，否则已加载模块与新断言混用产生假失败。已知jsdom CSS提示与2MBchunk警告保留。后端gofmt本次文件、vet+全量；默认PG/MinIO未配置用例SKIP、缓存不能充实际集成。

原生PG `/opt/homebrew/opt/postgresql@16/bin`，MinIO `/opt/homebrew/bin/minio`。所有隔离脚本需本地绑定权限，使用require_escalated并解释一次性无业务库；自动审查至今都允许。

浏览器：在仓库根设置上述PATH并加PGbin；`GOCACHE=/private/tmp/four-check-go-cache BROWSER_DB_MODE=native BROWSER_SUITE=late-return-import PLAYWRIGHT_CLI_JS=/Users/ningshuo/.npm/_npx/81fb41e6b6793dc6/node_modules/@playwright/cli/playwright-cli.js python3 server/scripts/verify-browser.py`。自动清理，产物在忽略的output/playwright。morning-import、unattended-import、import-rollback等可单独选。不得输出fixture凭据；报告已脱敏。

临时native脚本仍在，可读并复用，每次freshDB，异常finally清理：
- `/private/tmp/four-check-late-import-test.py`：晚归实际PG/race；结果同名前缀-result.log。
- `/private/tmp/four-check-morning-test.py`：早签实际PG/race。
- `/private/tmp/four-check-crud-test.py`：七类CRUD实际PG/race。
- `/private/tmp/four-check-confirm-test.py`：课程预览后12类关联失效、第二行重复回滚。
- `/private/tmp/four-check-rollback-test.py`：安全回滚实际PG/race。
- `/private/tmp/four-check-course-integration.py`：freshPG+MinIO课程导入及CLI发布兼容/race，结果`/private/tmp/course-integration-{course,cli}.log`。

浏览器回归JS可参考`web/e2e/morning-import.js`、`late-return-import.js`。AntD双中文按钮可自动插空格，匹配正则；必填label用后缀正则。Playwright CLI上下文没有全局URL，取base用page.url().split('/').slice(0,3).join('/')。勿对带root的drawer locator使用has跨层嵌套。待全量检查完成再开始新前端修改。

## 沟通

中文简明进度，持续工作每60秒以内给有意义更新。每项核对额度并记录剩余，≤5%不再启动下一项，汇总已完成/验证/未交付/风险。用户允许后续交接，仍需传递上述规则，不能交接后让工作无人继续。不要因“阶段测试过了”写成总验收完成。

# 项目实际交付与历史问题复盘（2026-09-20）

> 后续执行更新：本报告下述问题为调研时快照。错误分类、字段错误契约及校历反馈/缓存已在本次工作区修复；隔离集成检查发现并修正了测试夹具冲突。最新验证与未完成项以[项目当前状态](../project-status.md)及[执行记录](2026-09-20-baseline-fixes.md)为准。


## 1. 核查范围与结论

核查 `feat-0.1-2` / `8be1aca` 的路由、服务、SQL、迁移、表单、测试配置、部署配置和历史任务；当前进度集中维护在[项目状态](../project-status.md)。本轮只整理 Markdown 和 AGENTS.md，未修改运行时代码、迁移、依赖或业务数据。

主要问题不是缺少更多计划，而是交付证据经常停在某一层：API 有实现但页面没有入口，测试用 mock 通过但实际 SQL 失败，代码更新但运行进程/迁移未同步，模板解析实现但目标库没有发布模板。这会反复产生“报告已完成，用户操作仍失败”。后续按用户操作链验收，并分开记录代码、环境和业务验收状态。

## 2. 历史问题与当前核对

下表的会话是历史来源；“已修复”仅限对应历史增量，未重演的浏览器行为不宣称今日通过。任务 ID 用于在 Codex 中检索原始上下文。

| 来源任务 | 实际发生的问题 | 当前证据与教训 |
| --- | --- | --- |
| `01a051fe-2249-7bb2-b492-14fb940b846d`（用户指定分支与统一导航） | 自行另建工作区；将日常业务和管理功能拆成两个入口；普通成员首页只有账号说明和链接 | 当前 AppLayout 与 `/` 已统一。保留分支规则和有效权限组合原则，不因角色名称另建操作空间 |
| 同上（部门筛选、导入与工作台） | 外层部门筛选被误当成导入门禁；个人待办和部门总量口径容易混淆；导入范围裸露 ID | 当前页面/抽屉已分目标选择，工作台 SQL 分个人与整体。将这些已确认规则写入 AGENTS，避免后续六类照搬早期错误 |
| `01a039cf-c328-7fa1-8d01-44ca5c761ac1`（注册审核） | 第一次只增加失败提示就报告修复，用户仍无法审核；随后才定位 PostgreSQL 参数类型推断 42P08 | 注册审核已有专用错误处理及数据库测试；但通用 writeDomainError 仍默认 400。须同时修根因与反馈，不能将局部修复外推全站 |
| 同上（返回登录与保存归属） | 按钮只有文字可点击；保存成功后草稿没清除，按钮仍显示可保存 | 测试须覆盖完整点击区域、提交后的状态和重试，而不只断言文案存在或请求发出 |
| `01a039e0-e578-7cc0-b645-17411788ca0d`（用户表格） | 声称采用 ProTable，实际保留自制筛选区，用户再次要求对齐组件原生布局 | 当前用户/注册/基础数据等使用原生 columns/search；后续复杂列表复用这一结构 |
| `01a05229-62e5-70d0-a62c-de313d6e8eea`（到课率页面） | 用 Input 代替关联筛选；固定业务还出现类型筛选；任意默认部门/学期；旧后端响应缺字段导致 undefined.map 白屏；宽表撑开页面 | 当前 AttendancePage 对 responsibleDepartments 有空值防护，使用 Select 与内部加载/空态。兼容字段只能避免崩溃，仍要核对服务是否加载新契约 |
| `01a0522e-003e-71e3-af22-c18c7e6094a9`（字典管理） | “修了回显”仍失败，因为没有测试新增→关闭→编辑；afterOpenChange 方案又造成先弹窗后回填；窄屏溢出 | 当前用 flushSync 挂载后设值；永久规则要求验证生命周期和首次回显，不固定某个实现技巧为唯一方案 |
| `01a07252-227f-7781-a272-cfd34fc89a33`（校历） | 学年/学期/周次编辑回显失败；筛选和编辑日期控件不一致 | 当前有范围日期组件、周次关联列及筛选；仍发现 save 缺少失败处理和跨页面缓存联动，列为待修而非沿用历史“全部通过” |
| `01a05232-cd7f-7b63-962b-bc699040db6c`（基础数据） | 007/011 未应用，查询缺表却返回 INVALID_REQUEST；编辑回显修复反复；关联选项只取前 100 条；为验收创建测试超级管理员后无法全部删除 | 当前已有分页关联加载及表单修复；业务库测试账号被审计引用的清理不能靠删审计。默认隔离验收、预设清理，不能复制旧测试方式 |
| 同上（基础数据重构） | 用户要求后端生成 ID、多班级课程、结构化排课及完整宿舍名称；曾针对当时测试数据明确允许清空 | 这些是已确认决定，不能再次让管理员填写技术编码；一次性清库许可不是未来迁移或测试的长期许可 |
| `01a075bc-62a6-7210-b819-0b6d31307bd0`（交接与 8080 联调） | 临时库通过但本地缺迁移；先查错 Docker 库，实际 API 连原生库；Ready 原先只 Ping；迁移后又发现模板为 0 | Ready 已检查部分 schema；09-07 已纠正连接目标。09-20 直连 Ready 再为 503，必须重新诊断，不能沿用历史恢复结论 |

未重新扫描或输出历史真实学生信息、凭据和审计明细。历史删除授权、执行结果及原因保持原上下文，不推断“代码迁移改成保留数据”就等于过去数据已经恢复。

## 3. 本次代码发现的未闭合点

1. [router.go](../../server/internal/httpapi/router.go) 的 `writeDomainError` 对未匹配错误仍返回 400/INVALID_REQUEST；基础数据、校历等沿用此出口。注册审核的专用 500 处理并未解决通用路径。需要保留合法 400 的同时为内部异常提供稳定、安全的 5xx 与诊断证据。
2. [AcademicCalendarPage.tsx](../../web/src/pages/admin/AcademicCalendarPage.tsx) 的 save 只有成功回调，只失效 academic-calendar；[AppProviders.tsx](../../web/src/app/providers/AppProviders.tsx) 未配置全局 mutation 错误反馈。可确定代码缺口，但实际失败提示/跨页更新表现仍需浏览器复现。
3. [App.tsx](../../web/src/app/App.tsx) 只有到课率检查路由；[records.go](../../server/internal/inspection/records.go) 和 [inspection.go](../../server/internal/store/inspection.go) 已支持七类 CRUD。因此“七类后端完成”和“七类业务交付”必须分开。
4. [012 迁移](../../server/migrations/012_phase_two_inspection_extensions.sql) 创建卫生区域、责任关联；仓储要求有效区域和责任对象。[masterdata/service.go](../../server/internal/masterdata/service.go) 仅十二种类型，没有卫生区域管理能力。除了六类表单，还缺它们依赖的配置/选项路径。
5. [importjob/service.go](../../server/internal/importjob/service.go) 仍使用 AttendanceRowValidator 和 ConfirmAttendanceImport，路由仅有到课率模板/上传；通用框架不能代替六类适配器。启动与迁移流程未提供完整的模板对象及发布初始化交付，测试夹具不等于业务库模板。
6. [compose.yaml](../../compose.yaml) 的 API 健康条件检查 live；[postgres.go](../../server/internal/store/postgres.go) 的 Ready 已加强 schema 检查，但 live 通过仍不能证明依赖或模板可用。
7. [web/package.json](../../web/package.json) 指定 pnpm@11.7.0，09-20 实际命令为 10.26.0；Node 为 v22.23.0，Docker 构建为 Node 24。记录差异，不据此断言测试挂起根因，不自动升级或重装依赖。
8. [二阶段计划](../plans/2026-08-30-phase-two-completion-implementation.md) 原有七张表、错误 typeCode、过时路径和“Task 5 全部完成才做退回流程”的依赖歧义，本轮按实际目录/已确认决策校正；增加 Task 10A/10B 把原范围中遗漏的前端与配置工作显式化。
9. [inspection_handlers.go](../../server/internal/httpapi/inspection_handlers.go) 的 DetailsValidationError 返回 `{error:{code,message,details}}`，其他分支大多为顶层 `{code,message}`；[httpClient.ts](../../web/src/shared/api/httpClient.ts) 只读取顶层。接入新六类页面前必须处理这个已存在的跨端差异，否则字段错误会丢失并降为 REQUEST_FAILED；本轮未修代码。

## 4. 本地只读验证及限制

2026-09-20 使用 Python urllib 在不携带会话的情况下探测。默认代理下曾得到 502/超时；改为 `ProxyHandler({})` 直连并采用 10 秒超时后，8080 live=200、ready=503，5173/8088 拒绝连接；未登录 work-context=401。没有登录、查业务库、读取密钥、重启进程、发布模板或执行迁移。

Ready 同时涉及 PostgreSQL/MinIO，本次 503 只能证明就绪门禁失败，不能据此确定缺迁移、对象存储停机或凭据错误。前端入口不可达也不等于代码构建失败。

同一调研上一轮：五个相关 Go 包测试通过，store 12 项真实数据库用例因未配置环境跳过；前端四文件抽查持续样式警告、未取得汇总即中断。警告本身不是已定位根因，测试中止也不等于业务断言失败。文档整理后只检查差异、链接与契约一致性。

## 5. 文档与规则维护方式

- [项目状态](../project-status.md)：维护当前总览、任务状态、运行证据和简短后续顺序。
- [二阶段总计划](../plans/2026-08-30-phase-two-completion-implementation.md)：保留原编号和验收门禁，补缺失交付项；计划中的预期命令不是已执行证据。
- [09-06 交接](../plans/2026-09-06-project-handover.md)、[09-07 联调](../plans/2026-09-07-local-api-verification.md)：保留日期化历史，入口指向当前状态。
- PRD、技术方案和页面梳理：维护需求/实现边界及已确认决策，不以未来完整产品描述冒充现有功能。
- [AGENTS.md](../../AGENTS.md)：只写可复用行为规则；不加入数据库数量、当前故障端口、任务完成百分比等易过时事实。

规则编写参考 OpenAI 官方 [Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md) 和 [Best practices](https://learn.chatgpt.com/guides/best-practices)，访问日期 2026-09-20。官方说明项目指令按目录层级加载，并建议保存实用、精简的长期约定；本仓库保留根级公共规则，把问题证据和执行计划放到链接文档中。新增的联调、表单和交付规则是本项目复盘结论，不冒充 OpenAI 对业务开发的专门要求。

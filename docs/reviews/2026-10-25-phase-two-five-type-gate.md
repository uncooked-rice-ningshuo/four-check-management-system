# 五类阶段二门禁本地验收（2026-10-25）

## 结论

**五类阶段二门禁通过并关闭。** 验收对象是 `feat-0.1-2` 当前工作区代码（含未提交改动），使用一次性 PostgreSQL、独立 MinIO、真实 API/Vite 和 Chrome 完成数据库、导入、页面和角色矩阵验证。用户确认发布版本已是最新；本轮不访问或修改发布环境。

范围为到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归。教室卫生和公共宿舍卫生继续按已确认决定暂停专项验收，不计入本门禁，也不声称七类全部验收完成。

## 门禁矩阵

| 检查层 | 结果 | 本轮证据 |
| --- | --- | --- |
| 模型与迁移 | 通过 | 默认浏览器套件和四个逐类导入套件均从空的 PostgreSQL 16.13 应用仓库全部迁移，包含 024、025。 |
| API 与数据范围 | 通过 | 真实 API 覆盖五类 CRUD、服务端部门归属、403/404 越权隐藏、角色授权与 DENY、状态和版本冲突。 |
| 页面 | 通过 | 五类列表、手工录入、编辑生命周期、加载/空/错误状态、窄屏和工作台均通过真实 Chrome 回归。 |
| 导入与恢复 | 通过 | 到课率默认套件及无机课堂、早签、晚归、宿舍卫生四个独立套件通过；覆盖模板下载、预览、确认、失败行、幂等重试和回滚。 |
| 部门批次与角色 | 通过 | 成员、部门审核员、学院只读者、办公室指定授权、普通系统管理员、超级管理员和校级独立批次均覆盖；五类退回/更正/重提/确认及确认后不可变通过。 |
| PostgreSQL/MinIO | 通过 | 存储、导入、MinIO、认证、模板 CLI 集成套件共 170 个顶层测试通过，零跳过。MinIO 故障暂停后恢复路径通过。 |
| 阶段边界 | 通过 | 工作台没有通报待办/伪进度，页面未请求阶段三通报接口。 |
| 全量代码检查 | 通过 | Go vet/全量测试、前端 ESLint/Vitest/TypeScript/Vite build 和 `git diff --check` 均通过。 |

## 验证记录

- **浏览器默认套件**：84 个场景通过，14 个故障注入，Chrome 156.0.8078.48；浏览器错误 0、清理错误 0。报告：[5a5ff60b0e](../../output/playwright/5a5ff60b0e/summary.json)。
- **部门提交流程/角色矩阵**：24 个场景通过；五类 API 与页面的提交、跨部门拒绝、退回/修正/重提/确认、学院只读、办公室指定范围、系统管理员/超级管理员、校级批次和阶段三负向断言均覆盖。报告：[fed2e51cdf](../../output/playwright/fed2e51cdf/summary.json)。
- **逐类导入**：无机课堂 19、早签 19、晚归 20、宿舍卫生 20 个场景通过；四份报告的浏览器错误和清理错误均为 0：[无机课堂](../../output/playwright/801ad0ed3e/summary.json)、[早签](../../output/playwright/52c6af8f61/summary.json)、[晚归](../../output/playwright/ec51024c6f/summary.json)、[宿舍卫生](../../output/playwright/b284f66048/summary.json)。
- **后端静态/单元检查**：Go 1.27.0 下 `GOTOOLCHAIN=local GOCACHE=/private/tmp/four-check-gocache go vet ./...`、`GOTOOLCHAIN=local GOCACHE=/private/tmp/four-check-gocache go test ./... -count=1` 通过。
- **真实数据库/对象存储集成**：PostgreSQL 16.13 + MinIO `RELEASE.2025-10-15T17-29-55Z`；`internal/store` 42、`internal/importjob` 75、`internal/storage` 25、`internal/auth` 20、`cmd/publish-attendance-template` 8 个顶层测试均通过，零跳过。服务和数据库均为临时实例，使用合成数据并在结束时销毁。
- **前端完整检查**：Node 24.12.0；ESLint 通过，Vitest 74 文件/396 项通过，`tsc -b` 通过，Vite production build 通过。仓库声明 `pnpm@11.7.0`，本机未缓存该版本；为避免下载或改锁文件，直接运行工作区已安装的 ESLint/Vitest/TypeScript/Vite 二进制，命令分别等价于 package scripts。构建保留已有约 2.21 MB 主 chunk 体积提示。
- **仓库检查**：`git diff --check` 通过，修改过的 Go 测试文件已 gofmt。

## 本轮修正的测试夹具

真实 PostgreSQL 执行发现两处过期测试假设：课程简化后教室选项不依赖排课关联；原无效输入的日期仍在有效周内且节次与学时一致；回滚测试把随机、未建档的提交 ID 写入受复合外键保护的记录。只更新了相关测试期望和合成夹具，未更改业务实现或数据模型。修正后重新运行相关真实 PostgreSQL/MinIO 集成并通过。

## 范围外与环境边界

- 本结论关闭的是五类阶段二门禁，不代表 PRD MVP 完整。统计口径、通报生成/审核/退回、通报内更正、定稿快照、Word/Excel 导出和归档仍未实现，应进入下一阶段规划。
- 两类暂停的卫生专项仍等待需求确认。
- 用户告知发布版本已是最新；本轮按要求只使用本地当前工作区进行隔离运行，没有探测目标端镜像、迁移账本、业务账号登录态或浏览器。
- 工作区仍有用户已有的 staged、unstaged 和 untracked 改动；本轮没有提交、推送、部署或改写业务库。

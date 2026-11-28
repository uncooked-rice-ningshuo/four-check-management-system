# 计算机学院日常管理系统

本项目面向学院内部日常检查工作，代码提供检查记录录入与 Excel 导入、部门审核以及通报版本管理。腾讯云环境目前处于试运行阶段。系统采用 React 前端、Go API、PostgreSQL 和私有 MinIO 文件存储。

## 项目状态

截至 2026-11-22，当前工作区已实现学年名单、固定年级授权、处分配置、学期违纪台账和本人消息中心。待处分事项、处分归档及相应业务通知尚待接入。各模块的实现和验证记录见[项目状态](docs/project-status.md)。

最近有记录的腾讯云试运行部署为 11 月 9 日的 r7。当次已核验迁移 038–049、服务就绪及八类导入模板发布。通报归档操作、Word/Excel 实际生成与查看、真实角色权限仍待业务验收。上述 11 月下旬功能尚待部署及真实账号验收。详见[部署回执](docs/deployment/ip-trial-deployment-log.md)。

## 功能概览

- **账号与权限**：注册审核、账号生命周期和角色管理。老师和主席团可关联多个部门；按授权来源校验动作、部门职责、检查类型、数据范围和有效日期。部门模板在授予时形成快照。
- **日常检查**：到课率、无机课堂、早锻炼/早签、宿舍卫生、晚归、晚自习、教室卫生、公共宿舍卫生。
- **记录导入与审核**：Excel 文件先解析和预览，再确认入库；部门归属记录按权限进入部门提交、审核、退回更正和确认流程。
- **校历与基础数据**：维护学年、学期、周次、组织、班级、学生、宿舍、教室、课程和字典等数据。管理员在「学期与周次 → 学年」配置年级、专业、班级的范围与顺序。
- **通报与追溯**：工作台按检查类型查看已确认数据、提交状态和版本记录；关键数据变更保留审计记录。部分旧版本未冻结当时的单位或提交人显示名称，详见[项目状态](docs/project-status.md)。
- **通报输出**：选择草稿或定稿版本后即时生成并下载 Word/Excel，导出保留审计，报告版本和状态保持原值。
- **处分管理**：在「学期违纪台账」核对每周最新学办定稿按当前发布规则计算的结果；查看和导出分别授权。「处分配置」维护累计项和处分档位。
- **本人消息中心**：通过顶栏或 `/notifications` 查看组织归属、角色和个人授权变更，支持分类查询、单条及全部标为已读。
- **文件管理**：上传文件存入私有 MinIO Bucket，由 API 校验权限后提供读取。

上述功能说明描述代码能力。各功能在目标环境的加载情况和验收状态请查看[项目状态](docs/project-status.md)。

## 技术栈

| 部分 | 技术 |
| --- | --- |
| 前端 | React 19、TypeScript、Vite、Ant Design、React Router、TanStack Query |
| 后端 | Go 1.25+、Gin，模块化单体 |
| 数据与文件 | PostgreSQL、MinIO |
| 本地部署 | Docker Compose |

```text
浏览器 → React Web → Go API → PostgreSQL
                              ↘ 私有 MinIO
```

## 本地启动

需要安装 Docker 和 Docker Compose。Compose 会启动本地数据库、文件存储、API 与 Web，并对 Compose 数据库执行尚未应用的迁移；请仅用于本地或一次性开发环境。

```bash
test -f .env || cp .env.example .env
```

启动前，编辑 `.env`，为数据库、JWT、初始管理员和 MinIO 分别设置独立的随机凭据。`.env.example` 中的值是待替换的占位符。`.env` 已被 Git 忽略，不要把它提交到仓库。

```bash
docker compose up --build
```

浏览器打开 <http://localhost:8088>。就绪检查地址为 <http://localhost:8088/health/ready>。首次登录后按提示修改初始管理员密码；普通用户注册后需要管理员审核。

全新 Compose 环境不会自动发布 Excel 导入模板；需在对应环境单独发布模板。

## 开发与检查

前端使用仓库声明的 `pnpm@11.7.0`。执行前先核对本机 Node.js 和 pnpm 版本；版本不一致时先定位工具链，再安装依赖或运行检查。后端使用 Go 1.25 或更新版本。

```bash
(cd server && go vet ./... && go test ./...)
```

```bash
node --version
pnpm --version
(cd web && pnpm install --frozen-lockfile)
(cd web && pnpm lint && pnpm test && pnpm build)
```

需要数据库或 MinIO 的集成验证时，只能连接一次性测试资源。相关测试可能执行全部迁移并写入数据；不要指向生产库、共享数据库或日常业务环境。前端和后端的详细开发约定见 [AGENTS.md](AGENTS.md)。

### 隔离真实服务浏览器回归

在 `web/` 执行 `pnpm test:browser`。脚本需要 Python 3、Go、Node.js 24、MinIO 服务端、mc 和 Google Chrome，默认通过 Docker 启动一次性 PostgreSQL 16。使用本机 PostgreSQL 工具时可设 `BROWSER_DB_MODE=native`。按业务选择用例，例如 `BROWSER_SUITE=evening-self-study pnpm test:browser` 验证晚自习录入、部门审核和周通报链路。脱敏结果保存在 `output/playwright/`。

### Midscene 可视化回归

Midscene 通过截图定位控件并操作页面，独立于现有的隔离 Playwright CLI 套件。使用 Node.js 22.12+、仓库指定的 pnpm 和已安装的 Google Chrome；示例已预配 OpenAI GPT-6 Luna 与 Responses API，在 `web/` 安装依赖后复制 `.env.midscene.example` 为 `.env.midscene.local`，只需填写 `MIDSCENE_MODEL_API_KEY`。只读登录表单冒烟默认启动本地 Vite，报告保存在 `web/midscene_run/report/`，失败跟踪保存在被 Git 忽略的 `output/midscene/`。

```bash
cd web
test -f .env.midscene.local || cp .env.midscene.example .env.midscene.local
corepack pnpm@11.7.0 test:midscene login-visual.spec.ts
```

如需对隔离环境中的其他页面回归，可在 `.env.midscene.local` 设置 `MIDSCENE_BASE_URL` 指向隔离应用地址。Midscene 会把页面截图发送到 OpenAI；仅使用合成数据，勿对生产或共享业务数据运行。

## CodeGraph（本地代码图）

仓库为 Codex 配置了项目级 CodeGraph MCP 服务（`.codex/config.toml`），固定使用 `@colbymchenry/codegraph@1.6.2`。Codex 需将本项目标记为可信后才会加载项目级 MCP 配置；首次启动时 `npx` 会从 npm 获取该版本。该工具只用于开发时的代码结构查询，不是应用运行依赖。

首次建立本地索引，在仓库根目录执行：

```bash
CODEGRAPH_TELEMETRY=0 npx --yes --package=@colbymchenry/codegraph@1.6.2 codegraph init --yes
```

索引位于本地 `.codegraph/`，不会提交到 Git；自动同步由 MCP 服务负责。匿名使用遥测在 Codex 配置和初始化命令中均已关闭。`codegraph.json` 排除了可能含敏感材料的参考文件、页面素材及输出目录。查看索引状态：

```bash
CODEGRAPH_TELEMETRY=0 npx --yes --package=@colbymchenry/codegraph@1.6.2 codegraph status
```

## 仓库结构

```text
web/                  React 前端
server/cmd/api/       Go API 入口
server/internal/      认证、权限、检查、通报和存储等业务模块
server/migrations/    只追加的数据库迁移
docs/                 项目状态、设计方案、验收和部署记录
compose.yaml          本地 Docker Compose 配置
```

## 项目文档

- [项目当前状态与后续任务](docs/project-status.md)：最近部署、验证边界和下一步任务。
- [产品需求文档](PRD-计算机学院日常管理系统.md)：产品范围、角色和业务规则。
- [技术方案](docs/plans/2026-07-11-four-check-management-system-technical-design.md)：架构、数据模型、权限和流程约束。
- [界面设计规范](Design.md)：页面布局、组件样式及交互反馈。
- [页面模块功能梳理](原型页面模块功能梳理.md)：导航、页面归属和交互范围。
- [日常检查调整方案](docs/plans/2026-11-01-daily-inspection-adjustments.md)：八类检查、导入和早签计划的近期调整。
- [按检查类型组织的通报工作台方案](docs/plans/2026-11-01-report-type-first-workbench-design.md)：工作台页面与数据查询调整。
- [腾讯云部署记录](docs/deployment/ip-trial-deployment-log.md)：试运行环境部署核验。
- [测试数据清理说明](docs/deployment/test-business-data-cleanup.md)：试运行环境测试数据清理流程。
- [CodeGraph 官方仓库](https://github.com/colbymchenry/codegraph)：本地代码图与 MCP 工具。

## 数据与安全

本系统处理学院业务记录和个人信息。开发、演示和回归使用虚构数据；生产或试运行数据仅在获批环境中按最小权限访问。不要在日志、问题报告或提交内容中包含密码、令牌、Cookie、完整身份证号、数据库连接凭据或上传文件原文。

### WebVPN 构建

学校网关会把模块入口包装为经典脚本。为该环境构建前端镜像时，在仓库根目录执行以下命令，再在学校部署配置中选择该镜像标签。

```bash
docker build --build-arg WEB_BUILD_MODE=webvpn -f web/Dockerfile -t four-check-web:webvpn .
```

离线试运行环境使用 `deploy/ip-trial/Dockerfile.web` 时可传入相同参数，并使用独立的发布标签。默认构建模式为 `production`。本地可在 `web/` 执行 `pnpm build:webvpn`，生成目录与 `pnpm build` 相同，均为 `dist/`。兼容模式将脚本合并为单个入口，构建时检查其经典脚本语法。部署后使用有效的学校 WebVPN 会话验证登录、登录后导航及深层页面刷新。

使用根目录 Compose 时，执行 `WEB_BUILD_MODE=webvpn docker compose build web`。本地构建后可在 `web/` 运行 `node scripts/verify-build.mjs webvpn` 或 `node scripts/verify-build.mjs production` 检查对应产物。

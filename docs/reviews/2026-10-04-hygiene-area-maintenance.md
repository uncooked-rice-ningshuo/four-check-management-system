# 2026-10-04 卫生区域维护接续验证

接续原会话的未完成工作，保留 `feat-0.1-2` 及已有未提交改动；没有提交、部署或写业务库。

## 已完成增量

- 补齐原导航测试的 QueryClientProvider，保留路由/权限断言；先复现 17 项渲染失败，再修复。
- 新增区域页面 StrictMode 回归，先复现保存失败后关闭重开仍显示旧错误，再在打开编辑时清除 mutation 错误；首次编辑、新增关闭后编辑、A/B 和重新打开均有组件覆盖。
- 补授权导航、关联分页和 HTTP 选项/分页/未登录/DENY 断言。
- 增加 `BROWSER_SUITE=hygiene-areas`，使用一次性原生 PostgreSQL、独立 MinIO、当前 API/Vite 与 Chrome。

## 本轮实际验证

显式 PATH 使用 `/Users/ningshuo/Library/pnpm/.tools/pnpm/11.7.0/bin` 与 `/Users/ningshuo/.nvm/versions/node/v24.12.0/bin`。锁文件未改。

- 路由、布局、管理入口聚焦：26 项通过；区域页面：3 项通过；客户端/导航：15 项通过。
- `GOCACHE=/private/tmp/four-check-go-cache go test ./internal/httpapi -run TestHygieneArea -count=1`：通过，命中两项区域测试。
- `pnpm lint`、`pnpm test`（50 文件/240 项）、`pnpm build`：通过；jsdom 样式提示和构建大包警告仍存在。
- `go vet ./...`、`go test ./...`：通过；包含缓存命中，默认外部集成跳过不视为数据库验证。
- `BROWSER_DB_MODE=native BROWSER_SUITE=hygiene-areas python3 server/scripts/verify-browser.py`：15 组通过（公共 7、区域 7、数据库审计 1），Chrome 154.0.8037.97，browserErrors 与 cleanupErrors 均为空。沙箱端口拒绝后使用审批允许的隔离执行；两次脚本定位错误修正后重跑。成功证据：`output/playwright/ac02044416/summary.json`。

真实场景覆盖两类创建、首次编辑、新增关闭后编辑、PUT 500 保留输入重试、关闭重开、停用确认、数据库持久化和三条变更历史；受限成员单一目标、创建成功、通用基础数据/其他类型写入 403、实时 DENY；390px 页面不横向溢出、表格可内部滚动。公共校历有完整控制台采集，区域专项仅断言未捕获异常，不宣称控制台完全无警告。

## 剩余与交付维度

| 维度 | 状态 |
| --- | --- |
| 模型/迁移 | 复用既有区域表和 masterdata，无新增迁移 |
| API | 鉴权、分页、响应异常、重复409、无效关联400及真实审计事务已验 |
| 页面 | 新增/编辑/启停/历史、异常恢复、关联缓存和分层已完成 |
| 模板/初始配置 | 本项不新增导入模板；目标环境区域配置未验 |
| 真实环境 | 一次性 PostgreSQL/MinIO 已验；业务 8080/5173 拒绝连接，未加载 |
| 角色验收 | 超级管理员、系统管理员全局维护、受限普通成员和DENY已验；完整跨业务角色总门禁未验 |

区域维护增量已完成；Task 10A 目标环境交付与阶段二总门禁保持未关闭。下一项公共宿舍卫生页面，再补教室卫生。用户已确认退回重提沿用同一提交、递增版本并保留修订；回滚采用整批且全部未提交/未锁定草稿基线，保留原文件、批次及审计。

## 收尾补强及缺陷根因

新增六项失败回归验证异常响应、缺失列表、重复分页与关联缓存，修复后聚焦 12 项通过；请求归入 hygiene-area/model Hook，抽屉按记录独立挂载，清除非空断言。全量前端和构建在最终源码重新通过。

真实 PostgreSQL 重复公共区域原返回 500：自动生成的唯一约束名为 `public_dormitory_hygiene_area_dormitory_building_id_floor_u_key`，截短后不含 name，旧分类漏判。先由一次性数据库读取实际约束并复现单测失败，再显式映射 ErrDuplicateName。`go test ./internal/store -run TestMasterDataErrorMaps -count=1`、vet 与全量 Go 测试通过；最终浏览器重放教室/公共区域重复请求均为 409 MASTER_DATA_NAME_EXISTS，无效关联为 400 INVALID_REQUEST。

最终隔离数据库确认区域审计总数十条，全部含操作者、请求标识、后值；四次修改含前值。注入失败、重复请求和无效关联未留下额外区域审计。API 服务使用脚本的 Go 1.25.0，默认 Go 静态/单元检查实际为 1.27.0；两者均为实际执行证据。一次重跑因装饰性 loading 图标影响按钮精确名称定位中止，快照已显示停用成功；改为断言状态列后最终重放通过，未增加等待时间。

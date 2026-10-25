# 每账号最多三个并发设备会话：技术方案

**状态：业务规则已确认；本文为实施规格，运行代码和数据库尚未修改。** 2026-10-25 用户确认：每账号最多 3 个并发会话；新设备登录超额时撤销最久未活动的会话。本文所称“设备”是一次独立登录形成的浏览器会话，不依赖不可靠的硬件指纹。同一浏览器多个标签共享同一 Refresh Cookie；清除 Cookie 后再次登录算新会话。

## 现状与目标

- `auth.Service.Login` 每次登录签发新的 Access Token，并通过 `SessionRepository.Save` 新增 `refresh_sessions` 行；没有数量限制。`Refresh` 原子撤销旧刷新行并插入新行。Access JWT 目前只有账号 ID 和 `session_version`，`AuthenticateAccessToken` 不检查单条刷新会话。
- `/auth/logout` 只撤销 Cookie 对应的刷新行；被撤销设备的已签发 Access Token 目前仍能用到 15 分钟到期。账号改密、禁用及权限敏感变更提高 `session_version` 并撤销全部刷新行，仍应保持原有即时失效语义。
- 目标是同账号最多三个有效的逻辑登录会话；第四次登录在一个事务内撤销最久未活动的旧会话并创建新会话。被撤销设备下一次受保护请求即收到 401，刷新亦返回 401。新登录失败不得踢掉任何设备。

## 数据模型和迁移

追加迁移 `028_login_device_sessions.sql`，不修改历史迁移：

1. 新表 `login_sessions`：`id uuid PK`、`account_id uuid FK`、`created_at`、`last_seen_at`、`expires_at`、`revoked_at`、`revoked_reason`（受控值，例如 `EVICTED`、`LOGOUT`、`ACCOUNT_CHANGED`）。索引 `(account_id, last_seen_at, created_at, id) WHERE revoked_at IS NULL`，以及到期清理索引。`last_seen_at` 使用服务端时间，不能由客户端提供。
2. `refresh_sessions` 增加 `login_session_id uuid` 外键，指向逻辑会话。新登录创建一条父会话和第一条刷新行；刷新轮换只更换刷新行，继承同一父会话 ID。新数据写入必须要求非空；历史数据回填完成并确认无空值后，再设置 `NOT NULL`。避免直接把 `refresh_sessions` 行数当设备数。
3. 历史有效刷新行逐行回填父会话，时间沿用原 `created_at`、`expires_at`；已撤销/过期行可按保留策略回填或保持空值并另行约束，不能在存在空值时直接加 `NOT NULL`。由于旧轮换链没有稳定会话 ID，历史残留刷新行各自代表一个可用登录；若某账号迁移时超过 3 条有效行，按同一排序规则保留最新 3 条，其余在迁移事务中撤销并审计。此上线动作会影响在线用户，需在发布计划中明确告知，并先在一次性数据库验证。
4. 父会话 `expires_at` 与当前刷新有效期同步；刷新成功时更新父会话到期时间。撤销父会话后，其所有刷新行即使仍有原值，也不得再轮换。旧行的物理清理与审计保留策略另行安排，不作为此功能正确性的前提。

## 登录、刷新与请求校验

### 登录限额

密码和账号状态先按现有流程核验；生成全部令牌材料后再进入 PostgreSQL 事务。事务按既有顺序锁定 `accounts` 行，重查 `ACTIVE` 和读取时的 `session_version`，再处理该账号的会话：

1. 按数据库时间将过期但未标记的父会话排除出计数；有效条件为 `revoked_at IS NULL AND expires_at > now()`。不要把历史已消费的刷新行计入设备数。
2. 若有效会话数已达 3，按 `last_seen_at ASC, created_at ASC, id ASC` 确定受影响会话并撤销至新会话可加入；相同时间有稳定结果。一个新登录只增加一个会话。记录被挤出的会话 ID 供审计；不提高账号级 `session_version`，否则会踢掉其余两台设备。
3. 插入父会话和刷新行，写入“超额挤出”和“新登录”审计，再提交。任一步失败，整体回滚并返回 500；不得返回新 Token 或错误地说密码无效。两个同时登录由账号行锁串行化，不会超过 3 个。

“最近活动”按服务端受保护 API 请求更新时间记录；为了避免每次请求都写库，可在成功鉴权且 `last_seen_at` 距今至少 5 分钟时更新一次，更新仍需检查 `revoked_at IS NULL`。5 分钟内按上次记录的时间排序；这个精度需在界面文案中说明，不能称为精确最后操作时间。登录和成功刷新也更新 `last_seen_at`。长期后台轮询是否算活动应在实现时检查现有请求模式；若会导致闲置设备永远占位，应改为只统计明确的人机交互请求，或将字段命名为“最近请求时间”并接受其排序语义。首版建议按“最近成功请求时间”执行。

### Access Token 立即失效

JWT 增加 `sid`（父会话 ID），继续保留账号 ID、账号 `session_version`、签名与到期校验。`AuthenticateAccessToken` 在现有账号检查后，用 `sid + account_id` 查询未撤销、未过期的父会话；任何数据库故障返回服务不可用，不能当作 401。此检查适用于所有受保护 API，含只读敏感数据。需要评估额外数据库读取的延迟和连接池负载，不用进程本地缓存掩盖撤销状态。被挤出设备在下一次 API 请求 401；已打开页面无需服务端推送即可在下一次请求退出。

旧版 JWT 没有 `sid`。滚动发布时先完成迁移、让所有新登录和刷新产出带 `sid` 的 JWT，再启用强制 `sid` 校验；在不超过旧 Access Token 的 15 分钟过渡窗口内，可让无 `sid` 的旧 JWT 仅按原账号版本规则通过，到窗口结束后拒绝。窗口起止由发布配置或数据库版本记录固定，不依赖单台进程启动时间；多实例须一致。若无法可靠实施过渡，则明确安排全员重新登录，不能暗中把无 `sid` 的 JWT 永久放行。旧 Refresh Cookie 由回填的父会话关联，成功刷新后签发带 `sid` 的 JWT。

### 刷新与退出

`FindRefreshSession` 同时读取父会话 ID；`Rotate` 保持账号行先锁、旧刷新行单次消费和后继插入同事务，并再次核对父会话属于该账号、未撤销且未到期。后继刷新行复用同一父会话 ID；刷新不增加设备数量。`Logout` 通过当前 Cookie 对应刷新行撤销其父会话及可用刷新行，不影响其他设备。重复退出保持幂等。改密、禁用和权限敏感变更在原有事务中撤销该账号全部父会话和刷新行，并提高账号版本；失败整体回滚。

现有“刷新已提交但响应丢失”问题保持严格单次消费；本方案不能把旧 Token 重放变为合法恢复。若以后采用[限时同操作幂等恢复方案](2026-10-25-refresh-response-loss-protocol.md)，后继刷新必须复用相同父会话 ID，且已被挤出的父会话不得通过恢复协议复活。

## HTTP 与前端行为

- `POST /api/v1/auth/login` 请求/成功响应保持兼容；父会话 ID 不暴露给前端。可增加安全的响应字段提示本次挤出了旧会话，但首版不必增加。`POST /auth/refresh` 与 `/auth/logout` 路径保持不变。
- 受保护 API 对被挤出的会话返回稳定 401 码 `SESSION_EVICTED`，对正常过期/无效仍返回现有 `UNAUTHORIZED`；刷新端对被挤出 Cookie 返回 401 `SESSION_EVICTED` 并清 Cookie，对其他失效返回 `INVALID_SESSION`。服务故障继续 500 `AUTH_SERVICE_UNAVAILABLE`，保留 Cookie。不要把数据库错误或账号级变更误标为挤出。
- `web/src/shared/api/httpClient.ts`：遇 `SESSION_EVICTED` 直接清内存令牌和私有查询缓存，不尝试刷新；普通 Access Token 过期仍仅共享一次刷新并重放原请求。原请求迟到、换账号和同页并发仍遵守 `sessionGeneration` 边界。
- `web/src/app/providers/AuthProvider.tsx`：保存失效原因，展示“账号已在其他设备登录，当前设备已退出”；普通到期沿用原提示。登录页不需要设备选择。若以后增加会话列表和手动踢设备，应作为独立增量，届时再补服务端本人范围鉴权、分页和脱敏展示；本次不扩展入口。

## 修改位置与顺序

| 顺序 | 位置 | 改动 |
| --- | --- | --- |
| 1 | `server/migrations/028_login_device_sessions.sql` | 父会话表、刷新行关联、索引、历史回填与超额处理；只追加迁移 |
| 2 | `server/internal/auth/service.go` | 逻辑会话类型、`sid` claim、登录/刷新/退出/Access 校验的业务编排、稳定错误分类 |
| 3 | `server/internal/store/postgres.go` | 账号锁下原子限额、父会话校验/活动时间、刷新轮换、按设备撤销及账号全撤销 |
| 4 | `server/internal/httpapi/router.go` | 401 错误码、Cookie 清理和客户端可见提示；登录/刷新成功结构尽量不变 |
| 5 | `web/src/shared/api/httpClient.ts`、`web/src/app/providers/AuthProvider.tsx` | 被挤出时清会话与私有缓存、明确提示；其他 401/5xx 路径不退化 |
| 6 | `server/internal/auth/*test.go`、`server/internal/httpapi/router_test.go`、`web/src/shared/api/sessionRenewal.test.ts`、`web/src/app/providers/AuthProvider.test.tsx` | 回归与错误契约；必要时补隔离浏览器场景 |
| 7 | 技术设计、部署说明与项目状态 | 同步上限、滚动发布/迁移和目标环境加载状态 |

现有 `server/internal/auth/service.go`、`server/internal/store/postgres.go`、`server/internal/httpapi/router.go` 在 10-25 工作区已有其他未提交改动；实施时先复核 diff，按局部补丁合并，不覆盖他人工作。

## 验收门槛

1. 一次性 PostgreSQL 从完整历史迁移升到 028：0/1/2/3/4 个旧有效会话、过期与已撤销行、相同活动时间、回填和重复执行边界；失败回滚不踢旧设备。
2. PostgreSQL 并发：两个同时的第四次登录始终不超过 3 个父会话；新设备成功、最久未活动设备 Access 和 Refresh 均拒绝，其余两台继续可用；轮换与挤出、改密/禁用与登录并发不复活会话。
3. HTTP：成功和失败路径断言状态、稳定错误码、Cookie；越权跨账号 `sid` 拒绝；数据库故障是 500，不能伪装 401。审计检查不含 Token、密码或完整 IP。
4. 前端聚焦测试与真实浏览器：三台登录、第四台挤出、旧设备下一次请求提示并清私有缓存；正常过期续期、临时 5xx 重试、同浏览器多标签和账号切换不回归。
5. 后端改动后运行 `gofmt`、`go vet ./...`、`go test ./...`；前端运行前核对 Node/pnpm/锁文件，随后 `pnpm lint`、`pnpm test`、`pnpm build`。真实浏览器使用一次性账号与数据库；目标环境需加载迁移和新 API 后重放原场景。未执行的检查不得记为通过。

## 来源与决策依据

- [Keycloak User Session Limits](https://github.com/keycloak/keycloak/blob/main/docs/documentation/server_admin/topics/authentication/flows.adoc)：并发会话上限可配合拒绝新登录或终止旧会话。
- [SuperTokens Session Invalidation](https://supertokens.com/docs/post-authentication/session-management/session-invalidation)：支持撤销指定和全部会话；仅撤销服务端会话不保证已签发 Access Token 立即失效。
- [SuperTokens Access Token Blacklisting](https://supertokens.com/docs/post-authentication/session-management/advanced-workflows/access-token-blacklisting)：立即失效需在受保护请求中做权威服务端检查。
- [Auth.js 数据库会话实现](https://github.com/nextauthjs/next-auth/blob/main/packages/core/src/lib/actions/session.ts)：数据库会话由服务端查询、更新和失效处理。

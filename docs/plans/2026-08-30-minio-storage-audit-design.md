# MinIO 文件存储与通用审计设计

## 1. 目标与范围

阶段 1 统一使用 MinIO 作为唯一文件存储，不提供本地目录适配器。开发、测试和演示环境由 Docker Compose 启动单节点 MinIO；生产环境通过相同配置连接独立或分布式 MinIO，业务代码不感知部署拓扑。

本轮交付：

- 私有 Bucket 初始化与 MinIO 就绪检查。
- `ObjectStorage` 接口及 MinIO 实现。
- 文件元数据、上传状态、SHA-256、大小和 MIME 信息。
- 安全对象 key、失败补偿、缺失文件识别和可重试恢复。
- 通用追加式审计日志及关键阶段 1 操作接入。
- 权限敏感配置操作的二次确认。

本轮不实现 Excel 解析、模板管理、Word/Excel 生成、通报归档和公开文件上传页面；这些能力在后续阶段复用本轮存储接口。

## 2. 已确认决策

1. 所有环境只使用 MinIO，不实现本地存储分支。
2. Bucket 永远私有，不启用匿名读写，不把 MinIO object key 返回给前端。
3. 文件上传、读取和下载必须经过 API 身份、动作权限、资源状态和数据范围校验。
4. 数据库保存文件元数据，MinIO 保存对象内容；不在 PostgreSQL 保存大文件二进制。
5. 数据库与 MinIO 无法形成原子事务，使用显式文件状态机和补偿实现最终一致性。
6. 审计日志只追加，普通业务接口不提供修改或删除能力。
7. 阶段 1 只提供内部文件服务和集成测试，不新增无业务归属的通用上传/下载页面。

## 3. 部署架构

```text
Browser → Nginx → Go API → PostgreSQL
                         ↘ MinIO（私有 Bucket）
```

Compose 新增：

- `minio`：对象存储服务，使用独立持久化卷。
- `minio-init`：等待 MinIO 健康后创建 Bucket、启用版本控制、创建应用账号并应用最小权限策略。
- `api`：在 PostgreSQL 迁移和 `minio-init` 成功后启动。

开发环境可以将 MinIO Console 绑定到 `127.0.0.1` 便于诊断；生产环境不暴露 MinIO API 和 Console，只允许 API 服务访问。生产环境必须通过 TLS 连接 MinIO。

## 4. 数据模型

新增只追加迁移 `server/migrations/004_storage_audit.sql`。

### 4.1 files

核心字段：

- `id uuid primary key`
- `storage_provider varchar(16)`，固定为 `MINIO`
- `bucket varchar(96)`
- `object_key varchar(512)`，全局唯一
- `original_name varchar(255)`，仅展示，不参与路径生成
- `declared_content_type varchar(128)`
- `detected_content_type varchar(128)`
- `size_bytes bigint`
- `sha256 char(64)`
- `status varchar(16)`：`PENDING`、`AVAILABLE`、`FAILED`、`MISSING`
- `retention_type varchar(32)`：`TEMPORARY`、`IMPORT_SOURCE`、`TEMPLATE`、`REPORT_DRAFT`、`REPORT_FINAL`、`ARCHIVE`
- `created_by uuid references accounts(id)`
- `failure_code varchar(64)`、`failure_message varchar(255)`
- `created_at`、`updated_at`

阶段 2、3 的 `import_batches`、`templates`、`report_files` 通过外键引用 `files.id`。不建立无外键约束的通用 `resource_type/resource_id` 文件关联表。

### 4.2 audit_logs

字段采用技术方案既定结构：`id`、`actor_id`、`action`、`resource_type`、`resource_id`、`before_json`、`after_json`、`reason`、`request_id`、`ip`、`user_agent`、`created_at`。

约束：

- 索引 `(resource_type, resource_id, created_at desc)` 和 `(actor_id, created_at desc)`。
- 不创建业务层 Update/Delete 方法。
- 数据库触发器拒绝普通 `UPDATE` 和 `DELETE`，后续保留期清理使用独立运维角色或分区策略。
- JSON 不保存密码、Token、Cookie、MinIO 凭证、完整身份证号或上传文件原文。

## 5. 后端组件

### 5.1 storage 模块

`server/internal/storage` 提供：

- `ObjectStorage.Put`：写入指定 Bucket/object key。
- `ObjectStorage.Open`：返回只读流和对象元信息。
- `ObjectStorage.Stat`：验证对象存在、大小及 ETag。
- `ObjectStorage.Remove`：仅用于失败补偿，不直接暴露给 HTTP API。
- `Service.Store`：创建元数据、上传、校验、转为 `AVAILABLE`。
- `Service.Open`：只允许读取 `AVAILABLE` 文件。
- `Service.Reconcile`：检查长时间 `PENDING`、对象缺失和失败补偿。

对象 key 由服务端生成，例如：

```text
system/2026/08/{fileId}
imports/{semesterId}/{weekId}/{batchId}/original/{fileId}
reports/{semesterId}/{weekId}/{reportId}/v{version}/{fileId}
```

原始文件名永远不进入 object key。

### 5.2 audit 模块

`server/internal/audit` 定义：

- `Metadata`：操作者、请求 ID、IP、User-Agent、原因。
- `Entry`：动作、资源、前后值。
- `Recorder`：追加审计记录。

HTTP 中间件生成或接收可信请求 ID，提取经过可信代理配置处理的客户端 IP，并限制 User-Agent 长度。Handler 将审计元数据显式传给 Application Service；关键业务变更与审计写入在同一 PostgreSQL 事务中完成。

首批审计动作：

- 注册审核、用户创建/编辑/启停、密码重置。
- 部门和组织角色分配、部门检查类型映射。
- 用户补充允许/拒绝规则。
- 学年、学期、周次创建与修改。
- 基础数据创建、修改、启停。
- 字典类型、字典项创建、修改、启停。
- 文件上传成功、失败、读取和完整性异常。

## 6. 文件一致性与失败恢复

上传流程：

1. 校验文件名长度、声明 MIME、大小上限和允许用途。
2. 创建 `PENDING` 文件元数据及服务端 object key。
3. 流式写入 MinIO，同时计算 SHA-256。
4. 使用 `StatObject` 校验对象存在及大小。
5. 在数据库事务中更新为 `AVAILABLE` 并写入审计。
6. 任一步失败则记录稳定错误码，将元数据置为 `FAILED`，并尽力删除不完整对象。

读取流程先从数据库取得元数据和业务关联，由业务服务完成权威鉴权，再调用 storage 服务。对象不存在时将状态标记为 `MISSING`、记录审计并返回稳定错误，不把 MinIO 内部错误直接暴露给前端。

启动时不扫描整个 Bucket。后台恢复任务只处理超过阈值的 `PENDING`/`FAILED` 文件，避免大规模对象枚举。

## 7. 安全基线

- MinIO 根账号仅用于初始化；API 使用独立应用账号和最小 Bucket 权限。
- 凭证通过环境变量或 Secret 注入，禁止进入仓库和日志。
- 生产使用 TLS，并校验证书；不允许 `secure=false`。
- Bucket 禁止匿名访问和前端直连；阶段 1 不使用预签名 URL。
- object key、Bucket 和底层错误不出现在公共 API 响应。
- 限制文件大小、原始名称长度和审计字段长度。
- MIME 检测以文件内容为准，扩展名和声明 MIME 只作为辅助。
- Office 宏、压缩炸弹、工作表数量和行数等深度校验留给阶段 2 导入服务，但 storage 服务保留检测结果字段。

## 8. 配置与依赖

新增环境变量：

- `MINIO_ENDPOINT`
- `MINIO_ACCESS_KEY`
- `MINIO_SECRET_KEY`
- `MINIO_BUCKET`
- `MINIO_USE_TLS`
- `MINIO_REGION`
- `MINIO_MAX_FILE_SIZE_BYTES`
- `MINIO_CONNECT_TIMEOUT`
- `MINIO_OPERATION_TIMEOUT`
- `TRUSTED_PROXY_CIDRS`

新增 Go 直接依赖：`github.com/minio/minio-go/v7`。Compose 固定 MinIO Server 和 `mc` 镜像版本，不使用浮动 `latest`。

## 9. 测试与验收

- storage 单元测试：object key、大小、MIME、SHA-256、状态转换和失败补偿。
- audit 单元测试：脱敏、字段限制、动作枚举和前后值。
- PostgreSQL 集成测试：迁移、追加写、索引、事务回滚。
- MinIO 集成测试：Bucket 初始化、Put/Stat/Open、对象缺失、错误凭证和重复 key。
- HTTP 测试：未登录、越权、MinIO 不可用、安全错误响应、请求 ID。
- 浏览器回归：权限敏感开关二次确认和明确失败提示。
- 恢复演练：PostgreSQL 元数据和 MinIO Bucket 联合备份后，在空环境恢复并抽检 SHA-256。

阶段 1 完成条件：MinIO 是所有环境唯一存储；文件元数据与对象可恢复；关键管理操作全部审计；普通 API 无法篡改审计；完整检查和真实浏览器验收通过。

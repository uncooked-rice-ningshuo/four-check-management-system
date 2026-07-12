# 计算机学院日常管理系统

当前交付范围：React/Ant Design 前端、Go API、PostgreSQL 和 Docker Compose 本地环境，以及注册、登录、刷新会话、当前用户和退出登录。

## 本地运行

```bash
cp .env.example .env
docker compose up --build
```

打开 <http://localhost:8088>。首次启动会创建数据库表。

> 当前阶段注册成功后账号直接为 `ACTIVE`。注册审核、部门、角色、RBAC 和业务权限均未实现，等待第 0 阶段规则确认后开发。

## 测试

```bash
docker run --rm -v "$PWD/server:/app" -w /app golang:1.24-alpine go test ./...
cd web && pnpm test && pnpm build
docker compose config
```

## API

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /health/live`

Access Token 只由前端保存在内存；Refresh Token 使用 HttpOnly、SameSite=Lax Cookie，并在刷新时轮换。

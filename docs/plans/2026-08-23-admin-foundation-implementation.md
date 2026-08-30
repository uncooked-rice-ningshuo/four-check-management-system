# Admin Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver the super-admin account, registration review, organization/inspection configuration, and manually configured academic calendar as a secured full-stack vertical slice.

**Architecture:** Extend authentication around a general login name and explicit account kind, then add small organization and academic application services backed by PostgreSQL repositories. Expose super-admin-only REST endpoints and consume them through TanStack Query in a shared Ant Design administration shell.

**Tech Stack:** Go 1.24, Gin, pgx/PostgreSQL, React 19, TypeScript, Ant Design, TanStack Query, Vitest.

---

### Task 1: Append the foundation migration and seed catalog

**Files:**
- Create: `server/migrations/002_admin_foundation.sql`
- Modify: `compose.yaml`
- Modify: `.env.example`

1. Add migration assertions for nullable student number, unique login name, account kind, super-admin flag, first-login password flag and review fields.
2. Add organization, inspection, mapping, user-role, academic-year, semester and week tables with constraints and indexes.
3. Seed four departments, four organization roles, seven inspection types and default department mappings idempotently.
4. Mount all migrations in Compose and add bootstrap admin environment variables.

### Task 2: Implement account lifecycle with TDD

**Files:**
- Modify: `server/internal/auth/service_test.go`
- Modify: `server/internal/auth/service.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router_test.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/cmd/api/main.go`

1. Write failing tests for pending registration, login by login name, idempotent admin bootstrap, first password change and approve/reject transitions.
2. Run focused tests and confirm failures are caused by missing behavior.
3. Implement the minimal domain contracts and PostgreSQL queries.
4. Add authenticated password-change and super-admin registration-review endpoints.
5. Run focused and complete backend tests.

### Task 3: Implement organization and inspection configuration with TDD

**Files:**
- Create: `server/internal/organization/service.go`
- Create: `server/internal/organization/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for listing catalogs, changing enabled state, assigning a user department/role and updating department inspection permissions.
2. Verify failures, implement domain validation and repositories, then expose super-admin-only endpoints.
3. Cover unauthenticated and non-super-admin rejection paths.
4. Run focused and complete backend tests.

### Task 4: Implement academic calendar with TDD

**Files:**
- Create: `server/internal/academic/service.go`
- Create: `server/internal/academic/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for creating years, semesters and weeks, rejecting invalid/overlapping ranges, and resolving no/current week.
2. Verify failures and implement validation, persistence and super-admin endpoints.
3. Run focused and complete backend tests.

### Task 5: Update authentication UI with TDD

**Files:**
- Modify: `web/src/entities/user/model/user.ts`
- Modify: `web/src/shared/api/authApi.ts`
- Modify: `web/src/app/providers/authContext.ts`
- Modify: `web/src/app/providers/AuthProvider.tsx`
- Modify: `web/src/pages/auth/LoginPage.tsx`
- Modify: `web/src/pages/auth/RegisterPage.tsx`
- Create: `web/src/pages/auth/ChangePasswordPage.tsx`
- Modify: `web/src/app/App.tsx`

1. Write failing UI/API tests for account-name login, pending registration result and first-password routing.
2. Implement the minimal UI and session-state changes.
3. Run focused frontend tests.

### Task 6: Build the super-admin management UI with TDD

**Files:**
- Create: `web/src/shared/layouts/AdminLayout.tsx`
- Create: `web/src/shared/api/adminApi.ts`
- Create: `web/src/entities/organization/model/organization.ts`
- Create: `web/src/entities/academic/model/academic.ts`
- Create: `web/src/pages/admin/RegistrationReviewPage.tsx`
- Create: `web/src/pages/admin/UserManagementPage.tsx`
- Create: `web/src/pages/admin/OrganizationConfigPage.tsx`
- Create: `web/src/pages/admin/AcademicCalendarPage.tsx`
- Create: `web/src/pages/admin/AdminPages.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/shared/styles/global.css`

1. Write failing tests for super-admin navigation, pending review actions, empty academic calendar guidance and configuration submission.
2. Implement a restrained school-blue administration shell with accessible tables, forms, status tags and confirmations.
3. Connect TanStack Query hooks to the admin API client and invalidate affected queries after mutations.
4. Run focused and complete frontend checks.

### Task 7: Align all documentation to seven inspection types

**Files:**
- Modify: `README.md`
- Modify: `PRD-计算机学院日常管理系统.md`
- Modify: `原型页面模块功能梳理.md`
- Modify: `计算机学院日常管理系统设计 内容梳理.md`
- Modify: `docs/plans/2026-07-11-four-check-management-system-technical-design.md`

1. Replace outdated six-type statements with seven-type wording where they describe system scope.
2. Add public dormitory hygiene under the dormitory department and document actual enrolled class size as the attendance denominator.
3. Update README setup, bootstrap admin and current delivered scope.
4. Search for stale six-type and deferred-rule wording.

### Task 8: Full verification

1. Format modified Go files with `gofmt`.
2. Run `go test ./...` and `go vet ./...`.
3. Run `pnpm lint`, `pnpm test` and `pnpm build`.
4. Run `docker compose config` and validate migration order on a disposable PostgreSQL database.
5. Review `git diff` for secrets, unrelated changes and generated artifacts.

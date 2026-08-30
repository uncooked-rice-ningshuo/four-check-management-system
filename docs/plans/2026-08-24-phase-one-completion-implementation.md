# Phase One Completion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete phase-one effective permissions, user lifecycle, master data, dictionary management, and standardize the React UI on Ant Design components.

**Architecture:** Add append-only PostgreSQL migrations and focused application services for permissions, master data, and dictionaries. Keep HTTP handlers as adapters, put validation and effective-permission compilation in services, keep high-risk identity and authorization management behind the super-admin guard, and protect master-data/dictionary operations with their specific effective-permission actions. Frontend pages use TanStack Query, Ant Design forms and feedback, and ProTable for list/search behavior.

**Tech Stack:** Go 1.24, Gin, pgx/PostgreSQL, React 19, TypeScript, Vite, Ant Design, Ant Design ProTable, TanStack Query, Vitest.

---

### Task 1: Add phase-one schema

**Files:**
- Create: `server/migrations/003_phase_one_completion.sql`
- Modify: `compose.yaml`
- Test: `server/internal/store/postgres_test.go`

1. Write migration assertions for system roles, scoped user permission rules, faculties, majors, staff, classes, dormitories, classrooms, courses, students, dictionary types/items, and change-history records.
2. Run the disposable PostgreSQL migration test and confirm the new objects are missing.
3. Add the append-only migration with foreign keys, checks, partial unique indexes, and searchable indexes.
4. Mount migration `003` after `002` and rerun the migration test.

### Task 2: Implement effective permissions with TDD

**Files:**
- Create: `server/internal/permission/service.go`
- Create: `server/internal/permission/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for super-admin access, member department access, minister review access, office report actions, scoped supplemental allow, and explicit deny precedence.
2. Run focused tests and verify the failures describe missing compilation behavior.
3. Implement action constants, permission source explanations, scoped rule matching, and repository persistence.
4. Add endpoints for effective permission details and user permission rule management.
5. Verify unauthenticated and non-super-admin management requests are rejected.

### Task 3: Complete user lifecycle with TDD

**Files:**
- Modify: `server/internal/auth/service.go`
- Modify: `server/internal/auth/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for admin-created users, profile edits, enable/disable transitions, reset-password session invalidation, and super-admin protection.
2. Implement service validation and transactional repository updates.
3. Add create, update, status, and password-reset endpoints with stable errors.
4. Run focused and complete backend tests.

### Task 4: Implement master data with TDD

**Files:**
- Create: `server/internal/masterdata/service.go`
- Create: `server/internal/masterdata/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for entity-kind validation, required fields, immutable codes, relation validation, list filtering, enable/disable, and change history.
2. Implement typed master-data records for faculties, majors, staff, classes, dormitories, classrooms, courses, and students.
3. Add paginated list/create/update/status/history endpoints.
4. Run focused and complete backend tests.

### Task 5: Implement dictionary management with TDD

**Files:**
- Create: `server/internal/dictionary/service.go`
- Create: `server/internal/dictionary/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Write failing tests for normalized codes, duplicate values, ordering, enabling/disabling, and system-type protection.
2. Implement dictionary type/item services and repositories.
3. Add super-admin list/create/update endpoints.
4. Run focused and complete backend tests.

### Task 6: Build management pages with Ant Design

**Files:**
- Create: `web/src/entities/permission/model/permission.ts`
- Create: `web/src/entities/masterdata/model/masterData.ts`
- Create: `web/src/entities/dictionary/model/dictionary.ts`
- Create: `web/src/pages/admin/PermissionDetailPage.tsx`
- Create: `web/src/pages/admin/MasterDataPage.tsx`
- Create: `web/src/pages/admin/DictionaryPage.tsx`
- Modify: `web/src/pages/admin/UserManagementPage.tsx`
- Modify: `web/src/shared/api/adminApi.ts`
- Modify: `web/src/shared/layouts/AdminLayout.tsx`
- Modify: `web/src/app/App.tsx`
- Test: `web/src/pages/admin/AdminPages.test.tsx`

1. Write failing tests for navigation, user create/edit/status/reset actions, permission-source display, master-data CRUD forms, and dictionary forms.
2. Implement ProTable lists, Ant Design Form/Select/DatePicker/Switch/Modal/Drawer/Popconfirm interactions, query invalidation, and actionable failures.
3. Keep destructive or session-invalidating operations behind confirmation dialogs.
4. Run focused frontend tests.

### Task 7: Audit and standardize UI components

**Files:**
- Modify: `web/src/pages/auth/RegisterPage.tsx`
- Modify: relevant `web/src/**/*.tsx`
- Modify: `web/src/shared/styles/global.css`

1. Search all TSX for native form/table/dialog controls, clickable non-buttons, direct navigation handlers, inaccessible icon buttons, and unconfirmed destructive actions.
2. Add a regression test asserting the registration agreement uses Ant Design Checkbox binding.
3. Replace native controls with Ant Design equivalents and correct Link/Button semantics.
4. Re-run the static audit and frontend tests.

### Task 8: Documentation and full verification

**Files:**
- Modify: `README.md`
- Modify: `PRD-计算机学院日常管理系统.md`
- Modify: `原型页面模块功能梳理.md`
- Modify: `docs/plans/2026-07-11-four-check-management-system-technical-design.md`

1. Document delivered permission, user, master-data, and dictionary behavior and the phase-two Excel import boundary.
2. Run `gofmt` on modified Go files.
3. Run `go test ./...` and `go vet ./...`.
4. Run `pnpm lint`, `pnpm test`, and `pnpm build`.
5. Run `docker compose config` and migrations against disposable PostgreSQL.
6. Review `git diff --check`, secrets, generated artifacts, and unrelated changes.

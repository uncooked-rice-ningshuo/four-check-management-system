# MinIO Storage and Audit Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete phase-one file storage and append-only audit capabilities with MinIO as the only storage provider in every environment.

**Architecture:** A storage application service persists file metadata in PostgreSQL and object content in a private MinIO bucket through a narrow `ObjectStorage` interface. Critical phase-one mutations write immutable audit entries in the same PostgreSQL transaction, while HTTP middleware supplies request metadata without logging secrets or sensitive bodies.

**Tech Stack:** Go 1.24+, Gin, pgx/PostgreSQL 16, MinIO, minio-go v7, Docker Compose, React 19, Ant Design, Vitest.

---

## Scope guard

Included: MinIO deployment, private Bucket initialization, storage interface, file metadata state machine, integrity checks, recovery hooks, immutable audit table/service, phase-one audit wiring, permission-sensitive confirmations, documentation and full verification.

Excluded: Excel parsing, Word/Excel generation, template management, report finalization, presigned URLs, anonymous files, local filesystem fallback and MinIO cluster administration.

### Task 1: Add the storage and audit migration with TDD

**Files:**
- Create: `server/migrations/004_storage_audit.sql`
- Modify: `server/internal/store/postgres_test.go`
- Modify: `compose.yaml`

1. Add failing migration assertions for `files`, `audit_logs`, required checks, unique object key and audit indexes.
2. Run `go test ./internal/store -run 'TestStorageAuditMigration' -v` and confirm failure because migration `004` is absent.
3. Add `files` and `audit_logs` with the fields and constraints from the design.
4. Add the append-only audit trigger and stable indexes.
5. Run migrations `001` through `004` against a disposable PostgreSQL container.
6. Run migration `004` again and verify idempotence.

### Task 2: Add audit domain contracts and redaction tests

**Files:**
- Create: `server/internal/audit/model.go`
- Create: `server/internal/audit/service.go`
- Create: `server/internal/audit/service_test.go`

1. Write failing tests for required action/resource fields, request metadata length limits and sensitive-key redaction.
2. Run `go test ./internal/audit -v` and confirm the missing behavior.
3. Implement `Metadata`, `Entry`, action constants and recursive JSON redaction for password/token/cookie/secret keys.
4. Re-run the focused tests.

### Task 3: Persist audit records atomically

**Files:**
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/store/postgres_test.go`
- Modify: relevant repository interfaces under `server/internal/{auth,organization,permission,academic,masterdata,dictionary}/`

1. Add a failing PostgreSQL integration test proving a business mutation and its audit entry commit or roll back together.
2. Add an internal transaction helper that accepts explicit `audit.Metadata` and `audit.Entry`.
3. Keep action selection in Application Services and persistence inside the same pgx transaction as the mutation.
4. Do not add Update/Delete audit repository methods.
5. Verify audit trigger rejection for direct update and delete.

### Task 4: Capture trusted HTTP request metadata

**Files:**
- Create: `server/internal/httpapi/requestmeta.go`
- Create: `server/internal/httpapi/requestmeta_test.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/cmd/api/main.go`

1. Write failing tests for generated request ID, accepted valid incoming request ID, User-Agent truncation and trusted-proxy IP handling.
2. Implement middleware and a helper that builds `audit.Metadata` from the authenticated request.
3. Return `X-Request-ID` in every response.
4. Ensure request bodies, passwords, cookies and access tokens are never copied into audit metadata.

### Task 5: Wire audit into phase-one mutations

**Files:**
- Modify: `server/internal/auth/service.go`
- Modify: `server/internal/organization/service.go`
- Modify: `server/internal/permission/service.go`
- Modify: `server/internal/academic/service.go`
- Modify: `server/internal/masterdata/service.go`
- Modify: `server/internal/dictionary/service.go`
- Modify: corresponding `*_test.go` files
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Add one failing audit assertion for each mutation family: user lifecycle, permission, organization, calendar, master data and dictionary.
2. Pass actor and request metadata explicitly from Handler to Application Service.
3. Record before/after values and reason where the action requires one.
4. Verify failed and unauthorized operations do not create success audit entries.
5. Verify password reset and authentication audit entries contain no password or token values.

### Task 6: Add MinIO deployment and configuration

**Files:**
- Modify: `compose.yaml`
- Modify: `.env.example`
- Modify: `README.md`
- Modify: `server/go.mod`
- Modify: `server/go.sum`

1. Pin MinIO Server and `mc` image versions.
2. Add `minio` and `minio-init` services, private network access, health checks and a persistent `minio_data` volume.
3. Create a private versioned Bucket and a least-privilege application user/policy.
4. Make `api` depend on successful Bucket initialization.
5. Add `github.com/minio/minio-go/v7` as a direct dependency.
6. Run `docker compose config` and verify no credential value is committed.

### Task 7: Implement the ObjectStorage MinIO adapter with TDD

**Files:**
- Create: `server/internal/storage/object_storage.go`
- Create: `server/internal/storage/minio.go`
- Create: `server/internal/storage/minio_test.go`

1. Write failing contract tests for Put, Open, Stat and compensation-only Remove.
2. Implement the MinIO client with bounded connection/operation contexts.
3. Map MinIO errors to stable domain errors such as unavailable, object missing and integrity mismatch.
4. Run tests against a disposable MinIO container with a unique Bucket.
5. Verify wrong credentials and unavailable endpoint fail safely without leaking endpoint details.

### Task 8: Implement file metadata and state transitions with TDD

**Files:**
- Create: `server/internal/storage/service.go`
- Create: `server/internal/storage/service_test.go`
- Modify: `server/internal/store/postgres.go`
- Modify: `server/internal/store/postgres_test.go`

1. Write failing tests for server-generated keys, file-size limit, SHA-256, MIME detection, `PENDING → AVAILABLE`, upload failure and missing object handling.
2. Implement metadata repository methods and the storage application service.
3. Stream content through SHA-256 calculation; never load the entire file into memory.
4. On failure, persist a stable failure code and perform best-effort object cleanup.
5. Implement targeted reconciliation for stale `PENDING` and `FAILED` rows.
6. Verify `Open` refuses non-`AVAILABLE` files.

### Task 9: Integrate readiness and safe internal usage

**Files:**
- Modify: `server/cmd/api/main.go`
- Modify: `server/internal/httpapi/router.go`
- Modify: `server/internal/httpapi/router_test.go`

1. Add failing readiness tests for PostgreSQL healthy/MinIO unavailable and both dependencies healthy.
2. Construct the MinIO adapter from validated environment configuration.
3. Make readiness fail when the private Bucket cannot be accessed; keep liveness independent of MinIO.
4. Do not add a generic unaffiliated upload/download endpoint in phase 1.
5. Expose storage service to later import/template/report modules through application wiring only.

### Task 10: Confirm permission-sensitive UI mutations

**Files:**
- Modify: `web/src/pages/admin/OrganizationConfigPage.tsx`
- Modify: `web/src/pages/admin/DictionaryPage.tsx`
- Modify: `web/src/pages/admin/PhaseOnePages.test.tsx`

1. Write failing interaction tests that require confirmation before disabling departments, inspection types and dictionary values or changing department permission scopes.
2. Replace immediate high-impact Switch mutations with confirmation-backed interactions while preserving keyboard access.
3. Show the affected object and impact in Chinese confirmation text.
4. Re-run focused tests and the static native-control audit.

### Task 11: Documentation, recovery exercise and full verification

**Files:**
- Modify: `README.md`
- Modify: `PRD-计算机学院日常管理系统.md`
- Modify: `docs/plans/2026-07-11-four-check-management-system-technical-design.md`
- Modify: `原型页面模块功能梳理.md`

1. Document MinIO as the only provider, required Secrets, private networking, backup and restore commands.
2. Document the audit action catalog and sensitive-field exclusions.
3. Back up a disposable PostgreSQL database and MinIO Bucket, restore both into an empty environment, and verify sampled SHA-256 values.
4. Run `gofmt` on every modified Go file.
5. Run `go test ./...`, `go vet ./...`, `pnpm lint`, `pnpm test` and `pnpm build`.
6. Run `docker compose config`, disposable PostgreSQL migrations and disposable MinIO integration tests.
7. Perform browser verification of phase-one account, permission, calendar, dictionary and confirmation flows.
8. Run `git diff --check`; review secrets, generated files, user reference materials and unrelated changes.
9. Request user authorization before staging, committing, pushing or creating a PR.

# 逐周应交范围与零记录回执实施计划

> **Implementation plan for Codex:** Execute the tasks in order in the existing `feat-0.1-2` checkout. Preserve the shared working tree; do not create a branch or worktree.

**Goal:** Let the student union office configure the exact department/type cells due for each academic week and let authorized staff submit an explicit zero-record receipt through the existing review workflow.

**Architecture:** Add an append-only migration for weekly obligation rows and an explicit submission kind. Keep report-scope configuration in the `reporting` service/repository; keep zero receipts in the existing `workflow` state machine and immutable submission revisions. Enforce authorization, scope, version, idempotency, and audit in the backend; expose configuration and submission actions through existing report and inspection pages.

**Tech Stack:** PostgreSQL migrations, Go/Gin application services and repositories, existing effective-permission rules, React/TypeScript, TanStack Query, Ant Design.

---

**Implementation status (2026-10-25):** Tasks 1–7 are implemented in the current checkout. The backend builds and passes `go vet`; frontend lint and production build pass. The current-week selector, including work-context failure feedback, is implemented. No tests were run, migration 027 was not executed, and no real database/API/browser environment has loaded or validated this code. Disposable PostgreSQL, HTTP authorization/transaction, and browser role flows remain pending.

---

## Confirmed rules

- Required units are configured independently for each semester/week, department or college-level scope, and report-participating inspection type; `can_enter` is not the obligation list.
- Configuration is maintained by the student union office minister or a designated office deputy/report administrator through a separate permission action and is audited.
- A zero-record receipt is an explicit submission kind, not an empty ordinary submission. It uses existing submission/review roles; explanation is optional.
- Ordinary record batches continue to require at least one record. Zero receipts become complete only after reviewer confirmation.
- If actual records are discovered after a confirmed zero receipt, allow a later actual-record batch and retain the original zero receipt as immutable history. The later batch does not replace or rewrite that receipt.
- The weekly report period selector defaults to the current week in the system work context, including a valid override, only when that week is within the viewer's available periods. With no visible current week, leave it unselected; never default to the first option. A manual selection is not overwritten by the default.

## Task 1: Add the separate obligation-management permission

**Files:**

- Modify `server/internal/permission/service.go`
- Modify `server/internal/httpapi/router.go`
- Modify `web/src/entities/permission/model/permission.ts`
- Modify `web/src/pages/admin/PermissionDetailPage.tsx`

1. Add `REPORT_OBLIGATION_MANAGE` to the action catalog and effective permissions.
2. Do not grant it through generic admin/super-admin defaults. Permit an explicit ALLOW only for a current student union office minister/deputy; revalidate the target’s primary department and office role on every effective-permission calculation and permission-rule create/update, following the force-generation eligibility pattern.
3. Map rejected targets to a stable role error and expose the action in the permission-detail UI with no class/grade/department scope controls; the action is global because it can configure any department or college-level cell.

## Task 2: Add the weekly obligation and submission-kind migration

**Files:**

- Create `server/migrations/027_weekly_reporting_obligations_and_zero_receipts.sql`
- Modify `server/internal/store/postgres.go` readiness only if the endpoint depends on the new table at startup

1. Create `weekly_report_obligations` with semester, week, nullable department, participating inspection type, version, create/update actor and timestamps, and soft-removal timestamp.
2. Enforce week/semester ownership, valid active report type, and department/type submission capability in the application transaction. Use a generated department-scope key or equivalent constraints so college-level and department rows have stable uniqueness. Add partial uniqueness for active rows and useful list indexes.
3. Add `submission_kind` to the submission and revision records, default existing rows to `RECORDS`, and enforce `RECORDS` → one or more record IDs versus `ZERO_RECORD` → zero record IDs. Add a composite reference so a revision’s kind cannot disagree with its parent. Preserve append-only revision protection.
4. Do not execute this migration against the business database. Validate it only against a disposable PostgreSQL when validation is permitted.

## Task 3: Implement audited obligation configuration API

**Files:**

- Modify `server/internal/reporting/model.go`
- Modify `server/internal/reporting/service.go`
- Modify `server/internal/store/reporting.go`
- Modify `server/internal/httpapi/router.go`
- Modify `server/internal/httpapi/reporting_handlers.go`
- Modify `web/src/entities/reporting/model/reporting.ts`
- Modify `web/src/shared/api/reportingApi.ts`
- Modify `web/src/features/reporting/model/queries.ts`

1. Add list/create/soft-remove operations scoped to an explicit semester/week. Creation rechecks `REPORT_OBLIGATION_MANAGE`, active week ownership, inspection-type report participation, department enabled state and `can_enter` capability where a department is selected. Removal rechecks scoped permission and row version; within an active week the list keeps obsolete type/department cells visible with `eligible=false`, so managers can clean up configuration after source eligibility changes.
2. Add a read-only options response for active, report-participating inspection types and eligible department/type choices; do not infer obligations from these options.
3. Record add/remove and before/after values through the audit transaction. Use optimistic version checks for removal and return a stable conflict error.
4. Add stable validation, forbidden, not-found and version-conflict error mappings. Do not let an unauthorized query look like an authorized empty result.

## Task 4: Make zero receipts explicit in the existing workflow

**Files:**

- Modify `server/internal/workflow/model.go`
- Modify `server/internal/store/workflow.go`
- Modify `server/internal/store/workflow_read.go`
- Modify `server/internal/httpapi/router.go`
- Modify `web/src/entities/inspection/model/departmentSubmission.ts`
- Modify `web/src/shared/api/departmentSubmissionApi.ts`

1. Add a closed submission-kind enum. Omitted kind remains `RECORDS` for backward compatibility. Validate that `ZERO_RECORD` has no record IDs and `RECORDS` has at least one.
2. Include kind in the idempotency fingerprint and persisted batch/revision response. A zero receipt must have an idempotency key just like an ordinary submission.
3. For a zero receipt, inside the submission transaction require a matching configured weekly obligation and a broad `INSPECTION_ENTER` grant for the full semester/week/department-or-college/type scope; class/grade-only grants cannot attest a whole cell. Recheck this scope grant on idempotent replay. A normal batch retains existing per-record authorization.
4. Keep zero receipt revisions immutable and empty. Review/confirm allows zero rows only for `ZERO_RECORD`; returned zero receipts can be resubmitted as zero with a new revision. Ordinary empty submissions remain invalid.
5. Preserve the no-record return/review path without querying or mutating inspection rows. Record every submit, resubmit, return, and confirmation in the existing audit stream.
6. Allow a new actual-record batch after a confirmed zero receipt. Keep the earlier receipt and its revision immutable and visible as historical submission; do not delete, overwrite, or convert it into the later record batch.

## Task 5: Surface required cells and receipt states in the summary

**Files:**

- Modify `server/internal/reporting/model.go`
- Modify `server/internal/store/reporting.go`
- Modify `server/internal/reporting/service.go`
- Modify `web/src/entities/reporting/model/reporting.ts`
- Modify `web/src/pages/report/WeeklyReportSummaryPage.tsx`

1. Include obligation cells in summary candidates even when no batch exists; mark them with `isRequired`.
2. Include zero-receipt counts by batch status. Keep confirmed record count at zero; do not mislabel the receipt as an ordinary empty record batch.
3. Display a required marker on tree children, explicit no-submission versus pending/returned/confirmed-zero states, and configured obligations in a scoped management section visible only to `REPORT_OBLIGATION_MANAGE` holders.
4. Keep summary totals as counts only; do not introduce statistical formulas or generation behavior.

## Task 6: Add the member zero-receipt submission interaction

**Files:**

- Modify `web/src/features/department-submission/ui/DepartmentSubmissionActions.tsx`
- Modify `web/src/shared/api/departmentSubmissionApi.ts`
- Modify `web/src/entities/inspection/model/departmentSubmission.ts`

1. Offer a zero-receipt action only when the page has an explicit complete period, department-or-college selection and inspection type; never infer a department from an empty filter.
2. Show a confirmation dialog describing the entire week/type scope and that the submission enters the existing review queue. Explanation is optional.
3. Reuse idempotency keys and invalidate batch, record and report-summary queries after success or retryable failure.
4. Display submission kind and zero-record state in batch list/detail, and preserve existing ordinary submit/resubmit behavior.

## Task 7: Synchronize requirements and project status

**Files:**

- Modify `PRD-计算机学院日常管理系统.md`
- Modify `docs/plans/2026-07-11-four-check-management-system-technical-design.md`
- Modify `docs/plans/2026-10-25-statistics-and-weekly-report-requirements.md`
- Modify `原型页面模块功能梳理.md`
- Modify `docs/project-status.md`

Record delivered model/migration, API, UI, initial configuration, verification, and environment status separately. Confirmed-zero followed by actual records uses an appended batch while preserving the original receipt. Do not claim report-generation readiness.

## Verification matrix

- Backend: `gofmt` changed Go files, `GOCACHE=/private/tmp/four-check-reporting-gocache go vet ./...`, `go build ./...`.
- Frontend: confirm Node/pnpm versions against `web/package.json`, then `pnpm lint` and `pnpm build`.
- Migration/API/security evidence ultimately needs disposable PostgreSQL and real HTTP coverage for ordinary submission, explicit zero, review/return/resubmit, forbidden scope, class-scoped grant denial, unauthorized configuration, idempotent replay and audit. Browser coverage needs office configuration and member submit/reviewer confirm. Do not use the business database or modify downloaded report files.
- Report each check that is not run as unverified; compilation alone does not prove the transaction, migration, authorization or browser paths.

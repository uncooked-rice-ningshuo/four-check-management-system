package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDepartmentWorkflowMigrationContract(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/016_department_submissions.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"CREATE TABLE department_submissions", "CREATE TABLE department_submission_revisions",
		"inspection_records_submission_scope_fkey", "uq_department_submissions_idempotency",
		"UNIQUE (submission_id, revision_no)",
	} {
		if !strings.Contains(string(sql), fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	appendOnlySQL, err := os.ReadFile("../../migrations/017_department_submission_revision_immutability.sql")
	if err != nil || !strings.Contains(string(appendOnlySQL), "BEFORE UPDATE OR DELETE ON department_submission_revisions") {
		t.Fatalf("append-only revision migration missing: %v", err)
	}
}

func TestDepartmentWorkflowMigration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL must point to a disposable migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, table := range []string{"department_submissions", "department_submission_revisions"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('public.' || $1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s missing: %v", table, err)
		}
	}
	var scopeFK, revisionUnique, idempotencyUnique, appendOnly bool
	err = pool.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM pg_constraint WHERE conname='inspection_records_submission_scope_fkey'),
		EXISTS (SELECT 1 FROM pg_constraint WHERE conname='department_submission_revisions_submission_id_revision_no_key'),
		EXISTS (SELECT 1 FROM pg_indexes WHERE indexname='uq_department_submissions_idempotency'),
		EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='department_submission_revisions_append_only' AND NOT tgisinternal)`).Scan(&scopeFK, &revisionUnique, &idempotencyUnique, &appendOnly)
	if err != nil || !scopeFK || !revisionUnique || !idempotencyUnique || !appendOnly {
		t.Fatalf("workflow constraints missing: scope=%v revision=%v idempotency=%v appendOnly=%v err=%v", scopeFK, revisionUnique, idempotencyUnique, appendOnly, err)
	}
	// Multiple batches in one department/week/type are intentional. There must
	// be no unique scope-only index that would block a second member submission.
	var scopeUnique bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename='department_submissions'
		AND indexdef ILIKE '%UNIQUE%' AND indexdef ~ '\(semester_id, week_id, department_id, inspection_type_id\)' )`).Scan(&scopeUnique)
	if err != nil || scopeUnique {
		t.Fatalf("submission scope must allow multiple batches: unique=%v err=%v", scopeUnique, err)
	}
}

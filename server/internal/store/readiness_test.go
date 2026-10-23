package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"four-check-management-system/server/internal/inspection"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReadyRequiresCurrentSchema(t *testing.T) {
	url := os.Getenv("READINESS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires empty disposable database")
	}
	ctx := context.Background()
	db, e := Open(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var exists bool
	if e = db.Pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&exists); e != nil || exists {
		t.Fatal("requires empty database", e)
	}
	if db.Ready(ctx) == nil {
		t.Fatal("empty database incorrectly reports ready")
	}
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("read migrations: %v (files=%d)", err, len(files))
	}
	for _, f := range files {
		if filepath.Base(f) == "016_department_submissions.sql" {
			if db.Ready(ctx) == nil {
				t.Fatal("schema missing department workflow reports ready")
			}
			_, readErr := db.GetInspection(ctx, "00000000-0000-4000-8000-000000000016", inspection.MutationAccess{Unrestricted: true})
			var pgErr *pgconn.PgError
			if !errors.As(readErr, &pgErr) || pgErr.Code != "42P01" {
				t.Fatalf("pre-016 generic inspection read should expose missing relation: %v", readErr)
			}
		}
		if filepath.Base(f) == "017_department_submission_revision_immutability.sql" && db.Ready(ctx) == nil {
			t.Fatal("schema missing immutable revisions reports ready")
		}
		if filepath.Base(f) == "015_file_cleanup_progress.sql" {
			if db.Ready(ctx) == nil {
				t.Fatal("schema missing cleanup migration reports ready")
			}
			if _, e := db.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,status,retention_type,failure_code) VALUES('00000000-0000-4000-8000-000000000015','isolated','isolated/migration','fixture','FAILED','TEMPORARY','STORAGE_UNAVAILABLE')`); e != nil {
				t.Fatal(e)
			}
		}
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	var preserved bool
	if e := db.Pool.QueryRow(ctx, `SELECT status='FAILED' AND failure_code='STORAGE_UNAVAILABLE' AND object_removed_at IS NULL FROM files WHERE id='00000000-0000-4000-8000-000000000015'`).Scan(&preserved); e != nil || !preserved {
		t.Fatal("cleanup migration changed existing failure metadata", e)
	}
	if e = db.Ready(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Pool.Exec(ctx, "DELETE FROM system_settings WHERE key='work_week_override'"); e != nil {
		t.Fatal(e)
	}
	if db.Ready(ctx) == nil {
		t.Fatal("missing singleton incorrectly reports ready")
	}
}

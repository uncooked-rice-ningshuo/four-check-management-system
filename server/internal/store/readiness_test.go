package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
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

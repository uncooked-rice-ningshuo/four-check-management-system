package store

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkContextPostgres(t *testing.T) {
	url := os.Getenv("WORK_CONTEXT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable database")
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
	files, _ := filepath.Glob("../../migrations/*.sql")
	for _, f := range files {
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	raw, e := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Pool.Exec(ctx, string(raw)); e != nil {
		t.Fatal(e)
	}
	service := academic.NewWorkContextService(db)
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	current, e := service.Get(ctx, now)
	if e != nil || current.Source != "CALENDAR" || current.Version != 1 {
		t.Fatal(current, e)
	}
	input := academic.WeekOverrideInput{WeekID: rf("04"), ExpiresAt: &future, Version: 1, Reason: "临时调整"}
	m := audit.Metadata{ActorID: rf("01")}
	changed, e := service.Set(ctx, true, input, now, m)
	if e != nil || changed.Source != "OVERRIDE" || changed.Version != 2 {
		t.Fatal(changed, e)
	}
	if _, e = service.Set(ctx, true, input, now, m); !errors.Is(e, academic.ErrWorkContextConflict) {
		t.Fatal("stale version", e)
	}
	expired, e := service.Get(ctx, future)
	if e != nil || expired.Source != "CALENDAR" {
		t.Fatal(expired, e)
	}
	if _, e = db.Pool.Exec(ctx, "UPDATE semesters SET enabled=false"); e != nil {
		t.Fatal(e)
	}
	invalid, e := service.Get(ctx, now)
	if e != nil || invalid.Source != "NONE" {
		t.Fatal("disabled parent", e)
	}
	if _, e = db.Pool.Exec(ctx, "UPDATE semesters SET enabled=true"); e != nil {
		t.Fatal(e)
	}
	cleared, e := service.Set(ctx, true, academic.WeekOverrideInput{Version: 2, Reason: "恢复"}, now, m)
	if e != nil || cleared.Source != "CALENDAR" || cleared.Version != 3 {
		t.Fatal(cleared, e)
	}
	// Force audit failure and prove the setting/version remain unchanged.
	if _, e = db.Pool.Exec(ctx, `CREATE FUNCTION fail_context_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test failure'; END $$; CREATE TRIGGER fail_context_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_context_audit();`); e != nil {
		t.Fatal(e)
	}
	input.Version = 3
	if _, e = service.Set(ctx, true, input, now, m); e == nil {
		t.Fatal("audit failure ignored")
	}
	after, e := service.Get(ctx, now)
	if e != nil || after.Version != 3 || after.Source != "CALENDAR" {
		t.Fatal("partial save", e)
	}
	boundary := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	after, e = service.Get(ctx, boundary)
	if e != nil || after.Source != "NONE" {
		t.Fatal("Shanghai date boundary", e)
	}
}

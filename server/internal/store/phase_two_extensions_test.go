package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPhaseTwoExtensionMigration(t *testing.T) {
	content, err := os.ReadFile("../../migrations/012_phase_two_inspection_extensions.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"morning_exercise_details", "late_return_details", "dorm_hygiene_details", "classroom_hygiene_details", "public_dormitory_hygiene_details", "classroom_hygiene_areas", "public_dormitory_hygiene_areas", "inspection_responsibility_classes", "inspection_responsibility_students", "inspection_responsibility_dormitories"} {
		if !strings.Contains(string(content), "CREATE TABLE "+table) {
			t.Errorf("missing %s", table)
		}
	}
	if strings.Contains(strings.ToUpper(string(content)), "TRUNCATE ") {
		t.Fatal("migration must preserve records")
	}
}
func TestPhaseTwoExtensionConstraints(t *testing.T) {
	url := os.Getenv("EXTENSION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("EXTENSION_TEST_DATABASE_URL must name an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	var existing bool
	if err = db.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing {
		t.Fatal("requires an empty disposable database; refusing to modify existing schema")
	}
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) >= "012_phase_two_inspection_extensions.sql" {
			continue
		}
		contents, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, string(contents)); e != nil {
			t.Fatalf("migration %s: %v", filepath.Base(file), e)
		}
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	var before string
	if err = db.QueryRow(ctx, "SELECT row_to_json(r)::text FROM inspection_records r WHERE remark='migration-preservation-fixture'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/012_phase_two_inspection_extensions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "BEGIN;"+string(migration)+"COMMIT;"); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err = db.QueryRow(ctx, `SELECT to_jsonb(r) - 'occurrence_kind' - 'inspection_date' - 'inspection_round' = $1::jsonb FROM inspection_records r WHERE remark='migration-preservation-fixture'`, before).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("legacy record changed: %v", err)
	}
	cases, err := os.ReadFile("testdata/phase_two_extension_constraints.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(cases)); err != nil {
		t.Fatal(err)
	}
}

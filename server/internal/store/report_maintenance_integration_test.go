package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/organization"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Requires a dedicated empty database: the caller drops it, including immutable audit.
func TestExclusiveReportMaintenancePostgres(t *testing.T) {
	url := os.Getenv("REPORT_MAINTENANCE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires empty disposable REPORT_MAINTENANCE_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err = pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("requires empty database", err)
	}
	files, _ := filepath.Glob("../../migrations/*.sql")
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(b)); e != nil {
			t.Fatal(f, e)
		}
	}
	db := &Postgres{Pool: pool}
	var office, study, dorm string
	for code, target := range map[string]*string{"OFFICE": &office, "STUDY": &study, "DORMITORY": &dorm} {
		if err = pool.QueryRow(ctx, "SELECT id::text FROM departments WHERE code=$1", code).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	mutation := audit.Mutation{Entry: audit.Entry{Action: audit.ActionOrganizationUpdated, ResourceType: "departmentDefaultPermissions", ResourceID: office}}
	current, err := db.GetDepartmentDefaults(ctx, office)
	if err != nil {
		t.Fatal(err)
	}
	if current.ReportMaintenance.DepartmentID != office {
		t.Fatal("missing owner summary")
	}
	current.ReportScope.Enabled = false
	if _, err = db.SaveDepartmentDefaults(ctx, current, mutation); !errors.Is(err, organization.ErrReportMaintenanceRequired) {
		t.Fatalf("close: %v", err)
	}
	if _, err = db.UpdateDepartment(ctx, organization.Department{ID: office, Name: "Office", Enabled: false}, mutation); !errors.Is(err, organization.ErrReportMaintenanceDepartmentInUse) {
		t.Fatalf("disable: %v", err)
	}
	other, err := db.GetDepartmentDefaults(ctx, study)
	if err != nil {
		t.Fatal(err)
	}
	before := other.Version
	other.ReportScope.Enabled = true
	if _, err = db.SaveDepartmentDefaults(ctx, other, mutation); !errors.Is(err, organization.ErrReportMaintenanceAssigned) {
		t.Fatalf("second: %v", err)
	}
	after, err := db.GetDepartmentDefaults(ctx, study)
	if err != nil || after.Version != before || after.ReportScope.Enabled {
		t.Fatal("conflict partially saved", err)
	}
	// Closing every action does not release the maintenance qualification.
	current, err = db.GetDepartmentDefaults(ctx, office)
	if err != nil {
		t.Fatal(err)
	}
	current.ReportScope = organization.DepartmentReportScope{Enabled: true}
	current.ReportRoles = nil
	current, err = db.SaveDepartmentDefaults(ctx, current, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if !current.ReportScope.Enabled || current.ReportMaintenance.DepartmentID != office {
		t.Fatal("empty actions released owner")
	}
	// Audit failure must roll back the whole successful-looking configuration update.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_maintenance_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected audit failure'; END $$;
 CREATE TRIGGER fail_maintenance_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_maintenance_audit()`); err != nil {
		t.Fatal(err)
	}
	current.ReportScope.CanView = true
	if _, err = db.SaveDepartmentDefaults(ctx, current, mutation); err == nil {
		t.Fatal("audit failure ignored")
	}
	verified, err := db.GetDepartmentDefaults(ctx, office)
	if err != nil || verified.Version != current.Version || verified.ReportScope.CanView {
		t.Fatal("audit failure partially committed", err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_maintenance_audit ON audit_logs; DROP FUNCTION fail_maintenance_audit()"); err != nil {
		t.Fatal(err)
	}
	// Explicit fixture reset models two concurrent first assignments, never a business operation.
	if _, err = pool.Exec(ctx, "UPDATE department_report_scopes SET enabled=false"); err != nil {
		t.Fatal(err)
	}
	candidates := make([]organization.DepartmentDefaultPermissions, 2)
	for i, id := range []string{study, dorm} {
		candidates[i], err = db.GetDepartmentDefaults(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		candidates[i].ReportScope.Enabled = true
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, input := range candidates {
		wg.Add(1)
		go func(v organization.DepartmentDefaultPermissions) {
			defer wg.Done()
			_, e := db.SaveDepartmentDefaults(ctx, v, mutation)
			results <- e
		}(input)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, organization.ErrReportMaintenanceAssigned) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	// A conflicting legacy installation must fail migration without choosing a winner.
	if _, err = pool.Exec(ctx, "DROP INDEX department_report_scopes_one_enabled; UPDATE department_report_scopes SET enabled=true"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/058_weekly_report_exclusive_owner.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err == nil {
		t.Fatal("migration accepted multiple owners")
	}
	var owners int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM department_report_scopes WHERE enabled").Scan(&owners); err != nil || owners < 2 {
		t.Fatal("migration silently changed owners", err)
	}

}

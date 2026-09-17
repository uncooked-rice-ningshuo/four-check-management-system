package store

import (
	"context"
	"os"
	"testing"

	"four-check-management-system/server/internal/dashboard"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test uses connection-local tables and never applies migrations or writes
// business tables. A single connection keeps the temporary fixture isolated.
func TestDashboardPostgresIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("cannot open test database")
	}
	defer pool.Close()
	repository := &Postgres{Pool: pool}
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE inspection_records (
 id uuid, created_by uuid, semester_id uuid, week_id uuid, department_id uuid,
 inspection_type_id uuid, class_id uuid, grade_snapshot text, status text, deleted_at timestamptz
 )`)
	if err != nil {
		t.Fatal(err)
	}
	const actor = "00000000-0000-0000-0000-000000000001"
	const other = "00000000-0000-0000-0000-000000000002"
	const semester = "00000000-0000-0000-0000-000000000003"
	const week = "00000000-0000-0000-0000-000000000004"
	const department = "00000000-0000-0000-0000-000000000005"
	const inspectionType = "00000000-0000-0000-0000-000000000006"
	const class = "00000000-0000-0000-0000-000000000007"
	for _, row := range []struct {
		creator, status, grade string
		deleted                bool
	}{
		{actor, "DRAFT", "2024", false}, {actor, "RETURNED", "2025", false},
		{other, "DRAFT", "2024", false}, {actor, "CONFIRMED", "2024", false},
		{actor, "DRAFT", "2024", true},
	} {
		_, err = pool.Exec(ctx, `INSERT INTO inspection_records VALUES
  (gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $9 THEN now() ELSE NULL END)`,
			row.creator, semester, week, department, inspectionType, class, row.grade, row.status, row.deleted)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, scenario := range []struct {
		name            string
		unrestricted    bool
		allowed, denied []dashboard.DataScope
		want            int
	}{
		{name: "no authorization", want: 0},
		{name: "super admin still counts only own records", unrestricted: true, want: 2},
		{name: "class allow", allowed: []dashboard.DataScope{{ClassID: class}}, want: 2},
		{name: "deny subtracts grade", allowed: []dashboard.DataScope{{DepartmentID: department}}, denied: []dashboard.DataScope{{Grade: "2025"}}, want: 1},
		{name: "different department", allowed: []dashboard.DataScope{{DepartmentID: other}}, want: 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			got, err := repository.PersonalTodos(ctx, dashboard.PersonalTodoQuery{AccountID: actor, SemesterID: semester, WeekID: week, Unrestricted: scenario.unrestricted, Allowed: scenario.allowed, Denied: scenario.denied})
			if err != nil {
				t.Fatal(err)
			}
			if got.Total != scenario.want {
				t.Fatalf("total=%d, want %d", got.Total, scenario.want)
			}
		})
	}
	counts, err := repository.Aggregate(ctx, dashboard.AggregateQuery{AccountID: actor, SemesterID: semester, WeekID: week, Scopes: []dashboard.Scope{{DepartmentID: department, InspectionTypeID: inspectionType}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 || counts[0].Total != 4 || counts[0].Draft != 2 || counts[0].Returned != 1 || counts[0].Confirmed != 1 {
		t.Fatalf("department counts=%+v", counts)
	}
	empty, err := repository.Aggregate(ctx, dashboard.AggregateQuery{SemesterID: semester, WeekID: week})
	if err != nil || len(empty) != 0 {
		t.Fatalf("unauthorized aggregate=%+v err=%v", empty, err)
	}
	counts, err = repository.Aggregate(ctx, dashboard.AggregateQuery{SemesterID: semester, WeekID: other, Scopes: []dashboard.Scope{{DepartmentID: department, InspectionTypeID: inspectionType}}})
	if err != nil || len(counts) != 1 || counts[0].Total != 0 {
		t.Fatalf("different week=%+v err=%v", counts, err)
	}
}

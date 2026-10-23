package store

import (
	"strings"
	"testing"

	"four-check-management-system/server/internal/dashboard"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
)

func TestDashboardAggregateSQLConstrainsScopeAndPersonalTodos(t *testing.T) {
	for _, fragment := range []string{
		"unnest($3::text[], $4::text[])",
		"COALESCE(ir.department_id::text,'') = a.department_id",
		"ir.inspection_type_id::text = a.inspection_type_id",
		"ir.semester_id = $1::uuid",
		"ir.week_id = $2::uuid",
		"ir.deleted_at IS NULL",
	} {
		if !strings.Contains(dashboardAggregateSQL, fragment) {
			t.Errorf("dashboard aggregate query does not contain %q", fragment)
		}
	}
	if strings.Contains(dashboardAggregateSQL, "created_by") {
		t.Fatal("department aggregate must remain independent from personal ownership")
	}
}

func TestBuildPersonalTodoQueryCompilesAllowAndDenyScopes(t *testing.T) {
	sql, args := buildPersonalTodoQuery(dashboard.PersonalTodoQuery{
		AccountID: "account-1", SemesterID: "semester-1", WeekID: "week-1",
		Allowed: []dashboard.DataScope{{DepartmentID: "department-1", InspectionTypeID: "type-1", ClassID: "class-1"}},
		Denied:  []dashboard.DataScope{{DepartmentID: "department-1", Grade: "2024"}},
	})
	for _, fragment := range []string{"ir.created_by = $1::uuid", "ir.semester_id = $2::uuid", "ir.week_id = $3::uuid", "ir.inspection_type_id = $4::uuid", "ir.department_id = $5::uuid", "ir.class_id = $6::uuid", "NOT (", "ir.grade_snapshot = $8", "ir.deleted_at IS NULL"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("personal query does not contain %q: %s", fragment, sql)
		}
	}
	if len(args) != 8 {
		t.Fatalf("args = %#v", args)
	}
}

func TestMutationSQLSeparatesOwnAndManagedRecords(t *testing.T) {
	args := []any{"record"}
	sql := mutationScopeSQL(inspection.MutationAccess{ActorID: "actor", Permissions: []permission.EffectivePermission{
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "study"}},
		{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: "class"}},
		{Action: permission.ActionInspectionManage, Effect: permission.EffectDeny, Scope: permission.Scope{AcademicYearID: "year", Grade: "2024"}},
	}}, &args)
	for _, fragment := range []string{"ir.created_by=", "ir.department_id=", "ir.class_id=", "sem.academic_year_id=", "ir.grade_snapshot=", "AND NOT", " OR "} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing %s", fragment)
		}
	}
}

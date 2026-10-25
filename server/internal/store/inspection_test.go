package store

import (
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"strings"
	"testing"
)

func TestInspectionScopeQuery(t *testing.T) {
	a := inspection.MutationAccess{ActorID: "actor", Permissions: []permission.EffectivePermission{
		{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: "class-one"}},
		{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: permission.Scope{Grade: "2026"}},
	}}
	args := []any{}
	sql := inspectionAccessSQL(a, "read", &args)
	if !strings.Contains(sql, "NOT EXISTS") || !strings.Contains(sql, "inspection_responsibility_classes") || !strings.Contains(sql, "rc.class_id") || !strings.Contains(sql, "rc.grade_snapshot") || strings.Contains(sql, "class-one") {
		t.Fatalf("scope missing or interpolated: %s", sql)
	}
	args = nil
	if sql = inspectionAccessSQL(inspection.MutationAccess{}, "read", &args); strings.Contains(sql, "TRUE") {
		t.Fatal("empty permission expanded")
	}
}

func TestReportScopedReadIncludesClasslessRecordsWithinAuthorizedCell(t *testing.T) {
	access := inspection.MutationAccess{
		ReportScopeValidated: true,
		Permissions: []permission.EffectivePermission{{
			Action: permission.ActionInspectionView, Effect: permission.EffectAllow,
			Scope: permission.Scope{SemesterID: "semester", WeekID: "week", InspectionTypeID: "type", DepartmentID: "department"},
		}},
	}
	args := []any{}
	sql := inspectionAccessSQL(access, "read", &args)
	if strings.Contains(sql, "ir.class_id IS NOT NULL") || strings.Contains(sql, "inspection_responsibility_classes") {
		t.Fatalf("report-scoped read still excludes classless records: %s", sql)
	}
	for _, dimension := range []string{"ir.semester_id", "ir.week_id", "ir.inspection_type_id", "ir.department_id"} {
		if !strings.Contains(sql, dimension) {
			t.Errorf("report-scoped read omitted %s: %s", dimension, sql)
		}
	}
}

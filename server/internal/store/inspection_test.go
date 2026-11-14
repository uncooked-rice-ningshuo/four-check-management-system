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

func TestPresidiumInspectionReadRequiresPrimaryDepartment(t *testing.T) {
	access := inspection.MutationAccess{RestrictReadDepartment: true, Permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow}}}
	args := []any{}
	if sql := inspectionAccessSQL(access, "read", &args); sql != "FALSE" {
		t.Fatalf("missing primary department must deny read: %s", sql)
	}
	access.ReadDepartmentID = "department-1"
	args = nil
	sql := inspectionAccessSQL(access, "read", &args)
	if !strings.Contains(sql, "ir.department_id=$1::uuid") || len(args) != 1 || args[0] != "department-1" {
		t.Fatalf("primary department must be a bound read condition: sql=%s args=%#v", sql, args)
	}
	args = nil
	if sql := inspectionAccessSQL(access, "mutate", &args); strings.Contains(sql, "ir.department_id=$1::uuid") {
		t.Fatalf("read restriction changed mutation semantics: %s", sql)
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

func TestClassGradeLabelKeepsSingleGradeSuffix(t *testing.T) {
	cases := []struct{ className, grade, want string }{
		{"计算机科学与技术1班", "2024级", "计算机科学与技术1班（2024级）"},
		{"计算机科学与技术1班", "2024", "计算机科学与技术1班（2024级）"},
		{"软件工程1班", " 2025级 ", "软件工程1班（2025级）"},
		{"软件工程1班", "", "软件工程1班"},
		{"软件工程1班", "   ", "软件工程1班"},
	}
	for _, c := range cases {
		if got := classGradeLabel(c.className, c.grade); got != c.want {
			t.Errorf("classGradeLabel(%q, %q) = %q, want %q", c.className, c.grade, got, c.want)
		}
		if strings.Contains(classGradeLabel(c.className, c.grade), "级级") {
			t.Errorf("classGradeLabel(%q, %q) repeated the 级 suffix", c.className, c.grade)
		}
	}
}

func TestInspectionViewIsBusinessScopedAndMutationSeparatesEntryFromManagement(t *testing.T) {
	access := inspection.MutationAccess{ActorID: "actor", Permissions: []permission.EffectivePermission{
		{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: "attendance"}},
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: "attendance"}},
		{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: "attendance"}},
	}}
	readArgs := []any{}
	readSQL := inspectionAccessSQL(access, "read", &readArgs)
	if strings.Contains(readSQL, "ir.created_by=") || !strings.Contains(readSQL, "ir.inspection_type_id") {
		t.Fatalf("view must apply the business scope without creator filtering: %s", readSQL)
	}
	if len(readArgs) != 2 || readArgs[0] != "attendance" || readArgs[1] != "attendance" {
		t.Fatalf("view query arguments = %#v", readArgs)
	}

	mutateArgs := []any{}
	mutateSQL := inspectionAccessSQL(access, "mutate", &mutateArgs)
	if !strings.Contains(mutateSQL, "ir.created_by=") || !strings.Contains(mutateSQL, "ir.inspection_type_id") || !strings.Contains(mutateSQL, " OR ") {
		t.Fatalf("mutation must allow owned ENTER records or scoped MANAGE records: %s", mutateSQL)
	}
}

package importjob_test

import (
	"context"
	"fmt"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestAttendanceContextOptionsPostgres(t *testing.T) {
	url := os.Getenv("CONTEXT_OPTIONS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	var yearID, typeID, departmentID, classID, gradeID string
	if err = db.Pool.QueryRow(ctx, `SELECT academic_year_id FROM semesters WHERE id=$1`, fixture.semesterID).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT i.id,ds.department_id FROM inspection_types i JOIN department_inspection_scopes ds ON ds.inspection_type_id=i.id WHERE i.code='ATTENDANCE_RATE' AND ds.enabled AND ds.can_enter LIMIT 1`).Scan(&typeID, &departmentID); err != nil {
		t.Fatal(err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT id,grade_id FROM classes ORDER BY id LIMIT 1`).Scan(&classID, &gradeID); err != nil {
		t.Fatal(err)
	}
	allow := permission.EffectivePermission{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{AcademicYearID: yearID, InspectionTypeID: typeID, DepartmentID: departmentID}}
	deny := func(scope permission.Scope) permission.EffectivePermission {
		return permission.EffectivePermission{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: scope}
	}
	otherAllow := allow
	otherAllow.Scope.AcademicYearID = uuid.NewString()
	for _, scenario := range []struct {
		name    string
		rules   []permission.EffectivePermission
		classes int
	}{
		{"matching year", []permission.EffectivePermission{allow}, 2},
		{"other year", []permission.EffectivePermission{otherAllow}, 0},
		{"denied year", []permission.EffectivePermission{allow, deny(permission.Scope{AcademicYearID: yearID})}, 0},
		{"unrelated deny", []permission.EffectivePermission{allow, deny(permission.Scope{AcademicYearID: uuid.NewString()})}, 2},
		{"class deny", []permission.EffectivePermission{allow, deny(permission.Scope{ClassID: classID})}, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, kind := range []string{"semesters", "weeks", "departments", "classes", "grades"} {
				result, err := db.ListContextOptions(ctx, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeAttendance, Purpose: "view", Kind: kind, SemesterID: fixture.semesterID, Page: 1, PageSize: 200}, inspection.MutationAccess{ActorID: fixture.accountID, Permissions: scenario.rules})
				count := 1
				if scenario.classes == 0 {
					count = 0
				} else if kind == "classes" {
					count = scenario.classes
				}
				if err != nil || result.Total != count || len(result.Items) != count {
					t.Fatalf("%s options total=%d len=%d: %v", kind, result.Total, len(result.Items), err)
				}
				if kind == "classes" {
					for _, c := range result.Items {
						if c.Grade != "2026级" {
							t.Fatal("missing current grade metadata")
						}
					}
				}
			}
		})
	}
	t.Run("authorized class pagination", func(t *testing.T) {
		for i := 0; i < 199; i++ {
			if _, err := db.Pool.Exec(ctx, `INSERT INTO classes(id,name,grade_id) VALUES($1,$2,$3)`, uuid.NewString(), fmt.Sprintf("isolated-option-%03d", i), gradeID); err != nil {
				t.Fatal(err)
			}
		}
		for page, size := range map[int]int{1: 200, 2: 1} {
			result, err := db.ListContextOptions(ctx, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeAttendance, Purpose: "view", Kind: "classes", SemesterID: fixture.semesterID, Page: page, PageSize: 200}, inspection.MutationAccess{ActorID: fixture.accountID, Permissions: []permission.EffectivePermission{allow}})
			if err != nil || result.Total != 201 || len(result.Items) != size {
				t.Fatal("pagination", result.Total, len(result.Items), err)
			}
		}
	})
}

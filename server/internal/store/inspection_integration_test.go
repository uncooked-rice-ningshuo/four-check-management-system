package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

const recordFixturePrefix = "a0000000-0000-0000-0000-"

func rf(n string) string { return recordFixturePrefix + "0000000000" + n }

func containsOption(items []inspection.EntryOption, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

type recordPermissions struct {
	rules []permission.EffectivePermission
}

func (p *recordPermissions) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.rules}, nil
}
func TestInspectionPostgresCRUD(t *testing.T) {
	url := os.Getenv("INSPECTION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("INSPECTION_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := &Postgres{Pool: pool}
	var exists bool
	if err = pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("requires empty disposable database", err)
	}
	files, _ := filepath.Glob("../../migrations/*.sql")
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(b)); e != nil {
			t.Fatal(file, e)
		}
	}
	var sportsDepartmentName string
	if err = pool.QueryRow(ctx, `SELECT name FROM departments WHERE code='SPORTS'`).Scan(&sportsDepartmentName); err != nil || sportsDepartmentName != "文体部" {
		t.Fatalf("SPORTS department name after migration chain = %q, err=%v", sportsDepartmentName, err)
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	sql := `UPDATE students SET dormitory_id='a0000000-0000-0000-0000-000000000012';
	 INSERT INTO students(id,student_no,name,grade_id,class_id,dormitory_id) VALUES('a0000000-0000-0000-0000-000000000023','S-FIXTURE-2','Other student','a0000000-0000-0000-0000-000000000005','a0000000-0000-0000-0000-000000000016','a0000000-0000-0000-0000-000000000012');
	 INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES('a0000000-0000-0000-0000-000000000024','inspection-member','Synthetic member','not-a-login-hash','ACTIVE','SYSTEM','MEMBER',false,false);
	 UPDATE semester_weeks SET attendance_penalty_multiplier=1 WHERE id='a0000000-0000-0000-0000-000000000004';
 INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter,can_review) SELECT '10000000-0000-0000-0000-000000000002',id,true,true,true FROM inspection_types ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true;`
	if _, err = pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	perms := &recordPermissions{}
	svc := inspection.NewRecordService(db, perms)
	admin := inspection.ManualActor{ID: rf("01"), IsSuperAdmin: true}
	member := inspection.ManualActor{ID: rf("01")}
	departmentActor := inspection.ManualActor{ID: rf("01"), IsMember: true, PrimaryDepartmentID: "10000000-0000-0000-0000-000000000002"}
	departmentSvc := inspection.NewRecordService(db, &recordPermissions{rules: []permission.EffectivePermission{
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID}},
		{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID}},
	}})
	types := map[string]string{}
	rows, err := pool.Query(ctx, "SELECT code,id FROM inspection_types")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code, id string
		if err = rows.Scan(&code, &id); err != nil {
			t.Fatal(err)
		}
		types[code] = id
	}
	rows.Close()
	makeInput := func(code string, extra map[string]any) inspection.InspectionInput {
		fields := map[string]any{"date": "2026-09-03", "remark": "Synthetic private text"}
		switch code {
		case inspection.InspectionTypeAttendance:
			// 到课率 keeps the period and hour columns; the clock time is no longer collected.
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["courseId"] = rf("13")
			fields["classroomId"] = rf("09")
			fields["period"] = "1-2"
			fields["absenceHours"] = 2
		case inspection.InspectionTypeUnattended:
			// 无机课堂 requires its course section for the weekly student/section identity.
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["courseId"] = rf("13")
			fields["classroomId"] = rf("09")
			fields["period"] = "1-2"
		case inspection.InspectionTypeEveningSelfStudy:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["classroomId"] = rf("09")
		case inspection.InspectionTypeMorning:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
		case inspection.InspectionTypeLateReturn:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["dormitoryId"] = rf("12")
		default:
			fields["result"] = "QUALIFIED"
			fields["responsibleClassIds"] = []string{rf("06")}
			fields["responsibleStudentIds"] = []string{rf("07")}
			if code == inspection.InspectionTypeDormitoryHygiene {
				fields["dormitoryId"] = rf("12")
			} else if code == inspection.InspectionTypeClassroomHygiene {
				fields["classroomId"] = rf("09")
			} else {
				fields["responsibleDormitoryIds"] = []string{rf("12")}
			}
		}
		for k, v := range extra {
			fields[k] = v
		}
		raw, _ := json.Marshal(fields)
		return inspection.InspectionInput{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[code], DepartmentID: "10000000-0000-0000-0000-000000000002", Details: raw}
	}

	t.Run("context options", func(t *testing.T) {
		service := inspection.NewContextOptionService(db, perms)
		q := inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeUnattended, Purpose: "view", Kind: "semesters", PageSize: 1}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: rf("06")}}}
		page, err := service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != rf("03") || page.InspectionTypeID != types[q.TypeCode] {
			t.Fatal("authorized semester options", page, err)
		}
		q.Kind = "weeks"
		q.SemesterID = rf("03")
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 1 || page.Items[0].ID != rf("04") {
			t.Fatal("week options", page, err)
		}
		q.Kind = "classes"
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 1 || page.Items[0].ID != rf("06") {
			t.Fatal("class options", page, err)
		}
		q.Kind = "departments"
		q.WeekID = rf("04")
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total == 0 {
			t.Fatal("department options", page, err)
		}
		q.Purpose = "entry"
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("view cannot enter", page, err)
		}
		perms.rules[0].Action = permission.ActionInspectionEnter
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 2 {
			t.Fatal("entry departments", page, err)
		}
		for _, scope := range []permission.Scope{{AcademicYearID: rf("02")}, {SemesterID: rf("03")}, {WeekID: rf("04")}, {InspectionTypeID: types[q.TypeCode]}, {DepartmentID: "10000000-0000-0000-0000-000000000002"}, {ClassID: rf("06")}, {Grade: "2026"}} {
			perms.rules = append(perms.rules[:1], permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: scope})
			page, err = service.ContextOptions(ctx, member, q)
			if err != nil || page.Total != 0 {
				t.Fatal("context deny", scope, page, err)
			}
		}
		perms.rules = nil
		q = inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeUnattended, Purpose: "view", Kind: "classes", PageSize: 1}
		first, err := service.ContextOptions(ctx, admin, q)
		q.Page = 2
		second, err2 := service.ContextOptions(ctx, admin, q)
		if err != nil || err2 != nil || first.Total != 2 || second.Total != 2 || len(first.Items) != 1 || len(second.Items) != 1 || first.Items[0].ID == second.Items[0].ID {
			t.Fatal("context pagination", err, err2)
		}
		record, err := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeUnattended, nil), audit.Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		q = inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeUnattended, Purpose: "entry", Kind: "semesters", RecordID: record.ID}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: rf("06")}}}
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 1 {
			t.Fatal("manager edit context", page, err)
		}
		q.RecordID = ""
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("manager cannot obtain creation context", err)
		}
		q.RecordID = record.ID
		perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionManage, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: rf("06")}})
		if _, err = service.ContextOptions(ctx, member, q); !errors.Is(err, inspection.ErrRecordNotFound) {
			t.Fatal("original context access denied", err)
		}
		if _, err = pool.Exec(ctx, "UPDATE semesters SET enabled=false WHERE id=$1", rf("03")); err != nil {
			t.Fatal(err)
		}
		q = inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeUnattended, Purpose: "view", Kind: "semesters"}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: rf("06")}}}
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 1 {
			t.Fatal("historical calendar remains visible", page, err)
		}
		perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: permission.Scope{Grade: "2026"}})
		page, err = service.ContextOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("historical scope deny", page, err)
		}
		q.Purpose = "entry"
		page, err = service.ContextOptions(ctx, admin, q)
		if err != nil || page.Total != 0 {
			t.Fatal("disabled calendar cannot enter", page, err)
		}
		if _, err = pool.Exec(ctx, "UPDATE semesters SET enabled=true WHERE id=$1", rf("03")); err != nil {
			t.Fatal(err)
		}
		if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: 1}}, Reason: "isolated fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
		perms.rules = nil
	})
	t.Run("entry options", func(t *testing.T) {
		options := inspection.NewEntryOptionService(db, perms)
		q := inspection.EntryOptionQuery{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[inspection.InspectionTypeUnattended], DepartmentID: "10000000-0000-0000-0000-000000000002", Kind: "classes", PageSize: 1}
		page, e := options.EntryOptions(ctx, admin, q)
		if e != nil || page.Total != 2 || len(page.Items) != 1 {
			t.Fatal("paged classes", page, e)
		}
		q.Page = 2
		next, e := options.EntryOptions(ctx, admin, q)
		if e != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
			t.Fatal("second page", e)
		}
		q.Page = 1
		if _, e = pool.Exec(ctx, `UPDATE grades SET name='2026级' WHERE id=$1`, rf("05")); e != nil {
			t.Fatal(e)
		}
		page, e = options.EntryOptions(ctx, admin, q)
		if _, restoreErr := pool.Exec(ctx, `UPDATE grades SET name='2026' WHERE id=$1`, rf("05")); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if e != nil || len(page.Items) == 0 || strings.Contains(page.Items[0].Label, "级级") {
			t.Fatal("grade suffix repeated in class option", page, e)
		}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: rf("06")}}}
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 1 || page.Items[0].ID != rf("06") {
			t.Fatal("class scope", page, e)
		}
		for _, kind := range []string{"students", "courses", "classrooms"} {
			q.Kind, q.ClassID, q.CourseID = kind, rf("06"), rf("13")
			page, e = options.EntryOptions(ctx, member, q)
			wantTotal := 1
			if kind == "classrooms" {
				// Classrooms are selected independently from course scheduling.
				wantTotal = 2
			}
			if e != nil || page.Total != wantTotal || len(page.Items) != 1 || page.Items[0].Label == "" {
				t.Fatal(kind, page, e)
			}
			if kind == "students" && page.Items[0].Label != "Synthetic student（S-FIXTURE，Fixture class）" {
				t.Fatal("student option must distinguish name, number and class", page.Items[0])
			}
		}
		q.InspectionTypeID = types[inspection.InspectionTypeAttendance]
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: rf("06")}}}
		q.Kind, q.Keyword = "courses", "Fixture"
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 1 || page.Items[0].Label != "Fixture course" {
			t.Fatal("unassociated course should be searchable for an authorized class", page, e)
		}
		q.Keyword = "no match"
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 0 {
			t.Fatal("course fuzzy search should filter unmatched names", page, e)
		}
		q.Kind, q.Keyword, q.CourseID = "classrooms", "", rf("13")
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 2 {
			t.Fatal("classroom option should not depend on a course schedule", page, e)
		}
		q.InspectionTypeID = types[inspection.InspectionTypeUnattended]
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: rf("06")}}}
		perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: rf("06")}})
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 0 || len(page.Items) != 0 {
			t.Fatal("deny", page, e)
		}

		for _, denied := range []permission.Scope{
			{AcademicYearID: rf("02")}, {SemesterID: rf("03")}, {WeekID: rf("04")},
			{InspectionTypeID: q.InspectionTypeID}, {DepartmentID: q.DepartmentID}, {Grade: "2026"},
		} {
			perms.rules = []permission.EffectivePermission{
				{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow},
				{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: denied},
			}
			page, e = options.EntryOptions(ctx, member, q)
			if e != nil || page.Total != 0 {
				t.Fatal("scope deny leaked options", e)
			}
		}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow}}
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 0 {
			t.Fatal("manage enabled creation", e)
		}
		record, e := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeUnattended, nil), audit.Metadata{})
		if e != nil {
			t.Fatal(e)
		}
		q.RecordID = record.ID
		page, e = options.EntryOptions(ctx, member, q)
		if e != nil || page.Total != 2 {
			t.Fatal("manage edit options", page, e)
		}
		perms.rules = nil
		if _, e = options.EntryOptions(ctx, member, q); !errors.Is(e, inspection.ErrRecordNotFound) {
			t.Fatal("original scope bypass", e)
		}
		if e = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: record.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); e != nil {
			t.Fatal(e)
		}
		perms.rules = nil
	})

	t.Run("morning scoped options", func(t *testing.T) {
		defer func() { perms.rules = nil }()
		options := inspection.NewEntryOptionService(db, perms)
		contexts := inspection.NewContextOptionService(db, perms)
		q := inspection.EntryOptionQuery{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[inspection.InspectionTypeMorning], DepartmentID: "10000000-0000-0000-0000-000000000002", Kind: "students", ClassID: rf("06")}
		cq := inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeMorning, Purpose: "entry", Kind: "classes", SemesterID: q.SemesterID, WeekID: q.WeekID, DepartmentID: q.DepartmentID}
		allow := permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: q.ClassID}}
		perms.rules = []permission.EffectivePermission{allow}
		page, err := options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != rf("07") {
			t.Fatal("morning student options", page, err)
		}
		cp, err := contexts.ContextOptions(ctx, member, cq)
		if err != nil || cp.Total != 1 || cp.Items[0].ID != q.ClassID {
			t.Fatal("morning context", cp, err)
		}
		for _, deny := range []permission.Scope{{AcademicYearID: rf("02")}, {SemesterID: q.SemesterID}, {WeekID: q.WeekID}, {InspectionTypeID: q.InspectionTypeID}, {DepartmentID: q.DepartmentID}, {ClassID: q.ClassID}, {Grade: "2026"}} {
			perms.rules = []permission.EffectivePermission{allow, {Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: deny}}
			page, err = options.EntryOptions(ctx, member, q)
			if err != nil || page.Total != 0 {
				t.Fatal("morning entry DENY leaked", page, err)
			}
			cp, err = contexts.ContextOptions(ctx, member, cq)
			if err != nil || cp.Total != 0 {
				t.Fatal("morning context DENY leaked", cp, err)
			}
		}
		perms.rules = []permission.EffectivePermission{allow}
		q.ClassID = rf("16")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("morning unrelated class", page, err)
		}
		q.ClassID = rf("06")
		for _, kind := range []string{"courses", "classrooms", "dormitories"} {
			q.Kind, q.CourseID, q.StudentID = kind, rf("13"), rf("07")
			if _, err = options.EntryOptions(ctx, admin, q); !errors.Is(err, inspection.ErrInspectionRequest) {
				t.Fatal("unrelated morning option kind accepted", kind, err)
			}
		}
		q.Kind = "students"
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow}}
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("morning manage enabled creation", page, err)
		}
		record, err := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-04"}), audit.Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		q.RecordID = record.ID
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 {
			t.Fatal("morning manage edit options", page, err)
		}
		if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: record.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("dormitory hygiene scoped options and display", func(t *testing.T) {
		defer func() { perms.rules = nil }()
		options := inspection.NewEntryOptionService(db, perms)
		contexts := inspection.NewContextOptionService(db, perms)
		q := inspection.EntryOptionQuery{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[inspection.InspectionTypeDormitoryHygiene], DepartmentID: "10000000-0000-0000-0000-000000000002", Kind: "dormitory-objects", ClassID: rf("06"), PageSize: 1}
		allow := permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: q.ClassID}}
		perms.rules = []permission.EffectivePermission{allow}
		contextQuery := inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeDormitoryHygiene, Purpose: "entry", Kind: "classes", SemesterID: q.SemesterID, WeekID: q.WeekID, DepartmentID: q.DepartmentID}
		contextPage, err := contexts.ContextOptions(ctx, member, contextQuery)
		if err != nil || contextPage.Total != 1 || contextPage.Items[0].ID != q.ClassID {
			t.Fatal("hygiene context", contextPage, err)
		}
		page, err := options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != rf("12") || page.Items[0].Label == "" {
			t.Fatal("hygiene room option", page, err)
		}
		q.Kind = "situation-snippets"
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 6 || page.Items[0].ID != "FLOOR" || page.Items[0].Label != "地面不洁" {
			t.Fatal("configured reason", page, err)
		}
		q.Kind = "students"
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 || page.Items[0].ID != rf("07") {
			t.Fatal("responsible student", page, err)
		}
		for _, deny := range []permission.Scope{{AcademicYearID: rf("02")}, {SemesterID: q.SemesterID}, {WeekID: q.WeekID}, {InspectionTypeID: q.InspectionTypeID}, {DepartmentID: q.DepartmentID}, {ClassID: q.ClassID}, {Grade: "2026"}} {
			perms.rules = []permission.EffectivePermission{allow, {Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: deny}}
			q.Kind = "dormitory-objects"
			page, err = options.EntryOptions(ctx, member, q)
			if err != nil || page.Total != 0 {
				t.Fatal("hygiene DENY leaked room", deny, page, err)
			}
			contextPage, err = contexts.ContextOptions(ctx, member, contextQuery)
			if err != nil || contextPage.Total != 0 {
				t.Fatal("hygiene DENY leaked context", deny, contextPage, err)
			}
		}
		perms.rules = []permission.EffectivePermission{allow}
		q.ClassID = rf("16")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("unrelated class exposed room", page, err)
		}
		q.ClassID = rf("06")
		for _, kind := range []string{"courses", "classrooms", "dormitories"} {
			q.Kind = kind
			q.StudentID, q.CourseID = rf("07"), rf("13")
			if _, err = options.EntryOptions(ctx, admin, q); !errors.Is(err, inspection.ErrInspectionRequest) {
				t.Fatal("unrelated hygiene kind accepted", kind, err)
			}
		}
		q.StudentID, q.CourseID, q.Kind = "", "", "dormitory-objects"
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow}}
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("manage enabled hygiene creation", page, err)
		}
		record, err := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeDormitoryHygiene, map[string]any{"result": "UNQUALIFIED", "description": "地面有灰尘"}), audit.Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		if record.CurrentDormitoryName == "" || len(record.ResponsibleClasses) != 1 || len(record.ResponsibleStudents) != 1 {
			t.Fatal("hygiene display labels", record)
		}
		if record.ResponsibleClasses[0].ID != rf("06") || record.ResponsibleStudents[0].ID != rf("07") || record.ResponsibleClasses[0].Label == "" || record.ResponsibleStudents[0].Label == "" {
			t.Fatal("hygiene snapshot identities", record)
		}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: q.ClassID}}}
		viewed, err := svc.Detail(ctx, member, record.ID)
		if err != nil || len(viewed.ResponsibleClasses) != 1 || viewed.CurrentDormitoryName != record.CurrentDormitoryName {
			t.Fatal("scoped hygiene detail", viewed, err)
		}
		perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: q.ClassID}})
		if _, err = svc.Detail(ctx, member, record.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
			t.Fatal("denied hygiene detail leaked", err)
		}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow}}
		q.RecordID = record.ID
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 {
			t.Fatal("manage hygiene edit options", page, err)
		}
		if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: record.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("hygiene scoped options without area or reason-code selectors", func(t *testing.T) {
		defer func() { perms.rules = nil }()
		options := inspection.NewEntryOptionService(db, perms)
		contexts := inspection.NewContextOptionService(db, perms)
		base := inspection.EntryOptionQuery{SemesterID: rf("03"), WeekID: rf("04"), DepartmentID: "10000000-0000-0000-0000-000000000002", ClassID: rf("06")}
		for _, scenario := range []struct {
			code, kind, want string
		}{
			{inspection.InspectionTypeClassroomHygiene, "classroom-objects", rf("09")},
			{inspection.InspectionTypeClassroomHygiene, "situation-snippets", "FLOOR"},
			{inspection.InspectionTypePublicDormitoryHygiene, "responsible-dormitories", rf("12")},
			{inspection.InspectionTypePublicDormitoryHygiene, "situation-snippets", "FLOOR"},
		} {
			q := base
			q.InspectionTypeID, q.Kind = types[scenario.code], scenario.kind
			allow := permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: q.ClassID}}
			perms.rules = []permission.EffectivePermission{allow}
			page, err := options.EntryOptions(ctx, member, q)
			found := false
			for _, item := range page.Items {
				if item.ID == scenario.want && item.Label != "" {
					found = true
				}
			}
			if err != nil || !found || page.Total < 1 {
				t.Fatal("hygiene option", scenario, page, err)
			}
			contextPage, err := contexts.ContextOptions(ctx, member, inspection.ContextOptionQuery{TypeCode: scenario.code, Purpose: "entry", Kind: "classes", SemesterID: q.SemesterID, WeekID: q.WeekID, DepartmentID: q.DepartmentID})
			if err != nil || contextPage.Total != 1 || contextPage.Items[0].ID != q.ClassID {
				t.Fatal("hygiene context", scenario, contextPage, err)
			}
			perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: q.ClassID}})
			page, err = options.EntryOptions(ctx, member, q)
			if err != nil || page.Total != 0 {
				t.Fatal("DENY leaked hygiene option", scenario, page, err)
			}
			perms.rules = nil
		}
		for _, code := range []string{inspection.InspectionTypeClassroomHygiene, inspection.InspectionTypePublicDormitoryHygiene} {
			record, err := departmentSvc.Create(ctx, departmentActor, makeInput(code, map[string]any{"result": "UNQUALIFIED", "description": "地面有灰尘"}), audit.Metadata{})
			if err != nil {
				t.Fatal(err)
			}
			if len(record.ResponsibleClasses) != 1 || len(record.ResponsibleStudents) != 1 || record.Details == nil {
				t.Fatal("hygiene display labels", code, record)
			}
			if code == inspection.InspectionTypePublicDormitoryHygiene && (len(record.ResponsibleDormitories) != 1 || record.ResponsibleDormitories[0].ID != rf("12") || record.ResponsibleDormitories[0].Label == "") {
				t.Fatal("responsible room labels", record)
			}
			if err := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: record.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
				t.Fatal(err)
			}
		}
		q := base
		for _, retired := range []struct{ code, kind string }{
			{inspection.InspectionTypeClassroomHygiene, "classroom-areas"},
			{inspection.InspectionTypePublicDormitoryHygiene, "public-areas"},
			{inspection.InspectionTypeClassroomHygiene, "reason-codes"},
		} {
			q.InspectionTypeID, q.Kind = types[retired.code], retired.kind
			if _, err := options.EntryOptions(ctx, admin, q); !errors.Is(err, inspection.ErrInspectionRequest) {
				t.Fatal("retired hygiene option accepted", retired, err)
			}
		}
	})
	t.Run("late return scoped options", func(t *testing.T) {
		options := inspection.NewEntryOptionService(db, perms)
		q := inspection.EntryOptionQuery{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[inspection.InspectionTypeLateReturn], DepartmentID: "10000000-0000-0000-0000-000000000002", Kind: "dormitories", ClassID: rf("06"), StudentID: rf("07")}
		allow := permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: q.InspectionTypeID, ClassID: q.ClassID}}
		perms.rules = []permission.EffectivePermission{allow}
		contextService := inspection.NewContextOptionService(db, perms)
		contexts, contextErr := contextService.ContextOptions(ctx, member, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeLateReturn, Purpose: "entry", Kind: "classes", SemesterID: q.SemesterID, WeekID: q.WeekID, DepartmentID: q.DepartmentID})
		if contextErr != nil || contexts.Total != 1 || contexts.Items[0].ID != q.ClassID {
			t.Fatal("late return class context", contexts, contextErr)
		}
		page, err := options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != rf("12") || page.Items[0].Label == "" {
			t.Fatal("student dormitory", page, err)
		}
		for _, denied := range []permission.Scope{{AcademicYearID: rf("02")}, {SemesterID: q.SemesterID}, {WeekID: q.WeekID}, {InspectionTypeID: q.InspectionTypeID}, {DepartmentID: q.DepartmentID}, {ClassID: q.ClassID}, {Grade: "2026"}} {
			perms.rules = []permission.EffectivePermission{allow, {Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: denied}}
			page, err = options.EntryOptions(ctx, member, q)
			if err != nil || page.Total != 0 {
				t.Fatal("denied dormitory exposed", page, err)
			}
		}
		perms.rules = []permission.EffectivePermission{allow}
		// Room-first entry: the room catalog is class-independent, and student
		// options carry the class that entry derives from the chosen student.
		q.Kind, q.StudentID, q.ClassID = "dormitory-objects", "", ""
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || !containsOption(page.Items, rf("12")) {
			t.Fatal("room catalog", page, err)
		}
		// Housing entry walks area → building → room.
		q.Kind, q.DormitoryAreaID, q.DormitoryBuildingID = "dormitory-areas", "", ""
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || !containsOption(page.Items, rf("10")) {
			t.Fatal("housing areas", page, err)
		}
		q.Kind, q.DormitoryAreaID = "dormitory-buildings", rf("10")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || !containsOption(page.Items, rf("11")) {
			t.Fatal("housing buildings", page, err)
		}
		q.Kind, q.DormitoryAreaID, q.DormitoryBuildingID = "dormitory-objects", "", rf("11")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != rf("12") || page.Items[0].Label != "101" {
			t.Fatal("building rooms", page, err)
		}
		q.DormitoryBuildingID = ""
		q.Kind, q.DormitoryID = "students", rf("12")
		page, err = options.EntryOptions(ctx, member, q)
		memberOption, found := inspection.EntryOption{}, false
		for _, item := range page.Items {
			if item.ID == rf("07") {
				memberOption, found = item, true
			}
		}
		if err != nil || !found || memberOption.ClassID != rf("06") {
			t.Fatal("dormitory members with class", page, err)
		}
		q.DormitoryID = rf("11")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("non-resident student", page, err)
		}
		q.Kind, q.DormitoryID, q.StudentID, q.ClassID = "dormitories", "", rf("01"), rf("06")
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("unrelated student", page, err)
		}
		q.StudentID = rf("07")
		q.Kind = "courses"
		if _, err = options.EntryOptions(ctx, member, q); !errors.Is(err, inspection.ErrInspectionRequest) {
			t.Fatal("late return course options accepted", err)
		}
		q.InspectionTypeID = types[inspection.InspectionTypeUnattended]
		for _, kind := range []string{"dormitories", "dormitory-objects"} {
			q.Kind = kind
			if _, err = options.EntryOptions(ctx, admin, q); !errors.Is(err, inspection.ErrInspectionRequest) {
				t.Fatal("unattended housing options accepted", err)
			}
		}
		q.InspectionTypeID = types[inspection.InspectionTypeLateReturn]
		q.Kind = "dormitories"
		if _, err = pool.Exec(ctx, "UPDATE dormitories SET enabled=false WHERE id=$1", rf("12")); err != nil {
			t.Fatal(err)
		}
		page, err = options.EntryOptions(ctx, member, q)
		if err != nil || page.Total != 0 {
			t.Fatal("disabled dormitory", page, err)
		}
		if _, err = pool.Exec(ctx, "UPDATE dormitories SET enabled=true WHERE id=$1", rf("12")); err != nil {
			t.Fatal(err)
		}
		perms.rules = nil
	})
	catalog, _ := inspection.InspectionCatalog()
	for _, invalid := range []inspection.InspectionInput{
		makeInput(inspection.InspectionTypeAttendance, map[string]any{"date": "2026-09-08"}),
		makeInput(inspection.InspectionTypeAttendance, map[string]any{"period": "3-4", "absenceHours": 1}),
	} {
		if _, e := departmentSvc.Create(ctx, departmentActor, invalid, audit.Metadata{}); e == nil {
			t.Fatal("invalid attendance details accepted")
		} else {
			if _, ok := e.(*inspection.DetailsValidationError); !ok {
				t.Fatalf("invalid attendance details returned %T: %v", e, e)
			}
		}
	}
	for _, invalid := range []inspection.InspectionInput{
		makeInput(inspection.InspectionTypeMorning, map[string]any{"studentId": rf("16")}),
		makeInput(inspection.InspectionTypeMorning, map[string]any{"classId": rf("16")}),
		makeInput(inspection.InspectionTypeClassroomHygiene, map[string]any{"classroomId": rf("20")}),
		makeInput(inspection.InspectionTypePublicDormitoryHygiene, map[string]any{"responsibleDormitoryIds": []string{rf("20")}}),
		makeInput(inspection.InspectionTypeDormitoryHygiene, map[string]any{"dormitoryId": rf("20")}),
	} {
		if _, e := departmentSvc.Create(ctx, departmentActor, invalid, audit.Metadata{}); !errors.Is(e, inspection.ErrInvalidAttendanceRecord) {
			t.Fatal("invalid reference accepted", e)
		}
	}
	t.Run("attendance penalty snapshot", func(t *testing.T) {
		var historicalMultiplier *int
		if err := pool.QueryRow(ctx, `SELECT penalty_multiplier FROM attendance_details WHERE record_id=$1`, rf("14")).Scan(&historicalMultiplier); err != nil || historicalMultiplier != nil {
			t.Fatal("historical penalty was silently backfilled", historicalMultiplier, err)
		}
		var ordinaryMultiplier int
		if err := pool.QueryRow(ctx, `SELECT attendance_penalty_multiplier FROM semester_weeks WHERE id=$1`, rf("04")).Scan(&ordinaryMultiplier); err != nil || ordinaryMultiplier != 1 {
			t.Fatal("unmarked week must default to ordinary 1x", ordinaryMultiplier, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=2 WHERE id=$1`, rf("04")); err != nil {
			t.Fatal(err)
		}
		record, err := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeAttendance, nil), audit.Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		check := func(value inspection.InspectionRecord) {
			t.Helper()
			details, ok := value.Details.(*inspection.AttendanceDetails)
			if !ok || details.PenaltyMultiplier == nil || *details.PenaltyMultiplier != 2 || details.PenaltyHours == nil || *details.PenaltyHours != 4 || details.AbsenceHours != 2 {
				t.Fatal("penalty snapshot differs from actual absence", value)
			}
		}
		check(record)
		if _, err := pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=1 WHERE id=$1`, rf("04")); err != nil {
			t.Fatal(err)
		}
		read, err := svc.Detail(ctx, admin, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		check(read)
		if err := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: record.ID, Version: record.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("member department attribution", func(t *testing.T) {
		defer func() { perms.rules = nil }()
		const primary = "10000000-0000-0000-0000-000000000002"
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: primary, InspectionTypeID: types[inspection.InspectionTypeAttendance], ClassID: rf("06")}}}
		actor := inspection.ManualActor{ID: rf("01"), IsMember: true, PrimaryDepartmentID: primary}
		input := makeInput(inspection.InspectionTypeAttendance, map[string]any{"remark": "member attribution"})
		input.DepartmentID = "10000000-0000-0000-0000-000000000003"
		if _, err := svc.Create(ctx, actor, input, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
			t.Fatalf("member chose another department: %v", err)
		}
		input.DepartmentID = ""
		created, err := svc.Create(ctx, actor, input, audit.Metadata{})
		if err != nil || created.DepartmentID != primary {
			t.Fatalf("member auto department: %+v %v", created, err)
		}
		input.DepartmentID = "10000000-0000-0000-0000-000000000003"
		if _, err := svc.Update(ctx, actor, created.ID, inspection.InspectionEdit{Version: created.RowVersion, Reason: "核对", Record: input}, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
			t.Fatalf("member reassigned draft: %v", err)
		}
		if err := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: created.ID, Version: created.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
	})
	for _, def := range catalog {
		t.Run(def.Code, func(t *testing.T) {
			input := makeInput(def.Code, nil)
			r, e := departmentSvc.Create(ctx, departmentActor, input, audit.Metadata{})
			if e != nil {
				t.Fatal("create", e)
			}
			if _, e = departmentSvc.Create(ctx, departmentActor, input, audit.Metadata{}); !errors.Is(e, inspection.ErrAttendanceDuplicate) {
				t.Fatal("duplicate", e)
			}
			read, e := svc.Detail(ctx, admin, r.ID)
			if e != nil || read.TypeCode != def.Code || read.Details == nil {
				t.Fatal("read", e)
			}
			if def.Code == inspection.InspectionTypeEveningSelfStudy {
				details, ok := read.Details.(*inspection.EveningSelfStudyDetails)
				if !ok || details.Period != "9-10" || details.PenaltyMultiplier == nil || *details.PenaltyMultiplier != 1 || details.PenaltyCount == nil || *details.PenaltyCount != 1 {
					t.Fatalf("server-derived evening rule or penalty snapshot missing: %#v", read.Details)
				}
				if read.CurrentCourseName != "" {
					t.Fatalf("evening self-study unexpectedly has a course: %#v", read)
				}
				if _, err := departmentSvc.Create(ctx, departmentActor, makeInput(def.Code, map[string]any{"period": "8-9"}), audit.Metadata{}); err == nil {
					t.Fatal("client supplied fixed evening period")
				}
				if _, err := departmentSvc.Create(ctx, departmentActor, makeInput(def.Code, map[string]any{"penaltyCount": 99}), audit.Metadata{}); err == nil {
					t.Fatal("client supplied server-owned evening count")
				}
			}
			if def.Code == inspection.InspectionTypeUnattended {
				if _, err := pool.Exec(ctx, "UPDATE students SET name='Renamed student' WHERE id=$1", rf("07")); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := pool.Exec(ctx, "UPDATE students SET name='Synthetic student' WHERE id=$1", rf("07")); err != nil {
						t.Error(err)
					}
				}()
				read, e = svc.Detail(ctx, admin, r.ID)
				if e != nil {
					t.Fatal(e)
				}
				page, err := svc.List(ctx, admin, inspection.InspectionFilter{InspectionTypeID: r.InspectionTypeID})
				if err != nil || len(page.Items) != 1 {
					t.Fatal("snapshot list", err)
				}
				for _, record := range []inspection.InspectionRecord{r, read, page.Items[0]} {
					if record.StudentName != "Synthetic student" || record.StudentNumber != "S-FIXTURE" || record.ClassName != "Fixture class" || record.Grade != "2026" {
						t.Fatal("create/detail/list must expose persisted student/class snapshots")
					}
				}
			}

			page, e := svc.List(ctx, admin, inspection.InspectionFilter{InspectionTypeID: types[def.Code], Page: 1, PageSize: 1})
			if e != nil || len(page.Items) != 1 || page.Total < 1 {
				t.Fatal("list", e)
			}
			assertCurrentLabels := func(record inspection.InspectionRecord) {
				t.Helper()
				raw, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err = json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				course, classroom, dormitory := "", "", ""
				if def.Code == inspection.InspectionTypeAttendance || def.Code == inspection.InspectionTypeUnattended {
					course = "Fixture course"
					classroom = "Fixture teaching building 101"
				}
				if def.Code == inspection.InspectionTypeEveningSelfStudy {
					classroom = "Fixture teaching building 101"
				}
				if def.Code == inspection.InspectionTypeClassroomHygiene {
					classroom = "Fixture teaching building 101"
				}
				housing := def.Code == inspection.InspectionTypeLateReturn || def.Code == inspection.InspectionTypeDormitoryHygiene || def.Code == inspection.InspectionTypePublicDormitoryHygiene
				if housing {
					dormitory = "Fixture dormitory area Fixture dormitory building 101"
				}
				if fields["currentDormitoryName"] != dormitory {
					t.Fatal("current dormitory label missing or incorrect", fields["currentDormitoryName"])
				}
				// Housing entry preselects area -> building by these identifiers; public
				// dormitory hygiene resolves them from its responsible room.
				areaID, buildingID := "", ""
				if housing {
					areaID, buildingID = rf("10"), rf("11")
				}
				if gotArea, _ := fields["currentDormitoryAreaId"].(string); gotArea != areaID {
					t.Fatal("current dormitory area missing or incorrect", fields["currentDormitoryAreaId"])
				}
				if gotBuilding, _ := fields["currentDormitoryBuildingId"].(string); gotBuilding != buildingID {
					t.Fatal("current dormitory building missing or incorrect", fields["currentDormitoryBuildingId"])
				}
				if fields["canEdit"] != true {
					t.Fatal("editable draft capability missing")
				}
				if fields["currentCourseName"] != course || fields["currentClassroomName"] != classroom {
					t.Fatal("current reference labels missing or incorrect")
				}
			}
			for _, record := range []inspection.InspectionRecord{r, read, page.Items[0]} {
				assertCurrentLabels(record)
			}
			input = makeInput(def.Code, map[string]any{"remark": "corrected"})
			if def.Code == inspection.InspectionTypeEveningSelfStudy {
				if _, e = pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=2 WHERE id=$1`, rf("04")); e != nil {
					t.Fatal(e)
				}
			}
			updated, e := svc.Update(ctx, admin, r.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: input}, audit.Metadata{})
			if e != nil || updated.RowVersion != 2 {
				t.Fatal("update", e)
			}
			if def.Code == inspection.InspectionTypeEveningSelfStudy {
				details, ok := updated.Details.(*inspection.EveningSelfStudyDetails)
				if _, restoreErr := pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=1 WHERE id=$1`, rf("04")); restoreErr != nil {
					t.Fatal(restoreErr)
				}
				if !ok || details.PenaltyMultiplier == nil || *details.PenaltyMultiplier != 2 || details.PenaltyCount == nil || *details.PenaltyCount != 2 {
					t.Fatalf("editable draft did not use the current week penalty configuration: %#v", updated.Details)
				}
			}
			assertCurrentLabels(updated)
			if def.Code == inspection.InspectionTypeUnattended {
				if _, err := pool.Exec(ctx, "UPDATE courses SET name='Renamed course',enabled=false WHERE id=$1", rf("13")); err != nil {
					t.Fatal(err)
				}
				renamed, err := svc.Detail(ctx, admin, r.ID)
				if err != nil || renamed.CurrentCourseName != "Renamed course" {
					t.Fatal("current labels must reflect renamed disabled references", err)
				}
				if _, err := pool.Exec(ctx, "UPDATE courses SET name='Fixture course',enabled=true WHERE id=$1", rf("13")); err != nil {
					t.Fatal(err)
				}
			}

			if def.Code == inspection.InspectionTypeLateReturn {
				if _, err := pool.Exec(ctx, "UPDATE dormitories SET room_no='102',enabled=false WHERE id=$1", rf("12")); err != nil {
					t.Fatal(err)
				}
				renamed, err := svc.Detail(ctx, admin, r.ID)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(renamed)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err = json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				if fields["currentDormitoryName"] != "Fixture dormitory area Fixture dormitory building 102" {
					t.Fatal("disabled dormitory current label missing")
				}
				perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: rf("16")}}}
				if _, err = svc.Detail(ctx, member, r.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
					t.Fatal("dormitory label leaked across scope", err)
				}
				if _, err = pool.Exec(ctx, "UPDATE dormitories SET room_no='101',enabled=true WHERE id=$1", rf("12")); err != nil {
					t.Fatal(err)
				}
			}

			if _, e = svc.Update(ctx, admin, r.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: input}, audit.Metadata{}); !errors.Is(e, inspection.ErrRecordVersionConflict) {
				t.Fatal("stale", e)
			}
			if e = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: r.ID, Version: 2}}, Reason: "重复"}, audit.Metadata{}); e != nil {
				t.Fatal("delete", e)
			}
			if _, e = svc.Detail(ctx, admin, r.ID); !errors.Is(e, inspection.ErrRecordNotFound) {
				t.Fatal("deleted visible", e)
			}
			recreated, recreateErr := departmentSvc.Create(ctx, departmentActor, input, audit.Metadata{})
			if recreateErr != nil {
				t.Fatal("recreate", e)
			}
			var auditText string
			if e = pool.QueryRow(ctx, `SELECT string_agg(after_json::text,' ') FROM audit_logs WHERE resource_id=$1`, r.ID).Scan(&auditText); e != nil {
				t.Fatal(e)
			}
			if strings.Contains(auditText, "currentDormitoryName") || strings.Contains(auditText, "Fixture dormitory") || strings.Contains(auditText, "Synthetic private text") || strings.Contains(auditText, "corrected") {
				t.Fatal("audit leaked text")
			}
			workflowAccess := &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID, InspectionTypeID: recreated.InspectionTypeID}}}}
			batchService := workflow.NewService(db, workflowAccess)
			batch, submitErr := batchService.Submit(ctx, workflow.Actor{ID: departmentActor.ID}, workflow.SubmitInput{
				Scope:   workflow.Scope{SemesterID: recreated.SemesterID, WeekID: recreated.WeekID, DepartmentID: recreated.DepartmentID, InspectionTypeID: recreated.InspectionTypeID},
				Records: []workflow.VersionedRecord{{ID: recreated.ID, Version: recreated.RowVersion}}, IdempotencyKey: "returned-edit-" + def.Code,
			}, audit.Metadata{})
			if submitErr != nil {
				t.Fatalf("submit %s: %v", def.Code, submitErr)
			}
			optionService := inspection.NewContextOptionService(db, &recordPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID}}}})
			if _, pendingErr := optionService.ContextOptions(ctx, departmentActor, inspection.ContextOptionQuery{RecordID: recreated.ID, TypeCode: def.Code, Purpose: "entry", Kind: "semesters"}); !errors.Is(pendingErr, inspection.ErrRecordNotEditable) {
				t.Fatalf("pending %s options exposed: %v", def.Code, pendingErr)
			}
			workflowAccess.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionReview, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID, InspectionTypeID: recreated.InspectionTypeID}}}
			returned, returnErr := batchService.Decide(ctx, workflow.Actor{ID: rf("24")}, batch.ID, workflow.Return, workflow.DecisionInput{Version: batch.RowVersion, Reason: "请核对"}, audit.Metadata{})
			if returnErr != nil || returned.Status != workflow.Returned {
				t.Fatalf("return %s: %+v %v", def.Code, returned, returnErr)
			}
			current, readErr := departmentSvc.Detail(ctx, departmentActor, recreated.ID)
			if readErr != nil || !current.CanEdit || current.Status != inspection.RecordStatusReturned {
				t.Fatalf("returned detail %s: %+v %v", def.Code, current, readErr)
			}
			for _, kind := range []string{"semesters", "weeks", "departments"} {
				query := inspection.ContextOptionQuery{RecordID: current.ID, TypeCode: def.Code, Purpose: "entry", Kind: kind, SemesterID: current.SemesterID, WeekID: current.WeekID, DepartmentID: current.DepartmentID}
				choices, optionErr := optionService.ContextOptions(ctx, departmentActor, query)
				if optionErr != nil || len(choices.Items) == 0 {
					t.Fatalf("returned %s %s options: %+v %v", def.Code, kind, choices, optionErr)
				}
			}
			if _, scopeErr := optionService.ContextOptions(ctx, departmentActor, inspection.ContextOptionQuery{RecordID: current.ID, TypeCode: def.Code, Purpose: "entry", Kind: "weeks", SemesterID: current.SemesterID, WeekID: rf("99")}); !errors.Is(scopeErr, inspection.ErrScopeMismatch) {
				t.Fatalf("returned %s accepted another week: %v", def.Code, scopeErr)
			}
			optionPermissions := &recordPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentActor.PrimaryDepartmentID}}}}
			entryService := inspection.NewEntryOptionService(db, optionPermissions)
			entryQuery := inspection.EntryOptionQuery{RecordID: current.ID, SemesterID: current.SemesterID, WeekID: current.WeekID, DepartmentID: current.DepartmentID, InspectionTypeID: current.InspectionTypeID, Kind: "classes"}
			entryChoices, entryErr := entryService.EntryOptions(ctx, departmentActor, entryQuery)
			if entryErr != nil || len(entryChoices.Items) == 0 {
				t.Fatalf("returned %s class options: %+v %v", def.Code, entryChoices, entryErr)
			}
			optionPermissions.rules = nil
			if _, deniedErr := entryService.EntryOptions(ctx, departmentActor, entryQuery); !errors.Is(deniedErr, inspection.ErrRecordNotFound) {
				t.Fatalf("returned %s options exposed without grant: %v", def.Code, deniedErr)
			}
			corrected, correctionErr := departmentSvc.Update(ctx, departmentActor, current.ID, inspection.InspectionEdit{Version: current.RowVersion, Reason: "退回更正", Record: makeInput(def.Code, map[string]any{"remark": "revised"})}, audit.Metadata{})
			if correctionErr != nil || corrected.RowVersion != current.RowVersion+1 {
				t.Fatalf("correct returned %s: %+v %v", def.Code, corrected, correctionErr)
			}
		})
	}
	t.Run("hygiene cannot be re-inspected by assigning another student", func(t *testing.T) {
		for _, scenario := range []struct {
			code     string
			location map[string]any
		}{
			{inspection.InspectionTypeDormitoryHygiene, map[string]any{"dormitoryId": rf("12")}},
			{inspection.InspectionTypeClassroomHygiene, map[string]any{"classroomId": rf("09")}},
			{inspection.InspectionTypePublicDormitoryHygiene, map[string]any{"responsibleDormitoryIds": []string{rf("12")}}},
		} {
			firstExtra := map[string]any{"date": "2026-09-05"}
			for key, value := range scenario.location {
				firstExtra[key] = value
			}
			first, err := departmentSvc.Create(ctx, departmentActor, makeInput(scenario.code, firstExtra), audit.Metadata{})
			if err != nil {
				t.Fatal("first inspection", scenario.code, err)
			}
			secondExtra := map[string]any{"date": "2026-09-05", "responsibleClassIds": []string{rf("16")}, "responsibleStudentIds": []string{rf("23")}}
			for key, value := range scenario.location {
				secondExtra[key] = value
			}
			if _, err = departmentSvc.Create(ctx, departmentActor, makeInput(scenario.code, secondExtra), audit.Metadata{}); !errors.Is(err, inspection.ErrAttendanceDuplicate) {
				t.Fatalf("same object/date accepted with another responsible student (%s): %v", scenario.code, err)
			}
			if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: first.ID, Version: first.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
				t.Fatal("cleanup", err)
			}
		}
	})
	t.Run("administrator records stay outside department workflows", func(t *testing.T) {
		input := makeInput(inspection.InspectionTypeAttendance, map[string]any{"date": "2026-09-06", "remark": "admin independent"})
		input.DepartmentID = ""
		adminRecord, err := svc.Create(ctx, admin, input, audit.Metadata{})
		if err != nil || adminRecord.AttributionMode != inspection.AttributionModeAdministrator || adminRecord.DepartmentID != "" {
			t.Fatalf("administrator attribution: %+v %v", adminRecord, err)
		}
		attendancePage, err := db.ListAttendance(ctx, inspection.AttendanceListQuery{
			SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: adminRecord.InspectionTypeID,
			DepartmentID: inspection.CollegeDepartmentOptionID, Sort: inspection.SortOccurredAtDescending, Page: 1, PageSize: 20,
		})
		if err != nil || attendancePage.Total != 1 || len(attendancePage.Items) != 1 {
			t.Fatalf("administrator attendance list: %+v %v", attendancePage, err)
		}
		listedAdminRecord := attendancePage.Items[0]
		if listedAdminRecord.ID != adminRecord.ID || listedAdminRecord.CreatedByName != "Schema fixture" ||
			listedAdminRecord.SubmitterName != "Schema fixture" || listedAdminRecord.SubmittedByName != "" ||
			listedAdminRecord.SubmittedAt != nil || !listedAdminRecord.CanDelete {
			t.Fatalf("attendance provenance or draft deletion capability: %+v", listedAdminRecord)
		}
		memberActor := inspection.ManualActor{ID: rf("24"), IsMember: true, PrimaryDepartmentID: "10000000-0000-0000-0000-000000000002"}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: adminRecord.InspectionTypeID, ClassID: rf("06")}}, {Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: adminRecord.InspectionTypeID, ClassID: rf("06")}}}
		visible, detailErr := svc.Detail(ctx, memberActor, adminRecord.ID)
		if detailErr != nil || visible.CanEdit || visible.CanDelete || visible.CreatedByName != "Schema fixture" || visible.SubmitterName != "Schema fixture" || visible.SubmittedByName != "" {
			t.Fatalf("business-scoped member must be able to view, but not mutate, another creator's record: %+v %v", visible, detailErr)
		}
		page, err := svc.List(ctx, memberActor, inspection.InspectionFilter{InspectionTypeID: adminRecord.InspectionTypeID, SubmissionCandidatesOnly: true})
		if err != nil {
			t.Fatal("department submission candidates", err)
		}
		for _, record := range page.Items {
			if record.ID == adminRecord.ID {
				t.Fatal("administrator record entered department submission candidates")
			}
		}
		manager := inspection.ManualActor{ID: rf("24")}
		perms.rules = []permission.EffectivePermission{
			{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: adminRecord.InspectionTypeID}},
			{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: adminRecord.InspectionTypeID}},
		}
		managed, err := svc.Detail(ctx, manager, adminRecord.ID)
		if err != nil || !managed.CanEdit || !managed.CanDelete {
			t.Fatalf("business manager capabilities for independent record: %+v %v", managed, err)
		}
		managerEdit := makeInput(inspection.InspectionTypeAttendance, map[string]any{
			"date": "2026-09-06", "remark": "manager corrected independent record",
		})
		managerEdit.DepartmentID = ""
		updated, err := svc.Update(ctx, manager, adminRecord.ID, inspection.InspectionEdit{
			Version: managed.RowVersion, Reason: "manager correction", Record: managerEdit,
		}, audit.Metadata{})
		if err != nil || updated.CreatedBy != adminRecord.CreatedBy || updated.AttributionMode != inspection.AttributionModeAdministrator {
			t.Fatalf("manager edit must preserve independent ownership: %+v %v", updated, err)
		}
		attendanceAccess := inspection.MutationAccess{ActorID: manager.ID, Permissions: perms.rules}
		attendanceDraft, err := db.ReadAttendanceForMutation(ctx, updated.ID, attendanceAccess)
		if err != nil {
			t.Fatal("attendance manager could not read an independent draft for mutation", err)
		}
		attendanceDraft.Remark = "attendance manager corrected independent record"
		attendanceUpdated, err := db.UpdateAttendance(ctx, attendanceDraft, attendanceDraft.RowVersion, attendanceAccess, audit.Mutation{
			Metadata: audit.Metadata{ActorID: manager.ID},
			Entry:    audit.Entry{Action: audit.ActionInspectionRecordUpdated, ResourceType: "inspectionRecord", ResourceID: updated.ID},
		})
		if err != nil || attendanceUpdated.RowVersion != updated.RowVersion+1 {
			t.Fatalf("attendance manager edit: %+v %v", attendanceUpdated, err)
		}
		if err = db.DeleteAttendance(ctx, []inspection.VersionedRecord{{ID: attendanceUpdated.ID, Version: attendanceUpdated.RowVersion}}, attendanceAccess, audit.Metadata{ActorID: manager.ID}); err != nil {
			t.Fatal("attendance manager could not delete an eligible independent draft", err)
		}
		if _, err = svc.Detail(ctx, admin, adminRecord.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
			t.Fatal("manager deletion did not remove independent draft", err)
		}
		perms.rules = nil
	})
	allow := func(action, class string) permission.EffectivePermission {
		return permission.EffectivePermission{Action: action, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: class}}
	}
	hygiene := makeInput(inspection.InspectionTypeDormitoryHygiene, map[string]any{"date": "2026-09-06", "responsibleClassIds": []string{rf("16")}, "responsibleStudentIds": []string{rf("23")}})
	cross, err := departmentSvc.Create(ctx, departmentActor, hygiene, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	perms.rules = []permission.EffectivePermission{allow(permission.ActionInspectionView, rf("06")), allow(permission.ActionInspectionEnter, rf("06"))}
	if _, err = svc.Detail(ctx, member, cross.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("unrelated responsibility read", err)
	}
	page, err := svc.List(ctx, member, inspection.InspectionFilter{InspectionTypeID: types[inspection.InspectionTypeDormitoryHygiene]})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Items {
		if r.ID == cross.ID {
			t.Fatal("unrelated responsibility list leaked")
		}
	}
	if _, err = svc.Update(ctx, member, cross.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: hygiene}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("unrelated responsibility edit", err)
	}
	if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: cross.ID, Version: cross.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
		t.Fatal("cleanup cross-scope fixture", err)
	}
	ownInput := makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-04"})
	own, err := svc.Create(ctx, member, ownInput, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	outside := makeInput(inspection.InspectionTypeMorning, map[string]any{"classId": rf("16"), "date": "2026-09-04"})
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: outside}, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
		t.Fatal("target scope", err)
	}
	// One inaccessible item must not delete the authorized item in a batch.
	err = svc.Delete(ctx, member, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 1}, {ID: cross.ID, Version: 1}}, Reason: "核对"}, audit.Metadata{})
	if !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("batch permission", err)
	}
	if _, err = svc.Detail(ctx, admin, own.ID); err != nil {
		t.Fatal("partial batch write", err)
	}
	perms.rules = []permission.EffectivePermission{allow(permission.ActionInspectionView, rf("06"))}
	visible, err := svc.Detail(ctx, member, own.ID)
	if err != nil || visible.CanEdit {
		t.Fatal("view-only draft capability", err)
	}
	perms.rules = append(perms.rules, allow(permission.ActionInspectionEnter, rf("06")))
	visible, err = svc.Detail(ctx, member, own.ID)
	if err != nil || !visible.CanEdit {
		t.Fatal("own editable draft capability", err)
	}
	// Independent manager permission may mutate another creator; ENTER alone may not.
	if _, err = pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role) VALUES($1,'other-fixture','Other fixture','not-a-login-hash','ACTIVE','SYSTEM','MEMBER')`, rf("30")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE inspection_records SET created_by=$1 WHERE id=$2", rf("30"), own.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: ownInput}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("enter edits other", err)
	}
	perms.rules = []permission.EffectivePermission{allow(permission.ActionInspectionManage, rf("06"))}
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: ownInput}, audit.Metadata{}); err != nil {
		t.Fatal("manager", err)
	}
	perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionManage, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: rf("06")}})
	if err = svc.Delete(ctx, member, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 2}}, Reason: "核对"}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("deny", err)
	}
	// Two simultaneous writers: exactly one advances the version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := svc.Update(ctx, admin, own.ID, inspection.InspectionEdit{Version: 2, Reason: "并发核对", Record: ownInput}, audit.Metadata{})
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	good, conflicts := 0, 0
	for e := range results {
		if e == nil {
			good++
		} else if errors.Is(e, inspection.ErrRecordVersionConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if good != 1 || conflicts != 1 {
		t.Fatal("lost update", good, conflicts)
	}
	// Audit storage failure must roll back the record write.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_record_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture audit failure'; END $$; CREATE TRIGGER fail_record_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_record_audit();`); err != nil {
		t.Fatal(err)
	}
	failedInput := makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-04", "classId": rf("16"), "studentId": rf("23")})
	if _, e := svc.Update(ctx, admin, own.ID, inspection.InspectionEdit{Version: 3, Reason: "核对", Record: ownInput}, audit.Metadata{}); e == nil {
		t.Fatal("audit failure accepted update")
	}
	if e := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 3}}, Reason: "核对"}, audit.Metadata{}); e == nil {
		t.Fatal("audit failure accepted delete")
	}
	if r, e := svc.Detail(ctx, admin, own.ID); e != nil || r.RowVersion != 3 {
		t.Fatal("audit rollback changed record", e)
	}
	if _, err = departmentSvc.Create(ctx, departmentActor, failedInput, audit.Metadata{}); err == nil || !strings.Contains(err.Error(), "fixture audit failure") {
		t.Fatal("audit failure was not the cause of rejected create", err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_record_audit ON audit_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err = departmentSvc.Create(ctx, departmentActor, failedInput, audit.Metadata{}); err != nil {
		t.Fatal("audit failure left duplicate", err)
	}
	// A submitted record must reference a real batch with the same scope.
	submissionID := rf("30")
	if _, err = pool.Exec(ctx, `INSERT INTO department_submissions(id,semester_id,week_id,department_id,inspection_type_id,created_by,request_fingerprint)
	 VALUES($1,$2,$3,$4,$5,$6,repeat('0',64))`, submissionID, own.SemesterID, own.WeekID, own.DepartmentID, own.InspectionTypeID, own.CreatedBy); err != nil {
		t.Fatal(err)
	}
	reportLockID := rf("31")
	if _, err = pool.Exec(ctx, `INSERT INTO weekly_reports(id,semester_id,week_id,created_by,updated_by)
VALUES($1,$2,$3,$4,$4)`, reportLockID, own.SemesterID, own.WeekID, admin.ID); err != nil {
		t.Fatal("create report lock fixture:", err)
	}
	for _, column := range []string{"status='CONFIRMED'", "status='RETURNED'", "status='LOCKED'", "status='DRAFT',submission_id='" + submissionID + "'", "status='DRAFT',submission_id=NULL,locked_by_report_id='" + reportLockID + "'"} {
		if _, err = pool.Exec(ctx, "UPDATE inspection_records SET "+column+" WHERE id=$1", own.ID); err != nil {
			t.Fatal(err)
		}
		protected, e := svc.Detail(ctx, admin, own.ID)
		if e != nil || protected.CanEdit {
			t.Fatal("protected record advertised as editable", e)
		}
		if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 3}}, Reason: "核对"}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotEditable) {
			t.Fatal("protected record", err)
		}
	}
	t.Run("college record is independent of department", func(t *testing.T) {
		input := makeInput(inspection.InspectionTypeAttendance, map[string]any{"date": "2026-09-07", "classId": rf("16"), "studentId": rf("23")})
		input.DepartmentID = inspection.CollegeDepartmentOptionID
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "10000000-0000-0000-0000-000000000002", InspectionTypeID: input.InspectionTypeID, ClassID: rf("16")}}}
		if _, err := svc.Create(ctx, member, input, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
			t.Fatalf("department grant created college record: %v", err)
		}
		perms.rules[0].Scope.DepartmentID = ""
		record, err := svc.Create(ctx, member, input, audit.Metadata{})
		if err != nil || record.DepartmentID != "" {
			t.Fatalf("college create: %+v %v", record, err)
		}
		var storedNull bool
		if err := pool.QueryRow(ctx, `SELECT department_id IS NULL FROM inspection_records WHERE id=$1`, record.ID).Scan(&storedNull); err != nil || !storedNull {
			t.Fatalf("college department persistence: %v %v", storedNull, err)
		}
		perms.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "10000000-0000-0000-0000-000000000002", InspectionTypeID: input.InspectionTypeID, ClassID: rf("16")}}}
		if _, err := svc.Detail(ctx, member, record.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
			t.Fatalf("department-scoped view reached college record: %v", err)
		}
		perms.rules[0].Scope.DepartmentID = ""
		page, err := svc.List(ctx, member, inspection.InspectionFilter{DepartmentID: inspection.CollegeDepartmentOptionID, InspectionTypeID: input.InspectionTypeID})
		if err != nil || page.Total != 1 || page.Items[0].ID != record.ID {
			t.Fatalf("college-only list: %+v %v", page, err)
		}
	})
	t.Run("account display assignment", func(t *testing.T) {
		departmentID := "10000000-0000-0000-0000-000000000002"
		if _, err := pool.Exec(ctx, `UPDATE accounts SET primary_department_id=$2 WHERE id=$1`, rf("01"), departmentID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id)
			VALUES($1,$2,$3,'20000000-0000-0000-0000-000000000004')`, rf("31"), rf("01"), departmentID); err != nil {
			t.Fatal(err)
		}
		account, err := db.FindByID(ctx, rf("01"))
		if err != nil || account.PrimaryDepartmentName != "学习部" || account.OrganizationRoleName != "部门委员" {
			t.Fatalf("account assignment names: %+v %v", account, err)
		}
	})
	t.Run("morning schedule default and audited update", func(t *testing.T) {
		service := inspection.NewMorningScheduleService(db, perms)
		before, err := service.Get(ctx, admin, rf("04"))
		wantDefault := []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-07"}
		if err != nil || !before.IsDefault || before.Version != 0 || !reflect.DeepEqual(before.PlannedDates, wantDefault) {
			t.Fatalf("default morning schedule=%+v err=%v", before, err)
		}
		updated, err := service.Save(ctx, admin, rf("04"), before.Version, []string{"2026-09-06"}, audit.Metadata{})
		wantUpdated := []string{"2026-09-06"}
		if err != nil || updated.IsDefault || updated.Version != 1 || !reflect.DeepEqual(updated.PlannedDates, wantUpdated) || updated.UpdatedBy != admin.ID {
			t.Fatalf("updated morning schedule=%+v err=%v", updated, err)
		}
		var auditCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='MORNING_SCHEDULE_CHANGED'", rf("04")).Scan(&auditCount); err != nil || auditCount != 1 {
			t.Fatalf("morning schedule audit count=%d err=%v", auditCount, err)
		}
		manualRecord, err := departmentSvc.Create(ctx, departmentActor, makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-06"}), audit.Metadata{})
		if err != nil {
			t.Fatalf("manual morning entry did not use the adjusted Sunday plan: %v", err)
		}
		if err := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: manualRecord.ID, Version: manualRecord.RowVersion}}, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("other hygiene reason database guard", func(t *testing.T) {
		for _, table := range []string{"dorm_hygiene_details", "classroom_hygiene_details", "public_dormitory_hygiene_details"} {
			var recordID string
			if err := pool.QueryRow(ctx, "SELECT record_id FROM "+table+" LIMIT 1").Scan(&recordID); err != nil {
				t.Fatal(table, err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, "UPDATE "+table+" SET reason_code='OTHER',description='  ' WHERE record_id=$1", recordID); err == nil {
				t.Fatal(table, "accepted blank other reason")
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			tx, err = pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, "UPDATE "+table+" SET reason_code='OTHER',description='门框积灰' WHERE record_id=$1", recordID); err != nil {
				t.Fatal(table, err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("student number sort uses snapshots and preserves time default", func(t *testing.T) {
		firstInput := makeInput(inspection.InspectionTypeAttendance, map[string]any{
			"date": "2026-09-05", "studentId": rf("07"), "classId": rf("06"),
		})
		firstInput.DepartmentID = ""
		first, err := svc.Create(ctx, admin, firstInput, audit.Metadata{})
		if err != nil {
			t.Fatal("create first sortable record", err)
		}
		secondInput := makeInput(inspection.InspectionTypeAttendance, map[string]any{
			"date": "2026-09-06", "studentId": rf("23"), "classId": rf("16"),
		})
		secondInput.DepartmentID = ""
		second, err := svc.Create(ctx, admin, secondInput, audit.Metadata{})
		if err != nil {
			t.Fatal("create second sortable record", err)
		}
		defer func() {
			items := []inspection.VersionedRecord{{ID: first.ID, Version: first.RowVersion}, {ID: second.ID, Version: second.RowVersion}}
			if err := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: items, Reason: "fixture cleanup"}, audit.Metadata{}); err != nil {
				t.Errorf("delete sortable records: %v", err)
			}
		}()

		// Exercise the legacy fallback used when only the responsibility relation has a student snapshot.
		if _, err := pool.Exec(ctx, `UPDATE inspection_records SET student_no_snapshot='' WHERE id=$1`, first.ID); err != nil {
			t.Fatal("clear direct student snapshot", err)
		}
		assertRelativeOrder := func(sort, before, after string) {
			t.Helper()
			page, err := svc.List(ctx, admin, inspection.InspectionFilter{
				InspectionTypeID: types[inspection.InspectionTypeAttendance], Sort: sort, Page: 1, PageSize: 100,
			})
			if err != nil {
				t.Fatalf("list sorted records (%s): %v", sort, err)
			}
			positions := map[string]int{}
			for index, record := range page.Items {
				if record.ID == first.ID || record.ID == second.ID {
					positions[record.ID] = index
				}
			}
			if len(positions) != 2 || positions[before] >= positions[after] {
				t.Fatalf("sort %q order = %v, want %s before %s", sort, positions, before, after)
			}
		}
		assertRelativeOrder(inspection.SortStudentNoAscending, first.ID, second.ID)
		assertRelativeOrder(inspection.SortStudentNoDescending, second.ID, first.ID)
		assertRelativeOrder(inspection.SortOccurredAtDescending, second.ID, first.ID)
	})
}

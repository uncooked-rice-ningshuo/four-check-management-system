package store

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

func TestTeacherAssignedTemplatesPostgres(t *testing.T) {
	db := openTestPostgres(t)
	ctx := context.Background()
	study := uuid.NewString()
	attendance := "30000000-0000-0000-0000-000000000001"
	var teacherRole string
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM organization_roles WHERE code='TEACHER'`).Scan(&teacherRole); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO departments(id,code,name) VALUES($1,$2,$3)`, study, "TEST_"+study[:8], "虚构权限测试部门"+study); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,can_enter,can_view,can_review,can_manage,enabled) VALUES($1,$2,true,true,true,true,true)`, study, attendance); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO department_inspection_role_defaults(department_id,inspection_type_id,role_code,can_view) VALUES($1,$2,'TEACHER',true)`, study, attendance); err != nil {
		t.Fatal(err)
	}
	newAccount := func() string {
		id := uuid.NewString()
		if _, err := db.Pool.Exec(ctx, `INSERT INTO accounts(id,student_no,login_name,name,password_hash,status,system_role) VALUES($1,$2,$2,'虚构老师','test-only','ACTIVE','MEMBER')`, id, "t"+id[:8]); err != nil {
			t.Fatal(err)
		}
		return id
	}
	assign := func(account string) {
		t.Helper()
		if err := db.AssignUserRole(ctx, organization.UserAssignment{AccountID: account, DepartmentID: study, DepartmentIDs: []string{study}, OrganizationRoleID: teacherRole}, audit.Mutation{Entry: audit.Entry{Action: audit.ActionOrganizationUpdated, ResourceType: "userAssignment", ResourceID: account}}); err != nil {
			t.Fatal(err)
		}
	}
	first := newAccount()
	assign(first)
	service := permission.NewService(db)
	before, err := service.Effective(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !before.HasAnyAllowed(permission.ActionInspectionView) || before.HasAnyAllowed(permission.ActionInspectionEnter) {
		t.Fatalf("teacher initial permissions incorrect: %+v", before)
	}
	config, err := db.GetDepartmentDefaults(ctx, study)
	if err != nil {
		t.Fatal(err)
	}
	for i := range config.InspectionRoles {
		if config.InspectionRoles[i].RoleCode == "TEACHER" && config.InspectionRoles[i].InspectionTypeID == attendance {
			config.InspectionRoles[i].CanEnter = true
		}
	}
	for i := range config.RolePolicies {
		if config.RolePolicies[i].RoleCode == "TEACHER" {
			config.RolePolicies[i].ValidityYears = 2
		}
	}
	if _, err := organization.NewService(db).SaveDepartmentDefaults(ctx, config, audit.Metadata{}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := service.Effective(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.HasAnyAllowed(permission.ActionInspectionEnter) {
		t.Fatal("template edit rewrote existing teacher")
	}
	second := newAccount()
	assign(second)
	effective, err := service.Effective(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if !effective.HasAnyAllowed(permission.ActionInspectionEnter) {
		t.Fatal("new assignment missed new template")
	}
	var start time.Time
	var end *time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT start_at,end_at FROM user_org_roles WHERE account_id=$1`, second).Scan(&start, &end); err != nil {
		t.Fatal(err)
	}
	if end == nil || !end.Equal(start.AddDate(2, 0, 0)) {
		t.Fatal("configured duration not applied")
	}
	past := time.Now().Add(-time.Hour)
	rule, err := service.CreateRule(ctx, second, permission.Rule{AccountID: second, Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Mode: "OVERRIDE", SourceDepartmentID: study, Scope: permission.Scope{InspectionTypeID: attendance}, StartsAt: past.Add(-time.Hour), EndsAt: &past, Note: "过期覆盖回归"})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := service.Effective(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if expired.HasAnyAllowed(permission.ActionInspectionEnter) {
		t.Fatal("expired override fell back to broad template")
	}
	if err := service.DeleteRule(ctx, second, rule.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := service.Effective(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.HasAnyAllowed(permission.ActionInspectionEnter) {
		t.Fatal("restore did not recover assigned template")
	}
	personal, err := service.CreateRule(ctx, second, permission.Rule{AccountID: second, Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Mode: "ADD", SourceDepartmentID: study, Scope: permission.Scope{InspectionTypeID: attendance}, StartsAt: past, Note: "角色变更处理"})
	if err != nil {
		t.Fatal(err)
	}
	var minister string
	if err := db.Pool.QueryRow(ctx, "SELECT id FROM organization_roles WHERE code='MINISTER'").Scan(&minister); err != nil {
		t.Fatal(err)
	}
	change := organization.UserAssignment{AccountID: second, DepartmentID: study, DepartmentIDs: []string{study}, OrganizationRoleID: minister}
	if err := db.Pool.QueryRow(ctx, "SELECT session_version FROM accounts WHERE id=$1", second).Scan(&change.SessionVersion); err != nil {
		t.Fatal(err)
	}
	mutation := audit.Mutation{Entry: audit.Entry{Action: audit.ActionOrganizationUpdated, ResourceType: "userAssignment", ResourceID: second}}
	if err := db.AssignUserRole(ctx, change, mutation); err == nil {
		t.Fatal("role change omitted explicit personal decisions")
	}
	// The failed role change adds no fifth message. Updating the template sent no
	// historical notification to first. Reading another recipient's row fails.
	stats := func(account string) (int, string) {
		t.Helper()
		result, err := db.InboxPage(ctx, account, 1, 20, false, "")
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Unread  int
			Through string
		}
		if err := json.Unmarshal(result, &page); err != nil {
			t.Fatal(err)
		}
		return page.Unread, page.Through
	}
	unreadCount, last := stats(second)
	if unreadCount != 4 {
		t.Fatalf("recipient unread=%d", unreadCount)
	}
	if n, _ := stats(first); n != 1 {
		t.Fatalf("historical recipient unread=%d", n)
	}
	id, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := db.ReadInbox(ctx, first, id, false); err != nil || found {
		t.Fatalf("foreign read: %v %v", found, err)
	}
	if _, err := db.ReadInbox(ctx, second, id, true); err != nil {
		t.Fatal(err)
	}
	if n, _ := stats(second); n != 0 {
		t.Fatalf("bulk read left %d", n)
	}
	change.PersonalRuleDecisions = map[string]bool{personal.ID: true}
	if err := db.AssignUserRole(ctx, change, mutation); err != nil {
		t.Fatal("keep personal rule on role change", err)
	}
	// Replaying a bulk-read request must leave messages created afterwards unread.
	if _, err := db.ReadInbox(ctx, second, id, true); err != nil {
		t.Fatal(err)
	}
	if n, _ := stats(second); n != 1 {
		t.Fatalf("new notification lost after bulk-read retry: %d", n)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM user_permission_rules WHERE id=$1", personal.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("kept rule missing", err)
	}
	if err := db.AssignUserRole(ctx, change, mutation); err == nil {
		t.Fatal("stale assignment accepted")
	}
	change.OrganizationRoleID = teacherRole
	change.PersonalRuleDecisions[personal.ID] = false
	if err := db.Pool.QueryRow(ctx, "SELECT session_version FROM accounts WHERE id=$1", second).Scan(&change.SessionVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.AssignUserRole(ctx, change, mutation); err != nil {
		t.Fatal("clear personal rule on role change", err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM user_permission_rules WHERE id=$1", personal.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("cleared rule survived", err)
	}

}

func TestTeacherRouteRangesPostgres(t *testing.T) {
	db := openTestPostgres(t)
	ctx := context.Background()
	study, yearOne, year, gradeOne, gradeTwo, major, class, record, actor, kind := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	rules := []permission.EffectivePermission{
		{Action: permission.ActionInspectionReview, Effect: permission.EffectAllow, SourceDepartmentID: study, Scope: permission.Scope{InspectionTypeID: kind}},
		{Action: permission.ActionInspectionReview, Effect: permission.EffectAllow, SourceDepartmentID: yearOne, Scope: permission.Scope{InspectionTypeID: kind}, Ranges: []permission.OperationRange{{GradeID: gradeOne}}},
	}
	// CTEs isolate reference data while executing the production permission SQL in PostgreSQL.
	prefix := `WITH classes AS (SELECT $1::uuid id,$2::uuid grade_id,$3::uuid major_id),
 academic_year_grade_stages AS (SELECT $4::uuid academic_year_id,1 stage,$5::uuid grade_id),
 sem AS (SELECT $4::uuid academic_year_id),
 ir AS (SELECT $6::uuid id,$1::uuid class_id,'test'::text grade_snapshot,$7::uuid inspection_type_id,
 $8::uuid created_by,$9::uuid department_id,NULL::uuid semester_id,NULL::uuid week_id)
 SELECT `
	for _, tc := range []struct {
		name, route, grade string
		want               bool
	}{
		{"second-year through study", study, gradeTwo, true},
		{"second-year through year-one", yearOne, gradeTwo, false},
		{"first-year through year-one", yearOne, gradeOne, true},
		{"unassigned department", uuid.NewString(), gradeOne, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []any{class, tc.grade, major, year, gradeOne, record, kind, actor, study}
			access := inspection.MutationAccess{ActorID: actor, TargetDepartmentID: tc.route, Permissions: rules}
			condition := inspectionAccessSQL(access, "review", &args)
			var allowed bool
			if err := db.Pool.QueryRow(ctx, prefix+condition+` FROM ir CROSS JOIN sem`, args...).Scan(&allowed); err != nil {
				t.Fatal(err)
			}
			if allowed != tc.want {
				t.Fatalf("allowed=%v want=%v", allowed, tc.want)
			}
		})
	}
}

func TestTemplateScopeOrderDoesNotChangeResponsibilities(t *testing.T) {
	first := organization.DepartmentInspectionScope{DepartmentID: "study", InspectionTypeID: "attendance", Enabled: true, CanEnter: true}
	second := organization.DepartmentInspectionScope{DepartmentID: "study", InspectionTypeID: "device", Enabled: true, CanView: true}
	if !sameDepartmentScopes([]organization.DepartmentInspectionScope{first, second}, []organization.DepartmentInspectionScope{second, first}) {
		t.Fatal("row order changed responsibility")
	}
	second.CanEnter = true
	if sameDepartmentScopes([]organization.DepartmentInspectionScope{first}, []organization.DepartmentInspectionScope{second}) {
		t.Fatal("duty change ignored")
	}
}

func TestUninitializedAssignmentPostgres(t *testing.T) {
	db := openTestPostgres(t)
	ctx := context.Background()
	id := uuid.NewString()
	var department, role string
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM departments WHERE code='STUDY'`).Scan(&department); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM organization_roles WHERE code='TEACHER'`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO accounts(id,student_no,login_name,name,password_hash,status,system_role) VALUES($1,$2,$2,'虚构未授予账号','test-only','ACTIVE','MEMBER')`, id, "pending-"+id[:16]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id) VALUES($1,$2,$3,$4)`, uuid.NewString(), id, department, role); err != nil {
		t.Fatal(err)
	}
	service := permission.NewService(db)
	result, err := service.Effective(ctx, id)
	if err != nil || len(result.PendingDepartmentIDs) != 1 || len(result.Permissions) != 0 {
		t.Fatalf("uninitialized account: %+v %v", result, err)
	}
	mutation := audit.Mutation{Entry: audit.Entry{Action: audit.ActionOrganizationUpdated, ResourceType: "userAssignment", ResourceID: id}}
	assignment := organization.UserAssignment{AccountID: id, DepartmentID: department, DepartmentIDs: []string{department}, OrganizationRoleID: role}
	if err := db.AssignUserRole(ctx, assignment, mutation); err != nil {
		t.Fatal(err)
	}
	result, err = service.Effective(ctx, id)
	if err != nil || len(result.PendingDepartmentIDs) != 0 {
		t.Fatal("saving department/role did not complete assignment", err)
	}
	var first time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT start_at FROM user_org_roles WHERE account_id=$1`, id).Scan(&first); err != nil {
		t.Fatal(err)
	}
	assignment.SessionVersion = 1
	if err := db.AssignUserRole(ctx, assignment, mutation); err != nil {
		t.Fatal(err)
	}
	var repeated time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT start_at FROM user_org_roles WHERE account_id=$1`, id).Scan(&repeated); err != nil || !first.Equal(repeated) {
		t.Fatal("complete grant was renewed", err)
	}
}

func TestYearMappingConsumersPostgres(t *testing.T) {
	db := openTestPostgres(t)
	ctx := context.Background()
	year, semester1, semester2, grade := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO grades(id,name) VALUES($1,$2)`, grade, "G-"+grade[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,$2,'2030-09-01','2031-08-31')`, year, "Y-"+year[:8]); err != nil {
		t.Fatal(err)
	}
	for index, id := range []string{semester1, semester2} {
		if _, err := db.Pool.Exec(ctx, `INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,$3,$3,'2030-09-01','2031-08-31')`, id, year, []string{"FIRST", "SECOND"}[index]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO academic_year_grade_stages(academic_year_id,stage,grade_id) VALUES($1,1,$2)`, year, grade); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{semester1, semester2} {
		scope, err := db.GetReportGrades(ctx, id)
		if err != nil || len(scope.GradeIDs) != 1 || scope.GradeIDs[0] != grade {
			t.Fatalf("year coverage: %+v %v", scope, err)
		}
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	entries := []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Ranges: []permission.OperationRange{{GradeID: grade}}}}
	suggested, err := suggestedInspectionFilters(ctx, tx, semester1, "", entries)
	if err != nil || suggested["grade"] != "G-"+grade[:8] {
		t.Fatalf("default fixed grade: %+v %v", suggested, err)
	}
	entries[0].Ranges = append(entries[0].Ranges, permission.OperationRange{ClassID: uuid.NewString()})
	suggested, err = suggestedInspectionFilters(ctx, tx, semester1, "", entries)
	if err != nil || len(suggested) != 0 {
		t.Fatalf("complex union misrepresented: %+v %v", suggested, err)
	}
}

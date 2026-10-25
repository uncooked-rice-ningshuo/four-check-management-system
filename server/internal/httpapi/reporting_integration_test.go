package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/store"
	"four-check-management-system/server/internal/workflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReportingHTTPWithPostgresAndEffectivePermissions(t *testing.T) {
	databaseURL := os.Getenv("REPORTING_API_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REPORTING_API_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatal("requires an empty disposable database", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		contents, readErr := os.ReadFile(migration)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := pool.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("migration %s: %v", filepath.Base(migration), execErr)
		}
	}
	seed, err := os.ReadFile("../store/testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE inspection_records
SET student_no_snapshot='S-FIXTURE',student_name_snapshot='Synthetic student',class_name_snapshot='Fixture class',grade_snapshot='2026'
WHERE id='a0000000-0000-0000-0000-000000000014'`); err != nil {
		t.Fatal("prepare synthetic attendance snapshot:", err)
	}
	database := &store.Postgres{Pool: pool}
	permissionService := permission.NewService(database)
	authService := auth.NewService(database, database, []byte("reporting-http-test-jwt-secret-32-bytes"), 5*time.Minute, time.Hour)
	admin, err := authService.BootstrapAdmin(ctx, "reporting-http-admin", "通报集成管理员", "ReportingTestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET must_change_password=false WHERE id=$1", admin.ID); err != nil {
		t.Fatal(err)
	}
	adminTokens, err := authService.Login(ctx, admin.LoginName, "ReportingTestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	collegeReaderID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO accounts(
		id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password,primary_department_id)
		SELECT $1,'reporting-college-reader','校级通报只读用户',password_hash,'ACTIVE','SYSTEM','MEMBER',false,false,NULL
		FROM accounts WHERE id=$2`, collegeReaderID, admin.ID); err != nil {
		t.Fatal("insert synthetic unassigned report reader:", err)
	}
	if _, err := permissionService.CreateRule(ctx, admin.ID, permission.Rule{
		AccountID: collegeReaderID, Action: permission.ActionReportView, Effect: permission.EffectAllow,
	}); err != nil {
		t.Fatal("grant synthetic college report reader:", err)
	}
	collegeReaderTokens, err := authService.Login(ctx, "reporting-college-reader", "ReportingTestPassword123")
	if err != nil {
		t.Fatal("login synthetic unassigned report reader:", err)
	}

	const semesterID = "a0000000-0000-0000-0000-000000000003"
	const academicYearID = "a0000000-0000-0000-0000-000000000002"
	const weekID = "a0000000-0000-0000-0000-000000000004"
	const departmentID = "10000000-0000-0000-0000-000000000002"
	const officeDepartmentID = "10000000-0000-0000-0000-000000000001"
	const inspectionTypeID = "30000000-0000-0000-0000-000000000001"
	if _, err := pool.Exec(ctx, `
INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES ('b0000000-0000-0000-0000-000000000001',$1,$2,$3,$4,'CONFIRMED',1,$5,$5,repeat('a',64),now())`,
		semesterID, weekID, departmentID, inspectionTypeID, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES ('c0000000-0000-0000-0000-000000000001','b0000000-0000-0000-0000-000000000001',1,$1,
 ARRAY['a0000000-0000-0000-0000-000000000014'::uuid],'[]'::jsonb)`, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES ('b0000000-0000-0000-0000-000000000002',$1,$2,NULL,$3,'CONFIRMED',1,$4,$4,repeat('b',64),now())`, semesterID, weekID, inspectionTypeID, admin.ID); err != nil {
		t.Fatal("insert independent college-level submission:", err)
	}
	if _, err := pool.Exec(ctx, `
WITH college_records(id,dedup_key) AS (VALUES
 ('c0000000-0000-0000-0000-000000000011'::uuid,repeat('c',64)),
 ('c0000000-0000-0000-0000-000000000012'::uuid,repeat('d',64)))
INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,
 student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,
 violation_count,remark,source,status,submission_id,dedup_key,created_by)
SELECT college_records.id,source.inspection_type_id,$1,$2,NULL,source.student_id,
 source.student_no_snapshot,source.student_name_snapshot,source.class_id,source.class_name_snapshot,
 source.grade_snapshot,source.occurred_at,source.violation_count,source.remark,source.source,
 'CONFIRMED',$3,college_records.dedup_key,source.created_by
FROM college_records CROSS JOIN inspection_records source WHERE source.id='a0000000-0000-0000-0000-000000000014'`, semesterID, weekID, "b0000000-0000-0000-0000-000000000002"); err != nil {
		t.Fatal("insert synthetic confirmed college-level records:", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id)
SELECT college_record.id,details.course_id,details.absence_hours,details.period_text,details.classroom_id
FROM (VALUES ('c0000000-0000-0000-0000-000000000011'::uuid),('c0000000-0000-0000-0000-000000000012'::uuid)) AS college_record(id)
CROSS JOIN attendance_details details WHERE details.record_id='a0000000-0000-0000-0000-000000000014'`); err != nil {
		t.Fatal("insert synthetic college-level attendance details:", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES ('c0000000-0000-0000-0000-000000000002','b0000000-0000-0000-0000-000000000002',1,$1,
 ARRAY['c0000000-0000-0000-0000-000000000011'::uuid,'c0000000-0000-0000-0000-000000000012'::uuid],'[]'::jsonb)`, admin.ID); err != nil {
		t.Fatal("insert college-level immutable revision:", err)
	}

	router := NewRouter(authService, false, AdminServices{
		Permission: permissionService,
		Reporting:  reporting.NewService(database, permissionService),
		Workflow:   workflow.NewService(database, permissionService),
	})
	if response := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated periods response = %d %s", response.Code, response.Body.String())
	}
	periodsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, adminTokens.AccessToken)
	if periodsResponse.Code != http.StatusOK {
		t.Fatalf("authorized periods response = %d %s", periodsResponse.Code, periodsResponse.Body.String())
	}
	var periodPage struct {
		Items []struct {
			SemesterID string `json:"semesterId"`
			WeekID     string `json:"weekId"`
		} `json:"items"`
	}
	if err := json.Unmarshal(periodsResponse.Body.Bytes(), &periodPage); err != nil || len(periodPage.Items) == 0 || periodPage.Items[0].SemesterID != semesterID || periodPage.Items[0].WeekID != weekID {
		t.Fatalf("periods body = %s, decode error = %v", periodsResponse.Body.String(), err)
	}
	summaryResponse := performJSON(router, http.MethodGet, "/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, adminTokens.AccessToken)
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("authorized summary response = %d %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	var summary struct {
		Rows []struct {
			DepartmentID         string `json:"departmentId"`
			CollegeLevel         bool   `json:"collegeLevel"`
			ConfirmedRecordCount int    `json:"confirmedRecordCount"`
		} `json:"rows"`
		Totals struct {
			ConfirmedRecordCount int `json:"confirmedRecordCount"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(summaryResponse.Body.Bytes(), &summary); err != nil || len(summary.Rows) == 0 || summary.Totals.ConfirmedRecordCount != 3 {
		t.Fatalf("summary body = %s, decode error = %v", summaryResponse.Body.String(), err)
	}
	if summary.Rows[0].DepartmentID != departmentID || summary.Rows[0].ConfirmedRecordCount != 1 {
		t.Fatalf("summary rows = %+v", summary.Rows)
	}
	collegeVisible := false
	for _, row := range summary.Rows {
		collegeVisible = collegeVisible || (row.CollegeLevel && row.ConfirmedRecordCount == 2)
	}
	if !collegeVisible {
		t.Fatalf("super-admin summary omitted separate college batch: %+v", summary.Rows)
	}

	const collegeOnlyWeekID = "a0000000-0000-0000-0000-000000000015"
	if _, err := pool.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date)
VALUES ($1,$2,2,'2026-09-08','2026-09-14')`, collegeOnlyWeekID, semesterID); err != nil {
		t.Fatal("insert college-only reporting week:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES ('b0000000-0000-0000-0000-000000000003',$1,$2,NULL,$3,'CONFIRMED',1,$4,$4,repeat('c',64),now())`,
		semesterID, collegeOnlyWeekID, inspectionTypeID, admin.ID); err != nil {
		t.Fatal("insert college-only submission:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES ('c0000000-0000-0000-0000-000000000003','b0000000-0000-0000-0000-000000000003',1,$1,
	ARRAY['a0000000-0000-0000-0000-000000000014'::uuid],'[]'::jsonb)`, admin.ID); err != nil {
		t.Fatal("insert college-only immutable revision:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES ('b0000000-0000-0000-0000-000000000004',$1,$2,NULL,'30000000-0000-0000-0000-000000000002','CONFIRMED',1,$3,$3,repeat('d',64),now())`,
		semesterID, collegeOnlyWeekID, admin.ID); err != nil {
		t.Fatal("insert second-type college-only submission:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES ('c0000000-0000-0000-0000-000000000004','b0000000-0000-0000-0000-000000000004',1,$1,
	ARRAY['c0000000-0000-0000-0000-000000000013'::uuid],'[]'::jsonb)`, admin.ID); err != nil {
		t.Fatal("insert second-type college-only immutable revision:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE department_inspection_scopes SET enabled=false`); err != nil {
		t.Fatal("disable department scopes for college-only period fixture:", err)
	}
	updatedPeriodsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, adminTokens.AccessToken)
	var updatedPeriodPage struct {
		Items []struct {
			WeekID string `json:"weekId"`
		} `json:"items"`
	}
	if updatedPeriodsResponse.Code != http.StatusOK || json.Unmarshal(updatedPeriodsResponse.Body.Bytes(), &updatedPeriodPage) != nil {
		t.Fatalf("periods response after college-only submission = %d %s", updatedPeriodsResponse.Code, updatedPeriodsResponse.Body.String())
	}
	collegeOnlyPeriodVisible := false
	for _, item := range updatedPeriodPage.Items {
		collegeOnlyPeriodVisible = collegeOnlyPeriodVisible || item.WeekID == collegeOnlyWeekID
	}
	if !collegeOnlyPeriodVisible {
		t.Fatalf("broad report API omitted college-only week: %+v", updatedPeriodPage.Items)
	}
	collegeOnlySummaryResponse := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+collegeOnlyWeekID, nil, adminTokens.AccessToken)
	var collegeOnlySummary struct {
		Rows []struct {
			DepartmentID         string `json:"departmentId"`
			CollegeLevel         bool   `json:"collegeLevel"`
			InspectionTypeID     string `json:"inspectionTypeId"`
			ConfirmedRecordCount int    `json:"confirmedRecordCount"`
		} `json:"rows"`
	}
	if collegeOnlySummaryResponse.Code != http.StatusOK || json.Unmarshal(collegeOnlySummaryResponse.Body.Bytes(), &collegeOnlySummary) != nil {
		t.Fatalf("college-only summary response = %d %s", collegeOnlySummaryResponse.Code, collegeOnlySummaryResponse.Body.String())
	}
	collegeAttendanceVisible := false
	for _, row := range collegeOnlySummary.Rows {
		if row.InspectionTypeID == inspectionTypeID && row.CollegeLevel && row.DepartmentID == "" && row.ConfirmedRecordCount == 1 {
			collegeAttendanceVisible = true
		}
	}
	if !collegeAttendanceVisible {
		t.Fatalf("college-only summary omitted its confirmed attendance row: %s", collegeOnlySummaryResponse.Body.String())
	}
	invalidPeriod := performJSON(router, http.MethodGet, "/api/v1/reports/summary?semesterId=bad&weekId=bad", nil, adminTokens.AccessToken)
	if invalidPeriod.Code != http.StatusBadRequest || !strings.Contains(invalidPeriod.Body.String(), "INVALID_REPORT_PERIOD") {
		t.Fatalf("invalid period response = %d %s", invalidPeriod.Code, invalidPeriod.Body.String())
	}

	memberID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
SELECT $1,'reporting-unprivileged','无通报权限用户',password_hash,'ACTIVE','SYSTEM','MEMBER',false,false
FROM accounts WHERE id=$2`, memberID, admin.ID); err != nil {
		t.Fatal(err)
	}
	memberTokens, err := authService.Login(ctx, "reporting-unprivileged", "ReportingTestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/reports/periods",
		"/api/v1/reports/summary?semesterId=" + semesterID + "&weekId=" + weekID,
	} {
		response := performJSON(router, http.MethodGet, path, nil, memberTokens.AccessToken)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "FORBIDDEN_SCOPE") {
			t.Fatalf("unprivileged response for %s = %d %s", path, response.Code, response.Body.String())
		}
	}

	ministerID, viceMinisterID, presidiumID, scopedAdminID, mismatchedLeaderID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, err = pool.Exec(ctx, `
INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password,primary_department_id)
SELECT fixture.account_id,fixture.login_name,fixture.display_name,source.password_hash,'ACTIVE','SYSTEM','MEMBER',false,false,fixture.department_id
FROM accounts source
CROSS JOIN (VALUES
 ($1::uuid,'reporting-office-minister','办公室部长',$5::uuid),
 ($2::uuid,'reporting-office-vice','办公室副部长',$5::uuid),
 ($3::uuid,'reporting-office-presidium','主席团用户',$5::uuid),
 ($4::uuid,'reporting-scoped-admin','指定通报管理员',NULL::uuid),
 ($7::uuid,'reporting-mismatched-leader','主部门与组织职务不匹配的用户',$5::uuid)
) AS fixture(account_id,login_name,display_name,department_id)
WHERE source.id=$6`, ministerID, viceMinisterID, presidiumID, scopedAdminID, officeDepartmentID, admin.ID, mismatchedLeaderID)
	if err != nil {
		t.Fatal("insert synthetic report actors:", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id)
VALUES
 ($1,$2,$5,'20000000-0000-0000-0000-000000000002'),
 ($3,$4,$5,'20000000-0000-0000-0000-000000000003'),
 ($6,$7,$5,'20000000-0000-0000-0000-000000000001'),
 ($8,$9,'10000000-0000-0000-0000-000000000002','20000000-0000-0000-0000-000000000002')`,
		uuid.NewString(), ministerID, uuid.NewString(), viceMinisterID, officeDepartmentID, uuid.NewString(), presidiumID, uuid.NewString(), mismatchedLeaderID)
	if err != nil {
		t.Fatal("insert synthetic office organization roles:", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO user_permission_rules(id,account_id,action,effect,source_type,academic_year_id,semester_id,week_id,inspection_type_id,department_id,note,enabled,created_by)
VALUES ($1,$2,'REPORT_VIEW','ALLOW','USER_RULE',$3,$4,NULL,$5,$6,'semester-wide department reporting scope',true,$7)`,
		uuid.NewString(), scopedAdminID, academicYearID, semesterID, inspectionTypeID, departmentID, admin.ID)
	if err != nil {
		t.Fatal("insert synthetic scoped report grant:", err)
	}

	denyScopeAccountID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
SELECT $1,'reporting-user-deny-scope','周期与类型拒绝范围用户',password_hash,'ACTIVE','SYSTEM','MEMBER',false,false
FROM accounts WHERE id=$2`, denyScopeAccountID, admin.ID); err != nil {
		t.Fatal("insert synthetic report deny-scope account:", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO user_permission_rules(id,account_id,action,effect,source_type,academic_year_id,semester_id,week_id,inspection_type_id,department_id,note,enabled,created_by)
VALUES
	($1,$2,'REPORT_VIEW','ALLOW','USER_RULE',$5,$6,NULL,NULL,NULL,'semester report scope',true,$8),
	($3,$2,'REPORT_VIEW','DENY','USER_RULE',$5,$6,$7,NULL,NULL,'exclude first week',true,$8),
	($4,$2,'REPORT_VIEW','DENY','USER_RULE',$5,$6,NULL,'30000000-0000-0000-0000-000000000002',NULL,'exclude unattended classroom',true,$8)`,
		uuid.NewString(), denyScopeAccountID, uuid.NewString(), uuid.NewString(), academicYearID, semesterID, weekID, admin.ID); err != nil {
		t.Fatal("insert synthetic report week/type denials:", err)
	}

	loginActor := func(loginName string) auth.Tokens {
		t.Helper()
		tokens, loginErr := authService.Login(ctx, loginName, "ReportingTestPassword123")
		if loginErr != nil {
			t.Fatal(loginErr)
		}
		return tokens
	}
	for _, actor := range []struct {
		name  string
		token string
	}{{"office minister", loginActor("reporting-office-minister").AccessToken}, {"office vice-minister", loginActor("reporting-office-vice").AccessToken}} {
		periods := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, actor.token)
		response := performJSON(router, http.MethodGet, "/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, actor.token)
		var roleSummary struct {
			Totals struct {
				ConfirmedRecordCount int `json:"confirmedRecordCount"`
			} `json:"totals"`
		}
		if periods.Code != http.StatusOK || response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &roleSummary) != nil || roleSummary.Totals.ConfirmedRecordCount != 3 {
			t.Fatalf("%s report access failed: periods=%d summary=%d %s", actor.name, periods.Code, response.Code, response.Body.String())
		}
	}

	presidiumTokens := loginActor("reporting-office-presidium")
	for _, path := range []string{
		"/api/v1/reports/periods",
		"/api/v1/reports/summary?semesterId=" + semesterID + "&weekId=" + weekID,
	} {
		response := performJSON(router, http.MethodGet, path, nil, presidiumTokens.AccessToken)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "FORBIDDEN_SCOPE") {
			t.Fatalf("Presidium unexpectedly received report access at %s: %d %s", path, response.Code, response.Body.String())
		}
	}

	mismatchedTokens := loginActor("reporting-mismatched-leader")
	mismatchedPeriods := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, mismatchedTokens.AccessToken)
	if mismatchedPeriods.Code != http.StatusForbidden || !strings.Contains(mismatchedPeriods.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("account with an office primary department and another department's minister role unexpectedly received report access: %d %s", mismatchedPeriods.Code, mismatchedPeriods.Body.String())
	}
	_, err = permissionService.CreateRule(ctx, admin.ID, permission.Rule{
		AccountID: mismatchedLeaderID, Action: permission.ActionReportForceGenerate, Effect: permission.EffectAllow,
	})
	if !errors.Is(err, permission.ErrForceGenerateRoleRequired) {
		t.Fatalf("force generation grant for mismatched department role should be rejected, got %v", err)
	}

	scopedTokens := loginActor("reporting-scoped-admin")
	scopedPeriods := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, scopedTokens.AccessToken)
	var scopedPeriodPage struct {
		Items []struct {
			SemesterID string `json:"semesterId"`
			WeekID     string `json:"weekId"`
		} `json:"items"`
	}
	if scopedPeriods.Code != http.StatusOK || json.Unmarshal(scopedPeriods.Body.Bytes(), &scopedPeriodPage) != nil || len(scopedPeriodPage.Items) != 1 ||
		scopedPeriodPage.Items[0].SemesterID != semesterID || scopedPeriodPage.Items[0].WeekID != weekID {
		t.Fatalf("scoped administrator periods = %d %s", scopedPeriods.Code, scopedPeriods.Body.String())
	}
	scopedSummary := performJSON(router, http.MethodGet, "/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, scopedTokens.AccessToken)
	var scopedPayload struct {
		Rows []struct {
			DepartmentID     string `json:"departmentId"`
			CollegeLevel     bool   `json:"collegeLevel"`
			InspectionTypeID string `json:"inspectionTypeId"`
		} `json:"rows"`
	}
	if scopedSummary.Code != http.StatusOK || json.Unmarshal(scopedSummary.Body.Bytes(), &scopedPayload) != nil || len(scopedPayload.Rows) != 1 ||
		scopedPayload.Rows[0].DepartmentID != departmentID || scopedPayload.Rows[0].CollegeLevel || scopedPayload.Rows[0].InspectionTypeID != inspectionTypeID {
		t.Fatalf("scoped administrator summary = %d %s", scopedSummary.Code, scopedSummary.Body.String())
	}
	scopedCollegeOnlySummary := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+collegeOnlyWeekID, nil, scopedTokens.AccessToken)
	if scopedCollegeOnlySummary.Code != http.StatusForbidden || !strings.Contains(scopedCollegeOnlySummary.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("department-scoped administrator accessed a college-only week: %d %s", scopedCollegeOnlySummary.Code, scopedCollegeOnlySummary.Body.String())
	}

	denyScopeTokens := loginActor("reporting-user-deny-scope")
	denyScopePeriods := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, denyScopeTokens.AccessToken)
	var denyScopePeriodPage struct {
		Items []struct {
			WeekID string `json:"weekId"`
		} `json:"items"`
	}
	if denyScopePeriods.Code != http.StatusOK || json.Unmarshal(denyScopePeriods.Body.Bytes(), &denyScopePeriodPage) != nil ||
		len(denyScopePeriodPage.Items) != 1 || denyScopePeriodPage.Items[0].WeekID != collegeOnlyWeekID {
		t.Fatalf("week/type-denied administrator periods = %d %s", denyScopePeriods.Code, denyScopePeriods.Body.String())
	}
	deniedWeekSummary := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, denyScopeTokens.AccessToken)
	if deniedWeekSummary.Code != http.StatusForbidden || !strings.Contains(deniedWeekSummary.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("week-denied summary should reject instead of returning empty success: %d %s", deniedWeekSummary.Code, deniedWeekSummary.Body.String())
	}
	allowedWeekSummary := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+collegeOnlyWeekID, nil, denyScopeTokens.AccessToken)
	var denyScopePayload struct {
		Rows []struct {
			InspectionTypeID     string `json:"inspectionTypeId"`
			CollegeLevel         bool   `json:"collegeLevel"`
			ConfirmedRecordCount int    `json:"confirmedRecordCount"`
		} `json:"rows"`
	}
	if allowedWeekSummary.Code != http.StatusOK || json.Unmarshal(allowedWeekSummary.Body.Bytes(), &denyScopePayload) != nil ||
		len(denyScopePayload.Rows) != 1 || denyScopePayload.Rows[0].InspectionTypeID != inspectionTypeID ||
		!denyScopePayload.Rows[0].CollegeLevel || denyScopePayload.Rows[0].ConfirmedRecordCount != 1 {
		t.Fatalf("week/type-scoped summary exposed a denied type or lost the allowed row: %d %s", allowedWeekSummary.Code, allowedWeekSummary.Body.String())
	}

	scopeTag, err := pool.Exec(ctx, `UPDATE department_inspection_scopes SET enabled=true
WHERE department_id=$1 AND inspection_type_id=$2 AND can_enter`, departmentID, inspectionTypeID)
	if err != nil || scopeTag.RowsAffected() != 1 {
		t.Fatalf("restore department entry scope for workflow submission: rows=%d err=%v", scopeTag.RowsAffected(), err)
	}
	const submittedRecordID = "a0000000-0000-0000-0000-000000000014"
	departmentSubmission := workflow.SubmitInput{
		Scope:   workflow.Scope{SemesterID: semesterID, WeekID: weekID, DepartmentID: departmentID, InspectionTypeID: inspectionTypeID},
		Records: []workflow.VersionedRecord{{ID: submittedRecordID, Version: 1}}, IdempotencyKey: "reporting-summary-flow-integration",
	}
	submissionResponse := performJSON(router, http.MethodPost, "/api/v1/department-submissions", departmentSubmission, adminTokens.AccessToken)
	var submittedBatch workflow.Submission
	if submissionResponse.Code != http.StatusCreated || json.Unmarshal(submissionResponse.Body.Bytes(), &submittedBatch) != nil ||
		submittedBatch.Status != workflow.PendingReview || submittedBatch.RevisionNo != 1 || len(submittedBatch.RecordIDs) != 1 {
		t.Fatalf("workflow submission response = %d %s", submissionResponse.Code, submissionResponse.Body.String())
	}
	confirmationResponse := performJSON(router, http.MethodPost, "/api/v1/department-submissions/"+submittedBatch.ID+"/confirm",
		workflow.DecisionInput{Version: submittedBatch.RowVersion}, adminTokens.AccessToken)
	var confirmedBatch workflow.Submission
	if confirmationResponse.Code != http.StatusOK || json.Unmarshal(confirmationResponse.Body.Bytes(), &confirmedBatch) != nil ||
		confirmedBatch.Status != workflow.Confirmed || confirmedBatch.RevisionNo != submittedBatch.RevisionNo {
		t.Fatalf("workflow confirmation response = %d %s", confirmationResponse.Code, confirmationResponse.Body.String())
	}

	refreshedSummaryResponse := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, adminTokens.AccessToken)
	var refreshedSummary reporting.Summary
	if refreshedSummaryResponse.Code != http.StatusOK || json.Unmarshal(refreshedSummaryResponse.Body.Bytes(), &refreshedSummary) != nil {
		t.Fatalf("summary after real department submit and confirmation = %d %s", refreshedSummaryResponse.Code, refreshedSummaryResponse.Body.String())
	}
	departmentRowIncludesSubmittedRecord := false
	for _, row := range refreshedSummary.Rows {
		if row.DepartmentID == departmentID && row.InspectionTypeID == inspectionTypeID {
			departmentRowIncludesSubmittedRecord = row.SubmissionCount == 2 && row.ConfirmedBatchCount == 2 && row.ConfirmedRecordCount == 2
		}
	}
	if !departmentRowIncludesSubmittedRecord || refreshedSummary.Totals.ConfirmedRecordCount != 4 {
		t.Fatalf("report summary did not include the workflow-confirmed record: rows=%+v totals=%+v", refreshedSummary.Rows, refreshedSummary.Totals)
	}
	recordsURL := "/api/v1/reports/records?semesterId=" + semesterID + "&weekId=" + weekID +
		"&departmentId=" + departmentID + "&collegeLevel=false&inspectionTypeId=" + inspectionTypeID + "&page=1&pageSize=20"
	recordsResponse := performJSON(router, http.MethodGet, recordsURL, nil, adminTokens.AccessToken)
	var confirmedRecords reporting.RecordPage
	if recordsResponse.Code != http.StatusOK || json.Unmarshal(recordsResponse.Body.Bytes(), &confirmedRecords) != nil ||
		confirmedRecords.Total != 1 || len(confirmedRecords.Items) != 1 || confirmedRecords.Items[0].ID != submittedRecordID ||
		confirmedRecords.Items[0].StudentName != "Synthetic student" || confirmedRecords.Items[0].Details.Period != "1-2" ||
		confirmedRecords.Items[0].CurrentCourseName != "Fixture course" {
		t.Fatalf("records after real department submit and confirmation = %d %s", recordsResponse.Code, recordsResponse.Body.String())
	}
	collegeRecordsURL := "/api/v1/reports/records?semesterId=" + semesterID + "&weekId=" + weekID +
		"&departmentId=&collegeLevel=true&inspectionTypeId=" + inspectionTypeID + "&page=1&pageSize=20"
	collegeRecordsResponse := performJSON(router, http.MethodGet, collegeRecordsURL, nil, adminTokens.AccessToken)
	var confirmedCollegeRecords reporting.RecordPage
	if collegeRecordsResponse.Code != http.StatusOK || json.Unmarshal(collegeRecordsResponse.Body.Bytes(), &confirmedCollegeRecords) != nil ||
		confirmedCollegeRecords.Total != 2 || len(confirmedCollegeRecords.Items) != 2 ||
		confirmedCollegeRecords.Items[0].Details.Period != "1-2" || confirmedCollegeRecords.Items[0].CurrentCourseName != "Fixture course" {
		t.Fatalf("college-level records response = %d %s", collegeRecordsResponse.Code, collegeRecordsResponse.Body.String())
	}
	collegeReaderResponse := performJSON(router, http.MethodGet, collegeRecordsURL, nil, collegeReaderTokens.AccessToken)
	var collegeReaderRecords reporting.RecordPage
	if collegeReaderResponse.Code != http.StatusOK || json.Unmarshal(collegeReaderResponse.Body.Bytes(), &collegeReaderRecords) != nil ||
		collegeReaderRecords.Total != 2 || len(collegeReaderRecords.Items) != 2 {
		t.Fatalf("college-level records response for unassigned report reader = %d %s", collegeReaderResponse.Code, collegeReaderResponse.Body.String())
	}
}

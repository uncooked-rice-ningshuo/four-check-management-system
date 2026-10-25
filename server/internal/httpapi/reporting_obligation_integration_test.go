package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

func TestWeeklyObligationAndZeroReceiptHTTPPostgres(t *testing.T) {
	databaseURL := os.Getenv("REPORTING_OBLIGATION_API_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REPORTING_OBLIGATION_API_TEST_DATABASE_URL must point to an empty disposable database")
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

	const (
		semesterID     = "a0000000-0000-0000-0000-000000000003"
		academicYearID = "a0000000-0000-0000-0000-000000000002"
		weekID         = "a0000000-0000-0000-0000-000000000004"
		departmentID   = "10000000-0000-0000-0000-000000000002"
		officeID       = "10000000-0000-0000-0000-000000000001"
		typeID         = "30000000-0000-0000-0000-000000000001"
		classID        = "a0000000-0000-0000-0000-000000000006"
		password       = "ReportingTestPassword123"
	)
	database := &store.Postgres{Pool: pool}
	permissionService := permission.NewService(database)
	authService := auth.NewService(database, database, []byte("reporting-obligation-http-test-jwt-secret-32-bytes"), 5*time.Minute, time.Hour)
	admin, err := authService.BootstrapAdmin(ctx, "reporting-obligation-admin", "隔离通报管理员", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET must_change_password=false WHERE id=$1", admin.ID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO department_inspection_scopes(
		department_id,inspection_type_id,enabled,can_enter,can_view,can_review)
		VALUES($1,$2,true,true,true,true)
		ON CONFLICT(department_id,inspection_type_id) DO UPDATE
		SET enabled=true,can_enter=true,can_view=true,can_review=true`, departmentID, typeID)
	if err != nil {
		t.Fatal("configure synthetic department/type scope:", err)
	}

	newAccount := func(loginName, name, department string) string {
		t.Helper()
		id := uuid.NewString()
		_, insertErr := pool.Exec(ctx, `INSERT INTO accounts(
			id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password,primary_department_id)
			SELECT $1,$2,$3,password_hash,'ACTIVE','SYSTEM','MEMBER',false,false,NULLIF($4,'')::uuid
			FROM accounts WHERE id=$5`, id, loginName, name, department, admin.ID)
		if insertErr != nil {
			t.Fatal("insert synthetic reporting actor:", insertErr)
		}
		return id
	}
	managerID := newAccount("obligation-manager", "应交范围管理员", officeID)
	submitterID := newAccount("zero-receipt-member", "零记录提交人", departmentID)
	reviewerID := newAccount("zero-receipt-reviewer", "零记录审核人", departmentID)
	classScopedID := newAccount("class-scoped-member", "班级范围成员", officeID)
	if _, err := pool.Exec(ctx, `UPDATE inspection_records SET
		student_id='a0000000-0000-0000-0000-000000000007',student_no_snapshot='S-FIXTURE',
		student_name_snapshot='Synthetic student',class_id=$1,class_name_snapshot='Fixture class',grade_snapshot='2026',created_by=$2
		WHERE id='a0000000-0000-0000-0000-000000000014'`, classID, submitterID); err != nil {
		t.Fatal("prepare a valid draft owned by the synthetic submitter:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id)
		VALUES($1,$2,$3,'20000000-0000-0000-0000-000000000002'),
		($4,$5,$6,'20000000-0000-0000-0000-000000000002')`,
		uuid.NewString(), managerID, officeID, uuid.NewString(), reviewerID, departmentID); err != nil {
		t.Fatal("assign synthetic office and department leaders:", err)
	}
	if _, err := permissionService.CreateRule(ctx, admin.ID, permission.Rule{
		AccountID: managerID, Action: permission.ActionReportObligationManage, Effect: permission.EffectAllow,
	}); err != nil {
		t.Fatal("grant explicit obligation-management permission to eligible office minister:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_permission_rules(
		id,account_id,action,effect,source_type,academic_year_id,semester_id,week_id,inspection_type_id,department_id,class_id,note,enabled,created_by)
		VALUES($1,$2,'INSPECTION_ENTER','ALLOW','USER_RULE',$3,$4,$5,$6,$7,$8,'class-limited entry only',true,$9)`,
		uuid.NewString(), classScopedID, academicYearID, semesterID, weekID, typeID, departmentID, classID, admin.ID); err != nil {
		t.Fatal("grant synthetic class-limited entry:", err)
	}

	login := func(loginName string) string {
		t.Helper()
		tokens, loginErr := authService.Login(ctx, loginName, password)
		if loginErr != nil {
			t.Fatal(loginErr)
		}
		return tokens.AccessToken
	}
	managerToken := login("obligation-manager")
	submitterToken := login("zero-receipt-member")
	reviewerToken := login("zero-receipt-reviewer")
	classScopedToken := login("class-scoped-member")
	router := NewRouter(authService, false, AdminServices{
		Permission: permissionService,
		Reporting:  reporting.NewService(database, permissionService),
		Workflow:   workflow.NewService(database, permissionService),
	})
	periodQuery := "?semesterId=" + semesterID + "&weekId=" + weekID
	optionsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/obligation-options"+periodQuery, nil, managerToken)
	var options reporting.ReportObligationOptions
	if optionsResponse.Code != http.StatusOK || json.Unmarshal(optionsResponse.Body.Bytes(), &options) != nil {
		t.Fatalf("authorized obligation options = %d %s", optionsResponse.Code, optionsResponse.Body.String())
	}
	departmentOptionFound := false
	for _, item := range options.DepartmentScopes {
		departmentOptionFound = departmentOptionFound || (item.DepartmentID == departmentID && item.InspectionTypeID == typeID)
	}
	if !departmentOptionFound {
		t.Fatalf("eligible department/type was missing from obligation options: %+v", options.DepartmentScopes)
	}
	unauthorizedList := performJSON(router, http.MethodGet, "/api/v1/reports/obligations"+periodQuery, nil, submitterToken)
	if unauthorizedList.Code != http.StatusForbidden {
		t.Fatalf("ordinary member obligation list = %d %s, want 403", unauthorizedList.Code, unauthorizedList.Body.String())
	}
	initialList := performJSON(router, http.MethodGet, "/api/v1/reports/obligations"+periodQuery, nil, managerToken)
	var initialItems struct {
		Items []reporting.ReportObligation `json:"items"`
	}
	if initialList.Code != http.StatusOK || json.Unmarshal(initialList.Body.Bytes(), &initialItems) != nil || len(initialItems.Items) != 0 {
		t.Fatalf("initial obligation list = %d %s", initialList.Code, initialList.Body.String())
	}
	createInput := reporting.CreateReportObligationInput{
		SemesterID: semesterID, WeekID: weekID, DepartmentID: departmentID, InspectionTypeID: typeID,
	}
	createResponse := performJSON(router, http.MethodPost, "/api/v1/reports/obligations", createInput, managerToken)
	var obligation reporting.ReportObligation
	if createResponse.Code != http.StatusCreated || json.Unmarshal(createResponse.Body.Bytes(), &obligation) != nil ||
		!obligation.Eligible || obligation.Version != 1 || obligation.DepartmentID != departmentID || obligation.InspectionTypeID != typeID {
		t.Fatalf("create obligation = %d %s", createResponse.Code, createResponse.Body.String())
	}
	duplicateObligation := performJSON(router, http.MethodPost, "/api/v1/reports/obligations", createInput, managerToken)
	if duplicateObligation.Code != http.StatusConflict {
		t.Fatalf("duplicate obligation = %d %s, want 409", duplicateObligation.Code, duplicateObligation.Body.String())
	}

	zeroInput := workflow.SubmitInput{
		Scope: workflow.Scope{SemesterID: semesterID, WeekID: weekID, DepartmentID: departmentID, InspectionTypeID: typeID},
		Kind:  workflow.SubmissionKindZeroRecord, Records: []workflow.VersionedRecord{}, IdempotencyKey: "weekly-zero-receipt-1",
	}
	classScopedZero := performJSON(router, http.MethodPost, "/api/v1/department-submissions", zeroInput, classScopedToken)
	if classScopedZero.Code != http.StatusForbidden {
		t.Fatalf("class-scoped actor submitted whole-cell zero receipt = %d %s, want 403", classScopedZero.Code, classScopedZero.Body.String())
	}
	zeroSubmitResponse := performJSON(router, http.MethodPost, "/api/v1/department-submissions", zeroInput, submitterToken)
	var zeroSubmission workflow.Submission
	if zeroSubmitResponse.Code != http.StatusCreated || json.Unmarshal(zeroSubmitResponse.Body.Bytes(), &zeroSubmission) != nil ||
		zeroSubmission.Kind != workflow.SubmissionKindZeroRecord || zeroSubmission.Status != workflow.PendingReview ||
		zeroSubmission.RevisionNo != 1 || len(zeroSubmission.RecordIDs) != 0 || len(zeroSubmission.Records) != 0 {
		t.Fatalf("zero receipt submit = %d %s", zeroSubmitResponse.Code, zeroSubmitResponse.Body.String())
	}
	zeroReplay := performJSON(router, http.MethodPost, "/api/v1/department-submissions", zeroInput, submitterToken)
	var replayedZero workflow.Submission
	if zeroReplay.Code != http.StatusCreated || json.Unmarshal(zeroReplay.Body.Bytes(), &replayedZero) != nil || replayedZero.ID != zeroSubmission.ID {
		t.Fatalf("zero receipt idempotent replay = %d %s", zeroReplay.Code, zeroReplay.Body.String())
	}
	returnZero := performJSON(router, http.MethodPost, "/api/v1/department-submissions/"+zeroSubmission.ID+"/return",
		workflow.DecisionInput{Version: zeroSubmission.RowVersion, Reason: "请复核本周检查范围后重提"}, reviewerToken)
	var returnedZero workflow.Submission
	if returnZero.Code != http.StatusOK || json.Unmarshal(returnZero.Body.Bytes(), &returnedZero) != nil ||
		returnedZero.ID != zeroSubmission.ID || returnedZero.Kind != workflow.SubmissionKindZeroRecord ||
		returnedZero.Status != workflow.Returned || returnedZero.RowVersion <= zeroSubmission.RowVersion ||
		returnedZero.ReturnReason != "请复核本周检查范围后重提" || len(returnedZero.RecordIDs) != 0 {
		t.Fatalf("zero receipt return = %d %s", returnZero.Code, returnZero.Body.String())
	}
	resubmitZero := performJSON(router, http.MethodPost, "/api/v1/department-submissions/"+zeroSubmission.ID+"/resubmit",
		workflow.ResubmitInput{Version: returnedZero.RowVersion, Kind: workflow.SubmissionKindZeroRecord, Records: []workflow.VersionedRecord{}}, submitterToken)
	var resubmittedZero workflow.Submission
	if resubmitZero.Code != http.StatusOK || json.Unmarshal(resubmitZero.Body.Bytes(), &resubmittedZero) != nil ||
		resubmittedZero.ID != zeroSubmission.ID || resubmittedZero.Kind != workflow.SubmissionKindZeroRecord ||
		resubmittedZero.Status != workflow.PendingReview || resubmittedZero.RevisionNo != 2 ||
		resubmittedZero.RowVersion <= returnedZero.RowVersion || len(resubmittedZero.RecordIDs) != 0 {
		t.Fatalf("zero receipt resubmission = %d %s", resubmitZero.Code, resubmitZero.Body.String())
	}
	confirmZero := performJSON(router, http.MethodPost, "/api/v1/department-submissions/"+zeroSubmission.ID+"/confirm",
		workflow.DecisionInput{Version: resubmittedZero.RowVersion}, reviewerToken)
	var confirmedZero workflow.Submission
	if confirmZero.Code != http.StatusOK || json.Unmarshal(confirmZero.Body.Bytes(), &confirmedZero) != nil ||
		confirmedZero.ID != zeroSubmission.ID || confirmedZero.Kind != workflow.SubmissionKindZeroRecord ||
		confirmedZero.Status != workflow.Confirmed || confirmedZero.RevisionNo != 2 || len(confirmedZero.RecordIDs) != 0 {
		t.Fatalf("zero receipt confirmation = %d %s", confirmZero.Code, confirmZero.Body.String())
	}
	var zeroRevisionCount, preservedZeroRevisionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE submission_kind='ZERO_RECORD' AND cardinality(record_ids)=0 AND records_snapshot='[]'::jsonb)
		FROM department_submission_revisions WHERE submission_id=$1`, zeroSubmission.ID).Scan(&zeroRevisionCount, &preservedZeroRevisionCount); err != nil ||
		zeroRevisionCount != 2 || preservedZeroRevisionCount != 2 {
		t.Fatalf("zero receipt returned revision history = total %d preserved %d, %v", zeroRevisionCount, preservedZeroRevisionCount, err)
	}

	initialSummary := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, managerToken)
	var summary reporting.Summary
	if initialSummary.Code != http.StatusOK || json.Unmarshal(initialSummary.Body.Bytes(), &summary) != nil {
		t.Fatalf("summary after zero receipt confirmation = %d %s", initialSummary.Code, initialSummary.Body.String())
	}
	assertSummary := func(wantSubmissions, wantConfirmedRecords, wantZeroConfirmed int) {
		t.Helper()
		for _, row := range summary.Rows {
			if row.DepartmentID == departmentID && row.InspectionTypeID == typeID {
				if !row.IsRequired || row.SubmissionCount != wantSubmissions || row.ConfirmedRecordCount != wantConfirmedRecords ||
					row.ZeroRecordConfirmedBatchCount != wantZeroConfirmed {
					t.Fatalf("department summary after submissions = %+v", row)
				}
				return
			}
		}
		t.Fatalf("summary omitted configured department/type cell: %+v", summary.Rows)
	}
	assertSummary(1, 0, 1)

	var recordVersion int
	const actualRecordID = "a0000000-0000-0000-0000-000000000014"
	if err := pool.QueryRow(ctx, "SELECT row_version FROM inspection_records WHERE id=$1", actualRecordID).Scan(&recordVersion); err != nil {
		t.Fatal("read synthetic record version:", err)
	}
	actualInput := workflow.SubmitInput{
		Scope: zeroInput.Scope, Kind: workflow.SubmissionKindRecords,
		Records: []workflow.VersionedRecord{{ID: actualRecordID, Version: recordVersion}}, IdempotencyKey: "weekly-actual-record-after-zero",
	}
	actualSubmitResponse := performJSON(router, http.MethodPost, "/api/v1/department-submissions", actualInput, submitterToken)
	var actualSubmission workflow.Submission
	if actualSubmitResponse.Code != http.StatusCreated || json.Unmarshal(actualSubmitResponse.Body.Bytes(), &actualSubmission) != nil ||
		actualSubmission.Kind != workflow.SubmissionKindRecords || actualSubmission.Status != workflow.PendingReview || len(actualSubmission.RecordIDs) != 1 {
		t.Fatalf("actual batch after zero receipt = %d %s", actualSubmitResponse.Code, actualSubmitResponse.Body.String())
	}
	confirmActual := performJSON(router, http.MethodPost, "/api/v1/department-submissions/"+actualSubmission.ID+"/confirm",
		workflow.DecisionInput{Version: actualSubmission.RowVersion}, reviewerToken)
	var confirmedActual workflow.Submission
	if confirmActual.Code != http.StatusOK || json.Unmarshal(confirmActual.Body.Bytes(), &confirmedActual) != nil || confirmedActual.Status != workflow.Confirmed {
		t.Fatalf("actual batch confirmation after zero receipt = %d %s", confirmActual.Code, confirmActual.Body.String())
	}
	updatedSummary := performJSON(router, http.MethodGet,
		"/api/v1/reports/summary?semesterId="+semesterID+"&weekId="+weekID, nil, managerToken)
	if updatedSummary.Code != http.StatusOK || json.Unmarshal(updatedSummary.Body.Bytes(), &summary) != nil {
		t.Fatalf("summary after actual batch = %d %s", updatedSummary.Code, updatedSummary.Body.String())
	}
	assertSummary(2, 1, 1)
	memberBatches := performJSON(router, http.MethodGet,
		"/api/v1/department-submissions?semesterId="+semesterID+"&weekId="+weekID+"&page=1&pageSize=20", nil, submitterToken)
	var batchPage workflow.Page
	if memberBatches.Code != http.StatusOK || json.Unmarshal(memberBatches.Body.Bytes(), &batchPage) != nil || batchPage.Total != 2 || len(batchPage.Items) != 2 {
		t.Fatalf("member batch history after actual append = %d %s", memberBatches.Code, memberBatches.Body.String())
	}
	seenZero, seenRecords := false, false
	for _, item := range batchPage.Items {
		seenZero = seenZero || (item.ID == zeroSubmission.ID && item.Kind == workflow.SubmissionKindZeroRecord && len(item.RecordIDs) == 0)
		seenRecords = seenRecords || (item.ID == actualSubmission.ID && item.Kind == workflow.SubmissionKindRecords && len(item.RecordIDs) == 1)
	}
	if !seenZero || !seenRecords {
		t.Fatalf("appended actual batch replaced or obscured the zero receipt: %+v", batchPage.Items)
	}
	var zeroAuditCount, actualAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action IN
		('DEPARTMENT_SUBMITTED','DEPARTMENT_RETURNED','DEPARTMENT_RESUBMITTED','DEPARTMENT_CONFIRMED')`, zeroSubmission.ID).Scan(&zeroAuditCount); err != nil || zeroAuditCount != 4 {
		t.Fatalf("zero receipt submit/return/resubmit/confirm audit count = %d, %v", zeroAuditCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action IN ('DEPARTMENT_SUBMITTED','DEPARTMENT_CONFIRMED')`, actualSubmission.ID).Scan(&actualAuditCount); err != nil || actualAuditCount != 2 {
		t.Fatalf("actual batch submit/confirm audit count = %d, %v", actualAuditCount, err)
	}

	staleRemove := performJSON(router, http.MethodDelete, "/api/v1/reports/obligations/"+obligation.ID,
		map[string]int{"version": obligation.Version + 1}, managerToken)
	if staleRemove.Code != http.StatusConflict {
		t.Fatalf("stale obligation removal = %d %s, want 409", staleRemove.Code, staleRemove.Body.String())
	}
	removeResponse := performJSON(router, http.MethodDelete, "/api/v1/reports/obligations/"+obligation.ID,
		map[string]int{"version": obligation.Version}, managerToken)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("obligation removal = %d %s", removeResponse.Code, removeResponse.Body.String())
	}
	removedCellZero := performJSON(router, http.MethodPost, "/api/v1/department-submissions", workflow.SubmitInput{
		Scope: zeroInput.Scope, Kind: workflow.SubmissionKindZeroRecord, Records: []workflow.VersionedRecord{}, IdempotencyKey: "weekly-zero-after-removal",
	}, submitterToken)
	if removedCellZero.Code != http.StatusForbidden {
		t.Fatalf("zero receipt submitted after obligation removal = %d %s, want 403", removedCellZero.Code, removedCellZero.Body.String())
	}
}

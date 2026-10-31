package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5/pgconn"
)

type reportingHTTPStub struct {
	periods          []reporting.PeriodOption
	periodsErr       error
	summary          reporting.Summary
	summaryErr       error
	periodActor      string
	summaryActor     string
	workflow         reporting.ReportWorkflow
	workflowErr      error
	workflowActor    string
	archive          reporting.ReportArchivePage
	archiveErr       error
	archiveQuery     reporting.ReportArchiveQuery
	archiveActor     string
	archiveWorkflow  reporting.ReportWorkflow
	archiveWFErr     error
	semesterID       string
	weekID           string
	versionRecords   reporting.RecordPage
	versionNo        int
	draft            reporting.ReportDraft
	draftErr         error
	transition       reporting.ReportState
	transitionErr    error
	transitionID     string
	transitionAction string
	correctionResult reporting.ReportCorrectionResult
	correctionErr    error
	correctionID     string
	adjustmentResult reporting.ReportAdjustmentResult
	adjustmentErr    error
	adjustmentID     string
	decisionState    reporting.ReportState
	decisionErr      error
	decisionID       string
}

func (s *reportingHTTPStub) Periods(_ context.Context, actorID string) ([]reporting.PeriodOption, error) {
	s.periodActor = actorID
	return s.periods, s.periodsErr
}

func (s *reportingHTTPStub) Summary(_ context.Context, actorID, semesterID, weekID string) (reporting.Summary, error) {
	s.summaryActor, s.semesterID, s.weekID = actorID, semesterID, weekID
	return s.summary, s.summaryErr
}

func (s *reportingHTTPStub) Workflow(_ context.Context, actorID, semesterID, weekID string) (reporting.ReportWorkflow, error) {
	s.workflowActor, s.semesterID, s.weekID = actorID, semesterID, weekID
	return s.workflow, s.workflowErr
}

func (s *reportingHTTPStub) ReportVersionRecords(_ context.Context, actorID, semesterID, weekID string,
	versionNo int, query reporting.ReportVersionRecordsQuery) (reporting.RecordPage, error) {
	s.workflowActor, s.semesterID, s.weekID, s.versionNo = actorID, semesterID, weekID, versionNo
	s.versionRecords.Page, s.versionRecords.PageSize = query.Page, query.PageSize
	return s.versionRecords, nil
}

func (s *reportingHTTPStub) Archive(_ context.Context, actorID string, query reporting.ReportArchiveQuery) (reporting.ReportArchivePage, error) {
	s.archiveActor, s.archiveQuery = actorID, query
	return s.archive, s.archiveErr
}

func (s *reportingHTTPStub) ArchiveWorkflow(_ context.Context, actorID, semesterID, weekID string) (reporting.ReportWorkflow, error) {
	s.archiveActor, s.semesterID, s.weekID = actorID, semesterID, weekID
	return s.archiveWorkflow, s.archiveWFErr
}

func (s *reportingHTTPStub) Records(context.Context, string, reporting.RecordsQuery) (reporting.RecordPage, error) {
	return reporting.RecordPage{Items: []reporting.Record{}}, nil
}

func (s *reportingHTTPStub) GenerateDraft(_ context.Context, _ string, _ reporting.GenerateDraftInput, _ audit.Metadata) (reporting.ReportDraft, error) {
	return s.draft, s.draftErr
}

func (s *reportingHTTPStub) TransitionReport(_ context.Context, _ string, id, action string, _ reporting.ReportTransitionInput, _ audit.Metadata) (reporting.ReportState, error) {
	s.transitionID, s.transitionAction = id, action
	return s.transition, s.transitionErr
}

func (s *reportingHTTPStub) AddReportCorrection(_ context.Context, _ string, id string, _ reporting.AddReportCorrectionInput, _ audit.Metadata) (reporting.ReportCorrectionResult, error) {
	s.correctionID = id
	return s.correctionResult, s.correctionErr
}

func (s *reportingHTTPStub) AddReportAdjustment(_ context.Context, _ string, id string, _ reporting.AddReportAdjustmentInput, _ audit.Metadata) (reporting.ReportAdjustmentResult, error) {
	s.adjustmentID = id
	return s.adjustmentResult, s.adjustmentErr
}

func (s *reportingHTTPStub) DecideReportCorrection(_ context.Context, _ string, id string, _ reporting.DecideReportCorrectionInput, _ audit.Metadata) (reporting.ReportState, error) {
	s.decisionID = id
	return s.decisionState, s.decisionErr
}

func TestReportingEndpointsRequireAuthenticationAndReturnContracts(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(repository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, err := authService.BootstrapAdmin(context.Background(), "report-admin", "通报管理员", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, err := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	period := reporting.PeriodOption{AcademicYearID: "year", SemesterID: "semester", WeekID: "week", WeekNo: 2}
	stub := &reportingHTTPStub{periods: []reporting.PeriodOption{period}, summary: reporting.Summary{
		Period: period,
		Rows:   []reporting.SummaryRow{},
	}}
	router := NewRouter(authService, false, AdminServices{Reporting: stub})

	for _, path := range []string{"/api/v1/reports/periods", "/api/v1/reports/summary?semesterId=semester&weekId=week", "/api/v1/reports/workflow?semesterId=semester&weekId=week", "/api/v1/reports/archive?page=1&pageSize=20", "/api/v1/reports/archive/semester/week"} {
		if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s response = %d %s", path, response.Code, response.Body.String())
		}
	}
	for _, route := range []struct {
		path string
		body any
	}{
		{path: "/api/v1/reports/weekly-report/finalize", body: reporting.ReportTransitionInput{RowVersion: 1}},
		{path: "/api/v1/reports/weekly-report/corrections", body: reporting.AddReportCorrectionInput{RowVersion: 1}},
		{path: "/api/v1/report-corrections/correction-id/decision", body: reporting.DecideReportCorrectionInput{RowVersion: 1}},
	} {
		if response := performJSON(router, http.MethodPost, route.path, route.body, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s response = %d %s", route.path, response.Code, response.Body.String())
		}
	}

	periodsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, tokens.AccessToken)
	if periodsResponse.Code != http.StatusOK || !strings.Contains(periodsResponse.Body.String(), `"items":[`) || stub.periodActor != admin.ID {
		t.Fatalf("periods response = %d %s actor=%q", periodsResponse.Code, periodsResponse.Body.String(), stub.periodActor)
	}
	stub.periods = []reporting.PeriodOption{}
	emptyPeriodsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/periods", nil, tokens.AccessToken)
	if emptyPeriodsResponse.Code != http.StatusOK || !strings.Contains(emptyPeriodsResponse.Body.String(), `"items":[]`) {
		t.Fatalf("empty periods response = %d %s; want an empty array", emptyPeriodsResponse.Code, emptyPeriodsResponse.Body.String())
	}
	summaryResponse := performJSON(router, http.MethodGet, "/api/v1/reports/summary?semesterId=semester&weekId=week", nil, tokens.AccessToken)
	if summaryResponse.Code != http.StatusOK || !strings.Contains(summaryResponse.Body.String(), `"rows":[]`) ||
		stub.summaryActor != admin.ID || stub.semesterID != "semester" || stub.weekID != "week" {
		t.Fatalf("summary response = %d %s actor=%q period=%q/%q", summaryResponse.Code, summaryResponse.Body.String(), stub.summaryActor, stub.semesterID, stub.weekID)
	}
	stub.workflow = reporting.ReportWorkflow{Versions: []reporting.ReportVersion{}, Reviews: []reporting.ReportReview{}, Corrections: []reporting.ReportCorrectionLog{}}
	workflowResponse := performJSON(router, http.MethodGet, "/api/v1/reports/workflow?semesterId=semester&weekId=week", nil, tokens.AccessToken)
	if workflowResponse.Code != http.StatusOK || !strings.Contains(workflowResponse.Body.String(), `"report":null`) ||
		!strings.Contains(workflowResponse.Body.String(), `"versions":[]`) || stub.workflowActor != admin.ID || stub.semesterID != "semester" || stub.weekID != "week" {
		t.Fatalf("workflow response = %d %s actor=%q period=%q/%q", workflowResponse.Code, workflowResponse.Body.String(), stub.workflowActor, stub.semesterID, stub.weekID)
	}
	stub.versionRecords = reporting.RecordPage{Items: []reporting.Record{}, Total: 0, Page: 2, PageSize: 5}
	versionRecordsResponse := performJSON(router, http.MethodGet, "/api/v1/reports/workflow/semester/week/versions/3/records?page=2&pageSize=5", nil, tokens.AccessToken)
	if versionRecordsResponse.Code != http.StatusOK || !strings.Contains(versionRecordsResponse.Body.String(), `"items":[]`) ||
		stub.workflowActor != admin.ID || stub.semesterID != "semester" || stub.weekID != "week" || stub.versionNo != 3 ||
		stub.versionRecords.Page != 2 || stub.versionRecords.PageSize != 5 {
		t.Fatalf("version records response = %d %s actor=%q period=%q/%q version=%d query=%+v", versionRecordsResponse.Code,
			versionRecordsResponse.Body.String(), stub.workflowActor, stub.semesterID, stub.weekID, stub.versionNo, stub.versionRecords)
	}
	archivePeriod := reporting.PeriodOption{AcademicYearID: "year", AcademicYearName: "2025-2026学年", SemesterID: "semester", SemesterName: "第二学期", WeekID: "week", WeekNo: 2, StartDate: time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}
	stub.archive = reporting.ReportArchivePage{Items: []reporting.ReportArchiveEntry{{
		ID: "report-id", Period: archivePeriod, Status: reporting.ReportStatusFinalized, VersionNo: 1,
		VersionCount: 1, ConfirmedRecordCount: 4, CellCount: 2, UpdatedAt: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
	}}, Total: 1, Page: 2, PageSize: 5}
	archiveResponse := performJSON(router, http.MethodGet, "/api/v1/reports/archive?page=2&pageSize=5&status=FINALIZED", nil, tokens.AccessToken)
	if archiveResponse.Code != http.StatusOK || !strings.Contains(archiveResponse.Body.String(), `"status":"FINALIZED"`) ||
		stub.archiveActor != admin.ID || stub.archiveQuery.Page != 2 || stub.archiveQuery.PageSize != 5 || stub.archiveQuery.Status != reporting.ReportStatusFinalized {
		t.Fatalf("archive response = %d %s actor=%q query=%+v", archiveResponse.Code, archiveResponse.Body.String(), stub.archiveActor, stub.archiveQuery)
	}
	if invalidArchiveQuery := performJSON(router, http.MethodGet, "/api/v1/reports/archive?page=bad", nil, tokens.AccessToken); invalidArchiveQuery.Code != http.StatusBadRequest || !strings.Contains(invalidArchiveQuery.Body.String(), "INVALID_REPORT_ARCHIVE_QUERY") {
		t.Fatalf("invalid archive query response = %d %s", invalidArchiveQuery.Code, invalidArchiveQuery.Body.String())
	}
	stub.archiveWorkflow = reporting.ReportWorkflow{Report: &reporting.ReportState{ID: "report-id", Status: reporting.ReportStatusFinalized}, Versions: []reporting.ReportVersion{}, Reviews: []reporting.ReportReview{}, Corrections: []reporting.ReportCorrectionLog{}}
	archiveWorkflowResponse := performJSON(router, http.MethodGet, "/api/v1/reports/archive/semester/week", nil, tokens.AccessToken)
	if archiveWorkflowResponse.Code != http.StatusOK || !strings.Contains(archiveWorkflowResponse.Body.String(), `"status":"FINALIZED"`) ||
		stub.semesterID != "semester" || stub.weekID != "week" || stub.archiveActor != admin.ID {
		t.Fatalf("archive workflow response = %d %s actor=%q period=%q/%q", archiveWorkflowResponse.Code, archiveWorkflowResponse.Body.String(), stub.archiveActor, stub.semesterID, stub.weekID)
	}
	stub.draft = reporting.ReportDraft{ID: "weekly-report", Status: reporting.ReportStatusDraft, VersionNo: 1}
	draftResponse := performJSON(router, http.MethodPost, "/api/v1/reports/drafts", reporting.GenerateDraftInput{
		SemesterID: "a0000000-0000-0000-0000-000000000003", WeekID: "a0000000-0000-0000-0000-000000000004",
	}, tokens.AccessToken)
	if draftResponse.Code != http.StatusCreated || !strings.Contains(draftResponse.Body.String(), `"status":"DRAFT"`) {
		t.Fatalf("draft response = %d %s", draftResponse.Code, draftResponse.Body.String())
	}
	stub.transition = reporting.ReportState{ID: "weekly-report", Status: reporting.ReportStatusPending, RowVersion: 2}
	submitResponse := performJSON(router, http.MethodPost, "/api/v1/reports/weekly-report/submit", reporting.ReportTransitionInput{RowVersion: 1}, tokens.AccessToken)
	if submitResponse.Code != http.StatusOK || stub.transitionAction != reporting.ReportActionSubmit || stub.transitionID != "weekly-report" {
		t.Fatalf("submit response = %d %s; stub action/id=%q/%q", submitResponse.Code, submitResponse.Body.String(), stub.transitionAction, stub.transitionID)
	}
	stub.transition = reporting.ReportState{ID: "weekly-report", Status: reporting.ReportStatusFinalized, RowVersion: 3}
	finalizeResponse := performJSON(router, http.MethodPost, "/api/v1/reports/weekly-report/finalize", reporting.ReportTransitionInput{RowVersion: 2}, tokens.AccessToken)
	if finalizeResponse.Code != http.StatusOK || stub.transitionAction != reporting.ReportActionFinalized {
		t.Fatalf("finalize response = %d %s; stub action=%q", finalizeResponse.Code, finalizeResponse.Body.String(), stub.transitionAction)
	}
	stub.correctionResult = reporting.ReportCorrectionResult{CorrectionID: "correction-id", Report: reporting.ReportState{ID: "weekly-report", Status: reporting.ReportStatusDraft, RowVersion: 4}}
	correctionResponse := performJSON(router, http.MethodPost, "/api/v1/reports/weekly-report/corrections", reporting.AddReportCorrectionInput{
		RowVersion: 3, RecordID: "record-id", Field: "details.remark", AfterValue: []byte(`"修正后"`), Reason: "修正通报错字",
	}, tokens.AccessToken)
	if correctionResponse.Code != http.StatusCreated || stub.correctionID != "weekly-report" {
		t.Fatalf("correction response = %d %s; stub report=%q", correctionResponse.Code, correctionResponse.Body.String(), stub.correctionID)
	}
	stub.adjustmentResult = reporting.ReportAdjustmentResult{AdjustmentID: "adjustment-id", Report: reporting.ReportState{ID: "weekly-report", Status: reporting.ReportStatusDraft, RowVersion: 5}}
	adjustmentResponse := performJSON(router, http.MethodPost, "/api/v1/reports/weekly-report/adjustments", reporting.AddReportAdjustmentInput{
		RowVersion: 4, RecordID: "record-id", Operation: "EXCLUDE", Reason: "本版排除误纳入记录",
	}, tokens.AccessToken)
	if adjustmentResponse.Code != http.StatusCreated || stub.adjustmentID != "weekly-report" {
		t.Fatalf("adjustment response = %d %s; stub report=%q", adjustmentResponse.Code, adjustmentResponse.Body.String(), stub.adjustmentID)
	}
	stub.decisionState = reporting.ReportState{ID: "weekly-report", Status: reporting.ReportStatusDraft, RowVersion: 5}
	decisionResponse := performJSON(router, http.MethodPost, "/api/v1/report-corrections/correction-id/decision", reporting.DecideReportCorrectionInput{
		RowVersion: 4, Decision: "CONFIRMED",
	}, tokens.AccessToken)
	if decisionResponse.Code != http.StatusOK || stub.decisionID != "correction-id" {
		t.Fatalf("decision response = %d %s; stub correction=%q", decisionResponse.Code, decisionResponse.Body.String(), stub.decisionID)
	}
}

func TestReportingEndpointsMapDomainErrorsAndHideInternalDetails(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(repository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, err := authService.BootstrapAdmin(context.Background(), "report-error-admin", "通报管理员", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, err := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &reportingHTTPStub{}
	router := NewRouter(authService, false, AdminServices{Reporting: stub})

	for _, test := range []struct {
		name     string
		method   string
		path     string
		body     any
		setError func()
		status   int
		code     string
		mustHide string
	}{
		{name: "invalid period", path: "/api/v1/reports/summary?semesterId=x&weekId=y", setError: func() { stub.summaryErr = reporting.ErrInvalidPeriod }, status: http.StatusBadRequest, code: "INVALID_REPORT_PERIOD"},
		{name: "forbidden scope", path: "/api/v1/reports/periods", setError: func() { stub.periodsErr = reporting.ErrForbidden }, status: http.StatusForbidden, code: "FORBIDDEN_SCOPE"},
		{name: "incomplete report", method: http.MethodPost, path: "/api/v1/reports/drafts", body: reporting.GenerateDraftInput{SemesterID: "a0000000-0000-0000-0000-000000000003", WeekID: "a0000000-0000-0000-0000-000000000004"}, setError: func() {
			stub.draftErr = &reporting.IncompleteReportError{Cells: []reporting.IncompleteReportCell{{DepartmentName: "学习部"}}}
		}, status: http.StatusConflict, code: "REPORT_INCOMPLETE"},
		{name: "changed report source", method: http.MethodPost, path: "/api/v1/reports/weekly-report/submit", body: reporting.ReportTransitionInput{RowVersion: 1}, setError: func() {
			stub.transitionErr = reporting.ErrReportSourceChanged
		}, status: http.StatusConflict, code: "REPORT_SOURCE_CHANGED"},
		{name: "invalid report correction", method: http.MethodPost, path: "/api/v1/reports/weekly-report/corrections", body: reporting.AddReportCorrectionInput{
			RowVersion: 1, RecordID: "record-id", Field: "details.remark", AfterValue: []byte(`"x"`), Reason: "修正",
		}, setError: func() { stub.correctionErr = reporting.ErrInvalidReportCorrection }, status: http.StatusBadRequest, code: "INVALID_REPORT_CORRECTION"},
		{name: "internal error", path: "/api/v1/reports/periods", setError: func() { stub.periodsErr = errors.New("private database diagnostic") }, status: http.StatusInternalServerError, code: "REPORT_SUMMARY_FAILED", mustHide: "private database diagnostic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub.periodsErr, stub.summaryErr, stub.draftErr = nil, nil, nil
			stub.transitionErr, stub.correctionErr, stub.decisionErr = nil, nil, nil
			test.setError()
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			response := performJSON(router, method, test.path, test.body, tokens.AccessToken)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) ||
				(test.mustHide != "" && strings.Contains(response.Body.String(), test.mustHide)) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReportErrorLogFieldsExposePostgresClassificationWithoutErrorDetails(t *testing.T) {
	err := &pgconn.PgError{
		Code:           "22P02",
		Message:        "invalid input syntax for type uuid",
		Detail:         "student name: private fixture",
		SchemaName:     "public",
		TableName:      "inspection_records",
		ColumnName:     "id",
		ConstraintName: "inspection_records_pkey",
	}
	fields := reportErrorLogFields(err)
	for _, expected := range []string{"errorType=*pgconn.PgError", "sqlState=22P02", `schema="public"`, `table="inspection_records"`, `column="id"`, `constraint="inspection_records_pkey"`} {
		if !strings.Contains(fields, expected) {
			t.Errorf("diagnostic fields %q do not include %q", fields, expected)
		}
	}
	for _, privateValue := range []string{"student name", "private fixture", "invalid input syntax"} {
		if strings.Contains(fields, privateValue) {
			t.Errorf("diagnostic fields leaked error detail %q: %s", privateValue, fields)
		}
	}
}

func TestReportingEndpointsRequirePasswordChangeBeforeAccess(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(repository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, err := authService.BootstrapAdmin(context.Background(), "report-password-admin", "通报管理员", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, err := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = true
	repository.values[admin.LoginName] = admin
	response := performJSON(NewRouter(authService, false, AdminServices{Reporting: &reportingHTTPStub{}}), http.MethodGet, "/api/v1/reports/periods", nil, tokens.AccessToken)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

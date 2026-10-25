package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5/pgconn"
)

type reportingHTTPStub struct {
	periods      []reporting.PeriodOption
	periodsErr   error
	summary      reporting.Summary
	summaryErr   error
	periodActor  string
	summaryActor string
	semesterID   string
	weekID       string
}

func (s *reportingHTTPStub) Periods(_ context.Context, actorID string) ([]reporting.PeriodOption, error) {
	s.periodActor = actorID
	return s.periods, s.periodsErr
}

func (s *reportingHTTPStub) Summary(_ context.Context, actorID, semesterID, weekID string) (reporting.Summary, error) {
	s.summaryActor, s.semesterID, s.weekID = actorID, semesterID, weekID
	return s.summary, s.summaryErr
}

func (s *reportingHTTPStub) Records(context.Context, string, reporting.RecordsQuery) (reporting.RecordPage, error) {
	return reporting.RecordPage{Items: []reporting.Record{}}, nil
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

	for _, path := range []string{"/api/v1/reports/periods", "/api/v1/reports/summary?semesterId=semester&weekId=week"} {
		if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s response = %d %s", path, response.Code, response.Body.String())
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
		path     string
		setError func()
		status   int
		code     string
		mustHide string
	}{
		{name: "invalid period", path: "/api/v1/reports/summary?semesterId=x&weekId=y", setError: func() { stub.summaryErr = reporting.ErrInvalidPeriod }, status: http.StatusBadRequest, code: "INVALID_REPORT_PERIOD"},
		{name: "forbidden scope", path: "/api/v1/reports/periods", setError: func() { stub.periodsErr = reporting.ErrForbidden }, status: http.StatusForbidden, code: "FORBIDDEN_SCOPE"},
		{name: "internal error", path: "/api/v1/reports/periods", setError: func() { stub.periodsErr = errors.New("private database diagnostic") }, status: http.StatusInternalServerError, code: "REPORT_SUMMARY_FAILED", mustHide: "private database diagnostic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub.periodsErr, stub.summaryErr = nil, nil
			test.setError()
			response := performJSON(router, http.MethodGet, test.path, nil, tokens.AccessToken)
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

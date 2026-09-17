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
	"four-check-management-system/server/internal/inspection"

	"github.com/google/uuid"
)

type manualHTTPStub struct {
	err   error
	actor inspection.ManualActor
	calls int
}

func (s *manualHTTPStub) Create(_ context.Context, a inspection.ManualActor, _ inspection.ManualAttendanceInput, _ audit.Metadata) (inspection.AttendanceRecord, error) {
	s.actor = a
	s.calls++
	return inspection.AttendanceRecord{ID: "created", Source: "MANUAL", Status: "DRAFT", RowVersion: 1}, s.err
}
func TestManualAttendanceHTTPContract(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	password := uuid.NewString() + "A1"
	service := auth.NewService(repository, &sessions{values: map[string]auth.RefreshSession{}}, []byte(uuid.NewString()), time.Minute, time.Hour)
	account, err := service.BootstrapAdmin(context.Background(), "manual-test", "测试账号", password)
	if err != nil {
		t.Fatal(err)
	}
	account.MustChangePassword = false
	repository.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, password)
	if err != nil {
		t.Fatal(err)
	}
	stub := &manualHTTPStub{}
	router := NewRouter(service, false, AdminServices{ManualAttendance: stub})
	body := map[string]any{"semesterId": uuid.NewString(), "weekId": uuid.NewString(), "inspectionTypeId": uuid.NewString(), "departmentId": uuid.NewString(), "classId": uuid.NewString(), "grade": "2026级"}
	response := performJSON(router, http.MethodPost, "/api/v1/inspections/attendance", body, "")
	if response.Code != 401 || stub.calls != 0 {
		t.Fatalf("unauthenticated status=%d calls=%d", response.Code, stub.calls)
	}
	for _, scenario := range []struct {
		err    error
		status int
		code   string
	}{
		{nil, 201, `"source":"MANUAL"`},
		{inspection.ErrScopeMismatch, 403, "FORBIDDEN_SCOPE"},
		{inspection.ErrAttendanceDuplicate, 409, "RECORD_ALREADY_EXISTS"},
		{&inspection.ManualValidationError{Message: "姓名不一致"}, 422, "VALIDATION_FAILED"},
		{errors.New("private SQL details"), 500, "ATTENDANCE_CREATE_FAILED"},
	} {
		stub.err = scenario.err
		response = performJSON(router, http.MethodPost, "/api/v1/inspections/attendance", body, tokens.AccessToken)
		if response.Code != scenario.status || !strings.Contains(response.Body.String(), scenario.code) || strings.Contains(response.Body.String(), "private SQL") {
			t.Fatalf("status=%d expected=%d", response.Code, scenario.status)
		}
	}
	if stub.actor.ID != account.ID || !stub.actor.IsSuperAdmin {
		t.Fatal("incorrect authenticated actor")
	}
	body["departmentId"] = "not-a-uuid"
	before := stub.calls
	response = performJSON(router, http.MethodPost, "/api/v1/inspections/attendance", body, tokens.AccessToken)
	if response.Code != 400 || stub.calls != before {
		t.Fatalf("invalid input status=%d", response.Code)
	}
	account.MustChangePassword = true
	repository.values[account.LoginName] = account
	response = performJSON(router, http.MethodPost, "/api/v1/inspections/attendance", body, tokens.AccessToken)
	if response.Code != 403 || !strings.Contains(response.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("password guard=%d", response.Code)
	}
	account.Status = auth.StatusDisabled
	repository.values[account.LoginName] = account
	response = performJSON(router, http.MethodPost, "/api/v1/inspections/attendance", body, tokens.AccessToken)
	if response.Code == 201 || stub.calls != before {
		t.Fatal("disabled account reached creation")
	}
}

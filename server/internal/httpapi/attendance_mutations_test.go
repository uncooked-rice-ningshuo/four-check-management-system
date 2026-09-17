package httpapi

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"testing"
	"time"
)

type mutationHTTPStub struct {
	err   error
	calls int
	actor inspection.ManualActor
	items []inspection.VersionedRecord
}

func (s *mutationHTTPStub) Detail(_ context.Context, a inspection.ManualActor, _ string) (inspection.AttendanceRecord, error) {
	s.calls++
	s.actor = a
	return inspection.AttendanceRecord{ID: "r", RowVersion: 2}, s.err
}
func (s *mutationHTTPStub) Update(_ context.Context, a inspection.ManualActor, _ string, _ inspection.AttendanceEditInput, _ audit.Metadata) (inspection.AttendanceRecord, error) {
	return s.Detail(context.Background(), a, "")
}
func (s *mutationHTTPStub) Delete(_ context.Context, a inspection.ManualActor, i inspection.DeleteAttendanceInput, _ audit.Metadata) error {
	s.calls++
	s.actor = a
	s.items = i.Items
	return s.err
}
func TestAttendanceMutationHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	password := uuid.NewString() + "A1"
	authService := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte(uuid.NewString()), time.Minute, time.Hour)
	account, err := authService.BootstrapAdmin(context.Background(), "mutation-test", "测试", password)
	if err != nil {
		t.Fatal(err)
	}
	account.MustChangePassword = false
	repo.values[account.LoginName] = account
	tokens, err := authService.Login(context.Background(), account.LoginName, password)
	if err != nil {
		t.Fatal(err)
	}
	stub := &mutationHTTPStub{}
	router := NewRouter(authService, false, AdminServices{AttendanceMutations: stub})
	id := uuid.NewString()
	record := map[string]any{"semesterId": uuid.NewString(), "weekId": uuid.NewString(), "departmentId": uuid.NewString(), "inspectionTypeId": uuid.NewString(), "classId": uuid.NewString()}
	for _, operation := range []struct {
		method, path string
		body         any
		success      int
	}{
		{http.MethodGet, "/api/v1/inspections/attendance/" + id, nil, 200},
		{http.MethodPatch, "/api/v1/inspections/attendance/" + id, map[string]any{"version": 1, "reason": "核对", "record": record}, 200},
		{http.MethodDelete, "/api/v1/inspections/attendance/" + id, map[string]any{"version": 1, "reason": "核对"}, 204},
		{http.MethodPost, "/api/v1/inspections/attendance/batch", map[string]any{"action": "DELETE", "items": []inspection.VersionedRecord{{ID: id, Version: 1}}, "reason": "核对"}, 204},
	} {
		before := stub.calls
		response := performJSON(router, operation.method, operation.path, operation.body, "")
		if response.Code != 401 || stub.calls != before {
			t.Fatalf("unauthorized %s: %d", operation.method, response.Code)
		}
		for _, scenario := range []struct {
			err    error
			status int
			code   string
		}{
			{nil, operation.success, ""}, {inspection.ErrRecordNotFound, 404, "INSPECTION_NOT_FOUND"}, {inspection.ErrScopeMismatch, 403, "FORBIDDEN_SCOPE"}, {inspection.ErrRecordVersionConflict, 409, "VERSION_CONFLICT"}, {inspection.ErrRecordNotEditable, 409, "RECORD_NOT_EDITABLE"}, {inspection.ErrInvalidAttendanceRecord, 422, "VALIDATION_FAILED"}, {errors.New("private SQL"), 500, "INSPECTION_MUTATION_FAILED"},
		} {
			stub.err = scenario.err
			response = performJSON(router, operation.method, operation.path, operation.body, tokens.AccessToken)
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), scenario.code) || strings.Contains(response.Body.String(), "private SQL") {
				t.Fatalf("%s status=%d expected=%d", operation.method, response.Code, scenario.status)
			}
		}
	}
	if stub.actor.ID != account.ID {
		t.Fatal("actor is not authenticated user")
	}
	if len(stub.items) != 1 || stub.items[0].ID != id || stub.items[0].Version != 1 {
		t.Fatal("batch versions not forwarded")
	}
	before := stub.calls
	response := performJSON(router, http.MethodPost, "/api/v1/inspections/attendance/batch", map[string]string{"action": "UPDATE"}, tokens.AccessToken)
	if response.Code != 400 || stub.calls != before {
		t.Fatal("unsupported action reached service")
	}
}

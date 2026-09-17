package httpapi

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

type inspectionHTTPStub struct {
	err   error
	calls int
	actor inspection.ManualActor
}

func (s *inspectionHTTPStub) Create(_ context.Context, a inspection.ManualActor, _ inspection.InspectionInput, _ audit.Metadata) (inspection.InspectionRecord, error) {
	s.calls++
	s.actor = a
	return inspection.InspectionRecord{ID: uuid.NewString(), Status: "DRAFT", Source: "MANUAL", RowVersion: 1}, s.err
}
func (s *inspectionHTTPStub) Detail(_ context.Context, a inspection.ManualActor, _ string) (inspection.InspectionRecord, error) {
	s.calls++
	s.actor = a
	return inspection.InspectionRecord{Status: "DRAFT"}, s.err
}
func (s *inspectionHTTPStub) List(_ context.Context, a inspection.ManualActor, _ inspection.InspectionFilter) (inspection.InspectionPage, error) {
	s.calls++
	s.actor = a
	return inspection.InspectionPage{Items: []inspection.InspectionRecord{}, Page: 1, PageSize: 20}, s.err
}
func (s *inspectionHTTPStub) Update(_ context.Context, a inspection.ManualActor, _ string, _ inspection.InspectionEdit, _ audit.Metadata) (inspection.InspectionRecord, error) {
	s.calls++
	s.actor = a
	return inspection.InspectionRecord{RowVersion: 2}, s.err
}
func (s *inspectionHTTPStub) Delete(_ context.Context, a inspection.ManualActor, _ inspection.DeleteAttendanceInput, _ audit.Metadata) error {
	s.calls++
	s.actor = a
	return s.err
}
func TestInspectionHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	password := uuid.NewString() + "A1"
	svc := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte(uuid.NewString()), time.Minute, time.Hour)
	account, err := svc.BootstrapAdmin(context.Background(), "inspection-http", "测试账号", password)
	if err != nil {
		t.Fatal(err)
	}
	account.MustChangePassword = false
	repo.values[account.LoginName] = account
	tokens, err := svc.Login(context.Background(), account.LoginName, password)
	if err != nil {
		t.Fatal(err)
	}
	stub := &inspectionHTTPStub{}
	router := NewRouter(svc, false, AdminServices{Inspections: stub})
	id := uuid.NewString()
	input := map[string]any{"semesterId": id, "weekId": id, "inspectionTypeId": id, "departmentId": id, "details": map[string]any{}}
	for _, route := range []struct {
		method, path string
		body         any
		status       int
	}{
		{"POST", "/api/v1/inspections", input, 201}, {"GET", "/api/v1/inspections", nil, 200}, {"GET", "/api/v1/inspections/" + id, nil, 200},
		{"PATCH", "/api/v1/inspections/" + id, map[string]any{"version": 1, "reason": "核对", "record": input}, 200},
		{"DELETE", "/api/v1/inspections/" + id, map[string]any{"version": 1, "reason": "重复"}, 204},
		{"POST", "/api/v1/inspections/batch", map[string]any{"action": "DELETE", "items": []inspection.VersionedRecord{{ID: id, Version: 1}}, "reason": "重复"}, 204},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			before := stub.calls
			r := performJSON(router, route.method, route.path, route.body, "")
			if r.Code != 401 || stub.calls != before {
				t.Fatal("authentication bypass")
			}
			stub.err = nil
			r = performJSON(router, route.method, route.path, route.body, tokens.AccessToken)
			if r.Code != route.status {
				t.Fatalf("%d %s", r.Code, r.Body.String())
			}
			for _, failure := range []struct {
				err    error
				status int
				code   string
			}{{inspection.ErrInspectionRequest, 400, "INVALID_REQUEST"}, {inspection.ErrScopeMismatch, 403, "FORBIDDEN_SCOPE"}, {inspection.ErrRecordNotFound, 404, "INSPECTION_NOT_FOUND"}, {inspection.ErrRecordVersionConflict, 409, "VERSION_CONFLICT"}, {inspection.ErrRecordNotEditable, 409, "RECORD_NOT_EDITABLE"}, {inspection.ErrAttendanceDuplicate, 409, "RECORD_ALREADY_EXISTS"}, {errors.New("private SQL"), 500, "INSPECTION_FAILED"}} {
				stub.err = failure.err
				r = performJSON(router, route.method, route.path, route.body, tokens.AccessToken)
				if r.Code != failure.status || !strings.Contains(r.Body.String(), failure.code) || strings.Contains(r.Body.String(), "private SQL") {
					t.Fatalf("%d %s", r.Code, r.Body.String())
				}
			}
		})
	}
	stub.err = nil
	if stub.actor.ID != account.ID {
		t.Fatal("untrusted actor")
	}
	r := performJSON(router, "GET", "/api/v1/inspections?page=bad", nil, tokens.AccessToken)
	if r.Code != 400 {
		t.Fatal("invalid pagination")
	}
	account.MustChangePassword = true
	repo.values[account.LoginName] = account
	r = performJSON(router, "GET", "/api/v1/inspections", nil, tokens.AccessToken)
	if r.Code != 403 {
		t.Fatal("password guard")
	}
}

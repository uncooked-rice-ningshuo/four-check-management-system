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
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
)

type rollbackHTTPStub struct {
	err      error
	input    importjob.RollbackInput
	actor    inspection.ManualActor
	metadata audit.Metadata
	calls    int
}

func (s *rollbackHTTPStub) Rollback(_ context.Context, a inspection.ManualActor, i importjob.RollbackInput, m audit.Metadata) (importjob.RollbackResult, error) {
	s.calls++
	s.input = i
	s.actor = a
	s.metadata = m
	return importjob.RollbackResult{BatchID: i.BatchID, Status: "ROLLED_BACK", RolledBackRows: 2, Version: i.Version + 1}, s.err
}

func TestImportRollbackHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	service := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := service.Register(context.Background(), auth.RegisterInput{StudentNo: "rollback-member", Name: "rollback", Password: "Password123", RequestedDepartmentID: "department", RequestedOrgRoleID: "member"})
	if err != nil {
		t.Fatal(err)
	}
	account := repo.values["rollback-member"]
	account.Status = auth.StatusActive
	repo.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &rollbackHTTPStub{}
	router := NewRouter(service, false, AdminServices{ImportRollback: stub})
	path := "/api/v1/imports/batch-id/rollback"
	if r := performJSON(router, http.MethodPost, path, map[string]any{"version": 4, "reason": "误导入"}, ""); r.Code != 401 || stub.calls != 0 {
		t.Fatal("unauthenticated request reached rollback", r.Code)
	}
	r := performJSON(router, http.MethodPost, path, map[string]any{"version": 4, "reason": "误导入"}, tokens.AccessToken)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"rolledBackRows":2`) || !strings.Contains(r.Body.String(), `"version":5`) || stub.actor.ID != account.ID || stub.input.Reason != "误导入" || stub.metadata.RequestID == "" {
		t.Fatalf("contract %d %s", r.Code, r.Body.String())
	}
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{importjob.ErrBatchNotFound, 404, "IMPORT_BATCH_NOT_FOUND"},
		{importjob.ErrBatchScopeMismatch, 403, "FORBIDDEN_SCOPE"},
		{importjob.ErrRollbackUnsafe, 409, "IMPORT_ROLLBACK_UNSAFE"},
		{importjob.ErrBatchVersionConflict, 409, "VERSION_CONFLICT"},
		{importjob.ErrInvalidBatchTransition, 409, "INVALID_BATCH_STATE"},
		{importjob.ErrInvalidBatch, 400, "INVALID_IMPORT_REQUEST"},
		{errors.New("private SQL failure"), 500, "IMPORT_SERVICE_UNAVAILABLE"},
	} {
		stub.err = tt.err
		r := performJSON(router, http.MethodPost, path, map[string]any{"version": 4, "reason": "误导入"}, tokens.AccessToken)
		if r.Code != tt.status || !strings.Contains(r.Body.String(), `"code":"`+tt.code+`"`) || strings.Contains(r.Body.String(), "private SQL") {
			t.Fatalf("error %v: %d %s", tt.err, r.Code, r.Body.String())
		}
	}
}

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/academicimport"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
)

type academicWeekImportHTTPStub struct {
	batch       academicimport.Batch
	confirm     academicimport.ConfirmInput
	actorID     string
	confirmResp academicimport.ConfirmResult
	err         error
}

func (s *academicWeekImportHTTPStub) Preview(context.Context, string, string, string, string, []byte, audit.Metadata) (academicimport.Batch, error) {
	return s.batch, s.err
}
func (s *academicWeekImportHTTPStub) GetBatch(_ context.Context, _ string, actorID string) (academicimport.Batch, error) {
	s.actorID = actorID
	return s.batch, s.err
}
func (s *academicWeekImportHTTPStub) Confirm(_ context.Context, input academicimport.ConfirmInput) (academicimport.ConfirmResult, error) {
	s.confirm = input
	return s.confirmResp, s.err
}

func TestAcademicWeekImportRoutesRequireSuperAdmin(t *testing.T) {
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := authService.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	accountsRepository.values[admin.LoginName] = admin
	member, err := authService.AdminCreateUser(context.Background(), admin, auth.AdminCreateUserInput{
		StudentNo: "23010001", Name: "普通用户", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}
	member.MustChangePassword = false
	accountsRepository.values[member.LoginName] = member
	adminTokens, _ := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	memberTokens, _ := authService.Login(context.Background(), member.LoginName, "TemporaryPassword123")
	router := NewRouter(authService, false, AdminServices{AcademicWeekImports: &academicWeekImportHTTPStub{}})
	path := "/api/v1/admin/academic/weeks/import-spec"

	if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodGet, path, nil, memberTokens.AccessToken); response.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodGet, path, nil, adminTokens.AccessToken); response.Code != http.StatusOK {
		t.Fatalf("super admin status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAcademicWeekImportBatchUsesCurrentActorAndIdempotencyHeader(t *testing.T) {
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := authService.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	accountsRepository.values[admin.LoginName] = admin
	tokens, _ := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	stub := &academicWeekImportHTTPStub{confirmResp: academicimport.ConfirmResult{BatchID: "batch-1", Status: academicimport.StatusConfirmed, ImportedRows: 2, Version: 2}}
	router := NewRouter(authService, false, AdminServices{AcademicWeekImports: stub})

	get := performJSON(router, http.MethodGet, "/api/v1/admin/academic/weeks/imports/batch-1", nil, tokens.AccessToken)
	if get.Code != http.StatusOK || stub.actorID != admin.ID {
		t.Fatalf("get status=%d actor=%q body=%s", get.Code, stub.actorID, get.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/academic/weeks/imports/batch-1/confirm", strings.NewReader(`{"version":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Idempotency-Key", "retry-key-0123456789")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || stub.confirm.ActorID != admin.ID || stub.confirm.ExpectedVersion != 1 || stub.confirm.IdempotencyKey != "retry-key-0123456789" {
		t.Fatalf("confirm status=%d input=%+v body=%s", response.Code, stub.confirm, response.Body.String())
	}
}

package httpapi

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/organization"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failingAuthAccounts struct {
	*accounts
	stage string
	reads int
}

func (r *failingAuthAccounts) FindByLoginName(ctx context.Context, name string) (auth.Account, error) {
	if r.stage == "login-read" {
		return auth.Account{}, errors.New("private account diagnostic")
	}
	return r.accounts.FindByLoginName(ctx, name)
}
func (r *failingAuthAccounts) FindByID(ctx context.Context, id string) (auth.Account, error) {
	if r.stage == "account-read" {
		return auth.Account{}, errors.New("private account diagnostic")
	}
	account, err := r.accounts.FindByID(ctx, id)
	if r.stage == "changed-after-auth" {
		r.reads++
		if r.reads > 1 {
			account.SessionVersion++
		}
	}
	return account, err
}
func (r *failingAuthAccounts) UpdatePassword(ctx context.Context, id, hash string, mustChange bool, version int, mutation audit.Mutation) error {
	if r.stage == "password-write" {
		return errors.New("private password diagnostic")
	}
	if r.stage == "password-conflict" {
		return auth.ErrVersionConflict
	}
	return r.accounts.UpdatePassword(ctx, id, hash, mustChange, version, mutation)
}

func (r *failingAuthAccounts) UpdateAccount(ctx context.Context, account auth.Account, mutation audit.Mutation) (auth.Account, error) {
	if r.stage == "account-conflict" {
		return auth.Account{}, auth.ErrVersionConflict
	}
	return r.accounts.UpdateAccount(ctx, account, mutation)
}
func (r *failingAuthAccounts) SetStatus(ctx context.Context, id, status string, expectedVersion int, mutation audit.Mutation) (auth.Account, error) {
	if r.stage == "status-conflict" {
		return auth.Account{}, auth.ErrVersionConflict
	}
	return r.accounts.SetStatus(ctx, id, status, expectedVersion, mutation)
}

type failingAuthSessions struct {
	*sessions
	stage string
}

func (s *failingAuthSessions) Save(ctx context.Context, value auth.RefreshSession, version int) error {
	if s.stage == "session-save" {
		return errors.New("private session diagnostic")
	}
	return s.sessions.Save(ctx, value, version)
}
func (s *failingAuthSessions) Revoke(ctx context.Context, hash string, now time.Time) error {
	if s.stage == "session-revoke" {
		return errors.New("private session diagnostic")
	}
	return s.sessions.Revoke(ctx, hash, now)
}

func TestAuthInfrastructureErrorsAreRetryableAndDoNotClearCookie(t *testing.T) {
	for _, stage := range []string{"login-read", "session-save", "account-read", "password-write", "session-revoke"} {
		t.Run(stage, func(t *testing.T) {
			accountsRepo := &failingAuthAccounts{accounts: &accounts{values: map[string]auth.Account{}}}
			sessionsRepo := &failingAuthSessions{sessions: &sessions{values: map[string]auth.RefreshSession{}}}
			svc := auth.NewService(accountsRepo, sessionsRepo, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			ctx := context.Background()
			if _, err := svc.BootstrapAdmin(ctx, "error-fixture", "Error fixture", "InitialPassword123"); err != nil {
				t.Fatal(err)
			}
			tokens, err := svc.Login(ctx, "error-fixture", "InitialPassword123")
			if err != nil {
				t.Fatal(err)
			}
			accountsRepo.stage = stage
			sessionsRepo.stage = stage
			router := NewRouter(svc, false)
			path, method, body := "/api/v1/auth/login", http.MethodPost, `{"loginName":"error-fixture","password":"InitialPassword123"}`
			switch stage {
			case "account-read":
				path, method, body = "/api/v1/auth/me", http.MethodGet, ""
			case "password-write":
				path, body = "/api/v1/auth/change-password", `{"currentPassword":"InitialPassword123","newPassword":"ChangedPassword456"}`
			case "session-revoke":
				path, body = "/api/v1/auth/logout", ""
			}
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
			r.AddCookie(&http.Cookie{Name: "refresh_token", Value: tokens.RefreshToken})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != 500 || !strings.Contains(w.Body.String(), "AUTH_SERVICE_UNAVAILABLE") || strings.Contains(w.Body.String(), "private") || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("stage=%s status=%d body=%s", stage, w.Code, w.Body.String())
			}
			accountsRepo.stage = ""
			sessionsRepo.stage = ""
			if _, err = svc.Refresh(ctx, tokens.RefreshToken); err != nil {
				t.Fatal("failed operation invalidated prior session", err)
			}
		})
	}
}

func TestPasswordVersionConflictIsExplicitForSelfAndAdmin(t *testing.T) {
	repo := &failingAuthAccounts{accounts: &accounts{values: map[string]auth.Account{}}}
	svc := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	ctx := context.Background()
	admin, err := svc.BootstrapAdmin(ctx, "conflict-admin", "Conflict fixture", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = false
	repo.values[admin.LoginName] = admin
	user, err := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: "conflict-user", Name: "Conflict user", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := svc.Login(ctx, admin.LoginName, "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	repo.stage = "password-conflict"
	router := NewRouter(svc, false)
	for _, path := range []string{"/api/v1/auth/change-password", "/api/v1/admin/users/" + user.ID + "/reset-password"} {
		response := performJSON(router, http.MethodPost, path, map[string]any{"currentPassword": "InitialPassword123", "newPassword": "ChangedPassword456", "version": 1}, tokens.AccessToken)
		if response.Code != 409 || !strings.Contains(response.Body.String(), "VERSION_CONFLICT") || response.Header().Get("Set-Cookie") != "" || strings.Contains(response.Body.String(), "temporaryPassword") {
			t.Fatalf("conflict status=%d body=%s", response.Code, response.Body.String())
		}
	}

	for _, operation := range []struct {
		stage, path string
		body        map[string]any
	}{
		{"account-conflict", "/api/v1/admin/users/" + user.ID, map[string]any{"studentNo": user.StudentNo, "name": "Updated name", "systemRole": auth.SystemRoleMember, "version": 1}},
		{"status-conflict", "/api/v1/admin/users/" + user.ID + "/status", map[string]any{"status": auth.StatusDisabled, "version": 1}},
	} {
		repo.stage = operation.stage
		response := performJSON(router, http.MethodPut, operation.path, operation.body, tokens.AccessToken)
		if response.Code != 409 || !strings.Contains(response.Body.String(), "VERSION_CONFLICT") || response.Header().Get("Set-Cookie") != "" {
			t.Fatalf("account conflict=%d %s", response.Code, response.Body.String())
		}
	}
	repo.stage = "changed-after-auth"
	repo.reads = 0
	response := performJSON(router, http.MethodPost, "/api/v1/auth/change-password", map[string]string{"currentPassword": "InitialPassword123", "newPassword": "ChangedPassword456"}, tokens.AccessToken)
	if response.Code != 401 || !strings.Contains(response.Body.String(), "UNAUTHORIZED") || response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("old actor status=%d body=%s", response.Code, response.Body.String())
	}
}

type assignmentHTTPRepository struct {
	attendanceOrganizationRepository
	users    []organization.UserAssignment
	writeErr error
}

func (r *assignmentHTTPRepository) ListUsers(context.Context) ([]organization.UserAssignment, error) {
	return r.users, nil
}
func (r *assignmentHTTPRepository) AssignUserRole(context.Context, organization.UserAssignment, audit.Mutation) error {
	return r.writeErr
}
func TestOrganizationAssignmentMissingConflictAndAuthorization(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	ctx := context.Background()
	admin, err := svc.BootstrapAdmin(ctx, "assignment-http", "Admin", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	admin.MustChangePassword = false
	repo.values[admin.LoginName] = admin
	user, err := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: "assignment-person", Name: "Person", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
	if err != nil {
		t.Fatal(err)
	}
	user.MustChangePassword = false
	repo.values[user.LoginName] = user
	adminTokens, err := svc.Login(ctx, admin.LoginName, "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	userTokens, err := svc.Login(ctx, user.LoginName, "TemporaryPassword123")
	if err != nil {
		t.Fatal(err)
	}
	organizationRepo := &assignmentHTTPRepository{users: []organization.UserAssignment{{AccountID: user.ID, Name: user.Name, Status: user.Status, SystemRole: user.SystemRole}}, writeErr: auth.ErrVersionConflict}
	router := NewRouter(svc, false, AdminServices{Organization: organization.NewService(organizationRepo)})
	body := map[string]any{"departmentId": "department", "organizationRoleId": "role", "version": 1}
	for _, test := range []struct {
		id, token, code string
		status          int
	}{{user.ID, "", "UNAUTHORIZED", 401}, {user.ID, userTokens.AccessToken, "FORBIDDEN", 403}, {"missing", adminTokens.AccessToken, "RESOURCE_NOT_FOUND", 404}, {user.ID, adminTokens.AccessToken, "VERSION_CONFLICT", 409}} {
		response := performJSON(router, http.MethodPut, "/api/v1/admin/users/"+test.id+"/assignment", body, test.token)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("assignment status=%d body=%s", response.Code, response.Body.String())
		}
	}
	organizationRepo.writeErr = organization.ErrInvalidInput
	response := performJSON(router, http.MethodPut, "/api/v1/admin/users/"+user.ID+"/assignment", body, adminTokens.AccessToken)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("assignment invalid reference status=%d body=%s", response.Code, response.Body.String())
	}
}

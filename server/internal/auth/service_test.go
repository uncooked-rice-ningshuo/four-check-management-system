package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
)

type memoryAccounts struct {
	byStudentNo map[string]Account
	mutations   []audit.Mutation
}

func newMemoryAccounts() *memoryAccounts { return &memoryAccounts{byStudentNo: map[string]Account{}} }

func (m *memoryAccounts) Create(_ context.Context, account Account, mutation audit.Mutation) error {
	if _, ok := m.byStudentNo[account.LoginName]; ok {
		return ErrLoginNameExists
	}
	m.byStudentNo[account.LoginName] = account
	m.mutations = append(m.mutations, mutation)
	return nil
}

func (m *memoryAccounts) FindByLoginName(_ context.Context, loginName string) (Account, error) {
	a, ok := m.byStudentNo[loginName]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return a, nil
}

func (m *memoryAccounts) UpdatePassword(_ context.Context, id, hash string, mustChange bool, version int, mutation audit.Mutation) error {
	for key, a := range m.byStudentNo {
		if a.ID == id {
			if a.SessionVersion != version {
				return ErrVersionConflict
			}
			a.PasswordHash = hash
			a.MustChangePassword = mustChange
			a.SessionVersion++
			m.byStudentNo[key] = a
			m.mutations = append(m.mutations, mutation)
			return nil
		}
	}
	return ErrAccountNotFound
}

func (m *memoryAccounts) ListPending(_ context.Context) ([]Account, error) {
	var result []Account
	for _, a := range m.byStudentNo {
		if a.Status == StatusPending {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *memoryAccounts) ReviewRegistration(_ context.Context, accountID, _ string, decision, _ string, _ time.Time, mutation audit.Mutation) (Account, error) {
	for key, a := range m.byStudentNo {
		if a.ID != accountID {
			continue
		}
		if a.Status != StatusPending {
			return Account{}, ErrInvalidTransition
		}
		if decision == ReviewApprove {
			a.Status = StatusActive
			a.PrimaryDepartmentID = a.RequestedDepartmentID
		} else {
			a.Status = StatusRejected
		}
		m.byStudentNo[key] = a
		m.mutations = append(m.mutations, mutation)
		return a, nil
	}
	return Account{}, ErrAccountNotFound
}

func (m *memoryAccounts) FindByID(_ context.Context, id string) (Account, error) {
	for _, a := range m.byStudentNo {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

func (m *memoryAccounts) UpdateAccount(_ context.Context, account Account, mutation audit.Mutation) (Account, error) {
	for key, value := range m.byStudentNo {
		if value.ID == account.ID {
			if value.SessionVersion != account.SessionVersion {
				return Account{}, ErrVersionConflict
			}
			delete(m.byStudentNo, key)
			account.PasswordHash = value.PasswordHash
			account.SessionVersion = value.SessionVersion + 1
			m.byStudentNo[account.LoginName] = account
			m.mutations = append(m.mutations, mutation)
			return account, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

func (m *memoryAccounts) SetStatus(_ context.Context, accountID, status string, expectedVersion int, mutation audit.Mutation) (Account, error) {
	for key, value := range m.byStudentNo {
		if value.ID == accountID {
			if value.SessionVersion != expectedVersion {
				return Account{}, ErrVersionConflict
			}
			value.Status = status
			value.SessionVersion++
			m.byStudentNo[key] = value
			m.mutations = append(m.mutations, mutation)
			return value, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

type memorySessions struct{ values map[string]RefreshSession }

func newMemorySessions() *memorySessions { return &memorySessions{values: map[string]RefreshSession{}} }
func (m *memorySessions) Save(_ context.Context, s RefreshSession, _ int) error {
	m.values[s.TokenHash] = s
	return nil
}
func (m *memorySessions) FindRefreshSession(_ context.Context, hash string, now time.Time) (RefreshSession, error) {
	s, ok := m.values[hash]
	if !ok || s.RevokedAt != nil || !s.ExpiresAt.After(now) {
		return RefreshSession{}, ErrInvalidRefreshToken
	}
	return s, nil
}
func (m *memorySessions) Rotate(ctx context.Context, hash string, now time.Time, next RefreshSession, _ int) error {
	s, err := m.FindRefreshSession(ctx, hash, now)
	if err != nil {
		return err
	}
	if s.AccountID != next.AccountID {
		return ErrInvalidRefreshToken
	}
	s.RevokedAt = &now
	m.values[hash] = s
	m.values[next.TokenHash] = next
	return nil
}
func (m *memorySessions) Revoke(_ context.Context, hash string, now time.Time) error {
	s, ok := m.values[hash]
	if !ok {
		return nil
	}
	s.RevokedAt = &now
	m.values[hash] = s
	return nil
}
func (m *memorySessions) RevokeAll(_ context.Context, accountID string, now time.Time) error {
	for key, session := range m.values {
		if session.AccountID == accountID && session.RevokedAt == nil {
			session.RevokedAt = &now
			m.values[key] = session
		}
	}
	return nil
}

func testService() *Service {
	return NewService(newMemoryAccounts(), newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
}

func TestAdminUserMutationsProvideSafeAuditEntries(t *testing.T) {
	repository := newMemoryAccounts()
	service := NewService(repository, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin := Account{ID: "admin-1", IsSuperAdmin: true}
	created, err := service.AdminCreateUser(context.Background(), admin, AdminCreateUserInput{
		StudentNo: "23019999", Name: "审计用户", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember,
	}, audit.Metadata{RequestID: "request-create-user"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Metadata.ActorID != admin.ID || mutation.Entry.Action != audit.ActionAccountCreated || mutation.Entry.ResourceID != created.ID {
		t.Fatalf("mutation = %+v", mutation)
	}
	if serialized := fmt.Sprintf("%v", mutation.Entry.After); strings.Contains(serialized, "TemporaryPassword123") || strings.Contains(strings.ToLower(serialized), "passwordhash") {
		t.Fatalf("audit entry contains password material: %s", serialized)
	}

	if err := service.AdminResetPassword(context.Background(), admin, created.ID, "ReplacementPassword123", audit.Metadata{RequestID: "request-reset"}); err != nil {
		t.Fatal(err)
	}
	reset := repository.mutations[1]
	if reset.Entry.Action != audit.ActionAccountPasswordReset {
		t.Fatalf("reset mutation = %+v", reset)
	}
	if serialized := fmt.Sprintf("%v", reset.Entry.After); strings.Contains(serialized, "ReplacementPassword123") || strings.Contains(strings.ToLower(serialized), "passwordhash") {
		t.Fatalf("reset audit entry contains password material: %s", serialized)
	}
}

func TestUnauthorizedAdminMutationDoesNotCreateSuccessAudit(t *testing.T) {
	repository := newMemoryAccounts()
	service := NewService(repository, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	_, err := service.AdminCreateUser(context.Background(), Account{ID: "member-1"}, AdminCreateUserInput{
		StudentNo: "23018888", Name: "无权限用户", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember,
	}, audit.Metadata{RequestID: "request-forbidden"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
	if len(repository.mutations) != 0 {
		t.Fatalf("unauthorized audit mutations = %d", len(repository.mutations))
	}
}

func TestRegisterHashesPassword(t *testing.T) {
	s := testService()
	a, err := s.Register(context.Background(), RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.PasswordHash == "Password123" || a.PasswordHash == "" {
		t.Fatal("password was not hashed")
	}
}

func TestRegisterCreatesPendingPersonAccount(t *testing.T) {
	s := testService()
	a, err := s.Register(context.Background(), RegisterInput{
		StudentNo: "22010102", Name: "李四", Password: "Password123",
		RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusPending {
		t.Fatalf("status = %s, want %s", a.Status, StatusPending)
	}
	if a.LoginName != a.StudentNo || a.AccountKind != AccountKindPerson {
		t.Fatalf("unexpected account identity: %+v", a)
	}
}

func TestBootstrapAdminCreatesSystemAccountAndIsIdempotent(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	first, err := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.BootstrapAdmin(context.Background(), "admin", "不会覆盖", "DifferentPassword123")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !first.IsSuperAdmin || !first.MustChangePassword || first.StudentNo != "" {
		t.Fatalf("unexpected bootstrap account: %+v", first)
	}
}

func TestPendingAccountCannotLoginUntilApproved(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	person, _ := s.Register(context.Background(), RegisterInput{
		StudentNo: "22010103", Name: "王五", Password: "Password123",
		RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1",
	})
	if _, err := s.Login(context.Background(), person.LoginName, "Password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("pending login error = %v", err)
	}
	if _, err := s.ReviewRegistration(context.Background(), admin, person.ID, ReviewApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(context.Background(), person.LoginName, "Password123"); err != nil {
		t.Fatalf("approved login error = %v", err)
	}
}

func TestFirstPasswordChangeClearsRequirement(t *testing.T) {
	repo := newMemoryAccounts()
	sessions := newMemorySessions()
	s := NewService(repo, sessions, []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	if err := s.ChangePassword(context.Background(), admin, "InitialPassword123", "ReplacementPassword123"); err != nil {
		t.Fatal(err)
	}
	updated, _ := repo.FindByID(context.Background(), admin.ID)
	if updated.MustChangePassword {
		t.Fatal("password change requirement was not cleared")
	}
	if _, err := s.Login(context.Background(), "admin", "ReplacementPassword123"); err != nil {
		t.Fatalf("login with replacement password: %v", err)
	}
}

func TestPasswordChangeInvalidatesExistingAccessToken(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	tokens, err := s.Login(context.Background(), "admin", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(context.Background(), admin, "InitialPassword123", "ReplacementPassword123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateAccessToken(context.Background(), tokens.AccessToken); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("old access token error = %v", err)
	}
}

func TestRegisterRejectsDuplicateStudentNumber(t *testing.T) {
	s := testService()
	in := RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"}
	if _, err := s.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(context.Background(), in); !errors.Is(err, ErrStudentNoExists) {
		t.Fatalf("error = %v", err)
	}
}

func TestLoginAndCurrentUser(t *testing.T) {
	s := testService()
	a, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	tokens, err := s.Login(context.Background(), "admin", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.AuthenticateAccessToken(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != a.ID {
		t.Fatalf("user id = %s, want %s", current.ID, a.ID)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	s := testService()
	_, _ = s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	if _, err := s.Login(context.Background(), "admin", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
}

func TestRefreshRotatesTokenAndLogoutRevokesIt(t *testing.T) {
	s := testService()
	_, _ = s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	first, _ := s.Login(context.Background(), "admin", "InitialPassword123")
	second, err := s.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := s.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("old token error = %v", err)
	}
	if err := s.Logout(context.Background(), second.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(context.Background(), second.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("logged-out token error = %v", err)
	}
}

type failingRefreshSessions struct {
	*memorySessions
	fail bool
}

func (m *failingRefreshSessions) Save(ctx context.Context, session RefreshSession, version int) error {
	if m.fail {
		return errors.New("injected session persistence failure")
	}
	return m.memorySessions.Save(ctx, session, version)
}
func (m *failingRefreshSessions) Rotate(ctx context.Context, hash string, now time.Time, next RefreshSession, version int) error {
	if m.fail {
		return errors.New("injected session persistence failure")
	}
	return m.memorySessions.Rotate(ctx, hash, now, next, version)
}

func TestRefreshPersistenceFailureLeavesOldSessionRetryable(t *testing.T) {
	s := testService()
	repository := &failingRefreshSessions{memorySessions: newMemorySessions()}
	s.sessions = repository
	ctx := context.Background()
	if _, err := s.BootstrapAdmin(ctx, "rotation-admin", "Rotation fixture", "InitialPassword123"); err != nil {
		t.Fatal(err)
	}
	first, err := s.Login(ctx, "rotation-admin", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	repository.fail = true
	if _, err = s.Refresh(ctx, first.RefreshToken); err == nil || errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatal("persistence failure must remain an infrastructure error")
	}
	repository.fail = false
	if _, err = s.Refresh(ctx, first.RefreshToken); err != nil {
		t.Fatalf("old session cannot retry after rolled-back refresh: %v", err)
	}
}

func TestAdminCreatesActiveUserWithTemporaryPassword(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	created, err := s.AdminCreateUser(context.Background(), admin, AdminCreateUserInput{StudentNo: "23010001", Name: "新用户", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusActive || !created.MustChangePassword || created.PasswordHash == "TemporaryPassword123" {
		t.Fatalf("created = %+v", created)
	}
}

func TestAdminUpdatesUserProfileAndSystemRole(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	user, _ := s.AdminCreateUser(context.Background(), admin, AdminCreateUserInput{StudentNo: "23010002", Name: "原姓名", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember})
	updated, err := s.AdminUpdateUser(context.Background(), admin, user.ID, AdminUpdateUserInput{Name: "新姓名", StudentNo: "23010002", SystemRole: SystemRoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "新姓名" || updated.SystemRole != SystemRoleAdmin {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestAdminDisablesUserAndInvalidatesSessions(t *testing.T) {
	repo := newMemoryAccounts()
	sessions := newMemorySessions()
	s := NewService(repo, sessions, []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	user, _ := s.AdminCreateUser(context.Background(), admin, AdminCreateUserInput{StudentNo: "23010003", Name: "用户", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember})
	tokens, _ := s.Login(context.Background(), user.LoginName, "TemporaryPassword123")
	if _, err := s.AdminSetUserStatus(context.Background(), admin, user.ID, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(context.Background(), tokens.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh error = %v", err)
	}
}

func TestAdminResetPasswordRequiresFirstLoginChange(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	user, _ := s.AdminCreateUser(context.Background(), admin, AdminCreateUserInput{StudentNo: "23010004", Name: "用户", TemporaryPassword: "TemporaryPassword123", SystemRole: SystemRoleMember})
	if err := s.AdminResetPassword(context.Background(), admin, user.ID, "ReplacementPassword123"); err != nil {
		t.Fatal(err)
	}
	updated, _ := repo.FindByID(context.Background(), user.ID)
	if !updated.MustChangePassword {
		t.Fatal("reset password must require a password change")
	}
}

func TestAdminCannotDisableSuperAdmin(t *testing.T) {
	repo := newMemoryAccounts()
	s := NewService(repo, newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
	admin, _ := s.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	if _, err := s.AdminSetUserStatus(context.Background(), admin, admin.ID, StatusDisabled); !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}

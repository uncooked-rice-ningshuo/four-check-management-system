package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryAccounts struct{ byStudentNo map[string]Account }

func newMemoryAccounts() *memoryAccounts { return &memoryAccounts{byStudentNo: map[string]Account{}} }

func (m *memoryAccounts) Create(_ context.Context, account Account) error {
	if _, ok := m.byStudentNo[account.StudentNo]; ok {
		return ErrStudentNoExists
	}
	m.byStudentNo[account.StudentNo] = account
	return nil
}

func (m *memoryAccounts) FindByStudentNo(_ context.Context, studentNo string) (Account, error) {
	a, ok := m.byStudentNo[studentNo]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return a, nil
}

func (m *memoryAccounts) FindByID(_ context.Context, id string) (Account, error) {
	for _, a := range m.byStudentNo {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

type memorySessions struct{ values map[string]RefreshSession }

func newMemorySessions() *memorySessions { return &memorySessions{values: map[string]RefreshSession{}} }
func (m *memorySessions) Save(_ context.Context, s RefreshSession) error {
	m.values[s.TokenHash] = s
	return nil
}
func (m *memorySessions) Consume(_ context.Context, hash string, now time.Time) (RefreshSession, error) {
	s, ok := m.values[hash]
	if !ok || s.RevokedAt != nil || !s.ExpiresAt.After(now) {
		return RefreshSession{}, ErrInvalidRefreshToken
	}
	t := now
	s.RevokedAt = &t
	m.values[hash] = s
	return s, nil
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

func testService() *Service {
	return NewService(newMemoryAccounts(), newMemorySessions(), []byte("01234567890123456789012345678901"), 15*time.Minute, 7*24*time.Hour)
}

func TestRegisterCreatesActiveAccount(t *testing.T) {
	s := testService()
	a, err := s.Register(context.Background(), RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusActive {
		t.Fatalf("status = %s, want %s", a.Status, StatusActive)
	}
	if a.PasswordHash == "Password123" || a.PasswordHash == "" {
		t.Fatal("password was not hashed")
	}
}

func TestRegisterRejectsDuplicateStudentNumber(t *testing.T) {
	s := testService()
	in := RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123"}
	if _, err := s.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(context.Background(), in); !errors.Is(err, ErrStudentNoExists) {
		t.Fatalf("error = %v", err)
	}
}

func TestLoginAndCurrentUser(t *testing.T) {
	s := testService()
	a, _ := s.Register(context.Background(), RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123"})
	tokens, err := s.Login(context.Background(), "22010101", "Password123")
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
	_, _ = s.Register(context.Background(), RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123"})
	if _, err := s.Login(context.Background(), "22010101", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
}

func TestRefreshRotatesTokenAndLogoutRevokesIt(t *testing.T) {
	s := testService()
	_, _ = s.Register(context.Background(), RegisterInput{StudentNo: "22010101", Name: "张三", Password: "Password123"})
	first, _ := s.Login(context.Background(), "22010101", "Password123")
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

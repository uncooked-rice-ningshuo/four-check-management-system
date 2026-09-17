package httpapi

import (
	"context"
	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

type workContextStub struct {
	err   error
	calls int
}

func (s *workContextStub) ReadWorkContext(context.Context, time.Time) (academic.WorkContextSnapshot, error) {
	return academic.WorkContextSnapshot{Version: 1}, nil
}
func (s *workContextStub) SaveWeekOverride(context.Context, academic.WeekOverrideInput, time.Time, audit.Metadata) error {
	s.calls++
	return s.err
}
func TestWorkContextHTTP(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	password := uuid.NewString() + "A1"
	svc := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte(uuid.NewString()), time.Minute, time.Hour)
	a, e := svc.BootstrapAdmin(context.Background(), "work-context-test", "测试账号", password)
	if e != nil {
		t.Fatal(e)
	}
	a.MustChangePassword = false
	repo.values[a.LoginName] = a
	tokens, e := svc.Login(context.Background(), a.LoginName, password)
	if e != nil {
		t.Fatal(e)
	}
	stub := &workContextStub{}
	router := NewRouter(svc, false, AdminServices{WorkContext: academic.NewWorkContextService(stub)})
	if r := performJSON(router, "GET", "/api/v1/work-context", nil, ""); r.Code != 401 {
		t.Fatal(r.Code)
	}
	if r := performJSON(router, "GET", "/api/v1/work-context", nil, tokens.AccessToken); r.Code != 200 || !strings.Contains(r.Body.String(), `"source":"NONE"`) {
		t.Fatal(r.Code, r.Body.String())
	}
	body := map[string]any{"version": 1, "reason": "恢复", "weekId": "", "expiresAt": nil}
	if r := performJSON(router, "PUT", "/api/v1/work-context", body, tokens.AccessToken); r.Code != 200 {
		t.Fatal(r.Code)
	}
	stub.err = academic.ErrWorkContextConflict
	if r := performJSON(router, "PUT", "/api/v1/work-context", body, tokens.AccessToken); r.Code != 409 {
		t.Fatal(r.Code)
	}
	a.IsSuperAdmin = false
	a.SystemRole = "ADMIN"
	repo.values[a.LoginName] = a
	count := stub.calls
	if r := performJSON(router, "PUT", "/api/v1/work-context", body, tokens.AccessToken); r.Code != 403 || stub.calls != count {
		t.Fatal("privilege bypass", r.Code)
	}
}

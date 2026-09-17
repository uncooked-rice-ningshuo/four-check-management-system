package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dashboard"
)

type dashboardHTTPStub struct {
	actor dashboard.Actor
	value dashboard.Workbench
}

func (s *dashboardHTTPStub) Workbench(_ context.Context, actor dashboard.Actor, _ time.Time) (dashboard.Workbench, error) {
	s.actor = actor
	return s.value, nil
}

func TestWorkbenchRequiresAuthenticationAndUsesCurrentActor(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(repository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := authService.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := authService.Login(context.Background(), admin.LoginName, "InitialPassword123")
	dashboardService := &dashboardHTTPStub{value: dashboard.Workbench{MyTodos: dashboard.MyTodos{Total: 2}, DepartmentStatuses: []dashboard.DepartmentStatus{}}}
	router := NewRouter(authService, false, AdminServices{Dashboard: dashboardService})

	if response := performJSON(router, http.MethodGet, "/api/v1/dashboard/workbench", nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated response = %d %s", response.Code, response.Body.String())
	}
	response := performJSON(router, http.MethodGet, "/api/v1/dashboard/workbench", nil, tokens.AccessToken)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":2`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if dashboardService.actor.ID != admin.ID || !dashboardService.actor.IsSuperAdmin {
		t.Fatalf("actor = %#v", dashboardService.actor)
	}
}

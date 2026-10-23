package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failingSessionRepository struct {
	*sessions
	stage string
}

func (s *failingSessionRepository) FindRefreshSession(ctx context.Context, hash string, now time.Time) (auth.RefreshSession, error) {
	if s.stage == "read" {
		return auth.RefreshSession{}, errors.New("private database diagnostic")
	}
	return s.sessions.FindRefreshSession(ctx, hash, now)
}
func (s *failingSessionRepository) Rotate(ctx context.Context, hash string, now time.Time, replacement auth.RefreshSession, version int) error {
	if s.stage == "write" {
		return errors.New("private database diagnostic")
	}
	return s.sessions.Rotate(ctx, hash, now, replacement, version)
}

func TestRefreshInfrastructureFailurePreservesCookieAndRetry(t *testing.T) {
	for _, stage := range []string{"read", "write"} {
		t.Run(stage, func(t *testing.T) {
			repo := &failingSessionRepository{sessions: &sessions{values: map[string]auth.RefreshSession{}}}
			svc := auth.NewService(&accounts{values: map[string]auth.Account{}}, repo, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			ctx := context.Background()
			if _, err := svc.BootstrapAdmin(ctx, "refresh-fixture", "Refresh fixture", "InitialPassword123"); err != nil {
				t.Fatal(err)
			}
			tokens, err := svc.Login(ctx, "refresh-fixture", "InitialPassword123")
			if err != nil {
				t.Fatal(err)
			}
			router := NewRouter(svc, false)
			request := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
				r.AddCookie(&http.Cookie{Name: "refresh_token", Value: tokens.RefreshToken})
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w
			}
			repo.stage = stage
			failed := request()
			if failed.Code != 500 || !strings.Contains(failed.Body.String(), "AUTH_SERVICE_UNAVAILABLE") || strings.Contains(failed.Body.String(), "private") || failed.Header().Get("Set-Cookie") != "" {
				t.Fatalf("failure response status=%d body=%s", failed.Code, failed.Body.String())
			}
			repo.stage = ""
			ok := request()
			var body map[string]any
			if err = json.Unmarshal(ok.Body.Bytes(), &body); err != nil || ok.Code != 200 || body["accessToken"] == nil || len(ok.Result().Cookies()) != 1 {
				t.Fatal("retry did not rotate credentials")
			}
			replay := request()
			if replay.Code != 401 || !strings.Contains(replay.Body.String(), "INVALID_SESSION") || len(replay.Result().Cookies()) != 1 || replay.Result().Cookies()[0].MaxAge >= 0 {
				t.Fatal("consumed token must be rejected and cookie cleared")
			}
		})
	}
}

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
)

type accounts struct{ values map[string]auth.Account }

func (r *accounts) Create(_ context.Context, a auth.Account) error {
	if _, ok := r.values[a.StudentNo]; ok {
		return auth.ErrStudentNoExists
	}
	r.values[a.StudentNo] = a
	return nil
}
func (r *accounts) FindByStudentNo(_ context.Context, no string) (auth.Account, error) {
	a, ok := r.values[no]
	if !ok {
		return auth.Account{}, auth.ErrAccountNotFound
	}
	return a, nil
}
func (r *accounts) FindByID(_ context.Context, id string) (auth.Account, error) {
	for _, a := range r.values {
		if a.ID == id {
			return a, nil
		}
	}
	return auth.Account{}, auth.ErrAccountNotFound
}

type sessions struct {
	values map[string]auth.RefreshSession
}

func (r *sessions) Save(_ context.Context, s auth.RefreshSession) error {
	r.values[s.TokenHash] = s
	return nil
}
func (r *sessions) Consume(_ context.Context, h string, now time.Time) (auth.RefreshSession, error) {
	s, ok := r.values[h]
	if !ok || s.RevokedAt != nil {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	s.RevokedAt = &now
	r.values[h] = s
	return s, nil
}
func (r *sessions) Revoke(_ context.Context, h string, now time.Time) error {
	s := r.values[h]
	s.RevokedAt = &now
	r.values[h] = s
	return nil
}

func TestRegisterLoginAndMe(t *testing.T) {
	svc := auth.NewService(&accounts{map[string]auth.Account{}}, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	r := NewRouter(svc, false)

	register := performJSON(r, http.MethodPost, "/api/v1/auth/register", map[string]string{"studentNo": "22010101", "name": "张三", "password": "Password123"}, "")
	if register.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", register.Code, register.Body.String())
	}
	login := performJSON(r, http.MethodPost, "/api/v1/auth/login", map[string]string{"studentNo": "22010101", "password": "Password123"}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var body struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &body)
	me := performJSON(r, http.MethodGet, "/api/v1/auth/me", nil, body.AccessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}
}

type httpHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

func performJSON(h httpHandler, method, path string, payload any, token string) *httptest.ResponseRecorder {
	var data []byte
	if payload != nil {
		data, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

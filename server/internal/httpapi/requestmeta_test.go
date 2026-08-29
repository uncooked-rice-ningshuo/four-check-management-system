package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"four-check-management-system/server/internal/auth"
	"github.com/gin-gonic/gin"
)

func TestRequestMetadataMiddlewareGeneratesRequestID(t *testing.T) {
	recorder, metadata := performMetadataRequest(t, nil, func(request *http.Request) {
		request.Header.Set("User-Agent", "test-agent")
	})
	requestID := recorder.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("X-Request-ID is empty")
	}
	if metadata.RequestID != requestID {
		t.Fatalf("metadata request ID = %q, response header = %q", metadata.RequestID, requestID)
	}
}

func TestRequestMetadataMiddlewareAcceptsValidIncomingRequestID(t *testing.T) {
	recorder, metadata := performMetadataRequest(t, nil, func(request *http.Request) {
		request.Header.Set("X-Request-ID", "request_2026-08-30.01")
	})
	if got := recorder.Header().Get("X-Request-ID"); got != "request_2026-08-30.01" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if metadata.RequestID != "request_2026-08-30.01" {
		t.Fatalf("metadata request ID = %q", metadata.RequestID)
	}
}

func TestRequestMetadataLimitsUserAgentAndUsesAuthenticatedActor(t *testing.T) {
	_, metadata := performMetadataRequest(t, nil, func(request *http.Request) {
		request.Header.Set("User-Agent", strings.Repeat("浏览器", 300))
		request.Header.Set("Authorization", "Bearer must-not-enter-audit-metadata")
		request.Header.Set("Cookie", "refresh_token=must-not-enter-audit-metadata")
	})
	if got := len([]rune(metadata.UserAgent)); got != 512 {
		t.Fatalf("User-Agent length = %d, want 512", got)
	}
	if metadata.ActorID != "account-1" {
		t.Fatalf("actor ID = %q", metadata.ActorID)
	}
}

func TestRequestMetadataUsesForwardedIPOnlyFromTrustedProxy(t *testing.T) {
	_, trusted := performMetadataRequest(t, []string{"10.0.0.0/8"}, func(request *http.Request) {
		request.RemoteAddr = "10.1.2.3:4321"
		request.Header.Set("X-Forwarded-For", "203.0.113.9")
	})
	if trusted.IP != "203.0.113.9" {
		t.Fatalf("trusted proxy IP = %q", trusted.IP)
	}

	_, untrusted := performMetadataRequest(t, []string{"10.0.0.0/8"}, func(request *http.Request) {
		request.RemoteAddr = "192.0.2.8:4321"
		request.Header.Set("X-Forwarded-For", "203.0.113.10")
	})
	if untrusted.IP != "192.0.2.8" {
		t.Fatalf("untrusted proxy IP = %q", untrusted.IP)
	}
}

func performMetadataRequest(t *testing.T, trustedProxies []string, configure func(*http.Request)) (*httptest.ResponseRecorder, auditMetadataResult) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := ConfigureTrustedProxies(router, strings.Join(trustedProxies, ",")); err != nil {
		t.Fatal(err)
	}
	router.Use(requestMetadataMiddleware())
	var captured auditMetadataResult
	router.GET("/metadata", func(c *gin.Context) {
		c.Set("account", auth.Account{ID: "account-1"})
		metadata := requestMetadata(c, "测试原因")
		captured = auditMetadataResult{
			ActorID: metadata.ActorID, RequestID: metadata.RequestID, IP: metadata.IP,
			UserAgent: metadata.UserAgent, Reason: metadata.Reason,
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/metadata", nil)
	configure(request)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder, captured
}

type auditMetadataResult struct {
	ActorID   string
	RequestID string
	IP        string
	UserAgent string
	Reason    string
}

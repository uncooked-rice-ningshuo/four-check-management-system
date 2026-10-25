package httpapi

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLogRecordsLifecycleWithoutSensitiveInput(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)

	router := gin.New()
	router.Use(requestMetadataMiddleware(), requestLogMiddleware())
	router.GET("/api/v1/items/:id", func(c *gin.Context) {
		logRequestStage(c, "handler", "lookup")
		c.Status(http.StatusForbidden)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/items/private-student-id?name=secret-name", nil)
	request.Header.Set("Authorization", "Bearer secret-token")
	request.Header.Set("Cookie", "refresh_token=secret-cookie")
	request.Header.Set("X-Request-ID", "request-123")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	got := output.String()
	for _, expected := range []string{"requestId=request-123", "route=/api/v1/items/:id", "stage=request result=started", "stage=handler result=lookup", "api request completed:", "status=403", "durationMs="} {
		if !strings.Contains(got, expected) {
			t.Errorf("log missing %q: %s", expected, got)
		}
	}
	for _, secret := range []string{"private-student-id", "secret-name", "secret-token", "secret-cookie"} {
		if strings.Contains(got, secret) {
			t.Errorf("log contains sensitive input %q", secret)
		}
	}
}

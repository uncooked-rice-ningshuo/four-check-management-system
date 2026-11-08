package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"four-check-management-system/server/internal/reportoutput"

	"github.com/gin-gonic/gin"
)

func TestReportOutputInternalErrorUsesOutputCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/reports/output-settings/editor", nil)
	(&handler{}).writeOutputError(context, errors.New("private database diagnostic"))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"REPORT_OUTPUT_FAILED"`) {
		t.Fatalf("unexpected output error: status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private database diagnostic") {
		t.Fatal("internal database diagnostic leaked to response")
	}
}

func TestReportOutputMissingWeekDateHasStableError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/reports/workflow/semester/week/versions/1/outputs", nil)
	(&handler{}).writeOutputError(context, reportoutput.ErrIssueDateMissing)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"REPORT_OUTPUT_DATE_REQUIRED"`) {
		t.Fatalf("unexpected output error: status=%d body=%s", response.Code, response.Body.String())
	}
}

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/masterdataimport"
	"github.com/gin-gonic/gin"
)

func TestMasterDataImportTemplateResponseIsNotCachedAndUsesCurrentContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/master-data/students/import-template", nil)
	context.Params = gin.Params{{Key: "kind", Value: masterdata.KindStudent}}

	(&handler{}).downloadMasterDataImportTemplate(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	parser, err := masterdataimport.NewParser(masterdata.KindStudent, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("students.xlsx", recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("downloaded template does not match current parser contract: %v", err)
	}
	if parsed.TemplateVersion != 4 || len(parsed.Rows) != 0 {
		t.Fatalf("downloaded template contract = version %d with %d imported rows, want version 4 with no rows", parsed.TemplateVersion, len(parsed.Rows))
	}
}

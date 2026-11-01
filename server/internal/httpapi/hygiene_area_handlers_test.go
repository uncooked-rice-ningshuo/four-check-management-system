package httpapi

import (
	"net/http"
	"testing"
)

func TestHygieneAreaConfigurationEndpointsAreRetired(t *testing.T) {
	router := NewRouter(nil, false, AdminServices{})
	for _, path := range []string{
		"/api/v1/admin/hygiene-areas/options",
		"/api/v1/admin/hygiene-areas/classroom-hygiene-areas/references",
		"/api/v1/admin/hygiene-areas/public-dormitory-hygiene-areas",
	} {
		if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusNotFound {
			t.Fatalf("retired hygiene configuration endpoint %s status=%d", path, response.Code)
		}
	}
}

package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"github.com/gin-gonic/gin"
)

func TestDomainErrorsSeparateInvalidInputFromInternalFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"academic input", academic.ErrInvalidInput, 400, "INVALID_REQUEST"},
		{"masterdata input", masterdata.ErrInvalidRecord, 400, "INVALID_REQUEST"},
		{"masterdata relation", masterdata.ErrInvalidRelation, 400, "INVALID_REQUEST"},
		{"masterdata kind", masterdata.ErrInvalidKind, 400, "INVALID_REQUEST"},
		{"immutable identifier", masterdata.ErrImmutableCode, 400, "INVALID_REQUEST"},
		{"dictionary input", dictionary.ErrInvalidDictionary, 400, "INVALID_REQUEST"},
		{"organization input", organization.ErrInvalidInput, 400, "INVALID_REQUEST"},
		{"permission input", permission.ErrInvalidRule, 400, "INVALID_REQUEST"},
		{"account input", auth.ErrInvalidInput, 400, "INVALID_REQUEST"},
		{"not found", masterdata.ErrNotFound, 404, "RESOURCE_NOT_FOUND"},
		{"dictionary missing", dictionary.ErrNotFound, 404, "RESOURCE_NOT_FOUND"},
		{"account missing", auth.ErrAccountNotFound, 404, "RESOURCE_NOT_FOUND"},
		{"calendar conflict", academic.ErrDateOverlap, 409, "ACADEMIC_DATE_OVERLAP"},
		{"dictionary duplicate", dictionary.ErrDuplicateValue, 409, "DICTIONARY_VALUE_EXISTS"},
		{"system dictionary", dictionary.ErrSystemTypeProtected, 409, "DICTIONARY_TYPE_PROTECTED"},
		{"account duplicate", auth.ErrLoginNameExists, 409, "ACCOUNT_IDENTIFIER_EXISTS"},
		{"forbidden", auth.ErrForbidden, 403, "FORBIDDEN"},
		{"state", auth.ErrInvalidTransition, 409, "INVALID_STATE"},
		{"unexpected repository error", errors.New("private database diagnostic"), 500, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest("GET", "/api/v1/admin/master-data/grades", nil)
			writeDomainError(c, fmt.Errorf("repository: %w", tc.err))
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if r.Code != tc.status || body.Code != tc.code || body.Message == "" {
				t.Fatalf("got %d %s", r.Code, r.Body.String())
			}
			if strings.Contains(r.Body.String(), "private database diagnostic") {
				t.Fatal("internal diagnostic leaked")
			}
		})
	}
}

func TestDomainInternalFailureDoesNotLogRawRepositoryError(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/master-data/grades", nil)
	writeDomainError(c, errors.New("private database diagnostic"))
	if output.Len() == 0 || strings.Contains(output.String(), "private database diagnostic") {
		t.Fatalf("expected safe diagnostic metadata, got %q", output.String())
	}
}

func TestInspectionValidationUsesTopLevelErrorContract(t *testing.T) {
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	writeInspectionError(c, &inspection.DetailsValidationError{Issues: []inspection.DetailIssue{{Path: "details.studentId", Code: "REQUIRED", Message: "请选择学生"}}})
	var body struct {
		Code    string                   `json:"code"`
		Details []inspection.DetailIssue `json:"details"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if r.Code != 422 || body.Code != "VALIDATION_FAILED" || len(body.Details) != 1 || body.Details[0].Path != "details.studentId" {
		t.Fatalf("invalid public error contract: %d %s", r.Code, r.Body.String())
	}
}

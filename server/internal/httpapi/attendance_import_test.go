package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
)

type importHTTPStub struct {
	batch        importjob.Batch
	rows         importjob.ImportRowPage
	template     importjob.Template
	parseErr     error
	confirmErr   error
	confirmKey   string
	confirmInput importjob.ConfirmBatchInput
	createErr    error
	discarded    bool
}

func (s *importHTTPStub) CreateBatch(_ context.Context, input importjob.CreateBatchInput) (importjob.Batch, error) {
	if s.createErr != nil {
		return importjob.Batch{}, s.createErr
	}
	s.batch = importjob.Batch{ID: "batch-1", InspectionTypeID: input.InspectionTypeID, SemesterID: input.SemesterID, WeekID: input.WeekID, DepartmentID: input.DepartmentID, ClassID: input.ClassID, Grade: input.Grade, OriginalFileID: input.SourceFile.ID, TemplateID: input.TemplateID, TemplateVersion: input.TemplateVersion, Status: importjob.BatchStatusUploaded, Version: 1, CreatedBy: input.CreatedBy}
	return s.batch, nil
}
func (s *importHTTPStub) ParseBatch(_ context.Context, input importjob.ParseBatchInput) (importjob.Batch, error) {
	if s.parseErr != nil {
		return importjob.Batch{}, s.parseErr
	}
	s.batch.Status, s.batch.Version = importjob.BatchStatusPreviewReady, input.ExpectedVersion+2
	return s.batch, nil
}
func (s *importHTTPStub) GetBatch(_ context.Context, id string) (importjob.Batch, error) {
	if id != s.batch.ID {
		return importjob.Batch{}, importjob.ErrBatchNotFound
	}
	return s.batch, nil
}
func (s *importHTTPStub) ListRows(_ context.Context, _ string, page, pageSize int) (importjob.ImportRowPage, error) {
	s.rows.Page, s.rows.PageSize = page, pageSize
	return s.rows, nil
}
func (s *importHTTPStub) PublishedTemplate(context.Context, string) (importjob.Template, error) {
	return s.template, nil
}
func (s *importHTTPStub) ConfirmBatch(_ context.Context, input importjob.ConfirmBatchInput) (importjob.ConfirmResult, error) {
	s.confirmInput = input
	if s.confirmErr != nil {
		return importjob.ConfirmResult{}, s.confirmErr
	}
	return importjob.ConfirmResult{BatchID: input.BatchID, Status: importjob.BatchStatusConfirmed, ImportedRows: 1, Version: input.ExpectedVersion + 1}, nil
}
func (s *importHTTPStub) CancelBatch(_ context.Context, id string, version int) (importjob.Batch, error) {
	if version != s.batch.Version {
		return importjob.Batch{}, importjob.ErrBatchVersionConflict
	}
	s.batch.Status, s.batch.Version = importjob.BatchStatusCancelled, version+1
	return s.batch, nil
}

type fileHTTPStub struct {
	stored    storage.File
	content   []byte
	storeErr  error
	metadata  audit.Metadata
	onDiscard func()
}

func (s *fileHTTPStub) Store(_ context.Context, input storage.StoreInput, metadata audit.Metadata) (storage.File, error) {
	s.metadata = metadata
	if s.storeErr != nil {
		return storage.File{}, s.storeErr
	}
	s.stored = storage.File{ID: "file-1", OriginalName: input.OriginalName, Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	return s.stored, nil
}
func (s *fileHTTPStub) ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error) {
	return storage.File{ID: "template-file", OriginalName: "到课率导入模板.xlsx", Status: storage.StatusAvailable}, s.content, nil
}
func (s *fileHTTPStub) Discard(context.Context, string, audit.Metadata) error {
	if s.onDiscard != nil {
		s.onDiscard()
	}
	return nil
}

type attendanceHTTPStub struct {
	page   inspection.AttendancePage
	err    error
	filter inspection.AttendanceListFilter
	scope  inspection.AttendanceScope
}

func (s *attendanceHTTPStub) ListAttendance(_ context.Context, filter inspection.AttendanceListFilter, scope inspection.AttendanceScope) (inspection.AttendancePage, error) {
	s.filter, s.scope = filter, scope
	return s.page, s.err
}

func attendanceRouter(t *testing.T, allowed bool) (http.Handler, string, *importHTTPStub, *attendanceHTTPStub) {
	t.Helper()
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, _ = authService.Register(context.Background(), auth.RegisterInput{StudentNo: "24010088", Name: "接口测试", Password: "Password123", RequestedDepartmentID: "department-study", RequestedOrgRoleID: "role-member"})
	account := accountsRepository.values["24010088"]
	account.Status = auth.StatusActive
	account.PrimaryDepartmentID = "department-study"
	accountsRepository.values[account.LoginName] = account
	tokens, _ := authService.Login(context.Background(), account.LoginName, "Password123")
	effect := permission.EffectDeny
	if allowed {
		effect = permission.EffectAllow
	}
	permissionRepository := &permissionRepository{profile: permission.AccessProfile{AccountID: account.ID, SystemRole: permission.SystemRoleMember, Rules: []permission.Rule{
		{Action: permission.ActionInspectionView, Effect: effect, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "department-study"}},
		{Action: permission.ActionInspectionEnter, Effect: effect, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "department-study"}},
		{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "department-study", ClassID: "blocked-class"}},
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "department-study", ClassID: "blocked-class"}},
	}}}
	imports := &importHTTPStub{
		batch:    importjob.Batch{ID: "batch-1", InspectionTypeID: "type-attendance", SemesterID: "semester-1", WeekID: "week-1", DepartmentID: "department-study", ClassID: "allowed-class", Status: importjob.BatchStatusPreviewReady, Version: 3, CreatedBy: account.ID},
		rows:     importjob.ImportRowPage{Items: []importjob.ImportRow{}, Total: 0},
		template: importjob.Template{ID: "template-1", InspectionTypeID: "type-attendance", Version: 1, FileID: "template-file", Status: "PUBLISHED"},
	}
	attendance := &attendanceHTTPStub{page: inspection.AttendancePage{Items: []inspection.AttendanceRecord{}, Page: 1, PageSize: 20}}
	files := &fileHTTPStub{content: []byte("xlsx-template"), onDiscard: func() { imports.discarded = true }}
	router := NewRouter(authService, false, AdminServices{Permission: permission.NewService(permissionRepository), ImportJobs: imports, Attendance: attendance, ImportFiles: files, ImportMaxUploadBytes: 4096})
	return router, tokens.AccessToken, imports, attendance
}

func TestAttendanceImportRoutesRequireAuthenticationAndEffectiveScope(t *testing.T) {
	router, token, _, _ := attendanceRouter(t, true)
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study&classId=allowed-class&page=1&pageSize=20"
	if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d %s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodGet, strings.Replace(path, "department-study", "other-department", 1), nil, token); response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("cross scope = %d %s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodGet, strings.Replace(path, "classId=allowed-class", "classId=blocked-class", 1), nil, token); response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("cross class scope = %d %s", response.Code, response.Body.String())
	}
	unbounded := strings.Replace(path, "&classId=allowed-class", "", 1)
	if response := performJSON(router, http.MethodGet, unbounded, nil, token); response.Code != http.StatusForbidden {
		t.Fatalf("unbounded deny overlap = %d %s", response.Code, response.Body.String())
	}
	denied, deniedToken, _, _ := attendanceRouter(t, false)
	if response := performJSON(denied, http.MethodGet, path, nil, deniedToken); response.Code != http.StatusForbidden {
		t.Fatalf("deny = %d %s", response.Code, response.Body.String())
	}
}

func TestAttendanceListReturnsStablePagination(t *testing.T) {
	router, token, _, attendance := attendanceRouter(t, true)
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study&classId=allowed-class&page=2&pageSize=25&sort=studentNoAsc"
	response := performJSON(router, http.MethodGet, path, nil, token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || attendance.filter.Page != 2 || attendance.filter.PageSize != 25 || attendance.scope.DepartmentID != "department-study" {
		t.Fatalf("response=%d %s filter=%#v scope=%#v", response.Code, response.Body.String(), attendance.filter, attendance.scope)
	}
}

func TestEffectivePermissionsReturnsOnlyCurrentAccountProjection(t *testing.T) {
	router, token, _, _ := attendanceRouter(t, true)
	response := performJSON(router, http.MethodGet, "/api/v1/permissions/effective", nil, token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), permission.ActionInspectionView) || strings.Contains(response.Body.String(), "Password123") {
		t.Fatalf("permissions=%d %s", response.Code, response.Body.String())
	}
}

func TestAttendanceTemplateDownloadAndUploadAreProtectedAndSafe(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	contextQuery := "semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study&classId=allowed-class"
	download := performJSON(router, http.MethodGet, "/api/v1/imports/attendance/template?"+contextQuery, nil, token)
	if download.Code != http.StatusOK || download.Header().Get("Content-Disposition") == "" || download.Header().Get("X-Import-Template-ID") != "template-1" || download.Header().Get("X-Import-Template-Version") != "1" || !bytes.Equal(download.Body.Bytes(), []byte("xlsx-template")) {
		t.Fatalf("download=%d headers=%v body=%q", download.Code, download.Header(), download.Body.Bytes())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "department-study", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "1"} {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile("file", "attendance.xlsx")
	_, _ = part.Write([]byte("small-xlsx"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || imports.batch.Status != importjob.BatchStatusPreviewReady {
		t.Fatalf("upload=%d %s batch=%#v", response.Code, response.Body.String(), imports.batch)
	}
}

func TestAttendanceUploadRejectsStaleTemplateBeforePersistingSourceFile(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "department-study", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "2"} {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile("file", "attendance.xlsx")
	_, _ = part.Write([]byte("small-xlsx"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") || imports.batch.OriginalFileID != "" {
		t.Fatalf("stale template=%d %s batch=%#v", response.Code, response.Body.String(), imports.batch)
	}
}

func TestAttendanceUploadDiscardsSourceWhenBatchCreationFails(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.createErr = importjob.ErrInvalidBatch
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "department-study", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "1"} {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile("file", "attendance.xlsx")
	_, _ = part.Write([]byte("small-xlsx"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !imports.discarded {
		t.Fatalf("batch failure=%d %s discarded=%v", response.Code, response.Body.String(), imports.discarded)
	}
}

func TestImportBatchByIDReauthorizesAndMapsVersionIdempotencyErrors(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.batch.DepartmentID = "other-department"
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/batch-1", nil, token); response.Code != http.StatusNotFound {
		t.Fatalf("idor=%d %s", response.Code, response.Body.String())
	}
	imports.batch.DepartmentID = "department-study"
	imports.batch.ClassID = "blocked-class"
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/batch-1", nil, token); response.Code != http.StatusNotFound {
		t.Fatalf("class idor=%d %s", response.Code, response.Body.String())
	}
	imports.batch.ClassID = "allowed-class"
	preview := performJSON(router, http.MethodGet, "/api/v1/imports/batch-1/rows?page=2&pageSize=10", nil, token)
	if preview.Code != http.StatusOK || imports.rows.Page != 2 || imports.rows.PageSize != 10 {
		t.Fatalf("preview=%d %s page=%#v", preview.Code, preview.Body.String(), imports.rows)
	}

	imports.confirmErr = importjob.ErrBatchVersionConflict
	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/imports/batch-1/confirm", strings.NewReader(`{"version":2}`))
	confirm.Header.Set("Content-Type", "application/json")
	confirm.Header.Set("Authorization", "Bearer "+token)
	confirm.Header.Set("Idempotency-Key", "confirm-1")
	confirmed := httptest.NewRecorder()
	router.ServeHTTP(confirmed, confirm)
	if confirmed.Code != http.StatusConflict || !strings.Contains(confirmed.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("confirm=%d %s", confirmed.Code, confirmed.Body.String())
	}
	if imports.confirmInput.IdempotencyKey != "confirm-1" {
		t.Fatalf("idempotency key=%q", imports.confirmInput.IdempotencyKey)
	}
	if imports.confirmInput.AuditMetadata.ActorID == "" {
		t.Fatal("confirm audit actor is empty")
	}

	cancel := performJSON(router, http.MethodPost, "/api/v1/imports/batch-1/cancel", map[string]int{"version": 2}, token)
	if cancel.Code != http.StatusConflict || !strings.Contains(cancel.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("cancel=%d %s", cancel.Code, cancel.Body.String())
	}
}

func TestAttendanceImportRejectsOversizedMultipartBody(t *testing.T) {
	router, token, _, _ := attendanceRouter(t, true)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "attendance.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(strings.Repeat("x", 2<<20)))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "FILE_TOO_LARGE") {
		t.Fatalf("oversized=%d %s", response.Code, response.Body.String())
	}
}

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

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
)

type attendanceOrganizationRepository struct {
	catalogs organization.Catalogs
	scopes   []organization.DepartmentInspectionScope
}

func (r *attendanceOrganizationRepository) ListCatalogs(context.Context, bool) (organization.Catalogs, error) {
	return r.catalogs, nil
}
func (r *attendanceOrganizationRepository) ListScopes(context.Context) ([]organization.DepartmentInspectionScope, error) {
	return r.scopes, nil
}
func (r *attendanceOrganizationRepository) UpsertScope(context.Context, organization.DepartmentInspectionScope, audit.Mutation) error {
	return nil
}
func (r *attendanceOrganizationRepository) CreateDepartment(_ context.Context, value organization.Department, _ audit.Mutation) (organization.Department, error) {
	return value, nil
}
func (r *attendanceOrganizationRepository) UpdateDepartment(_ context.Context, value organization.Department, _ audit.Mutation) (organization.Department, error) {
	return value, nil
}
func (r *attendanceOrganizationRepository) UpdateInspectionType(_ context.Context, value organization.InspectionType, _ audit.Mutation) (organization.InspectionType, error) {
	return value, nil
}
func (r *attendanceOrganizationRepository) ListUsers(context.Context) ([]organization.UserAssignment, error) {
	return nil, nil
}
func (r *attendanceOrganizationRepository) AssignUserRole(context.Context, organization.UserAssignment, audit.Mutation) error {
	return nil
}

type attendanceAcademicRepository struct{ calendar academic.Calendar }

func (r *attendanceAcademicRepository) List(context.Context) (academic.Calendar, error) {
	return r.calendar, nil
}
func (r *attendanceAcademicRepository) CreateYear(_ context.Context, value academic.AcademicYear, _ audit.Mutation) (academic.AcademicYear, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) UpdateYear(_ context.Context, value academic.AcademicYear, _ audit.Mutation) (academic.AcademicYear, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) CreateSemester(_ context.Context, value academic.Semester, _ audit.Mutation) (academic.Semester, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) UpdateSemester(_ context.Context, value academic.Semester, _ audit.Mutation) (academic.Semester, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) CreateWeek(_ context.Context, value academic.Week, _ audit.Mutation) (academic.Week, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) UpdateWeek(_ context.Context, value academic.Week, _ audit.Mutation) (academic.Week, error) {
	return value, nil
}
func (r *attendanceAcademicRepository) CurrentWeek(context.Context, time.Time) (*academic.Week, error) {
	if len(r.calendar.Weeks) == 0 {
		return nil, nil
	}
	return &r.calendar.Weeks[0], nil
}

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
	rowStatus    string
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
func (s *importHTTPStub) ListRows(_ context.Context, _ string, page, pageSize int, validationStatus string) (importjob.ImportRowPage, error) {
	s.rows.Page, s.rows.PageSize = page, pageSize
	s.rowStatus = validationStatus
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
	return attendanceRouterWithRole(t, allowed, false)
}

func attendanceRouterWithRole(t *testing.T, allowed, superAdmin bool) (http.Handler, string, *importHTTPStub, *attendanceHTTPStub) {
	t.Helper()
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, _ = authService.Register(context.Background(), auth.RegisterInput{StudentNo: "24010088", Name: "接口测试", Password: "Password123", RequestedDepartmentID: "department-study", RequestedOrgRoleID: "role-member"})
	account := accountsRepository.values["24010088"]
	account.Status = auth.StatusActive
	account.PrimaryDepartmentID = "department-study"
	account.IsSuperAdmin = superAdmin
	if superAdmin {
		account.SystemRole = permission.SystemRoleSuperAdmin
	}
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
	organizationService := organization.NewService(&attendanceOrganizationRepository{catalogs: organization.Catalogs{
		Departments: []organization.Department{
			{ID: "department-study", Code: "STUDY", Name: "学习部", Enabled: true},
			{ID: "department-office", Code: "OFFICE", Name: "办公室", Enabled: true},
			{ID: "other-department", Code: "OTHER", Name: "其他部门", Enabled: true},
		},
		InspectionTypes: []organization.InspectionType{
			{ID: "type-attendance", Code: "ATTENDANCE_RATE", Name: "到课率", Enabled: true},
			{ID: "type-other", Code: "OTHER", Name: "其他检查", Enabled: true},
		},
	}, scopes: []organization.DepartmentInspectionScope{
		{DepartmentID: "department-study", InspectionTypeID: "type-attendance", CanEnter: true, CanView: true, Enabled: true},
		{DepartmentID: "department-office", InspectionTypeID: "type-attendance", CanView: true, Enabled: true},
	}})
	academicService := academic.NewService(&attendanceAcademicRepository{calendar: academic.Calendar{
		Semesters: []academic.Semester{{ID: "semester-1", AcademicYearID: "year-1", Code: "FIRST", Name: "2026-2027 学年上学期", Enabled: true}},
		Weeks:     []academic.Week{{ID: "week-1", SemesterID: "semester-1", WeekNo: 1, Enabled: true}},
	}})
	masterDataService := masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{
		masterdata.KindClass + ":allowed-class": {ID: "allowed-class", Kind: masterdata.KindClass, Code: "CS2401", Name: "计科 2401", Grade: "2024", Enabled: true},
	}})
	router := NewRouter(authService, false, AdminServices{
		Organization: organizationService, Academic: academicService, Permission: permission.NewService(permissionRepository), MasterData: masterDataService,
		ImportJobs: imports, Attendance: attendance, ImportFiles: files, ImportMaxUploadBytes: 4096,
	})
	return router, tokens.AccessToken, imports, attendance
}

func TestSuperAdminAttendanceListAllowsEmptyDepartmentFilter(t *testing.T) {
	router, token, _, attendance := attendanceRouterWithRole(t, true, true)
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&page=1&pageSize=20"

	response := performJSON(router, http.MethodGet, path, nil, token)

	if response.Code != http.StatusOK || attendance.filter.DepartmentID != "" || attendance.scope.DepartmentID != "" {
		t.Fatalf("response=%d %s filter=%#v scope=%#v", response.Code, response.Body.String(), attendance.filter, attendance.scope)
	}
}

func TestAttendanceListCompilesAuthorizedDepartmentsWhenFilterIsEmpty(t *testing.T) {
	router, token, _, attendance := attendanceRouter(t, true)
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&classId=allowed-class&page=1&pageSize=20"

	response := performJSON(router, http.MethodGet, path, nil, token)

	if response.Code != http.StatusOK || len(attendance.scope.DepartmentIDs) != 1 || attendance.scope.DepartmentIDs[0] != "department-study" {
		t.Fatalf("response=%d %s scope=%#v", response.Code, response.Body.String(), attendance.scope)
	}
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
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study&classId=allowed-class&status=DRAFT&source=EXCEL&page=2&pageSize=25&sort=studentNoAsc"
	response := performJSON(router, http.MethodGet, path, nil, token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || attendance.filter.Page != 2 || attendance.filter.PageSize != 25 || attendance.filter.Status != inspection.RecordStatusDraft || attendance.filter.Source != inspection.RecordSourceExcel || attendance.scope.DepartmentID != "department-study" {
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

func TestAttendanceFilterOptionsAreAuthenticatedAndBusinessScoped(t *testing.T) {
	router, token, _, _ := attendanceRouter(t, true)
	path := "/api/v1/inspections/attendance/filter-options"
	if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated options = %d %s", response.Code, response.Body.String())
	}
	response := performJSON(router, http.MethodGet, path, nil, token)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"code":"ATTENDANCE_RATE"`) || !strings.Contains(body, `"responsibleDepartments":[{"id":"department-study"`) || !strings.Contains(body, `"name":"计科 2401"`) || !strings.Contains(body, `"weekNo":1`) {
		t.Fatalf("options=%d %s", response.Code, body)
	}
	if strings.Contains(body, "other-department") || strings.Contains(body, "department-office") || strings.Contains(body, "type-other") {
		t.Fatalf("options leaked another business scope: %s", body)
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
	preview := performJSON(router, http.MethodGet, "/api/v1/imports/batch-1/rows?page=2&pageSize=10&validationStatus=invalid", nil, token)
	if preview.Code != http.StatusOK || imports.rows.Page != 2 || imports.rows.PageSize != 10 || imports.rowStatus != importjob.RowStatusInvalid {
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

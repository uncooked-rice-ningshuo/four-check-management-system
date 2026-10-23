package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	"github.com/gin-gonic/gin"
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

type attendanceAcademicRepository struct {
	calendar academic.Calendar
	listErr  error
}

func (r *attendanceAcademicRepository) List(context.Context) (academic.Calendar, error) {
	return r.calendar, r.listErr
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
	cancelMetadata                                audit.Metadata
	getErr                                        error
	getCalls, rowCalls, confirmCalls, cancelCalls int
	batch                                         importjob.Batch
	rows                                          importjob.ImportRowPage
	template                                      importjob.Template
	parseErr                                      error
	confirmErr                                    error
	confirmKey                                    string
	confirmInput                                  importjob.ConfirmBatchInput
	createErr                                     error
	discarded                                     bool
	rowStatus                                     string
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
	s.getCalls++
	if s.getErr != nil {
		return importjob.Batch{}, s.getErr
	}
	if id != s.batch.ID {
		return importjob.Batch{}, importjob.ErrBatchNotFound
	}
	return s.batch, nil
}
func (s *importHTTPStub) ListRows(_ context.Context, _ string, page, pageSize int, validationStatus string) (importjob.ImportRowPage, error) {
	s.rowCalls++
	s.rows.Page, s.rows.PageSize = page, pageSize
	s.rowStatus = validationStatus
	return s.rows, nil
}
func (s *importHTTPStub) PublishedTemplate(context.Context, string) (importjob.Template, error) {
	return s.template, nil
}
func (s *importHTTPStub) ConfirmBatch(_ context.Context, input importjob.ConfirmBatchInput) (importjob.ConfirmResult, error) {
	s.confirmCalls++
	s.confirmInput = input
	if s.confirmErr != nil {
		return importjob.ConfirmResult{}, s.confirmErr
	}
	return importjob.ConfirmResult{BatchID: input.BatchID, Status: importjob.BatchStatusConfirmed, ImportedRows: 1, Version: input.ExpectedVersion + 1}, nil
}
func (s *importHTTPStub) CancelBatch(_ context.Context, id string, version int, metadata audit.Metadata) (importjob.Batch, error) {
	s.cancelCalls++
	s.cancelMetadata = metadata
	if version != s.batch.Version {
		return importjob.Batch{}, importjob.ErrBatchVersionConflict
	}
	s.batch.Status, s.batch.Version = importjob.BatchStatusCancelled, version+1
	return s.batch, nil
}

type fileHTTPStub struct {
	stored         storage.File
	content        []byte
	storeErr       error
	metadata       audit.Metadata
	onDiscard      func()
	discardContext context.Context
	discardErr     error
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
func (s *fileHTTPStub) Discard(ctx context.Context, _ string, _ audit.Metadata) error {
	s.discardContext = ctx
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.onDiscard != nil {
		s.onDiscard()
	}
	return s.discardErr
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
	return attendanceRouterWithPasswordState(t, allowed, superAdmin, false)
}

func attendanceRouterWithPasswordState(t *testing.T, allowed, superAdmin, mustChangePassword bool, customRules ...[]permission.Rule) (http.Handler, string, *importHTTPStub, *attendanceHTTPStub) {
	t.Helper()
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, _ = authService.Register(context.Background(), auth.RegisterInput{StudentNo: "24010088", Name: "接口测试", Password: "Password123", RequestedDepartmentID: "10000000-0000-0000-0000-000000000002", RequestedOrgRoleID: "role-member"})
	account := accountsRepository.values["24010088"]
	account.Status = auth.StatusActive
	account.PrimaryDepartmentID = "10000000-0000-0000-0000-000000000002"
	account.IsSuperAdmin = superAdmin
	account.MustChangePassword = mustChangePassword
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
		{Action: permission.ActionInspectionView, Effect: effect, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "10000000-0000-0000-0000-000000000002"}},
		{Action: permission.ActionInspectionEnter, Effect: effect, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "10000000-0000-0000-0000-000000000002"}},
		{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "10000000-0000-0000-0000-000000000002", ClassID: "blocked-class"}},
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Enabled: true, Scope: permission.Scope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "10000000-0000-0000-0000-000000000002", ClassID: "blocked-class"}},
	}}}
	if len(customRules) > 0 {
		permissionRepository.profile.Rules = customRules[0]
	}
	imports := &importHTTPStub{
		batch:    importjob.Batch{ID: "batch-1", InspectionTypeID: "type-attendance", SemesterID: "semester-1", WeekID: "week-1", DepartmentID: "10000000-0000-0000-0000-000000000002", ClassID: "allowed-class", Status: importjob.BatchStatusPreviewReady, Version: 3, CreatedBy: account.ID},
		rows:     importjob.ImportRowPage{Items: []importjob.ImportRow{}, Total: 0},
		template: importjob.Template{ID: "template-1", Code: "ATTENDANCE_IMPORT", InspectionTypeID: "type-attendance", Version: 1, FileID: "template-file", Status: "PUBLISHED"},
	}
	attendance := &attendanceHTTPStub{page: inspection.AttendancePage{Items: []inspection.AttendanceRecord{}, Page: 1, PageSize: 20}}
	files := &fileHTTPStub{content: []byte("xlsx-template"), onDiscard: func() { imports.discarded = true }}
	organizationService := organization.NewService(&attendanceOrganizationRepository{catalogs: organization.Catalogs{
		Departments: []organization.Department{
			{ID: "10000000-0000-0000-0000-000000000002", Code: "STUDY", Name: "学习部", Enabled: true},
			{ID: "department-office", Code: "OFFICE", Name: "办公室", Enabled: true},
			{ID: "other-department", Code: "OTHER", Name: "其他部门", Enabled: true},
		},
		InspectionTypes: []organization.InspectionType{
			{ID: "type-attendance", Code: "ATTENDANCE_RATE", Name: "到课率", Enabled: true},
			{ID: "type-other", Code: "OTHER", Name: "其他检查", Enabled: true},
		},
	}, scopes: []organization.DepartmentInspectionScope{
		{DepartmentID: "10000000-0000-0000-0000-000000000002", InspectionTypeID: "type-attendance", CanEnter: true, CanView: true, Enabled: true},
		{DepartmentID: "department-office", InspectionTypeID: "type-attendance", CanView: true, Enabled: true},
	}})
	academicService := academic.NewService(&attendanceAcademicRepository{calendar: academic.Calendar{
		Semesters: []academic.Semester{{ID: "semester-1", AcademicYearID: "year-1", Code: "FIRST", Name: "2026-2027 学年上学期", Enabled: true}},
		Weeks:     []academic.Week{{ID: "week-1", SemesterID: "semester-1", WeekNo: 1, Enabled: true}},
	}})
	masterDataService := masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{
		masterdata.KindClass + ":allowed-class": {ID: "allowed-class", Kind: masterdata.KindClass, Code: "CS2401", Name: "计科 2401", Grade: "2024", Enabled: true},
	}})
	effective, effectiveErr := permission.NewService(permissionRepository).Effective(context.Background(), account.ID)
	if effectiveErr != nil {
		t.Fatal(effectiveErr)
	}
	visible := superAdmin || effective.Allowed(permission.ActionInspectionView, permission.Scope{AcademicYearID: "year-1", SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-attendance", DepartmentID: "10000000-0000-0000-0000-000000000002", ClassID: "allowed-class", Grade: "2024"})
	router := NewRouter(authService, false, AdminServices{
		InspectionContextOptions: &attendanceContextOptionsFixture{visible: visible},
		Organization:             organizationService, Academic: academicService, Permission: permission.NewService(permissionRepository), MasterData: masterDataService,
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

	if response.Code != http.StatusOK || len(attendance.scope.DepartmentIDs) != 1 || attendance.scope.DepartmentIDs[0] != "10000000-0000-0000-0000-000000000002" {
		t.Fatalf("response=%d %s scope=%#v", response.Code, response.Body.String(), attendance.scope)
	}
}

func TestAttendanceImportRoutesRequireAuthenticationAndEffectiveScope(t *testing.T) {
	router, token, _, _ := attendanceRouter(t, true)
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class&page=1&pageSize=20"
	if response := performJSON(router, http.MethodGet, path, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d %s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodGet, strings.Replace(path, "10000000-0000-0000-0000-000000000002", "other-department", 1), nil, token); response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "FORBIDDEN_SCOPE") {
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
	path := "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class&status=DRAFT&source=EXCEL&page=2&pageSize=25&sort=studentNoAsc"
	response := performJSON(router, http.MethodGet, path, nil, token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || attendance.filter.Page != 2 || attendance.filter.PageSize != 25 || attendance.filter.Status != inspection.RecordStatusDraft || attendance.filter.Source != inspection.RecordSourceExcel || attendance.scope.DepartmentID != "10000000-0000-0000-0000-000000000002" {
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
	if response.Code != http.StatusOK || !strings.Contains(body, `"code":"ATTENDANCE_RATE"`) || !strings.Contains(body, `"responsibleDepartments":[{"id":"10000000-0000-0000-0000-000000000002"`) || !strings.Contains(body, `"name":"计科 2401"`) || !strings.Contains(body, `"weekNo":1`) {
		t.Fatalf("options=%d %s", response.Code, body)
	}
	if strings.Contains(body, "other-department") || strings.Contains(body, "department-office") || strings.Contains(body, "type-other") {
		t.Fatalf("options leaked another business scope: %s", body)
	}
}

func TestAttendanceTemplateDownloadAndUploadAreProtectedAndSafe(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	contextQuery := "semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	download := performJSON(router, http.MethodGet, "/api/v1/imports/attendance/template?"+contextQuery, nil, token)
	if download.Code != http.StatusOK || download.Header().Get("Content-Disposition") == "" || download.Header().Get("X-Import-Template-ID") != "template-1" || download.Header().Get("X-Import-Template-Version") != "1" || !bytes.Equal(download.Body.Bytes(), []byte("xlsx-template")) {
		t.Fatalf("download=%d headers=%v body=%q", download.Code, download.Header(), download.Body.Bytes())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "10000000-0000-0000-0000-000000000002", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "1"} {
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

func TestMemberUploadUsesTrustedDepartment(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	upload := func(departmentID string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": departmentID, "classId": "allowed-class", "templateId": "template-1", "templateVersion": "1"} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		part, err := writer.CreateFormFile("file", "attendance.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("small-xlsx")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if result := upload("10000000-0000-0000-0000-000000000003"); result.Code != 403 || !strings.Contains(result.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("forged import department: %d %s", result.Code, result.Body.String())
	}
	if result := upload(""); result.Code != 201 || imports.batch.DepartmentID != "10000000-0000-0000-0000-000000000002" {
		t.Fatalf("member import department: %d %s %+v", result.Code, result.Body.String(), imports.batch)
	}
}

func TestAttendanceUploadRejectsStaleTemplateBeforePersistingSourceFile(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "10000000-0000-0000-0000-000000000002", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "2"} {
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
	for key, value := range map[string]string{"semesterId": "semester-1", "weekId": "week-1", "inspectionTypeId": "type-attendance", "departmentId": "10000000-0000-0000-0000-000000000002", "classId": "allowed-class", "templateId": "template-1", "templateVersion": "1"} {
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
	imports.batch.DepartmentID = "10000000-0000-0000-0000-000000000002"
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
	if imports.cancelMetadata.ActorID == "" {
		t.Fatal("cancel audit actor is empty")
	}
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

func TestCourseImportTemplateRoutesKeepTypeAndScopeBoundaries(t *testing.T) {
	query := "?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	for _, code := range []string{"ATTENDANCE_IMPORT", "UNATTENDED_CLASSROOM_IMPORT", "UNKNOWN"} {
		router, token, imports, _ := attendanceRouter(t, true)
		imports.template.Code = code
		for _, endpoint := range []string{"attendance", "course-records"} {
			response := performJSON(router, http.MethodGet, "/api/v1/imports/"+endpoint+"/template"+query, nil, token)
			accepted := code == "ATTENDANCE_IMPORT" || (code == "UNATTENDED_CLASSROOM_IMPORT" && endpoint == "course-records")
			if accepted {
				if response.Code != http.StatusOK || response.Header().Get("X-Import-Template-ID") != "template-1" {
					t.Fatalf("%s/%s: %d %s", endpoint, code, response.Code, response.Body.String())
				}
			} else if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") {
				t.Fatalf("%s/%s: %d %s", endpoint, code, response.Code, response.Body.String())
			}
		}
	}
	router, token, _, _ := attendanceRouter(t, true)
	endpoint := "/api/v1/imports/course-records/template"
	if r := performJSON(router, http.MethodGet, endpoint+query, nil, ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", r.Code)
	}
	if r := performJSON(router, http.MethodGet, endpoint+strings.Replace(query, "classId=allowed-class", "classId=blocked-class", 1), nil, token); r.Code != http.StatusForbidden || !strings.Contains(r.Body.String(), "FORBIDDEN_SCOPE") {
		t.Fatalf("scope: %d %s", r.Code, r.Body.String())
	}
}

func TestImportConfirmationReturnsSafeStalePreviewConflict(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.confirmErr = importjob.ErrPreviewStale
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/batch-1/confirm", strings.NewReader(`{"version":3}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", "stale-preview")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "IMPORT_PREVIEW_STALE") || !strings.Contains(response.Body.String(), "重新上传") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}

func TestImportConfirmationDuplicateReturnsConflict(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.confirmErr = inspection.ErrAttendanceDuplicate
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/batch-1/confirm", strings.NewReader(`{"version":3}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", "duplicate-preview")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "RECORD_ALREADY_EXISTS") || strings.Contains(response.Body.String(), "SQL") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}

func TestMorningTemplateOnlyUsesGenericInspectionEntry(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.template.Code = "MORNING_ATTENDANCE_IMPORT"
	query := "?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	for _, entry := range []string{"attendance", "course-records", "inspections"} {
		response := performJSON(router, http.MethodGet, "/api/v1/imports/"+entry+"/template"+query, nil, token)
		if entry == "inspections" {
			if response.Code != http.StatusOK || response.Header().Get("X-Import-Template-Version") != "1" {
				t.Fatalf("generic entry: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") {
			t.Fatalf("%s: %d %s", entry, response.Code, response.Body.String())
		}
	}
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/inspections/template"+query, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatal("unprotected template", response.Code)
	}
}

func TestLateReturnTemplateOnlyUsesGenericInspectionEntry(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.template.Code = "LATE_RETURN_IMPORT"
	query := "?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	for _, entry := range []string{"attendance", "course-records", "inspections"} {
		response := performJSON(router, http.MethodGet, "/api/v1/imports/"+entry+"/template"+query, nil, token)
		if entry == "inspections" {
			if response.Code != http.StatusOK || response.Header().Get("X-Import-Template-Version") != "1" {
				t.Fatalf("generic entry: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") {
			t.Fatalf("%s: %d %s", entry, response.Code, response.Body.String())
		}
	}
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/inspections/template"+query, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatal("unprotected template", response.Code)
	}
}

func TestImportBatchRequiresPasswordChangeBeforeSuperAdminBypass(t *testing.T) {
	for _, superAdmin := range []bool{false, true} {
		for _, route := range []struct{ name, method, suffix string }{
			{"batch", http.MethodGet, ""}, {"rows", http.MethodGet, "/rows"},
			{"confirm", http.MethodPost, "/confirm"}, {"cancel", http.MethodPost, "/cancel"},
		} {
			role := "member"
			if superAdmin {
				role = "super-admin"
			}
			t.Run(role+"/"+route.name, func(t *testing.T) {
				router, token, imports, _ := attendanceRouterWithPasswordState(t, true, superAdmin, true)
				request := httptest.NewRequest(route.method, "/api/v1/imports/batch-1"+route.suffix, strings.NewReader(`{"version":3}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Idempotency-Key", "password-state-test")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"IMPORT_BATCH_NOT_FOUND"`) {
					t.Errorf("must-change-password batch access: status=%d body=%s", response.Code, response.Body.String())
				}
				if imports.getCalls != 0 || imports.rowCalls != 0 || imports.confirmCalls != 0 || imports.cancelCalls != 0 {
					t.Errorf("business service called: get=%d rows=%d confirm=%d cancel=%d", imports.getCalls, imports.rowCalls, imports.confirmCalls, imports.cancelCalls)
				}
			})
		}
	}
}

func TestImportBatchReadFailureIsNotReportedAsMissing(t *testing.T) {
	for _, failure := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing", importjob.ErrBatchNotFound, http.StatusNotFound, "IMPORT_BATCH_NOT_FOUND"},
		{"internal", errors.New("private database diagnostic"), http.StatusInternalServerError, "IMPORT_SERVICE_UNAVAILABLE"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			router, token, imports, _ := attendanceRouter(t, true)
			imports.getErr = failure.err
			response := performJSON(router, http.MethodGet, "/api/v1/imports/batch-1", nil, token)
			if response.Code != failure.status || !strings.Contains(response.Body.String(), `"code":"`+failure.code+`"`) || strings.Contains(response.Body.String(), "private database") {
				t.Fatalf("batch read failure: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestImportAcademicYearScopeUsesAuthoritativeSemester(t *testing.T) {
	for _, scenario := range []struct {
		name, allowYear, denyYear string
		allowed                   bool
	}{
		{"current year allow", "year-1", "", true},
		{"current year deny", "", "year-1", false},
		{"other year deny", "", "year-other", true},
		{"other year allow", "year-other", "", false},
	} {
		for _, route := range []struct {
			name, path string
			hidden     bool
		}{
			{"template", "/api/v1/imports/attendance/template?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&academicYearId=year-other", false},
			{"batch", "/api/v1/imports/batch-1?academicYearId=year-other", true},
			{"confirm", "/api/v1/imports/batch-1/confirm?academicYearId=year-other", true},
			{"cancel", "/api/v1/imports/batch-1/cancel?academicYearId=year-other", true},
			{"rows", "/api/v1/imports/batch-1/rows?academicYearId=year-other", true},
			{"attendance", "/api/v1/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&academicYearId=year-other", false},
		} {
			t.Run(scenario.name+"/"+route.name, func(t *testing.T) {
				rules := []permission.Rule{{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Enabled: true, Scope: permission.Scope{AcademicYearID: scenario.allowYear}}}
				if scenario.denyYear != "" {
					rules = append(rules, permission.Rule{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Enabled: true, Scope: permission.Scope{AcademicYearID: scenario.denyYear}})
				}
				for _, rule := range append([]permission.Rule(nil), rules...) {
					rule.Action = permission.ActionInspectionEnter
					rules = append(rules, rule)
				}
				router, token, _, _ := attendanceRouterWithPasswordState(t, true, false, false, rules)
				method := http.MethodGet
				if route.name == "confirm" || route.name == "cancel" {
					method = http.MethodPost
				}
				request := httptest.NewRequest(method, route.path, strings.NewReader(`{"version":3}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Idempotency-Key", "year-scope-test")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				expected := http.StatusOK
				if !scenario.allowed {
					expected = http.StatusForbidden
					if route.hidden {
						expected = http.StatusNotFound
					}
				}
				if response.Code != expected {
					t.Fatalf("year scope: expected=%d actual=%d body=%s", expected, response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestInspectionAcademicYearLookupFailsClosed(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		repository *attendanceAcademicRepository
		hidden     bool
		status     int
		code       string
	}{
		{"unavailable", nil, false, 503, "SERVICE_UNAVAILABLE"},
		{"internal", &attendanceAcademicRepository{listErr: errors.New("private database diagnostic")}, false, 500, "INTERNAL_ERROR"},
		{"missing semester", &attendanceAcademicRepository{}, false, 403, "FORBIDDEN_SCOPE"},
		{"missing batch semester", &attendanceAcademicRepository{}, true, 404, "IMPORT_BATCH_NOT_FOUND"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			h := &handler{}
			if scenario.repository != nil {
				h.academic = academic.NewService(scenario.repository)
			}
			_, ok := h.inspectionScopeWithAcademicYear(c, permission.Scope{SemesterID: "unknown", AcademicYearID: "spoofed"}, scenario.hidden)
			if ok || recorder.Code != scenario.status || !strings.Contains(recorder.Body.String(), `"code":"`+scenario.code+`"`) || strings.Contains(recorder.Body.String(), "private database") {
				t.Fatalf("lookup not closed: ok=%v status=%d body=%s", ok, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDormitoryHygieneTemplateOnlyUsesGenericInspectionEntry(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.template.Code = "DORMITORY_HYGIENE_IMPORT"
	query := "?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	for _, entry := range []string{"attendance", "course-records", "inspections"} {
		response := performJSON(router, http.MethodGet, "/api/v1/imports/"+entry+"/template"+query, nil, token)
		if entry == "inspections" {
			if response.Code != http.StatusOK || response.Header().Get("X-Import-Template-Version") != "1" {
				t.Fatalf("generic entry: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") {
			t.Fatalf("%s: %d %s", entry, response.Code, response.Body.String())
		}
	}
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/inspections/template"+query, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatal("unprotected template", response.Code)
	}
}

func TestClassroomHygieneTemplateOnlyUsesGenericInspectionEntry(t *testing.T) {
	router, token, imports, _ := attendanceRouter(t, true)
	imports.template.Code = "CLASSROOM_HYGIENE_IMPORT"
	query := "?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=10000000-0000-0000-0000-000000000002&classId=allowed-class"
	for _, entry := range []string{"attendance", "course-records", "inspections"} {
		response := performJSON(router, http.MethodGet, "/api/v1/imports/"+entry+"/template"+query, nil, token)
		if entry == "inspections" {
			if response.Code != http.StatusOK || response.Header().Get("X-Import-Template-Version") != "1" {
				t.Fatalf("generic entry: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "TEMPLATE_VERSION_MISMATCH") {
			t.Fatalf("%s: %d %s", entry, response.Code, response.Body.String())
		}
	}
	if response := performJSON(router, http.MethodGet, "/api/v1/imports/inspections/template"+query, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatal("unprotected template", response.Code)
	}
}

func TestAttendanceFilterOptionsRespectAcademicYearAndDeny(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		rules []permission.Rule
	}{
		{"other academic year", []permission.Rule{{ID: "only-other", Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{AcademicYearID: "other-year"}, Enabled: true}}},
		{"denied academic year", []permission.Rule{{ID: "allow", Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Enabled: true}, {ID: "deny-year", Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: permission.Scope{AcademicYearID: "year-1"}, Enabled: true}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router, token, _, _ := attendanceRouterWithPasswordState(t, true, false, false, scenario.rules)
			response := performJSON(router, http.MethodGet, "/api/v1/inspections/attendance/filter-options", nil, token)
			var result attendanceFilterOptionsResponse
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
				t.Fatalf("options status: %d", response.Code)
			}
			if len(result.Semesters) != 0 || len(result.Weeks) != 0 || len(result.Classes) != 0 || len(result.Departments) != 0 || len(result.ResponsibleDepartments) != 0 {
				t.Fatal("unauthorized candidate scope exposed")
			}
		})
	}
}

// The fixture represents already-authorized context candidates, not a second
// implementation of the repository scope compiler tested against PostgreSQL.
type attendanceContextOptionsFixture struct{ visible bool }

func (s *attendanceContextOptionsFixture) ContextOptions(_ context.Context, _ inspection.ManualActor, q inspection.ContextOptionQuery) (inspection.ContextOptionPage, error) {
	result := inspection.ContextOptionPage{InspectionTypeID: "type-attendance", EntryOptionPage: inspection.EntryOptionPage{Items: []inspection.EntryOption{}, Page: q.Page, PageSize: q.PageSize}}
	if !s.visible {
		return result, nil
	}
	value := inspection.EntryOption{}
	switch q.Kind {
	case "semesters":
		value = inspection.EntryOption{ID: "semester-1", Label: "测试学期"}
	case "weeks":
		value = inspection.EntryOption{ID: "week-1", Label: "第1周"}
	case "departments":
		value = inspection.EntryOption{ID: "10000000-0000-0000-0000-000000000002", Label: "学习部"}
	case "classes":
		value = inspection.EntryOption{ID: "allowed-class", Label: "计科 2401", Grade: "2024"}
	case "grades":
		value = inspection.EntryOption{ID: "2024", Label: "2024"}
	}
	result.Items = []inspection.EntryOption{value}
	result.Total = 1
	return result, nil
}

type attendanceContextOptionsFunc func(context.Context, inspection.ManualActor, inspection.ContextOptionQuery) (inspection.ContextOptionPage, error)

func (f attendanceContextOptionsFunc) ContextOptions(ctx context.Context, a inspection.ManualActor, q inspection.ContextOptionQuery) (inspection.ContextOptionPage, error) {
	return f(ctx, a, q)
}

func TestAttendanceAuthorizedOptionsPaginationFailsClosed(t *testing.T) {
	for _, scenario := range []string{"complete", "empty second page", "duplicate identifier", "wrong type", "missing grade"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			h := handler{inspectionContextOptions: attendanceContextOptionsFunc(func(_ context.Context, _ inspection.ManualActor, q inspection.ContextOptionQuery) (inspection.ContextOptionPage, error) {
				calls++
				result := inspection.ContextOptionPage{InspectionTypeID: "type", EntryOptionPage: inspection.EntryOptionPage{Page: q.Page, PageSize: 200, Total: 201}}
				start, end := 0, 200
				if q.Page == 2 {
					start, end = 200, 201
				}
				for i := start; i < end; i++ {
					result.Items = append(result.Items, inspection.EntryOption{ID: strconv.Itoa(i), Label: "fixture", Grade: "2026"})
				}
				if q.Page == 2 {
					switch scenario {
					case "empty second page":
						result.Items = nil
					case "duplicate identifier":
						result.Items[0].ID = "0"
					case "wrong type":
						result.InspectionTypeID = "other"
					case "missing grade":
						result.Items[0].Grade = ""
					}
				}
				return result, nil
			})}
			result, err := h.authorizedAttendanceOptions(context.Background(), inspection.ManualActor{}, "type", "classes", "")
			if scenario == "complete" {
				if err != nil || len(result) != 201 || calls != 2 {
					t.Fatal(len(result), calls, err)
				}
			} else if err == nil || result != nil {
				t.Fatal("invalid partial response accepted")
			}
		})
	}
}

func TestImportCompensationOutlivesCancellationAndLogsSafeFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/imports/attendance", nil).WithContext(ctx)
	files := &fileHTTPStub{discardErr: errors.New("private-object-key-and-credential")}
	called := false
	files.onDiscard = func() {
		called = true
		if err := files.discardContext.Err(); err != nil {
			t.Fatal(err)
		}
		deadline, ok := files.discardContext.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("unbounded compensation")
		}
	}
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	h := &handler{importFiles: files}
	h.discardUnboundImportSource(c, "isolated-file")
	if !called || !strings.Contains(logs.String(), "compensation failed") || !strings.Contains(logs.String(), "fileId=isolated-file") || strings.Contains(logs.String(), "private-object") {
		t.Fatalf("unsafe or missing compensation diagnostic: %s", logs.String())
	}
	if !errors.Is(files.discardContext.Err(), context.Canceled) {
		t.Fatal("compensation context not released")
	}
}

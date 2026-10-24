package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/academicimport"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dashboard"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/masterdataimport"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/workflow"
	"github.com/gin-gonic/gin"
)

type AdminServices struct {
	Organization             *organization.Service
	Academic                 *academic.Service
	AcademicWeekImports      academicWeekImportHTTPService
	WorkContext              *academic.WorkContextService
	Permission               *permission.Service
	MasterData               *masterdata.Service
	MasterDataImports        masterDataImportHTTPService
	Dictionary               *dictionary.Service
	Storage                  *storage.Service
	PostgresReady            readinessChecker
	ObjectStorageReady       readinessChecker
	ImportJobs               importJobHTTPService
	ImportRollback           importRollbackHTTPService
	Attendance               attendanceHTTPService
	ManualAttendance         manualAttendanceHTTPService
	Inspections              inspectionHTTPService
	InspectionOptions        inspectionOptionHTTPService
	InspectionContextOptions inspectionContextOptionHTTPService
	AttendanceMutations      attendanceMutationHTTPService
	Dashboard                dashboardHTTPService
	Workflow                 workflowHTTPService
	ImportFiles              importFileHTTPService
	ImportMaxUploadBytes     int64
}

type importJobHTTPService interface {
	CreateBatch(context.Context, importjob.CreateBatchInput) (importjob.Batch, error)
	ParseBatch(context.Context, importjob.ParseBatchInput) (importjob.Batch, error)
	GetBatch(context.Context, string) (importjob.Batch, error)
	ListRows(context.Context, string, int, int, string) (importjob.ImportRowPage, error)
	PublishedTemplate(context.Context, string) (importjob.Template, error)
	ConfirmBatch(context.Context, importjob.ConfirmBatchInput) (importjob.ConfirmResult, error)
	CancelBatch(context.Context, string, int, audit.Metadata) (importjob.Batch, error)
}

type attendanceHTTPService interface {
	ListAttendance(context.Context, inspection.AttendanceListFilter, inspection.AttendanceScope) (inspection.AttendancePage, error)
}

type dashboardHTTPService interface {
	Workbench(context.Context, dashboard.Actor, time.Time) (dashboard.Workbench, error)
}

type workflowHTTPService interface {
	Submit(context.Context, workflow.Actor, workflow.SubmitInput, audit.Metadata) (workflow.Submission, error)
	Decide(context.Context, workflow.Actor, string, workflow.Action, workflow.DecisionInput, audit.Metadata) (workflow.Submission, error)
	Resubmit(context.Context, workflow.Actor, string, workflow.ResubmitInput, audit.Metadata) (workflow.Submission, error)
	List(context.Context, workflow.Actor, workflow.ListQuery) (workflow.Page, error)
	Detail(context.Context, workflow.Actor, string) (workflow.Submission, error)
}

type importFileHTTPService interface {
	Store(context.Context, storage.StoreInput, audit.Metadata) (storage.File, error)
	ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error)
	Discard(context.Context, string, audit.Metadata) error
}

type masterDataImportHTTPService interface {
	Preview(context.Context, string, string, string, string, []byte, audit.Metadata) (masterdataimport.Batch, error)
	GetBatch(context.Context, string, string, string) (masterdataimport.Batch, error)
	Confirm(context.Context, masterdataimport.ConfirmInput) (masterdataimport.ConfirmResult, error)
}

type academicWeekImportHTTPService interface {
	Preview(context.Context, string, string, string, string, []byte, audit.Metadata) (academicimport.Batch, error)
	GetBatch(context.Context, string, string) (academicimport.Batch, error)
	Confirm(context.Context, academicimport.ConfirmInput) (academicimport.ConfirmResult, error)
}

type readinessChecker interface {
	Ready(context.Context) error
}

type handler struct {
	auth                     *auth.Service
	organization             *organization.Service
	academic                 *academic.Service
	academicWeekImports      academicWeekImportHTTPService
	permission               *permission.Service
	masterData               *masterdata.Service
	masterDataImports        masterDataImportHTTPService
	dictionary               *dictionary.Service
	storage                  *storage.Service
	postgresReady            readinessChecker
	objectStorageReady       readinessChecker
	importJobs               importJobHTTPService
	importRollback           importRollbackHTTPService
	attendance               attendanceHTTPService
	manualAttendance         manualAttendanceHTTPService
	inspections              inspectionHTTPService
	inspectionOptions        inspectionOptionHTTPService
	inspectionContextOptions inspectionContextOptionHTTPService
	workContext              *academic.WorkContextService
	attendanceMutations      attendanceMutationHTTPService
	dashboard                dashboardHTTPService
	workflow                 workflowHTTPService
	importFiles              importFileHTTPService
	importMaxUploadBytes     int64
	secureCookie             bool
}

func NewRouter(authService *auth.Service, secureCookie bool, dependencies ...AdminServices) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(requestMetadataMiddleware(), gin.Recovery())
	h := &handler{auth: authService, secureCookie: secureCookie}
	if len(dependencies) > 0 {
		h.organization, h.academic, h.permission = dependencies[0].Organization, dependencies[0].Academic, dependencies[0].Permission
		h.academicWeekImports = dependencies[0].AcademicWeekImports
		h.masterData, h.dictionary = dependencies[0].MasterData, dependencies[0].Dictionary
		h.masterDataImports = dependencies[0].MasterDataImports
		h.storage = dependencies[0].Storage
		h.postgresReady, h.objectStorageReady = dependencies[0].PostgresReady, dependencies[0].ObjectStorageReady
		h.importJobs, h.attendance, h.importFiles = dependencies[0].ImportJobs, dependencies[0].Attendance, dependencies[0].ImportFiles
		h.importRollback = dependencies[0].ImportRollback
		h.dashboard = dependencies[0].Dashboard
		h.workflow = dependencies[0].Workflow
		h.manualAttendance = dependencies[0].ManualAttendance
		h.inspections = dependencies[0].Inspections
		h.inspectionOptions = dependencies[0].InspectionOptions
		h.inspectionContextOptions = dependencies[0].InspectionContextOptions
		h.workContext = dependencies[0].WorkContext
		h.attendanceMutations = dependencies[0].AttendanceMutations
		h.importMaxUploadBytes = dependencies[0].ImportMaxUploadBytes
	}
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", h.readiness)

	v1 := r.Group("/api/v1")
	v1.GET("/public/registration-options", h.registrationOptions)
	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", h.register)
	authRoutes.POST("/login", h.login)
	authRoutes.POST("/refresh", h.refresh)
	authRoutes.POST("/logout", h.logout)
	authRoutes.GET("/me", h.me)
	authRoutes.POST("/change-password", h.changePassword)

	admin := v1.Group("/admin", h.requireSuperAdmin())
	admin.GET("/registration-requests", h.listRegistrationRequests)
	admin.POST("/registration-requests/:id/review", h.reviewRegistration)
	admin.GET("/catalogs", h.listCatalogs)
	admin.POST("/departments", h.createDepartment)
	admin.PUT("/departments/:id", h.updateDepartment)
	admin.PUT("/inspection-types/:id", h.updateInspectionType)
	admin.GET("/department-inspection-scopes", h.listScopes)
	admin.PUT("/department-inspection-scopes", h.upsertScope)
	admin.GET("/users", h.listUsers)
	admin.POST("/users", h.createUser)
	admin.PUT("/users/:id", h.updateUser)
	admin.PUT("/users/:id/status", h.updateUserStatus)
	admin.POST("/users/:id/reset-password", h.resetUserPassword)
	admin.PUT("/users/:id/assignment", h.assignUserRole)
	admin.GET("/users/:id/permissions", h.effectivePermissions)
	admin.GET("/users/:id/permission-rules", h.listPermissionRules)
	admin.POST("/users/:id/permission-rules", h.createPermissionRule)
	admin.PUT("/users/:id/permission-rules/:ruleId", h.updatePermissionRule)
	admin.DELETE("/users/:id/permission-rules/:ruleId", h.deletePermissionRule)
	admin.GET("/academic/calendar", h.listCalendar)
	admin.POST("/academic/years", h.createAcademicYear)
	admin.PUT("/academic/years/:id", h.updateAcademicYear)
	admin.POST("/academic/semesters", h.createSemester)
	admin.PUT("/academic/semesters/:id", h.updateSemester)
	admin.POST("/academic/weeks", h.createWeek)
	admin.PUT("/academic/weeks/:id", h.updateWeek)
	admin.GET("/academic/weeks/import-spec", h.getAcademicWeekImportSpec)
	admin.GET("/academic/weeks/import-template", h.downloadAcademicWeekImportTemplate)
	admin.POST("/academic/weeks/imports", h.previewAcademicWeekImport)
	admin.GET("/academic/weeks/imports/:batchID", h.getAcademicWeekImportBatch)
	admin.POST("/academic/weeks/imports/:batchID/confirm", h.confirmAcademicWeekImportBatch)
	masterDataAdmin := v1.Group("/admin", h.requireAction(permission.ActionMasterDataManage))
	masterDataAdmin.GET("/master-data/:kind", h.listMasterData)
	masterDataAdmin.GET("/master-data/:kind/import-spec", h.getMasterDataImportSpec)
	masterDataAdmin.GET("/master-data/:kind/import-template", h.downloadMasterDataImportTemplate)
	masterDataAdmin.POST("/master-data/:kind/imports", h.previewMasterDataImport)
	masterDataAdmin.GET("/master-data/:kind/imports/:batchID", h.getMasterDataImportBatch)
	masterDataAdmin.POST("/master-data/:kind/imports/:batchID/confirm", h.confirmMasterDataImportBatch)
	masterDataAdmin.POST("/master-data/:kind", h.createMasterData)
	masterDataAdmin.PUT("/master-data/:kind/:id", h.updateMasterData)
	masterDataAdmin.GET("/master-data/:kind/:id/changes", h.listMasterDataChanges)
	// Area maintenance accepts a department/type scoped master-data grant without
	// exposing the general master-data endpoints to that scoped actor.
	v1.GET("/admin/hygiene-areas/options", h.hygieneAreaOptions)
	v1.GET("/admin/hygiene-areas/:kind/references", h.requireHygieneAreaManage(), h.hygieneAreaReferences)
	v1.GET("/admin/hygiene-areas/:kind", h.requireHygieneAreaManage(), h.listMasterData)
	v1.POST("/admin/hygiene-areas/:kind", h.requireHygieneAreaManage(), h.createMasterData)
	v1.PUT("/admin/hygiene-areas/:kind/:id", h.requireHygieneAreaManage(), h.updateMasterData)
	v1.GET("/admin/hygiene-areas/:kind/:id/changes", h.requireHygieneAreaManage(), h.listMasterDataChanges)
	dictionaryAdmin := v1.Group("/admin", h.requireAction(permission.ActionDictionaryManage))
	dictionaryAdmin.GET("/dictionaries", h.listDictionaries)
	dictionaryAdmin.POST("/dictionaries/types", h.createDictionaryType)
	dictionaryAdmin.PUT("/dictionaries/types/:typeId", h.updateDictionaryType)
	dictionaryAdmin.POST("/dictionaries/types/:typeId/items", h.createDictionaryItem)
	dictionaryAdmin.PUT("/dictionaries/types/:typeId/items/:id", h.updateDictionaryItem)
	v1.GET("/academic/current-week", h.currentWeek)
	v1.GET("/work-context", h.getWorkContext)
	v1.PUT("/work-context", h.setWorkContext)
	v1.GET("/inspections/attendance/filter-options", h.attendanceFilterOptions)
	v1.GET("/inspections/attendance", h.listAttendance)
	v1.POST("/inspections/attendance", h.createManualAttendance)
	v1.POST("/inspections", h.createInspection)
	v1.GET("/inspections", h.listInspections)
	v1.GET("/inspections/entry-options", h.inspectionEntryOptions)
	v1.GET("/inspections/context-options", h.inspectionContextOptionList)
	v1.GET("/inspections/:id", h.getInspection)
	v1.PATCH("/inspections/:id", h.updateInspection)
	v1.DELETE("/inspections/:id", h.deleteInspection)
	v1.POST("/inspections/batch", h.batchInspections)
	v1.POST("/department-submissions", h.submitDepartmentBatch)
	v1.GET("/department-submissions", h.listDepartmentBatches)
	v1.GET("/department-submissions/:id", h.getDepartmentBatch)
	v1.POST("/department-submissions/:id/return", h.returnDepartmentBatch)
	v1.POST("/department-submissions/:id/confirm", h.confirmDepartmentBatch)
	v1.POST("/department-submissions/:id/resubmit", h.resubmitDepartmentBatch)
	v1.GET("/inspections/attendance/:id", h.attendanceDetail)
	v1.PATCH("/inspections/attendance/:id", h.updateAttendance)
	v1.DELETE("/inspections/attendance/:id", h.deleteAttendance)
	v1.POST("/inspections/attendance/batch", h.batchAttendance)
	v1.GET("/permissions/effective", h.currentEffectivePermissions)
	v1.GET("/dashboard/workbench", h.workbench)
	v1.GET("/imports/attendance/template", h.downloadAttendanceTemplate)
	v1.POST("/imports/attendance", h.uploadAttendanceImport)
	v1.GET("/imports/course-records/template", h.downloadAttendanceTemplate)
	v1.POST("/imports/course-records", h.uploadAttendanceImport)
	v1.GET("/imports/inspections/template", h.downloadAttendanceTemplate)
	v1.POST("/imports/inspections", h.uploadAttendanceImport)
	v1.GET("/imports/:id", h.getImportBatch)
	v1.GET("/imports/:id/rows", h.listImportRows)
	v1.POST("/imports/:id/confirm", h.confirmImportBatch)
	v1.POST("/imports/:id/cancel", h.cancelImportBatch)
	v1.POST("/imports/:id/rollback", h.rollbackImportBatch)
	return r
}

func (h *handler) currentEffectivePermissions(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.permission == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "权限服务尚未就绪")
		return
	}
	result, err := h.permission.Effective(c, account.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "PERMISSION_LOOKUP_FAILED", "无法读取当前权限")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) readiness(c *gin.Context) {
	if h.postgresReady == nil || h.objectStorageReady == nil || h.postgresReady.Ready(c.Request.Context()) != nil || h.objectStorageReady.Ready(c.Request.Context()) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) register(c *gin.Context) {
	var in struct {
		StudentNo             string `json:"studentNo"`
		Name                  string `json:"name"`
		Password              string `json:"password"`
		RequestedDepartmentID string `json:"requestedDepartmentId"`
		RequestedOrgRoleID    string `json:"requestedOrgRoleId"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请检查注册信息")
		return
	}
	a, err := h.auth.Register(c, auth.RegisterInput{StudentNo: in.StudentNo, Name: in.Name, Password: in.Password, RequestedDepartmentID: in.RequestedDepartmentID, RequestedOrgRoleID: in.RequestedOrgRoleID}, requestMetadata(c, ""))
	if errors.Is(err, auth.ErrLoginNameExists) {
		writeError(c, http.StatusConflict, "STUDENT_NO_EXISTS", "该学号已注册")
		return
	}
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REGISTRATION", "请完整填写注册信息，密码至少 8 位")
		return
	}
	c.JSON(http.StatusCreated, publicAccount(a))
}

func (h *handler) login(c *gin.Context) {
	var in struct {
		LoginName string `json:"loginName"`
		StudentNo string `json:"studentNo"`
		Password  string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请输入账号和密码")
		return
	}
	if in.LoginName == "" {
		in.LoginName = in.StudentNo
	}
	tokens, err := h.auth.Login(c, in.LoginName, in.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误，或账号尚未启用")
		} else {
			writeAuthServiceError(c, err)
		}
		return
	}
	h.setRefreshCookie(c, tokens)
	c.JSON(http.StatusOK, gin.H{"accessToken": tokens.AccessToken, "expiresAt": tokens.AccessExpiresAt})
}

func (h *handler) refresh(c *gin.Context) {
	raw, err := c.Cookie("refresh_token")
	if err != nil {
		writeError(c, http.StatusUnauthorized, "INVALID_SESSION", "登录状态已失效")
		return
	}
	tokens, err := h.auth.Refresh(c, raw)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			h.clearCookie(c)
			writeError(c, http.StatusUnauthorized, "INVALID_SESSION", "登录状态已失效")
		} else {
			writeAuthServiceError(c, err)
		}
		return
	}
	h.setRefreshCookie(c, tokens)
	c.JSON(http.StatusOK, gin.H{"accessToken": tokens.AccessToken, "expiresAt": tokens.AccessExpiresAt})
}

func (h *handler) logout(c *gin.Context) {
	raw, _ := c.Cookie("refresh_token")
	if err := h.auth.Logout(c, raw); err != nil {
		writeAuthServiceError(c, err)
		return
	}
	h.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *handler) me(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, publicAccount(a))
}

func (h *handler) changePassword(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请检查密码信息")
		return
	}
	if err := h.auth.ChangePassword(c, a, in.CurrentPassword, in.NewPassword, requestMetadata(c, "")); err != nil {
		if errors.Is(err, auth.ErrInvalidAccessToken) {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效，请重新登录")
		} else if errors.Is(err, auth.ErrVersionConflict) {
			writeError(c, http.StatusConflict, "VERSION_CONFLICT", "账号状态已变化，请重新登录后重试")
		} else if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "PASSWORD_CHANGE_FAILED", "当前密码错误，或新密码不符合要求")
		} else {
			writeAuthServiceError(c, err)
		}
		return
	}
	h.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *handler) registrationOptions(c *gin.Context) {
	if h.organization == nil {
		c.JSON(http.StatusOK, organization.Catalogs{})
		return
	}
	result, err := h.organization.ListCatalogs(c, true)
	writeResult(c, result, err)
}

func (h *handler) listRegistrationRequests(c *gin.Context) {
	result, err := h.auth.ListPendingRegistrations(c, c.MustGet("account").(auth.Account))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	public := make([]gin.H, 0, len(result))
	for _, account := range result {
		public = append(public, publicAccount(account))
	}
	c.JSON(http.StatusOK, public)
}

func (h *handler) reviewRegistration(c *gin.Context) {
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请检查审核信息")
		return
	}
	result, err := h.auth.ReviewRegistration(c, c.MustGet("account").(auth.Account), c.Param("id"), in.Decision, in.Reason, requestMetadata(c, in.Reason))
	if err != nil {
		writeRegistrationReviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, publicAccount(result))
}

func (h *handler) listCatalogs(c *gin.Context) {
	result, err := h.organization.ListCatalogs(c, false)
	writeResult(c, result, err)
}
func (h *handler) createDepartment(c *gin.Context) {
	var in organization.Department
	if bind(c, &in) {
		result, err := h.organization.CreateDepartment(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateDepartment(c *gin.Context) {
	var in organization.Department
	if bind(c, &in) {
		in.ID = c.Param("id")
		result, err := h.organization.UpdateDepartment(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) updateInspectionType(c *gin.Context) {
	var in organization.InspectionType
	if bind(c, &in) {
		in.ID = c.Param("id")
		result, err := h.organization.UpdateInspectionType(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) listScopes(c *gin.Context) {
	result, err := h.organization.ListScopes(c)
	writeResult(c, result, err)
}
func (h *handler) upsertScope(c *gin.Context) {
	var in organization.DepartmentInspectionScope
	if bind(c, &in) {
		writeNoContent(c, h.organization.UpsertScope(c, in, requestMetadata(c, "")))
	}
}
func (h *handler) listUsers(c *gin.Context) {
	result, err := h.organization.ListUsers(c)
	writeResult(c, result, err)
}
func (h *handler) createUser(c *gin.Context) {
	var in auth.AdminCreateUserInput
	if bind(c, &in) {
		password, err := auth.NewTemporaryPassword()
		if err != nil {
			writeResultStatus(c, http.StatusCreated, nil, err)
			return
		}
		in.TemporaryPassword = password
		result, err := h.auth.AdminCreateUser(c, c.MustGet("account").(auth.Account), in, requestMetadata(c, ""))
		if err != nil {
			writeResultStatus(c, http.StatusCreated, nil, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response := publicAccount(result)
		response["temporaryPassword"] = password
		writeResultStatus(c, http.StatusCreated, response, nil)
	}
}
func (h *handler) updateUser(c *gin.Context) {
	var in auth.AdminUpdateUserInput
	if bind(c, &in) {
		result, err := h.auth.AdminUpdateUser(c, c.MustGet("account").(auth.Account), c.Param("id"), in, requestMetadata(c, ""))
		writeResult(c, publicAccount(result), err)
	}
}
func (h *handler) updateUserStatus(c *gin.Context) {
	var in struct {
		Status string `json:"status"`
	}
	if bind(c, &in) {
		result, err := h.auth.AdminSetUserStatus(c, c.MustGet("account").(auth.Account), c.Param("id"), in.Status, requestMetadata(c, ""))
		writeResult(c, publicAccount(result), err)
	}
}
func (h *handler) resetUserPassword(c *gin.Context) {
	password, err := auth.NewTemporaryPassword()
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	if err := h.auth.AdminResetPassword(c, c.MustGet("account").(auth.Account), c.Param("id"), password, requestMetadata(c, "")); err != nil {
		writeResult(c, nil, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	writeResult(c, gin.H{"temporaryPassword": password}, nil)
}
func (h *handler) assignUserRole(c *gin.Context) {
	var in organization.UserAssignment
	if bind(c, &in) {
		in.AccountID = c.Param("id")
		writeNoContent(c, h.organization.AssignUserRole(c, in, requestMetadata(c, "")))
	}
}
func (h *handler) effectivePermissions(c *gin.Context) {
	if h.permission == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "权限服务尚未就绪")
		return
	}
	result, err := h.permission.Effective(c, c.Param("id"))
	writeResult(c, result, err)
}
func (h *handler) listPermissionRules(c *gin.Context) {
	if h.permission == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "权限服务尚未就绪")
		return
	}
	result, err := h.permission.ListRules(c, c.Param("id"))
	writeResult(c, result, err)
}
func (h *handler) createPermissionRule(c *gin.Context) {
	var in permission.Rule
	if bind(c, &in) {
		in.AccountID = c.Param("id")
		result, err := h.permission.CreateRule(c, c.MustGet("account").(auth.Account).ID, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updatePermissionRule(c *gin.Context) {
	var in permission.Rule
	if bind(c, &in) {
		in.ID, in.AccountID = c.Param("ruleId"), c.Param("id")
		result, err := h.permission.UpdateRule(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) deletePermissionRule(c *gin.Context) {
	writeNoContent(c, h.permission.DeleteRule(c, c.Param("id"), c.Param("ruleId"), requestMetadata(c, "")))
}
func (h *handler) listMasterData(c *gin.Context) {
	if h.masterData == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "基础数据服务尚未就绪")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	var enabled *bool
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "启用状态筛选值无效")
			return
		}
		enabled = &value
	}
	result, err := h.masterData.List(c, masterdata.Query{
		Kind: c.Param("kind"), Keyword: c.Query("keyword"), Code: c.Query("code"), Name: c.Query("name"), Enabled: enabled,
		FacultyID: c.Query("facultyId"), MajorID: c.Query("majorId"), HeadTeacherID: c.Query("headTeacherId"),
		ClassID: c.Query("classId"), DormitoryID: c.Query("dormitoryId"),
		ClassroomID: c.Query("classroomId"), GradeID: c.Query("gradeId"), Gender: c.Query("gender"),
		EnrollmentStatus: c.Query("enrollmentStatus"), CampusArea: c.Query("campusArea"),
		BuildingNo: c.Query("buildingNo"), TeachingBuildingID: c.Query("teachingBuildingId"), Page: page, PageSize: pageSize,
		DormitoryAreaID: c.Query("dormitoryAreaId"), DormitoryBuildingID: c.Query("dormitoryBuildingId"),
	})
	writeResult(c, result, err)
}
func (h *handler) createMasterData(c *gin.Context) {
	var in masterdata.Record
	if bind(c, &in) {
		in.Kind = c.Param("kind")
		result, err := h.masterData.Create(c, c.MustGet("account").(auth.Account).ID, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateMasterData(c *gin.Context) {
	var in masterdata.Record
	if bind(c, &in) {
		in.Kind, in.ID = c.Param("kind"), c.Param("id")
		result, err := h.masterData.Update(c, c.MustGet("account").(auth.Account).ID, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) listMasterDataChanges(c *gin.Context) {
	result, err := h.masterData.ListChanges(c, c.Param("kind"), c.Param("id"))
	writeResult(c, result, err)
}

func (h *handler) downloadMasterDataImportTemplate(c *gin.Context) {
	spec, ok := masterdataimport.SpecFor(c.Param("kind"))
	if !ok {
		writeMasterDataImportError(c, masterdataimport.ErrInvalidKind)
		return
	}
	content, err := masterdataimport.TemplateXLSX(spec.Kind)
	if err != nil {
		writeMasterDataImportError(c, err)
		return
	}
	filename := spec.Label + "导入模板.xlsx"
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *handler) getMasterDataImportSpec(c *gin.Context) {
	spec, ok := masterdataimport.SpecFor(c.Param("kind"))
	if !ok {
		writeMasterDataImportError(c, masterdataimport.ErrInvalidKind)
		return
	}
	c.JSON(http.StatusOK, spec)
}

func (h *handler) previewMasterDataImport(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.masterDataImports == nil || h.importFiles == nil || h.importMaxUploadBytes <= 0 {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "基础数据导入服务尚未就绪")
		return
	}
	if _, ok := masterdataimport.SpecFor(c.Param("kind")); !ok {
		writeMasterDataImportError(c, masterdataimport.ErrInvalidKind)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, multipartRequestLimit(h.importMaxUploadBytes))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		} else {
			writeError(c, http.StatusBadRequest, "INVALID_FILE", "请选择有效的 XLSX 文件")
		}
		return
	}
	if fileHeader.Size <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "上传文件不能为空")
		return
	}
	if fileHeader.Size > h.importMaxUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		return
	}
	opened, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "无法读取上传文件")
		return
	}
	content, readErr := io.ReadAll(io.LimitReader(opened, h.importMaxUploadBytes+1))
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(readErr, &maxBytesError) {
			writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
			return
		}
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "无法读取上传文件")
		return
	}
	if int64(len(content)) == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "上传文件不能为空")
		return
	}
	if int64(len(content)) > h.importMaxUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		return
	}
	stored, err := h.importFiles.Store(c, storage.StoreInput{
		OriginalName: fileHeader.Filename, DeclaredContentType: fileHeader.Header.Get("Content-Type"), Size: int64(len(content)),
		RetentionType: storage.RetentionImportSource, CreatedBy: account.ID, Body: bytes.NewReader(content),
	}, requestMetadata(c, ""))
	if err != nil {
		writeImportError(c, err)
		return
	}
	batch, err := h.masterDataImports.Preview(c, c.Param("kind"), stored.ID, fileHeader.Filename, account.ID, content, requestMetadata(c, ""))
	if err != nil {
		h.discardUnboundImportSource(c, stored.ID)
		writeMasterDataImportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, batch)
}

func (h *handler) getMasterDataImportBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.masterDataImports == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "基础数据导入服务尚未就绪")
		return
	}
	batch, err := h.masterDataImports.GetBatch(c, c.Param("kind"), c.Param("batchID"), account.ID)
	if err != nil {
		writeMasterDataImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, batch)
}

func (h *handler) confirmMasterDataImportBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.masterDataImports == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "基础数据导入服务尚未就绪")
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if !bind(c, &input) {
		return
	}
	result, err := h.masterDataImports.Confirm(c, masterdataimport.ConfirmInput{
		BatchID: c.Param("batchID"), Kind: c.Param("kind"), ActorID: account.ID,
		ExpectedVersion: input.Version, IdempotencyKey: c.GetHeader("Idempotency-Key"), Metadata: requestMetadata(c, ""),
	})
	if err != nil {
		writeMasterDataImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func writeMasterDataImportError(c *gin.Context, err error) {
	var parseError *importjob.ParseError
	if errors.As(err, &parseError) {
		writeError(c, http.StatusUnprocessableEntity, string(parseError.Code), parseError.Message)
		return
	}
	switch {
	case errors.Is(err, masterdataimport.ErrInvalidKind):
		writeError(c, http.StatusBadRequest, "INVALID_MASTER_DATA_KIND", "该基础数据类型暂不支持 Excel 导入")
	case errors.Is(err, masterdataimport.ErrBatchNotFound):
		writeError(c, http.StatusNotFound, "MASTER_DATA_IMPORT_NOT_FOUND", "导入预览不存在或无权访问")
	case errors.Is(err, masterdataimport.ErrBatchVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "导入预览已变化，请刷新后重试")
	case errors.Is(err, masterdataimport.ErrBatchHasErrors):
		writeError(c, http.StatusUnprocessableEntity, "IMPORT_HAS_ERRORS", "预览中仍有错误，请修正 Excel 后重新上传")
	case errors.Is(err, masterdataimport.ErrAlreadyConfirmed):
		writeError(c, http.StatusConflict, "IMPORT_ALREADY_CONFIRMED", "该批次已使用其他请求确认")
	case errors.Is(err, masterdataimport.ErrInvalidIdempotencyKey), errors.Is(err, masterdataimport.ErrInvalidBatch):
		writeError(c, http.StatusBadRequest, "INVALID_IMPORT_REQUEST", "导入确认参数无效")
	case errors.Is(err, masterdataimport.ErrEmptyWorkbook):
		writeError(c, http.StatusUnprocessableEntity, "EMPTY_IMPORT", "文件中没有可导入的数据行")
	case errors.Is(err, masterdataimport.ErrPreviewStale):
		writeError(c, http.StatusConflict, "IMPORT_PREVIEW_STALE", "预览关联的基础数据已变化，请重新上传并校验")
	case errors.Is(err, masterdata.ErrDuplicateCode), errors.Is(err, masterdata.ErrDuplicateName), errors.Is(err, masterdata.ErrInvalidRelation):
		writeDomainError(c, err)
	default:
		log.Printf("master data import failed: method=%s route=%s errorType=%T", c.Request.Method, c.FullPath(), err)
		writeError(c, http.StatusInternalServerError, "MASTER_DATA_IMPORT_UNAVAILABLE", "基础数据导入失败，请稍后重试")
	}
}

func (h *handler) listDictionaries(c *gin.Context) {
	if h.dictionary == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "字典服务尚未就绪")
		return
	}
	result, err := h.dictionary.List(c, c.Query("typeId"))
	writeResult(c, result, err)
}
func (h *handler) createDictionaryType(c *gin.Context) {
	var in dictionary.Type
	if bind(c, &in) {
		result, err := h.dictionary.CreateType(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateDictionaryType(c *gin.Context) {
	var in dictionary.Type
	if bind(c, &in) {
		in.ID = c.Param("typeId")
		result, err := h.dictionary.UpdateType(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) createDictionaryItem(c *gin.Context) {
	var in dictionary.Item
	if bind(c, &in) {
		in.TypeID = c.Param("typeId")
		result, err := h.dictionary.CreateItem(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateDictionaryItem(c *gin.Context) {
	var in dictionary.Item
	if bind(c, &in) {
		in.ID, in.TypeID = c.Param("id"), c.Param("typeId")
		result, err := h.dictionary.UpdateItem(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) listCalendar(c *gin.Context) {
	result, err := h.academic.List(c)
	writeResult(c, result, err)
}
func (h *handler) createAcademicYear(c *gin.Context) {
	var in academic.AcademicYear
	if bind(c, &in) {
		result, err := h.academic.CreateYear(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateAcademicYear(c *gin.Context) {
	var in academic.AcademicYear
	if bind(c, &in) {
		in.ID = c.Param("id")
		result, err := h.academic.UpdateYear(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) createSemester(c *gin.Context) {
	var in academic.Semester
	if bind(c, &in) {
		result, err := h.academic.CreateSemester(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateSemester(c *gin.Context) {
	var in academic.Semester
	if bind(c, &in) {
		in.ID = c.Param("id")
		result, err := h.academic.UpdateSemester(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) createWeek(c *gin.Context) {
	var in academic.Week
	if bind(c, &in) {
		result, err := h.academic.CreateWeek(c, in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, result, err)
	}
}
func (h *handler) updateWeek(c *gin.Context) {
	var in academic.Week
	if bind(c, &in) {
		in.ID = c.Param("id")
		result, err := h.academic.UpdateWeek(c, in, requestMetadata(c, ""))
		writeResult(c, result, err)
	}
}
func (h *handler) currentWeek(c *gin.Context) {
	if h.academic == nil {
		c.JSON(http.StatusOK, gin.H{"week": nil})
		return
	}
	result, err := h.academic.CurrentWeek(c, time.Now())
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"week": result})
}

type attendanceClassOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Grade string `json:"grade"`
}

type attendanceFilterOptionsResponse struct {
	InspectionType         organization.InspectionType `json:"inspectionType"`
	ResponsibleDepartments []organization.Department   `json:"responsibleDepartments"`
	Departments            []organization.Department   `json:"departments"`
	Semesters              []academic.Semester         `json:"semesters"`
	Weeks                  []academic.Week             `json:"weeks"`
	Classes                []attendanceClassOption     `json:"classes"`
	Grades                 []string                    `json:"grades"`
}

func attendanceResponsibleDepartments(departments []organization.Department, scopes []organization.DepartmentInspectionScope, inspectionTypeID string) []organization.Department {
	responsibleIDs := make(map[string]struct{})
	for _, scope := range scopes {
		if scope.Enabled && scope.CanEnter && scope.InspectionTypeID == inspectionTypeID {
			responsibleIDs[scope.DepartmentID] = struct{}{}
		}
	}
	result := make([]organization.Department, 0)
	for _, department := range departments {
		if _, responsible := responsibleIDs[department.ID]; department.Enabled && responsible {
			result = append(result, department)
		}
	}
	return result
}

func attendanceInspectionType(values []organization.InspectionType) (organization.InspectionType, bool) {
	for _, value := range values {
		if value.Code == inspection.InspectionTypeAttendance && value.Enabled {
			return value, true
		}
	}
	return organization.InspectionType{}, false
}

func (h *handler) listAttendance(c *gin.Context) {
	account, scope, allowedDepartments, ok := h.authorizeAttendanceList(c)
	if !ok {
		return
	}
	_ = account
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	filter := inspection.AttendanceListFilter{
		SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID,
		DepartmentID: scope.DepartmentID, ClassID: scope.ClassID, Grade: scope.Grade,
		Keyword: c.Query("keyword"), Status: c.Query("status"), Source: c.Query("source"),
		Sort: c.DefaultQuery("sort", inspection.SortOccurredAtDescending), Page: page, PageSize: pageSize,
	}
	result, err := h.attendance.ListAttendance(c, filter, inspection.AttendanceScope{
		SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID,
		DepartmentID: scope.DepartmentID, DepartmentIDs: allowedDepartments, ClassID: scope.ClassID, Grade: scope.Grade,
	})
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) authorizeAttendanceList(c *gin.Context) (auth.Account, permission.Scope, []string, bool) {
	scope := permission.Scope{
		SemesterID: c.Query("semesterId"), WeekID: c.Query("weekId"), InspectionTypeID: c.Query("inspectionTypeId"),
		DepartmentID: c.Query("departmentId"), ClassID: c.Query("classId"), Grade: c.Query("grade"),
	}
	account, ok := h.currentAccount(c)
	if !ok {
		return auth.Account{}, permission.Scope{}, nil, false
	}
	if account.MustChangePassword || strings.TrimSpace(scope.SemesterID) == "" || strings.TrimSpace(scope.WeekID) == "" || strings.TrimSpace(scope.InspectionTypeID) == "" {
		writeInspectionForbidden(c, false)
		return auth.Account{}, permission.Scope{}, nil, false
	}
	if account.IsSuperAdmin {
		return account, scope, nil, true
	}
	if h.permission == nil || h.organization == nil {
		writeInspectionForbidden(c, false)
		return auth.Account{}, permission.Scope{}, nil, false
	}
	var scopeOK bool
	scope, scopeOK = h.inspectionScopeWithAcademicYear(c, scope, false)
	if !scopeOK {
		return auth.Account{}, permission.Scope{}, nil, false
	}
	effective, err := h.permission.Effective(c, account.ID)
	if err != nil {
		writeInspectionForbidden(c, false)
		return auth.Account{}, permission.Scope{}, nil, false
	}
	if scope.DepartmentID != "" {
		requested := scope
		if requested.DepartmentID == inspection.CollegeDepartmentOptionID {
			requested.DepartmentID = ""
		}
		if !effective.Allowed(permission.ActionInspectionView, requested) {
			writeInspectionForbidden(c, false)
			return auth.Account{}, permission.Scope{}, nil, false
		}
		return account, scope, nil, true
	}
	catalogs, err := h.organization.ListCatalogs(c, true)
	if err != nil {
		writeDomainError(c, err)
		return auth.Account{}, permission.Scope{}, nil, false
	}
	departments := make([]string, 0)
	for _, department := range catalogs.Departments {
		candidate := scope
		candidate.DepartmentID = department.ID
		if effective.Allowed(permission.ActionInspectionView, candidate) {
			departments = append(departments, department.ID)
		}
	}
	college := scope
	college.DepartmentID = ""
	if effective.Allowed(permission.ActionInspectionView, college) {
		departments = append(departments, inspection.CollegeDepartmentOptionID)
	}
	if len(departments) == 0 {
		writeInspectionForbidden(c, false)
		return auth.Account{}, permission.Scope{}, nil, false
	}
	return account, scope, departments, true
}

func (h *handler) downloadAttendanceTemplate(c *gin.Context) {
	account, scope, ok := h.authorizeInspectionRequest(c, permission.ActionInspectionView)
	if !ok {
		return
	}
	template, err := h.importJobs.PublishedTemplate(c, scope.InspectionTypeID)
	if err != nil || template.InspectionTypeID != scope.InspectionTypeID {
		writeImportError(c, err)
		return
	}
	if !courseTemplateAllowed(c, template.Code) {
		writeError(c, http.StatusUnprocessableEntity, "TEMPLATE_VERSION_MISMATCH", "模板类型与当前导入入口不一致")
		return
	}
	file, content, err := h.importFiles.ReadVerified(c, template.FileID, requestMetadata(c, ""))
	if err != nil {
		writeImportError(c, err)
		return
	}
	_ = account
	name := strings.TrimSpace(file.OriginalName)
	if name == "" {
		name = "attendance-template.xlsx"
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Header("X-Import-Template-ID", template.ID)
	c.Header("X-Import-Template-Version", strconv.Itoa(template.Version))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

// Compensation outlives request cancellation but remains bounded to the uploaded source.
func (h *handler) discardUnboundImportSource(c *gin.Context, fileID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	if err := h.importFiles.Discard(ctx, fileID, requestMetadata(c, "导入批次创建失败，回收未关联源文件")); err != nil {
		log.Printf("import source compensation failed: method=%s route=%s fileId=%s errorType=%T", c.Request.Method, c.FullPath(), fileID, err)
	}
}

func (h *handler) uploadAttendanceImport(c *gin.Context) {
	requester, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.importMaxUploadBytes <= 0 {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "导入服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, multipartRequestLimit(h.importMaxUploadBytes))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		} else {
			writeError(c, http.StatusBadRequest, "INVALID_FILE", "请选择有效的 XLSX 文件")
		}
		return
	}
	scope := inspectionScopeFromForm(c)
	departmentID, err := inspection.ResolveEntryDepartment(inspection.ManualActor{ID: requester.ID, IsSuperAdmin: requester.IsSuperAdmin, IsMember: requester.SystemRole == auth.SystemRoleMember, PrimaryDepartmentID: requester.PrimaryDepartmentID}, scope.DepartmentID)
	if err != nil {
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "当前账号无有效录入部门或不能选择其他部门")
		return
	}
	scope.DepartmentID = departmentID
	account, ok := h.authorizeInspectionScope(c, permission.ActionInspectionEnter, scope, false)
	if !ok {
		return
	}
	templateVersion, err := strconv.Atoi(c.PostForm("templateVersion"))
	if err != nil || templateVersion < 1 || strings.TrimSpace(c.PostForm("templateId")) == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "模板信息无效")
		return
	}
	published, err := h.importJobs.PublishedTemplate(c, scope.InspectionTypeID)
	if err != nil {
		writeImportError(c, err)
		return
	}
	if !courseTemplateAllowed(c, published.Code) {
		writeError(c, http.StatusUnprocessableEntity, "TEMPLATE_VERSION_MISMATCH", "模板类型与当前导入入口不一致")
		return
	}
	if published.ID != c.PostForm("templateId") || published.Version != templateVersion || published.InspectionTypeID != scope.InspectionTypeID {
		writeError(c, http.StatusUnprocessableEntity, string(importjob.ErrorTemplateVersionMismatch), "模板版本与当前发布版本不一致")
		return
	}
	opened, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "无法读取上传文件")
		return
	}
	defer opened.Close()
	stored, err := h.importFiles.Store(c, storage.StoreInput{
		OriginalName: fileHeader.Filename, DeclaredContentType: fileHeader.Header.Get("Content-Type"), Size: fileHeader.Size,
		RetentionType: storage.RetentionImportSource, CreatedBy: account.ID, Body: opened,
	}, requestMetadata(c, ""))
	if err != nil {
		writeImportError(c, err)
		return
	}
	batch, err := h.importJobs.CreateBatch(c, importjob.CreateBatchInput{
		InspectionTypeID: scope.InspectionTypeID, SemesterID: scope.SemesterID, WeekID: scope.WeekID, DepartmentID: scope.DepartmentID,
		ClassID: scope.ClassID, Grade: scope.Grade,
		SourceFile: stored, TemplateID: c.PostForm("templateId"), TemplateVersion: templateVersion, CreatedBy: account.ID,
	})
	if err != nil {
		h.discardUnboundImportSource(c, stored.ID)
		writeImportError(c, err)
		return
	}
	batch, err = h.importJobs.ParseBatch(c, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, AuditMetadata: requestMetadata(c, "")})
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, batch)
}

func (h *handler) getImportBatch(c *gin.Context) {
	_, batch, ok := h.authorizedBatch(c, permission.ActionInspectionView)
	if ok {
		c.JSON(http.StatusOK, batch)
	}
}

func (h *handler) listImportRows(c *gin.Context) {
	_, batch, ok := h.authorizedBatch(c, permission.ActionInspectionView)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	validationStatus := strings.ToUpper(strings.TrimSpace(c.Query("validationStatus")))
	result, err := h.importJobs.ListRows(c, batch.ID, page, pageSize, validationStatus)
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) confirmImportBatch(c *gin.Context) {
	account, batch, ok := h.authorizedBatch(c, permission.ActionInspectionEnter)
	if !ok {
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if !bind(c, &input) {
		return
	}
	result, err := h.importJobs.ConfirmBatch(c, importjob.ConfirmBatchInput{
		BatchID: batch.ID, ExpectedVersion: input.Version, IdempotencyKey: c.GetHeader("Idempotency-Key"),
		Scope: importjob.ConfirmScope{SemesterID: batch.SemesterID, WeekID: batch.WeekID, InspectionTypeID: batch.InspectionTypeID,
			DepartmentID: batch.DepartmentID, ClassID: batch.ClassID, Grade: batch.Grade, ActorID: account.ID},
		AuditMetadata: requestMetadata(c, ""),
	})
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) cancelImportBatch(c *gin.Context) {
	_, batch, ok := h.authorizedBatch(c, permission.ActionInspectionEnter)
	if !ok {
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if !bind(c, &input) {
		return
	}
	result, err := h.importJobs.CancelBatch(c, batch.ID, input.Version, requestMetadata(c, ""))
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) authorizeInspectionRequest(c *gin.Context, action string) (auth.Account, permission.Scope, bool) {
	scope := permission.Scope{
		SemesterID: c.Query("semesterId"), WeekID: c.Query("weekId"), InspectionTypeID: c.Query("inspectionTypeId"),
		DepartmentID: c.Query("departmentId"), ClassID: c.Query("classId"), Grade: c.Query("grade"),
	}
	if scope.DepartmentID == inspection.CollegeDepartmentOptionID {
		scope.DepartmentID = ""
	}
	account, ok := h.authorizeInspectionScope(c, action, scope, false)
	return account, scope, ok
}

func inspectionScopeFromForm(c *gin.Context) permission.Scope {
	departmentID := c.PostForm("departmentId")
	if departmentID == inspection.CollegeDepartmentOptionID {
		departmentID = ""
	}
	return permission.Scope{SemesterID: c.PostForm("semesterId"), WeekID: c.PostForm("weekId"), InspectionTypeID: c.PostForm("inspectionTypeId"), DepartmentID: departmentID, ClassID: c.PostForm("classId"), Grade: c.PostForm("grade")}
}

func (h *handler) authorizeInspectionScope(c *gin.Context, action string, scope permission.Scope, hideExistence bool) (auth.Account, bool) {
	account, ok := h.currentAccount(c)
	if !ok {
		return auth.Account{}, false
	}
	if account.MustChangePassword || strings.TrimSpace(scope.SemesterID) == "" || strings.TrimSpace(scope.WeekID) == "" || strings.TrimSpace(scope.InspectionTypeID) == "" {
		writeInspectionForbidden(c, hideExistence)
		return auth.Account{}, false
	}
	if account.IsSuperAdmin {
		return account, true
	}
	if h.permission == nil {
		writeInspectionForbidden(c, hideExistence)
		return auth.Account{}, false
	}
	var scopeOK bool
	scope, scopeOK = h.inspectionScopeWithAcademicYear(c, scope, hideExistence)
	if !scopeOK {
		return auth.Account{}, false
	}
	effective, err := h.permission.Effective(c, account.ID)
	if err != nil || !effective.Allowed(action, scope) {
		writeInspectionForbidden(c, hideExistence)
		return auth.Account{}, false
	}
	return account, true
}

func (h *handler) authorizedBatch(c *gin.Context, action string) (auth.Account, importjob.Batch, bool) {
	account, ok := h.currentAccount(c)
	if !ok {
		return auth.Account{}, importjob.Batch{}, false
	}
	// Password reset restrictions apply before any business access, including super administrators.
	if account.MustChangePassword {
		writeInspectionForbidden(c, true)
		return auth.Account{}, importjob.Batch{}, false
	}
	batch, err := h.importJobs.GetBatch(c, c.Param("id"))
	if err != nil {
		writeImportError(c, err)
		return auth.Account{}, importjob.Batch{}, false
	}
	scope := permission.Scope{SemesterID: batch.SemesterID, WeekID: batch.WeekID, InspectionTypeID: batch.InspectionTypeID, DepartmentID: batch.DepartmentID, ClassID: batch.ClassID, Grade: batch.Grade}
	if account.IsSuperAdmin {
		return account, batch, true
	}
	if h.permission == nil {
		writeInspectionForbidden(c, true)
		return auth.Account{}, importjob.Batch{}, false
	}
	var scopeOK bool
	scope, scopeOK = h.inspectionScopeWithAcademicYear(c, scope, true)
	if !scopeOK {
		return auth.Account{}, importjob.Batch{}, false
	}
	effective, err := h.permission.Effective(c, account.ID)
	if err != nil || !effective.Allowed(action, scope) {
		writeInspectionForbidden(c, true)
		return auth.Account{}, importjob.Batch{}, false
	}
	return account, batch, true
}

// inspectionScopeWithAcademicYear derives the permission dimension from the
// authoritative semester, never from a client-provided year.
func (h *handler) inspectionScopeWithAcademicYear(c *gin.Context, scope permission.Scope, hideExistence bool) (permission.Scope, bool) {
	if h.academic == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "校历服务尚未就绪")
		return permission.Scope{}, false
	}
	calendar, err := h.academic.List(c)
	if err != nil {
		writeDomainError(c, err)
		return permission.Scope{}, false
	}
	for _, semester := range calendar.Semesters {
		if semester.ID == scope.SemesterID && semester.AcademicYearID != "" {
			scope.AcademicYearID = semester.AcademicYearID
			return scope, true
		}
	}
	writeInspectionForbidden(c, hideExistence)
	return permission.Scope{}, false
}

func writeInspectionForbidden(c *gin.Context, hideExistence bool) {
	if hideExistence {
		writeError(c, http.StatusNotFound, "IMPORT_BATCH_NOT_FOUND", "导入批次不存在或无权访问")
		return
	}
	writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权访问当前检查范围")
}

func pagination(c *gin.Context) (int, int, bool) {
	page, errPage := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, errPageSize := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if errPage != nil || errPageSize != nil || page < 1 || pageSize < 1 || pageSize > 100 {
		writeError(c, http.StatusBadRequest, "INVALID_PAGINATION", "分页参数无效")
		return 0, 0, false
	}
	return page, pageSize, true
}

func multipartRequestLimit(fileLimit int64) int64 {
	const envelopeAllowance = int64(1 << 20)
	if fileLimit > (1<<63-1)-envelopeAllowance {
		return 1<<63 - 1
	}
	return fileLimit + envelopeAllowance
}

func writeImportError(c *gin.Context, err error) {
	switch {
	case err == nil:
		writeError(c, http.StatusNotFound, "IMPORT_TEMPLATE_NOT_FOUND", "导入模板不存在")
	case errors.Is(err, importjob.ErrBatchNotFound):
		writeError(c, http.StatusNotFound, "IMPORT_BATCH_NOT_FOUND", "导入批次不存在或无权访问")
	case errors.Is(err, importjob.ErrTemplateNotFound):
		writeError(c, http.StatusNotFound, "IMPORT_TEMPLATE_NOT_FOUND", "当前检查类型尚未发布导入模板")
	case errors.Is(err, importjob.ErrBatchScopeMismatch):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权访问当前检查范围")
	case errors.Is(err, importjob.ErrBatchVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "数据已更新，请刷新后重试")
	case errors.Is(err, importjob.ErrRollbackUnsafe):
		writeError(c, http.StatusConflict, "IMPORT_ROLLBACK_UNSAFE", "批次含已修改、已提交、已锁定或缺失记录，不能整批回滚")
	case errors.Is(err, inspection.ErrAttendanceDuplicate):
		writeError(c, http.StatusConflict, "RECORD_ALREADY_EXISTS", "已有相同检查记录，请修正文件并重新上传校验")
	case errors.Is(err, inspection.ErrAttendancePenaltyUnconfigured):
		writeError(c, http.StatusConflict, "ATTENDANCE_WEEK_RULE_UNCONFIGURED", "该周到课率违纪计入规则尚未配置，请联系管理员先完成校历配置")
	case errors.Is(err, importjob.ErrPreviewStale):
		writeError(c, http.StatusConflict, "IMPORT_PREVIEW_STALE", "基础数据或检查范围已变化，请重新上传并校验后确认")
	case errors.Is(err, importjob.ErrBatchHasErrors):
		writeError(c, http.StatusConflict, "BATCH_HAS_ERRORS", "批次仍有错误，不能确认")
	case errors.Is(err, importjob.ErrBatchAlreadyConfirmed):
		writeError(c, http.StatusConflict, "BATCH_ALREADY_CONFIRMED", "批次已使用其他请求确认")
	case errors.Is(err, importjob.ErrInvalidBatchTransition):
		writeError(c, http.StatusConflict, "INVALID_BATCH_STATE", "当前批次状态不允许该操作")
	case errors.Is(err, importjob.ErrInvalidIdempotencyKey):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "确认请求缺少有效幂等键")
	case errors.Is(err, importjob.ErrInvalidBatch), errors.Is(err, importjob.ErrSourceFileNotReady), errors.Is(err, inspection.ErrInvalidAttendanceQuery):
		writeError(c, http.StatusBadRequest, "INVALID_IMPORT_REQUEST", "导入请求不符合要求")
	case errors.Is(err, storage.ErrFileTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
	case errors.Is(err, storage.ErrFileUnavailable), errors.Is(err, storage.ErrIntegrityMismatch), errors.Is(err, storage.ErrObjectMissing):
		writeError(c, http.StatusConflict, "SOURCE_FILE_UNAVAILABLE", "文件不可用或完整性校验失败")
	default:
		var parsed *importjob.ParseError
		if errors.As(err, &parsed) {
			writeError(c, http.StatusUnprocessableEntity, string(parsed.Code), parsed.Message)
			return
		}
		// Keep diagnostics without exposing SQL, source rows, or credentials.
		log.Printf("import operation failed: method=%s route=%s errorType=%T", c.Request.Method, c.FullPath(), err)
		writeError(c, http.StatusInternalServerError, "IMPORT_SERVICE_UNAVAILABLE", "导入服务暂时不可用，请稍后重试")
	}
}

func (h *handler) requireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := h.currentAccount(c)
		if !ok {
			c.Abort()
			return
		}
		if !a.IsSuperAdmin || a.MustChangePassword {
			writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		c.Set("account", a)
		c.Next()
	}
}

func (h *handler) requireAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := h.currentAccount(c)
		if !ok {
			c.Abort()
			return
		}
		if a.MustChangePassword {
			writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		if a.IsSuperAdmin {
			c.Set("account", a)
			c.Next()
			return
		}
		if h.permission == nil {
			writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		effective, err := h.permission.Effective(c, a.ID)
		if err != nil || !effective.Allowed(action, permission.Scope{}) {
			writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		c.Set("account", a)
		c.Next()
	}
}

func (h *handler) requireHygieneAreaManage() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := h.currentAccount(c)
		if !ok {
			c.Abort()
			return
		}
		if a.MustChangePassword {
			writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		if h.masterData == nil {
			writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "基础数据服务尚未就绪")
			c.Abort()
			return
		}
		typeID, err := h.masterData.HygieneAreaInspectionScope(c, c.Param("kind"), c.Query("departmentId"))
		if errors.Is(err, masterdata.ErrInvalidKind) || errors.Is(err, masterdata.ErrInvalidRelation) {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "区域类型或目标部门无效")
			c.Abort()
			return
		}
		if err != nil {
			writeResult(c, nil, err)
			c.Abort()
			return
		}
		if !a.IsSuperAdmin {
			if h.permission == nil {
				writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
				c.Abort()
				return
			}
			effective, effectiveErr := h.permission.Effective(c, a.ID)
			if effectiveErr != nil || !effective.Allowed(permission.ActionMasterDataManage, permission.Scope{DepartmentID: c.Query("departmentId"), InspectionTypeID: typeID}) {
				writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
				c.Abort()
				return
			}
		}
		c.Set("account", a)
		c.Next()
	}
}

func (h *handler) currentAccount(c *gin.Context) (auth.Account, bool) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
		return auth.Account{}, false
	}
	a, err := h.auth.AuthenticateAccessToken(c, strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidAccessToken) {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		} else {
			writeAuthServiceError(c, err)
		}
		return auth.Account{}, false
	}
	c.Set("account", a)
	return a, true
}

func (h *handler) setRefreshCookie(c *gin.Context, t auth.Tokens) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", t.RefreshToken, int(time.Until(t.RefreshExpiresAt).Seconds()), "/api/v1/auth", "", h.secureCookie, true)
}
func (h *handler) clearCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, "/api/v1/auth", "", h.secureCookie, true)
}
func publicAccount(a auth.Account) gin.H {
	return gin.H{"id": a.ID, "loginName": a.LoginName, "studentNo": a.StudentNo, "name": a.Name, "status": a.Status, "accountKind": a.AccountKind, "systemRole": a.SystemRole, "isSuperAdmin": a.IsSuperAdmin, "mustChangePassword": a.MustChangePassword, "primaryDepartmentId": a.PrimaryDepartmentID, "requestedDepartmentId": a.RequestedDepartmentID, "requestedOrgRoleId": a.RequestedOrgRoleID}
}
func bind(c *gin.Context, target any) bool {
	if c.ShouldBindJSON(target) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请检查提交信息")
		return false
	}
	return true
}
func writeNoContent(c *gin.Context, err error) {
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func writeResult(c *gin.Context, result any, err error) {
	writeResultStatus(c, http.StatusOK, result, err)
}
func writeResultStatus(c *gin.Context, status int, result any, err error) {
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(status, result)
}
func writeDomainError(c *gin.Context, err error) {
	if errors.Is(err, masterdata.ErrDuplicateCode) {
		writeError(c, http.StatusConflict, "MASTER_DATA_IDENTIFIER_EXISTS", "该学号或工号已存在")
		return
	}
	if errors.Is(err, masterdata.ErrDuplicateName) {
		writeError(c, http.StatusConflict, "MASTER_DATA_NAME_EXISTS", "该名称已存在")
		return
	}
	if errors.Is(err, auth.ErrForbidden) {
		writeError(c, http.StatusForbidden, "FORBIDDEN", "无权执行该操作")
		return
	}
	if errors.Is(err, auth.ErrInvalidTransition) {
		writeError(c, http.StatusConflict, "INVALID_STATE", "当前状态不允许该操作")
		return
	}
	if errors.Is(err, auth.ErrVersionConflict) {
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "账号状态已变化，请刷新后重试")
		return
	}
	switch {
	case errors.Is(err, masterdata.ErrNotFound), errors.Is(err, dictionary.ErrNotFound), errors.Is(err, auth.ErrAccountNotFound), errors.Is(err, organization.ErrUserNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "记录不存在或已被删除")
	case errors.Is(err, academic.ErrDateOverlap):
		writeError(c, http.StatusConflict, "ACADEMIC_DATE_OVERLAP", "日期范围重叠，请核对校历")
	case errors.Is(err, dictionary.ErrDuplicateValue):
		writeError(c, http.StatusConflict, "DICTIONARY_VALUE_EXISTS", "字典值已存在")
	case errors.Is(err, dictionary.ErrSystemTypeProtected):
		writeError(c, http.StatusConflict, "DICTIONARY_TYPE_PROTECTED", "系统字典类型不能修改")
	case errors.Is(err, auth.ErrLoginNameExists):
		writeError(c, http.StatusConflict, "ACCOUNT_IDENTIFIER_EXISTS", "登录账号已存在")
	case errors.Is(err, academic.ErrInvalidInput), errors.Is(err, masterdata.ErrInvalidRecord), errors.Is(err, masterdata.ErrInvalidRelation), errors.Is(err, masterdata.ErrInvalidKind), errors.Is(err, masterdata.ErrImmutableCode), errors.Is(err, dictionary.ErrInvalidDictionary), errors.Is(err, organization.ErrInvalidInput), errors.Is(err, permission.ErrInvalidRule), errors.Is(err, auth.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "提交信息不符合要求")
	default:
		// 数据库错误可能包含 SQL 和个人信息，只记录路由模板及错误类型。
		log.Printf("domain operation failed: method=%s route=%s errorType=%T", c.Request.Method, c.FullPath(), err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试")
	}
}
func writeRegistrationReviewError(c *gin.Context, err error) {
	if errors.Is(err, auth.ErrForbidden) {
		writeError(c, http.StatusForbidden, "FORBIDDEN", "无权执行该操作")
		return
	}
	if errors.Is(err, auth.ErrInvalidTransition) {
		writeError(c, http.StatusConflict, "INVALID_STATE", "该申请已被处理，请刷新列表")
		return
	}
	if errors.Is(err, auth.ErrAccountNotFound) {
		writeError(c, http.StatusNotFound, "REGISTRATION_NOT_FOUND", "注册申请不存在或已被删除")
		return
	}
	if errors.Is(err, auth.ErrInvalidInput) {
		writeError(c, http.StatusBadRequest, "INVALID_REVIEW", "请检查审核决定，驳回时必须填写原因")
		return
	}
	log.Printf("registration review failed: %v", err)
	writeError(c, http.StatusInternalServerError, "REGISTRATION_REVIEW_FAILED", "审核失败，请稍后重试")
}
func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}

// Infrastructure failures do not prove a session is invalid. Keep credentials
// untouched so the user can retry after the dependency recovers.
func writeAuthServiceError(c *gin.Context, err error) {
	log.Printf("auth operation failed: method=%s route=%s errorType=%T", c.Request.Method, c.FullPath(), err)
	writeError(c, http.StatusInternalServerError, "AUTH_SERVICE_UNAVAILABLE", "认证服务暂时不可用，请稍后重试")
}

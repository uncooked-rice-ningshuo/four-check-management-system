package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"github.com/gin-gonic/gin"
)

type AdminServices struct {
	Organization       *organization.Service
	Academic           *academic.Service
	Permission         *permission.Service
	MasterData         *masterdata.Service
	Dictionary         *dictionary.Service
	Storage            *storage.Service
	PostgresReady      readinessChecker
	ObjectStorageReady readinessChecker
}

type readinessChecker interface {
	Ready(context.Context) error
}

type handler struct {
	auth               *auth.Service
	organization       *organization.Service
	academic           *academic.Service
	permission         *permission.Service
	masterData         *masterdata.Service
	dictionary         *dictionary.Service
	storage            *storage.Service
	postgresReady      readinessChecker
	objectStorageReady readinessChecker
	secureCookie       bool
}

func NewRouter(authService *auth.Service, secureCookie bool, dependencies ...AdminServices) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(requestMetadataMiddleware(), gin.Recovery())
	h := &handler{auth: authService, secureCookie: secureCookie}
	if len(dependencies) > 0 {
		h.organization, h.academic, h.permission = dependencies[0].Organization, dependencies[0].Academic, dependencies[0].Permission
		h.masterData, h.dictionary = dependencies[0].MasterData, dependencies[0].Dictionary
		h.storage = dependencies[0].Storage
		h.postgresReady, h.objectStorageReady = dependencies[0].PostgresReady, dependencies[0].ObjectStorageReady
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
	masterDataAdmin := v1.Group("/admin", h.requireAction(permission.ActionMasterDataManage))
	masterDataAdmin.GET("/master-data/:kind", h.listMasterData)
	masterDataAdmin.POST("/master-data/:kind", h.createMasterData)
	masterDataAdmin.PUT("/master-data/:kind/:id", h.updateMasterData)
	masterDataAdmin.GET("/master-data/:kind/:id/changes", h.listMasterDataChanges)
	dictionaryAdmin := v1.Group("/admin", h.requireAction(permission.ActionDictionaryManage))
	dictionaryAdmin.GET("/dictionaries", h.listDictionaries)
	dictionaryAdmin.POST("/dictionaries/types", h.createDictionaryType)
	dictionaryAdmin.PUT("/dictionaries/types/:typeId", h.updateDictionaryType)
	dictionaryAdmin.POST("/dictionaries/types/:typeId/items", h.createDictionaryItem)
	dictionaryAdmin.PUT("/dictionaries/types/:typeId/items/:id", h.updateDictionaryItem)
	v1.GET("/academic/current-week", h.currentWeek)
	return r
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
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误，或账号尚未启用")
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
		h.clearCookie(c)
		writeError(c, http.StatusUnauthorized, "INVALID_SESSION", "登录状态已失效")
		return
	}
	h.setRefreshCookie(c, tokens)
	c.JSON(http.StatusOK, gin.H{"accessToken": tokens.AccessToken, "expiresAt": tokens.AccessExpiresAt})
}

func (h *handler) logout(c *gin.Context) {
	raw, _ := c.Cookie("refresh_token")
	_ = h.auth.Logout(c, raw)
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
		writeError(c, http.StatusBadRequest, "PASSWORD_CHANGE_FAILED", "当前密码错误，或新密码不符合要求")
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
		result, err := h.auth.AdminCreateUser(c, c.MustGet("account").(auth.Account), in, requestMetadata(c, ""))
		writeResultStatus(c, http.StatusCreated, publicAccount(result), err)
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
	var in struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if bind(c, &in) {
		writeNoContent(c, h.auth.AdminResetPassword(c, c.MustGet("account").(auth.Account), c.Param("id"), in.TemporaryPassword, requestMetadata(c, "")))
	}
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
	result, err := h.masterData.List(c, masterdata.Query{Kind: c.Param("kind"), Keyword: c.Query("keyword"), Enabled: enabled, Page: page, PageSize: pageSize})
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

func (h *handler) currentAccount(c *gin.Context) (auth.Account, bool) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
		return auth.Account{}, false
	}
	a, err := h.auth.AuthenticateAccessToken(c, strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return auth.Account{}, false
	}
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
	if errors.Is(err, auth.ErrForbidden) {
		writeError(c, http.StatusForbidden, "FORBIDDEN", "无权执行该操作")
		return
	}
	if errors.Is(err, auth.ErrInvalidTransition) {
		writeError(c, http.StatusConflict, "INVALID_STATE", "当前状态不允许该操作")
		return
	}
	writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "提交信息不符合要求")
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

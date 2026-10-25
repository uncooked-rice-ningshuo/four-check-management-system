package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type inspectionHTTPService interface {
	Create(context.Context, inspection.ManualActor, inspection.InspectionInput, audit.Metadata) (inspection.InspectionRecord, error)
	Detail(context.Context, inspection.ManualActor, string) (inspection.InspectionRecord, error)
	List(context.Context, inspection.ManualActor, inspection.InspectionFilter) (inspection.InspectionPage, error)
	Update(context.Context, inspection.ManualActor, string, inspection.InspectionEdit, audit.Metadata) (inspection.InspectionRecord, error)
	Delete(context.Context, inspection.ManualActor, inspection.DeleteAttendanceInput, audit.Metadata) error
}

func (h *handler) inspectionActor(c *gin.Context) (inspection.ManualActor, bool) {
	a, ok := h.currentAccount(c)
	if !ok {
		return inspection.ManualActor{}, false
	}
	if a.MustChangePassword {
		writeError(c, 403, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return inspection.ManualActor{}, false
	}
	if h.inspections == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "检查记录服务尚未就绪")
		return inspection.ManualActor{}, false
	}
	return inspection.ManualActor{ID: a.ID, IsSuperAdmin: a.IsSuperAdmin, IsMember: a.SystemRole == auth.SystemRoleMember, PrimaryDepartmentID: a.PrimaryDepartmentID}, true
}
func bindInspection(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeError(c, 400, "INVALID_REQUEST", "请求格式无效或字段不受支持")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		writeError(c, 400, "INVALID_REQUEST", "请求必须包含单个 JSON 对象")
		return false
	}
	return true
}
func (h *handler) createInspection(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	var i inspection.InspectionInput
	if !bindInspection(c, &i) {
		return
	}
	r, e := h.inspections.Create(c, a, i, requestMetadata(c, ""))
	if e != nil {
		writeInspectionError(c, e)
		return
	}
	c.JSON(201, r)
}
func (h *handler) getInspection(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	r, e := h.inspections.Detail(c, a, c.Param("id"))
	if e != nil {
		writeInspectionError(c, e)
		return
	}
	c.JSON(200, r)
}
func (h *handler) listInspections(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	var f inspection.InspectionFilter
	if c.ShouldBindQuery(&f) != nil {
		writeError(c, 400, "INVALID_REQUEST", "分页或筛选格式无效")
		return
	}
	r, e := h.inspections.List(c, a, f)
	if e != nil {
		writeInspectionError(c, e)
		return
	}
	c.JSON(200, r)
}

func (h *handler) listSubmissionCandidates(c *gin.Context) {
	if h.inspections == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "检查记录服务尚未就绪")
		return
	}
	account, _, ok := h.authorizeInspectionRequest(c, permission.ActionInspectionEnter)
	if !ok {
		return
	}
	if _, _, ok = h.authorizeInspectionRequest(c, permission.ActionInspectionView); !ok {
		return
	}
	if strings.TrimSpace(c.Query("departmentId")) == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请明确选择部门或校级范围")
		return
	}
	actor := inspection.ManualActor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin, IsMember: account.SystemRole == auth.SystemRoleMember, PrimaryDepartmentID: account.PrimaryDepartmentID}
	var filter inspection.InspectionFilter
	if c.ShouldBindQuery(&filter) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "分页或筛选格式无效")
		return
	}
	if account.SystemRole == auth.SystemRoleMember {
		filter.DepartmentID = account.PrimaryDepartmentID
	}
	filter.SubmissionCandidatesOnly = true
	filter.Status = inspection.RecordStatusDraft
	page, err := h.inspections.List(c, actor, filter)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}
func (h *handler) updateInspection(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	var i inspection.InspectionEdit
	if !bindInspection(c, &i) {
		return
	}
	r, e := h.inspections.Update(c, a, c.Param("id"), i, requestMetadata(c, ""))
	if e != nil {
		writeInspectionError(c, e)
		return
	}
	c.JSON(200, r)
}
func (h *handler) deleteInspection(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	var i struct {
		Version int    `json:"version"`
		Reason  string `json:"reason"`
	}
	if !bindInspection(c, &i) {
		return
	}
	e := h.inspections.Delete(c, a, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: c.Param("id"), Version: i.Version}}, Reason: i.Reason}, requestMetadata(c, ""))
	if e != nil {
		writeInspectionError(c, e)
		return
	}
	c.Status(204)
}
func (h *handler) batchInspections(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	var i struct {
		Action string                       `json:"action"`
		Items  []inspection.VersionedRecord `json:"items"`
		Reason string                       `json:"reason"`
	}
	if !bindInspection(c, &i) {
		return
	}
	if i.Action != "DELETE" {
		writeError(c, 400, "INVALID_REQUEST", "当前仅支持批量删除草稿")
		return
	}
	if e := h.inspections.Delete(c, a, inspection.DeleteAttendanceInput{Items: i.Items, Reason: i.Reason}, requestMetadata(c, "")); e != nil {
		writeInspectionError(c, e)
		return
	}
	c.Status(204)
}
func writeInspectionError(c *gin.Context, err error) {
	var validation *inspection.DetailsValidationError
	switch {
	case errors.As(err, &validation):
		c.JSON(422, gin.H{"code": "VALIDATION_FAILED", "message": "请核对检查明细", "details": validation.Issues})
	case errors.Is(err, inspection.ErrInspectionRequest):
		writeError(c, 400, "INVALID_REQUEST", "请核对记录标识、版本、操作原因及筛选参数")
	case errors.Is(err, inspection.ErrScopeMismatch):
		writeError(c, 403, "FORBIDDEN_SCOPE", "无权操作所选范围")
	case errors.Is(err, inspection.ErrRecordNotFound):
		writeError(c, 404, "INSPECTION_NOT_FOUND", "记录不存在或无权访问")
	case errors.Is(err, inspection.ErrRecordVersionConflict):
		writeError(c, 409, "VERSION_CONFLICT", "记录已变化，请刷新核对后重试")
	case errors.Is(err, inspection.ErrRecordNotEditable):
		writeError(c, 409, "RECORD_NOT_EDITABLE", "仅未提交且未锁定的草稿可以修改或删除")
	case errors.Is(err, inspection.ErrAttendanceDuplicate):
		writeError(c, 409, "RECORD_ALREADY_EXISTS", "相同检查记录已存在，请核对日期、时间或轮次")
	case errors.Is(err, inspection.ErrAttendancePenaltyUnconfigured):
		writeError(c, 409, "ATTENDANCE_WEEK_RULE_UNCONFIGURED", "该周到课率违纪计入规则尚未配置，请联系管理员先完成校历配置")
	case errors.Is(err, inspection.ErrAttendanceLegacyRuleUnknown):
		writeError(c, 409, "ATTENDANCE_LEGACY_RULE_UNKNOWN", "历史记录的违纪计入口径尚未确认，请先完成历史数据核对")
	case errors.Is(err, inspection.ErrInvalidAttendanceRecord):
		writeError(c, 422, "VALIDATION_FAILED", "基础数据、关联关系或配置已变化，请刷新后核对")
	default:
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			if pgError.Code == "42P01" {
				log.Printf("inspection failed: route=%s sqlState=%s missingRelation=%s", c.FullPath(), pgError.Code, pgError.Message)
			} else {
				log.Printf("inspection failed: route=%s sqlState=%s", c.FullPath(), pgError.Code)
			}
		} else {
			log.Printf("inspection failed: route=%s errorType=%T", c.FullPath(), err)
		}
		writeError(c, 500, "INSPECTION_FAILED", "操作失败，请稍后重试")
	}
}

type inspectionOptionHTTPService interface {
	EntryOptions(context.Context, inspection.ManualActor, inspection.EntryOptionQuery) (inspection.EntryOptionPage, error)
}

func (h *handler) inspectionEntryOptions(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	if h.inspectionOptions == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "录入选项服务尚未就绪")
		return
	}
	var q inspection.EntryOptionQuery
	if c.ShouldBindQuery(&q) != nil {
		writeError(c, 400, "INVALID_REQUEST", "分页或筛选格式无效")
		return
	}
	page, err := h.inspectionOptions.EntryOptions(c, a, q)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(200, page)
}

type inspectionContextOptionHTTPService interface {
	ContextOptions(context.Context, inspection.ManualActor, inspection.ContextOptionQuery) (inspection.ContextOptionPage, error)
}

func (h *handler) inspectionContextOptionList(c *gin.Context) {
	a, ok := h.inspectionActor(c)
	if !ok {
		return
	}
	if h.inspectionContextOptions == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "检查选项服务尚未就绪")
		return
	}
	var q inspection.ContextOptionQuery
	if c.ShouldBindQuery(&q) != nil {
		writeError(c, 400, "INVALID_REQUEST", "分页或筛选格式无效")
		return
	}
	page, err := h.inspectionContextOptions.ContextOptions(c, a, q)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(200, page)
}

package httpapi

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/inspection"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
)

type attendanceMutationHTTPService interface {
	Detail(context.Context, inspection.ManualActor, string) (inspection.AttendanceRecord, error)
	Update(context.Context, inspection.ManualActor, string, inspection.AttendanceEditInput, audit.Metadata) (inspection.AttendanceRecord, error)
	Delete(context.Context, inspection.ManualActor, inspection.DeleteAttendanceInput, audit.Metadata) error
}

func (h *handler) mutationActor(c *gin.Context) (auth.Account, bool) {
	a, ok := h.currentAccount(c)
	if !ok {
		return a, false
	}
	if a.MustChangePassword {
		writeError(c, 403, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return a, false
	}
	if h.attendanceMutations == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "记录管理服务尚未就绪")
		return a, false
	}
	return a, true
}
func validMutationID(c *gin.Context, id string) bool {
	if _, err := uuid.Parse(id); err != nil {
		writeError(c, 400, "INVALID_REQUEST", "记录标识无效")
		return false
	}
	return true
}
func (h *handler) attendanceDetail(c *gin.Context) {
	a, ok := h.mutationActor(c)
	if !ok || !validMutationID(c, c.Param("id")) {
		return
	}
	r, err := h.attendanceMutations.Detail(c, inspection.ManualActor{ID: a.ID, IsSuperAdmin: a.IsSuperAdmin}, c.Param("id"))
	if err != nil {
		writeAttendanceMutationError(c, err)
		return
	}
	c.JSON(200, r)
}
func (h *handler) updateAttendance(c *gin.Context) {
	a, ok := h.mutationActor(c)
	if !ok || !validMutationID(c, c.Param("id")) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24<<10)
	var input inspection.AttendanceEditInput
	if !bind(c, &input) {
		return
	}
	for _, id := range []string{input.Record.SemesterID, input.Record.WeekID, input.Record.DepartmentID, input.Record.ClassID, input.Record.InspectionTypeID} {
		if !validMutationID(c, id) {
			return
		}
	}
	r, err := h.attendanceMutations.Update(c, inspection.ManualActor{ID: a.ID, IsSuperAdmin: a.IsSuperAdmin}, c.Param("id"), input, requestMetadata(c, ""))
	if err != nil {
		writeAttendanceMutationError(c, err)
		return
	}
	c.JSON(200, r)
}
func (h *handler) deleteAttendance(c *gin.Context) {
	a, ok := h.mutationActor(c)
	if !ok || !validMutationID(c, c.Param("id")) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var input struct {
		Version int    `json:"version"`
		Reason  string `json:"reason"`
	}
	if !bind(c, &input) {
		return
	}
	err := h.attendanceMutations.Delete(c, inspection.ManualActor{ID: a.ID, IsSuperAdmin: a.IsSuperAdmin}, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: c.Param("id"), Version: input.Version}}, Reason: input.Reason}, requestMetadata(c, ""))
	if err != nil {
		writeAttendanceMutationError(c, err)
		return
	}
	c.Status(204)
}
func (h *handler) batchAttendance(c *gin.Context) {
	a, ok := h.mutationActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24<<10)
	var input struct {
		Action string                       `json:"action"`
		Items  []inspection.VersionedRecord `json:"items"`
		Reason string                       `json:"reason"`
	}
	if !bind(c, &input) {
		return
	}
	if input.Action != "DELETE" {
		writeError(c, 400, "INVALID_REQUEST", "当前仅支持批量删除草稿")
		return
	}
	for _, item := range input.Items {
		if !validMutationID(c, item.ID) {
			return
		}
	}
	if err := h.attendanceMutations.Delete(c, inspection.ManualActor{ID: a.ID, IsSuperAdmin: a.IsSuperAdmin}, inspection.DeleteAttendanceInput{Items: input.Items, Reason: input.Reason}, requestMetadata(c, "")); err != nil {
		writeAttendanceMutationError(c, err)
		return
	}
	c.Status(204)
}
func writeAttendanceMutationError(c *gin.Context, err error) {
	var v *inspection.ManualValidationError
	switch {
	case errors.Is(err, inspection.ErrRecordNotFound):
		writeError(c, 404, "INSPECTION_NOT_FOUND", "记录不存在或无权操作")
	case errors.Is(err, inspection.ErrScopeMismatch):
		writeError(c, 403, "FORBIDDEN_SCOPE", "无权操作所选记录范围")
	case errors.Is(err, inspection.ErrRecordVersionConflict):
		writeError(c, 409, "VERSION_CONFLICT", "记录已被修改，请刷新后重试")
	case errors.Is(err, inspection.ErrRecordNotEditable):
		writeError(c, 409, "RECORD_NOT_EDITABLE", "仅未提交且未锁定的草稿可编辑或删除")
	case errors.Is(err, inspection.ErrAttendanceDuplicate):
		writeError(c, 409, "RECORD_ALREADY_EXISTS", "存在相同记录，请核对后重试")
	case errors.Is(err, inspection.ErrInvalidAttendanceRecord):
		writeError(c, 422, "VALIDATION_FAILED", "请核对记录、版本号和操作原因")
	case errors.As(err, &v):
		writeError(c, 422, "VALIDATION_FAILED", v.Message)
	default:
		writeError(c, 500, "INSPECTION_MUTATION_FAILED", "操作失败，请稍后重试")
	}
}

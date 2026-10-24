package httpapi

import (
	"context"
	"errors"
	"net/http"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/inspection"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type manualAttendanceHTTPService interface {
	Create(context.Context, inspection.ManualActor, inspection.ManualAttendanceInput, audit.Metadata) (inspection.AttendanceRecord, error)
}

func (h *handler) createManualAttendance(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, 403, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.manualAttendance == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "手工录入服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input inspection.ManualAttendanceInput
	if !bind(c, &input) {
		return
	}
	actor := inspection.ManualActor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin, IsMember: account.SystemRole == auth.SystemRoleMember, PrimaryDepartmentID: account.PrimaryDepartmentID}
	departmentID, err := inspection.ResolveEntryDepartment(actor, input.DepartmentID)
	if err != nil {
		writeError(c, 403, "FORBIDDEN_SCOPE", "当前账号无有效录入部门或不能选择其他部门")
		return
	}
	input.DepartmentID = departmentID
	for _, id := range []string{input.SemesterID, input.WeekID, input.InspectionTypeID, input.DepartmentID, input.ClassID, input.CourseID} {
		if _, err := uuid.Parse(id); err != nil {
			writeError(c, 400, "INVALID_REQUEST", "请选择有效的学期、周次、部门和班级")
			return
		}
	}
	result, err := h.manualAttendance.Create(c, actor, input, requestMetadata(c, ""))
	if err != nil {
		var validation *inspection.ManualValidationError
		switch {
		case errors.Is(err, inspection.ErrScopeMismatch):
			writeError(c, 403, "FORBIDDEN_SCOPE", "无权在当前目标范围录入到课率")
		case errors.Is(err, inspection.ErrAttendanceDuplicate):
			writeError(c, 409, "RECORD_ALREADY_EXISTS", "相同到课率记录已存在，请刷新列表核对")
		case errors.Is(err, inspection.ErrAttendancePenaltyUnconfigured):
			writeError(c, 409, "ATTENDANCE_WEEK_RULE_UNCONFIGURED", "该周到课率违纪计入规则尚未配置，请联系管理员先完成校历配置")
		case errors.As(err, &validation):
			writeError(c, 422, "VALIDATION_FAILED", validation.Message)
		case errors.Is(err, inspection.ErrInvalidAttendanceRecord):
			writeError(c, 422, "VALIDATION_FAILED", "基础数据或周次已变化，请刷新并核对录入信息")
		default:
			writeError(c, 500, "ATTENDANCE_CREATE_FAILED", "保存失败，请稍后重试")
		}
		return
	}
	c.JSON(http.StatusCreated, result)
}

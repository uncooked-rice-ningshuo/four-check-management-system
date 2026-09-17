package httpapi

import (
	"errors"
	"four-check-management-system/server/internal/academic"
	"github.com/gin-gonic/gin"
	"time"
)

func (h *handler) getWorkContext(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if a.MustChangePassword {
		writeError(c, 403, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.workContext == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "工作上下文服务尚未就绪")
		return
	}
	r, e := h.workContext.Get(c, time.Now())
	if e != nil {
		writeError(c, 500, "WORK_CONTEXT_FAILED", "读取工作上下文失败")
		return
	}
	c.JSON(200, r)
}
func (h *handler) setWorkContext(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if a.MustChangePassword || !a.IsSuperAdmin {
		writeError(c, 403, "FORBIDDEN", "无权修改全局工作周次")
		return
	}
	if h.workContext == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "工作上下文服务尚未就绪")
		return
	}
	var i academic.WeekOverrideInput
	if !bindInspection(c, &i) {
		return
	}
	m := requestMetadata(c, "")
	m.ActorID = a.ID
	r, e := h.workContext.Set(c, true, i, time.Now(), m)
	if e != nil {
		switch {
		case errors.Is(e, academic.ErrWorkContextConflict):
			writeError(c, 409, "VERSION_CONFLICT", "工作周次已被修改，请刷新后核对")
		case errors.Is(e, academic.ErrInvalidInput):
			writeError(c, 422, "VALIDATION_FAILED", "请核对启用周次、到期时间、版本及原因")
		default:
			writeError(c, 500, "WORK_CONTEXT_FAILED", "修改失败，请稍后重试")
		}
		return
	}
	c.JSON(200, r)
}

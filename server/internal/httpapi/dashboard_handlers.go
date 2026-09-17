package httpapi

import (
	"net/http"
	"time"

	"four-check-management-system/server/internal/dashboard"
	"github.com/gin-gonic/gin"
)

func (h *handler) workbench(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.dashboard == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "工作台服务尚未就绪")
		return
	}
	result, err := h.dashboard.Workbench(c, dashboard.Actor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}, time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DASHBOARD_LOAD_FAILED", "工作台数据加载失败")
		return
	}
	c.JSON(http.StatusOK, result)
}

package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *handler) getReportGrades(c *gin.Context) {
	if h.reportGrades == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "学年年级映射服务尚未就绪")
		return
	}
	result, err := h.reportGrades.Get(c, c.Param("id"))
	writeResult(c, result, err)
}

func (h *handler) previewReportGrades(c *gin.Context) {
	if h.reportGrades == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "学年年级映射服务尚未就绪")
		return
	}
	result, err := h.reportGrades.Preview(c, c.QueryArray("gradeId"))
	writeResult(c, result, err)
}

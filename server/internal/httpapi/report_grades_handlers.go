package httpapi

import (
	"net/http"

	"four-check-management-system/server/internal/academic"
	"github.com/gin-gonic/gin"
)

func (h *handler) getReportGrades(c *gin.Context) {
	if h.reportGrades == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "学期年级配置服务尚未就绪")
		return
	}
	result, err := h.reportGrades.Get(c, c.Param("id"))
	writeResult(c, result, err)
}

func (h *handler) previewReportGrades(c *gin.Context) {
	if h.reportGrades == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "学期年级配置服务尚未就绪")
		return
	}
	if _, err := h.reportGrades.Get(c, c.Param("id")); err != nil {
		writeResult(c, academic.ReportGradePreview{}, err)
		return
	}
	result, err := h.reportGrades.Preview(c, c.QueryArray("gradeId"))
	writeResult(c, result, err)
}

func (h *handler) replaceReportGrades(c *gin.Context) {
	if h.reportGrades == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "学期年级配置服务尚未就绪")
		return
	}
	var input academic.ReportGradeScope
	if !bind(c, &input) {
		return
	}
	input.SemesterID = c.Param("id")
	result, err := h.reportGrades.Replace(c, input, requestMetadata(c, ""))
	writeResult(c, result, err)
}

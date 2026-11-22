package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ledgerFilters(c *gin.Context) (reporting.LedgerFilter, bool) {
	var filter reporting.LedgerFilter

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&filter); err != nil || len(filter.Search) > 200 || len(filter.Class) > 400 || len(filter.Grade) > 200 || len(filter.Major) > 400 {
		writeError(c, 400, "INVALID_REQUEST", "筛选条件无效")
		return filter, false
	}
	return filter, true
}
func (h *handler) semesterLedger(c *gin.Context) {
	filter, valid := ledgerFilters(c)
	if !valid {
		return
	}
	semester := c.Query("semesterId")
	page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, f := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	_, idErr := uuid.Parse(semester)
	if (semester != "" && idErr != nil) || e != nil || f != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	service, ok := h.reporting.(reporting.LedgerReader)
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "汇总表尚未就绪")
		return
	}
	result, err := service.SemesterLedger(c, semester, filter, page, size)
	if err != nil {
		writeError(c, 500, "LEDGER_QUERY_FAILED", "读取学期违纪台账失败")
		return
	}
	c.Data(200, "application/json", result)
}

func (h *handler) exportSemesterLedger(c *gin.Context) {
	filter, valid := ledgerFilters(c)
	if !valid {
		return
	}
	semester := c.Query("semesterId")
	if _, err := uuid.Parse(semester); err != nil {
		writeError(c, 400, "INVALID_REQUEST", "请选择学期并检查查询条件")
		return
	}
	service, ok := h.reporting.(interface {
		ExportSemesterLedger(context.Context, string, reporting.LedgerFilter, audit.Metadata) ([]byte, error)
	})
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "台账导出尚未就绪")
		return
	}
	content, err := service.ExportSemesterLedger(c, semester, filter, requestMetadata(c, "导出学期违纪台账"))
	if err != nil {
		writeError(c, 500, "LEDGER_EXPORT_FAILED", "导出未完成，请重试")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=semester-ledger.xlsx")
	c.Header("Cache-Control", "no-store")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *handler) semesterLedgerDetail(c *gin.Context) {
	var f reporting.LedgerDetailFilter
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&f); err != nil || !isValidLedgerDetail(f) {
		writeError(c, 400, "INVALID_REQUEST", "明细查询条件无效")
		return
	}
	reader, ok := h.reporting.(reporting.LedgerDetailReader)
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "台账明细尚未就绪")
		return
	}
	result, err := reader.SemesterLedgerDetail(c, f)
	if err != nil {
		writeError(c, 500, "LEDGER_QUERY_FAILED", "台账明细读取失败")
		return
	}
	c.Data(200, "application/json", result)
}
func isValidLedgerDetail(f reporting.LedgerDetailFilter) bool {
	_, err := uuid.Parse(f.Semester)
	return err == nil && f.Student != "" && (f.Kind == "sources" || f.Kind == "cases") && f.Page >= 1 && f.Page <= 1000000 && f.Size >= 1 && f.Size <= 100 && f.Week >= 0
}

func (h *handler) reviewDisciplineCase(c *gin.Context) {
	actor, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.DisciplineReview
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if _, err := uuid.Parse(c.Param("id")); err != nil || c.ShouldBindJSON(&input) != nil {
		writeError(c, 400, "INVALID_REQUEST", "审批参数无效")
		return
	}
	svc, ok := h.reporting.(interface {
		ReviewDisciplineCase(context.Context, string, string, reporting.DisciplineReview, audit.Metadata) error
	})
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "审批服务尚未就绪")
		return
	}
	if err := svc.ReviewDisciplineCase(c, c.Param("id"), actor.ID, input, requestMetadata(c, "")); err != nil {
		h.writeReportError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) disciplineReviewQueue(c *gin.Context) {
	actor, ok := h.reportActor(c)
	if !ok {
		return
	}
	var f reporting.ReviewQueueFilter
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&f) != nil {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	svc, ok := h.reporting.(interface {
		DisciplineReviewQueue(context.Context, string, reporting.ReviewQueueFilter) (json.RawMessage, error)
	})
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "审批服务尚未就绪")
		return
	}
	result, err := svc.DisciplineReviewQueue(c, actor.ID, f)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.Data(200, "application/json", result)
}

func (h *handler) disciplineArchive(c *gin.Context) {
	actor, ok := h.reportActor(c)
	if !ok {
		return
	}
	var f reporting.ArchiveFilter
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&f) != nil {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	svc, ok := h.reporting.(interface {
		DisciplineArchive(context.Context, string, reporting.ArchiveFilter) (json.RawMessage, error)
	})
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "归档服务尚未就绪")
		return
	}
	result, err := svc.DisciplineArchive(c, actor.ID, f)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.Data(200, "application/json", result)
}

func (h *handler) exportDisciplineArchive(c *gin.Context) {
	actor, ok := h.reportActor(c)
	if !ok {
		return
	}
	var f reporting.ArchiveFilter
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&f) != nil {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	svc, ok := h.reporting.(interface {
		ExportDisciplineArchive(context.Context, string, reporting.ArchiveFilter, audit.Metadata) ([]byte, error)
	})
	if !ok {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "归档服务尚未就绪")
		return
	}
	result, err := svc.ExportDisciplineArchive(c, actor.ID, f, requestMetadata(c, "导出处分归档"))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=discipline-archive.xlsx")
	c.Header("Cache-Control", "no-store")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", result)
}

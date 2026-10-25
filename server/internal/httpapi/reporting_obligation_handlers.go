package httpapi

import (
	"errors"
	"log"
	"net/http"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/reporting"
	"github.com/gin-gonic/gin"
)

func (h *handler) reportObligationOptions(c *gin.Context) {
	account, ok := h.reportObligationActor(c)
	if !ok {
		return
	}
	var query reporting.ObligationPeriodQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OBLIGATION", "请选择有效的学期和周次")
		return
	}
	options, err := h.reportingObligationService.ReportObligationOptions(c, account.ID, query)
	if err != nil {
		writeReportObligationError(c, err)
		return
	}
	c.JSON(http.StatusOK, options)
}

func (h *handler) reportObligations(c *gin.Context) {
	account, ok := h.reportObligationActor(c)
	if !ok {
		return
	}
	var query reporting.ObligationPeriodQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OBLIGATION", "请选择有效的学期和周次")
		return
	}
	items, err := h.reportingObligationService.ReportObligations(c, account.ID, query)
	if err != nil {
		writeReportObligationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *handler) createReportObligation(c *gin.Context) {
	account, ok := h.reportObligationActor(c)
	if !ok {
		return
	}
	var input reporting.CreateReportObligationInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OBLIGATION", "请检查应交范围信息")
		return
	}
	item, err := h.reportingObligationService.CreateReportObligation(c, account.ID, input, requestMetadata(c, ""))
	if err != nil {
		writeReportObligationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *handler) removeReportObligation(c *gin.Context) {
	account, ok := h.reportObligationActor(c)
	if !ok {
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OBLIGATION", "请刷新应交范围后重试")
		return
	}
	if err := h.reportingObligationService.RemoveReportObligation(c, account.ID, c.Param("id"), input.Version, requestMetadata(c, "")); err != nil {
		writeReportObligationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) reportObligationActor(c *gin.Context) (account auth.Account, ok bool) {
	account, ok = h.reportActor(c)
	if !ok {
		return account, false
	}
	if h.reportingObligationService == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "逐周应交范围服务尚未就绪")
		return account, false
	}
	return account, true
}

func writeReportObligationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reporting.ErrInvalidObligation):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OBLIGATION", "请检查学期、周次、部门和检查类型")
	case errors.Is(err, reporting.ErrForbidden):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权管理该周的应交范围")
	case errors.Is(err, reporting.ErrObligationPeriodUnavailable):
		writeError(c, http.StatusNotFound, "REPORT_PERIOD_NOT_FOUND", "学期或周次不存在、已停用或不匹配")
	case errors.Is(err, reporting.ErrObligationScopeUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "REPORT_OBLIGATION_SCOPE_INVALID", "该部门与检查类型当前不具备应交配置条件")
	case errors.Is(err, reporting.ErrObligationAlreadyExists):
		writeError(c, http.StatusConflict, "REPORT_OBLIGATION_EXISTS", "该周的应交范围已配置")
	case errors.Is(err, reporting.ErrObligationNotFound):
		writeError(c, http.StatusNotFound, "REPORT_OBLIGATION_NOT_FOUND", "应交范围不存在或已移除")
	case errors.Is(err, reporting.ErrObligationVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "应交范围已变化，请刷新后重试")
	default:
		log.Printf("weekly report obligation operation failed: errorType=%T", err)
		writeError(c, http.StatusInternalServerError, "REPORT_OBLIGATION_FAILED", "应交范围暂时不可用，请稍后重试")
	}
}

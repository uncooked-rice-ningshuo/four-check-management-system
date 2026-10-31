package httpapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/reporting"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

func (h *handler) reportPeriods(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	periods, err := h.reporting.Periods(c, account.ID)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": periods})
}

func (h *handler) reportSummary(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	result, err := h.reporting.Summary(c, account.ID, c.Query("semesterId"), c.Query("weekId"))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportWorkflow(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	result, err := h.reporting.Workflow(c, account.ID, c.Query("semesterId"), c.Query("weekId"))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportVersionRecords(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	versionNo, err := strconv.Atoi(c.Param("versionNo"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_RECORD_QUERY", "记录范围或分页参数无效")
		return
	}
	var query reporting.ReportVersionRecordsQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_RECORD_QUERY", "记录范围或分页参数无效")
		return
	}
	result, err := h.reporting.ReportVersionRecords(c, account.ID, c.Param("semesterId"), c.Param("weekId"), versionNo, query)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportArchive(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var query reporting.ReportArchiveQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_ARCHIVE_QUERY", "归档查询参数无效")
		return
	}
	result, err := h.reporting.Archive(c, account.ID, query)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportArchiveWorkflow(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	result, err := h.reporting.ArchiveWorkflow(c, account.ID, c.Param("semesterId"), c.Param("weekId"))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportRecords(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var query reporting.RecordsQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_RECORD_QUERY", "记录范围或分页参数无效")
		return
	}
	result, err := h.reporting.Records(c, account.ID, query)
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) generateReportDraft(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.GenerateDraftInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_REQUEST", "通报生成参数无效")
		return
	}
	result, err := h.reporting.GenerateDraft(c, account.ID, input, requestMetadata(c, input.Reason))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *handler) submitReportDraft(c *gin.Context) {
	h.transitionReport(c, reporting.ReportActionSubmit)
}
func (h *handler) returnReportDraft(c *gin.Context) {
	h.transitionReport(c, reporting.ReportActionReturn)
}
func (h *handler) approveReportDraft(c *gin.Context) {
	h.transitionReport(c, reporting.ReportActionApprove)
}
func (h *handler) finalizeReportDraft(c *gin.Context) {
	h.transitionReport(c, reporting.ReportActionFinalized)
}

func (h *handler) transitionReport(c *gin.Context, action string) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.ReportTransitionInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_REQUEST", "通报操作参数无效")
		return
	}
	result, err := h.reporting.TransitionReport(c, account.ID, c.Param("id"), action, input, requestMetadata(c, input.Comment))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) addReportCorrection(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.AddReportCorrectionInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_CORRECTION", "通报修正参数无效")
		return
	}
	result, err := h.reporting.AddReportCorrection(c, account.ID, c.Param("id"), input, requestMetadata(c, input.Reason))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *handler) addReportAdjustment(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.AddReportAdjustmentInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_ADJUSTMENT", "通报数据调整参数无效")
		return
	}
	result, err := h.reporting.AddReportAdjustment(c, account.ID, c.Param("id"), input, requestMetadata(c, input.Reason))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *handler) decideReportCorrection(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	var input reporting.DecideReportCorrectionInput
	if c.ShouldBindJSON(&input) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_CORRECTION", "部门确认参数无效")
		return
	}
	result, err := h.reporting.DecideReportCorrection(c, account.ID, c.Param("id"), input, requestMetadata(c, input.Comment))
	if err != nil {
		h.writeReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) reportActor(c *gin.Context) (account auth.Account, ok bool) {
	account, ok = h.currentAccount(c)
	if !ok {
		return account, false
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return account, false
	}
	if h.reporting == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报汇总服务尚未就绪")
		return account, false
	}
	return account, true
}

func (h *handler) writeReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reporting.ErrInvalidReportRequest):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_REQUEST", "通报操作参数无效")
	case errors.Is(err, reporting.ErrInvalidReportArchiveQuery):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_ARCHIVE_QUERY", "归档查询参数无效")
	case errors.Is(err, reporting.ErrReportIncomplete):
		var incomplete *reporting.IncompleteReportError
		if errors.As(err, &incomplete) {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "REPORT_INCOMPLETE", "message": "存在未完成的应交单元，当前不能生成通报", "details": gin.H{"incompleteCells": incomplete.Cells}}})
			return
		}
		writeError(c, http.StatusConflict, "REPORT_INCOMPLETE", "存在未完成的应交单元，当前不能生成通报")
	case errors.Is(err, reporting.ErrReportVersionConflict):
		writeError(c, http.StatusConflict, "REPORT_VERSION_CONFLICT", "通报已被其他操作更新，请刷新后重试")
	case errors.Is(err, reporting.ErrInvalidReportTransition):
		writeError(c, http.StatusConflict, "INVALID_REPORT_TRANSITION", "当前通报状态不允许此操作")
	case errors.Is(err, reporting.ErrReportNotFound):
		writeError(c, http.StatusNotFound, "REPORT_NOT_FOUND", "未找到通报")
	case errors.Is(err, reporting.ErrInvalidReportCorrection):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_CORRECTION", "通报修正参数无效")
	case errors.Is(err, reporting.ErrInvalidReportAdjustment):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_ADJUSTMENT", "通报数据调整参数无效")
	case errors.Is(err, reporting.ErrReportCorrectionNotFound):
		writeError(c, http.StatusNotFound, "REPORT_CORRECTION_NOT_FOUND", "未找到待确认的通报修正")
	case errors.Is(err, reporting.ErrReportCorrectionDecided):
		writeError(c, http.StatusConflict, "REPORT_CORRECTION_ALREADY_DECIDED", "该通报修正已处理")
	case errors.Is(err, reporting.ErrReportReconfirmationPending):
		writeError(c, http.StatusConflict, "REPORT_RECONFIRMATION_PENDING", "存在待部门重新确认的事实修正，当前不能提交审核")
	case errors.Is(err, reporting.ErrReportSourceChanged):
		writeError(c, http.StatusConflict, "REPORT_SOURCE_CHANGED", "确认数据在通报生成后发生变化，请重新生成新版本")
	case errors.Is(err, reporting.ErrInvalidPeriod):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_PERIOD", "请选择有效的学期和周次")
	case errors.Is(err, reporting.ErrInvalidRecordsQuery):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_RECORD_QUERY", "记录范围或分页参数无效")
	case errors.Is(err, reporting.ErrForbidden):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权查看该通报范围")
	default:
		log.Printf("report request failed: route=%s %s", c.FullPath(), reportErrorLogFields(err))
		writeError(c, http.StatusInternalServerError, "REPORT_SUMMARY_FAILED", "通报汇总暂时不可用，请稍后重试")
	}
}

// reportErrorLogFields keeps PostgreSQL classification while omitting Message
// and Detail, which can echo submitted values into server logs.
func reportErrorLogFields(err error) string {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		return fmt.Sprintf("errorType=%T sqlState=%s schema=%q table=%q column=%q constraint=%q",
			databaseError, databaseError.Code, databaseError.SchemaName, databaseError.TableName,
			databaseError.ColumnName, databaseError.ConstraintName)
	}
	return fmt.Sprintf("errorType=%T", err)
}

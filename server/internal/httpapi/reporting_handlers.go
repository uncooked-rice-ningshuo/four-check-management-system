package httpapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"

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

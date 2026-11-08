package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/reportoutput"
	"four-check-management-system/server/internal/storage"
	"github.com/gin-gonic/gin"
)

func (h *handler) getReportOutputEditorSettings(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	settings, types, err := h.reportOutput.EditorSettings(c)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"introduction": settings.Introduction, "introductionDocument": settings.IntroductionDocument, "wordTypeCodes": settings.WordTypeCodes, "excelTypeCodes": settings.ExcelTypeCodes, "version": settings.Version, "inspectionTypes": types})
}

func (h *handler) previewReportOutputWord(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var input struct {
		IntroductionDocument json.RawMessage `json:"introductionDocument"`
		TypeCodes            []string        `json:"typeCodes"`
	}
	if !bind(c, &input) {
		return
	}
	data, err := h.reportOutput.PreviewWord(c, input.IntroductionDocument, input.TypeCodes, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=report-preview.docx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
}

func (h *handler) previewReportOutputExcel(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var input struct {
		TypeCodes []string `json:"typeCodes"`
	}
	if !bind(c, &input) {
		return
	}
	data, err := h.reportOutput.PreviewExcel(c, input.TypeCodes, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=report-preview.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h *handler) outputVersion(c *gin.Context) (int, bool) {
	version, err := strconv.Atoi(c.Param("versionNo"))
	if err != nil || version < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OUTPUT", "通报版本无效")
		return 0, false
	}
	return version, true
}
func (h *handler) getReportOutputSettings(c *gin.Context) {
	if _, ok := h.reportActor(c); !ok {
		return
	}
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	result, err := h.reportOutput.Settings(c)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *handler) updateReportOutputSettings(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var input reportoutput.Settings
	if !bind(c, &input) {
		return
	}
	result, err := h.reportOutput.ReplaceSettings(c, input, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *handler) generateReportOutput(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	version, ok := h.outputVersion(c)
	if !ok {
		return
	}
	var input reportoutput.GenerateInput
	if !bind(c, &input) {
		return
	}
	input.SemesterID, input.WeekID, input.VersionNo = c.Param("semesterId"), c.Param("weekId"), version
	result, err := h.reportOutput.Generate(c, account.ID, input, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}
func (h *handler) listReportOutputs(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	version, ok := h.outputVersion(c)
	if !ok {
		return
	}
	result, err := h.reportOutput.List(c, account.ID, c.Param("semesterId"), c.Param("weekId"), version)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}
func (h *handler) downloadReportOutput(c *gin.Context) {
	account, ok := h.reportActor(c)
	if !ok {
		return
	}
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	version, ok := h.outputVersion(c)
	if !ok {
		return
	}
	output, data, err := h.reportOutput.Download(c, account.ID, c.Param("semesterId"), c.Param("weekId"), version, c.Param("outputId"), requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	mime, ext := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"
	if output.Format == "WORD" {
		mime, ext = "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=report-v%d-%s.%s", version, output.ID, ext))
	c.Data(http.StatusOK, mime, data)
}
func (h *handler) writeOutputError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reportoutput.ErrInvalidSelection):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OUTPUT", "文件格式或检查类型选择无效")
	case errors.Is(err, reportoutput.ErrConfigConflict):
		writeError(c, http.StatusConflict, "REPORT_OUTPUT_CONFIG_CONFLICT", "输出设置已更新，请刷新后重试")
	case errors.Is(err, reportoutput.ErrPreviewRateLimited):
		writeError(c, http.StatusTooManyRequests, "REPORT_OUTPUT_PREVIEW_LIMIT", "预览过于频繁，请稍后再试")
	case errors.Is(err, reportoutput.ErrNotFound), errors.Is(err, reporting.ErrReportNotFound):
		writeError(c, http.StatusNotFound, "REPORT_OUTPUT_NOT_FOUND", "未找到对应通报文件")
	case errors.Is(err, reportoutput.ErrUnavailable), errors.Is(err, storage.ErrFileUnavailable), errors.Is(err, storage.ErrObjectMissing):
		writeError(c, http.StatusConflict, "REPORT_OUTPUT_UNAVAILABLE", "文件尚未生成或已不可用，请重试生成")
	case errors.Is(err, reporting.ErrForbidden):
		writeError(c, http.StatusForbidden, "FORBIDDEN", "无权生成或下载该通报文件")
	case errors.Is(err, reporting.ErrInvalidPeriod), errors.Is(err, reporting.ErrReportIncomplete),
		errors.Is(err, reporting.ErrReportVersionConflict), errors.Is(err, reporting.ErrInvalidReportTransition),
		errors.Is(err, reporting.ErrReportSourceChanged), errors.Is(err, reporting.ErrReportGradesUnconfigured),
		errors.Is(err, reporting.ErrReportMetricScopeInvalid), errors.Is(err, reporting.ErrInvalidReportRequest):
		h.writeReportError(c, err)
	default:
		log.Printf("report output request failed: route=%s %s", c.FullPath(), reportErrorLogFields(err))
		writeError(c, http.StatusInternalServerError, "REPORT_OUTPUT_FAILED", "通报文件设置或生成暂时不可用，请稍后重试")
	}
}

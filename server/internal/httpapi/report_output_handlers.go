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
func (h *handler) listReportOutputWeekDates(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	items, err := h.reportOutput.WeekDates(c)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h *handler) saveReportOutputWeekDate(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "通报输出服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input reportoutput.WeekDate
	if !bind(c, &input) {
		return
	}
	input.SemesterID, input.WeekID = c.Param("semesterId"), c.Param("weekId")
	result, err := h.reportOutput.SaveWeekDate(c, input, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
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
		SemesterID           string          `json:"semesterId"`
		WeekID               string          `json:"weekId"`
	}
	if !bind(c, &input) {
		return
	}
	data, err := h.reportOutput.PreviewWord(c, input.IntroductionDocument, input.TypeCodes, input.SemesterID, input.WeekID, requestMetadata(c, ""))
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
	result, data, err := h.reportOutput.GenerateDownload(c, account.ID, input, requestMetadata(c, ""))
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	mime, ext := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"
	if result.Format == "WORD" {
		mime, ext = "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=report-v%d.%s", version, ext))
	c.Data(http.StatusOK, mime, data)
}
func (h *handler) writeOutputError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reportoutput.ErrInvalidSelection):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OUTPUT", "文件格式或检查类型选择无效")
	case errors.Is(err, reportoutput.ErrInvalidWeekDate):
		writeError(c, http.StatusBadRequest, "INVALID_REPORT_OUTPUT_DATE", "文件日期无效，请填写有效日期")
	case errors.Is(err, reportoutput.ErrWeekNotFound):
		writeError(c, http.StatusNotFound, "REPORT_OUTPUT_WEEK_NOT_FOUND", "所选通报周次不存在")
	case errors.Is(err, reportoutput.ErrRetiredClauseField):
		writeError(c, http.StatusUnprocessableEntity, "DISCIPLINE_TEMPLATE_FIELD_RETIRED", "旧依据字段已停用，请改用文书依据和依据条款")
	case errors.Is(err, reportoutput.ErrDisciplineBasisRetired):
		writeError(c, http.StatusConflict, "DISCIPLINE_BASIS_RETIRED", "此处分所用规则已失效，无法再次生成文书")
	case errors.Is(err, reportoutput.ErrTemplateUnchanged):
		writeError(c, http.StatusUnprocessableEntity, "TEMPLATE_EDIT_REQUIRED", "请先修改来源模板的正文、字段或版式，再启用")
	case errors.Is(err, reportoutput.ErrConfigConflict):
		writeError(c, http.StatusConflict, "REPORT_OUTPUT_CONFIG_CONFLICT", "输出设置已更新，请刷新后重试")
	case errors.Is(err, reportoutput.ErrIssueDateMissing):
		writeError(c, http.StatusConflict, "REPORT_OUTPUT_DATE_REQUIRED", "本周文件日期尚未配置，请在通报文件设置中保存日期后重试")
	case errors.Is(err, reportoutput.ErrPreviewRateLimited):
		writeError(c, http.StatusTooManyRequests, "REPORT_OUTPUT_PREVIEW_LIMIT", "预览过于频繁，请稍后再试")
	case errors.Is(err, reportoutput.ErrNotFound), errors.Is(err, reporting.ErrReportNotFound):
		writeError(c, http.StatusNotFound, "REPORT_OUTPUT_NOT_FOUND", "未找到对应通报文件")
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

func (h *handler) documentTemplates(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "文书服务尚未就绪")
		return
	}
	items, err := h.reportOutput.DocumentTemplates(c)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	sources := make([]reportoutput.DisciplineTemplate, 0, len(items))
	for _, item := range items {
		if item.TemplateID != "" {
			sources = append(sources, item)
		}
	}
	c.JSON(200, gin.H{"items": items, "sources": sources})
}
func (h *handler) saveDocumentTemplate(c *gin.Context) {
	if h.reportOutput == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "文书服务尚未就绪")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 70<<10)
	var input reportoutput.DisciplineTemplate
	if !bind(c, &input) {
		return
	}
	if err := h.reportOutput.SaveDocumentTemplate(c, input, requestMetadata(c, "")); err != nil {
		h.writeOutputError(c, err)
		return
	}
	c.JSON(200, gin.H{"saved": true})
}
func (h *handler) downloadDisciplineDocument(c *gin.Context) { h.processDisciplineDocument(c, true) }
func (h *handler) previewDisciplineDocument(c *gin.Context)  { h.processDisciplineDocument(c, false) }
func (h *handler) processDisciplineDocument(c *gin.Context, export bool) {
	if h.reportOutput == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "文书服务尚未就绪")
		return
	}
	var input reportoutput.DocumentRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.Request.ContentLength != 0 && !bind(c, &input) {
		return
	}
	preview, data, err := h.reportOutput.DisciplineDocument(c, c.Param("id"), input, requestMetadata(c, ""), export)
	if err != nil {
		h.writeOutputError(c, err)
		return
	}
	if !export {
		c.JSON(200, preview)
		return
	}
	if input.Variant == "both" {
		c.Header("Content-Disposition", "attachment; filename=discipline.zip")
		c.Data(200, "application/zip", data)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=discipline.docx")
	c.Data(200, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
}

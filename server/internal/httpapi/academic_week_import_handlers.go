package httpapi

import (
	"bytes"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/academicimport"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/storage"
	"github.com/gin-gonic/gin"
)

func (h *handler) getAcademicWeekImportSpec(c *gin.Context) {
	c.JSON(http.StatusOK, academicimport.GetSpec())
}

func (h *handler) downloadAcademicWeekImportTemplate(c *gin.Context) {
	content, err := academicimport.TemplateXLSX()
	if err != nil {
		log.Printf("academic week template generation failed: errorType=%T", err)
		writeError(c, http.StatusInternalServerError, "ACADEMIC_WEEK_TEMPLATE_UNAVAILABLE", "周次导入模板暂时不可用")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "周次导入模板.xlsx"}))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *handler) previewAcademicWeekImport(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.academicWeekImports == nil || h.importFiles == nil || h.importMaxUploadBytes <= 0 {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "周次导入服务尚未就绪")
		return
	}
	semesterID := c.PostForm("semesterId")
	if semesterID == "" {
		writeError(c, http.StatusBadRequest, "INVALID_IMPORT_REQUEST", "请选择目标学期")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, multipartRequestLimit(h.importMaxUploadBytes))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if maxBytesError(err) {
			writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		} else {
			writeError(c, http.StatusBadRequest, "INVALID_FILE", "请选择有效的 XLSX 文件")
		}
		return
	}
	if fileHeader.Size <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "上传文件不能为空")
		return
	}
	if fileHeader.Size > h.importMaxUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		return
	}
	opened, err := fileHeader.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "无法读取上传文件")
		return
	}
	content, readErr := io.ReadAll(io.LimitReader(opened, h.importMaxUploadBytes+1))
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil {
		if maxBytesError(readErr) {
			writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		} else {
			writeError(c, http.StatusBadRequest, "INVALID_FILE", "无法读取上传文件")
		}
		return
	}
	if int64(len(content)) == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_FILE", "上传文件不能为空")
		return
	}
	if int64(len(content)) > h.importMaxUploadBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "上传文件超过大小限制")
		return
	}
	stored, err := h.importFiles.Store(c, storage.StoreInput{
		OriginalName: fileHeader.Filename, DeclaredContentType: fileHeader.Header.Get("Content-Type"), Size: int64(len(content)),
		RetentionType: storage.RetentionImportSource, CreatedBy: account.ID, Body: bytes.NewReader(content),
	}, requestMetadata(c, ""))
	if err != nil {
		writeImportError(c, err)
		return
	}
	batch, err := h.academicWeekImports.Preview(c, semesterID, stored.ID, fileHeader.Filename, account.ID, content, requestMetadata(c, ""))
	if err != nil {
		h.discardUnboundImportSource(c, stored.ID)
		writeAcademicWeekImportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, batch)
}

func (h *handler) getAcademicWeekImportBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.academicWeekImports == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "周次导入服务尚未就绪")
		return
	}
	batch, err := h.academicWeekImports.GetBatch(c, c.Param("batchID"), account.ID)
	if err != nil {
		writeAcademicWeekImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, batch)
}

func (h *handler) confirmAcademicWeekImportBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if h.academicWeekImports == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "周次导入服务尚未就绪")
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if !bind(c, &input) {
		return
	}
	result, err := h.academicWeekImports.Confirm(c, academicimport.ConfirmInput{
		BatchID: c.Param("batchID"), ActorID: account.ID, ExpectedVersion: input.Version,
		IdempotencyKey: c.GetHeader("Idempotency-Key"), Metadata: requestMetadata(c, ""),
	})
	if err != nil {
		writeAcademicWeekImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func writeAcademicWeekImportError(c *gin.Context, err error) {
	var parseError *importjob.ParseError
	if errors.As(err, &parseError) {
		writeError(c, http.StatusUnprocessableEntity, string(parseError.Code), parseError.Message)
		return
	}
	switch {
	case errors.Is(err, academicimport.ErrBatchNotFound):
		writeError(c, http.StatusNotFound, "ACADEMIC_WEEK_IMPORT_NOT_FOUND", "导入预览不存在或无权访问")
	case errors.Is(err, academicimport.ErrSemesterNotFound):
		writeError(c, http.StatusNotFound, "ACADEMIC_SEMESTER_NOT_FOUND", "目标学期不存在")
	case errors.Is(err, academicimport.ErrBatchVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "导入预览已变化，请重新上传")
	case errors.Is(err, academicimport.ErrBatchHasErrors):
		writeError(c, http.StatusUnprocessableEntity, "IMPORT_HAS_ERRORS", "预览中仍有错误，请修正 Excel 后重新上传")
	case errors.Is(err, academicimport.ErrAlreadyConfirmed):
		writeError(c, http.StatusConflict, "IMPORT_ALREADY_CONFIRMED", "该批次已使用其他请求确认")
	case errors.Is(err, academicimport.ErrPreviewStale), errors.Is(err, academic.ErrDateOverlap):
		writeError(c, http.StatusConflict, "IMPORT_PREVIEW_STALE", "预览后校历已变化或日期发生冲突，请重新上传并校验")
	case errors.Is(err, academicimport.ErrInvalidIdempotencyKey), errors.Is(err, academicimport.ErrInvalidBatch):
		writeError(c, http.StatusBadRequest, "INVALID_IMPORT_REQUEST", "导入请求参数无效")
	case errors.Is(err, academicimport.ErrEmptyWorkbook):
		writeError(c, http.StatusUnprocessableEntity, "EMPTY_IMPORT", "文件中没有可导入的数据行")
	default:
		log.Printf("academic week import failed: method=%s route=%s errorType=%T", c.Request.Method, c.FullPath(), err)
		writeError(c, http.StatusInternalServerError, "ACADEMIC_WEEK_IMPORT_UNAVAILABLE", "周次导入失败，请稍后重试")
	}
}

func maxBytesError(err error) bool {
	var value *http.MaxBytesError
	return errors.As(err, &value)
}

package importjob

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
)

type Repository interface {
	CreateImportBatch(context.Context, Batch) error
	FindImportBatch(context.Context, string) (Batch, error)
	UpdateImportBatchState(context.Context, string, int, string, BatchFailure) (Batch, error)
	CancelImportBatch(context.Context, Batch, audit.Metadata) (Batch, error)
	ReplaceImportPreview(context.Context, Batch, []ImportRow) (Batch, error)
	ConfirmAttendanceImport(context.Context, ConfirmBatchInput, audit.Mutation) (ConfirmResult, error)
	FindPublishedImportTemplate(context.Context, string) (Template, error)
	ListImportRows(context.Context, string, int, int, string) (ImportRowPage, error)
}

type VerifiedFileReader interface {
	ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error)
}

type WorkbookParser interface {
	Parse(string, []byte) (ParsedWorkbook, error)
}

type AttendanceRowValidator interface {
	Validate(context.Context, AttendanceImportScope, []ParsedRow) (AttendanceValidationResult, error)
}

type Service struct {
	inspectionValidators map[string]InspectionRowValidator
	repository           Repository
	files                VerifiedFileReader
	parser               WorkbookParser
	parsers              map[string]WorkbookParser
	validator            AttendanceRowValidator
}

type ParseBatchInput struct {
	BatchID         string
	ExpectedVersion int
	AuditMetadata   audit.Metadata
}

type CreateBatchInput struct {
	InspectionTypeID string
	SemesterID       string
	WeekID           string
	DepartmentID     string
	ClassID          string
	Grade            string
	SourceFile       storage.File
	TemplateID       string
	TemplateVersion  int
	CreatedBy        string
}

func NewService(repository Repository, files VerifiedFileReader, parser WorkbookParser, validator AttendanceRowValidator) *Service {
	return &Service{repository: repository, files: files, parser: parser, validator: validator}
}

func (s *Service) CreateBatch(ctx context.Context, input CreateBatchInput) (Batch, error) {
	if input.DepartmentID == inspection.CollegeDepartmentOptionID {
		input.DepartmentID = ""
	}
	if strings.TrimSpace(input.InspectionTypeID) == "" || strings.TrimSpace(input.SemesterID) == "" ||
		strings.TrimSpace(input.WeekID) == "" ||
		strings.TrimSpace(input.TemplateID) == "" || input.TemplateVersion < 1 || strings.TrimSpace(input.CreatedBy) == "" {
		return Batch{}, ErrInvalidBatch
	}
	if strings.TrimSpace(input.SourceFile.ID) == "" || input.SourceFile.Status != storage.StatusAvailable || len(strings.TrimSpace(input.SourceFile.SHA256)) != 64 {
		return Batch{}, ErrSourceFileNotReady
	}
	now := time.Now().UTC()
	batch := Batch{
		ID: uuid.NewString(), InspectionTypeID: strings.TrimSpace(input.InspectionTypeID), SemesterID: strings.TrimSpace(input.SemesterID),
		WeekID: strings.TrimSpace(input.WeekID), DepartmentID: strings.TrimSpace(input.DepartmentID), OriginalFileID: input.SourceFile.ID,
		ClassID: strings.TrimSpace(input.ClassID), Grade: strings.TrimSpace(input.Grade),
		TemplateID: strings.TrimSpace(input.TemplateID), TemplateVersion: input.TemplateVersion, Status: BatchStatusUploaded,
		Version: 1, CreatedBy: strings.TrimSpace(input.CreatedBy), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repository.CreateImportBatch(ctx, batch); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func (s *Service) GetBatch(ctx context.Context, id string) (Batch, error) {
	if strings.TrimSpace(id) == "" {
		return Batch{}, ErrBatchNotFound
	}
	return s.repository.FindImportBatch(ctx, id)
}

func (s *Service) ListRows(ctx context.Context, batchID string, page, pageSize int, validationStatus string) (ImportRowPage, error) {
	validationStatus = strings.ToUpper(strings.TrimSpace(validationStatus))
	if strings.TrimSpace(batchID) == "" || page < 1 || pageSize < 1 || pageSize > 100 {
		return ImportRowPage{}, ErrInvalidBatch
	}
	if validationStatus != "" && validationStatus != RowStatusValid && validationStatus != RowStatusInvalid {
		return ImportRowPage{}, ErrInvalidBatch
	}
	return s.repository.ListImportRows(ctx, batchID, page, pageSize, validationStatus)
}

func (s *Service) PublishedTemplate(ctx context.Context, inspectionTypeID string) (Template, error) {
	if strings.TrimSpace(inspectionTypeID) == "" {
		return Template{}, ErrInvalidBatch
	}
	return s.repository.FindPublishedImportTemplate(ctx, inspectionTypeID)
}

func (s *Service) ParseBatch(ctx context.Context, input ParseBatchInput) (Batch, error) {
	batch, err := s.repository.FindImportBatch(ctx, input.BatchID)
	if err != nil {
		return Batch{}, err
	}
	if batch.Version != input.ExpectedVersion {
		return Batch{}, ErrBatchVersionConflict
	}
	if err := ValidateBatchTransition(batch.Status, BatchStatusParsing); err != nil {
		return Batch{}, err
	}
	batch, err = s.repository.UpdateImportBatchState(ctx, batch.ID, batch.Version, BatchStatusParsing, BatchFailure{})
	if err != nil {
		return Batch{}, err
	}
	file, content, err := s.files.ReadVerified(ctx, batch.OriginalFileID, input.AuditMetadata)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, stableParseFailure(err), err)
	}
	parser := s.parser
	if s.parsers != nil {
		var supported bool
		parser, supported = s.parsers[batch.TypeCode]
		if !supported {
			return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "当前检查类型不支持此导入格式"}, ErrInvalidBatch)
		}
	}
	workbook, err := parser.Parse(file.OriginalName, content)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, stableParseFailure(err), err)
	}
	if workbook.TemplateVersion != batch.TemplateVersion || batch.TemplateCode != "" && workbook.TemplateCode != batch.TemplateCode {
		err = parseError(ErrorTemplateVersionMismatch, "模板版本与批次记录不一致")
		return Batch{}, s.failParsing(ctx, batch, stableParseFailure(err), err)
	}
	scope := AttendanceImportScope{
		TypeCode: batch.TypeCode, InspectionTypeID: batch.InspectionTypeID, SemesterID: batch.SemesterID, WeekID: batch.WeekID,
		DepartmentID: batch.DepartmentID, ImportBatchID: batch.ID, CreatedBy: batch.CreatedBy,
		ClassID: batch.ClassID, Grade: batch.Grade,
		WeekStart: batch.WeekStart, WeekEnd: batch.WeekEnd, Location: batch.Location,
	}
	if validator := s.inspectionValidators[batch.TypeCode]; validator != nil {
		return s.parseInspectionPreview(ctx, batch, scope, workbook.Rows, validator)
	}
	validation, err := s.validator.Validate(ctx, scope, workbook.Rows)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "导入数据校验失败"}, err)
	}
	if !validationMatchesParsedRows(workbook.Rows, validation) {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "导入校验结果不完整"}, ErrValidationResultMismatch)
	}
	rows := previewRows(batch.ID, workbook.Rows, validation.Rows)
	previewBatch, err := s.repository.ReplaceImportPreview(ctx, batch, rows)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorPreviewPersistence), Message: "导入预览保存失败"}, err)
	}
	return previewBatch, nil
}

func (s *Service) CancelBatch(ctx context.Context, id string, expectedVersion int, metadata audit.Metadata) (Batch, error) {
	if strings.TrimSpace(metadata.ActorID) == "" {
		return Batch{}, ErrInvalidBatch
	}
	batch, err := s.repository.FindImportBatch(ctx, id)
	if err != nil {
		return Batch{}, err
	}
	if batch.Version != expectedVersion {
		return Batch{}, ErrBatchVersionConflict
	}
	if err := ValidateBatchTransition(batch.Status, BatchStatusCancelled); err != nil {
		return Batch{}, err
	}
	return s.repository.CancelImportBatch(ctx, batch, metadata)
}

func (s *Service) ConfirmBatch(ctx context.Context, input ConfirmBatchInput) (ConfirmResult, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return ConfirmResult{}, ErrInvalidIdempotencyKey
	}
	batch, err := s.repository.FindImportBatch(ctx, input.BatchID)
	if err != nil {
		return ConfirmResult{}, err
	}
	if !batchWithinConfirmScope(batch, input.Scope) {
		return ConfirmResult{}, ErrBatchScopeMismatch
	}
	if batch.Status == BatchStatusConfirmed {
		if batch.ConfirmIdempotencyKey == input.IdempotencyKey {
			return ConfirmResult{BatchID: batch.ID, Status: batch.Status, ImportedRows: batch.ImportedRows, Version: batch.Version}, nil
		}
		return ConfirmResult{}, ErrBatchAlreadyConfirmed
	}
	if batch.Version != input.ExpectedVersion {
		return ConfirmResult{}, ErrBatchVersionConflict
	}
	if err := ValidateBatchTransition(batch.Status, BatchStatusConfirming); err != nil {
		return ConfirmResult{}, err
	}
	if batch.TotalRows <= 0 || batch.InvalidRows != 0 || batch.ValidRows != batch.TotalRows {
		return ConfirmResult{}, ErrBatchHasErrors
	}
	mutation := audit.Mutation{Metadata: input.AuditMetadata, Entry: audit.Entry{
		Action: audit.ActionImportBatchConfirmed, ResourceType: "import_batch", ResourceID: batch.ID,
		Before: map[string]any{"status": batch.Status, "version": batch.Version},
		After: map[string]any{
			"status": BatchStatusConfirmed, "importedRows": batch.ValidRows, "templateVersion": batch.TemplateVersion,
			"semesterId": batch.SemesterID, "weekId": batch.WeekID, "inspectionTypeId": batch.InspectionTypeID, "departmentId": batch.DepartmentID,
		},
	}}
	return s.repository.ConfirmAttendanceImport(ctx, input, mutation)
}

func batchWithinConfirmScope(batch Batch, scope ConfirmScope) bool {
	return batch.SemesterID == strings.TrimSpace(scope.SemesterID) &&
		batch.WeekID == strings.TrimSpace(scope.WeekID) &&
		batch.InspectionTypeID == strings.TrimSpace(scope.InspectionTypeID) &&
		batch.DepartmentID == strings.TrimSpace(scope.DepartmentID) &&
		batch.ClassID == strings.TrimSpace(scope.ClassID) &&
		batch.Grade == strings.TrimSpace(scope.Grade) &&
		batch.CreatedBy == strings.TrimSpace(scope.ActorID)
}

func (s *Service) failParsing(ctx context.Context, batch Batch, failure BatchFailure, cause error) error {
	_, transitionErr := s.repository.UpdateImportBatchState(ctx, batch.ID, batch.Version, BatchStatusParseFailed, failure)
	if transitionErr != nil {
		return fmt.Errorf("parse failed (%v) and failure state could not be saved: %w", cause, transitionErr)
	}
	return cause
}

func stableParseFailure(err error) BatchFailure {
	var parsed *ParseError
	if errors.As(err, &parsed) {
		return BatchFailure{Code: string(parsed.Code), Message: parsed.Message}
	}
	if errors.Is(err, storage.ErrFileUnavailable) || errors.Is(err, storage.ErrObjectMissing) || errors.Is(err, storage.ErrIntegrityMismatch) {
		return BatchFailure{Code: string(ErrorSourceFileUnavailable), Message: "导入源文件不可用或完整性校验失败"}
	}
	return BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "导入文件处理失败"}
}

func previewRows(batchID string, parsed []ParsedRow, validated []AttendanceValidationRow) []ImportRow {
	byNumber := make(map[int]AttendanceValidationRow, len(validated))
	for _, row := range validated {
		byNumber[row.RowNo] = row
	}
	result := make([]ImportRow, 0, len(parsed))
	for _, row := range parsed {
		validation := byNumber[row.RowNo]
		status := RowStatusValid
		if len(validation.Errors) != 0 {
			status = RowStatusInvalid
		}
		result = append(result, ImportRow{
			ID: uuid.NewString(), BatchID: batchID, RowNo: row.RowNo, RawData: row.Values,
			NormalizedRecord: validation.Record, ValidationStatus: status, Errors: validation.Errors,
		})
	}
	return result
}

func validationMatchesParsedRows(parsed []ParsedRow, validation AttendanceValidationResult) bool {
	if len(parsed) != len(validation.Rows) || validation.ValidCount+validation.InvalidCount != len(parsed) {
		return false
	}
	parsedRows := make(map[int]struct{}, len(parsed))
	for _, row := range parsed {
		if _, duplicate := parsedRows[row.RowNo]; duplicate {
			return false
		}
		parsedRows[row.RowNo] = struct{}{}
	}
	for _, row := range validation.Rows {
		if _, exists := parsedRows[row.RowNo]; !exists {
			return false
		}
		delete(parsedRows, row.RowNo)
	}
	return len(parsedRows) == 0
}

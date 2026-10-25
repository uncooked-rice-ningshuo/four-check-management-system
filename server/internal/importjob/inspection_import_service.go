package importjob

import (
	"context"
	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
)

// InspectionRowValidator produces a typed date/object preview instead of forcing
// non-course checks into attendance-specific fields.
type InspectionRowValidator interface {
	Validate(context.Context, AttendanceImportScope, []ParsedRow) (InspectionValidationResult, error)
}

// NewInspectionImportService adds typed student and object adapters while keeping
// existing course imports and their persisted preview format compatible.
func NewInspectionImportService(repository Repository, files VerifiedFileReader, course AttendanceRowValidator, morning, lateReturn, dormitoryHygiene, classroomHygiene InspectionRowValidator, maxBytes int64, evening ...InspectionRowValidator) *Service {
	service := NewCourseImportService(repository, files, course, maxBytes)
	service.parsers[inspection.InspectionTypeMorning] = NewMorningXLSXParser(maxBytes)
	service.parsers[inspection.InspectionTypeLateReturn] = NewLateReturnXLSXParser(maxBytes)
	service.parsers[inspection.InspectionTypeEveningSelfStudy], _ = NewInspectionXLSXParser(inspection.InspectionTypeEveningSelfStudy, maxBytes)
	hygieneConfig := NewAttendanceXLSXParser(maxBytes).config
	hygieneConfig.ExpectedTemplateCode = "DORMITORY_HYGIENE_IMPORT"
	hygieneConfig.ExpectedSheetName = "宿舍卫生导入"
	hygieneConfig.Fields = append([]TemplateField(nil), dormitoryHygieneImportFields...)
	service.parsers[inspection.InspectionTypeDormitoryHygiene] = NewXLSXParser(hygieneConfig)
	classroomConfig := NewAttendanceXLSXParser(maxBytes).config
	classroomConfig.ExpectedTemplateCode = "CLASSROOM_HYGIENE_IMPORT"
	classroomConfig.ExpectedSheetName = "教室卫生导入"
	classroomConfig.Fields = append([]TemplateField(nil), classroomHygieneImportFields...)
	service.parsers[inspection.InspectionTypeClassroomHygiene] = NewXLSXParser(classroomConfig)
	service.inspectionValidators = map[string]InspectionRowValidator{inspection.InspectionTypeMorning: morning, inspection.InspectionTypeLateReturn: lateReturn, inspection.InspectionTypeDormitoryHygiene: dormitoryHygiene, inspection.InspectionTypeClassroomHygiene: classroomHygiene}
	if len(evening) > 0 && evening[0] != nil {
		service.inspectionValidators[inspection.InspectionTypeEveningSelfStudy] = evening[0]
	}
	return service
}

func (s *Service) parseInspectionPreview(ctx context.Context, batch Batch, scope AttendanceImportScope, parsed []ParsedRow, validator InspectionRowValidator) (Batch, error) {
	validation, err := validator.Validate(ctx, scope, parsed)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "导入数据校验失败"}, err)
	}
	legacySummary := AttendanceValidationResult{ValidCount: validation.ValidCount, InvalidCount: validation.InvalidCount}
	for _, row := range validation.Rows {
		legacySummary.Rows = append(legacySummary.Rows, AttendanceValidationRow{RowNo: row.RowNo, Errors: row.Errors})
	}
	if !validationMatchesParsedRows(parsed, legacySummary) {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorInvalidWorkbook), Message: "导入校验结果不完整"}, ErrValidationResultMismatch)
	}
	byNumber := map[int]InspectionValidationRow{}
	for _, row := range validation.Rows {
		byNumber[row.RowNo] = row
	}
	rows := make([]ImportRow, 0, len(parsed))
	for _, row := range parsed {
		validated := byNumber[row.RowNo]
		status := RowStatusValid
		if len(validated.Errors) > 0 {
			status = RowStatusInvalid
		}
		record := validated.Record
		rows = append(rows, ImportRow{ID: uuid.NewString(), BatchID: batch.ID, RowNo: row.RowNo, RawData: row.Values, NormalizedInspection: &record, ValidationStatus: status, Errors: validated.Errors})
	}
	result, err := s.repository.ReplaceImportPreview(ctx, batch, rows)
	if err != nil {
		return Batch{}, s.failParsing(ctx, batch, BatchFailure{Code: string(ErrorPreviewPersistence), Message: "导入预览保存失败"}, err)
	}
	return result, nil
}

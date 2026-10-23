package importjob

import (
	"context"
	"four-check-management-system/server/internal/inspection"
)

// ClassroomHygieneAdapter validates a configured classroom area and responsibilities.
type ClassroomHygieneAdapter struct{ repository HygieneImportRepository }

// NewClassroomHygieneAdapter shares the scoped hygiene reference repository.
func NewClassroomHygieneAdapter(repository HygieneImportRepository) *ClassroomHygieneAdapter {
	return &ClassroomHygieneAdapter{repository: repository}
}
func (a *ClassroomHygieneAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypeClassroomHygiene {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	return validateHygieneImport(ctx, a.repository, scope, rows)
}

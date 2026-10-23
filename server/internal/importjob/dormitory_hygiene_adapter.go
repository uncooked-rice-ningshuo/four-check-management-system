package importjob

import (
	"context"
	"four-check-management-system/server/internal/inspection"
)

// DormitoryHygieneAdapter validates object-level results and every responsibility.
type DormitoryHygieneAdapter struct{ repository HygieneImportRepository }

// NewDormitoryHygieneAdapter resolves an inspected room independently of its responsibilities.
func NewDormitoryHygieneAdapter(repository HygieneImportRepository) *DormitoryHygieneAdapter {
	return &DormitoryHygieneAdapter{repository: repository}
}

func (a *DormitoryHygieneAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypeDormitoryHygiene {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	return validateHygieneImport(ctx, a.repository, scope, rows)
}

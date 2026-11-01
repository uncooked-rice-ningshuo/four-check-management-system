package importjob

import (
	"context"
	"four-check-management-system/server/internal/inspection"
)

// PublicDormitoryHygieneAdapter records a responsible dormitory without an
// inspected public-area identifier.
type PublicDormitoryHygieneAdapter struct{ repository HygieneImportRepository }

func NewPublicDormitoryHygieneAdapter(repository HygieneImportRepository) *PublicDormitoryHygieneAdapter {
	return &PublicDormitoryHygieneAdapter{repository: repository}
}

func (a *PublicDormitoryHygieneAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypePublicDormitoryHygiene {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	return validateHygieneImport(ctx, a.repository, scope, rows)
}

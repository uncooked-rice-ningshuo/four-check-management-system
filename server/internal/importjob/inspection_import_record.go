package importjob

import "four-check-management-system/server/internal/inspection"

// InspectionImportRecord stores a normalized typed input with preview snapshots.
// Confirmation must revalidate current references before using these snapshots.
type InspectionImportRecord struct {
	SchemaVersion int                             `json:"schemaVersion"`
	ID            string                          `json:"id"`
	TypeCode      string                          `json:"typeCode"`
	Input         inspection.InspectionInput      `json:"input"`
	References    inspection.InspectionReferences `json:"references"`
	CreatedBy     string                          `json:"createdBy"`
	ImportBatchID string                          `json:"importBatchId"`
	DedupKey      string                          `json:"-"`
}

// InspectionValidationRow preserves source row numbers for precise corrections.
type InspectionValidationRow struct {
	RowNo  int
	Record InspectionImportRecord
	Errors []ValidationError
}

// InspectionValidationResult summarizes the all-or-nothing preview gate.
type InspectionValidationResult struct {
	Rows         []InspectionValidationRow
	ValidCount   int
	InvalidCount int
}

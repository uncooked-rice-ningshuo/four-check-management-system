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
	Duplicate     *InspectionDuplicateCandidate   `json:"-"`
}

// InspectionDuplicateCandidate carries only normalized identity fields used to
// match existing records during import preview; it contains no display data.
type InspectionDuplicateCandidate struct {
	Key         string `json:"key"`
	ClassID     string `json:"class_id"`
	StudentID   string `json:"student_id"`
	ObjectID    string `json:"object_id,omitempty"`
	Date        string `json:"date"`
	PeriodStart int    `json:"period_start,omitempty"`
	PeriodEnd   int    `json:"period_end,omitempty"`
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

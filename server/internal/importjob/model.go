package importjob

import (
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
)

const (
	BatchStatusUploaded      = "UPLOADED"
	BatchStatusParsing       = "PARSING"
	BatchStatusPreviewReady  = "PREVIEW_READY"
	BatchStatusConfirming    = "CONFIRMING"
	BatchStatusConfirmed     = "CONFIRMED"
	BatchStatusParseFailed   = "PARSE_FAILED"
	BatchStatusConfirmFailed = "CONFIRM_FAILED"
	BatchStatusCancelled     = "CANCELLED"

	RowStatusValid   = "VALID"
	RowStatusInvalid = "INVALID"

	ErrorSourceFileUnavailable ErrorCode = "SOURCE_FILE_UNAVAILABLE"
	ErrorPreviewPersistence    ErrorCode = "PREVIEW_PERSISTENCE_FAILED"
)

var (
	ErrBatchNotFound            = errors.New("import batch not found")
	ErrBatchVersionConflict     = errors.New("import batch version conflict")
	ErrInvalidBatchTransition   = errors.New("invalid import batch transition")
	ErrBatchHasErrors           = errors.New("import batch has validation errors")
	ErrBatchAlreadyConfirmed    = errors.New("import batch already confirmed with another idempotency key")
	ErrInvalidIdempotencyKey    = errors.New("invalid idempotency key")
	ErrBatchScopeMismatch       = errors.New("import batch is outside effective scope")
	ErrValidationResultMismatch = errors.New("validation result does not match parsed rows")
	ErrSourceFileNotReady       = errors.New("import source file is not verified and available")
	ErrInvalidBatch             = errors.New("invalid import batch")
	ErrTemplateNotFound         = errors.New("published import template not found")
)

type Batch struct {
	ID                    string         `json:"id"`
	InspectionTypeID      string         `json:"inspectionTypeId"`
	SemesterID            string         `json:"semesterId"`
	WeekID                string         `json:"weekId"`
	DepartmentID          string         `json:"departmentId"`
	ClassID               string         `json:"classId,omitempty"`
	Grade                 string         `json:"grade,omitempty"`
	OriginalFileID        string         `json:"originalFileId,omitempty"`
	TemplateID            string         `json:"templateId"`
	TemplateVersion       int            `json:"templateVersion"`
	Status                string         `json:"status"`
	TotalRows             int            `json:"totalRows"`
	ValidRows             int            `json:"validRows"`
	InvalidRows           int            `json:"invalidRows"`
	ImportedRows          int            `json:"importedRows"`
	Version               int            `json:"version"`
	ConfirmIdempotencyKey string         `json:"-"`
	FailureCode           string         `json:"failureCode,omitempty"`
	FailureMessage        string         `json:"failureMessage,omitempty"`
	CreatedBy             string         `json:"createdBy"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
	ConfirmedAt           time.Time      `json:"confirmedAt,omitempty"`
	CancelledAt           time.Time      `json:"cancelledAt,omitempty"`
	WeekStart             time.Time      `json:"-"`
	WeekEnd               time.Time      `json:"-"`
	Location              *time.Location `json:"-"`
}

type BatchFailure struct {
	Code    string
	Message string
}

type ImportRow struct {
	ID               string                      `json:"id"`
	BatchID          string                      `json:"batchId"`
	RowNo            int                         `json:"rowNo"`
	RawData          map[string]string           `json:"rawData"`
	NormalizedRecord inspection.AttendanceRecord `json:"normalizedRecord"`
	ValidationStatus string                      `json:"validationStatus"`
	Errors           []ValidationError           `json:"errors"`
	RecordID         string                      `json:"recordId,omitempty"`
}

type ImportRowPage struct {
	Items    []ImportRow `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

type Template struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	InspectionTypeID string `json:"inspectionTypeId"`
	Version          int    `json:"version"`
	FileID           string `json:"fileId"`
	Status           string `json:"status"`
}

type ConfirmScope struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
	ActorID          string
}

type ConfirmBatchInput struct {
	BatchID         string
	ExpectedVersion int
	IdempotencyKey  string
	Scope           ConfirmScope
	AuditMetadata   audit.Metadata
}

type ConfirmResult struct {
	BatchID      string `json:"batchId"`
	Status       string `json:"status"`
	ImportedRows int    `json:"importedRows"`
	Version      int    `json:"version"`
}

func ValidateBatchTransition(current, target string) error {
	allowed := map[string]map[string]bool{
		BatchStatusUploaded:      {BatchStatusParsing: true, BatchStatusCancelled: true},
		BatchStatusParsing:       {BatchStatusPreviewReady: true, BatchStatusParseFailed: true},
		BatchStatusParseFailed:   {BatchStatusParsing: true, BatchStatusCancelled: true},
		BatchStatusPreviewReady:  {BatchStatusConfirming: true, BatchStatusCancelled: true},
		BatchStatusConfirming:    {BatchStatusConfirmed: true, BatchStatusConfirmFailed: true},
		BatchStatusConfirmFailed: {BatchStatusConfirming: true, BatchStatusCancelled: true},
	}
	if !allowed[current][target] {
		return ErrInvalidBatchTransition
	}
	return nil
}

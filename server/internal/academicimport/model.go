package academicimport

import (
	"context"
	"errors"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
)

const (
	StatusPreviewReady     = "PREVIEW_READY"
	StatusPreviewHasErrors = "PREVIEW_HAS_ERRORS"
	StatusConfirmed        = "CONFIRMED"
	RowValid               = "VALID"
	RowInvalid             = "INVALID"
)

var (
	ErrInvalidBatch          = errors.New("academic week import batch is invalid")
	ErrBatchNotFound         = errors.New("academic week import batch not found")
	ErrBatchVersionConflict  = errors.New("academic week import batch version conflict")
	ErrBatchHasErrors        = errors.New("academic week import batch contains invalid rows")
	ErrAlreadyConfirmed      = errors.New("academic week import batch was confirmed with another idempotency key")
	ErrInvalidIdempotencyKey = errors.New("academic week import idempotency key is invalid")
	ErrPreviewStale          = errors.New("academic week import preview references changed data")
	ErrEmptyWorkbook         = errors.New("academic week import workbook has no rows")
	ErrSemesterNotFound      = errors.New("academic week import semester not found")
)

type RowError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type PreviewRow struct {
	RowNo  int               `json:"rowNo"`
	Values map[string]string `json:"values"`
	Status string            `json:"status"`
	Errors []RowError        `json:"errors"`
	Week   *academic.Week    `json:"week,omitempty"`
}

type Batch struct {
	ID                    string       `json:"batchId"`
	Status                string       `json:"status"`
	Version               int          `json:"version"`
	TemplateVersion       int          `json:"templateVersion"`
	SemesterID            string       `json:"semesterId"`
	SemesterName          string       `json:"semesterName"`
	AcademicYearName      string       `json:"academicYearName"`
	TotalRows             int          `json:"totalRows"`
	ValidRows             int          `json:"validRows"`
	InvalidRows           int          `json:"invalidRows"`
	AffectsToday          bool         `json:"affectsToday"`
	Rows                  []PreviewRow `json:"rows"`
	CreatedAt             time.Time    `json:"createdAt"`
	ConfirmedAt           *time.Time   `json:"confirmedAt,omitempty"`
	SourceFileID          string       `json:"-"`
	CreatedBy             string       `json:"-"`
	ConfirmIdempotencyKey string       `json:"-"`
}

type ConfirmInput struct {
	BatchID         string
	ActorID         string
	ExpectedVersion int
	IdempotencyKey  string
	Metadata        audit.Metadata
}

type ConfirmResult struct {
	BatchID      string `json:"batchId"`
	Status       string `json:"status"`
	ImportedRows int    `json:"importedRows"`
	Version      int    `json:"version"`
}

type Catalog interface {
	List(context.Context) (academic.Calendar, error)
}

type Repository interface {
	CreateAcademicWeekImportBatch(context.Context, Batch, audit.Metadata) error
	FindAcademicWeekImportBatch(context.Context, string, string) (Batch, error)
	ConfirmAcademicWeekImportBatch(context.Context, ConfirmInput) (ConfirmResult, error)
}

package masterdataimport

import (
	"context"
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/masterdata"
)

const (
	StatusPreviewReady     = "PREVIEW_READY"
	StatusPreviewHasErrors = "PREVIEW_HAS_ERRORS"
	StatusConfirmed        = "CONFIRMED"
	RowValid               = "VALID"
	RowInvalid             = "INVALID"
)

var (
	ErrInvalidKind           = errors.New("master data import kind is unsupported")
	ErrInvalidBatch          = errors.New("master data import batch is invalid")
	ErrBatchNotFound         = errors.New("master data import batch not found")
	ErrBatchVersionConflict  = errors.New("master data import batch version conflict")
	ErrBatchHasErrors        = errors.New("master data import batch contains invalid rows")
	ErrAlreadyConfirmed      = errors.New("master data import batch was confirmed with another idempotency key")
	ErrInvalidIdempotencyKey = errors.New("master data import idempotency key is invalid")
	ErrPreviewStale          = errors.New("master data import references changed after preview")
	ErrEmptyWorkbook         = errors.New("master data import workbook has no data rows")
	ErrReferenceNotUnique    = errors.New("master data reference name is ambiguous")
)

type LookupKey struct {
	ParentID string
	Code     string
	Name     string
	RoomNo   string
}

type RowError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type PreviewRow struct {
	RowNo  int                `json:"rowNo"`
	Values map[string]string  `json:"values"`
	Status string             `json:"status"`
	Errors []RowError         `json:"errors"`
	Record *masterdata.Record `json:"record,omitempty"`
}

type Batch struct {
	ID                    string       `json:"id"`
	Kind                  string       `json:"kind"`
	Status                string       `json:"status"`
	TemplateVersion       int          `json:"templateVersion"`
	Fields                []FieldSpec  `json:"fields"`
	SourceFileID          string       `json:"sourceFileId,omitempty"`
	TotalRows             int          `json:"totalRows"`
	ValidRows             int          `json:"validRows"`
	InvalidRows           int          `json:"invalidRows"`
	Version               int          `json:"version"`
	Rows                  []PreviewRow `json:"rows"`
	ConfirmIdempotencyKey string       `json:"-"`
	CreatedBy             string       `json:"-"`
	CreatedAt             time.Time    `json:"createdAt"`
	ConfirmedAt           *time.Time   `json:"confirmedAt,omitempty"`
}

type ConfirmInput struct {
	BatchID         string
	Kind            string
	ActorID         string
	ExpectedVersion int
	IdempotencyKey  string
	Metadata        audit.Metadata
}

type ConfirmResult struct {
	BatchID      string `json:"batchId"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	ImportedRows int    `json:"importedRows"`
	Version      int    `json:"version"`
}

type Catalog interface {
	FindMasterDataImportRecords(context.Context, string, []LookupKey, bool) ([]masterdata.Record, error)
	EnabledDictionaryValues(context.Context, string) ([]string, error)
}

type Repository interface {
	CreateMasterDataImportBatch(context.Context, Batch, audit.Metadata) error
	FindMasterDataImportBatch(context.Context, string, string, string) (Batch, error)
	ConfirmMasterDataImportBatch(context.Context, ConfirmInput) (ConfirmResult, error)
}

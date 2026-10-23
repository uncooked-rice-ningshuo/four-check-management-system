package importjob

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

// BatchStatusRolledBack retains a completed confirmation and its original files.
const BatchStatusRolledBack = "ROLLED_BACK"

// ErrRollbackUnsafe means the original confirmation can no longer be undone as a whole.
var ErrRollbackUnsafe = errors.New("import batch no longer consists of untouched drafts")

// RollbackRecord contains only fields required to protect the imported snapshot.
type RollbackRecord struct {
	ID                                    string
	Status                                string
	Version                               int
	HasSubmission, HasReportLock, Deleted bool
}

// ValidateRollback rejects partial, edited, submitted or locked confirmations.
func ValidateRollback(batch Batch, version int, records []RollbackRecord) error {
	if batch.Version != version {
		return ErrBatchVersionConflict
	}
	if batch.Status != BatchStatusConfirmed {
		return ErrInvalidBatchTransition
	}
	if batch.ImportedRows <= 0 || len(records) != batch.ImportedRows {
		return ErrRollbackUnsafe
	}
	for _, record := range records {
		if record.Status != inspection.RecordStatusDraft || record.Version != 1 || record.HasSubmission || record.HasReportLock || record.Deleted {
			return ErrRollbackUnsafe
		}
	}
	return nil
}

// RollbackTransaction locks the batch before its records, preserving confirmation ordering.
type RollbackTransaction interface {
	Batch(context.Context, string, inspection.MutationAccess) (Batch, error)
	Records(context.Context, string, inspection.MutationAccess) ([]RollbackRecord, error)
	Apply(context.Context, Batch, []RollbackRecord, audit.Metadata) error
}

// RollbackRepository supplies the atomic boundary for an entire batch.
type RollbackRepository interface {
	WithImportRollback(context.Context, func(RollbackTransaction) error) error
}
type rollbackPermissions interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}

// RollbackService coordinates one authorized, auditable transaction.
type RollbackService struct {
	repository  RollbackRepository
	permissions rollbackPermissions
}

// NewRollbackService uses the same effective grants as manual record mutations.
func NewRollbackService(repository RollbackRepository, permissions rollbackPermissions) *RollbackService {
	return &RollbackService{repository, permissions}
}

// RollbackInput identifies the original confirmation and requires an audit reason.
type RollbackInput struct {
	BatchID string
	Version int
	Reason  string
}

// RollbackResult reports the new batch version without exposing source content.
type RollbackResult struct {
	BatchID        string `json:"batchId"`
	Status         string `json:"status"`
	RolledBackRows int    `json:"rolledBackRows"`
	Version        int    `json:"version"`
}

// Rollback preserves source files, preview rows and original record details.
func (s *RollbackService) Rollback(ctx context.Context, actor inspection.ManualActor, input RollbackInput, metadata audit.Metadata) (RollbackResult, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	batchID, err := uuid.Parse(input.BatchID)
	if err != nil {
		return RollbackResult{}, ErrInvalidBatch
	}
	input.BatchID = batchID.String()
	if _, err := uuid.Parse(actor.ID); err != nil {
		return RollbackResult{}, ErrBatchScopeMismatch
	}
	if input.Version < 1 || input.Reason == "" || utf8.RuneCountInString(input.Reason) > 255 {
		return RollbackResult{}, ErrInvalidBatch
	}
	access := inspection.MutationAccess{ActorID: actor.ID, Unrestricted: actor.IsSuperAdmin}
	if !actor.IsSuperAdmin {
		permissions, err := s.permissions.Effective(ctx, actor.ID)
		if err != nil {
			return RollbackResult{}, err
		}
		access.Permissions = permissions.Permissions
	}
	metadata.ActorID, metadata.Reason = actor.ID, input.Reason
	var result RollbackResult
	err = s.repository.WithImportRollback(ctx, func(tx RollbackTransaction) error {
		batch, err := tx.Batch(ctx, input.BatchID, access)
		if err != nil {
			return err
		}
		records, err := tx.Records(ctx, batch.ID, access)
		if err != nil {
			return err
		}
		if err = ValidateRollback(batch, input.Version, records); err != nil {
			return err
		}
		if err = tx.Apply(ctx, batch, records, metadata); err != nil {
			return err
		}
		result = RollbackResult{batch.ID, BatchStatusRolledBack, len(records), batch.Version + 1}
		return nil
	})
	return result, err
}

package store

import (
	"context"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
)

// CancelImportBatch keeps the original file and preview while committing the
// versioned state change and its audit together.
func (p *Postgres) CancelImportBatch(ctx context.Context, expected importjob.Batch, metadata audit.Metadata) (importjob.Batch, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return importjob.Batch{}, err
	}
	defer tx.Rollback(ctx)
	batch, err := scanImportBatch(tx.QueryRow(ctx, importBatchSelect+` WHERE b.id=$1 FOR UPDATE OF b`, expected.ID))
	if err != nil {
		return importjob.Batch{}, err
	}
	if batch.Version != expected.Version {
		return importjob.Batch{}, importjob.ErrBatchVersionConflict
	}
	if err := importjob.ValidateBatchTransition(batch.Status, importjob.BatchStatusCancelled); err != nil {
		return importjob.Batch{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE import_batches SET status='CANCELLED',failure_code='',failure_message='',version=version+1,updated_at=now(),cancelled_at=now() WHERE id=$1`, batch.ID); err != nil {
		return importjob.Batch{}, err
	}
	result, err := scanImportBatch(tx.QueryRow(ctx, importBatchSelect+` WHERE b.id=$1`, batch.ID))
	if err != nil {
		return importjob.Batch{}, err
	}
	entry := audit.Entry{Action: audit.ActionImportBatchCancelled, ResourceType: "import_batch", ResourceID: batch.ID, Before: map[string]any{"status": batch.Status, "version": batch.Version}, After: map[string]any{"status": result.Status, "version": result.Version}}
	if err := insertAuditTx(ctx, tx, metadata, entry); err != nil {
		return importjob.Batch{}, err
	}
	return result, tx.Commit(ctx)
}

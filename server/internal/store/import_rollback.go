package store

import (
	"context"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"github.com/jackc/pgx/v5"
)

type importRollbackTx struct{ tx pgx.Tx }

// WithImportRollback commits record deletion, batch state and audit together.
func (p *Postgres) WithImportRollback(ctx context.Context, fn func(importjob.RollbackTransaction) error) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(&importRollbackTx{tx}); err != nil {
		return inspectionDBError(err)
	}
	return inspectionDBError(tx.Commit(ctx))
}

func (t *importRollbackTx) Batch(ctx context.Context, id string, access inspection.MutationAccess) (importjob.Batch, error) {
	args := []any{id}
	guard := inspectionAccessSQL(access, "write", &args)
	// Only a caller able to mutate at least one associated record can discover
	// this batch. Records below still requires authorization for the entire set.
	return scanImportBatch(t.tx.QueryRow(ctx, importBatchSelect+` WHERE b.id=$1 AND EXISTS (
 SELECT 1 FROM inspection_records ir JOIN semesters sem ON sem.id=ir.semester_id
 WHERE ir.import_batch_id=b.id AND `+guard+`) FOR UPDATE OF b`, args...))
}

func (t *importRollbackTx) Records(ctx context.Context, id string, access inspection.MutationAccess) ([]importjob.RollbackRecord, error) {
	args := []any{id}
	guard := inspectionAccessSQL(access, "write", &args)
	rows, err := t.tx.Query(ctx, `SELECT ir.id,ir.status,ir.row_version,ir.submission_id IS NOT NULL,
 ir.locked_by_report_id IS NOT NULL,ir.deleted_at IS NOT NULL
 FROM inspection_records ir JOIN semesters sem ON sem.id=ir.semester_id
 WHERE ir.import_batch_id=$1 AND `+guard+` ORDER BY ir.id FOR UPDATE OF ir`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []importjob.RollbackRecord{}
	for rows.Next() {
		var r importjob.RollbackRecord
		if err = rows.Scan(&r.ID, &r.Status, &r.Version, &r.HasSubmission, &r.HasReportLock, &r.Deleted); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	var total, linked int
	if err = t.tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE EXISTS (
 SELECT 1 FROM import_rows r WHERE r.batch_id=$1 AND r.record_id=ir.id))
 FROM inspection_records ir WHERE ir.import_batch_id=$1`, id).Scan(&total, &linked); err != nil {
		return nil, err
	}
	if len(result) != total {
		return nil, importjob.ErrBatchScopeMismatch
	}
	if linked != total {
		return nil, importjob.ErrRollbackUnsafe
	}
	return result, nil
}

func (t *importRollbackTx) Apply(ctx context.Context, batch importjob.Batch, records []importjob.RollbackRecord, metadata audit.Metadata) error {
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.ID
	}
	tag, err := t.tx.Exec(ctx, `UPDATE inspection_records SET deleted_at=now(),updated_at=now(),row_version=row_version+1
 WHERE id=ANY($1::uuid[]) AND import_batch_id=$2 AND row_version=1 AND status='DRAFT'
 AND submission_id IS NULL AND locked_by_report_id IS NULL AND deleted_at IS NULL`, ids, batch.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(records)) {
		return importjob.ErrRollbackUnsafe
	}
	tag, err = t.tx.Exec(ctx, `UPDATE import_batches SET status='ROLLED_BACK',rolled_back_at=now(),updated_at=now(),version=version+1
 WHERE id=$1 AND version=$2 AND status='CONFIRMED'`, batch.ID, batch.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return importjob.ErrBatchVersionConflict
	}
	for _, r := range records {
		if err = insertAuditTx(ctx, t.tx, metadata, audit.Entry{Action: audit.ActionInspectionRecordDeleted, ResourceType: "inspection_record", ResourceID: r.ID,
			Before: map[string]any{"status": r.Status, "version": r.Version, "importBatchId": batch.ID},
			After:  map[string]any{"deleted": true, "version": r.Version + 1, "importBatchId": batch.ID}}); err != nil {
			return err
		}
	}
	return insertAuditTx(ctx, t.tx, metadata, audit.Entry{Action: audit.ActionImportBatchRolledBack, ResourceType: "import_batch", ResourceID: batch.ID,
		Before: map[string]any{"status": batch.Status, "version": batch.Version, "importedRows": batch.ImportedRows},
		After:  map[string]any{"status": importjob.BatchStatusRolledBack, "version": batch.Version + 1, "rolledBackRows": len(records)}})
}

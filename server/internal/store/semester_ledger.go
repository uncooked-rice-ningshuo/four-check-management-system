package store

import (
	"context"
	_ "embed"
	"encoding/json"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
)

//go:embed semester_ledger.sql
var semesterLedgerSQL string

// SemesterLedger aggregates frozen records and uses one statement snapshot for rows and rules.
func (p *Postgres) SemesterLedger(ctx context.Context, semester string, filter reporting.LedgerFilter, page, size int) (json.RawMessage, error) {
	var result json.RawMessage
	err := p.Pool.QueryRow(ctx, semesterLedgerSQL, semester, filter, size, (page-1)*size, page).Scan(&result)
	return result, err
}

func (p *Postgres) RecordLedgerExport(ctx context.Context, semester string, rows int, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = insertAuditTx(ctx, tx, metadata, audit.Entry{Action: "DISCIPLINE_LEDGER_EXPORTED", ResourceType: "semester", ResourceID: semester, After: map[string]any{"studentCount": rows}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

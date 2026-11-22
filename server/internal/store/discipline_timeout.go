package store

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ConfirmExpiredDisciplineCases locks due rows and records each system decision in the same statement.
func (p *Postgres) ConfirmExpiredDisciplineCases(ctx context.Context) (int64, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH due AS (
 SELECT id FROM discipline_cases WHERE status='PENDING' AND due_at<=statement_timestamp()
 ORDER BY due_at,id LIMIT 100 FOR UPDATE SKIP LOCKED
), confirmed AS (
 UPDATE discipline_cases c SET status='APPROVED',decision_source='TIMEOUT',
 decided_at=statement_timestamp(),row_version=row_version+1 FROM due WHERE c.id=due.id
 RETURNING c.id,c.rule_version_id,c.report_version_id,c.due_at,c.row_version
)
INSERT INTO audit_logs(id,action,resource_type,resource_id,before_json,after_json)
SELECT gen_random_uuid(),'DISCIPLINE_TIMEOUT_CONFIRMED','discipline_case',id::text,
 jsonb_build_object('status','PENDING','rowVersion',row_version-1),
 jsonb_build_object('status','APPROVED','source','TIMEOUT','rowVersion',row_version,'dueAt',due_at,
 'ruleVersionId',rule_version_id,'reportVersionId',report_version_id) FROM confirmed RETURNING resource_id`)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	if err := notifyDisciplineCases(ctx, tx, ids); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

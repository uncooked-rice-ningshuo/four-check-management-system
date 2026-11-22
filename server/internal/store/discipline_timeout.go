package store

import "context"

// ConfirmExpiredDisciplineCases locks due rows and records each system decision in the same statement.
func (p *Postgres) ConfirmExpiredDisciplineCases(ctx context.Context) (int64, error) {
	tag, err := p.Pool.Exec(ctx, `WITH due AS (
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
 'ruleVersionId',rule_version_id,'reportVersionId',report_version_id) FROM confirmed`)
	return tag.RowsAffected(), err
}

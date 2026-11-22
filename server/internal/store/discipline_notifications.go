package store

import (
	"context"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// notifyDisciplineCases uses the review queue's class predicate and the same
// transaction as the decision. Former recipients receive only a status update.
func notifyDisciplineCases(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO account_notifications(account_id,category,body,discipline_case_id,discipline_case_version,discipline_event_status)
 SELECT DISTINCT n.account_id,'discipline',CASE d.status WHEN 'VOID' THEN '原处分事项已作废。' WHEN 'DEFERRED' THEN '处分事项已延迟处理。' ELSE '处分事项已确认。' END,d.id,d.row_version,d.status
 FROM discipline_cases d JOIN account_notifications n ON n.discipline_case_id=d.id
 JOIN accounts a ON a.id=n.account_id AND a.status='ACTIVE'
 WHERE d.id=ANY($1::uuid[]) AND d.status<>'PENDING' ON CONFLICT DO NOTHING`, ids)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT a.id::text FROM accounts a WHERE a.status='ACTIVE' AND
 (a.system_role='SUPER_ADMIN' OR EXISTS(SELECT 1 FROM user_org_roles r WHERE r.account_id=a.id
 AND r.default_permissions->'rolePolicies' @> jsonb_build_array(jsonb_build_object('disciplineActions',jsonb_build_array($2::text))))
 OR EXISTS(SELECT 1 FROM user_permission_rules r WHERE r.account_id=a.id AND r.action=$2))
 AND EXISTS(SELECT 1 FROM discipline_cases d WHERE d.id=ANY($1::uuid[]) AND d.status IN ('PENDING','APPROVED'))`, ids, permission.ActionDisciplineConfirm)
	if err != nil {
		return err
	}
	accounts, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, account := range accounts {
		profile, err := loadAccessProfile(ctx, tx, account)
		if err != nil {
			return err
		}
		args := []any{account, ids}
		predicate := disciplineScopeSQL(permission.EvaluateProfile(profile), permission.ActionDisciplineConfirm, &args)
		_, err = tx.Exec(ctx, `INSERT INTO account_notifications(account_id,category,body,discipline_case_id,discipline_case_version,discipline_event_status,related_deferred_ids)
 SELECT $1,'discipline',CASE d.status WHEN 'PENDING' THEN format('待核对档位：%s。处理截止：%s（北京时间）。',coalesce(d.evidence->'tier'->>'name',d.tier_code),to_char(d.due_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI')) ELSE '处分事项已确认。' END ||
 CASE WHEN cardinality(related.ids)>0 THEN '该学生另有已延迟事项。' ELSE '' END,d.id,d.row_version,d.status,related.ids
 FROM discipline_cases d JOIN classes c ON c.id=d.responsibility_class_id
 CROSS JOIN LATERAL (SELECT array_agg(old.id ORDER BY old.created_at,old.id) AS ids FROM discipline_cases old
 WHERE d.status='PENDING' AND old.student_number=d.student_number AND old.semester_id=d.semester_id
 AND old.status='DEFERRED' AND old.responsibility_class_id=d.responsibility_class_id) related
 WHERE d.id=ANY($2::uuid[]) AND d.status IN ('PENDING','APPROVED') AND (`+predicate+`) ON CONFLICT DO NOTHING`, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func changeAndNotifyCases(ctx context.Context, tx pgx.Tx, actorID string, query string, args ...any) error {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		_, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,actor_id,action,resource_type,resource_id,after_json,reason)
 SELECT gen_random_uuid(),$1,CASE WHEN status='VOID' THEN 'DISCIPLINE_CASE_VOIDED' ELSE 'DISCIPLINE_CASE_CREATED' END,
 'discipline_case',id::text,jsonb_build_object('status',status,'rowVersion',row_version,'ruleVersion',rule_version_id,
 'reportVersion',report_version_id,'reassessmentRule',reassessment_rule_version_id,'reassessmentReport',reassessment_report_version_id,
 'previousCase',previous_case_id),coalesce(void_reason,'') FROM discipline_cases WHERE id=ANY($2::uuid[])`, actorID, ids)
		if err != nil {
			return err
		}
	}
	return notifyDisciplineCases(ctx, tx, ids)
}

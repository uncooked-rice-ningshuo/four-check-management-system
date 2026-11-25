package store

import (
	"context"
	"encoding/json"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
)

// DisciplineArchive returns final and void decisions in a stable order across pages.
func (p *Postgres) DisciplineArchive(ctx context.Context, f reporting.ArchiveFilter) (json.RawMessage, error) {
	var result json.RawMessage
	err := p.Pool.QueryRow(ctx, `
WITH filtered AS (
 SELECT d.id,d.student_number,d.group_key,d.tier_code,d.evidence->'tier'->>'name' AS tier,
        d.void_reason,d.voided_at,d.previous_case_id,d.reassessment_rule_version_id,d.reassessment_report_version_id,d.evidence,d.delay_reason,d.rule_version_id,d.report_version_id,d.status,d.accumulated_amount,d.decided_at,d.decision_source,s.name AS semester
 FROM discipline_cases d JOIN semesters s ON s.id=d.semester_id
 WHERE d.status IN ('APPROVED','VOID') AND ($1='' OR d.status=$1)
 AND strpos(d.student_number,$2)>0 AND ($3='' OR d.semester_id::text=$3)
 AND ($9='' OR d.id::text=$9) AND ($4='' OR d.group_key=$4) AND ($5='' OR d.tier_code=$5)
), page AS (
 SELECT * FROM filtered ORDER BY decided_at DESC NULLS LAST,id LIMIT NULLIF($6,0) OFFSET $7
)
SELECT jsonb_build_object('total',(SELECT count(*) FROM filtered),'page',$8::int,'pageSize',$6::int,
'items',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'studentNumber',student_number,
'semester',semester,'group',group_key,'tier',coalesce(tier,tier_code),'status',status,'amount',accumulated_amount,
'decidedAt',decided_at,'decisionSource',decision_source) || CASE WHEN $9<>'' THEN jsonb_build_object('clauseReference',evidence->'tier'->>'clause_reference','authorityNumber',evidence->'tier'->>'authority_number','sources',evidence->'sources','reason',delay_reason,'voidReason',void_reason,'voidedAt',voided_at,'previousCaseId',previous_case_id,'reassessmentRuleVersionId',reassessment_rule_version_id,'reassessmentReportVersionId',reassessment_report_version_id,'ruleVersion',rule_version_id,'reportVersion',report_version_id) ELSE '{}'::jsonb END ORDER BY decided_at DESC NULLS LAST,id) FROM page),'[]'))
`, f.Status, f.Student, f.SemesterID, f.Group, f.Tier, f.PageSize, (f.Page-1)*f.PageSize, f.Page, f.CaseID).Scan(&result)
	return result, err
}

func (p *Postgres) RecordDisciplineArchiveExport(ctx context.Context, rows int, meta audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = insertAuditTx(ctx, tx, meta, audit.Entry{Action: "DISCIPLINE_ARCHIVE_EXPORTED", ResourceType: "discipline_archive", ResourceID: "filtered", After: map[string]int{"rows": rows}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

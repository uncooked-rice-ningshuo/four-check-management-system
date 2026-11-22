package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reportoutput"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ListDocumentTemplates(ctx context.Context) ([]reportoutput.DisciplineTemplate, error) {
	rows, err := p.Pool.Query(ctx, `SELECT g.version_id::text,g.group_key,g.name,k.code,k.name,coalesce(t.revision,0),coalesce(t.id::text,''),t.document,coalesce(to_jsonb(g)->>'violation_name',''),g.result_unit,trim_scale(k.lower_bound)::text
 FROM discipline_rule_current c JOIN discipline_rule_groups g ON g.version_id=c.version_id
 JOIN discipline_rule_tiers k ON k.group_id=g.id
 LEFT JOIN discipline_tier_template_bindings b ON (b.rule_id,b.group_key,b.tier_code)=(g.version_id,g.group_key,k.code)
 LEFT JOIN discipline_document_templates t ON t.id=b.template_id ORDER BY g.sort_order,k.sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []reportoutput.DisciplineTemplate{}
	for rows.Next() {
		var t reportoutput.DisciplineTemplate
		if err := rows.Scan(&t.RuleID, &t.GroupKey, &t.Name, &t.TierCode, &t.TierName, &t.Revision, &t.TemplateID, &t.Document, &t.ViolationName, &t.Unit, &t.ExampleAmount); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// Historical group templates can seed a draft but never act as a tier binding.
func (p *Postgres) ListDocumentTemplateSources(ctx context.Context) ([]reportoutput.DisciplineTemplate, error) {
	rows, err := p.Pool.Query(ctx, `SELECT t.source_rule_id::text,t.group_key,g.name,coalesce(t.tier_code,''),coalesce(k.name,'原累计项模板'),t.revision,t.id::text,t.document
 FROM discipline_document_templates t JOIN discipline_rule_groups g ON (g.version_id,g.group_key)=(t.source_rule_id,t.group_key)
 LEFT JOIN discipline_rule_tiers k ON k.group_id=g.id AND k.code=t.tier_code ORDER BY g.name,k.sort_order,t.revision DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (reportoutput.DisciplineTemplate, error) {
		var t reportoutput.DisciplineTemplate
		err := row.Scan(&t.RuleID, &t.GroupKey, &t.Name, &t.TierCode, &t.TierName, &t.Revision, &t.TemplateID, &t.Document)
		return t, err
	})
}
func (p *Postgres) SaveDocumentTemplate(ctx context.Context, t reportoutput.DisciplineTemplate, meta audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var previous int
	err = tx.QueryRow(ctx, `SELECT 1 FROM discipline_rule_current c JOIN discipline_rule_groups g ON g.version_id=c.version_id JOIN discipline_rule_tiers k ON k.group_id=g.id WHERE c.version_id=$1 AND g.group_key=$2 AND k.code=$3 FOR UPDATE OF c`, t.RuleID, t.GroupKey, t.TierCode).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return reportoutput.ErrConfigConflict
	}
	if err != nil {
		return err
	}
	// Read the binding after acquiring the current-rule lock so concurrent editors see the committed revision.
	err = tx.QueryRow(ctx, `SELECT coalesce((SELECT t.revision FROM discipline_tier_template_bindings b JOIN discipline_document_templates t ON t.id=b.template_id WHERE b.rule_id=$1 AND b.group_key=$2 AND b.tier_code=$3),0)`, t.RuleID, t.GroupKey, t.TierCode).Scan(&previous)
	if err != nil {
		return err
	}
	if previous != t.Revision {
		return reportoutput.ErrConfigConflict
	}
	if t.SourceTemplateID != "" {
		var source json.RawMessage
		err := tx.QueryRow(ctx, `SELECT document FROM discipline_document_templates WHERE id=$1`, t.SourceTemplateID).Scan(&source)
		if errors.Is(err, pgx.ErrNoRows) {
			return reportoutput.ErrConfigConflict
		}
		if err != nil {
			return err
		}
		if err := reportoutput.CheckTemplateAdaptation(source, t.Document); err != nil {
			return err
		}
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO discipline_document_templates(group_key,source_rule_id,revision,document,created_by,tier_code,source_template_id) SELECT $1,$2,coalesce(max(revision),0)+1,$3,$4,$5,nullif($6,'')::uuid FROM discipline_document_templates WHERE group_key=$1 AND tier_code=$5 RETURNING id::text`, t.GroupKey, t.RuleID, t.Document, meta.ActorID, t.TierCode, t.SourceTemplateID).Scan(&id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO discipline_tier_template_bindings(rule_id,group_key,tier_code,template_id) VALUES($1,$2,$4,$3) ON CONFLICT(rule_id,group_key,tier_code) DO UPDATE SET template_id=excluded.template_id`, t.RuleID, t.GroupKey, id, t.TierCode)
	if err != nil {
		return err
	}
	if err = insertAuditTx(ctx, tx, meta, audit.Entry{Action: "DISCIPLINE_DOCUMENT_TEMPLATE_SAVED", ResourceType: "discipline_document_template", ResourceID: id, After: map[string]string{"group": t.GroupKey, "rule": t.RuleID}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ProcessDisciplineDocument(ctx context.Context, id string, input reportoutput.DocumentRequest, meta audit.Metadata, export bool) (reportoutput.DocumentPreview, []byte, error) {
	var result reportoutput.DocumentPreview
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, nil, err
	}
	defer tx.Rollback(ctx)
	var ruleID, groupKey, tierCode string
	err = tx.QueryRow(ctx, `SELECT b.template_id::text,d.rule_version_id::text,d.group_key,d.tier_code,jsonb_strip_nulls(jsonb_build_object(
 'discipline.studentName',d.evidence->'student'->>'name','discipline.studentNumber',d.student_number,'discipline.class',d.evidence->'student'->>'class',
 'discipline.sex',d.evidence->'student'->>'sex','discipline.enrollmentYear',d.evidence->'student'->>'grade','discipline.major',d.evidence->'student'->>'major','discipline.classNumber',d.evidence->'student'->>'classNumber',
 'discipline.schoolYear',y.name,'discipline.semester',CASE s.code WHEN 'FIRST' THEN '第一学期' WHEN 'SECOND' THEN '第二学期' ELSE s.name END,'discipline.violationCount',(SELECT nullif(count(DISTINCT r->>'record'),0)::text FROM jsonb_array_elements(d.evidence->'sources') r),
 'discipline.violationName',d.evidence->>'violationName','discipline.authorityNumber',d.evidence->'tier'->>'authority_number','discipline.clauseReference',d.evidence->'tier'->>'clause_reference',
 'discipline.group',g.name,'discipline.amount',d.accumulated_amount::text,'discipline.unit',g.result_unit,'discipline.tier',coalesce(d.evidence->'tier'->>'name',k.name),'discipline.clause',d.evidence->'tier'->>'clause','discipline.decidedAt',to_char(d.decided_at AT TIME ZONE 'Asia/Shanghai','YYYY年MM月DD日')))
 FROM discipline_cases d JOIN semesters s ON s.id=d.semester_id JOIN academic_years y ON y.id=s.academic_year_id
 JOIN discipline_rule_groups g ON (g.version_id,g.group_key)=(d.rule_version_id,d.group_key)
 JOIN discipline_rule_tiers k ON k.group_id=g.id AND k.code=d.tier_code
 JOIN discipline_tier_template_bindings b ON (b.rule_id,b.group_key,b.tier_code)=(g.version_id,g.group_key,d.tier_code)
 WHERE d.id=$1 AND d.status='APPROVED' FOR SHARE OF d,b`, id).Scan(&result.TemplateID, &ruleID, &groupKey, &tierCode, &result.Fields)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil, reportoutput.ErrNotFound
	}
	if err != nil {
		return result, nil, err
	}

	if input.TemplateID != "" {
		result.TemplateID = input.TemplateID
	}
	rows, err := tx.Query(ctx, `SELECT id::text,revision,CASE WHEN id=$4 THEN document END FROM discipline_document_templates t JOIN discipline_tier_template_history h ON h.template_id=t.id WHERE h.group_key=$1 AND h.tier_code=$2 AND h.rule_id=$3 ORDER BY revision DESC`, groupKey, tierCode, ruleID, result.TemplateID)
	if err != nil {
		return result, nil, err
	}
	for rows.Next() {
		var e reportoutput.DocumentEdition
		var doc []byte
		if err = rows.Scan(&e.ID, &e.Revision, &doc); err != nil {
			rows.Close()
			return result, nil, err
		}
		result.Editions = append(result.Editions, e)
		if doc != nil {
			result.Document = doc
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, nil, err
	}
	if len(result.Document) == 0 {
		return result, nil, reportoutput.ErrNotFound
	}
	now := time.Now()
	if err = result.Fill(input, now); err != nil {
		return result, nil, err
	}
	if !export {
		return result, nil, tx.Commit(ctx)
	}
	if len(result.Missing) > 0 {
		return result, nil, reportoutput.ErrInvalidSelection
	}
	data, err := reportoutput.RenderDisciplineDocument(result.Document, result.Fields, now)
	if err != nil {
		return result, nil, err
	}
	if err = reportoutput.ValidateGeneratedFile("WORD", data); err != nil {
		return result, nil, err
	}
	err = insertAuditTx(ctx, tx, meta, audit.Entry{Action: "DISCIPLINE_DOCUMENT_EXPORTED", ResourceType: "discipline_case", ResourceID: id, After: map[string]string{"templateId": result.TemplateID, "ruleId": ruleID, "generatedAt": now.UTC().Format(time.RFC3339), "decisionDate": input.DecisionDate, "issueDate": input.IssueDate, "issueNumber": input.IssueNumber}})
	if err != nil {
		return result, nil, err
	}
	return result, data, tx.Commit(ctx)
}

package store

import (
	"context"
	_ "embed"
	"encoding/json"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
)

//go:embed semester_discipline_amounts.sql
var semesterDisciplineAmountsSQL string

//go:embed generate_discipline_cases.sql
var generateDisciplineCasesSQL string

//go:embed semester_ledger.sql
var semesterLedgerSQL string

// SemesterLedger aggregates frozen records and uses one statement snapshot for rows and rules.
func (p *Postgres) SemesterLedger(ctx context.Context, semester string, filter reporting.LedgerFilter, page, size int) (json.RawMessage, error) {
	var result json.RawMessage
	err := p.Pool.QueryRow(ctx, semesterDisciplineAmountsSQL+semesterLedgerSQL, semester, filter, size, (page-1)*size, page).Scan(&result)
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

func (p *Postgres) SemesterLedgerDetail(ctx context.Context, f reporting.LedgerDetailFilter) (json.RawMessage, error) {
	tierOptions := "'[]'::jsonb"
	query := semesterDisciplineAmountsSQL + `, filtered AS (
 SELECT jsonb_build_object('id',inspection_record_id,'date',coalesce(date,''),'week',week_no,'version',version_no,'code',code,'amount',amount) AS item,
 week_no AS ord,inspection_record_id AS id FROM amounts a WHERE number=$2 AND ($3='' OR code=$3) AND ($4=0 OR week_no=$4)
 AND ($5='' OR EXISTS(SELECT 1 FROM discipline_rule_current cur JOIN discipline_rule_groups g ON g.version_id=cur.version_id
 JOIN discipline_rule_group_terms t ON t.group_id=g.id JOIN inspection_types i ON i.id=t.inspection_type_id WHERE g.group_key=$5 AND i.code=a.code)))`
	args := []any{f.Semester, f.Student, f.Code, f.Week, f.Group}
	if f.Kind == "cases" {
		tierOptions = `(SELECT coalesce(jsonb_agg(name ORDER BY name),'[]') FROM
        (SELECT DISTINCT evidence->'tier'->>'name' AS name FROM discipline_cases
         WHERE semester_id=$1::uuid AND student_number=$2 AND ($3='' OR group_key=$3)) choices)`
		query = `WITH filtered AS (SELECT jsonb_build_object('id',d.id,'groupKey',group_key,'tier',evidence->'tier'->>'name','amount',accumulated_amount,
 'status',d.status,'week',w.week_no,'version',v.version_no,'dueAt',due_at) AS item,d.created_at AS ord,d.id FROM discipline_cases d
 JOIN weekly_report_versions v ON v.id=d.report_version_id JOIN weekly_reports r ON r.id=v.report_id JOIN semester_weeks w ON w.id=r.week_id
 WHERE d.semester_id=$1::uuid AND student_number=$2 AND ($3='' OR group_key=$3) AND ($4='' OR d.status=$4) AND ($5='' OR evidence->'tier'->>'name'=$5))`
		args = []any{f.Semester, f.Student, f.Group, f.Status, f.Tier}
	}
	query += `, sliced AS (SELECT * FROM filtered ORDER BY ord DESC,id LIMIT $6 OFFSET $7)
 SELECT jsonb_build_object('tiers',` + tierOptions + `,'total',(SELECT count(*) FROM filtered),'items',coalesce((SELECT jsonb_agg(item ORDER BY ord DESC,id) FROM sliced),'[]'),'page',$8::int,'pageSize',$6::int)`
	var raw json.RawMessage
	err := p.Pool.QueryRow(ctx, query, append(args, f.Size, (f.Page-1)*f.Size, f.Page)...).Scan(&raw)
	return raw, err
}

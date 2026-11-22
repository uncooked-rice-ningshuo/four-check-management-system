package store

// Use the same current rule on both sides of a report correction. Only a
// threshold actually lost in this revision may invalidate a prior decision.
const reachedTiersSQL = `
 SELECT a.number,a.group_key,t.code FROM group_amounts a
 JOIN discipline_rule_tiers t ON t.group_id=a.group_id AND a.amount>=t.lower_bound`

const voidLostTiersSQL = `, lost AS (
 SELECT number,group_key,code FROM previous_reached_tiers EXCEPT ` + reachedTiersSQL + `
)
UPDATE discipline_cases d SET status='VOID',row_version=d.row_version+1,
 voided_at=statement_timestamp(),void_reason='定稿更正后未达到该档位',
 reassessment_report_version_id=$2::uuid,reassessment_rule_version_id=cur.version_id
FROM lost l CROSS JOIN discipline_rule_current cur
WHERE d.semester_id=$1::uuid AND d.student_number=l.number AND d.group_key=l.group_key
 AND d.tier_code=l.code AND d.status<>'VOID'`

const activeTierConflictSQL = ` ON CONFLICT (semester_id,student_number,group_key,tier_code) WHERE status <> 'VOID' DO NOTHING`

// Compare fields that determine the accumulated amount and the disposition.
// Version-specific row IDs and display ordering do not replace pending cases.
const changedRuleCasesSQL = `CREATE TEMP TABLE changed_rule_cases ON COMMIT DROP AS
WITH shapes AS (
 SELECT g.version_id,g.group_key,t.code,
 jsonb_build_array(g.result_unit,t.name,t.lower_bound,t.upper_bound,t.clause,t.teacher_confirmation_required,
 (SELECT jsonb_agg(jsonb_build_array(x.inspection_type_id,x.operator,x.conversion_factor,x.source_unit)
 ORDER BY x.inspection_type_id) FROM discipline_rule_group_terms x WHERE x.group_id=g.id)) AS shape
 FROM discipline_rule_groups g JOIN discipline_rule_tiers t ON t.group_id=g.id
 WHERE g.version_id IN ($1::uuid,$2::uuid)
)
SELECT d.id,d.semester_id,d.student_number,d.group_key,d.tier_code
FROM discipline_cases d JOIN shapes a ON a.version_id=$1 AND a.group_key=d.group_key AND a.code=d.tier_code
LEFT JOIN shapes b ON b.version_id=$2 AND b.group_key=a.group_key AND b.code=a.code
WHERE d.status IN ('PENDING','DEFERRED') AND a.shape IS DISTINCT FROM b.shape`

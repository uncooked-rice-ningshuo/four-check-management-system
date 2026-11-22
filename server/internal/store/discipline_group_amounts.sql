, group_amounts AS (
 SELECT a.number,g.id AS group_id,g.group_key,g.version_id,
        sum(a.amount*t.conversion_factor*CASE t.operator WHEN '-' THEN -1 ELSE 1 END) AS amount,
        jsonb_agg(jsonb_build_object('record',a.inspection_record_id,'week',a.week_no,
          'version',a.version_no,'code',a.code,'amount',a.amount)
          ORDER BY a.week_no,a.inspection_record_id) AS sources
 FROM amounts a
 JOIN inspection_types i ON i.code=a.code
 JOIN discipline_rule_group_terms t ON t.inspection_type_id=i.id
 JOIN discipline_rule_current cur ON cur.version_id=t.version_id
 JOIN discipline_rule_groups g ON g.id=t.group_id
 GROUP BY a.number,g.id HAVING count(*)=count(a.amount)
)

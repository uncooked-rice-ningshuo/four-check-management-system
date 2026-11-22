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
), student_classes AS (
 SELECT DISTINCT ON (number) number,nullif(class_id,'')::uuid AS class_id
 FROM amounts ORDER BY number,week_no DESC,version_no DESC,class_id,name
), entered AS (SELECT statement_timestamp() AS at)
INSERT INTO discipline_cases(semester_id,student_number,group_key,tier_code,report_version_id,
 rule_version_id,accumulated_amount,evidence,status,created_at,due_at,responsibility_class_id,decision_source,decided_at)
SELECT $1::uuid,a.number,a.group_key,t.code,$2::uuid,a.version_id,a.amount,
 jsonb_build_object('tier',to_jsonb(t),'sources',a.sources),
 CASE WHEN t.teacher_confirmation_required THEN 'PENDING' ELSE 'APPROVED' END,
 entered.at,CASE WHEN t.teacher_confirmation_required THEN entered.at+interval '48 hours' END,sc.class_id,
 CASE WHEN NOT t.teacher_confirmation_required THEN 'RULE' END,
 CASE WHEN NOT t.teacher_confirmation_required THEN entered.at END
FROM group_amounts a JOIN discipline_rule_tiers t ON t.group_id=a.group_id AND a.amount>=t.lower_bound
JOIN student_classes sc ON sc.number=a.number
CROSS JOIN entered
ON CONFLICT (semester_id,student_number,group_key,tier_code) DO NOTHING

, student_classes AS (
 SELECT DISTINCT ON (number) number,name,grade,major,class,nullif(class_id,'')::uuid AS class_id
 FROM amounts ORDER BY number,week_no DESC,version_no DESC,class_id,name
), entered AS (SELECT statement_timestamp() AS at)
INSERT INTO discipline_cases(semester_id,student_number,group_key,tier_code,report_version_id,
 rule_version_id,accumulated_amount,evidence,status,created_at,due_at,responsibility_class_id,decision_source,decided_at,previous_case_id)
SELECT $1::uuid,a.number,a.group_key,t.code,$2::uuid,a.version_id,a.amount,
 jsonb_build_object('tier',to_jsonb(t),'sources',a.sources,'student',jsonb_build_object('name',sc.name,'grade',sc.grade,'major',sc.major,'class',sc.class)),
 CASE WHEN t.teacher_confirmation_required THEN 'PENDING' ELSE 'APPROVED' END,
 entered.at,CASE WHEN t.teacher_confirmation_required THEN entered.at+interval '48 hours' END,sc.class_id,
 CASE WHEN NOT t.teacher_confirmation_required THEN 'RULE' END,
 CASE WHEN NOT t.teacher_confirmation_required THEN entered.at END,
 (SELECT old.id FROM discipline_cases old
  WHERE old.semester_id=$1::uuid AND old.student_number=a.number AND old.group_key=a.group_key
    AND old.tier_code=t.code AND old.status='VOID'
  ORDER BY old.voided_at DESC NULLS LAST,old.created_at DESC,old.id LIMIT 1)
FROM group_amounts a JOIN discipline_rule_tiers t ON t.group_id=a.group_id AND a.amount>=t.lower_bound
JOIN student_classes sc ON sc.number=a.number
CROSS JOIN entered

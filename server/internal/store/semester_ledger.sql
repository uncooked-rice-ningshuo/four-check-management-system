, identities AS (
 SELECT DISTINCT ON(number) number,name,class,grade,major,class_id FROM amounts
 ORDER BY number,week_no DESC,version_no DESC,class_id,name
), filtered AS (
 SELECT i.*,g.sort_order AS grade_order,c.major_order,c.class_order FROM identities i CROSS JOIN selected s
 LEFT JOIN academic_report_classes c ON c.academic_year_id=s.year_id AND c.class_id::text=i.class_id
 LEFT JOIN academic_report_grades g ON g.academic_year_id=s.year_id AND g.grade_id=c.grade_id
 WHERE strpos(lower(concat_ws(' ',i.number,i.name)),lower($2::jsonb->>'search'))>0
 AND ($2::jsonb->>'grade'='' OR i.grade=$2::jsonb->>'grade')
 AND ($2::jsonb->>'major'='' OR i.major=$2::jsonb->>'major')
 AND ($2::jsonb->>'class'='' OR i.class_id=$2::jsonb->>'class')
), students_page AS (
 SELECT * FROM filtered ORDER BY grade_order,major_order,class_order,class,number LIMIT nullif($3::integer,0) OFFSET $4
), totals AS (
 SELECT a.number,code,CASE WHEN count(amount)=count(*) THEN sum(amount) END AS amount
 FROM amounts a JOIN students_page s USING(number) GROUP BY a.number,code
), rules AS (
 SELECT g.id,g.group_key,g.name,g.result_unit,g.sort_order,t.inspection_type_id,t.operator,t.conversion_factor,t.inspection_type_name,t.sort_order AS term_order,i.code
 FROM discipline_rule_current cur JOIN discipline_rule_groups g ON g.version_id=cur.version_id
 JOIN discipline_rule_group_terms t ON t.group_id=g.id JOIN inspection_types i ON i.id=t.inspection_type_id
), grouped AS (
 SELECT s.number,r.id::text AS code, CASE WHEN bool_or(t.code IS NOT NULL AND t.amount IS NULL) THEN NULL
 ELSE sum(coalesce(t.amount,0)*r.conversion_factor*CASE r.operator WHEN '-' THEN -1 ELSE 1 END) END AS amount
 FROM students_page s CROSS JOIN rules r LEFT JOIN totals t ON t.number=s.number AND t.code=r.code GROUP BY s.number,r.id
), metrics AS (SELECT * FROM totals UNION ALL SELECT * FROM grouped),
student_values AS (
 SELECT number,jsonb_object_agg(code,amount) AS values FROM metrics GROUP BY number
), cols AS (
 SELECT code,name,CASE code WHEN 'ATTENDANCE_RATE' THEN '学时' ELSE '次' END AS unit,0 AS section,sort_order,'[]'::jsonb AS members,NULL::text AS group_key FROM inspection_types i
 WHERE enabled OR EXISTS(SELECT 1 FROM amounts a WHERE a.code=i.code)
 UNION ALL SELECT id::text,name,result_unit,1,sort_order,
 jsonb_agg(jsonb_build_object('code',code,'name',inspection_type_name,'factor',conversion_factor,'operator',operator) ORDER BY term_order),group_key
 FROM rules GROUP BY id,name,result_unit,sort_order,group_key
)
SELECT jsonb_build_object(
 'ruleVersion',(SELECT v.version FROM discipline_rule_current cur JOIN discipline_rule_versions v ON v.id=cur.version_id),
 'organizations',coalesce((SELECT jsonb_agg(jsonb_build_object('grade',g.name,'major',coalesce(m.name,''),'class',coalesce(c.name,''),'classId',coalesce(c.id::text,'')) ORDER BY rg.sort_order,gm.sort_order,rc.sort_order)
 FROM selected s JOIN academic_year_roster_grades rg ON rg.academic_year_id=s.year_id
 JOIN grades g ON g.id=rg.grade_id
 LEFT JOIN academic_year_roster_grade_majors gm USING(academic_year_id,grade_id)
 LEFT JOIN majors m ON m.id=gm.major_id
 LEFT JOIN academic_year_roster_classes rc ON rc.academic_year_id=gm.academic_year_id AND rc.grade_id=gm.grade_id AND rc.major_id=gm.major_id
 LEFT JOIN classes c ON c.id=rc.class_id),'[]'),
 'semesters',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'name',year||' · '||name) ORDER BY start_date DESC,id) FROM terms),'[]'),
 'weeks',coalesce((SELECT jsonb_agg(week_no ORDER BY week_no) FROM finals),'[]'),
 'unassigned',(SELECT count(*) FROM facts f WHERE NOT EXISTS (SELECT 1 FROM attributed a WHERE a.inspection_record_id=f.inspection_record_id AND a.week_no=f.week_no)),
 'columns',coalesce((SELECT jsonb_agg(to_jsonb(cols) ORDER BY section,sort_order,code) FROM cols),'[]'),
 'items',coalesce((SELECT jsonb_agg(jsonb_build_object('number',s.number,'name',s.name,'className',s.class,'grade',s.grade,
 'sources','[]'::jsonb,'values',sv.values,
 'cases',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',dc.id,'groupKey',dc.group_key,'groupName',dg.name,'tier',dc.evidence->'tier'->>'name',
 'amount',dc.accumulated_amount,'status',dc.status,'week',sw.week_no,'version',rv.version_no,'dueAt',dc.due_at)
 ORDER BY dc.created_at DESC, (dc.evidence->'tier'->>'lower_bound')::numeric DESC,dc.id),'[]')
 FROM (SELECT DISTINCT ON (group_key) * FROM discipline_cases
 WHERE semester_id=$1::uuid AND student_number=s.number ORDER BY group_key,created_at DESC,(evidence->'tier'->>'lower_bound')::numeric DESC,id) dc JOIN discipline_rule_groups dg ON dg.version_id=dc.rule_version_id AND dg.group_key=dc.group_key JOIN weekly_report_versions rv ON rv.id=dc.report_version_id
 JOIN weekly_reports wr ON wr.id=rv.report_id JOIN semester_weeks sw ON sw.id=wr.week_id
 WHERE dc.semester_id=$1::uuid AND dc.student_number=s.number))
 ORDER BY grade_order,major_order,class_order,class,number) FROM students_page s JOIN student_values sv USING(number)),'[]'),
 'total',(SELECT count(*) FROM filtered),'page',$5::integer,'pageSize',$3::integer)

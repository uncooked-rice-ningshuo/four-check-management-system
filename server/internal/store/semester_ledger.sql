-- Pick the most recent finalized version per week even while a correction is still a draft.
WITH terms AS (
 SELECT s.id,s.name,y.name AS year,s.start_date,y.id AS year_id
 FROM semesters s JOIN academic_years y ON y.id=s.academic_year_id
), selected AS (
 SELECT * FROM terms WHERE id=nullif($1,'')::uuid
), finals AS (
 SELECT DISTINCT ON (w.id) v.id,v.version_no,k.week_no,v.content_json
 FROM weekly_reports w JOIN selected s ON w.semester_id=s.id
 JOIN semester_weeks k ON k.id=w.week_id
 JOIN weekly_report_versions v ON v.report_id=w.id
 JOIN weekly_report_reviews r ON r.report_version_id=v.id AND r.action='FINALIZED'
 ORDER BY w.id,v.version_no DESC
), class_snapshots AS (
 SELECT f.id,c->>'classId' AS class_id,c->>'majorName' AS major
 FROM finals f CROSS JOIN LATERAL jsonb_array_elements(f.content_json->'indicators'->'inputs'->'classes') c
), facts AS (
 SELECT f.week_no,f.version_no,rec.included_data_json AS d,coalesce(cs.major,'') AS major,rec.source_submission_id,rec.source_revision_no,rec.inspection_record_id
 FROM finals f JOIN weekly_report_version_records rec ON rec.report_version_id=f.id
 LEFT JOIN class_snapshots cs ON cs.id=f.id AND cs.class_id=rec.included_data_json->>'classId'
 WHERE rec.included_data_json->>'typeCode' NOT IN ('CLASSROOM_HYGIENE','DORMITORY_HYGIENE','PUBLIC_DORMITORY_HYGIENE')
 OR rec.included_data_json->'details'->>'result'='UNQUALIFIED'
), attributed AS (
 -- Collective findings identify students through the immutable submission revision.
 SELECT f.*,d->>'studentNumber' AS student_number,d->>'studentName' AS student_name FROM facts f WHERE nullif(d->>'studentNumber','') IS NOT NULL
 UNION ALL
 SELECT f.*,person->>'studentNumber',person->>'label' FROM facts f
 JOIN department_submission_revisions rev ON rev.submission_id=f.source_submission_id AND rev.revision_no=f.source_revision_no
 CROSS JOIN LATERAL jsonb_array_elements(rev.records_snapshot) snap
 CROSS JOIN LATERAL jsonb_array_elements(snap->'responsibleStudents') person
 WHERE nullif(f.d->>'studentNumber','') IS NULL AND snap->'record'->>'id'=f.inspection_record_id::text AND nullif(person->>'studentNumber','') IS NOT NULL
), amounts AS (
 SELECT major,student_number AS number,student_name AS name,d->>'className' AS class,
 d->>'grade' AS grade,d->>'classId' AS class_id,d->>'typeCode' AS code,week_no,version_no,d->'details'->>'date' AS date,inspection_record_id,
 CASE WHEN d->>'typeCode'='ATTENDANCE_RATE' THEN (d->'details'->>'penaltyHours')::numeric
 WHEN d->>'typeCode' IN ('EVENING_SELF_STUDY','UNATTENDED_CLASSROOM') THEN (d->'details'->>'penaltyCount')::numeric WHEN d->>'typeCode'='MORNING_ATTENDANCE' THEN (d->'details'->>'absenceCount')::numeric ELSE 1 END AS amount
 FROM attributed
), identities AS (
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
 SELECT g.id,g.name,g.result_unit,g.sort_order,t.inspection_type_id,t.operator,t.conversion_factor,t.inspection_type_name,t.sort_order AS term_order,i.code
 FROM discipline_rule_current cur JOIN discipline_rule_groups g ON g.version_id=cur.version_id
 JOIN discipline_rule_group_terms t ON t.group_id=g.id JOIN inspection_types i ON i.id=t.inspection_type_id
), grouped AS (
 SELECT s.number,r.id::text AS code, CASE WHEN bool_or(t.code IS NOT NULL AND t.amount IS NULL) THEN NULL
 ELSE sum(coalesce(t.amount,0)*r.conversion_factor*CASE r.operator WHEN '-' THEN -1 ELSE 1 END) END AS amount
 FROM students_page s CROSS JOIN rules r LEFT JOIN totals t ON t.number=s.number AND t.code=r.code GROUP BY s.number,r.id
), metrics AS (SELECT * FROM totals UNION ALL SELECT * FROM grouped),
student_values AS (
 SELECT number,jsonb_object_agg(code,amount) AS values FROM metrics GROUP BY number
), student_sources AS (
 SELECT a.number,jsonb_agg(jsonb_build_object('week',a.week_no,'version',a.version_no,'code',a.code,
 'amount',a.amount,'id',a.inspection_record_id,'date',coalesce(a.date,'')) ORDER BY a.week_no,a.date,a.code,a.inspection_record_id) AS sources
 FROM amounts a JOIN students_page s USING(number) WHERE $3::integer>0 GROUP BY a.number
), cols AS (
 SELECT code,name,CASE code WHEN 'ATTENDANCE_RATE' THEN '学时' ELSE '次' END AS unit,0 AS section,sort_order,'[]'::jsonb AS members FROM inspection_types i
 WHERE enabled OR EXISTS(SELECT 1 FROM amounts a WHERE a.code=i.code)
 UNION ALL SELECT id::text,name,result_unit,1,sort_order,
 jsonb_agg(jsonb_build_object('code',code,'name',inspection_type_name,'factor',conversion_factor,'operator',operator) ORDER BY term_order)
 FROM rules GROUP BY id,name,result_unit,sort_order
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
 'sources',ss.sources,'values',sv.values)
 ORDER BY grade_order,major_order,class_order,class,number) FROM students_page s JOIN student_values sv USING(number) LEFT JOIN student_sources ss USING(number)),'[]'),
 'total',(SELECT count(*) FROM filtered),'page',$5::integer,'pageSize',$3::integer)

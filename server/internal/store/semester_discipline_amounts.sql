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
)

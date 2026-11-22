-- Rebuild only the lookup key for active records with a known date and section.
WITH sections AS (
    SELECT r.id, r.semester_id, r.week_id, r.inspection_type_id, r.student_id,
           COALESCE(r.inspection_date, (r.occurred_at AT TIME ZONE 'Asia/Shanghai')::date) AS business_date,
           regexp_match(replace(trim(d.period_text), ' ', ''),
               '^第?(10|[1-9])(?:[-—－](10|[1-9]))?节?$') AS bounds
    FROM inspection_records r
    JOIN inspection_types t ON t.id = r.inspection_type_id AND t.code = 'UNATTENDED_CLASSROOM'
    JOIN attendance_details d ON d.record_id = r.id
    WHERE r.deleted_at IS NULL AND r.student_id IS NOT NULL
), keys AS (
    SELECT id, encode(sha256(convert_to(concat_ws(chr(31), semester_id, week_id, inspection_type_id,
               student_id, to_char(business_date, 'YYYY-MM-DD'), bounds[1] || '-' || COALESCE(bounds[2], bounds[1])),
               'UTF8')), 'hex') AS daily_key
    FROM sections
    WHERE business_date IS NOT NULL AND bounds[1]::integer <= COALESCE(bounds[2], bounds[1])::integer
)
UPDATE inspection_records r SET dedup_key = keys.daily_key
FROM keys WHERE r.id = keys.id AND r.dedup_key <> keys.daily_key;

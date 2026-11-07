-- Align new 无机课堂 identities with the confirmed weekly key:
-- semester + week + inspection type + student + normalized course section.
-- Legacy rows without a section keep their existing immutable key.

DO $$
BEGIN
    IF EXISTS (
        WITH source AS (
            SELECT r.id,
                   r.semester_id::text AS semester_id,
                   r.week_id::text AS week_id,
                   r.inspection_type_id::text AS inspection_type_id,
                   r.student_id::text AS student_id,
                   replace(replace(replace(replace(replace(trim(d.period_text), ' ', ''), '第', ''), '节', ''), '—', '-'), '－', '-') AS period_text
            FROM inspection_records r
            JOIN attendance_details d ON d.record_id = r.id
            JOIN inspection_types t ON t.id = r.inspection_type_id AND t.code = 'UNATTENDED_CLASSROOM'
            WHERE r.deleted_at IS NULL AND d.period_text <> '' AND r.student_id IS NOT NULL
        ), parsed AS (
            SELECT *,
                   split_part(period_text, '-', 1)::integer AS period_start,
                   CASE WHEN strpos(period_text, '-') > 0 THEN split_part(period_text, '-', 2)::integer
                        ELSE split_part(period_text, '-', 1)::integer END AS period_end
            FROM source
            WHERE period_text ~ '^(10|[1-9])(-(10|[1-9]))?$'
        ), candidates AS (
            SELECT id,
                   encode(sha256(convert_to(concat_ws(chr(31), semester_id, week_id, inspection_type_id,
                       student_id, period_start::text || '-' || period_end::text), 'UTF8')), 'hex') AS new_key
            FROM parsed
            WHERE period_start <= period_end
        )
        SELECT new_key FROM candidates GROUP BY new_key HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION '041_unattended_student_period_dedup found existing duplicate student/period records; review them before retrying';
    END IF;
END $$;

WITH source AS (
    SELECT r.id,
           r.semester_id::text AS semester_id,
           r.week_id::text AS week_id,
           r.inspection_type_id::text AS inspection_type_id,
           r.student_id::text AS student_id,
           replace(replace(replace(replace(replace(trim(d.period_text), ' ', ''), '第', ''), '节', ''), '—', '-'), '－', '-') AS period_text
    FROM inspection_records r
    JOIN attendance_details d ON d.record_id = r.id
    JOIN inspection_types t ON t.id = r.inspection_type_id AND t.code = 'UNATTENDED_CLASSROOM'
    WHERE r.deleted_at IS NULL AND d.period_text <> '' AND r.student_id IS NOT NULL
), parsed AS (
    SELECT *,
           split_part(period_text, '-', 1)::integer AS period_start,
           CASE WHEN strpos(period_text, '-') > 0 THEN split_part(period_text, '-', 2)::integer
                ELSE split_part(period_text, '-', 1)::integer END AS period_end
    FROM source
    WHERE period_text ~ '^(10|[1-9])(-(10|[1-9]))?$'
), candidates AS (
    SELECT id,
           encode(sha256(convert_to(concat_ws(chr(31), semester_id, week_id, inspection_type_id,
               student_id, period_start::text || '-' || period_end::text), 'UTF8')), 'hex') AS new_key
    FROM parsed
    WHERE period_start <= period_end
)
UPDATE inspection_records r
SET dedup_key = candidates.new_key
FROM candidates
WHERE r.id = candidates.id AND r.dedup_key <> candidates.new_key;

-- Extend daily inspection contracts without rewriting business fields or frozen report snapshots.

ALTER TABLE inspection_records
    ADD COLUMN contract_version smallint NOT NULL DEFAULT 2 CHECK (contract_version IN (1, 2)),
    ADD COLUMN dedup_policy smallint NOT NULL DEFAULT 0 CHECK (dedup_policy IN (0, 1, 2, 3)),
    ADD COLUMN dedup_class_id uuid,
    ADD COLUMN dedup_student_id uuid,
    ADD COLUMN dedup_object_id uuid,
    ADD COLUMN dedup_date date,
    ADD COLUMN dedup_period_start smallint,
    ADD COLUMN dedup_period_end smallint;

UPDATE inspection_records SET contract_version = 1;

UPDATE inspection_records r
SET dedup_policy = CASE t.code
        WHEN 'ATTENDANCE_RATE' THEN 1
        WHEN 'MORNING_ATTENDANCE' THEN 2
        WHEN 'LATE_RETURN' THEN 2
        WHEN 'EVENING_SELF_STUDY' THEN 2
        WHEN 'DORMITORY_HYGIENE' THEN 3
        WHEN 'CLASSROOM_HYGIENE' THEN 3
        WHEN 'PUBLIC_DORMITORY_HYGIENE' THEN 3
        ELSE 0
    END,
    dedup_class_id = r.class_id,
    dedup_student_id = r.student_id,
    dedup_date = COALESCE(r.inspection_date, (r.occurred_at AT TIME ZONE 'Asia/Shanghai')::date)
FROM inspection_types t
WHERE t.id = r.inspection_type_id;

-- Personal hygiene responsibility was historically stored in relation tables.
-- Only single-person/single-class rows receive dimensions; multi-person history remains intact.
UPDATE inspection_records r
SET dedup_class_id = (SELECT class_id FROM inspection_responsibility_classes WHERE record_id = r.id LIMIT 1),
    dedup_student_id = (SELECT student_id FROM inspection_responsibility_students WHERE record_id = r.id LIMIT 1)
WHERE EXISTS (SELECT 1 FROM inspection_types t WHERE t.id = r.inspection_type_id
              AND t.code IN ('DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'))
  AND (SELECT count(*) FROM inspection_responsibility_classes WHERE record_id = r.id) = 1
  AND (SELECT count(*) FROM inspection_responsibility_students WHERE record_id = r.id) = 1;

UPDATE inspection_records r
SET dedup_object_id = d.dormitory_id
FROM dorm_hygiene_details d, inspection_types t
WHERE d.record_id = r.id AND t.id = r.inspection_type_id AND t.code = 'DORMITORY_HYGIENE';

UPDATE inspection_records r
SET dedup_object_id = d.classroom_id
FROM classroom_hygiene_details d, inspection_types t
WHERE d.record_id = r.id AND t.id = r.inspection_type_id AND t.code = 'CLASSROOM_HYGIENE';

UPDATE inspection_records r
SET dedup_object_id = (SELECT dormitory_id FROM inspection_responsibility_dormitories WHERE record_id = r.id LIMIT 1)
WHERE EXISTS (SELECT 1 FROM inspection_types t WHERE t.id = r.inspection_type_id AND t.code = 'PUBLIC_DORMITORY_HYGIENE')
  AND (SELECT count(*) FROM inspection_responsibility_dormitories WHERE record_id = r.id) = 1;

UPDATE inspection_records r
SET dedup_period_start = split_part(regexp_replace(translate(replace(a.period_text, ' ', ''), '—－', '--'), '[第节]', '', 'g'), '-', 1)::smallint,
    dedup_period_end = COALESCE(NULLIF(split_part(regexp_replace(translate(replace(a.period_text, ' ', ''), '—－', '--'), '[第节]', '', 'g'), '-', 2), '')::smallint,
                                split_part(regexp_replace(translate(replace(a.period_text, ' ', ''), '—－', '--'), '[第节]', '', 'g'), '-', 1)::smallint)
FROM attendance_details a, inspection_types t
WHERE a.record_id = r.id AND t.id = r.inspection_type_id AND t.code = 'ATTENDANCE_RATE'
  AND regexp_replace(translate(replace(a.period_text, ' ', ''), '—－', '--'), '[第节]', '', 'g') ~ '^(10|[1-9])(-((10)|([1-9])))?$';

-- Fail safely if existing active records collide under the new business keys.
-- No user-entered record, status, audit entry or report snapshot is changed by this check.
DO $$
DECLARE conflicts bigint;
BEGIN
    SELECT count(*) INTO conflicts FROM (
        SELECT 1
        FROM inspection_records
        WHERE deleted_at IS NULL AND dedup_policy = 1
          AND dedup_class_id IS NOT NULL AND dedup_student_id IS NOT NULL AND dedup_date IS NOT NULL
          AND dedup_period_start IS NOT NULL AND dedup_period_end IS NOT NULL
        GROUP BY semester_id, week_id, inspection_type_id, dedup_class_id, dedup_student_id,
                 dedup_date, dedup_period_start, dedup_period_end
        HAVING count(*) > 1
        UNION ALL
        SELECT 1
        FROM inspection_records
        WHERE deleted_at IS NULL AND dedup_policy = 2
          AND dedup_class_id IS NOT NULL AND dedup_student_id IS NOT NULL AND dedup_date IS NOT NULL
        GROUP BY semester_id, week_id, inspection_type_id, dedup_class_id, dedup_student_id, dedup_date
        HAVING count(*) > 1
        UNION ALL
        SELECT 1
        FROM inspection_records
        WHERE deleted_at IS NULL AND dedup_policy = 3
          AND dedup_object_id IS NOT NULL AND dedup_date IS NOT NULL
        GROUP BY semester_id, week_id, inspection_type_id, dedup_object_id, dedup_date
        HAVING count(*) > 1
    ) duplicates;

    IF conflicts > 0 THEN
        RAISE EXCEPTION 'migration 037 stopped: % active record groups conflict with the new daily duplicate rules; resolve them in a reviewed data correction before retrying', conflicts;
    END IF;
END;
$$;

-- PostgreSQL unique indexes are the final race guard for manual writes and import confirmation.
CREATE UNIQUE INDEX uq_inspection_records_daily_attendance
    ON inspection_records(semester_id, week_id, inspection_type_id, dedup_class_id, dedup_student_id,
                         dedup_date, dedup_period_start, dedup_period_end)
    WHERE deleted_at IS NULL AND dedup_policy = 1 AND dedup_class_id IS NOT NULL
      AND dedup_student_id IS NOT NULL AND dedup_date IS NOT NULL
      AND dedup_period_start IS NOT NULL AND dedup_period_end IS NOT NULL;
CREATE UNIQUE INDEX uq_inspection_records_daily_student
    ON inspection_records(semester_id, week_id, inspection_type_id, dedup_class_id, dedup_student_id, dedup_date)
    WHERE deleted_at IS NULL AND dedup_policy = 2 AND dedup_class_id IS NOT NULL
      AND dedup_student_id IS NOT NULL AND dedup_date IS NOT NULL;
CREATE UNIQUE INDEX uq_inspection_records_daily_hygiene
    ON inspection_records(semester_id, week_id, inspection_type_id, dedup_object_id, dedup_date)
    WHERE deleted_at IS NULL AND dedup_policy = 3 AND dedup_object_id IS NOT NULL AND dedup_date IS NOT NULL;

-- New writes use one responsible student; old associations remain readable and untouched.
CREATE OR REPLACE FUNCTION check_hygiene_has_responsibility() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_id uuid;
    current_version smallint;
    class_count integer;
    student_count integer;
    dormitory_count integer;
    type_code text;
BEGIN
    IF TG_OP = 'DELETE' THEN target_id := OLD.record_id; ELSE target_id := NEW.record_id; END IF;
    SELECT contract_version, t.code INTO current_version, type_code
    FROM inspection_records r JOIN inspection_types t ON t.id = r.inspection_type_id
    WHERE r.id = target_id FOR UPDATE OF r;

    IF NOT FOUND THEN RETURN NULL; END IF;

    IF EXISTS (SELECT 1 FROM dorm_hygiene_details WHERE record_id = target_id)
       OR EXISTS (SELECT 1 FROM classroom_hygiene_details WHERE record_id = target_id)
       OR EXISTS (SELECT 1 FROM public_dormitory_hygiene_details WHERE record_id = target_id) THEN
        SELECT count(*) INTO class_count FROM inspection_responsibility_classes WHERE record_id = target_id;
        IF class_count = 0 THEN
            RAISE EXCEPTION 'object inspection requires a responsibility class' USING ERRCODE='23514';
        END IF;
        IF current_version >= 2 THEN
            SELECT count(*) INTO student_count FROM inspection_responsibility_students WHERE record_id = target_id;
            IF class_count <> 1 OR student_count <> 1 THEN
                RAISE EXCEPTION 'daily hygiene requires one class and one responsible student' USING ERRCODE='23514';
            END IF;
            IF type_code = 'PUBLIC_DORMITORY_HYGIENE' THEN
                SELECT count(*) INTO dormitory_count FROM inspection_responsibility_dormitories WHERE record_id = target_id;
                IF dormitory_count <> 1 THEN
                    RAISE EXCEPTION 'public dormitory hygiene requires one responsible dormitory' USING ERRCODE='23514';
                END IF;
            END IF;
        END IF;
    END IF;
    RETURN NULL;
END;
$$;

ALTER TABLE classroom_hygiene_details ALTER COLUMN area_id DROP NOT NULL;
ALTER TABLE public_dormitory_hygiene_details ALTER COLUMN area_id DROP NOT NULL;
ALTER TABLE evening_self_study_details ALTER COLUMN classroom_id DROP NOT NULL;

DO $$
DECLARE table_name text; constraint_row record;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['dorm_hygiene_details','classroom_hygiene_details','public_dormitory_hygiene_details'] LOOP
        FOR constraint_row IN
            SELECT conname FROM pg_constraint
            WHERE conrelid = table_name::regclass AND contype = 'c'
              AND pg_get_constraintdef(oid) ILIKE '%reason_code%'
        LOOP
            EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', table_name, constraint_row.conname);
        END LOOP;
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK (result <> ''UNQUALIFIED'' OR length(btrim(description)) > 0) NOT VALID',
                       table_name, table_name || '_situation_required');
    END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION check_inspection_record_shape() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE type_code text;
BEGIN
    SELECT code INTO type_code FROM inspection_types WHERE id = NEW.inspection_type_id;
    NEW.dedup_policy := CASE type_code
        WHEN 'ATTENDANCE_RATE' THEN 1
        WHEN 'MORNING_ATTENDANCE' THEN 2
        WHEN 'LATE_RETURN' THEN 2
        WHEN 'EVENING_SELF_STUDY' THEN 2
        WHEN 'DORMITORY_HYGIENE' THEN 3
        WHEN 'CLASSROOM_HYGIENE' THEN 3
        WHEN 'PUBLIC_DORMITORY_HYGIENE' THEN 3
        ELSE 0
    END;
    NEW.dedup_date := COALESCE(NEW.inspection_date, (NEW.occurred_at AT TIME ZONE 'Asia/Shanghai')::date);
    IF NEW.dedup_policy IN (1, 2) THEN
        NEW.dedup_class_id := NEW.class_id;
        NEW.dedup_student_id := NEW.student_id;
    END IF;

    IF type_code = 'MORNING_ATTENDANCE' AND (NEW.occurrence_kind <> 'DATE' OR NEW.violation_count <> 1) THEN
        RAISE EXCEPTION 'morning inspection requires one daily event' USING ERRCODE='23514';
    END IF;
    IF type_code IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM')
       AND (NEW.occurrence_kind <> 'TIME' OR NEW.violation_count <= 0) THEN
        RAISE EXCEPTION 'personal course inspection requires a timestamp' USING ERRCODE='23514';
    END IF;
    IF type_code = 'LATE_RETURN' THEN
        IF NEW.contract_version >= 2 AND (NEW.occurrence_kind <> 'DATE' OR NEW.occurred_at IS NOT NULL OR NEW.class_id IS NULL OR NEW.student_id IS NULL) THEN
            RAISE EXCEPTION 'late return requires a date-only event' USING ERRCODE='23514';
        END IF;
        IF NEW.violation_count <> 1 THEN
            RAISE EXCEPTION 'late return requires one event per student' USING ERRCODE='23514';
        END IF;
    END IF;
    IF type_code = 'EVENING_SELF_STUDY' AND NEW.contract_version >= 2
       AND (NEW.occurrence_kind <> 'DATE' OR NEW.occurred_at IS NOT NULL OR NEW.violation_count <> 1 OR NEW.class_id IS NULL OR NEW.student_id IS NULL) THEN
        RAISE EXCEPTION 'evening self-study requires one date-only absence event' USING ERRCODE='23514';
    END IF;
    IF type_code IN ('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE')
       AND NEW.contract_version >= 2
       AND (NEW.occurrence_kind <> 'DATE' OR NEW.occurred_at IS NOT NULL OR NEW.student_id IS NOT NULL OR NEW.class_id IS NOT NULL) THEN
        RAISE EXCEPTION 'daily object inspection requires a date and separate responsibility association' USING ERRCODE='23514';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF type_code NOT IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM') AND EXISTS(SELECT 1 FROM attendance_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code <> 'MORNING_ATTENDANCE' AND EXISTS(SELECT 1 FROM morning_exercise_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code <> 'LATE_RETURN' AND EXISTS(SELECT 1 FROM late_return_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code <> 'DORMITORY_HYGIENE' AND EXISTS(SELECT 1 FROM dorm_hygiene_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code <> 'CLASSROOM_HYGIENE' AND EXISTS(SELECT 1 FROM classroom_hygiene_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code <> 'PUBLIC_DORMITORY_HYGIENE' AND EXISTS(SELECT 1 FROM public_dormitory_hygiene_details WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
        IF type_code NOT IN ('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE') AND EXISTS(SELECT 1 FROM inspection_responsibility_classes WHERE record_id=NEW.id) THEN RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION check_evening_self_study_record_shape() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE type_code text;
BEGIN
    SELECT code INTO type_code FROM inspection_types WHERE id = NEW.inspection_type_id;
    IF type_code = 'EVENING_SELF_STUDY' AND NEW.contract_version >= 2
       AND (NEW.occurrence_kind <> 'DATE' OR NEW.occurred_at IS NOT NULL OR NEW.violation_count <> 1) THEN
        RAISE EXCEPTION 'evening self-study requires one date-only absence event' USING ERRCODE='23514';
    END IF;
    IF type_code <> 'EVENING_SELF_STUDY' AND EXISTS(SELECT 1 FROM evening_self_study_details WHERE record_id=NEW.id) THEN
        RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE morning_attendance_week_schedules (
    week_id uuid PRIMARY KEY REFERENCES semester_weeks(id) ON DELETE RESTRICT,
    planned_dates date[] NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    updated_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(planned_dates) <= 7)
);

INSERT INTO dictionary_types (id, code, name, description, system, enabled) VALUES
    (gen_random_uuid(), 'DORMITORY_HYGIENE_SITUATION_SNIPPET', '宿舍卫生情况说明短语', '仅作为卫生情况说明文本框的可编辑起稿', true, true),
    (gen_random_uuid(), 'CLASSROOM_HYGIENE_SITUATION_SNIPPET', '教室卫生情况说明短语', '仅作为卫生情况说明文本框的可编辑起稿', true, true),
    (gen_random_uuid(), 'PUBLIC_DORMITORY_HYGIENE_SITUATION_SNIPPET', '公共宿舍卫生情况说明短语', '仅作为卫生情况说明文本框的可编辑起稿', true, true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO dictionary_items (id, type_id, value, label, sort_order, enabled)
SELECT gen_random_uuid(), target.id, source.value, source.label, source.sort_order, source.enabled
FROM dictionary_types source_type
JOIN dictionary_items source ON source.type_id = source_type.id
JOIN dictionary_types target ON target.code = replace(source_type.code, '_REASON', '_SITUATION_SNIPPET')
WHERE source_type.code IN ('DORMITORY_HYGIENE_REASON','CLASSROOM_HYGIENE_REASON','PUBLIC_DORMITORY_HYGIENE_REASON')
ON CONFLICT (type_id, value) DO NOTHING;

-- Keep old null-department rows classified as college data while allowing new
-- super-administrator records to be attributable to the operator itself.
ALTER TABLE inspection_records ADD COLUMN attribution_mode varchar(24) NOT NULL DEFAULT 'COLLEGE'
    CHECK (attribution_mode IN ('DEPARTMENT','COLLEGE','ADMINISTRATOR'));
UPDATE inspection_records SET attribution_mode='DEPARTMENT' WHERE department_id IS NOT NULL;
CREATE INDEX idx_inspection_records_attribution_owner
    ON inspection_records(attribution_mode,created_by,week_id) WHERE deleted_at IS NULL;

ALTER TABLE import_batches ADD COLUMN attribution_mode varchar(24) NOT NULL DEFAULT 'COLLEGE'
    CHECK (attribution_mode IN ('DEPARTMENT','COLLEGE','ADMINISTRATOR'));
UPDATE import_batches SET attribution_mode='DEPARTMENT' WHERE department_id IS NOT NULL;

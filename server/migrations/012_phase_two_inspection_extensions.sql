-- Preserve legacy timestamp records; date-only events and named rounds never
-- manufacture an occurrence time. Existing attendance writers keep TIME defaults.
ALTER TABLE inspection_records ALTER COLUMN occurred_at DROP NOT NULL;
ALTER TABLE inspection_records
    ADD COLUMN occurrence_kind varchar(8) NOT NULL DEFAULT 'TIME',
    ADD COLUMN inspection_date date,
    ADD COLUMN inspection_round varchar(64),
    ADD CONSTRAINT inspection_occurrence_shape CHECK (
        (occurrence_kind = 'TIME' AND occurred_at IS NOT NULL AND inspection_date IS NULL AND inspection_round IS NULL)
        OR (occurrence_kind = 'DATE' AND occurred_at IS NULL AND inspection_date IS NOT NULL AND inspection_round IS NULL)
        OR (occurrence_kind = 'ROUND' AND occurred_at IS NULL AND inspection_date IS NOT NULL
            AND inspection_round IS NOT NULL AND length(btrim(inspection_round)) > 0 AND inspection_round = btrim(inspection_round))
    );
-- Qualified object inspections are not violations. Attendance validation is unchanged.
ALTER TABLE inspection_records DROP CONSTRAINT inspection_records_violation_count_check;
ALTER TABLE inspection_records ADD CONSTRAINT inspection_records_violation_count_check CHECK (violation_count >= 0);
CREATE INDEX idx_inspection_records_calendar_date
    ON inspection_records(semester_id, week_id, inspection_type_id, inspection_date)
    WHERE deleted_at IS NULL AND occurrence_kind IN ('DATE', 'ROUND');
-- uq_inspection_records_active_dedup continues to protect active event keys.

CREATE TABLE classroom_hygiene_areas (
    id uuid PRIMARY KEY,
    classroom_id uuid NOT NULL REFERENCES classrooms(id) ON DELETE RESTRICT,
    name varchar(128) NOT NULL CHECK (length(btrim(name)) > 0 AND name = btrim(name)),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (classroom_id, name),
    UNIQUE (id, classroom_id)
);
CREATE TABLE public_dormitory_hygiene_areas (
    id uuid PRIMARY KEY,
    dormitory_building_id uuid NOT NULL REFERENCES dormitory_buildings(id) ON DELETE RESTRICT,
    floor_unit varchar(64) NOT NULL CHECK (length(btrim(floor_unit)) > 0 AND floor_unit = btrim(floor_unit)),
    name varchar(128) NOT NULL CHECK (length(btrim(name)) > 0 AND name = btrim(name)),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (dormitory_building_id, floor_unit, name)
);

CREATE TABLE morning_exercise_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE RESTRICT,
    absence_count smallint NOT NULL DEFAULT 1 CHECK (absence_count = 1)
);
CREATE TABLE late_return_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE RESTRICT,
    dormitory_id uuid NOT NULL REFERENCES dormitories(id) ON DELETE RESTRICT
);
CREATE INDEX idx_late_return_dormitory ON late_return_details(dormitory_id, record_id);

CREATE TABLE dorm_hygiene_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE RESTRICT,
    dormitory_id uuid NOT NULL REFERENCES dormitories(id) ON DELETE RESTRICT,
    result varchar(16) NOT NULL CHECK (result IN ('EXCELLENT', 'QUALIFIED', 'UNQUALIFIED')),
    reason_code varchar(64),
    description varchar(500) NOT NULL DEFAULT '',
    CHECK (reason_code IS NULL OR (length(btrim(reason_code)) > 0 AND reason_code = btrim(reason_code))),
    CHECK (result <> 'UNQUALIFIED' OR reason_code IS NOT NULL)
);
CREATE INDEX idx_dorm_hygiene_details_object ON dorm_hygiene_details(dormitory_id, record_id);

CREATE TABLE classroom_hygiene_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE RESTRICT,
    classroom_id uuid NOT NULL REFERENCES classrooms(id) ON DELETE RESTRICT,
    area_id uuid NOT NULL,
    result varchar(16) NOT NULL CHECK (result IN ('EXCELLENT', 'QUALIFIED', 'UNQUALIFIED')),
    reason_code varchar(64),
    description varchar(500) NOT NULL DEFAULT '',
    CHECK (reason_code IS NULL OR (length(btrim(reason_code)) > 0 AND reason_code = btrim(reason_code))),
    CHECK (result <> 'UNQUALIFIED' OR reason_code IS NOT NULL),
    FOREIGN KEY (area_id, classroom_id) REFERENCES classroom_hygiene_areas(id, classroom_id) ON DELETE RESTRICT
);
CREATE INDEX idx_classroom_hygiene_details_object ON classroom_hygiene_details(area_id, record_id);

CREATE TABLE public_dormitory_hygiene_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE RESTRICT,
    area_id uuid NOT NULL REFERENCES public_dormitory_hygiene_areas(id) ON DELETE RESTRICT,
    result varchar(16) NOT NULL CHECK (result IN ('EXCELLENT', 'QUALIFIED', 'UNQUALIFIED')),
    reason_code varchar(64),
    description varchar(500) NOT NULL DEFAULT '',
    CHECK (reason_code IS NULL OR (length(btrim(reason_code)) > 0 AND reason_code = btrim(reason_code))),
    CHECK (result <> 'UNQUALIFIED' OR reason_code IS NOT NULL)
);
CREATE INDEX idx_public_dormitory_hygiene_details_object ON public_dormitory_hygiene_details(area_id, record_id);

-- Responsibility scopes are queryable associations, not arrays or free-text names.
-- Snapshots do not depend on a student's current class after a transfer.
CREATE TABLE inspection_responsibility_classes (
    record_id uuid NOT NULL REFERENCES inspection_records(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    class_name_snapshot varchar(96) NOT NULL CHECK (length(btrim(class_name_snapshot)) > 0),
    grade_snapshot varchar(16) NOT NULL CHECK (length(btrim(grade_snapshot)) > 0),
    PRIMARY KEY (record_id, class_id)
);
CREATE INDEX idx_inspection_responsibility_class_scope ON inspection_responsibility_classes(class_id, record_id);
CREATE INDEX idx_inspection_responsibility_grade_scope ON inspection_responsibility_classes(grade_snapshot, record_id);
CREATE TABLE inspection_responsibility_students (
    record_id uuid NOT NULL,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL,
    student_no_snapshot varchar(32) NOT NULL CHECK (length(btrim(student_no_snapshot)) > 0),
    student_name_snapshot varchar(64) NOT NULL CHECK (length(btrim(student_name_snapshot)) > 0),
    PRIMARY KEY (record_id, student_id),
    FOREIGN KEY (record_id, class_id) REFERENCES inspection_responsibility_classes(record_id, class_id) ON DELETE RESTRICT
);
CREATE INDEX idx_inspection_responsibility_student_scope ON inspection_responsibility_students(student_id, record_id);
CREATE INDEX idx_inspection_responsibility_student_class ON inspection_responsibility_students(record_id, class_id);
CREATE TABLE inspection_responsibility_dormitories (
    record_id uuid NOT NULL REFERENCES public_dormitory_hygiene_details(record_id) ON DELETE RESTRICT,
    dormitory_id uuid NOT NULL REFERENCES dormitories(id) ON DELETE RESTRICT,
    PRIMARY KEY (record_id, dormitory_id)
);
CREATE INDEX idx_inspection_responsibility_dormitory ON inspection_responsibility_dormitories(dormitory_id, record_id);

-- The row lock makes type checks serialize with changes of the parent record.
CREATE FUNCTION check_inspection_detail_type() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE record_type text;
BEGIN
    SELECT t.code INTO record_type
    FROM inspection_records r JOIN inspection_types t ON t.id=r.inspection_type_id
    WHERE r.id=NEW.record_id FOR SHARE OF r;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'inspection record does not exist' USING ERRCODE='23503';
    END IF;
    IF NOT (record_type = ANY(TG_ARGV)) THEN
        RAISE EXCEPTION 'inspection detail type mismatch' USING ERRCODE='23514';
    END IF;
    IF TG_OP='UPDATE' AND NEW.record_id <> OLD.record_id THEN
        RAISE EXCEPTION 'inspection detail identity is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER attendance_details_type_guard BEFORE INSERT OR UPDATE ON attendance_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('ATTENDANCE_RATE','UNATTENDED_CLASSROOM');
CREATE TRIGGER morning_exercise_details_type_guard BEFORE INSERT OR UPDATE ON morning_exercise_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('MORNING_ATTENDANCE');
CREATE TRIGGER late_return_details_type_guard BEFORE INSERT OR UPDATE ON late_return_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('LATE_RETURN');
CREATE TRIGGER dorm_hygiene_details_type_guard BEFORE INSERT OR UPDATE ON dorm_hygiene_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('DORMITORY_HYGIENE');
CREATE TRIGGER classroom_hygiene_details_type_guard BEFORE INSERT OR UPDATE ON classroom_hygiene_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('CLASSROOM_HYGIENE');
CREATE TRIGGER public_dormitory_hygiene_details_type_guard BEFORE INSERT OR UPDATE ON public_dormitory_hygiene_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('PUBLIC_DORMITORY_HYGIENE');
CREATE TRIGGER inspection_responsibility_classes_type_guard BEFORE INSERT OR UPDATE ON inspection_responsibility_classes
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE');

-- Updating the common row cannot invalidate an already attached detail model.
CREATE FUNCTION check_inspection_record_shape() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE type_code text;
BEGIN
    SELECT code INTO type_code FROM inspection_types WHERE id=NEW.inspection_type_id;
    IF type_code='MORNING_ATTENDANCE' AND (NEW.occurrence_kind <> 'DATE' OR NEW.violation_count <> 1) THEN
        RAISE EXCEPTION 'morning inspection requires one daily event' USING ERRCODE='23514';
    END IF;
    IF type_code IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM','LATE_RETURN')
        AND (NEW.occurrence_kind <> 'TIME' OR NEW.violation_count <= 0) THEN
        RAISE EXCEPTION 'personal inspection requires a timestamp' USING ERRCODE='23514';
    END IF;
    IF type_code='LATE_RETURN' AND NEW.violation_count <> 1 THEN
        RAISE EXCEPTION 'late return requires one event per student' USING ERRCODE='23514';
    END IF;
    IF type_code IN ('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE') THEN
        IF NEW.occurrence_kind NOT IN ('TIME','ROUND') OR NEW.student_id IS NOT NULL OR NEW.class_id IS NOT NULL THEN
            RAISE EXCEPTION 'object inspection requires separate responsibilities' USING ERRCODE='23514';
        END IF;
    END IF;
    IF TG_OP='UPDATE' THEN
        IF type_code NOT IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM') AND EXISTS(SELECT 1 FROM attendance_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('MORNING_ATTENDANCE') AND EXISTS(SELECT 1 FROM morning_exercise_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('LATE_RETURN') AND EXISTS(SELECT 1 FROM late_return_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('DORMITORY_HYGIENE') AND EXISTS(SELECT 1 FROM dorm_hygiene_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('CLASSROOM_HYGIENE') AND EXISTS(SELECT 1 FROM classroom_hygiene_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('PUBLIC_DORMITORY_HYGIENE') AND EXISTS(SELECT 1 FROM public_dormitory_hygiene_details WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
        IF type_code NOT IN ('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE') AND EXISTS(SELECT 1 FROM inspection_responsibility_classes WHERE record_id=NEW.id) THEN
            RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER inspection_record_shape_guard BEFORE INSERT OR UPDATE ON inspection_records
    FOR EACH ROW EXECUTE FUNCTION check_inspection_record_shape();

-- Deferred checks allow a service to insert the detail and class associations
-- in either order inside one transaction, but never persist unscoped hygiene data.
CREATE FUNCTION check_hygiene_has_responsibility() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_id uuid;
BEGIN
    IF TG_OP='DELETE' THEN target_id:=OLD.record_id; ELSE target_id:=NEW.record_id; END IF;
    PERFORM 1 FROM inspection_records WHERE id=target_id FOR UPDATE;
    IF (EXISTS(SELECT 1 FROM dorm_hygiene_details WHERE record_id=target_id)
        OR EXISTS(SELECT 1 FROM classroom_hygiene_details WHERE record_id=target_id)
        OR EXISTS(SELECT 1 FROM public_dormitory_hygiene_details WHERE record_id=target_id))
        AND NOT EXISTS(SELECT 1 FROM inspection_responsibility_classes WHERE record_id=target_id) THEN
        RAISE EXCEPTION 'object inspection requires a responsibility class' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER dorm_hygiene_details_responsibility_guard AFTER INSERT OR UPDATE OR DELETE ON dorm_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_has_responsibility();
CREATE CONSTRAINT TRIGGER classroom_hygiene_details_responsibility_guard AFTER INSERT OR UPDATE OR DELETE ON classroom_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_has_responsibility();
CREATE CONSTRAINT TRIGGER public_dormitory_hygiene_details_responsibility_guard AFTER INSERT OR UPDATE OR DELETE ON public_dormitory_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_has_responsibility();
CREATE CONSTRAINT TRIGGER inspection_responsibility_classes_responsibility_guard AFTER INSERT OR UPDATE OR DELETE ON inspection_responsibility_classes
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_has_responsibility();

-- Result and count may change together in either statement order; validate the
-- final transaction state rather than making valid result corrections impossible.
CREATE FUNCTION check_hygiene_result_count() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_id uuid; current_count numeric; current_result text;
BEGIN
    IF TG_TABLE_NAME='inspection_records' THEN target_id:=NEW.id; ELSE target_id:=NEW.record_id; END IF;
    SELECT violation_count INTO current_count FROM inspection_records WHERE id=target_id FOR UPDATE;
    SELECT result INTO current_result FROM (
        SELECT result FROM dorm_hygiene_details WHERE record_id=target_id
        UNION ALL SELECT result FROM classroom_hygiene_details WHERE record_id=target_id
        UNION ALL SELECT result FROM public_dormitory_hygiene_details WHERE record_id=target_id
    ) results LIMIT 1;
    IF current_result IS NOT NULL AND current_count <> (CASE WHEN current_result='UNQUALIFIED' THEN 1 ELSE 0 END) THEN
        RAISE EXCEPTION 'inspection result and violation count mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER inspection_records_result_count_guard AFTER INSERT OR UPDATE ON inspection_records
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_result_count();
CREATE CONSTRAINT TRIGGER dorm_hygiene_details_result_count_guard AFTER INSERT OR UPDATE ON dorm_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_result_count();
CREATE CONSTRAINT TRIGGER classroom_hygiene_details_result_count_guard AFTER INSERT OR UPDATE ON classroom_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_result_count();
CREATE CONSTRAINT TRIGGER public_dormitory_hygiene_details_result_count_guard AFTER INSERT OR UPDATE ON public_dormitory_hygiene_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_hygiene_result_count();

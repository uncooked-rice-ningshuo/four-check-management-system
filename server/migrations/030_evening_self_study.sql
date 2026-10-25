INSERT INTO inspection_types(id, code, name, extension_kind, enabled, participates_in_report, sort_order)
VALUES(gen_random_uuid(), 'EVENING_SELF_STUDY', '晚自习', 'EVENING_SELF_STUDY', true, true, 80)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    extension_kind = EXCLUDED.extension_kind,
    enabled = true,
    participates_in_report = true,
    sort_order = EXCLUDED.sort_order;

CREATE TABLE evening_self_study_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE CASCADE,
    classroom_id uuid NOT NULL REFERENCES classrooms(id),
    period_text varchar(64) NOT NULL CHECK (period_text = '9-10'),
    absence_hours numeric(6,2) NOT NULL CHECK (absence_hours = 2),
    penalty_multiplier smallint NOT NULL CHECK (penalty_multiplier IN (1,2)),
    penalty_hours numeric(6,2) NOT NULL CHECK (penalty_hours = absence_hours * penalty_multiplier)
);

CREATE TRIGGER evening_self_study_details_type_guard BEFORE INSERT OR UPDATE ON evening_self_study_details
    FOR EACH ROW EXECUTE FUNCTION check_inspection_detail_type('EVENING_SELF_STUDY');

CREATE FUNCTION check_evening_self_study_record_shape() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE type_code text;
BEGIN
    SELECT code INTO type_code FROM inspection_types WHERE id=NEW.inspection_type_id;
    IF type_code='EVENING_SELF_STUDY'
       AND (NEW.occurrence_kind <> 'TIME' OR NEW.violation_count <> 1) THEN
        RAISE EXCEPTION 'evening self-study requires one timed absence event' USING ERRCODE='23514';
    END IF;
    IF type_code <> 'EVENING_SELF_STUDY'
       AND EXISTS(SELECT 1 FROM evening_self_study_details WHERE record_id=NEW.id) THEN
        RAISE EXCEPTION 'existing inspection detail type mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER evening_self_study_record_shape_guard BEFORE INSERT OR UPDATE ON inspection_records
    FOR EACH ROW EXECUTE FUNCTION check_evening_self_study_record_shape();

CREATE FUNCTION check_evening_self_study_detail_present() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_id uuid; target_type text;
BEGIN
    IF TG_TABLE_NAME='inspection_records' THEN
        IF TG_OP='DELETE' THEN target_id:=OLD.id; ELSE target_id:=NEW.id; END IF;
    ELSE
        target_id:=OLD.record_id;
    END IF;
    SELECT it.code INTO target_type FROM inspection_records ir
    JOIN inspection_types it ON it.id=ir.inspection_type_id WHERE ir.id=target_id;
    IF target_type='EVENING_SELF_STUDY'
       AND NOT EXISTS(SELECT 1 FROM evening_self_study_details WHERE record_id=target_id) THEN
        RAISE EXCEPTION 'evening self-study detail is required' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER evening_self_study_record_detail_guard
    AFTER INSERT OR UPDATE ON inspection_records
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION check_evening_self_study_detail_present();
CREATE CONSTRAINT TRIGGER evening_self_study_detail_record_guard
    AFTER DELETE ON evening_self_study_details
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION check_evening_self_study_detail_present();

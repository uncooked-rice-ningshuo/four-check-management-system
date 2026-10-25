-- Weekly reporting obligations are configured independently of department
-- entry scopes. A department/type/week can be removed and later re-added
-- without losing the historical configuration row.
ALTER TABLE semester_weeks
    ADD CONSTRAINT semester_weeks_id_semester_unique UNIQUE (id, semester_id);

CREATE TABLE weekly_report_obligations (
    id uuid PRIMARY KEY,
    semester_id uuid NOT NULL,
    week_id uuid NOT NULL,
    department_id uuid REFERENCES departments(id) ON DELETE RESTRICT,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id) ON DELETE RESTRICT,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    CONSTRAINT weekly_report_obligations_week_semester_fkey
        FOREIGN KEY (week_id, semester_id)
        REFERENCES semester_weeks(id, semester_id) ON DELETE RESTRICT
);

-- NULL department_id means the college-level reporting cell. Separate
-- partial unique indexes keep those cells distinct from department cells.
CREATE UNIQUE INDEX uq_weekly_report_obligations_active_department
    ON weekly_report_obligations(semester_id, week_id, department_id, inspection_type_id)
    WHERE removed_at IS NULL AND department_id IS NOT NULL;
CREATE UNIQUE INDEX uq_weekly_report_obligations_active_college
    ON weekly_report_obligations(semester_id, week_id, inspection_type_id)
    WHERE removed_at IS NULL AND department_id IS NULL;
CREATE INDEX idx_weekly_report_obligations_week_active
    ON weekly_report_obligations(semester_id, week_id, department_id, inspection_type_id)
    WHERE removed_at IS NULL;

-- Existing batches and revisions are RECORDS. The default preserves their
-- meaning while allowing new zero-record receipts to be explicit.
ALTER TABLE department_submissions
    ADD COLUMN submission_kind varchar(16) NOT NULL DEFAULT 'RECORDS',
    ADD CONSTRAINT department_submissions_submission_kind_check
        CHECK (submission_kind IN ('RECORDS', 'ZERO_RECORD')),
    ADD CONSTRAINT department_submissions_id_kind_unique
        UNIQUE (id, submission_kind);

ALTER TABLE department_submission_revisions
    ADD COLUMN submission_kind varchar(16) NOT NULL DEFAULT 'RECORDS';

ALTER TABLE department_submission_revisions
    DROP CONSTRAINT department_submission_revisions_record_ids_check,
    DROP CONSTRAINT department_submission_revisions_submission_id_fkey,
    ADD CONSTRAINT department_submission_revisions_submission_kind_check
        CHECK (submission_kind IN ('RECORDS', 'ZERO_RECORD')),
    ADD CONSTRAINT department_submission_revisions_kind_record_ids_check
        CHECK (
            (submission_kind = 'RECORDS' AND cardinality(record_ids) > 0)
            OR (
                submission_kind = 'ZERO_RECORD'
                AND cardinality(record_ids) = 0
                AND records_snapshot = '[]'::jsonb
            )
        ),
    ADD CONSTRAINT department_submission_revisions_submission_kind_fkey
        FOREIGN KEY (submission_id, submission_kind)
        REFERENCES department_submissions(id, submission_kind) ON DELETE RESTRICT;

-- A receipt's kind is part of its identity. Later discovered records must be
-- appended as a separate RECORDS batch, never by converting a ZERO_RECORD batch.
CREATE FUNCTION prevent_department_submission_kind_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.submission_kind IS DISTINCT FROM NEW.submission_kind THEN
        RAISE EXCEPTION 'department submission kind is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER department_submissions_submission_kind_immutable
    BEFORE UPDATE OF submission_kind ON department_submissions
    FOR EACH ROW EXECUTE FUNCTION prevent_department_submission_kind_mutation();

-- An unassigned inspection is a college-level record, not an office or other
-- department record. Existing rows retain their department scope unchanged.
ALTER TABLE inspection_records ALTER COLUMN department_id DROP NOT NULL;
ALTER TABLE import_batches ALTER COLUMN department_id DROP NOT NULL;
ALTER TABLE department_submissions ALTER COLUMN department_id DROP NOT NULL;

-- PostgreSQL composite foreign keys skip checks when any component is NULL.
-- A generated sentinel key keeps the batch/record scope constraint effective
-- for both department and college-level submissions without storing a fake
-- department in business data.
ALTER TABLE department_submissions
    ADD COLUMN scope_department_key uuid GENERATED ALWAYS AS
        (COALESCE(department_id, '00000000-0000-0000-0000-000000000000'::uuid)) STORED;
ALTER TABLE inspection_records
    ADD COLUMN scope_department_key uuid GENERATED ALWAYS AS
        (COALESCE(department_id, '00000000-0000-0000-0000-000000000000'::uuid)) STORED;
ALTER TABLE department_submissions
    ADD CONSTRAINT department_submissions_scope_key_unique
    UNIQUE (id, semester_id, week_id, scope_department_key, inspection_type_id);
ALTER TABLE inspection_records
    DROP CONSTRAINT inspection_records_submission_scope_fkey;
ALTER TABLE inspection_records
    ADD CONSTRAINT inspection_records_submission_scope_fkey
    FOREIGN KEY (submission_id, semester_id, week_id, scope_department_key, inspection_type_id)
    REFERENCES department_submissions(id, semester_id, week_id, scope_department_key, inspection_type_id)
    ON DELETE RESTRICT;

CREATE INDEX idx_college_inspection_records_week
    ON inspection_records(semester_id, week_id, inspection_type_id, status, updated_at DESC)
    WHERE department_id IS NULL AND deleted_at IS NULL;
CREATE INDEX idx_college_submissions_week
    ON department_submissions(semester_id, week_id, inspection_type_id, status, updated_at DESC)
    WHERE department_id IS NULL;

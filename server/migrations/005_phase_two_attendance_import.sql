CREATE TABLE IF NOT EXISTS import_templates (
    id uuid PRIMARY KEY,
    code varchar(64) NOT NULL,
    name varchar(128) NOT NULL,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    version_no integer NOT NULL CHECK (version_no > 0),
    file_id uuid NOT NULL REFERENCES files(id),
    field_definition_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(16) NOT NULL DEFAULT 'DRAFT'
        CONSTRAINT import_templates_status_check CHECK (status IN ('DRAFT', 'PUBLISHED', 'RETIRED')),
    published_at timestamptz,
    created_by uuid REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(inspection_type_id, version_no),
    UNIQUE(code, version_no)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_import_templates_published
    ON import_templates(inspection_type_id) WHERE status = 'PUBLISHED';

CREATE TABLE IF NOT EXISTS import_batches (
    id uuid PRIMARY KEY,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    semester_id uuid NOT NULL REFERENCES semesters(id),
    week_id uuid NOT NULL REFERENCES semester_weeks(id),
    department_id uuid NOT NULL REFERENCES departments(id),
	class_id uuid REFERENCES classes(id),
	grade varchar(16) NOT NULL DEFAULT '',
    original_file_id uuid NOT NULL REFERENCES files(id),
    template_id uuid NOT NULL REFERENCES import_templates(id),
    template_version integer NOT NULL CHECK (template_version > 0),
    status varchar(24) NOT NULL DEFAULT 'UPLOADED'
        CONSTRAINT import_batches_status_check CHECK (status IN (
            'UPLOADED', 'PARSING', 'PREVIEW_READY', 'CONFIRMING', 'CONFIRMED',
            'PARSE_FAILED', 'CONFIRM_FAILED', 'CANCELLED'
        )),
    total_rows integer NOT NULL DEFAULT 0 CHECK (total_rows >= 0),
    valid_rows integer NOT NULL DEFAULT 0 CHECK (valid_rows >= 0),
    invalid_rows integer NOT NULL DEFAULT 0 CHECK (invalid_rows >= 0),
    imported_rows integer NOT NULL DEFAULT 0 CHECK (imported_rows >= 0),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    confirm_idempotency_key varchar(128),
    failure_code varchar(64) NOT NULL DEFAULT '',
    failure_message varchar(255) NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    cancelled_at timestamptz,
    CHECK (valid_rows + invalid_rows <= total_rows),
    CHECK (imported_rows <= valid_rows)
);

CREATE INDEX IF NOT EXISTS idx_import_batches_scope
    ON import_batches(semester_id, week_id, inspection_type_id, department_id, class_id, grade, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_import_batches_confirm_idempotency
    ON import_batches(created_by, confirm_idempotency_key)
    WHERE confirm_idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS inspection_records (
    id uuid PRIMARY KEY,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    semester_id uuid NOT NULL REFERENCES semesters(id),
    week_id uuid NOT NULL REFERENCES semester_weeks(id),
    department_id uuid NOT NULL REFERENCES departments(id),
    student_id uuid REFERENCES students(id),
    student_no_snapshot varchar(32) NOT NULL DEFAULT '',
    student_name_snapshot varchar(64) NOT NULL DEFAULT '',
    class_id uuid REFERENCES classes(id),
    class_name_snapshot varchar(96) NOT NULL DEFAULT '',
    grade_snapshot varchar(16) NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    violation_count numeric(10,2) NOT NULL DEFAULT 1 CHECK (violation_count > 0),
    remark text NOT NULL DEFAULT '',
    source varchar(24) NOT NULL
        CONSTRAINT inspection_records_source_check CHECK (source IN ('MANUAL', 'EXCEL', 'REPORT_CORRECTION', 'MIGRATION')),
    status varchar(32) NOT NULL DEFAULT 'DRAFT'
        CONSTRAINT inspection_records_status_check CHECK (status IN ('DRAFT', 'PENDING_REVIEW', 'RETURNED', 'CONFIRMED', 'LOCKED')),
    submission_id uuid,
    import_batch_id uuid REFERENCES import_batches(id),
    locked_by_report_id uuid,
    dedup_key char(64) NOT NULL,
    row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_inspection_records_scope
    ON inspection_records(semester_id, week_id, inspection_type_id, department_id, status)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_inspection_records_student
    ON inspection_records(student_id, occurred_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_inspection_records_class
    ON inspection_records(class_id, week_id, inspection_type_id)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_inspection_records_active_dedup
    ON inspection_records(semester_id, week_id, inspection_type_id, dedup_key)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS attendance_details (
    record_id uuid PRIMARY KEY REFERENCES inspection_records(id) ON DELETE CASCADE,
    course_id uuid NOT NULL REFERENCES courses(id),
    absence_hours numeric(6,2) NOT NULL CHECK (absence_hours > 0),
    period_text varchar(64) NOT NULL,
    classroom_id uuid REFERENCES classrooms(id)
);

CREATE TABLE IF NOT EXISTS import_rows (
    id uuid PRIMARY KEY,
    batch_id uuid NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    row_no integer NOT NULL CHECK (row_no > 0),
    raw_data_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    normalized_data_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    validation_status varchar(16) NOT NULL
        CONSTRAINT import_rows_validation_status_check CHECK (validation_status IN ('VALID', 'INVALID')),
    errors_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    record_id uuid REFERENCES inspection_records(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(batch_id, row_no)
);

CREATE INDEX IF NOT EXISTS idx_import_rows_batch_status
    ON import_rows(batch_id, validation_status, row_no);

CREATE TABLE weekly_reports (
    id uuid PRIMARY KEY,
    semester_id uuid NOT NULL REFERENCES semesters(id) ON DELETE RESTRICT,
    week_id uuid NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'PENDING_REVIEW', 'RETURNED', 'FINALIZING', 'FINALIZED', 'PUBLISHED', 'ARCHIVED')),
    current_version integer NOT NULL DEFAULT 0 CHECK (current_version >= 0),
    row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    finalized_by uuid REFERENCES accounts(id) ON DELETE RESTRICT,
    finalized_at timestamptz,
    published_by uuid REFERENCES accounts(id) ON DELETE RESTRICT,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT weekly_reports_week_semester_fkey FOREIGN KEY (week_id, semester_id)
        REFERENCES semester_weeks(id, semester_id) ON DELETE RESTRICT,
    CONSTRAINT weekly_reports_one_per_week UNIQUE (semester_id, week_id),
    CONSTRAINT weekly_reports_finalization_fields_check CHECK (
        (status NOT IN ('FINALIZED', 'PUBLISHED', 'ARCHIVED') OR (finalized_by IS NOT NULL AND finalized_at IS NOT NULL))
    ),
    CONSTRAINT weekly_reports_publish_fields_check CHECK (
        (status NOT IN ('PUBLISHED', 'ARCHIVED') OR (published_by IS NOT NULL AND published_at IS NOT NULL))
    )
);

CREATE TABLE weekly_report_versions (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL REFERENCES weekly_reports(id) ON DELETE RESTRICT,
    version_no integer NOT NULL CHECK (version_no > 0),
    content_json jsonb NOT NULL,
    source_fingerprint char(64) NOT NULL,
    forced_generation_reason varchar(255) NOT NULL DEFAULT '',
    scope_limited boolean NOT NULL DEFAULT false,
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(report_id, version_no),
    UNIQUE(id, report_id)
);

CREATE TABLE weekly_report_version_records (
    report_version_id uuid NOT NULL REFERENCES weekly_report_versions(id) ON DELETE RESTRICT,
    inspection_record_id uuid NOT NULL REFERENCES inspection_records(id) ON DELETE RESTRICT,
    source_submission_id uuid NOT NULL REFERENCES department_submissions(id) ON DELETE RESTRICT,
    source_revision_no integer NOT NULL CHECK (source_revision_no > 0),
    record_version integer NOT NULL CHECK (record_version > 0),
    included_data_json jsonb NOT NULL,
    PRIMARY KEY (report_version_id, inspection_record_id)
);
CREATE INDEX idx_weekly_report_version_records_record
    ON weekly_report_version_records(inspection_record_id, report_version_id);

CREATE TABLE weekly_report_reviews (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_version_id uuid NOT NULL,
    action varchar(24) NOT NULL CHECK (action IN ('SUBMITTED', 'APPROVED', 'RETURNED', 'FINALIZATION_STARTED')),
    comment varchar(1000) NOT NULL DEFAULT '',
    actor_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (report_version_id, report_id)
        REFERENCES weekly_report_versions(id, report_id) ON DELETE RESTRICT
);
CREATE INDEX idx_weekly_report_reviews_timeline
    ON weekly_report_reviews(report_id, created_at, id);

CREATE TABLE weekly_report_corrections (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_version_id uuid NOT NULL,
    inspection_record_id uuid NOT NULL REFERENCES inspection_records(id) ON DELETE RESTRICT,
    before_json jsonb NOT NULL,
    after_json jsonb NOT NULL,
    reason varchar(500) NOT NULL,
    fact_changed boolean NOT NULL,
    department_reconfirmation_status varchar(24) NOT NULL
        CHECK (department_reconfirmation_status IN ('NOT_REQUIRED', 'REQUIRED', 'CONFIRMED')),
    corrected_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    corrected_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (report_version_id, report_id)
        REFERENCES weekly_report_versions(id, report_id) ON DELETE RESTRICT,
    CHECK ((fact_changed AND department_reconfirmation_status IN ('REQUIRED', 'CONFIRMED')) OR
           (NOT fact_changed AND department_reconfirmation_status = 'NOT_REQUIRED'))
);
CREATE INDEX idx_weekly_report_corrections_report
    ON weekly_report_corrections(report_id, report_version_id, corrected_at);

CREATE TABLE weekly_report_version_files (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_version_id uuid NOT NULL,
    file_id uuid NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
    stage varchar(16) NOT NULL CHECK (stage IN ('DRAFT', 'FINAL')),
    format varchar(8) NOT NULL CHECK (format IN ('WORD', 'EXCEL')),
    content_sha256 char(64) NOT NULL,
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (report_version_id, report_id)
        REFERENCES weekly_report_versions(id, report_id) ON DELETE RESTRICT,
    UNIQUE(report_version_id, stage, format),
    UNIQUE(file_id)
);

ALTER TABLE inspection_records
    ADD CONSTRAINT inspection_records_locked_report_fkey
    FOREIGN KEY (locked_by_report_id) REFERENCES weekly_reports(id) ON DELETE RESTRICT;

CREATE FUNCTION prevent_weekly_report_history_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'weekly report history is append-only' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER weekly_report_versions_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_versions
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();
CREATE TRIGGER weekly_report_version_records_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_version_records
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();
CREATE TRIGGER weekly_report_reviews_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_reviews
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();
CREATE TRIGGER weekly_report_corrections_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_corrections
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();
CREATE TRIGGER weekly_report_version_files_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_version_files
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();

CREATE TABLE IF NOT EXISTS academic_week_import_batches (
    id uuid PRIMARY KEY,
    semester_id uuid NOT NULL REFERENCES semesters(id) ON DELETE RESTRICT,
    semester_name varchar(160) NOT NULL,
    academic_year_name varchar(160) NOT NULL,
    source_file_id uuid NOT NULL UNIQUE REFERENCES files(id) ON DELETE RESTRICT,
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    template_version integer NOT NULL CHECK (template_version = 1),
    status varchar(32) NOT NULL CHECK (status IN ('PREVIEW_READY', 'PREVIEW_HAS_ERRORS', 'CONFIRMED')),
    total_rows integer NOT NULL CHECK (total_rows BETWEEN 1 AND 60),
    valid_rows integer NOT NULL CHECK (valid_rows >= 0),
    invalid_rows integer NOT NULL CHECK (invalid_rows >= 0),
    affects_today boolean NOT NULL DEFAULT false,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    preview_json jsonb NOT NULL,
    confirm_idempotency_key varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT academic_week_import_row_counts_check CHECK (valid_rows + invalid_rows = total_rows),
    CONSTRAINT academic_week_import_status_counts_check CHECK (
        (status = 'PREVIEW_READY' AND invalid_rows = 0 AND confirmed_at IS NULL AND confirm_idempotency_key = '') OR
        (status = 'PREVIEW_HAS_ERRORS' AND invalid_rows > 0 AND confirmed_at IS NULL AND confirm_idempotency_key = '') OR
        (status = 'CONFIRMED' AND invalid_rows = 0 AND confirmed_at IS NOT NULL AND confirm_idempotency_key <> '')
    )
);

CREATE INDEX IF NOT EXISTS idx_academic_week_import_batches_actor_created
    ON academic_week_import_batches(created_by, created_at DESC);

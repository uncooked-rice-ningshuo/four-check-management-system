CREATE TABLE IF NOT EXISTS master_data_import_batches (
    id uuid PRIMARY KEY,
    kind varchar(48) NOT NULL CHECK (kind IN (
        'faculties', 'grades', 'majors', 'staff', 'classes', 'dormitory-areas',
        'dormitory-buildings', 'dormitories', 'teaching-buildings', 'classrooms', 'students'
    )),
    source_file_id uuid NOT NULL UNIQUE REFERENCES files(id),
    created_by uuid NOT NULL REFERENCES accounts(id),
    template_version integer NOT NULL CHECK (template_version = 1),
    status varchar(32) NOT NULL CHECK (status IN ('PREVIEW_READY', 'PREVIEW_HAS_ERRORS', 'CONFIRMED')),
    total_rows integer NOT NULL CHECK (total_rows BETWEEN 1 AND 1000),
    valid_rows integer NOT NULL CHECK (valid_rows >= 0),
    invalid_rows integer NOT NULL CHECK (invalid_rows >= 0),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    preview_json jsonb NOT NULL,
    confirm_idempotency_key varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT master_data_import_batch_row_counts_check CHECK (valid_rows + invalid_rows = total_rows),
    CONSTRAINT master_data_import_batch_status_counts_check CHECK (
        (status = 'PREVIEW_READY' AND invalid_rows = 0 AND confirmed_at IS NULL AND confirm_idempotency_key = '') OR
        (status = 'PREVIEW_HAS_ERRORS' AND invalid_rows > 0 AND confirmed_at IS NULL AND confirm_idempotency_key = '') OR
        (status = 'CONFIRMED' AND invalid_rows = 0 AND confirmed_at IS NOT NULL AND confirm_idempotency_key <> '')
    )
);

CREATE INDEX IF NOT EXISTS idx_master_data_import_batches_actor_created
    ON master_data_import_batches(created_by, created_at DESC);

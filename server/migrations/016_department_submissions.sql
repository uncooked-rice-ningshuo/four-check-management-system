-- A department can receive several member batches in one week and check type.
-- A batch owns one current revision; its immutable submitted revisions retain
-- the record set and data snapshot seen by the reviewer at each submission.
CREATE TABLE department_submissions (
    id uuid PRIMARY KEY,
    semester_id uuid NOT NULL REFERENCES semesters(id) ON DELETE RESTRICT,
    week_id uuid NOT NULL REFERENCES semester_weeks(id) ON DELETE RESTRICT,
    department_id uuid NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id) ON DELETE RESTRICT,
    status varchar(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'PENDING_REVIEW', 'RETURNED', 'CONFIRMED')),
    revision_no integer NOT NULL DEFAULT 0 CHECK (revision_no >= 0),
    row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    submitted_by uuid REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_by uuid REFERENCES accounts(id) ON DELETE RESTRICT,
    return_reason varchar(500) NOT NULL DEFAULT '',
    idempotency_key varchar(128),
    request_fingerprint char(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    submitted_at timestamptz,
    reviewed_at timestamptz,
    UNIQUE (id, semester_id, week_id, department_id, inspection_type_id),
    CHECK (idempotency_key IS NULL OR length(btrim(idempotency_key)) > 0),
    CHECK (status <> 'PENDING_REVIEW' OR (revision_no > 0 AND submitted_at IS NOT NULL))
);

CREATE INDEX idx_department_submissions_week_status
    ON department_submissions(semester_id, week_id, department_id, inspection_type_id, status, updated_at DESC);
CREATE INDEX idx_department_submissions_creator
    ON department_submissions(created_by, updated_at DESC);
CREATE UNIQUE INDEX uq_department_submissions_idempotency
    ON department_submissions(created_by, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE department_submission_revisions (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES department_submissions(id) ON DELETE RESTRICT,
    revision_no integer NOT NULL CHECK (revision_no > 0),
    submitted_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    record_ids uuid[] NOT NULL CHECK (cardinality(record_ids) > 0),
    records_snapshot jsonb NOT NULL CHECK (jsonb_typeof(records_snapshot) = 'array'),
    UNIQUE (submission_id, revision_no)
);

-- The scope key prevents a record from being attached to a batch for a
-- different department, week, semester or inspection type. Existing records
-- have NULL submission_id and therefore remain unchanged.
ALTER TABLE inspection_records
    ADD CONSTRAINT inspection_records_submission_scope_fkey
    FOREIGN KEY (submission_id, semester_id, week_id, department_id, inspection_type_id)
    REFERENCES department_submissions(id, semester_id, week_id, department_id, inspection_type_id)
    ON DELETE RESTRICT;
CREATE INDEX idx_inspection_records_submission
    ON inspection_records(submission_id)
    WHERE submission_id IS NOT NULL AND deleted_at IS NULL;

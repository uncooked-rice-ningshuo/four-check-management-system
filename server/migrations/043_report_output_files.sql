-- Output settings are independent of inspection participation and report data state.
CREATE TABLE report_output_settings (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    introduction text NOT NULL DEFAULT '',
    default_type_codes text[] NOT NULL DEFAULT ARRAY[]::text[],
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by uuid REFERENCES accounts(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO report_output_settings(id) VALUES(true);

-- Each attempt is retained independently; a failed attempt is never downloadable.
CREATE TABLE report_output_files (
    id uuid PRIMARY KEY,
    report_version_id uuid NOT NULL REFERENCES weekly_report_versions(id) ON DELETE RESTRICT,
    format varchar(8) NOT NULL CHECK (format IN ('WORD','EXCEL')),
    type_codes text[] NOT NULL CHECK (cardinality(type_codes) BETWEEN 1 AND 8),
    combined boolean NOT NULL,
    status varchar(16) NOT NULL CHECK (status IN ('PENDING','AVAILABLE','FAILED')),
    file_id uuid UNIQUE REFERENCES files(id) ON DELETE RESTRICT,
    failure_code varchar(64) NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'AVAILABLE' OR file_id IS NOT NULL)
);
CREATE INDEX idx_report_output_files_version ON report_output_files(report_version_id,created_at DESC);

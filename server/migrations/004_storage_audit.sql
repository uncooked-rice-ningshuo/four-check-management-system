CREATE TABLE IF NOT EXISTS files (
    id uuid PRIMARY KEY,
    storage_provider varchar(16) NOT NULL DEFAULT 'MINIO'
        CONSTRAINT files_storage_provider_check CHECK (storage_provider = 'MINIO'),
    bucket varchar(96) NOT NULL,
    object_key varchar(512) NOT NULL CONSTRAINT files_object_key_key UNIQUE,
    original_name varchar(255) NOT NULL,
    declared_content_type varchar(128) NOT NULL DEFAULT '',
    detected_content_type varchar(128) NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0
        CONSTRAINT files_size_bytes_check CHECK (size_bytes >= 0),
    sha256 char(64) NOT NULL DEFAULT '',
    status varchar(16) NOT NULL DEFAULT 'PENDING'
        CONSTRAINT files_status_check CHECK (status IN ('PENDING', 'AVAILABLE', 'FAILED', 'MISSING')),
    retention_type varchar(32) NOT NULL
        CONSTRAINT files_retention_type_check CHECK (retention_type IN (
            'TEMPORARY', 'IMPORT_SOURCE', 'TEMPLATE', 'REPORT_DRAFT', 'REPORT_FINAL', 'ARCHIVE'
        )),
    created_by uuid REFERENCES accounts(id),
    failure_code varchar(64) NOT NULL DEFAULT '',
    failure_message varchar(255) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_files_status_updated
    ON files(status, updated_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id uuid PRIMARY KEY,
    actor_id uuid REFERENCES accounts(id),
    action varchar(64) NOT NULL,
    resource_type varchar(64) NOT NULL,
    resource_id varchar(128) NOT NULL,
    before_json jsonb,
    after_json jsonb,
    reason varchar(255) NOT NULL DEFAULT '',
    request_id varchar(128) NOT NULL DEFAULT '',
    ip inet,
    user_agent varchar(512) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
    ON audit_logs(resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor
    ON audit_logs(actor_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_audit_log_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit logs are append-only' USING ERRCODE = '55000';
END;
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgname = 'audit_logs_append_only'
          AND tgrelid = 'audit_logs'::regclass
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER audit_logs_append_only
            BEFORE UPDATE OR DELETE ON audit_logs
            FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();
    END IF;
END $$;

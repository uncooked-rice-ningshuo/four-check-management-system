CREATE TABLE weekly_report_version_adjustments (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_version_id uuid NOT NULL,
    inspection_record_id uuid NOT NULL REFERENCES inspection_records(id) ON DELETE RESTRICT,
    operation varchar(16) NOT NULL CHECK (operation IN ('INCLUDE', 'EXCLUDE')),
    reason varchar(500) NOT NULL,
    record_data_json jsonb NOT NULL,
    adjusted_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    adjusted_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (report_version_id, report_id)
        REFERENCES weekly_report_versions(id, report_id) ON DELETE RESTRICT
);

CREATE INDEX idx_weekly_report_version_adjustments_report
    ON weekly_report_version_adjustments(report_id, report_version_id, adjusted_at);

CREATE TRIGGER weekly_report_version_adjustments_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_version_adjustments
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();

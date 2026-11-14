CREATE TABLE weekly_report_revision_intents (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL REFERENCES weekly_reports(id),
    base_version integer NOT NULL,
    initiated_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(report_id, base_version)
);

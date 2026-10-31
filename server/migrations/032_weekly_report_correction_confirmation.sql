ALTER TABLE weekly_report_corrections
    ADD COLUMN source_submission_id uuid REFERENCES department_submissions(id) ON DELETE RESTRICT;

CREATE TABLE weekly_report_correction_responses (
    id uuid PRIMARY KEY,
    correction_id uuid NOT NULL UNIQUE REFERENCES weekly_report_corrections(id) ON DELETE RESTRICT,
    decision varchar(16) NOT NULL CHECK (decision IN ('CONFIRMED', 'REJECTED')),
    comment varchar(500) NOT NULL DEFAULT '',
    actor_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER weekly_report_correction_responses_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_correction_responses
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();

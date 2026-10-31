-- Preserve the exact zero-record receipt revisions referenced by each report
-- version. A count alone cannot be traced back to an immutable department decision.
CREATE TABLE weekly_report_version_zero_receipts (
    report_version_id uuid NOT NULL REFERENCES weekly_report_versions(id) ON DELETE RESTRICT,
    submission_id uuid NOT NULL,
    source_revision_no integer NOT NULL CHECK (source_revision_no > 0),
    PRIMARY KEY (report_version_id, submission_id),
    FOREIGN KEY (submission_id, source_revision_no)
        REFERENCES department_submission_revisions(submission_id, revision_no) ON DELETE RESTRICT
);

CREATE INDEX idx_weekly_report_version_zero_receipts_submission
    ON weekly_report_version_zero_receipts(submission_id, source_revision_no);

CREATE TRIGGER weekly_report_version_zero_receipts_append_only
    BEFORE UPDATE OR DELETE ON weekly_report_version_zero_receipts
    FOR EACH ROW EXECUTE FUNCTION prevent_weekly_report_history_mutation();

ALTER TABLE weekly_report_reviews
    DROP CONSTRAINT weekly_report_reviews_action_check,
    ADD CONSTRAINT weekly_report_reviews_action_check
        CHECK (action IN ('SUBMITTED', 'APPROVED', 'RETURNED', 'FINALIZATION_STARTED', 'FINALIZED'));

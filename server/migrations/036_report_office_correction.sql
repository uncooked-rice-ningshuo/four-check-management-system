-- Office review applies factual corrections to the new report snapshot without
-- changing the source department's immutable submission or requiring its approval.
ALTER TABLE weekly_report_corrections
    DROP CONSTRAINT weekly_report_corrections_check;

ALTER TABLE weekly_report_corrections
    ADD CONSTRAINT weekly_report_corrections_check
    CHECK ((fact_changed AND department_reconfirmation_status IN ('NOT_REQUIRED', 'REQUIRED', 'CONFIRMED')) OR
           (NOT fact_changed AND department_reconfirmation_status = 'NOT_REQUIRED'));

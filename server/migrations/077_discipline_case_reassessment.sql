-- A voided decision remains available for audit. A later crossing of the same
-- threshold must be able to create a new case with its own approval deadline.
DO $$
DECLARE
    original_key name;
BEGIN
    SELECT conname INTO STRICT original_key FROM pg_constraint
    WHERE conrelid = 'discipline_cases'::regclass AND contype = 'u'
      AND pg_get_constraintdef(oid) = 'UNIQUE (semester_id, student_number, group_key, tier_code)';
    EXECUTE format('ALTER TABLE discipline_cases DROP CONSTRAINT %I', original_key);
END $$;

CREATE UNIQUE INDEX discipline_cases_active_tier
    ON discipline_cases (semester_id, student_number, group_key, tier_code)
    WHERE status <> 'VOID';

ALTER TABLE discipline_cases
    ADD COLUMN void_reason text,
    ADD COLUMN voided_at timestamptz,
    ADD COLUMN reassessment_rule_version_id uuid REFERENCES discipline_rule_versions(id),
    ADD COLUMN reassessment_report_version_id uuid REFERENCES weekly_report_versions(id),
    ADD COLUMN previous_case_id uuid REFERENCES discipline_cases(id);

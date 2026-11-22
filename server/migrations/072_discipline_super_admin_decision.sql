ALTER TABLE discipline_cases DROP CONSTRAINT discipline_cases_decision_source_check;
ALTER TABLE discipline_cases ADD CONSTRAINT discipline_cases_decision_source_check
    CHECK (decision_source IN ('TEACHER','SUPER_ADMIN','TIMEOUT','RULE'));

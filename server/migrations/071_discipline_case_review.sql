ALTER TABLE discipline_cases
    ADD COLUMN responsibility_class_id uuid,
    ADD COLUMN row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
    ADD COLUMN delay_reason text,
    ADD COLUMN decision_source text CHECK (decision_source IN ('TEACHER','TIMEOUT','RULE')),
    ADD COLUMN decided_at timestamptz,
    ADD COLUMN decided_by uuid REFERENCES accounts(id);

-- Before teacher review was introduced, only exempt tiers could be generated as approved.
UPDATE discipline_cases SET decision_source='RULE', decided_at=created_at WHERE status='APPROVED';

CREATE INDEX discipline_cases_pending_deadline ON discipline_cases(due_at,id) WHERE status='PENDING';

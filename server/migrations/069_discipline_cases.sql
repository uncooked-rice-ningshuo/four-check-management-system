CREATE TABLE discipline_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    semester_id uuid NOT NULL REFERENCES semesters(id),
    student_number text NOT NULL,
    group_key text NOT NULL,
    tier_code text NOT NULL,
    report_version_id uuid NOT NULL REFERENCES weekly_report_versions(id),
    rule_version_id uuid NOT NULL REFERENCES discipline_rule_versions(id),
    accumulated_amount numeric NOT NULL,
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    status text NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'DEFERRED', 'VOID')),
    created_at timestamptz NOT NULL DEFAULT now(),
    due_at timestamptz,
    CHECK (status <> 'PENDING' OR due_at IS NOT NULL),
    UNIQUE (semester_id, student_number, group_key, tier_code)
);

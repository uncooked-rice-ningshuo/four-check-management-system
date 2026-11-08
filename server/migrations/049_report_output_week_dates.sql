-- A weekly issuing date is presentation configuration, independent of the
-- immutable report data version and the global Word/Excel layout settings.
CREATE TABLE report_output_week_dates (
    week_id uuid PRIMARY KEY,
    semester_id uuid NOT NULL,
    issue_date date NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT report_output_week_dates_period_fkey FOREIGN KEY (week_id, semester_id)
        REFERENCES semester_weeks(id, semester_id) ON DELETE RESTRICT
);

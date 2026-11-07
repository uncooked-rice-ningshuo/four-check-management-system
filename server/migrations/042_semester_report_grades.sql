-- A semester owns the explicit grade scope for every weekly report within it.
CREATE TABLE semester_report_grade_scopes (
    semester_id uuid PRIMARY KEY REFERENCES semesters(id),
    version integer NOT NULL DEFAULT 0 CHECK (version >= 0),
    updated_by uuid REFERENCES accounts(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE semester_report_grades (
    semester_id uuid NOT NULL REFERENCES semester_report_grade_scopes(semester_id),
    grade_id uuid NOT NULL REFERENCES grades(id),
    PRIMARY KEY (semester_id, grade_id)
);
CREATE INDEX idx_semester_report_grades_grade ON semester_report_grades(grade_id);

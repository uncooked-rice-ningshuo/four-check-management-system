ALTER TABLE academic_years ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK (version > 0);
CREATE TABLE academic_year_grade_stages (
 academic_year_id uuid NOT NULL REFERENCES academic_years(id),
 stage smallint NOT NULL CHECK (stage BETWEEN 1 AND 4),
 grade_id uuid NOT NULL REFERENCES grades(id),
 PRIMARY KEY (academic_year_id, stage),
 UNIQUE (academic_year_id, grade_id)
);

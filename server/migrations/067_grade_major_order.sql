-- Start each cohort with the sequence users saw before ordering became independent.
ALTER TABLE academic_year_roster_grade_majors ADD COLUMN sort_order integer;
UPDATE academic_year_roster_grade_majors gm
SET sort_order=m.sort_order FROM academic_year_roster_majors m
WHERE m.academic_year_id=gm.academic_year_id AND m.major_id=gm.major_id;
ALTER TABLE academic_year_roster_grade_majors
    ALTER COLUMN sort_order SET NOT NULL,
    ADD CHECK (sort_order >= 0),
    ADD UNIQUE (academic_year_id,grade_id,sort_order);

CREATE OR REPLACE VIEW academic_report_classes AS
SELECT c.academic_year_id,c.grade_id,c.class_id,gm.sort_order AS major_order,c.sort_order AS class_order
FROM academic_year_roster_classes c JOIN academic_year_roster_grade_majors gm USING(academic_year_id,grade_id,major_id)
UNION ALL
SELECT s.academic_year_id,s.grade_id,c.id,0,0 FROM academic_year_grade_stages s JOIN classes c ON c.grade_id=s.grade_id
WHERE NOT EXISTS (SELECT 1 FROM academic_year_rosters r WHERE r.academic_year_id=s.academic_year_id);

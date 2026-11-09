-- A class name identifies a class only together with its grade, faculty and major.
-- NULLS NOT DISTINCT keeps unassigned faculties/majors from bypassing uniqueness.
ALTER TABLE classes DROP CONSTRAINT classes_name_key;
ALTER TABLE classes ADD CONSTRAINT classes_scope_name_key
    UNIQUE NULLS NOT DISTINCT (grade_id, faculty_id, major_id, name);

CREATE TABLE IF NOT EXISTS grades (
    id uuid PRIMARY KEY,
    code varchar(16) NOT NULL UNIQUE,
    name varchar(64) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO grades(id, code, name)
SELECT gen_random_uuid(), grade, grade || '级'
FROM (
    SELECT grade FROM classes
    UNION
    SELECT grade FROM students
) existing_grades
WHERE grade <> ''
ON CONFLICT (code) DO NOTHING;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'classes_grade_fkey') THEN
        ALTER TABLE classes ADD CONSTRAINT classes_grade_fkey FOREIGN KEY (grade) REFERENCES grades(code);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'students_grade_fkey') THEN
        ALTER TABLE students ADD CONSTRAINT students_grade_fkey FOREIGN KEY (grade) REFERENCES grades(code);
    END IF;
END $$;

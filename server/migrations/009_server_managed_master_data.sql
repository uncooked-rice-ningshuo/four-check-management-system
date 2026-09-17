ALTER TABLE classes DROP CONSTRAINT IF EXISTS classes_grade_fkey;
ALTER TABLE students DROP CONSTRAINT IF EXISTS students_grade_fkey;
DROP INDEX IF EXISTS idx_students_grade;

ALTER TABLE classes ADD COLUMN grade_id uuid REFERENCES grades(id);
ALTER TABLE students ADD COLUMN grade_id uuid REFERENCES grades(id);

UPDATE classes value
SET grade_id = grade.id
FROM grades grade
WHERE grade.code = value.grade;

UPDATE students value
SET grade_id = grade.id
FROM grades grade
WHERE grade.code = value.grade;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM classes WHERE grade_id IS NULL) OR
       EXISTS (SELECT 1 FROM students WHERE grade_id IS NULL) THEN
        RAISE EXCEPTION 'cannot migrate class/student grade references to grades.id';
    END IF;
END $$;

ALTER TABLE classes ALTER COLUMN grade_id SET NOT NULL;
ALTER TABLE students ALTER COLUMN grade_id SET NOT NULL;
ALTER TABLE classes DROP COLUMN grade;
ALTER TABLE students DROP COLUMN grade;
CREATE INDEX idx_classes_grade ON classes(grade_id, enabled);
CREATE INDEX idx_students_grade ON students(grade_id, enabled);

ALTER TABLE dormitories DROP CONSTRAINT IF EXISTS dormitories_campus_area_building_no_room_no_key;
ALTER TABLE dormitories DROP COLUMN campus_area;
ALTER TABLE dormitories DROP COLUMN building_no;
ALTER TABLE dormitories ADD CONSTRAINT dormitories_building_room_key UNIQUE(dormitory_building_id, room_no);

CREATE TABLE course_classes (
    course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    class_id uuid NOT NULL REFERENCES classes(id),
    PRIMARY KEY(course_id, class_id)
);
CREATE INDEX idx_course_classes_class ON course_classes(class_id, course_id);

INSERT INTO course_classes(course_id, class_id)
SELECT id, class_id
FROM courses
WHERE class_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Legacy schedule text cannot be converted reliably without a confirmed parser.
-- Preserve it with its classroom reference for explicit administrator review
-- instead of silently deleting the only copy during the schema transition.
CREATE TABLE course_schedule_migration_backlog (
    course_id uuid PRIMARY KEY REFERENCES courses(id) ON DELETE CASCADE,
    schedule_text varchar(128) NOT NULL DEFAULT '',
    classroom_id uuid REFERENCES classrooms(id),
    preserved_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO course_schedule_migration_backlog(course_id, schedule_text, classroom_id)
SELECT id, schedule_text, classroom_id
FROM courses
WHERE schedule_text <> '' OR classroom_id IS NOT NULL;

CREATE TABLE course_schedules (
    id uuid PRIMARY KEY,
    course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    semester_id uuid NOT NULL REFERENCES semesters(id),
    classroom_id uuid NOT NULL REFERENCES classrooms(id),
    weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    start_section smallint NOT NULL CHECK (start_section BETWEEN 1 AND 10),
    end_section smallint NOT NULL CHECK (end_section BETWEEN start_section AND 10),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(course_id, semester_id, weekday, start_section, end_section)
);
CREATE INDEX idx_course_schedules_semester ON course_schedules(semester_id, weekday, start_section, end_section);

ALTER TABLE courses DROP COLUMN schedule_text;
ALTER TABLE courses DROP COLUMN class_id;
ALTER TABLE courses DROP COLUMN classroom_id;

ALTER TABLE faculties DROP COLUMN code;
ALTER TABLE grades DROP COLUMN code;
ALTER TABLE majors DROP COLUMN code;
ALTER TABLE classes DROP COLUMN code;
ALTER TABLE dormitory_areas DROP COLUMN code;
ALTER TABLE dormitory_buildings DROP COLUMN code;
ALTER TABLE dormitories DROP COLUMN code;
ALTER TABLE classrooms DROP COLUMN code;
ALTER TABLE courses DROP COLUMN code;

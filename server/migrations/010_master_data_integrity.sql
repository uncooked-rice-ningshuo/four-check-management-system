-- Course names are the canonical human-readable identifier after removing
-- administrator-maintained codes. The exclusion constraint is the database
-- backstop for concurrent writes that bypass application overlap validation.
ALTER TABLE courses ADD CONSTRAINT courses_name_key UNIQUE(name);

CREATE EXTENSION IF NOT EXISTS btree_gist;
ALTER TABLE course_schedules ADD CONSTRAINT course_schedules_no_overlap
    EXCLUDE USING gist (
        course_id WITH =,
        semester_id WITH =,
        weekday WITH =,
        int4range(start_section, end_section, '[]') WITH &&
    );

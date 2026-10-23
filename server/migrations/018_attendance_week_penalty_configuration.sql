-- Existing weeks have no verified punishment rule. NULL keeps that absence
-- explicit until an administrator classifies the week; it is never 1 by default.
ALTER TABLE semester_weeks
    ADD COLUMN attendance_penalty_multiplier smallint
    CHECK (attendance_penalty_multiplier IN (1, 2));

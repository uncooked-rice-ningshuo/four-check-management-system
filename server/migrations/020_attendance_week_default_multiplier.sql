-- Unmarked weeks are ordinary weeks. Only an explicit teaching-ceremony week
-- classification uses 2x; historical record snapshots remain untouched.
ALTER TABLE semester_weeks
    ALTER COLUMN attendance_penalty_multiplier SET DEFAULT 1;
UPDATE semester_weeks
SET attendance_penalty_multiplier = 1
WHERE attendance_penalty_multiplier IS NULL;
ALTER TABLE semester_weeks
    ALTER COLUMN attendance_penalty_multiplier SET NOT NULL;

-- Historical absence_hours may already have been counted under an unknown rule.
-- Keep both snapshot columns NULL for existing rows instead of backfilling guesses.
ALTER TABLE attendance_details
    ADD COLUMN penalty_multiplier smallint CHECK (penalty_multiplier IN (1, 2)),
    ADD COLUMN penalty_hours numeric(7,2)
        GENERATED ALWAYS AS (absence_hours * penalty_multiplier) STORED;

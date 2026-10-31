-- Preserve old test data and snapshots; new evening records use incident counts.
ALTER TABLE evening_self_study_details ALTER COLUMN absence_hours DROP NOT NULL;
ALTER TABLE evening_self_study_details ALTER COLUMN penalty_hours DROP NOT NULL;
ALTER TABLE evening_self_study_details ADD COLUMN penalty_count smallint;
ALTER TABLE evening_self_study_details ADD CONSTRAINT evening_penalty_count_matches_multiplier CHECK (penalty_count IS NULL OR penalty_count = penalty_multiplier);

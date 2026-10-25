-- Keep the current approved five-type reporting scope explicit. The two
-- paused hygiene types remain configurable for future, separately approved work.
UPDATE inspection_types
SET participates_in_report = false
WHERE code IN ('CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE');

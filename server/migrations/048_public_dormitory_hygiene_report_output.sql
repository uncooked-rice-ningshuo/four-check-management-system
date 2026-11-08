-- Include the eighth configured inspection dimension even when no submission exists.
UPDATE inspection_types
SET participates_in_report = true
WHERE code = 'PUBLIC_DORMITORY_HYGIENE';

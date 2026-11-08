-- Classroom hygiene is now an approved selectable report attachment.
-- Existing Word/Excel selections remain unchanged until an administrator saves them.
UPDATE inspection_types
SET participates_in_report = true
WHERE code = 'CLASSROOM_HYGIENE';

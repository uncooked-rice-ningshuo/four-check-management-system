-- These inspection actions have no delivered endpoint. Export remains a department
-- configuration intent for future minister/deputy export, never a personal rule.
WITH removed AS (
    DELETE FROM user_permission_rules
    WHERE action IN ('INSPECTION_SUMMARIZE', 'INSPECTION_EXPORT')
    RETURNING account_id
)
UPDATE accounts SET session_version=session_version+1,updated_at=now()
WHERE id IN (SELECT account_id FROM removed);

UPDATE department_inspection_scopes AS scope
SET can_export=false,updated_at=now()
FROM departments AS department
WHERE scope.department_id=department.id
  AND department.code NOT IN ('STUDY','SPORTS','DORMITORY')
  AND scope.can_export;

ALTER TABLE department_inspection_scopes DROP COLUMN can_summarize;

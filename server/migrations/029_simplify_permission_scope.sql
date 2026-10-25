-- User-specific inspection permissions now use only academic year, semester,
-- week, and inspection type. A scoped ALLOW becomes a wider type/time grant;
-- a scoped DENY cannot safely become global, so it is disabled instead.
-- Keep this connection-local snapshot until the migration connection closes.
-- Some supported runners execute a SQL file in autocommit mode.
CREATE TEMP TABLE affected_permission_rules AS
SELECT id, account_id, action, effect, academic_year_id, semester_id, week_id,
       inspection_type_id, department_id, class_id, grade, enabled
FROM user_permission_rules
WHERE department_id IS NOT NULL OR class_id IS NOT NULL OR grade IS NOT NULL;

WITH changed AS (
    UPDATE user_permission_rules AS rules
    SET department_id = NULL,
        class_id = NULL,
        grade = NULL,
        enabled = CASE WHEN rules.effect = 'DENY' THEN false ELSE rules.enabled END,
        updated_at = now()
    FROM affected_permission_rules AS affected
    WHERE rules.id = affected.id
    RETURNING rules.id, rules.account_id, rules.action, rules.effect,
              rules.academic_year_id, rules.semester_id, rules.week_id,
              rules.inspection_type_id, rules.department_id, rules.class_id,
              rules.grade, rules.enabled
)
INSERT INTO audit_logs(id, actor_id, action, resource_type, resource_id, before_json, after_json, reason)
SELECT gen_random_uuid(), NULL, 'PERMISSION_SCOPE_SIMPLIFIED', 'permissionRule', changed.id::text,
       jsonb_build_object(
           'accountId', affected.account_id,
           'action', affected.action,
           'effect', affected.effect,
           'scope', jsonb_build_object(
               'academicYearId', affected.academic_year_id,
               'semesterId', affected.semester_id,
               'weekId', affected.week_id,
               'inspectionTypeId', affected.inspection_type_id,
               'departmentId', affected.department_id,
               'classId', affected.class_id,
               'grade', affected.grade
           ),
           'enabled', affected.enabled
       ),
       jsonb_build_object(
           'accountId', changed.account_id,
           'action', changed.action,
           'effect', changed.effect,
           'scope', jsonb_build_object(
               'academicYearId', changed.academic_year_id,
               'semesterId', changed.semester_id,
               'weekId', changed.week_id,
               'inspectionTypeId', changed.inspection_type_id,
               'departmentId', changed.department_id,
               'classId', changed.class_id,
               'grade', changed.grade
           ),
           'enabled', changed.enabled
       ),
       CASE WHEN affected.effect = 'DENY'
           THEN '停用依赖部门、班级或年级范围的拒绝规则'
           ELSE '移除部门、班级和年级授权范围'
       END
FROM affected_permission_rules AS affected
JOIN changed ON changed.id = affected.id;

UPDATE accounts AS account
SET session_version = account.session_version + 1,
    updated_at = now()
WHERE account.id IN (SELECT DISTINCT account_id FROM affected_permission_rules);

UPDATE refresh_sessions AS session
SET revoked_at = COALESCE(session.revoked_at, now())
WHERE session.account_id IN (SELECT DISTINCT account_id FROM affected_permission_rules)
  AND session.revoked_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'user_permission_rules_time_type_scope_only'
          AND conrelid = 'user_permission_rules'::regclass
    ) THEN
        ALTER TABLE user_permission_rules
            ADD CONSTRAINT user_permission_rules_time_type_scope_only
            CHECK (department_id IS NULL AND class_id IS NULL AND grade IS NULL);
    END IF;
END $$;

ALTER TABLE departments ADD COLUMN discipline_scope text[] NOT NULL DEFAULT '{}';

-- These codes only seed the editable department configuration.
UPDATE departments SET discipline_scope = CASE WHEN code = 'OFFICE' THEN
 ARRAY['DISCIPLINE_RULE_VIEW','DISCIPLINE_RULE_EDIT','DISCIPLINE_RULE_PUBLISH',
       'DISCIPLINE_LEDGER_VIEW','DISCIPLINE_LEDGER_EXPORT','DISCIPLINE_CASE_VIEW',
       'DISCIPLINE_CASE_CONFIRM','DISCIPLINE_ARCHIVE_VIEW','DISCIPLINE_ARCHIVE_EXPORT']
 ELSE ARRAY['DISCIPLINE_CASE_VIEW','DISCIPLINE_CASE_CONFIRM'] END
WHERE code IN ('OFFICE','GRADE_ONE','GRADE_TWO','GRADE_THREE','GRADE_FOUR');

UPDATE departments d SET role_policies = (
 SELECT jsonb_agg((p - 'canConfirmDiscipline') || jsonb_build_object('disciplineActions',
    (CASE WHEN d.code='OFFICE' THEN ARRAY['DISCIPLINE_ARCHIVE_VIEW'] ELSE ARRAY[]::text[] END)
    || (CASE WHEN d.discipline_scope @> ARRAY['DISCIPLINE_CASE_CONFIRM']
          AND p @> '{"canConfirmDiscipline":true}' THEN ARRAY['DISCIPLINE_CASE_CONFIRM']
        ELSE ARRAY[]::text[] END)) ORDER BY n)
 FROM jsonb_array_elements(d.role_policies) WITH ORDINALITY AS policies(p,n)
), permission_version=permission_version+1 WHERE jsonb_array_length(role_policies)>0;

-- Translate assigned snapshots without granting newly seeded role defaults.
UPDATE user_org_roles u SET default_permissions=jsonb_set(default_permissions,'{rolePolicies}',(
 SELECT jsonb_agg((p - 'canConfirmDiscipline') || jsonb_build_object('disciplineActions',
    CASE WHEN p @> '{"canConfirmDiscipline":true}' THEN ARRAY['DISCIPLINE_CASE_CONFIRM']
         ELSE ARRAY[]::text[] END) ORDER BY n)
 FROM jsonb_array_elements(u.default_permissions->'rolePolicies') WITH ORDINALITY AS policies(p,n)
)) WHERE jsonb_array_length(default_permissions->'rolePolicies')>0;

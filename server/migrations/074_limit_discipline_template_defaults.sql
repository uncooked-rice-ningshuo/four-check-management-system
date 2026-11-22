-- Remove the blanket teacher default introduced by 073 from unrelated
-- departments. Assigned snapshots and explicit personal grants have a
-- separate lifecycle and must not be rewritten by a template correction.
UPDATE departments AS d
SET role_policies = (
    SELECT jsonb_agg(
        CASE WHEN policy->>'roleCode' = 'TEACHER'
             THEN policy || '{"canConfirmDiscipline": false}'::jsonb
             ELSE policy END ORDER BY position
    )
    FROM jsonb_array_elements(d.role_policies) WITH ORDINALITY AS roles(policy, position)
), permission_version = permission_version + 1
WHERE d.code NOT IN ('OFFICE', 'GRADE_ONE', 'GRADE_TWO', 'GRADE_THREE', 'GRADE_FOUR')
  AND d.role_policies @> '[{"roleCode":"TEACHER","canConfirmDiscipline":true}]'::jsonb;

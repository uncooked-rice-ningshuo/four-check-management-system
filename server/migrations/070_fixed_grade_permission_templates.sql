-- Preserve genuinely unrestricted actions. Losing the last legacy alternative
-- must disable its role action before the empty scope can mean unrestricted.
DO $$
DECLARE
    department record;
    policy jsonb;
    action_scope jsonb;
    fixed_ranges jsonb;
    actions jsonb;
    policies jsonb;
BEGIN
    -- Session claims and permission screens must refresh after active rules change.
    UPDATE accounts SET session_version=session_version+1, updated_at=now()
    WHERE id IN (
        SELECT account_id FROM user_permission_rules
        WHERE enabled AND operation_ranges @? '$[*].gradeStage'
        UNION
        SELECT u.account_id FROM user_org_roles u
        JOIN organization_roles r ON r.id=u.organization_role_id
        CROSS JOIN LATERAL jsonb_array_elements(NULLIF(u.default_permissions->'rolePolicies','null'::jsonb)) p
        WHERE p->>'roleCode'=r.code AND p @? '$.operationRanges[*].ranges[*].gradeStage'
    );
    FOR department IN SELECT id, role_policies FROM departments
        WHERE role_policies @? '$[*].operationRanges[*].ranges[*].gradeStage' FOR UPDATE
    LOOP
        policies := '[]';
        FOR policy IN SELECT value FROM jsonb_array_elements(department.role_policies)
        LOOP
            actions := '[]';
            FOR action_scope IN SELECT value FROM jsonb_array_elements(NULLIF(policy->'operationRanges','null'::jsonb))
            LOOP
                IF action_scope @? '$.ranges[*].gradeStage' THEN
                    SELECT jsonb_agg(value ORDER BY ord) INTO fixed_ranges
                    FROM jsonb_array_elements(action_scope->'ranges') WITH ORDINALITY AS clauses(value,ord)
                    WHERE NOT (value ? 'gradeStage');
                    IF fixed_ranges IS NULL THEN
                        UPDATE department_inspection_role_defaults SET
                            can_enter = can_enter AND action_scope->>'action' <> 'INSPECTION_ENTER',
                            can_manage = can_manage AND action_scope->>'action' <> 'INSPECTION_MANAGE',
                            can_review = can_review AND action_scope->>'action' <> 'INSPECTION_REVIEW'
                        WHERE department_id=department.id AND role_code=policy->>'roleCode'
                          AND inspection_type_id=(action_scope->>'inspectionTypeId')::uuid;
                        CONTINUE;
                    END IF;
                    action_scope := jsonb_set(action_scope,'{ranges}',fixed_ranges);
                END IF;
                actions := actions || jsonb_build_array(action_scope);
            END LOOP;
            policies := policies || jsonb_build_array(jsonb_set(policy,'{operationRanges}',actions));
        END LOOP;
        UPDATE departments SET role_policies=policies, permission_version=permission_version+1,
            updated_at=now() WHERE id=department.id;
    END LOOP;

    -- A retired override must continue suppressing its assigned baseline. Dropping
    -- the override instead could restore an unrestricted permission.
    WITH cleaned AS (
        SELECT r.id, (
            SELECT jsonb_agg(value ORDER BY ord)
            FROM jsonb_array_elements(r.operation_ranges) WITH ORDINALITY AS clauses(value,ord)
            WHERE NOT (value ? 'gradeStage')
        ) AS remaining
        FROM user_permission_rules r WHERE r.operation_ranges @? '$[*].gradeStage'
    )
    UPDATE user_permission_rules r SET
        updated_at=now(),
        operation_ranges=COALESCE(cleaned.remaining,'[]'::jsonb),
        enabled=r.enabled AND (cleaned.remaining IS NOT NULL OR r.mode IN ('OVERRIDE','DISABLE')),
        mode=CASE WHEN cleaned.remaining IS NULL AND r.mode='OVERRIDE' THEN 'DISABLE' ELSE r.mode END
    FROM cleaned WHERE r.id=cleaned.id;
END $$;

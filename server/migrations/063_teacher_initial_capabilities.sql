-- Initial configuration keeps existing morning-plan use and gives the report owner
-- a separate file-output capability. These are editable defaults, not role gates.
UPDATE department_report_scopes SET can_file_output=true,can_force_generate=true,can_obligation_manage=true WHERE enabled;
UPDATE department_report_role_defaults SET can_file_output=true
WHERE department_id IN (SELECT department_id FROM department_report_scopes WHERE enabled);
UPDATE department_inspection_role_defaults SET can_enter=false,can_manage=false,can_review=false WHERE role_code='PRESIDIUM';
UPDATE departments SET can_manage_morning_schedule=true WHERE code='SPORTS';
UPDATE departments d SET role_policies=(SELECT jsonb_agg(CASE WHEN policy->>'roleCode' IN ('MINISTER','VICE_MINISTER')
 THEN jsonb_set(policy,'{canManageMorningSchedule}','true') ELSE policy END)
 FROM jsonb_array_elements(d.role_policies) policy) WHERE d.code='SPORTS';

INSERT INTO organization_roles(id,code,name,sort_order) VALUES(gen_random_uuid(),'TEACHER','老师',50) ON CONFLICT(code) DO NOTHING;
INSERT INTO departments(id,code,name) VALUES
 (gen_random_uuid(),'GRADE_ONE','大一年级部'),(gen_random_uuid(),'GRADE_TWO','大二年级部'),
 (gen_random_uuid(),'GRADE_THREE','大三年级部'),(gen_random_uuid(),'GRADE_FOUR','大四年级部') ON CONFLICT(code) DO NOTHING;
ALTER TABLE department_inspection_role_defaults DROP CONSTRAINT department_inspection_role_defaults_by_type_role_code_check;
INSERT INTO department_inspection_role_defaults(department_id,inspection_type_id,role_code,can_view,can_enter,can_manage,can_review,can_export)
SELECT department_id,inspection_type_id,new_role,can_view,can_enter,can_manage,can_review,can_export
FROM department_inspection_role_defaults CROSS JOIN (VALUES('MINISTER'),('VICE_MINISTER')) roles(new_role) WHERE role_code='LEADERSHIP';
DELETE FROM department_inspection_role_defaults WHERE role_code='LEADERSHIP';
ALTER TABLE department_inspection_role_defaults ADD CHECK(role_code IN ('MEMBER','MINISTER','VICE_MINISTER','PRESIDIUM','TEACHER'));
ALTER TABLE department_report_role_defaults DROP CONSTRAINT department_report_role_defaults_role_code_check, DROP CONSTRAINT office_member_report_boundary;
ALTER TABLE department_report_role_defaults ADD CHECK(role_code IN ('MEMBER','MINISTER','VICE_MINISTER','PRESIDIUM','TEACHER'));
INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_view,can_enter,can_manage,can_review)
SELECT d.id,t.id,true,true,true,true,true FROM departments d CROSS JOIN inspection_types t
WHERE d.code IN ('GRADE_ONE','GRADE_TWO','GRADE_THREE','GRADE_FOUR') AND t.code IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM') ON CONFLICT DO NOTHING;
INSERT INTO department_inspection_role_defaults(department_id,inspection_type_id,role_code,can_view,can_enter,can_manage,can_review)
SELECT s.department_id,s.inspection_type_id,r.code,true,r.code<>'TEACHER',r.code IN ('MINISTER','VICE_MINISTER'),r.code IN ('MINISTER','VICE_MINISTER')
FROM department_inspection_scopes s CROSS JOIN organization_roles r
WHERE s.enabled AND r.code IN ('MEMBER','MINISTER','VICE_MINISTER','PRESIDIUM','TEACHER') ON CONFLICT DO NOTHING;
INSERT INTO department_report_role_defaults(department_id,role_code,can_view,can_archive,can_file_output)
SELECT d.department_id,r.code,true,true,true FROM department_report_scopes d CROSS JOIN organization_roles r
WHERE d.enabled AND r.code IN ('TEACHER','PRESIDIUM') ON CONFLICT DO NOTHING;
ALTER TABLE departments ADD COLUMN role_policies jsonb NOT NULL DEFAULT '[]', ADD COLUMN can_manage_morning_schedule boolean NOT NULL DEFAULT false;
ALTER TABLE user_org_roles ADD COLUMN default_permissions jsonb NOT NULL DEFAULT '{}', ADD COLUMN template_version integer NOT NULL DEFAULT 1;
ALTER TABLE user_org_roles ALTER COLUMN start_at TYPE timestamptz USING start_at::timestamptz, ALTER COLUMN end_at TYPE timestamptz USING end_at::timestamptz;
ALTER TABLE department_report_scopes ADD COLUMN can_force_generate boolean NOT NULL DEFAULT false, ADD COLUMN can_obligation_manage boolean NOT NULL DEFAULT false;
ALTER TABLE department_report_role_defaults ADD COLUMN can_force_generate boolean NOT NULL DEFAULT false, ADD COLUMN can_obligation_manage boolean NOT NULL DEFAULT false;

UPDATE departments d SET role_policies=(SELECT jsonb_agg(jsonb_build_object(
 'roleCode',r.code,'validityYears',CASE WHEN r.code='TEACHER' THEN 0 ELSE 1 END,
 'canManageMorningSchedule',false,'operationRanges',COALESCE((SELECT jsonb_agg(jsonb_build_object('action',a.action,'inspectionTypeId',t.id,'ranges',jsonb_build_array(jsonb_build_object('gradeStage',CASE d.code WHEN 'GRADE_ONE' THEN 1 WHEN 'GRADE_TWO' THEN 2 WHEN 'GRADE_THREE' THEN 3 WHEN 'GRADE_FOUR' THEN 4 END)))) FROM inspection_types t CROSS JOIN (VALUES('INSPECTION_ENTER'),('INSPECTION_MANAGE'),('INSPECTION_REVIEW')) a(action) WHERE t.code IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM') AND d.code IN ('GRADE_ONE','GRADE_TWO','GRADE_THREE','GRADE_FOUR')),'[]'::jsonb))) FROM organization_roles r WHERE r.code IN ('MEMBER','MINISTER','VICE_MINISTER','PRESIDIUM','TEACHER'));

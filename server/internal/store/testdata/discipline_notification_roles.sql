-- Each pair has the same role and action; only the assigned class differs.
INSERT INTO accounts(id,login_name,student_no,name,password_hash,status,account_kind,system_role,primary_department_id)
SELECT gen_random_uuid(),x.login,'FX-'||right(md5(x.login),12),x.label,'test-only','ACTIVE','PERSON','MEMBER',d.id
FROM departments d CROSS JOIN (VALUES
 ('discipline-teacher-in','班内老师'),('discipline-teacher-out','班外老师'),
 ('discipline-member-in','班内学生'),('discipline-member-revoke-in','撤权测试学生'),('discipline-member-out','班外学生')) x(login,label)
WHERE d.code='OFFICE';

INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id,start_at,default_permissions)
SELECT gen_random_uuid(),a.id,a.primary_department_id,r.id,now()-interval '1 day',
 jsonb_build_object('version',1,'rolePolicies',jsonb_build_array(jsonb_build_object('roleCode',r.code,
 'disciplineActions',CASE WHEN r.code='TEACHER' THEN ARRAY['DISCIPLINE_CASE_CONFIRM'] ELSE ARRAY[]::text[] END,
 'operationRanges',jsonb_build_array(jsonb_build_object('action','DISCIPLINE_CASE_CONFIRM','ranges',
 jsonb_build_array(jsonb_build_object('classId',c.id)))))))
FROM accounts a JOIN organization_roles r ON r.code=CASE WHEN a.login_name LIKE 'discipline-teacher-%' THEN 'TEACHER' ELSE 'MEMBER' END
JOIN classes c ON c.name=CASE WHEN a.login_name LIKE '%-in' THEN 'Fixture class' ELSE 'Other fixture class' END
WHERE a.login_name LIKE 'discipline-teacher-%' OR a.login_name LIKE 'discipline-member-%';

INSERT INTO user_permission_rules(id,account_id,action,effect,source_type,source_department_id,mode,operation_ranges,starts_at,note,enabled)
SELECT gen_random_uuid(),a.id,'DISCIPLINE_CASE_CONFIRM','ALLOW','MANUAL',a.primary_department_id,'ADD',
 jsonb_build_array(jsonb_build_object('classId',c.id)),now()-interval '1 day','本地隔离验收',true
FROM accounts a JOIN classes c ON c.name=CASE WHEN a.login_name LIKE '%-in' THEN 'Fixture class' ELSE 'Other fixture class' END
WHERE a.login_name LIKE 'discipline-member-%';

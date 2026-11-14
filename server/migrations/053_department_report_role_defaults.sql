ALTER TABLE departments ADD COLUMN permission_version integer NOT NULL DEFAULT 1;

CREATE TABLE department_inspection_role_defaults (
    department_id uuid NOT NULL REFERENCES departments(id),
    role_code varchar(32) NOT NULL CHECK (role_code IN ('MEMBER','LEADERSHIP','PRESIDIUM')),
    can_view boolean NOT NULL DEFAULT false,
    can_enter boolean NOT NULL DEFAULT false,
    can_manage boolean NOT NULL DEFAULT false,
    can_review boolean NOT NULL DEFAULT false,
    can_export boolean NOT NULL DEFAULT false,
    PRIMARY KEY (department_id, role_code),
    CONSTRAINT inspection_role_boundary CHECK (
        role_code='LEADERSHIP' OR (NOT can_manage AND NOT can_review AND NOT can_export)
    ),
    CONSTRAINT presidium_read_only CHECK (role_code<>'PRESIDIUM' OR NOT can_enter)
);

INSERT INTO department_inspection_role_defaults
    (department_id,role_code,can_view,can_enter,can_manage,can_review,can_export)
SELECT id,role_code,true,can_enter,can_manage,can_review,can_export
FROM departments CROSS JOIN (VALUES
    ('MEMBER',true,false,false,false),
    ('LEADERSHIP',true,true,true,true),
    ('PRESIDIUM',false,false,false,false)
) AS roles(role_code,can_enter,can_manage,can_review,can_export)
WHERE code IN ('STUDY','SPORTS','DORMITORY');

INSERT INTO department_inspection_role_defaults(department_id,role_code,can_view)
SELECT id,role_code,true FROM departments
CROSS JOIN (VALUES ('MEMBER'),('LEADERSHIP'),('PRESIDIUM')) AS roles(role_code)
WHERE code='OFFICE';

CREATE TABLE department_report_role_defaults (
    department_id uuid NOT NULL REFERENCES departments(id),
    role_code varchar(32) NOT NULL CHECK (role_code IN ('MEMBER','MINISTER','VICE_MINISTER')),
    can_view boolean NOT NULL DEFAULT false,
    can_generate boolean NOT NULL DEFAULT false,
    can_correct boolean NOT NULL DEFAULT false,
    can_review boolean NOT NULL DEFAULT false,
    can_return boolean NOT NULL DEFAULT false,
    can_finalize boolean NOT NULL DEFAULT false,
    can_publish boolean NOT NULL DEFAULT false,
    can_output_settings boolean NOT NULL DEFAULT false,
    can_archive boolean NOT NULL DEFAULT false,
    PRIMARY KEY (department_id, role_code),
    CONSTRAINT office_member_report_boundary CHECK (
        role_code <> 'MEMBER' OR
        (NOT can_review AND NOT can_return AND NOT can_finalize AND NOT can_publish AND NOT can_output_settings)
    )
);

INSERT INTO department_report_role_defaults
    (department_id,role_code,can_view,can_generate,can_correct,can_review,can_return,
     can_finalize,can_publish,can_output_settings,can_archive)
SELECT id,role_code,true,true,true,leadership,leadership,leadership,leadership,leadership,true
FROM departments
CROSS JOIN (VALUES ('MEMBER',false),('MINISTER',true),('VICE_MINISTER',true)) AS roles(role_code,leadership)
WHERE code='OFFICE';

WITH removed AS (
    DELETE FROM user_permission_rules
    WHERE effect='ALLOW' AND action IN
        ('REPORT_REVIEW','REPORT_RETURN','REPORT_FINALIZE','REPORT_PUBLISH','REPORT_OUTPUT_SETTINGS_MANAGE')
      AND account_id NOT IN (
          SELECT a.id FROM accounts a
          JOIN departments d ON d.id=a.primary_department_id AND d.code='OFFICE'
          JOIN user_org_roles ur ON ur.account_id=a.id AND ur.department_id=d.id
          JOIN organization_roles r ON r.id=ur.organization_role_id AND r.code IN ('MINISTER','VICE_MINISTER')
          WHERE (ur.start_at IS NULL OR ur.start_at<=CURRENT_DATE)
            AND (ur.end_at IS NULL OR ur.end_at>=CURRENT_DATE)
      )
      AND account_id NOT IN (SELECT id FROM accounts WHERE is_super_admin)
    RETURNING account_id
)
UPDATE accounts SET session_version=session_version+1,updated_at=now()
WHERE id IN (SELECT account_id FROM removed);

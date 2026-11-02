CREATE TABLE IF NOT EXISTS departments (
    id uuid PRIMARY KEY,
    code varchar(32) NOT NULL UNIQUE,
    name varchar(64) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization_roles (
    id uuid PRIMARY KEY,
    code varchar(32) NOT NULL UNIQUE,
    name varchar(64) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS inspection_types (
    id uuid PRIMARY KEY,
    code varchar(48) NOT NULL UNIQUE,
    name varchar(64) NOT NULL UNIQUE,
    extension_kind varchar(48) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    participates_in_report boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0
);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS login_name varchar(64);
UPDATE accounts SET login_name = student_no WHERE login_name IS NULL;
ALTER TABLE accounts ALTER COLUMN login_name SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_accounts_login_name ON accounts(login_name);
ALTER TABLE accounts ALTER COLUMN student_no DROP NOT NULL;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS account_kind varchar(16) NOT NULL DEFAULT 'PERSON';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_super_admin boolean NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS must_change_password boolean NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_version integer NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS primary_department_id uuid REFERENCES departments(id);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS requested_department_id uuid REFERENCES departments(id);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS requested_org_role_id uuid REFERENCES organization_roles(id);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS reviewed_by uuid REFERENCES accounts(id);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS review_reason varchar(500);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'accounts_account_kind_check') THEN
        ALTER TABLE accounts ADD CONSTRAINT accounts_account_kind_check CHECK (account_kind IN ('SYSTEM','PERSON'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'accounts_person_student_no_check') THEN
        ALTER TABLE accounts ADD CONSTRAINT accounts_person_student_no_check
            CHECK ((account_kind = 'SYSTEM' AND student_no IS NULL) OR (account_kind = 'PERSON' AND student_no IS NOT NULL));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS user_org_roles (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    department_id uuid NOT NULL REFERENCES departments(id),
    organization_role_id uuid NOT NULL REFERENCES organization_roles(id),
    start_at date,
    end_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(account_id, department_id, organization_role_id)
);

CREATE TABLE IF NOT EXISTS department_inspection_scopes (
    department_id uuid NOT NULL REFERENCES departments(id),
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    can_enter boolean NOT NULL DEFAULT false,
    can_view boolean NOT NULL DEFAULT false,
    can_review boolean NOT NULL DEFAULT false,
    can_export boolean NOT NULL DEFAULT false,
    can_summarize boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(department_id, inspection_type_id)
);

CREATE TABLE IF NOT EXISTS academic_years (
    id uuid PRIMARY KEY,
    name varchar(32) NOT NULL UNIQUE,
    start_date date NOT NULL,
    end_date date NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (start_date <= end_date)
);

CREATE TABLE IF NOT EXISTS semesters (
    id uuid PRIMARY KEY,
    academic_year_id uuid NOT NULL REFERENCES academic_years(id),
    code varchar(32) NOT NULL,
    name varchar(64) NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(academic_year_id, code),
    CHECK (start_date <= end_date)
);

CREATE TABLE IF NOT EXISTS semester_weeks (
    id uuid PRIMARY KEY,
    semester_id uuid NOT NULL REFERENCES semesters(id) ON DELETE CASCADE,
    week_no integer NOT NULL CHECK (week_no BETWEEN 1 AND 60),
    start_date date NOT NULL,
    end_date date NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    note varchar(255) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(semester_id, week_no),
    CHECK (start_date <= end_date)
);
CREATE INDEX IF NOT EXISTS idx_semester_weeks_date ON semester_weeks(start_date, end_date) WHERE enabled;

INSERT INTO departments(id, code, name) VALUES
    ('10000000-0000-0000-0000-000000000001','OFFICE','学生会办公室'),
    ('10000000-0000-0000-0000-000000000002','STUDY','学习部'),
    ('10000000-0000-0000-0000-000000000003','DORMITORY','宿管部'),
    ('10000000-0000-0000-0000-000000000004','SPORTS','体育部')
ON CONFLICT (code) DO NOTHING;

INSERT INTO organization_roles(id, code, name, sort_order) VALUES
    ('20000000-0000-0000-0000-000000000001','PRESIDIUM','主席团成员',10),
    ('20000000-0000-0000-0000-000000000002','MINISTER','部门部长',20),
    ('20000000-0000-0000-0000-000000000003','VICE_MINISTER','部门副部长',30),
    ('20000000-0000-0000-0000-000000000004','MEMBER','部门委员',40)
ON CONFLICT (code) DO NOTHING;

INSERT INTO inspection_types(id, code, name, extension_kind, sort_order) VALUES
    ('30000000-0000-0000-0000-000000000001','ATTENDANCE_RATE','到课率','COURSE_ATTENDANCE',10),
    ('30000000-0000-0000-0000-000000000002','UNATTENDED_CLASSROOM','无机课堂','COURSE_ATTENDANCE',20),
    ('30000000-0000-0000-0000-000000000003','CLASSROOM_HYGIENE','教室卫生','CLASSROOM_HYGIENE',30),
    ('30000000-0000-0000-0000-000000000004','MORNING_ATTENDANCE','早锻炼/早签','MORNING_ATTENDANCE',40),
    ('30000000-0000-0000-0000-000000000005','DORMITORY_HYGIENE','宿舍卫生','DORMITORY_HYGIENE',50),
    ('30000000-0000-0000-0000-000000000006','LATE_RETURN','晚归','LATE_RETURN',60),
    ('30000000-0000-0000-0000-000000000007','PUBLIC_DORMITORY_HYGIENE','公共宿舍卫生','PUBLIC_DORMITORY_HYGIENE',70)
ON CONFLICT (code) DO NOTHING;

INSERT INTO department_inspection_scopes(department_id, inspection_type_id, can_enter, can_view, can_review, can_export, can_summarize)
SELECT d.id, i.id, true, true, true, true, false
FROM departments d JOIN inspection_types i ON
    (d.code = 'STUDY' AND i.code IN ('ATTENDANCE_RATE','UNATTENDED_CLASSROOM','CLASSROOM_HYGIENE')) OR
    (d.code = 'SPORTS' AND i.code = 'MORNING_ATTENDANCE') OR
    (d.code = 'DORMITORY' AND i.code IN ('DORMITORY_HYGIENE','LATE_RETURN','PUBLIC_DORMITORY_HYGIENE'))
ON CONFLICT (department_id, inspection_type_id) DO NOTHING;

INSERT INTO department_inspection_scopes(department_id, inspection_type_id, can_enter, can_view, can_review, can_export, can_summarize)
SELECT d.id, i.id, false, true, false, true, true
FROM departments d CROSS JOIN inspection_types i
WHERE d.code = 'OFFICE'
ON CONFLICT (department_id, inspection_type_id) DO NOTHING;

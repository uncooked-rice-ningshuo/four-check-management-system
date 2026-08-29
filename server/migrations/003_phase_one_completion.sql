ALTER TABLE accounts ADD COLUMN IF NOT EXISTS system_role varchar(24) NOT NULL DEFAULT 'MEMBER';

UPDATE accounts SET system_role = 'SUPER_ADMIN' WHERE is_super_admin;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'accounts_system_role_check') THEN
        ALTER TABLE accounts ADD CONSTRAINT accounts_system_role_check
            CHECK (system_role IN ('SUPER_ADMIN', 'ADMIN', 'MEMBER'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS user_permission_rules (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    action varchar(48) NOT NULL,
    effect varchar(8) NOT NULL CHECK (effect IN ('ALLOW', 'DENY')),
    source_type varchar(32) NOT NULL DEFAULT 'USER_RULE',
    academic_year_id uuid REFERENCES academic_years(id),
    semester_id uuid REFERENCES semesters(id),
    week_id uuid REFERENCES semester_weeks(id),
    inspection_type_id uuid REFERENCES inspection_types(id),
    department_id uuid REFERENCES departments(id),
    class_id uuid,
    grade varchar(16),
    note varchar(255) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_permission_rules_account ON user_permission_rules(account_id, enabled, action);

CREATE TABLE IF NOT EXISTS faculties (
    id uuid PRIMARY KEY,
    code varchar(32) NOT NULL UNIQUE,
    name varchar(96) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS majors (
    id uuid PRIMARY KEY,
    faculty_id uuid NOT NULL REFERENCES faculties(id),
    code varchar(32) NOT NULL,
    name varchar(96) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(faculty_id, code),
    UNIQUE(faculty_id, name)
);

CREATE TABLE IF NOT EXISTS staff (
    id uuid PRIMARY KEY,
    employee_no varchar(32) NOT NULL UNIQUE,
    name varchar(64) NOT NULL,
    gender varchar(16) NOT NULL DEFAULT 'UNKNOWN',
    phone varchar(32) NOT NULL DEFAULT '',
    faculty_id uuid REFERENCES faculties(id),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS classes (
    id uuid PRIMARY KEY,
    code varchar(32) NOT NULL UNIQUE,
    name varchar(96) NOT NULL UNIQUE,
    grade varchar(16) NOT NULL,
    faculty_id uuid REFERENCES faculties(id),
    major_id uuid REFERENCES majors(id),
    head_teacher_id uuid REFERENCES staff(id),
    enrolled_size integer NOT NULL DEFAULT 0 CHECK (enrolled_size >= 0),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dormitories (
    id uuid PRIMARY KEY,
    code varchar(64) NOT NULL UNIQUE,
    campus_area varchar(64) NOT NULL,
    building_no varchar(32) NOT NULL,
    room_no varchar(32) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(campus_area, building_no, room_no)
);

CREATE TABLE IF NOT EXISTS classrooms (
    id uuid PRIMARY KEY,
    code varchar(64) NOT NULL UNIQUE,
    building varchar(64) NOT NULL,
    room_no varchar(32) NOT NULL,
    floor integer NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(building, room_no)
);

CREATE TABLE IF NOT EXISTS courses (
    id uuid PRIMARY KEY,
    code varchar(32) NOT NULL UNIQUE,
    name varchar(128) NOT NULL,
    teacher_id uuid REFERENCES staff(id),
    class_id uuid REFERENCES classes(id),
    classroom_id uuid REFERENCES classrooms(id),
    schedule_text varchar(128) NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS students (
    id uuid PRIMARY KEY,
    student_no varchar(32) NOT NULL UNIQUE,
    name varchar(64) NOT NULL,
    gender varchar(16) NOT NULL DEFAULT 'UNKNOWN',
    grade varchar(16) NOT NULL,
    faculty_id uuid REFERENCES faculties(id),
    major_id uuid REFERENCES majors(id),
    class_id uuid REFERENCES classes(id),
    dormitory_id uuid REFERENCES dormitories(id),
    enrollment_status varchar(24) NOT NULL DEFAULT 'ENROLLED',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_students_class ON students(class_id, enabled);
CREATE INDEX IF NOT EXISTS idx_students_grade ON students(grade, enabled);

ALTER TABLE user_permission_rules
    DROP CONSTRAINT IF EXISTS user_permission_rules_class_id_fkey;
ALTER TABLE user_permission_rules
    ADD CONSTRAINT user_permission_rules_class_id_fkey FOREIGN KEY (class_id) REFERENCES classes(id);

CREATE TABLE IF NOT EXISTS dictionary_types (
    id uuid PRIMARY KEY,
    code varchar(48) NOT NULL UNIQUE,
    name varchar(96) NOT NULL UNIQUE,
    description varchar(255) NOT NULL DEFAULT '',
    system boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dictionary_items (
    id uuid PRIMARY KEY,
    type_id uuid NOT NULL REFERENCES dictionary_types(id) ON DELETE CASCADE,
    value varchar(64) NOT NULL,
    label varchar(96) NOT NULL,
    description varchar(255) NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(type_id, value)
);
CREATE INDEX IF NOT EXISTS idx_dictionary_items_type_order ON dictionary_items(type_id, sort_order, value);

INSERT INTO dictionary_types(id, code, name, description, system, enabled) VALUES
    ('30000000-0000-0000-0000-000000000001', 'GENDER', '性别', '教职工和学生基础数据使用的性别选项', true, true),
    ('30000000-0000-0000-0000-000000000002', 'ENROLLMENT_STATUS', '学籍状态', '学生基础数据使用的学籍状态选项', true, true)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    system = true,
    enabled = true,
    updated_at = now();

INSERT INTO dictionary_items(id, type_id, value, label, sort_order, enabled) VALUES
    ('31000000-0000-0000-0000-000000000001', (SELECT id FROM dictionary_types WHERE code = 'GENDER'), 'MALE', '男', 10, true),
    ('31000000-0000-0000-0000-000000000002', (SELECT id FROM dictionary_types WHERE code = 'GENDER'), 'FEMALE', '女', 20, true),
    ('31000000-0000-0000-0000-000000000003', (SELECT id FROM dictionary_types WHERE code = 'GENDER'), 'UNKNOWN', '未设置', 30, true),
    ('31000000-0000-0000-0000-000000000004', (SELECT id FROM dictionary_types WHERE code = 'ENROLLMENT_STATUS'), 'ENROLLED', '在籍', 10, true),
    ('31000000-0000-0000-0000-000000000005', (SELECT id FROM dictionary_types WHERE code = 'ENROLLMENT_STATUS'), 'SUSPENDED', '休学', 20, true),
    ('31000000-0000-0000-0000-000000000006', (SELECT id FROM dictionary_types WHERE code = 'ENROLLMENT_STATUS'), 'GRADUATED', '毕业', 30, true),
    ('31000000-0000-0000-0000-000000000007', (SELECT id FROM dictionary_types WHERE code = 'ENROLLMENT_STATUS'), 'LEFT', '离校', 40, true)
ON CONFLICT (type_id, value) DO UPDATE SET
    label = EXCLUDED.label,
    sort_order = EXCLUDED.sort_order,
    enabled = true,
    updated_at = now();

CREATE TABLE IF NOT EXISTS master_data_changes (
    id uuid PRIMARY KEY,
    entity_type varchar(32) NOT NULL,
    entity_id uuid NOT NULL,
    action varchar(24) NOT NULL,
    before_json jsonb,
    after_json jsonb,
    actor_id uuid REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_master_data_changes_entity ON master_data_changes(entity_type, entity_id, created_at DESC);

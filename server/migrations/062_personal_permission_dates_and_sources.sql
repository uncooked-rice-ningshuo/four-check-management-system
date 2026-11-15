ALTER TABLE user_permission_rules DROP CONSTRAINT user_permission_rules_time_type_scope_only;
ALTER TABLE user_permission_rules
 DROP COLUMN academic_year_id, DROP COLUMN semester_id, DROP COLUMN week_id,
 DROP COLUMN department_id, DROP COLUMN class_id, DROP COLUMN grade,
 ADD COLUMN source_department_id uuid REFERENCES departments(id),
 ADD COLUMN mode varchar(16) NOT NULL DEFAULT 'ADD' CHECK(mode IN ('ADD','OVERRIDE','DISABLE')),
 ADD COLUMN operation_ranges jsonb NOT NULL DEFAULT '[]',
 ADD COLUMN starts_at timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN ends_at timestamptz,
 ADD CHECK(ends_at IS NULL OR ends_at>starts_at);
CREATE UNIQUE INDEX permission_source_override ON user_permission_rules(account_id,source_department_id,action,COALESCE(inspection_type_id,'00000000-0000-0000-0000-000000000000'::uuid)) WHERE mode IN ('OVERRIDE','DISABLE') AND enabled;

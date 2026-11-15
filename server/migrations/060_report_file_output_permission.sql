ALTER TABLE department_report_scopes ADD COLUMN can_file_output boolean NOT NULL DEFAULT false;
ALTER TABLE department_report_role_defaults ADD COLUMN can_file_output boolean NOT NULL DEFAULT false;

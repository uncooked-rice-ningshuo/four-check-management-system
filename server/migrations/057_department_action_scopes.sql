-- Complete the one-to-one action catalog for the existing two MVP modules.
ALTER TABLE department_inspection_scopes ADD COLUMN can_manage boolean NOT NULL DEFAULT false;
UPDATE department_inspection_scopes SET can_manage=can_enter;

CREATE TABLE department_report_scopes (
    department_id uuid PRIMARY KEY REFERENCES departments(id),
    enabled boolean NOT NULL DEFAULT false,
    can_view boolean NOT NULL DEFAULT false,
    can_generate boolean NOT NULL DEFAULT false,
    can_correct boolean NOT NULL DEFAULT false,
    can_review boolean NOT NULL DEFAULT false,
    can_return boolean NOT NULL DEFAULT false,
    can_finalize boolean NOT NULL DEFAULT false,
    can_publish boolean NOT NULL DEFAULT false,
    can_output_settings boolean NOT NULL DEFAULT false,
    can_archive boolean NOT NULL DEFAULT false
);

-- Preserve the current office baseline; other departments start with no
-- weekly-report default capability and may be configured by an administrator.
INSERT INTO department_report_scopes
    (department_id,enabled,can_view,can_generate,can_correct,can_review,
     can_return,can_finalize,can_publish,can_output_settings,can_archive)
SELECT id,true,true,true,true,true,true,true,true,true,true
FROM departments WHERE code='OFFICE';

-- Each inspection item has its own role defaults. Preserve the previous
-- department-wide values for every existing item when upgrading a test database.
CREATE TABLE department_inspection_role_defaults_by_type (
    department_id uuid NOT NULL REFERENCES departments(id),
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    role_code varchar(32) NOT NULL CHECK (role_code IN ('MEMBER','LEADERSHIP','PRESIDIUM')),
    can_view boolean NOT NULL DEFAULT false,
    can_enter boolean NOT NULL DEFAULT false,
    can_manage boolean NOT NULL DEFAULT false,
    can_review boolean NOT NULL DEFAULT false,
    can_export boolean NOT NULL DEFAULT false,
    PRIMARY KEY (department_id, inspection_type_id, role_code),
    CONSTRAINT inspection_role_boundary_by_type CHECK (
        role_code='LEADERSHIP' OR (NOT can_manage AND NOT can_review AND NOT can_export)
    ),
    CONSTRAINT presidium_read_only_by_type CHECK (role_code<>'PRESIDIUM' OR NOT can_enter)
);

INSERT INTO department_inspection_role_defaults_by_type
    (department_id,inspection_type_id,role_code,can_view,can_enter,can_manage,can_review,can_export)
SELECT old.department_id,types.id,old.role_code,old.can_view,old.can_enter,old.can_manage,old.can_review,old.can_export
FROM department_inspection_role_defaults old CROSS JOIN inspection_types types;

DROP TABLE department_inspection_role_defaults;
ALTER TABLE department_inspection_role_defaults_by_type RENAME TO department_inspection_role_defaults;

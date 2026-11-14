-- Action eligibility comes from the saved department × inspection item × role
-- template. Batch ownership, record state and version guards remain enforced.
ALTER TABLE department_inspection_role_defaults
    DROP CONSTRAINT inspection_role_boundary_by_type,
    DROP CONSTRAINT presidium_read_only_by_type;

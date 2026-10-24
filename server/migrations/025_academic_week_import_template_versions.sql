ALTER TABLE academic_week_import_batches
    DROP CONSTRAINT IF EXISTS academic_week_import_batches_template_version_check;

ALTER TABLE academic_week_import_batches
    ADD CONSTRAINT academic_week_import_batches_template_version_positive
    CHECK (template_version > 0);

-- Import batches must accept the currently published template version.
-- Template revisions do not change the batch schema, so constrain to a positive version.
ALTER TABLE master_data_import_batches
    DROP CONSTRAINT IF EXISTS master_data_import_batches_template_version_check;

ALTER TABLE master_data_import_batches
    ADD CONSTRAINT master_data_import_batches_template_version_positive
    CHECK (template_version > 0);

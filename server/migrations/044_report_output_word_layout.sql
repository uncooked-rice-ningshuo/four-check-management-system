-- Store the restricted cover document independently from legacy plain text.
ALTER TABLE report_output_settings
    ADD COLUMN introduction_document_json jsonb;

-- Every generated file keeps the exact settings and layout revision used.
ALTER TABLE report_output_files
    ADD COLUMN settings_version integer,
    ADD COLUMN layout_version varchar(32),
    ADD COLUMN introduction_document_snapshot_json jsonb;

ALTER TABLE report_output_files
    ADD CONSTRAINT report_output_settings_version_positive CHECK (settings_version IS NULL OR settings_version > 0);

-- Word and Excel each own their ordered defaults.
ALTER TABLE report_output_settings
  ADD COLUMN word_type_codes text[] NOT NULL DEFAULT ARRAY[]::text[],
  ADD COLUMN excel_type_codes text[] NOT NULL DEFAULT ARRAY[]::text[];

-- Preserve the existing ordered defaults for both formats before retiring the
-- shared setting. The two arrays are independent after this migration.
UPDATE report_output_settings
SET word_type_codes = default_type_codes,
    excel_type_codes = default_type_codes;

ALTER TABLE report_output_settings DROP COLUMN default_type_codes;

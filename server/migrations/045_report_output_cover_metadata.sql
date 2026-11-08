-- The issuing unit belongs to the layout setting; the issue date belongs to each Word file.
ALTER TABLE report_output_settings
    ADD COLUMN signature_text varchar(100) NOT NULL DEFAULT '计算机科学与工程学院学生会办公室';

ALTER TABLE report_output_files
    ADD COLUMN signature_snapshot varchar(100),
    ADD COLUMN issue_date date;

ALTER TABLE report_output_files
    ADD CONSTRAINT report_output_issue_date_word_cover CHECK (
        issue_date IS NULL OR (format = 'WORD' AND combined = true)
    );

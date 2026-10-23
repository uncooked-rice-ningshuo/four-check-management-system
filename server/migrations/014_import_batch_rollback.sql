-- Preserve confirmation provenance and files when an untouched draft batch is undone.
ALTER TABLE import_batches DROP CONSTRAINT import_batches_status_check;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_status_check CHECK (status IN (
    'UPLOADED','PARSING','PREVIEW_READY','CONFIRMING','CONFIRMED',
    'PARSE_FAILED','CONFIRM_FAILED','CANCELLED','ROLLED_BACK'
));
ALTER TABLE import_batches ADD COLUMN rolled_back_at timestamptz;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_rollback_timestamp_check
    CHECK ((status = 'ROLLED_BACK') = (rolled_back_at IS NOT NULL));

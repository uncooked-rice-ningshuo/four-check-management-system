-- Preserve failure metadata while excluding completed object removals from recovery.
ALTER TABLE files ADD COLUMN object_removed_at timestamptz;
ALTER TABLE files ADD CONSTRAINT files_object_removed_state_check
    CHECK (object_removed_at IS NULL OR status = 'FAILED');
CREATE INDEX idx_files_reconciliation_pending
    ON files(updated_at, id)
    WHERE status IN ('PENDING', 'FAILED') AND object_removed_at IS NULL;

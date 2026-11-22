-- Preserve the event and related cases visible at delivery time. Opening a
-- target still uses the business page's current authorization and data scope.
ALTER TABLE account_notifications
    ADD COLUMN discipline_event_status text CHECK (discipline_event_status IN ('PENDING','DEFERRED','APPROVED','VOID')),
    ADD COLUMN related_deferred_ids uuid[];

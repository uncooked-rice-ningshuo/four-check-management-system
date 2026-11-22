ALTER TABLE account_notifications
    DROP CONSTRAINT account_notifications_category_check,
    ADD COLUMN discipline_case_id uuid REFERENCES discipline_cases(id),
    ADD COLUMN discipline_case_version integer,
    ADD CONSTRAINT account_notifications_category_check
        CHECK (category IN ('organization', 'permission', 'discipline')),
    ADD CONSTRAINT account_notifications_discipline_reference_check CHECK (
        (category = 'discipline' AND discipline_case_id IS NOT NULL
          AND discipline_case_version IS NOT NULL AND discipline_case_version > 0)
        OR (category <> 'discipline' AND discipline_case_id IS NULL AND discipline_case_version IS NULL)
    );

-- Each state transition advances the case version. A recipient sees it once.
CREATE UNIQUE INDEX account_notifications_discipline_event
    ON account_notifications(account_id, discipline_case_id, discipline_case_version)
    WHERE category = 'discipline';

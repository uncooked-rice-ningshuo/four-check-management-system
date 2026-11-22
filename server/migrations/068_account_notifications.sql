CREATE TABLE account_notifications (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    category text NOT NULL CHECK (category IN ('organization','permission')),
    permission_action text,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz
);
CREATE INDEX account_notifications_inbox ON account_notifications(account_id,id DESC);

-- A logical login survives refresh-token rotation. Existing live refresh rows
-- each represent one independently usable browser session.
CREATE TABLE login_sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_reason varchar(24)
);
ALTER TABLE refresh_sessions ADD COLUMN login_session_id uuid REFERENCES login_sessions(id);
INSERT INTO login_sessions(id, account_id, created_at, last_seen_at, expires_at, revoked_at)
SELECT id, account_id, created_at, created_at, expires_at, revoked_at FROM refresh_sessions;
UPDATE refresh_sessions SET login_session_id=id;
ALTER TABLE refresh_sessions ALTER COLUMN login_session_id SET NOT NULL;

-- On migration, retain the three most recently created live sessions.
WITH ranked AS (
    SELECT id, row_number() OVER (PARTITION BY account_id ORDER BY last_seen_at DESC, created_at DESC, id DESC) AS rank
    FROM login_sessions WHERE revoked_at IS NULL AND expires_at > now()
), evicted AS (
    UPDATE login_sessions s SET revoked_at=now(), revoked_reason='EVICTED'
    FROM ranked r WHERE s.id=r.id AND r.rank>3 RETURNING s.id, s.account_id
), revoked_tokens AS (
    UPDATE refresh_sessions r SET revoked_at=now() FROM evicted e
    WHERE r.login_session_id=e.id AND r.revoked_at IS NULL RETURNING e.id, e.account_id
)
INSERT INTO audit_logs(id, actor_id, action, resource_type, resource_id, after_json, reason)
SELECT gen_random_uuid(), account_id, 'SESSION_EVICTED', 'login_session', id::text,
       '{"reason":"migration_limit"}'::jsonb, '并发设备上限迁移' FROM revoked_tokens;

CREATE INDEX idx_login_sessions_account_active ON login_sessions(account_id, last_seen_at, created_at, id)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_login_sessions_expiry ON login_sessions(expires_at);
CREATE INDEX idx_refresh_sessions_login_session ON refresh_sessions(login_session_id)
    WHERE revoked_at IS NULL;

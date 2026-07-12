CREATE TABLE IF NOT EXISTS accounts (
    id uuid PRIMARY KEY,
    student_no varchar(32) NOT NULL UNIQUE,
    name varchar(64) NOT NULL,
    password_hash varchar(255) NOT NULL,
    status varchar(24) NOT NULL CHECK (status IN ('ACTIVE','PENDING','DISABLED','REJECTED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_account ON refresh_sessions(account_id, expires_at DESC);

CREATE TABLE IF NOT EXISTS access_policies (
    token_hash TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    wings TEXT NOT NULL, -- Comma-separated list of wings
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used DATETIME
);

CREATE INDEX IF NOT EXISTS idx_policy_name ON access_policies(name);

CREATE TABLE IF NOT EXISTS webhook_dlq (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    attempts INTEGER DEFAULT 0,
    failed_at REAL NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX IF NOT EXISTS idx_dlq_attempts ON webhook_dlq(attempts);
CREATE INDEX IF NOT EXISTS idx_dlq_failed_at ON webhook_dlq(failed_at);

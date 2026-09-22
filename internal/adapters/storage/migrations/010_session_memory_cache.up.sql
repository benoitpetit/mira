CREATE TABLE IF NOT EXISTS session_memory_cache (
    session_id TEXT NOT NULL,
    memory_id BLOB NOT NULL,
    expires_at REAL NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);

CREATE INDEX IF NOT EXISTS idx_session_memory_cache_expiry ON session_memory_cache(expires_at);

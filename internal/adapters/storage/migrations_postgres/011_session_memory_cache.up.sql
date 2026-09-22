CREATE TABLE IF NOT EXISTS session_memory_cache (
    session_id TEXT NOT NULL,
    memory_id UUID NOT NULL,
    expires_at DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);

CREATE INDEX IF NOT EXISTS idx_session_memory_cache_expiry ON session_memory_cache(expires_at);

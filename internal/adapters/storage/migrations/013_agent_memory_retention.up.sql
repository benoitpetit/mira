CREATE TABLE IF NOT EXISTS agent_memory_retention_policy (
 agent_id TEXT PRIMARY KEY,
 max_versions INTEGER NOT NULL DEFAULT 100,
 updated_at TEXT NOT NULL
);

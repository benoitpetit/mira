CREATE TABLE IF NOT EXISTS agent_memory_trait_evidence (
 snapshot_id TEXT NOT NULL,
 trait_name TEXT NOT NULL,
 role TEXT NOT NULL,
 excerpt TEXT NOT NULL,
 session_id TEXT,
 observed_at TEXT NOT NULL,
 confidence REAL NOT NULL DEFAULT 0,
 PRIMARY KEY(snapshot_id, trait_name, excerpt)
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_evidence_snapshot ON agent_memory_trait_evidence(snapshot_id);

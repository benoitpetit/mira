CREATE TABLE IF NOT EXISTS beliefs (
 id TEXT PRIMARY KEY, subject TEXT NOT NULL, predicate TEXT NOT NULL, value TEXT NOT NULL,
 valid_from TEXT, valid_until TEXT, confidence REAL NOT NULL DEFAULT 0,
 sources TEXT NOT NULL DEFAULT '[]', status TEXT NOT NULL DEFAULT 'active', updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS belief_feedback (
 belief_id TEXT NOT NULL, feedback TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY (belief_id, feedback)
);

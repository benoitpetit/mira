CREATE TABLE IF NOT EXISTS beliefs (
 id UUID PRIMARY KEY, subject TEXT NOT NULL, predicate TEXT NOT NULL, value TEXT NOT NULL,
 valid_from TIMESTAMPTZ, valid_until TIMESTAMPTZ, confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
 sources JSONB NOT NULL DEFAULT '[]', status TEXT NOT NULL DEFAULT 'active', updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS belief_feedback (
 belief_id UUID NOT NULL, feedback TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY (belief_id, feedback)
);

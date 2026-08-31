CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS embedding_models (
    model_hash TEXT PRIMARY KEY,
    model_name TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    created_at DOUBLE PRECISION NOT NULL,
    metadata JSONB
);

CREATE TABLE IF NOT EXISTS verbatim (
    id UUID PRIMARY KEY,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at DOUBLE PRECISION NOT NULL,
    wing TEXT NOT NULL,
    room TEXT,
    metadata JSONB,
    metrics JSONB
);

CREATE INDEX IF NOT EXISTS idx_verbatim_wing_room ON verbatim(wing, room);
CREATE INDEX IF NOT EXISTS idx_verbatim_created ON verbatim(created_at);
CREATE INDEX IF NOT EXISTS idx_verbatim_wing_time ON verbatim(wing, created_at);

CREATE TABLE IF NOT EXISTS fingerprints (
    id UUID PRIMARY KEY,
    verbatim_id UUID NOT NULL REFERENCES verbatim(id) ON DELETE CASCADE,
    ftype TEXT NOT NULL,
    extracted_at DOUBLE PRECISION NOT NULL,
    entities JSONB,
    subjects JSONB,
    decision TEXT,
    data JSONB NOT NULL,
    fact_count INTEGER DEFAULT 0,
    token_estimate INTEGER DEFAULT 0,
    model_hash TEXT REFERENCES embedding_models(model_hash)
);

CREATE INDEX IF NOT EXISTS idx_fp_type ON fingerprints(ftype);
CREATE INDEX IF NOT EXISTS idx_fp_verbatim_id ON fingerprints(verbatim_id);

CREATE TABLE IF NOT EXISTS embeddings (
    id UUID PRIMARY KEY REFERENCES verbatim(id) ON DELETE CASCADE,
    model_hash TEXT NOT NULL REFERENCES embedding_models(model_hash),
    dim INTEGER NOT NULL,
    vector VECTOR, -- pgvector type
    normalized INTEGER DEFAULT 1,
    created_at DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS causal_nodes (
    id UUID PRIMARY KEY REFERENCES fingerprints(id) ON DELETE CASCADE,
    node_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    timestamp DOUBLE PRECISION NOT NULL,
    wing TEXT NOT NULL,
    room TEXT
);

CREATE TABLE IF NOT EXISTS causal_edges (
    from_id UUID NOT NULL REFERENCES causal_nodes(id) ON DELETE CASCADE,
    to_id UUID NOT NULL REFERENCES causal_nodes(id) ON DELETE CASCADE,
    relation TEXT NOT NULL,
    weight DOUBLE PRECISION DEFAULT 1.0,
    detected_at DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (from_id, to_id, relation)
);

CREATE INDEX IF NOT EXISTS idx_edges_from ON causal_edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON causal_edges(to_id);
CREATE INDEX IF NOT EXISTS idx_edges_timestamp ON causal_edges(detected_at);

CREATE TABLE IF NOT EXISTS overlap_cache (
    id_a UUID NOT NULL,
    id_b UUID NOT NULL,
    similarity DOUBLE PRECISION NOT NULL,
    computed_at DOUBLE PRECISION NOT NULL,
    ttl DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (id_a, id_b)
);

CREATE INDEX IF NOT EXISTS idx_overlap_ttl ON overlap_cache(ttl);

CREATE TABLE IF NOT EXISTS webhook_dlq (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    attempts INTEGER DEFAULT 0,
    failed_at DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_dlq_attempts ON webhook_dlq(attempts);
CREATE INDEX IF NOT EXISTS idx_dlq_failed_at ON webhook_dlq(failed_at);

CREATE TABLE IF NOT EXISTS memory_tags (
    id SERIAL PRIMARY KEY,
    verbatim_id UUID NOT NULL REFERENCES verbatim(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    tag_type TEXT NOT NULL, -- 'entity' | 'subject' | 'keyword'
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(verbatim_id, tag, tag_type)
);
CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);
CREATE INDEX IF NOT EXISTS idx_memory_tags_verbatim ON memory_tags(verbatim_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    resource TEXT NOT NULL,
    status INTEGER NOT NULL,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor);

CREATE TABLE IF NOT EXISTS access_policies (
    token_hash TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    wings TEXT NOT NULL, -- Comma-separated list of wings
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_policy_name ON access_policies(name);

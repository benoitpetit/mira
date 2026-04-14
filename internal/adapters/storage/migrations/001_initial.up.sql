CREATE TABLE IF NOT EXISTS embedding_models (
    model_hash TEXT PRIMARY KEY,
    model_name TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    created_at REAL NOT NULL,
    metadata TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS verbatim (
    id BLOB PRIMARY KEY,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT,
    metadata TEXT
) STRICT;

CREATE INDEX IF NOT EXISTS idx_verbatim_wing_room ON verbatim(wing, room);
CREATE INDEX IF NOT EXISTS idx_verbatim_created ON verbatim(created_at);
CREATE INDEX IF NOT EXISTS idx_verbatim_wing_time ON verbatim(wing, created_at);

CREATE TABLE IF NOT EXISTS fingerprints (
    id BLOB PRIMARY KEY,
    verbatim_id BLOB NOT NULL REFERENCES verbatim(id),
    ftype TEXT NOT NULL,
    extracted_at REAL NOT NULL,
    entities TEXT,
    subjects TEXT,
    decision TEXT,
    data TEXT NOT NULL,
    fact_count INTEGER DEFAULT 0,
    token_estimate INTEGER DEFAULT 0,
    model_hash TEXT REFERENCES embedding_models(model_hash)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_fp_type ON fingerprints(ftype);
CREATE INDEX IF NOT EXISTS idx_fp_entities ON fingerprints(entities);
CREATE INDEX IF NOT EXISTS idx_fp_subjects ON fingerprints(subjects);
CREATE INDEX IF NOT EXISTS idx_fp_decision ON fingerprints(decision);

CREATE TABLE IF NOT EXISTS embeddings (
    id BLOB PRIMARY KEY REFERENCES verbatim(id),
    model_hash TEXT NOT NULL REFERENCES embedding_models(model_hash),
    dim INTEGER NOT NULL,
    vector BLOB NOT NULL,
    normalized INTEGER DEFAULT 1,
    created_at REAL NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS causal_nodes (
    id BLOB PRIMARY KEY REFERENCES fingerprints(id),
    node_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    timestamp REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS causal_edges (
    from_id BLOB NOT NULL REFERENCES causal_nodes(id),
    to_id BLOB NOT NULL REFERENCES causal_nodes(id),
    relation TEXT NOT NULL,
    weight REAL DEFAULT 1.0,
    detected_at REAL NOT NULL,
    PRIMARY KEY (from_id, to_id, relation)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_edges_from ON causal_edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON causal_edges(to_id);
CREATE INDEX IF NOT EXISTS idx_edges_timestamp ON causal_edges(detected_at);

CREATE TABLE IF NOT EXISTS overlap_cache (
    id_a BLOB NOT NULL,
    id_b BLOB NOT NULL,
    similarity REAL NOT NULL,
    computed_at REAL NOT NULL,
    ttl REAL NOT NULL DEFAULT (unixepoch() + 2592000),
    PRIMARY KEY (id_a, id_b)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_overlap_ttl ON overlap_cache(ttl);

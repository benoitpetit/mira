-- Keep PostgreSQL lexical fallback functional when HNSW is unavailable.
CREATE INDEX IF NOT EXISTS idx_verbatim_content_fts
    ON verbatim USING GIN (to_tsvector('simple', content));

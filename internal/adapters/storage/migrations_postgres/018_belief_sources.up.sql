CREATE TABLE IF NOT EXISTS belief_sources (
    belief_id UUID NOT NULL,
    source_id UUID NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (belief_id, source_id)
);
CREATE INDEX IF NOT EXISTS idx_belief_sources_source ON belief_sources(source_id);
CREATE INDEX IF NOT EXISTS idx_belief_sources_active ON belief_sources(belief_id, active);
CREATE TABLE IF NOT EXISTS belief_source_migration_diagnostics (
    migration_version INTEGER PRIMARY KEY,
    discarded_source_references BIGINT NOT NULL
);

INSERT INTO belief_sources (belief_id, source_id, active)
SELECT b.id, source_id.parsed_id,
       COALESCE(v.lifecycle_state, 'active')='active'
FROM beliefs b
CROSS JOIN LATERAL (
    SELECT value,
           CASE WHEN value ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN value::uuid ELSE NULL END AS parsed_id
    FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(b.sources)='array' THEN b.sources ELSE '[]'::jsonb END) AS source(value)
) AS source_id
JOIN verbatim v ON v.id=source_id.parsed_id
ON CONFLICT (belief_id, source_id) DO NOTHING;

INSERT INTO belief_source_migration_diagnostics(migration_version, discarded_source_references)
SELECT 18, COALESCE(SUM(discarded), 0) FROM (
    SELECT CASE WHEN jsonb_typeof(b.sources)='array' THEN
        (SELECT COUNT(*) FROM jsonb_array_elements_text(b.sources) s(value)
         WHERE NOT EXISTS (SELECT 1 FROM verbatim v WHERE lower(v.id::text)=lower(s.value)))
        ELSE 1 END AS discarded
    FROM beliefs b
) refs
ON CONFLICT (migration_version) DO UPDATE SET discarded_source_references=EXCLUDED.discarded_source_references;

UPDATE beliefs
SET sources=COALESCE((SELECT jsonb_agg(to_jsonb(source_id::text)) FROM belief_sources WHERE belief_id=beliefs.id AND active=TRUE), '[]'::jsonb),
    status=CASE WHEN EXISTS (SELECT 1 FROM belief_sources WHERE belief_id=beliefs.id AND active=TRUE) THEN 'active' ELSE 'revoked' END;

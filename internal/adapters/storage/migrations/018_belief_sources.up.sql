CREATE TABLE IF NOT EXISTS belief_sources (
    belief_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (belief_id, source_id)
);
CREATE INDEX IF NOT EXISTS idx_belief_sources_source ON belief_sources(source_id);
CREATE INDEX IF NOT EXISTS idx_belief_sources_active ON belief_sources(belief_id, active);
CREATE TABLE IF NOT EXISTS belief_source_migration_diagnostics (
    migration_version INTEGER PRIMARY KEY,
    discarded_source_references INTEGER NOT NULL
);

INSERT OR IGNORE INTO belief_sources (belief_id, source_id, active)
SELECT b.id, json_each.value,
       CASE WHEN v.id IS NOT NULL AND COALESCE(v.lifecycle_state, 'active')='active' THEN 1 ELSE 0 END
FROM beliefs b
JOIN json_each(CASE WHEN json_valid(b.sources) THEN CASE WHEN json_type(b.sources)='array' THEN b.sources ELSE '[]' END ELSE '[]' END)
JOIN verbatim v ON lower(hex(v.id)) = replace(lower(json_each.value), '-', '');

INSERT OR REPLACE INTO belief_source_migration_diagnostics(migration_version, discarded_source_references)
SELECT 18, COALESCE(SUM(discarded), 0) FROM (
    SELECT CASE WHEN json_valid(b.sources) THEN CASE WHEN json_type(b.sources)='array' THEN
        (SELECT COUNT(*) FROM json_each(b.sources) s
         WHERE NOT EXISTS (SELECT 1 FROM verbatim v WHERE lower(hex(v.id))=replace(lower(s.value), '-', '')))
        ELSE 1 END ELSE 1 END AS discarded
    FROM beliefs b
);

UPDATE beliefs
SET sources=COALESCE((SELECT json_group_array(source_id) FROM belief_sources WHERE belief_id=beliefs.id AND active=1), '[]'),
    status=CASE WHEN EXISTS (SELECT 1 FROM belief_sources WHERE belief_id=beliefs.id AND active=1) THEN 'active' ELSE 'revoked' END;

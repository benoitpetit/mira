ALTER TABLE verbatim ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE verbatim ADD COLUMN superseded_by TEXT;

UPDATE verbatim
SET lifecycle_state = CASE
    WHEN json_extract(metadata, '$.lifecycle_state') IN ('active', 'superseded', 'archived', 'contested')
        THEN json_extract(metadata, '$.lifecycle_state')
    ELSE 'active'
END,
superseded_by = CASE
    WHEN json_extract(metadata, '$.superseded_by') IS NOT NULL
        THEN json_extract(metadata, '$.superseded_by')
    ELSE NULL
END;

CREATE INDEX IF NOT EXISTS idx_verbatim_lifecycle_state ON verbatim(lifecycle_state, wing, created_at);
CREATE INDEX IF NOT EXISTS idx_verbatim_superseded_by ON verbatim(superseded_by);

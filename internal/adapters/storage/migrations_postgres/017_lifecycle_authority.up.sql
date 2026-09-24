ALTER TABLE verbatim ADD COLUMN IF NOT EXISTS lifecycle_state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE verbatim ADD COLUMN IF NOT EXISTS superseded_by UUID;

UPDATE verbatim
SET lifecycle_state = CASE
    WHEN metadata->>'lifecycle_state' IN ('active', 'superseded', 'archived', 'contested')
        THEN metadata->>'lifecycle_state'
    ELSE 'active'
END,
superseded_by = CASE
    WHEN (metadata->>'superseded_by') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
        THEN (metadata->>'superseded_by')::uuid
    ELSE NULL
END;

CREATE INDEX IF NOT EXISTS idx_verbatim_lifecycle_state ON verbatim(lifecycle_state, wing, created_at);
CREATE INDEX IF NOT EXISTS idx_verbatim_superseded_by ON verbatim(superseded_by);

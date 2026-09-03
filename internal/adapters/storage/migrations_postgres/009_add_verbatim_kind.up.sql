ALTER TABLE verbatim ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'knowledge';

CREATE INDEX IF NOT EXISTS idx_verbatim_kind ON verbatim(kind);

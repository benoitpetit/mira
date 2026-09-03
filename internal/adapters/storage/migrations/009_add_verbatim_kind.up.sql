ALTER TABLE verbatim ADD COLUMN kind TEXT NOT NULL DEFAULT 'knowledge';

CREATE INDEX IF NOT EXISTS idx_verbatim_kind ON verbatim(kind);

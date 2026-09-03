ALTER TABLE verbatim ADD COLUMN IF NOT EXISTS valid_from DOUBLE PRECISION;
ALTER TABLE verbatim ADD COLUMN IF NOT EXISTS valid_until DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_verbatim_validity ON verbatim(valid_from, valid_until);

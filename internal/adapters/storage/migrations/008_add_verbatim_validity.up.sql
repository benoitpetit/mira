ALTER TABLE verbatim ADD COLUMN valid_from REAL;
ALTER TABLE verbatim ADD COLUMN valid_until REAL;

CREATE INDEX IF NOT EXISTS idx_verbatim_validity ON verbatim(valid_from, valid_until);

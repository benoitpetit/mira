CREATE INDEX IF NOT EXISTS idx_fp_verbatim_id ON fingerprints(verbatim_id);
CREATE INDEX IF NOT EXISTS idx_fp_extracted_at_id ON fingerprints(extracted_at DESC, id DESC);

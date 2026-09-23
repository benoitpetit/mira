ALTER TABLE causal_edges ADD COLUMN confidence REAL NOT NULL DEFAULT 0.7;
ALTER TABLE causal_edges ADD COLUMN status TEXT NOT NULL DEFAULT 'confirmed';
ALTER TABLE causal_edges ADD COLUMN evidence TEXT NOT NULL DEFAULT '';
ALTER TABLE causal_edges ADD COLUMN detector TEXT NOT NULL DEFAULT 'legacy';

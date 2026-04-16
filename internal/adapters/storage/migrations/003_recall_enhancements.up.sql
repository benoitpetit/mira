-- Table de tags extraits automatiquement
CREATE TABLE IF NOT EXISTS memory_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    verbatim_id BLOB NOT NULL,
    tag TEXT NOT NULL,
    tag_type TEXT NOT NULL, -- 'entity' | 'subject' | 'keyword'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (verbatim_id) REFERENCES verbatim(id) ON DELETE CASCADE,
    UNIQUE(verbatim_id, tag, tag_type)
);
CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);
CREATE INDEX IF NOT EXISTS idx_memory_tags_verbatim ON memory_tags(verbatim_id);

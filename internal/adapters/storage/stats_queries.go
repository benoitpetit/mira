package storage

const activeEmbeddingCountQuery = `
	SELECT COUNT(*)
	FROM verbatim v
	JOIN embeddings e ON v.id = e.id
	WHERE COALESCE(v.lifecycle_state, 'active') = 'active'
`

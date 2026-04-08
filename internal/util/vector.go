package util

// CosineSimilarity computes cosine similarity between two vectors
// Assumes vectors are pre-normalized (returns dot product)
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return dot
}

// CosineDistance computes cosine distance (1 - similarity)
func CosineDistance(a, b []float32) float64 {
	return 1 - CosineSimilarity(a, b)
}

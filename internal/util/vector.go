package util

import "math"

// CosineSimilarity computes cosine similarity between two vectors
// Returns a value in range [-1, 1] where 1 means identical direction
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CosineSimilarityNormalized computes cosine similarity for pre-normalized vectors
// This is faster as it only computes the dot product
// Assumes vectors are already L2-normalized (magnitude = 1)
func CosineSimilarityNormalized(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return dot
}

// CosineDistance computes cosine distance (1 - cosine similarity)
// Returns a value in range [0, 2] where 0 means identical direction
func CosineDistance(a, b []float32) float32 {
	return float32(1.0 - CosineSimilarity(a, b))
}

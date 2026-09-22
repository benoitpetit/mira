package vector

// createTestVector creates a deterministic test vector for every platform.
func createTestVector(dim int, value float32) []float32 {
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = value
	}
	return vec
}

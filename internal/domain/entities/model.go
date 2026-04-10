// EmbeddingModel entity
package entities

import "time"

// EmbeddingModel represents metadata about an embedding model
type EmbeddingModel struct {
	ModelHash string
	ModelName string
	Dimension int
	CreatedAt time.Time
	Metadata  map[string]any
}

// NewEmbeddingModel creates a new model metadata
func NewEmbeddingModel(modelName string, dimension int) *EmbeddingModel {
	return &EmbeddingModel{
		ModelHash: computeModelHash(modelName),
		ModelName: modelName,
		Dimension: dimension,
		CreatedAt: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// WithMetadata adds metadata to the model
func (m *EmbeddingModel) WithMetadata(key string, value any) *EmbeddingModel {
	m.Metadata[key] = value
	return m
}

func computeModelHash(modelName string) string {
	// Simple hash - first 16 chars of model name
	if len(modelName) > 16 {
		return modelName[:16]
	}
	return modelName
}

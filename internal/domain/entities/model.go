// EmbeddingModel entity
package entities

import (
	"time"

	"github.com/benoitpetit/mira/internal/util"
)

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
		ModelHash: util.ComputeModelHash(modelName),
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

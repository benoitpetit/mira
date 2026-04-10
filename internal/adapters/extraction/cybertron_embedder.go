// Cybertron embedder adapter using real NLP models
package extraction

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
	"github.com/rs/zerolog"
)

// CybertronEmbedder uses Cybertron for real embeddings
type CybertronEmbedder struct {
	model     textencoding.Interface
	modelsDir string
	modelName string
	dimension int
	mu        sync.RWMutex
}

// CybertronEmbedderOptions configures the embedder
type CybertronEmbedderOptions struct {
	ModelName string
	ModelsDir string
	Dimension int
}

// NewCybertronEmbedder creates a new Cybertron embedder
func NewCybertronEmbedder(opts CybertronEmbedderOptions) (*CybertronEmbedder, error) {
	// Ensure models directory exists
	if err := os.MkdirAll(opts.ModelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	// Suppress Cybertron's JSON logs by disabling zerolog output
	// Cybertron uses zerolog for logging
	zerolog.SetGlobalLevel(zerolog.Disabled)

	// Load the model
	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: opts.ModelsDir,
		ModelName: opts.ModelName,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load cybertron model %s: %w", opts.ModelName, err)
	}

	return &CybertronEmbedder{
		model:     m,
		modelsDir: opts.ModelsDir,
		modelName: opts.ModelName,
		dimension: opts.Dimension,
	}, nil
}

// Encode implements Embedder
func (c *CybertronEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return make([]float32, c.dimension), nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	result, err := c.model.Encode(ctx, text, int(bert.MeanPooling))
	if err != nil {
		return nil, fmt.Errorf("encoding failed: %w", err)
	}

	// Convert to float32
	vec64 := result.Vector.Data().F64()
	vec32 := make([]float32, len(vec64))
	for i, v := range vec64 {
		vec32[i] = float32(v)
	}

	// Ensure correct dimension
	if len(vec32) != c.dimension {
		if len(vec32) > c.dimension {
			vec32 = vec32[:c.dimension]
		} else {
			padded := make([]float32, c.dimension)
			copy(padded, vec32)
			vec32 = padded
		}
	}

	return vec32, nil
}

// Close releases resources
func (c *CybertronEmbedder) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.model != nil {
		tasks.Finalize(c.model)
		c.model = nil
	}
	return nil
}

// ModelName returns the model name
func (c *CybertronEmbedder) ModelName() string {
	return c.modelName
}

// Ensure CybertronEmbedder implements Embedder
var _ ports.Embedder = (*CybertronEmbedder)(nil)

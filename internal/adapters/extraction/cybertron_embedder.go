// Cybertron embedder adapter using real NLP models
package extraction

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

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

	// Check if model already exists locally
	modelPath := filepath.Join(opts.ModelsDir, opts.ModelName)
	modelExists := false
	if info, err := os.Stat(modelPath); err == nil && info.IsDir() {
		// Check if directory contains model files (config.json or spago_model.bin)
		if _, err := os.Stat(filepath.Join(modelPath, "config.json")); err == nil {
			modelExists = true
		}
	}

	// If model doesn't exist, show download message and start progress indicator
	var progressStop chan struct{}
	if !modelExists {
		log.Printf("[Embedder] Model not found locally, downloading: %s", opts.ModelName)
		log.Printf("[Embedder] This may take several minutes depending on your connection...")
		
		progressStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					log.Printf("[Embedder] Download in progress... please wait")
				case <-progressStop:
					return
				}
			}
		}()
	}

	// Load the model
	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: opts.ModelsDir,
		ModelName: opts.ModelName,
	})

	// Stop progress indicator if it was started
	if progressStop != nil {
		close(progressStop)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load cybertron model %s: %w", opts.ModelName, err)
	}

	if !modelExists {
		log.Printf("[Embedder] Model downloaded and loaded successfully")
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

	// Use full Lock instead of RLock because the underlying spago library
	// has race conditions when called concurrently
	c.mu.Lock()
	defer c.mu.Unlock()

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

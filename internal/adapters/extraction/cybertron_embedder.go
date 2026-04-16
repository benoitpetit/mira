// Package extraction provides embedding generation and NLP extraction adapters.
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

// CybertronEmbedder uses Cybertron for real embeddings with a model pool
// to allow concurrent encoding without global serialization.
type CybertronEmbedder struct {
	modelPool  chan textencoding.Interface
	allModels  []textencoding.Interface
	modelsDir  string
	modelName  string
	dimension  int
	closeOnce  sync.Once
	closed     bool
	mu         sync.Mutex
}

// CybertronEmbedderOptions configures the embedder
type CybertronEmbedderOptions struct {
	ModelName string
	ModelsDir string
	Dimension int
	PoolSize  int
}

// NewCybertronEmbedder creates a new Cybertron embedder with a model pool.
func NewCybertronEmbedder(opts CybertronEmbedderOptions) (*CybertronEmbedder, error) {
	if opts.PoolSize <= 0 {
		opts.PoolSize = 2
	}

	// Ensure models directory exists
	if err := os.MkdirAll(opts.ModelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	// Suppress Cybertron's JSON logs by disabling zerolog output
	zerolog.SetGlobalLevel(zerolog.Disabled)

	// Check if model already exists locally
	modelPath := filepath.Join(opts.ModelsDir, opts.ModelName)
	modelExists := false
	if info, err := os.Stat(modelPath); err == nil && info.IsDir() {
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

	// Load the first model instance (triggers download if needed)
	models := make([]textencoding.Interface, 0, opts.PoolSize)
	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: opts.ModelsDir,
		ModelName: opts.ModelName,
	})
	if progressStop != nil {
		close(progressStop)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load cybertron model %s: %w", opts.ModelName, err)
	}
	models = append(models, m)

	// Load additional model instances for the pool
	for i := 1; i < opts.PoolSize; i++ {
		mi, err := tasks.Load[textencoding.Interface](&tasks.Config{
			ModelsDir: opts.ModelsDir,
			ModelName: opts.ModelName,
		})
		if err != nil {
			// Finalize already loaded models on error
			for _, loaded := range models {
				tasks.Finalize(loaded)
			}
			return nil, fmt.Errorf("failed to load cybertron model instance %d: %w", i+1, err)
		}
		models = append(models, mi)
	}

	if !modelExists {
		log.Printf("[Embedder] Model downloaded and loaded successfully (%d instances)", opts.PoolSize)
	} else {
		log.Printf("[Embedder] Model loaded from cache (%d instances)", opts.PoolSize)
	}

	pool := make(chan textencoding.Interface, opts.PoolSize)
	for _, mi := range models {
		pool <- mi
	}

	return &CybertronEmbedder{
		modelPool: pool,
		allModels: models,
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

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("embedder is closed")
	}
	c.mu.Unlock()

	var model textencoding.Interface
	select {
	case model = <-c.modelPool:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	result, err := model.Encode(ctx, text, int(bert.MeanPooling))

	// Return model to pool
	select {
	case c.modelPool <- model:
	case <-ctx.Done():
		// Still try to return the model to avoid leaking it
		select {
		case c.modelPool <- model:
		default:
		}
		if err == nil {
			err = ctx.Err()
		}
	}

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

// Close releases all model instances
func (c *CybertronEmbedder) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()

		// Drain the pool to collect all models
		drained := make([]textencoding.Interface, 0, len(c.allModels))
		for i := 0; i < len(c.allModels); i++ {
			select {
			case m := <-c.modelPool:
				drained = append(drained, m)
			case <-time.After(2 * time.Second):
				log.Printf("[Embedder] Timeout draining model pool during close")
				break
			}
		}
		for _, m := range drained {
			tasks.Finalize(m)
		}
		close(c.modelPool)
	})
	return nil
}

// ModelName returns the model name
func (c *CybertronEmbedder) ModelName() string {
	return c.modelName
}

// Ensure CybertronEmbedder implements Embedder
var _ ports.Embedder = (*CybertronEmbedder)(nil)

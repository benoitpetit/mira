package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/usecases/ports"
)

func repairVectorStore(ctx context.Context, store ports.VectorStore) error {
	repairer, ok := store.(ports.VectorStoreRepairer)
	if !ok {
		return fmt.Errorf("vector store does not support rebuild")
	}
	if err := repairer.Rebuild(ctx); err != nil {
		return fmt.Errorf("rebuild vector index: %w", err)
	}
	return nil
}

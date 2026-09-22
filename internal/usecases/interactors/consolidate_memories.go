// Package interactors provides the application use cases.
package interactors

import (
	"context"
	"fmt"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// ConsolidateMemoriesInput contains the input for memory consolidation
type ConsolidateMemoriesInput struct {
	Wing                string
	SimilarityThreshold float64
}

// ConsolidateMemoriesOutput contains the output of memory consolidation
type ConsolidateMemoriesOutput struct {
	ConsolidatedCount int `json:"consolidated_count"`
	RemovedCount      int `json:"removed_count"`
}

// ConsolidateMemories merges redundant session notes into synthesized facts.
type ConsolidateMemories struct {
	repository  ports.Repository
	vectorStore ports.VectorStore
	embedder    ports.Embedder
	extractor   ports.Extractor //nolint:staticcheck // extractor methods are consumed together by consolidation
}

// NewConsolidateMemories creates a new consolidation interactor
func NewConsolidateMemories(
	repository ports.Repository,
	vectorStore ports.VectorStore,
	embedder ports.Embedder,
	extractor ports.Extractor, //nolint:staticcheck // extractor methods are consumed together by consolidation
) *ConsolidateMemories {
	return &ConsolidateMemories{
		repository:  repository,
		vectorStore: vectorStore,
		embedder:    embedder,
		extractor:   extractor,
	}
}

// Execute scans session notes in the given wing, clusters highly similar items,
// and creates a synthetic fact for each cluster.
func (uc *ConsolidateMemories) Execute(ctx context.Context, input ConsolidateMemoriesInput) (*ConsolidateMemoriesOutput, error) {
	threshold := input.SimilarityThreshold
	if threshold <= 0 || threshold > 1 {
		threshold = 0.92
	}

	// Fetch session notes for the wing
	memType := valueobjects.TypeSessionNote
	timelineItems, err := uc.repository.GetTimeline(ctx, input.Wing, nil, &memType, nil, nil, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch timeline: %w", err)
	}

	if len(timelineItems) < 2 {
		return &ConsolidateMemoriesOutput{}, nil
	}

	// Load full verbatims and embeddings
	type note struct {
		item      *valueobjects.TimelineItem
		verbatim  *entities.Verbatim
		embedding []float32
	}

	notes := make([]note, 0, len(timelineItems))
	for _, item := range timelineItems {
		uid, err := uuid.Parse(item.ID)
		if err != nil {
			continue
		}
		v, err := uc.repository.GetVerbatimByID(ctx, uid)
		if err != nil {
			continue
		}
		emb, err := uc.repository.GetEmbeddingByID(ctx, uid)
		if err != nil {
			continue
		}
		notes = append(notes, note{
			item:      item,
			verbatim:  v,
			embedding: emb.Vector,
		})
	}

	if len(notes) < 2 {
		return &ConsolidateMemoriesOutput{}, nil
	}

	// Greedy clustering by similarity
	visited := make(map[int]bool)
	var clusters [][]note

	for i := 0; i < len(notes); i++ {
		if visited[i] {
			continue
		}
		cluster := []note{notes[i]}
		visited[i] = true
		for j := i + 1; j < len(notes); j++ {
			if visited[j] {
				continue
			}
			// Compare with a stable representative to avoid single-link
			// transitive chains swallowing distant notes.
			similar := util.CosineSimilarity(cluster[0].embedding, notes[j].embedding) >= threshold
			if similar {
				cluster = append(cluster, notes[j])
				visited[j] = true
			}
		}
		if len(cluster) > 1 {
			clusters = append(clusters, cluster)
		}
	}

	output := &ConsolidateMemoriesOutput{}

	// For each cluster, create a synthetic fact
	for _, cluster := range clusters {
		var contents []string
		for _, n := range cluster {
			contents = append(contents, n.verbatim.Content)
		}

		// Abstractive synthesis using LLM (if available) or basic join (fallback)
		syntheticContent, err := uc.extractor.Summarize(ctx, contents)
		if err != nil || syntheticContent == "" {
			// Extreme fallback if summarization fails
			syntheticContent = strings.Join(contents, "; ")
			syntheticContent = truncateMemoryContent(syntheticContent, 500)
		}

		// Store as a fact
		factType := valueobjects.TypeFact
		storeInput := StoreMemoryInput{
			Content: syntheticContent,
			Wing:    input.Wing,
			Room:    func() *string { r := "consolidated"; return &r }(),
			Type:    &factType,
		}

		// We reuse the extraction pipeline directly to avoid circular dependency
		verbatim := entities.NewVerbatim(storeInput.Content, storeInput.Wing, storeInput.Room)
		if verbatim.Metadata == nil {
			verbatim.Metadata = make(map[string]any)
		}
		sourceIDs := make([]string, 0, len(cluster))
		for _, n := range cluster {
			sourceIDs = append(sourceIDs, n.verbatim.ID.String())
		}
		verbatim.Metadata["consolidated_from"] = sourceIDs
		verbatim.Metadata["consolidation_similarity_threshold"] = threshold
		fp, emb, err := uc.extractor.ExtractPipeline(ctx, verbatim, &factType)
		if err != nil {
			return nil, fmt.Errorf("failed to extract consolidated memory: %w", err)
		}

		tx, err := uc.repository.Begin()
		if err != nil {
			return nil, fmt.Errorf("failed to begin consolidation transaction: %w", err)
		}
		if err := uc.repository.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store consolidated verbatim: %w", err)
		}
		if err := uc.repository.StoreFingerprintTx(ctx, tx, fp); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store consolidated fingerprint: %w", err)
		}
		if err := uc.repository.StoreEmbeddingTx(ctx, tx, emb); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store consolidated embedding: %w", err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to commit consolidation transaction: %w", err)
		}
		reindexMemoryDerivedData(ctx, uc.repository, fp, verbatim, syntheticContent, uc.extractor, nil)

		candidate := entities.NewCandidate(fp, verbatim, emb.Vector)
		if err := uc.vectorStore.AddCandidate(ctx, candidate); err != nil {
			// The SQL repository is authoritative. Repair the derived index before
			// removing sources; if repair cannot complete, compensate by deleting the
			// synthesized memory so a retry cannot create a duplicate consolidation.
			if repairErr := repairVectorStore(ctx, uc.vectorStore); repairErr != nil {
				cleanupErr := uc.repository.DeleteVerbatimByID(ctx, verbatim.ID)
				if cleanupErr != nil {
					return nil, fmt.Errorf("failed to index consolidated memory: %w (rollback of synthesized memory also failed: %v)", err, cleanupErr)
				}
				return nil, fmt.Errorf("failed to index consolidated memory: %w (repair failed: %v)", err, repairErr)
			}
		}

		output.ConsolidatedCount++

		// Remove original notes by ID (not by room, to avoid deleting unrelated memories)
		idsToRemove := make([]uuid.UUID, 0, len(cluster))
		for _, n := range cluster {
			idsToRemove = append(idsToRemove, n.verbatim.ID)
		}
		deleted, err := uc.repository.ClearByIDs(ctx, idsToRemove)
		if err != nil {
			return nil, fmt.Errorf("failed to remove consolidated source memories: %w", err)
		}
		for _, id := range idsToRemove {
			if err := uc.vectorStore.Delete(ctx, id); err != nil {
				// Sources are already gone from the authoritative repository. Rebuild
				// the derived index so deleted notes cannot remain recallable.
				if repairErr := repairVectorStore(ctx, uc.vectorStore); repairErr != nil {
					return nil, fmt.Errorf("source memories removed but vector index repair failed: %w", repairErr)
				}
				break
			}
		}
		output.RemovedCount += deleted
	}

	return output, nil
}

func truncateMemoryContent(content string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "..."
}

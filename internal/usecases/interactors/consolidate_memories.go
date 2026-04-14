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
	ConsolidatedCount int
	RemovedCount      int
}

// ConsolidateMemories merges redundant session notes into synthesized facts.
type ConsolidateMemories struct {
	repository ports.Repository
	vectorStore ports.VectorStore
	embedder   ports.Embedder
	extractor  ports.FingerprintExtractor
}

// NewConsolidateMemories creates a new consolidation interactor
func NewConsolidateMemories(
	repository ports.Repository,
	vectorStore ports.VectorStore,
	embedder ports.Embedder,
	extractor ports.FingerprintExtractor,
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
			// Check similarity with any member of the cluster
			similar := false
			for _, member := range cluster {
				sim := util.CosineSimilarity(member.embedding, notes[j].embedding)
				if sim >= threshold {
					similar = true
					break
				}
			}
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
		var subjects []string
		for _, n := range cluster {
			contents = append(contents, n.verbatim.Content)
			if len(subjects) == 0 {
				subjects = append(subjects, n.item.Summary)
			}
		}

		syntheticContent := strings.Join(contents, "; ")
		if len(syntheticContent) > 500 {
			syntheticContent = syntheticContent[:500] + "..."
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
		fp, emb, err := uc.extractor.ExtractPipeline(ctx, verbatim, &factType)
		if err != nil {
			continue
		}

		tx, err := uc.repository.Begin()
		if err != nil {
			continue
		}
		if tx != nil {
			_ = uc.repository.StoreVerbatimTx(ctx, tx, verbatim)
			_ = uc.repository.StoreFingerprintTx(ctx, tx, fp)
			_ = uc.repository.StoreEmbeddingTx(ctx, tx, emb)
			_ = tx.Commit()
		} else {
			_ = uc.repository.StoreVerbatim(ctx, verbatim)
			_ = uc.repository.StoreFingerprint(ctx, fp)
			_ = uc.repository.StoreEmbedding(ctx, emb)
		}

		candidate := entities.NewCandidate(fp, verbatim, emb.Vector)
		_ = uc.vectorStore.AddCandidate(ctx, candidate)

		output.ConsolidatedCount++

		// Remove original notes
		for _, n := range cluster {
			_, _ = uc.repository.ClearByRoom(ctx, input.Wing, func() *string { r := n.verbatim.Room; if r == nil { return nil }; return r }())
			output.RemovedCount++
		}
	}

	return output, nil
}

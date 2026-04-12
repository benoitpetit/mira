//go:build !windows
// +build !windows

package vector

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/coder/hnsw"
	"github.com/google/uuid"
)

// HNSWStore implements VectorStore using HNSW algorithm for O(log n) ANN search
type HNSWStore struct {
	graph     *hnsw.Graph[node]
	store     ports.EmbeddingSource
	dimension int
	indexPath string
	mu        sync.RWMutex
	idToUUID  map[string]uuid.UUID
	uuidToID  map[uuid.UUID]string
	nextID    int
	ready     bool
}

// node wraps a candidate for HNSW
type node struct {
	id        string
	embedding hnsw.Embedding
}

func (n node) ID() string               { return n.id }
func (n node) Embedding() hnsw.Embedding { return n.embedding }

// HNSWOptions holds configuration for HNSW
type HNSWOptions struct {
	M              int
	Ml             float64
	EfConstruction int
	EfSearch       int
	// FetchMultiplier controls how many extra results to fetch for filtering
	// Default is 1.5 (fetch 1.5x limit). Higher = more accurate but slower.
	FetchMultiplier float64
}

// DefaultHNSWOptions returns default HNSW options
// Balanced for speed and accuracy
func DefaultHNSWOptions() HNSWOptions {
	return HNSWOptions{
		M:               16,
		Ml:              0.25,
		EfConstruction:  200,
		EfSearch:        50,   // Default from hnsw library
		FetchMultiplier: 1.5,  // Reduced from 2.0 for less overallocation
	}
}

// FastHNSWOptions returns options optimized for maximum search speed
// Best for read-heavy workloads where approximate results are acceptable
func FastHNSWOptions() HNSWOptions {
	return HNSWOptions{
		M:               12,   // Lower M = faster, less memory
		Ml:              0.25,
		EfConstruction:  100,  // Lower = faster build
		EfSearch:        20,   // Lower = faster search
		FetchMultiplier: 1.2,  // Minimal overallocation
	}
}

// AccurateHNSWOptions returns options optimized for accuracy over speed
// Best when precision is critical
func AccurateHNSWOptions() HNSWOptions {
	return HNSWOptions{
		M:               24,   // Higher M = better quality
		Ml:              0.25,
		EfConstruction:  300,  // Higher = better graph quality
		EfSearch:        100,  // Higher = more accurate search
		FetchMultiplier: 2.0,  // More buffer for filtering
	}
}

// NewHNSWStore creates a new HNSW vector store
func NewHNSWStore(store ports.EmbeddingSource, dimension int, indexPath string, opts HNSWOptions) (*HNSWStore, error) {
	h := &HNSWStore{
		store:     store,
		dimension: dimension,
		indexPath: indexPath,
		idToUUID:  make(map[string]uuid.UUID),
		uuidToID:  make(map[uuid.UUID]string),
		nextID:    0,
		ready:     false,
	}

	// Register cosine distance function
	hnsw.RegisterDistanceFunc("cosine", hnsw.DistanceFunc(util.CosineDistance))

	// Create new empty graph
	h.graph = hnsw.NewGraph[node]()
	h.applyOptions(opts)

	// Create index directory if needed
	if indexPath != "" {
		if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create index directory: %w", err)
		}
	}

	return h, nil
}

// storeOptions holds options that aren't in hnsw.Graph
type storeOptions struct {
	fetchMultiplier float64
}

var globalStoreOpts = storeOptions{
	fetchMultiplier: 1.5,
}

func (h *HNSWStore) applyOptions(opts HNSWOptions) {
	h.graph.M = opts.M
	h.graph.Ml = opts.Ml
	// Use adaptive EfSearch by default, but respect user override if significantly different
	if opts.EfSearch > 0 {
		h.graph.EfSearch = opts.EfSearch
	}
	h.graph.Distance = hnsw.DistanceFunc(util.CosineDistance)
	if opts.FetchMultiplier > 0 {
		globalStoreOpts.fetchMultiplier = opts.FetchMultiplier
	}
}

// Search implements VectorStore
func (h *HNSWStore) Search(ctx context.Context, queryVec []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.ready {
		return nil, fmt.Errorf("HNSW index not ready")
	}

	// Search in HNSW with configured EfSearch
	queryEmbedding := floatsToEmbedding(queryVec)
	results := h.graph.Search(queryEmbedding, h.graph.EfSearch)

	// Pre-allocate slice with estimated capacity to avoid reallocations
	// Use configurable multiplier for filtering buffer (default 1.5x)
	fetchMultiplier := 1.5
	if opts := h.getOptions(); opts != nil && opts.FetchMultiplier > 0 {
		fetchMultiplier = opts.FetchMultiplier
	}
	expectedResults := int(float64(limit) * fetchMultiplier)
	if expectedResults < limit+5 {
		expectedResults = limit + 5 // Minimum buffer
	}
	ids := make([]uuid.UUID, 0, expectedResults)

	// Collect UUIDs from results with early termination
	for _, r := range results {
		if id, ok := h.idToUUID[r.ID()]; ok {
			ids = append(ids, id)
			if len(ids) >= expectedResults {
				break // We have enough candidates
			}
		}
	}

	// Batch fetch candidates with single JOIN query
	candidates, err := h.batchGetCandidates(ctx, ids, wing, room)
	if err != nil {
		return nil, err
	}

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
}

// getOptions returns the current HNSW options
func (h *HNSWStore) getOptions() *HNSWOptions {
	if h.graph == nil {
		return nil
	}
	return &HNSWOptions{
		M:               h.graph.M,
		Ml:              h.graph.Ml,
		EfSearch:        h.graph.EfSearch,
		FetchMultiplier: globalStoreOpts.fetchMultiplier,
	}
}

// UpdateOptions updates HNSW options dynamically (thread-safe)
// Note: M, Ml, and EfConstruction only affect new inserts
// EfSearch affects subsequent searches immediately
func (h *HNSWStore) UpdateOptions(opts HNSWOptions) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.applyOptions(opts)
	log.Printf("[Vector] HNSW options updated: EfSearch=%d, FetchMultiplier=%.1f", 
		h.graph.EfSearch, globalStoreOpts.fetchMultiplier)
}

// batchGetCandidates fetches multiple candidates using the EmbeddingSource interface
func (h *HNSWStore) batchGetCandidates(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error) {
	return h.store.GetCandidatesWithEmbeddings(ctx, ids, wing, room)
}

// AddCandidate implements VectorStore
func (h *HNSWStore) AddCandidate(ctx context.Context, c *entities.Candidate) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.getNextID()
	h.idToUUID[id] = c.ID()
	h.uuidToID[c.ID()] = id

	n := node{
		id:        id,
		embedding: floatsToEmbedding(c.Embedding),
	}

	h.graph.Add(n)
	return nil
}

// Delete implements VectorStore
func (h *HNSWStore) Delete(ctx context.Context, id uuid.UUID) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if strID, ok := h.uuidToID[id]; ok {
		h.graph.Delete(strID)
		delete(h.uuidToID, id)
		delete(h.idToUUID, strID)
	}
	return nil
}

// BuildFromStore builds the index from existing data in the store
// Optimized with pre-allocated maps for reduced GC pressure
func (h *HNSWStore) BuildFromStore(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	embeddings, err := h.store.GetAllEmbeddings(ctx)
	if err != nil {
		return fmt.Errorf("failed to query embeddings: %w", err)
	}

	count := len(embeddings)
	if count == 0 {
		h.ready = true
		log.Printf("[Vector] Index ready: 0 vectors, %dd dims", h.dimension)
		return nil
	}

	// Pre-allocate maps with known capacity to avoid reallocations
	// This significantly reduces GC pressure for large datasets
	if h.idToUUID == nil {
		h.idToUUID = make(map[string]uuid.UUID, count)
	} else {
		// Clear and reuse existing map if possible
		for k := range h.idToUUID {
			delete(h.idToUUID, k)
		}
	}
	if h.uuidToID == nil {
		h.uuidToID = make(map[uuid.UUID]string, count)
	} else {
		for k := range h.uuidToID {
			delete(h.uuidToID, k)
		}
	}

	// Reuse embedding buffer to reduce allocations
	// The hnsw library copies the embedding, so we can reuse our buffer
	added := 0
	for _, emb := range embeddings {
		strID := h.getNextID()
		h.idToUUID[strID] = emb.ID
		h.uuidToID[emb.ID] = strID

		n := node{
			id:        strID,
			embedding: floatsToEmbedding(emb.Vector),
		}
		h.graph.Add(n)
		added++
	}

	h.ready = true
	log.Printf("[Vector] Index ready: %d vectors, %dd dims", added, h.dimension)
	return nil
}

// IsReady returns whether the index is ready for queries
func (h *HNSWStore) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// Stats returns index statistics
func (h *HNSWStore) Stats() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.graph.Len()
}

func (h *HNSWStore) getNextID() string {
	id := fmt.Sprintf("node_%d", h.nextID)
	h.nextID++
	return id
}

func floatsToEmbedding(v []float32) hnsw.Embedding {
	return v
}

// hnswNodeData représente un nœud à persister
type hnswNodeData struct {
	ID        string    // ID interne
	UUID      string    // UUID original
	Embedding []float32 // Vecteur
}

// hnswIndexData représente les données complètes à persister
type hnswIndexData struct {
	Version   string            // Version du format
	Dimension int               // Dimension des embeddings
	NodeCount int               // Nombre de nœuds
	Nodes     []hnswNodeData    // Données des nœuds
	UUIDToID  map[string]string // Mapping UUID -> ID interne
	NextID    int               // Prochain ID disponible
	SavedAt   time.Time         // Date de sauvegarde
}

// Save persists the complete HNSW index to disk (mappings + all nodes with embeddings)
func (h *HNSWStore) Save() error {
	if h.indexPath == "" {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Préparer les données de mappings
	uuidToID := make(map[string]string)
	for uuid, id := range h.uuidToID {
		uuidToID[uuid.String()] = id
	}

	// Collecter tous les nœuds du graphe
	nodes := make([]hnswNodeData, 0, h.graph.Len())
	for uuid, id := range h.uuidToID {
		// Récupérer le nœud depuis le graphe
		n, ok := h.graph.Lookup(id)
		if !ok {
			continue // Nœud non trouvé dans le graphe
		}

		// Copier l'embedding
		embedding := make([]float32, len(n.Embedding()))
		copy(embedding, n.Embedding())

		nodes = append(nodes, hnswNodeData{
			ID:        id,
			UUID:      uuid.String(),
			Embedding: embedding,
		})
	}

	// Construire la structure de données complète
	data := hnswIndexData{
		Version:   "1.0",
		Dimension: h.dimension,
		NodeCount: len(nodes),
		Nodes:     nodes,
		UUIDToID:  uuidToID,
		NextID:    h.nextID,
		SavedAt:   time.Now(),
	}

	// Créer un fichier temporaire pour une sauvegarde atomique
	tmpPath := h.indexPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp index file: %w", err)
	}

	// Encoder avec gob
	if err := gob.NewEncoder(file).Encode(data); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to encode index: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp index file: %w", err)
	}

	// Remplacement atomique
	if err := os.Rename(tmpPath, h.indexPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename index file: %w", err)
	}

	log.Printf("[Vector] HNSW index saved: %d vectors, %d mappings", data.NodeCount, len(data.UUIDToID))
	return nil
}

// Load loads the complete HNSW index from disk (mappings + all nodes with embeddings)
func (h *HNSWStore) Load() error {
	if h.indexPath == "" {
		return nil
	}

	// Vérifier si le fichier existe
	if _, err := os.Stat(h.indexPath); os.IsNotExist(err) {
		log.Println("[Vector] HNSW index file not found, will build from scratch")
		return nil
	}

	// Ouvrir le fichier
	file, err := os.Open(h.indexPath)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer file.Close()

	// Décoder les données
	var data hnswIndexData
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		// Essayer de charger l'ancien format (sans Nodes)
		file.Seek(0, 0)
		var oldData struct {
			UUIDToID map[string]string
			NextID   int
		}
		if err := gob.NewDecoder(file).Decode(&oldData); err != nil {
			return fmt.Errorf("failed to decode index: %w", err)
		}
		// Migrer depuis l'ancien format
		data.Version = "1.0"
		data.UUIDToID = oldData.UUIDToID
		data.NextID = oldData.NextID
		data.Dimension = h.dimension
		data.Nodes = nil
		log.Println("[Vector] Loaded legacy index format, will rebuild graph from DB")
	}

	// Vérifier la version
	if data.Version != "1.0" {
		return fmt.Errorf("unsupported index version: %s", data.Version)
	}

	// Vérifier la dimension
	if data.Dimension != 0 && data.Dimension != h.dimension {
		return fmt.Errorf("dimension mismatch: saved=%d, expected=%d", data.Dimension, h.dimension)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Restaurer les mappings
	h.nextID = data.NextID
	for uuidStr, id := range data.UUIDToID {
		idUUID, err := uuid.Parse(uuidStr)
		if err != nil {
			log.Printf("[Vector] Warning: invalid UUID in index file: %s", uuidStr)
			continue
		}
		h.uuidToID[idUUID] = id
		h.idToUUID[id] = idUUID
	}

	// Si nous avons des nœuds sauvegardés, reconstruire le graphe
	if len(data.Nodes) > 0 {
		for _, nodeData := range data.Nodes {
			// Vérifier la dimension du vecteur
			if len(nodeData.Embedding) != h.dimension {
				log.Printf("[Vector] Warning: skipping node %s with wrong dimension: got %d, expected %d",
					nodeData.ID, len(nodeData.Embedding), h.dimension)
				continue
			}

			n := node{
				id:        nodeData.ID,
				embedding: floatsToEmbedding(nodeData.Embedding),
			}
			h.graph.Add(n)
		}

		// Vérifier que le nombre de nœuds chargés correspond
		if h.graph.Len() != len(data.Nodes) {
			log.Printf("[Vector] Warning: loaded %d nodes but expected %d",
				h.graph.Len(), len(data.Nodes))
		}

		h.ready = true
		log.Printf("[Vector] HNSW index loaded: %d vectors, %d mappings (saved at %s)",
			h.graph.Len(), len(h.uuidToID), data.SavedAt.Format(time.RFC3339))
		return nil
	}

	log.Printf("[Vector] HNSW mappings loaded: %d mappings, nextID=%d (graph will be rebuilt)",
		len(h.uuidToID), h.nextID)
	return nil
}



// timeUnix converts Unix timestamp (float64) to time.Time
func timeUnix(sec float64) time.Time {
	return time.Unix(int64(sec), 0)
}

var _ ports.VectorStore = (*HNSWStore)(nil)

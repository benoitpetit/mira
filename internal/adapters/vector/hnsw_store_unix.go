// Package vector provides vector search adapters using HNSW and SQLite.
//go:build !windows
// +build !windows

package vector

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"crypto/sha256"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/coder/hnsw"
	"github.com/google/uuid"
)

// HNSWStore implements VectorStore using HNSW algorithm for O(log n) ANN search
type HNSWStore struct {
	graph         *hnsw.Graph[node]
	store         ports.EmbeddingSource
	dimension     int
	modelHash     string
	indexPath     string
	encryptionKey []byte // 32-byte AES-256 key; nil = no encryption
	mu            sync.RWMutex
	idToUUID      map[string]uuid.UUID
	uuidToID      map[uuid.UUID]string
	nextID        int
	ready         bool
}

// node wraps a candidate for HNSW
type node struct {
	id        string
	embedding hnsw.Embedding
}

func (n node) ID() string                { return n.id }
func (n node) Embedding() hnsw.Embedding { return n.embedding }

// HNSWOptions holds configuration for HNSW
type HNSWOptions struct {
	M              int
	Ml             float64
	EfConstruction int
	EfSearch       int
}

// DefaultHNSWOptions returns default HNSW options
func DefaultHNSWOptions() HNSWOptions {
	return HNSWOptions{
		M:              16,
		Ml:             0.25,
		EfConstruction: 200,
		EfSearch:       50,
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

func (h *HNSWStore) applyOptions(opts HNSWOptions) {
	h.graph.M = opts.M
	h.graph.Ml = opts.Ml
	h.graph.EfSearch = opts.EfSearch
	h.graph.Distance = hnsw.DistanceFunc(util.CosineDistance)
}

// SetModelHash sets the expected embedding model hash for validation.
func (h *HNSWStore) SetModelHash(hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.modelHash = hash
}

// SetEncryptionKey sets the AES-256-GCM key used to encrypt/decrypt vectors.bin.
// key must be exactly 32 bytes (derived externally, e.g. via SHA-256 of a passphrase).
// Passing nil disables encryption.
func (h *HNSWStore) SetEncryptionKey(key []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(key) == 32 {
		h.encryptionKey = key
	} else if len(key) == 0 {
		h.encryptionKey = nil
	} else {
		// Normalise to 32 bytes via SHA-256 so callers don't have to
		sum := sha256.Sum256(key)
		h.encryptionKey = sum[:]
	}
}

// encryptAESGCM encrypts plaintext with AES-256-GCM using the store's key.
// Output format: random 12-byte nonce || ciphertext || 16-byte GCM tag.
func (h *HNSWStore) encryptAESGCM(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil) // nonce prepended
	return ciphertext, nil
}

// decryptAESGCM decrypts data produced by encryptAESGCM.
func (h *HNSWStore) decryptAESGCM(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aes gcm decrypt: %w", err)
	}
	return plaintext, nil
}

// SearchLexical implements VectorStore by delegating to the underlying EmbeddingSource.
func (h *HNSWStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return h.store.SearchLexical(ctx, query, limit, wing, room)
}

// SearchExact implements VectorStore exact search by delegating to the underlying store.
func (h *HNSWStore) SearchExact(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if h.store != nil {
		if exactStore, ok := h.store.(interface {
			SearchExact(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error)
		}); ok {
			return exactStore.SearchExact(ctx, query, limit, wing, room)
		}
	}
	return nil, nil
}

// Search implements VectorStore
func (h *HNSWStore) Search(ctx context.Context, queryVec []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.ready {
		return nil, fmt.Errorf("HNSW index not ready")
	}

	// Validate query vector dimension
	if len(queryVec) != h.dimension {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d, expected %d", len(queryVec), h.dimension)
	}

	queryEmbedding := floatsToEmbedding(queryVec)
	totalVectors := h.graph.Len()

	// When no wing/room filter is active a single pass at limit*2 is sufficient.
	// When a filter IS active (sparse wings), the initial pass may miss vectors that
	// belong to that wing but rank outside the top limit*2 globally.  We progressively
	// widen the search — limit*2 → limit*8 → limit*32 → all — until we have at least
	// one filtered result or we have exhausted the index.
	wideSearch := wing != nil || room != nil

	searchK := limit * 2
	if searchK > totalVectors {
		searchK = totalVectors
	}

	var candidates []*entities.Candidate
	for {
		results := h.graph.Search(queryEmbedding, searchK)

		// Collect UUIDs from results
		var ids []uuid.UUID
		for _, r := range results {
			if id, ok := h.idToUUID[r.ID()]; ok {
				ids = append(ids, id)
			}
		}

		// Batch fetch candidates with single JOIN query
		var err error
		candidates, err = h.batchGetCandidates(ctx, ids, wing, room)
		if err != nil {
			return nil, err
		}

		// Stop when we have enough results, or when no filter is active (single pass),
		// or when we have already searched the full index.
		if len(candidates) >= limit || !wideSearch || searchK >= totalVectors {
			break
		}

		// Expand: 4× each time until the whole index is covered.
		next := searchK * 4
		if next > totalVectors {
			next = totalVectors
		}
		if next == searchK {
			break // no progress possible
		}
		searchK = next
	}

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
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
	h.idToUUID[id] = c.Verbatim.ID
	h.uuidToID[c.Verbatim.ID] = id

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

// ClearAll implements VectorStore
func (h *HNSWStore) ClearAll(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.graph = hnsw.NewGraph[node]()
	h.idToUUID = make(map[string]uuid.UUID)
	h.uuidToID = make(map[uuid.UUID]string)
	h.nextID = 0
	h.ready = false

	if h.indexPath != "" {
		_ = os.Remove(h.indexPath)
	}

	return nil
}

// ClearByRoom implements VectorStore
func (h *HNSWStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	// Rebuild the index from the database, which has already been cleared
	return h.BuildFromStore(ctx)
}

// BuildFromStore builds the index from existing data in the store
func (h *HNSWStore) BuildFromStore(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	embeddings, err := h.store.GetAllEmbeddings(ctx)
	if err != nil {
		return fmt.Errorf("failed to query embeddings: %w", err)
	}

	count := 0
	for _, emb := range embeddings {
		// Add to HNSW
		strID := h.getNextID()
		h.idToUUID[strID] = emb.ID
		h.uuidToID[emb.ID] = strID

		n := node{
			id:        strID,
			embedding: floatsToEmbedding(emb.Vector),
		}
		h.graph.Add(n)
		count++
	}

	h.ready = true
	log.Printf("[Vector] Index ready: %d vectors, %dd dims", count, h.dimension)
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
	ModelHash string            // Hash du modèle d'embedding
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
		ModelHash: h.modelHash,
		NodeCount: len(nodes),
		Nodes:     nodes,
		UUIDToID:  uuidToID,
		NextID:    h.nextID,
		SavedAt:   time.Now(),
	}

	// Encoder en mémoire (nécessaire pour le chiffrement optionnel)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode index: %w", err)
	}
	payload := buf.Bytes()

	// Chiffrement AES-256-GCM optionnel
	if len(h.encryptionKey) == 32 {
		encrypted, err := h.encryptAESGCM(payload)
		if err != nil {
			return fmt.Errorf("failed to encrypt index: %w", err)
		}
		payload = encrypted
		log.Printf("[Vector] HNSW index encrypted (AES-256-GCM)")
	}

	// Sauvegarde atomique : écriture dans .tmp puis rename
	tmpPath := h.indexPath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0600); err != nil {
		return fmt.Errorf("failed to write temp index file: %w", err)
	}

	// Remplacement atomique
	if err := os.Rename(tmpPath, h.indexPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename index file: %w", err)
	}

	// Calculer et écrire le checksum sur le fichier final (chiffré ou non)
	written, err := os.ReadFile(h.indexPath)
	if err != nil {
		return fmt.Errorf("failed to read saved index for checksum: %w", err)
	}
	hash := sha256.Sum256(written)
	checksumPath := h.indexPath + ".sha256"
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(hash[:])), 0644); err != nil {
		return fmt.Errorf("failed to write checksum file: %w", err)
	}

	encrypted := len(h.encryptionKey) == 32
	log.Printf("[Vector] HNSW index saved: %d vectors, %d mappings, encrypted=%v", data.NodeCount, len(data.UUIDToID), encrypted)
	return nil
}

// Load loads the complete HNSW index from disk (mappings + all nodes with embeddings)
func (h *HNSWStore) Load() error {
	if h.indexPath == "" {
		return nil
	}

	// Vérifier si le fichier existe
	if _, err := os.Stat(h.indexPath); os.IsNotExist(err) {
		log.Printf("[Vector] HNSW index file not found, will build from scratch")
		return nil
	}

	// Lire les bytes bruts du fichier (chiffrés ou non)
	raw, err := os.ReadFile(h.indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	// Vérifier le checksum sur les bytes bruts (avant déchiffrement)
	checksumPath := h.indexPath + ".sha256"
	if checksumData, err := os.ReadFile(checksumPath); err == nil {
		hash := sha256.Sum256(raw)
		expected := hex.EncodeToString(hash[:])
		if expected != string(checksumData) {
			log.Printf("[Vector] HNSW index checksum mismatch. Rebuild required.")
			return fmt.Errorf("index checksum mismatch")
		}
	}

	// Déchiffrement AES-256-GCM optionnel
	payload := raw
	if len(h.encryptionKey) == 32 {
		decrypted, err := h.decryptAESGCM(raw)
		if err != nil {
			return fmt.Errorf("failed to decrypt index (wrong key?): %w", err)
		}
		payload = decrypted
		log.Printf("[Vector] HNSW index decrypted (AES-256-GCM)")
	}

	// Décoder les données
	var data hnswIndexData
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&data); err != nil {
		// Essayer de charger l'ancien format (sans Nodes) — seulement si non chiffré
		if len(h.encryptionKey) == 32 {
			return fmt.Errorf("failed to decode encrypted index: %w", err)
		}
		var oldData struct {
			UUIDToID map[string]string
			NextID   int
		}
		if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&oldData); err != nil {
			return fmt.Errorf("failed to decode index: %w", err)
		}
		// Migrer depuis l'ancien format
		data.Version = "1.0"
		data.UUIDToID = oldData.UUIDToID
		data.NextID = oldData.NextID
		data.Dimension = h.dimension
		data.Nodes = nil
		log.Printf("[Vector] Loaded legacy index format, will rebuild graph from DB")
	}

	// Vérifier la version
	if data.Version != "1.0" {
		return fmt.Errorf("unsupported index version: %s", data.Version)
	}

	// Vérifier la dimension
	if data.Dimension != 0 && data.Dimension != h.dimension {
		return fmt.Errorf("dimension mismatch: saved=%d, expected=%d", data.Dimension, h.dimension)
	}

	// Vérifier le model hash
	if data.ModelHash != "" && h.modelHash != "" && data.ModelHash != h.modelHash {
		return fmt.Errorf("model hash mismatch: saved=%s, expected=%s", data.ModelHash, h.modelHash)
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

package budget

import (
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/benoitpetit/mira/extract"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
)

var (
	benchExtractorOnce sync.Once
	benchExtractor     *extract.Extractor
)

func getBenchExtractor() *extract.Extractor {
	benchExtractorOnce.Do(func() {
		embedder := &benchEmbedder{}
		var err error
		benchExtractor, err = extract.NewExtractorWithOptions("test-model", embedder, extract.ExtractorOptions{})
		if err != nil {
			panic(err)
		}
	})
	return benchExtractor
}

type benchEmbedder struct{}

func (m *benchEmbedder) Encode(text string) ([]float32, error) {
	vec := make([]float32, 384)
	// Generate deterministic pseudo-random vector based on text
	seed := int64(0)
	for _, c := range text {
		seed = seed*31 + int64(c)
	}
	r := rand.New(rand.NewSource(seed))
	for i := range vec {
		vec[i] = r.Float32()*2 - 1 // [-1, 1]
	}
	// Normalize
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

func generateCandidates(n int, memoryType types.MemoryType) []*types.Candidate {
	candidates := make([]*types.Candidate, n)
	now := time.Now()
	embedder := &benchEmbedder{}

	for i := 0; i < n; i++ {
		content := "Test content for candidate " + string(rune('A'+i%26)) + " with decision about PostgreSQL and database migration"
		vec, _ := embedder.Encode(content)

		candidates[i] = &types.Candidate{
			Memory: &types.Fingerprint{
				ID:            uuid.New(),
				Type:          memoryType,
				FactCount:     rand.Intn(10) + 1,
				TokenEstimate: rand.Intn(50) + 20,
				Data: types.FingerprintData{
					Subject:  []string{"database"},
					Decision: "Use PostgreSQL",
				},
			},
			Verbatim: &types.Verbatim{
				ID:         uuid.New(),
				Content:    content,
				TokenCount: rand.Intn(100) + 50,
				CreatedAt:  now.Add(-time.Duration(rand.Intn(30)) * 24 * time.Hour),
				Wing:       "backend",
			},
			Embedding: vec,
		}
	}
	return candidates
}

func BenchmarkScoreCandidates(b *testing.B) {
	candidates := generateCandidates(100, types.TypeDecision)
	alloc := &Allocator{opts: AllocatorOptions{}.withDefaults()}
	queryVec, _ := (&benchEmbedder{}).Encode("PostgreSQL database decision")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		alloc.scoreCandidates(candidates, queryVec)
	}
}

func BenchmarkAllocate10(b *testing.B) {
	benchmarkAllocate(b, 10)
}

func BenchmarkAllocate50(b *testing.B) {
	benchmarkAllocate(b, 50)
}

func BenchmarkAllocate100(b *testing.B) {
	benchmarkAllocate(b, 100)
}

func BenchmarkAllocate200(b *testing.B) {
	benchmarkAllocate(b, 200)
}

func benchmarkAllocate(b *testing.B, n int) {
	candidates := generateCandidates(n, types.TypeDecision)
	vecStore := &mockVectorStore{candidates: candidates}
	causal := &mockCausalGraph{}

	// Use shared extractor to avoid tiktoken reload
	alloc := NewAllocatorWithOptions(vecStore, nil, causal, getBenchExtractor(), AllocatorOptions{})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		alloc.Allocate("query", 4000, nil, nil)
	}
}

func BenchmarkCosineSimilarity(b *testing.B) {
	vec1 := make([]float32, 384)
	vec2 := make([]float32, 384)
	for i := range vec1 {
		vec1[i] = rand.Float32()
		vec2[i] = rand.Float32()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		util.CosineSimilarity(vec1, vec2)
	}
}

func BenchmarkDetermineRenderMode(b *testing.B) {
	alloc := &Allocator{opts: AllocatorOptions{}.withDefaults()}
	c := &types.Candidate{
		Verbatim: &types.Verbatim{TokenCount: 100},
		Memory:   &types.Fingerprint{TokenEstimate: 50},
	}

	modes := []int{50, 500, 1500}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alloc.determineRenderMode(c, modes[i%len(modes)])
	}
}

func BenchmarkEmbeddingCache(b *testing.B) {
	cache := newEmbeddingCache(1000)
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = rand.Float32()
	}

	b.Run("Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.set(string(rune('a'+i%26)), vec)
		}
	})

	b.Run("Read", func(b *testing.B) {
		cache.set("test", vec)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.get("test")
		}
	})

	b.Run("ReadWrite", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				cache.set(string(rune('a'+i%26)), vec)
			} else {
				cache.get(string(rune('a' + (i-1)%26)))
			}
		}
	})
}

func BenchmarkPriorityQueue(b *testing.B) {
	b.Run("Push", func(b *testing.B) {
		pq := make(priorityQueue, 0)
		for i := 0; i < b.N; i++ {
			c := &types.Candidate{Score: rand.Float64()}
			pq.Push(&item{candidate: c, priority: c.Score})
		}
	})

	b.Run("PushPop", func(b *testing.B) {
		pq := make(priorityQueue, 0)
		for i := 0; i < 100; i++ {
			c := &types.Candidate{Score: rand.Float64()}
			pq.Push(&item{candidate: c, priority: c.Score})
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pq.Push(&item{candidate: &types.Candidate{Score: rand.Float64()}, priority: rand.Float64()})
			pq.Pop()
		}
	})
}

// Benchmark complete workflow
func BenchmarkCompleteAllocationWorkflow(b *testing.B) {
	// Generate diverse candidates
	candidates := generateCandidates(100, types.TypeDecision)
	candidates = append(candidates, generateCandidates(50, types.TypeFact)...)
	candidates = append(candidates, generateCandidates(50, types.TypePreference)...)

	vecStore := &mockVectorStore{candidates: candidates}
	causal := &mockCausalGraph{}

	// Use shared extractor
	alloc := NewAllocatorWithOptions(vecStore, nil, causal, getBenchExtractor(), AllocatorOptions{})

	queries := []string{"q1", "q2", "q3", "q4"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		alloc.Allocate(query, 4000, nil, nil)
	}
}

// Memory allocation benchmark
func BenchmarkAllocatorMemory(b *testing.B) {
	candidates := generateCandidates(100, types.TypeDecision)
	vecStore := &mockVectorStore{candidates: candidates}
	causal := &mockCausalGraph{}

	alloc := NewAllocatorWithOptions(vecStore, nil, causal, getBenchExtractor(), AllocatorOptions{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alloc.Allocate("test query", 4000, nil, nil)
	}
}

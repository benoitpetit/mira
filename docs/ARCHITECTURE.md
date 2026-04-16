# MIRA Architecture

This document provides a comprehensive technical deep-dive into MIRA's architecture, algorithms, and data model.

---

## Table of Contents

1. [Clean Architecture](#clean-architecture)
2. [T0/T1/T2 Memory Hierarchy](#t0t1t2-memory-hierarchy)
3. [Context Budget Allocator (CBA)](#context-budget-allocator-cba)
4. [Causal Graph](#causal-graph)
5. [Recall Pipeline](#recall-pipeline)
6. [Storage Layer](#storage-layer)
7. [Vector Search](#vector-search)
8. [Configuration Model](#configuration-model)

---

## Clean Architecture

MIRA follows **Uncle Bob's Clean Architecture** with strict dependency direction from outer layers inward.

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CLEAN ARCHITECTURE                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  DOMAIN (Enterprise Rules)                                  │   │
│   │  • entities: Verbatim, Fingerprint, Embedding, Candidate    │   │
│   │  • valueobjects: MemoryType, RenderMode, RelationType       │   │
│   │  ✓ No external dependencies                                 │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │ Dependency                           │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  USE CASES (Application Rules)                              │   │
│   │  • StoreMemory, RecallMemory (CBA), LoadMemory              │   │
│   │  • GetTimeline, GetStatus, GetCausalChain, Archive          │   │
│   │  • ports: Repository interfaces                             │   │
│   │  ✓ Depends only on Domain                                   │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  INTERFACE ADAPTERS                                         │   │
│   │  • storage: SQLiteRepository                                │   │
│   │  • vector: HNSWStore, SQLiteVectorStore, FallbackVectorStore│   │
│   │  • extraction: NativeExtractor, CybertronEmbedder           │   │
│   │  • webhook, metrics, logging                                │   │
│   │  ✓ Implements ports                                         │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  FRAMEWORKS & DRIVERS                                       │   │
│   │  • SQLite3, HNSW lib, Cybertron, MCP Server                 │   │
│   │  ✓ External technical details                               │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Project Structure

```
mira/
├── cmd/mira/                    # Entry point
├── internal/
│   ├── domain/                  # Pure business logic
│   │   ├── entities/            # Verbatim, Fingerprint, Embedding, Candidate, CausalNode
│   │   └── valueobjects/        # MemoryType, RenderMode, RelationType, Stats
│   ├── usecases/                # Application rules
│   │   ├── ports/               # Repository and service interfaces
│   │   └── interactors/         # Use case implementations
│   ├── adapters/                # Infrastructure implementations
│   │   ├── storage/             # SQLite repository, migrations
│   │   ├── vector/              # HNSW, SQLite vector store, overlap cache
│   │   ├── extraction/          # NLP, embeddings (Cybertron, Simple)
│   │   ├── logging/             # Structured logging
│   │   ├── metrics/             # Prometheus and simple collectors
│   │   └── webhook/             # HTTP notifications
│   ├── interfaces/              # External protocol adapters
│   │   └── mcp/                 # MCP controller
│   ├── config/                  # Configuration loading and validation
│   └── app/                     # Composition root (DI)
│       ├── main.go              # Dependency injection
│       ├── health.go            # Health checks
│       └── main_test.go         # Application tests
├── docs/                        # Documentation
├── scripts/                     # Benchmarks and utilities
├── config.example.yaml          # Example configuration
└── README.md                    # Project overview
```

---

## T0/T1/T2 Memory Hierarchy

The human brain does not record everything with the same fidelity. MIRA mimics this with three representation levels:

### T0 — Verbatim (Episodic Memory)
- **Content**: Full original UTF-8 text (max 64KB)
- **Usage**: Rich context when budget allows
- **Cost**: ~200 tokens

### T1 — Fingerprint (Semantic Memory)
- **Content**: Structured canonical JSON with extracted facts (~15% of T0 tokens)
- **Usage**: Dense context for medium budgets
- **Cost**: ~30 tokens

### T2 — Embedding (Procedural Memory)
- **Content**: 384-dimensional float32 vector
- **Usage**: Semantic search, zero token cost at inference
- **Model**: `sentence-transformers/all-MiniLM-L6-v2`

### Render Modes

| Remaining Budget | Mode        | Tokens | Content              |
|------------------|-------------|--------|----------------------|
| < 100            | Header      | 2-5    | `[type\|date\|wing]` |
| < 1000           | Fingerprint | ~15%   | Essential facts T1   |
| ≥ 1000           | Verbatim    | 100%   | Original text T0     |

### Memory Types and Decay

| Type           | λ (day⁻¹) | Half-life | Auto-Archive | Usage                   |
|----------------|-----------|-----------|--------------|-------------------------|
| `decision`     | 0.001     | ~693 days | Never        | Architectural choices   |
| `fact`         | 0.005     | ~139 days | Never        | Knowledge, configs      |
| `preference`   | 0.01      | ~69 days  | Never        | User preferences        |
| `session_note` | 0.1       | ~7 days   | 30 days      | Session context         |
| `debug_log`    | 0.5       | ~1.4 days | 7 days       | Errors and fixes        |

---

## Context Budget Allocator (CBA)

MIRA's retrieval is not "top-k by similarity". It is a **constrained optimization** that maximizes information density within a token budget.

### Composite Score Formula

```
S(m) = ρ × δ × η × (1-σ) × τ × χ × 𝟙[ρ>θ]

where:
  ρ = Semantic Relevance     cos(embedding_m, embedding_q)
  δ = Information Density    sigmoid(facts / √tokens)
  η = Temporal Weight        exp(-λ × age_days)
  σ = Max Overlap            sim(m, already_selected)
  τ = Session Boost          1.2 if within 2h window
  χ = Causal Penalty         exp(-0.15 × |causal_links|)
  θ = Adaptive Threshold     dynamic relevance floor
```

### Algorithm — O(n log n)

```
1. EMBEDDING
   e_q ← Embed(q) with LRU cache (1000 entries)
   If query expansion enabled:
      variants ← expandQuery(q)
      e_q ← average(embed(variants))

2. VECTOR SEARCH
   dense ← HNSW_Search(e_q, N=100, wing, room)        # O(log n)
   lexical ← FTS5_Search(q, N=100, wing, room)        # lexical fallback
   candidates ← RRF_Fusion(dense, lexical, k=60)

3. SEARCH-TIME CLUSTERING (optional)
   clusters ← clusterCandidates(candidates, threshold=0.88)
   candidates ← selectClusterRepresentatives(clusters)

4. SCORING & PRUNING
   scored ← scoreCandidates(candidates, e_q, tagBoosts)
   pruned ← pruneCandidates(scored)  # adaptive threshold

5. BROAD FALLBACK (if sparse results)
   If |pruned| < 3:
      broad ← Search(e_q, N=300)
      pruned ← merge(pruned, broad with θ=0.15)

6. RERANKING (optional)
   If reranker enabled:
      pruned ← applyHeuristicReranker(q, pruned)

7. GREEDY SELECTION
   S ← ∅, tokens_used ← 0
   WHILE candidates remain AND tokens_used < B:
      Recalculate overlap, causal penalty, session boost
      Select best candidate
      Determine render mode from REMAINING budget
      Downgrade mode if necessary (Verbatim → Fingerprint → Header)
      Add to S

8. RETURN S sorted by descending score
```

### Adaptive Threshold Methods

MIRA dynamically computes the relevance cutoff based on the score distribution:

| Method   | Description                                      | Best For                 |
|----------|--------------------------------------------------|--------------------------|
| `iqr`    | First quartile (Q1) of relevance scores          | General purpose (default)|
| `elbow`  | Largest discrete derivative drop                 | Clear separation         |
| `mean_stddev` | mean - stddev                               | Gaussian distributions   |

The threshold is always clamped between `threshold_floor` (default 0.15) and `threshold_ceiling` (default 0.75).

---

## Causal Graph

MIRA automatically detects and stores causal relationships between memories.

### Supported Relations

| Relation     | Meaning                          | Example Pattern                  |
|--------------|----------------------------------|----------------------------------|
| `BECAUSE`    | B explains why A                 | "because", "since", "due to"     |
| `TRIGGERED`  | B triggered/caused A             | "following", "after", "in response to" |
| `CONTRADICTS`| A and B contradict               | "contradicts", "however"         |
| `UPDATES`    | B replaces/updates A             | "updates", "replaces"            |
| `RESOLVES`   | B resolves problem A             | "resolves", "solves", "fixes"    |

### Traversal
- `GetChain()`: BFS upstream (ancestors/causes)
- `GetConsequences()`: BFS downstream (descendants/effects)
- Cycle detection with visited-node tracking

---

## Recall Pipeline

The recall process in MIRA is a multi-stage retrieval pipeline:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        RECALL PIPELINE                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Query Input                                                           │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Variantes : clean, sans stop-words, mots-clés     │
│   │ Query       │───────────────────────────────────────────────────────│
│   │ Expansion   │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Recherche dense (HNSW) + recherche lexicale (FTS5)│
│   │ Hybrid      │    ─────────────────────────────────────────────────  │
│   │ Search      │                                                       │
│   │ (RRF k=60)  │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Clustering cos-sim ≥ 0.88 → un représentant       │
│   │ Search-Time │    ─────────────────────────────────────────────────  │
│   │ Clustering  │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Scoring CBA + boost par tags + seuil adaptatif    │
│   │ Scoring &   │    ─────────────────────────────────────────────────  │
│   │ Pruning     │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Reranking lexical heuristique (optionnel)         │
│   │ Heuristic   │    ─────────────────────────────────────────────────  │
│   │ Reranker    │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    Sélection gloutonne avec gestion de budget        │
│   │ Greedy CBA  │    ─────────────────────────────────────────────────  │
│   │ Allocation  │                                                       │
│   └─────────────┘                                                       │
│       │                                                                 │
│       ▼                                                                 │
│   Optimized Context Output                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Storage Layer

### SQLite Schema

The repository uses SQLite with WAL mode for concurrent read/write performance.

**Main Tables:**
- `verbatim` — T0 content, wing, room, tokens, timestamp
- `fingerprints` — T1 structured data, fact count, token estimate
- `embeddings` — T2 binary vectors (LittleEndian float32)
- `causal_nodes` / `causal_edges` — Graph structure
- `models` — Registered embedding model hashes
- `webhook_endpoints` / `webhook_dlq` — Webhook configuration and dead letter queue

**Additional Schema Elements:**
- `memory_tags` — Extracted tags (entity, subject, keyword) with FK to verbatim
- `verbatim_fts` — FTS5 virtual table for full-text search with auto-triggers

### Migrations

Migrations are applied automatically on startup:
- `001_initial.up.sql` — Core schema
- `002_add_webhook_dlq.up.sql` — Webhook DLQ
- `003_recall_enhancements.up.sql` — Tags and FTS5 support

---

## Vector Search

### HNSW (Primary)
- **Library**: `github.com/coder/hnsw`
- **Complexity**: O(log n) approximate nearest neighbor
- **Persistence**: Saved to `vectors.bin` on shutdown, loaded on startup
- **Background Build**: If not ready, builds asynchronously from SQLite embeddings
- **Platform Support**: Unix (full) and Windows (stub with fallback)

### SQLite Vector Store (Fallback)
- **Method**: Brute-force cosine similarity over all embeddings
- **Complexity**: O(n) where n = number of memories
- **Use Case**: Exact search, filtered queries, or when HNSW is unavailable

### Fallback Vector Store (Wrapper)
- Transparently delegates `Search()` to HNSW
- If HNSW returns "not ready", automatically falls back to SQLite vector store
- Ensures recall always works even during index warm-up

---

## Configuration Model

The `Config` struct is organized into logical sections:

| Section       | Purpose                                      |
|---------------|----------------------------------------------|
| `system`      | Version string                               |
| `storage`     | DB path, SQLite PRAGMAs                      |
| `embeddings`  | Model, dimension, cache size                 |
| `allocator`   | CBA defaults: budget, session window, sigmoid|
| `hnsw`        | HNSW graph parameters (M, Ml, ef_*)          |
| `metrics`     | Prometheus enablement and address            |
| `webhooks`    | HTTP callback configuration                  |
| `recall`      | FTS5, RRF, expansion, clustering, reranker |
| `decay_rates` | Per-type exponential decay constants         |
| `archive_thresholds` | Auto-archive days per type           |
| `overlap_cache` | TTL and max entries for pairwise cache     |
| `extraction`  | NLP parameters (entity length, causal lookback) |
| `mcp`         | Server name, version, transport, timeout     |

### Example: New `recall` Section

```yaml
recall:
  adaptive_threshold_method: "iqr"   # iqr | elbow | mean_stddev
  adaptive_threshold_floor: 0.15
  adaptive_threshold_ceiling: 0.75
  enable_fts5: true
  fts5_limit: 100
  rrf_k: 60
  query_expansion:
    enabled: true
    num_variants: 3
    temperature: 0.3
  search_time_clustering:
    enabled: true
    similarity_threshold: 0.88
  reranker:
    enabled: false
    top_k: 30
```

---

*For practical usage examples, see [API_REFERENCES.md](API_REFERENCES.md).*  
*For a feature-by-feature catalog, see [FEATURES.md](FEATURES.md).*

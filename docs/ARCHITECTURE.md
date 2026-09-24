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

## Autonomous agent bridge

Agent installation is a thin outer adapter around the existing MIRA use cases:

```text
agent install
  ├─ client adapter → MCP registration + managed instructions
  ├─ manifest       → .mira/agent.yaml policy and stable wing
  └─ native hook    → short-lived event bridge
                         ├─ redact secrets
                         ├─ filter transient content
                         ├─ bounded recall (reference-only)
                         └─ StoreMemory history / soul evidence
```

The bridge does not create a second memory pipeline. It calls the same
`RecallMemory` and `StoreMemory` use cases used by MCP and REST. Hook failures
are fail-open, diagnostics never include captured content, and clients without
native interception are marked instruction-guided by `mira agent doctor`.

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
│   │  • storage: SQLiteRepository / PostgresRepository             │   │
│   │  • vector: HNSWStore, BruteForceVectorStore, fallback        │   │
│   │  • extraction: NativeExtractor, CybertronEmbedder           │   │
│   │  • webhook, metrics, logging                                │   │
│   │  • mcp: MCP controller (stdio / SSE / HTTP)                 │   │
│   │  • rest: REST HTTP API (optional, :8080)                    │   │
│   │  ✓ Implements ports                                         │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  FRAMEWORKS & DRIVERS                                       │   │
│   │  • SQLite3, HNSW lib, Cybertron, MCP Server, net/http       │   │
│   │  ✓ External technical details                               │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Project Structure

```
mira/
├── cmd/mira/                    # Entry point (cobra CLI)
│   └── main.go                  # Subcommands: server, migrate, doctor, query, export, import
├── internal/
│   ├── domain/                  # Pure business logic
│   │   ├── entities/            # Verbatim, Fingerprint, Embedding, Candidate, CausalNode
│   │   └── valueobjects/        # MemoryType, RenderMode, RelationType, Stats
│   ├── usecases/                # Application rules
│   │   ├── ports/               # Repository and service interfaces
│   │   └── interactors/         # Use case implementations
│   ├── adapters/                # Infrastructure implementations
│   │   ├── storage/             # SQL repositories, migrations
│   │   ├── vector/              # HNSW, portable fallback, SQL caches
│   │   ├── extraction/          # NLP, embeddings (Cybertron, Simple)
│   │   ├── logging/             # Structured logging
│   │   ├── metrics/             # Prometheus and simple collectors
│   │   └── webhook/             # HTTP notifications
│   ├── interfaces/              # External protocol adapters
│   │   ├── mcp/                 # MCP controller (stdio / SSE / HTTP)
│   │   └── rest/                # Optional REST HTTP API (:8080)
│   │       ├── handler.go       # 13 route handlers (Go 1.22+ method+path ServeMux)
│   │       ├── middleware.go    # recovery, structured logging, Bearer auth
│   │       └── openapi.go       # OpenAPI 3.1 spec (Go structs → JSON, served at /openapi.json)
│   ├── config/                  # Configuration loading and validation
│   └── app/                     # Composition root (DI)
│       ├── main.go              # Dependency injection + REST server wiring
│       ├── health.go            # Health checks
│       └── main_test.go         # Application tests
├── docs/                        # Documentation
├── scripts/                     # Benchmarks and utilities
├── config.example.yaml          # Example configuration
└── README.md                    # Project overview
```

---

## T0/T1/T2 Memory Hierarchy

### Autonomous coherence layer

The SQL records remain authoritative while derived indexes stay repairable. Soul capture filters provenance before extraction and stores bounded `TraitEvidence` beside each immutable snapshot. Retention compaction marks intermediate snapshots instead of deleting lineage. Consolidation marks source memories `superseded`, keeps `consolidated_from`, and supports revocation.

Causal edges carry `confidence`, `status`, `evidence`, and `detector`; generic subjects such as `Note` are not sufficient for confirmation, and the configured lookback/age windows are applied. CBA multiplies relevance by calibrated extraction confidence and validation freshness, excludes non-active lifecycle rows, and keeps causal neighbors available for explanation. A local `BeliefRegistry` resolves active subject/predicate/value assertions and bounds feedback calibration to avoid autonomous score drift.

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
S(m) = ρ × δ × η × q × β × (1-σ) × τ × χ × υ × 𝟙[ρ>θ]

where:
  ρ = Semantic Relevance     cos(embedding_m, embedding_q)
  δ = Information Density    sigmoid(facts / √tokens)
  η = Temporal Weight        exp(-λ × age_days)
  q = Quality Envelope        extraction confidence × validation freshness × active lifecycle
  β = Belief Calibration      bounded local calibration in [0.75, 1.2]
  σ = Max Overlap            sim(m, already_selected)
  τ = Session Boost          1.2 if within 2h window
  χ = Causal Relation Factor preserves and weights reliable causal neighbors
  υ = Diversity Modifier     1 + α × (new_subjects / total_subjects), optional
  θ = Adaptive Threshold     dynamic relevance floor
```

The eight core signals are ρ, δ, η, q, β, σ, τ and χ. `υ` is a separate
selection modifier, while the threshold is a gate. The implementation applies
quality and belief calibration before greedy re-normalisation, and skips
non-active lifecycle rows.

### Algorithm — O(n²) greedy selection

> **Note on complexity**: The current implementation uses a max-heap with lazy
> re-scoring. Candidate overlap still compares against the selected set, so the
> greedy allocator remains O(n²) in the practical worst case while heap operations
> keep the common path efficient.

```
1. EMBEDDING
   e_q ← Embed(q) with LRU cache (1000 entries)
   If query expansion enabled:
      variants ← expandQuery(q)
      e_q ← average(embed(variants))

2. VECTOR SEARCH
   dense ← HNSW_Search(e_q, N=100, wing, room)        # O(log n)
   lexical ← SQL lexical search(q, N=100, wing, room) # FTS5 or PostgreSQL GIN
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
      Recalculate overlap, causal relation, session and diversity modifiers
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

### SQL Schema

MIRA supports SQLite (local deployment) and PostgreSQL (shared deployment). Both
are authoritative stores for T0/T1/T2 data; the vector index and session/overlap
caches are derived data.

**Main Tables:**
- `verbatim` — T0 content, wing, room, tokens, timestamp
- `fingerprints` — T1 structured data, fact count, token estimate
- `embeddings` — T2 binary vectors (LittleEndian float32)
- `causal_nodes` / `causal_edges` — Graph structure
- `models` — Registered embedding model hashes
- `webhook_endpoints` / `webhook_dlq` — Webhook configuration and dead letter queue

**Additional Schema Elements:**
- `memory_tags` — Extracted tags (entity, subject, keyword) with FK to verbatim
- `verbatim_fts` — SQLite FTS5 virtual table with auto-triggers
- PostgreSQL uses `to_tsvector('simple', content)` with a GIN index
- `session_memory_cache` — persistent session-selected memory IDs and expiry

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
- **Background Build**: If not ready, builds asynchronously from authoritative SQL embeddings
- **Platform Support**: Unix (full) and Windows (stub with fallback)

### Portable Brute-Force Fallback
- **Method**: Brute-force cosine similarity over all embeddings
- **Complexity**: O(n) where n = number of memories
- **Implementation**: `BruteForceVectorStore` reads through the repository port
- **Use Case**: Exact search, filtered queries, or when HNSW is unavailable

### Fallback Vector Store (Wrapper)
- Transparently delegates `Search()` to HNSW
- If HNSW returns "not ready", automatically falls back to the portable brute-force store
- Ensures recall always works even during index warm-up
- Health checks compare HNSW with the authoritative embedding count and report both missing and orphaned index entries

### Consistency Rules
- SQL storage is authoritative; HNSW, overlap cache, and session cache are derived data.
- Consolidation writes a synthesized fact in one SQL transaction, marks source notes `superseded`, and keeps their provenance. Revoking the synthesis restores source lifecycle state; if incremental index work fails, MIRA rebuilds derived indexes from SQL.
- Identity snapshots are immutable, versioned records that share MIRA's configured SQL backend and bounded recall budget.

### Autonomous coherence pipeline

```text
capture → authoritative T0/T1/T2 transaction
        → lifecycle + tags + causal + belief projections
        → dense/lexical retrieval with temporal/status filters
        → quality-aware CBA and bounded render
```

Lifecycle state is stored explicitly on the verbatim record and mirrored in
metadata for compatibility. Vector, tag, causal and belief projections only
serve active records during recall; they are repairable read models, never a
second source of truth. A structured decision or preference can produce a
deterministic belief keyed by its source, with `valid_from`/`valid_until`,
confidence and source provenance. Feedback is counted locally and changes the
source calibration only within `0.75..1.20`.

Soul recall has a deliberate budget partition: 60% is reserved for identity
invariants and 40% for MIRA evidence. Evidence is truncated before provider
rendering, while milestone history is bounded by
`agent_memory.evolution.max_milestone_versions` (default: 8).

### Compatibility and migrations

The lifecycle authority is additive: migration `017_lifecycle_authority` adds
explicit columns and indexes while backfilling the metadata already written by
older MIRA versions. Migration `016_beliefs_quality_feedback` remains the
storage contract for local beliefs and feedback. Existing MCP, REST and CLI
operations keep their shapes; the new lifecycle, belief and calibration rules
are automatic projections behind those boundaries.

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
| `recall`      | FTS5, RRF, expansion, clustering, reranker   |
| `decay_rates` | Per-type exponential decay constants         |
| `archive_thresholds` | Auto-archive days per type           |
| `overlap_cache` | TTL and max entries for pairwise cache     |
| `extraction`  | NLP parameters (entity length, causal lookback) |
| `mcp`         | Server name, version, transport, address, timeout |
| `api`         | REST HTTP API: enabled, address, auth_token, timeouts |
| `agent_memory` | Built-in identity and continuity settings (internal configuration key) |

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

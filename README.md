# MIRA - Memory with Information-theoretic Relevance Allocation

**Version:** 0.1.2 | **Language:** Go 1.21+ | **License:** MIT

Long-term memory system for LLMs with optimal context budget allocation, approximation guarantees, and temporal coherence. 100% local, deterministic, O(n log n).

![MIRA Logo](./logo.png)

---

## Table of Contents

1. [Installation](#installation)
2. [System Architecture](#system-architecture)
3. [Data Model](#data-model)
4. [Scoring Mathematics](#scoring-mathematics)
5. [Extraction Pipeline](#extraction-pipeline)
6. [Allocation Algorithm](#allocation-algorithm)
7. [Causal Graph](#causal-graph)
8. [Performance & Complexity](#performance--complexity)
9. [Configuration](#configuration)
10. [MCP API](#mcp-api)
11. [Development](#development)

---

## Installation

### From Source (Go 1.21+)

```bash
go install github.com/benoitpetit/mira/cmd/mira@latest
```

### From Binary Releases

Download pre-built binaries from the [Releases page](https://github.com/benoitpetit/mira/releases):

```bash
# Linux/macOS
tar -xzf mira-linux-amd64.tar.gz
sudo mv mira /usr/local/bin/
mira --version

# Windows
unzip mira-windows-amd64.zip
.\mira.exe --version
```

### Quick Start

```bash
# 1. Copy and edit configuration
cp config.example.yaml config.yaml
# Edit config.yaml to match your environment

# 2. Run the MCP server
mira
```

---

## System Architecture

### Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              MCP Server                                 │
│  ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌──────────────────────────┐   │
│  │mira_store│ │mira_recall│ │mira_load │ │    mira_causal_chain     │   │
│  └────┬─────┘ └────┬──────┘ └────┬─────┘ └───────────┬──────────────┘   │
│       └────────────┴─────────┬───┴───────────────────┘                  │
│                              │                                          │
└──────────────────────────────┼──────────────────────────────────────────┘
                               │
                    ┌──────────┴──────────┐
                    │  Budget Allocator   │
                    │   (CBA Algorithm)   │
                    │   O(n log n)        │
                    └──────────┬──────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
   ┌────┴────┐          ┌──────┴─────┐        ┌───────┴───┐
   │ Extract │          │   Store    │        │  Causal   │
   │ Pipeline│          │  (SQLite)  │        │   Graph   │
   │T0→T1,T2 │          │            │        │   (BFS)   │
   └────┬────┘          └────────────┘        └───────────┘
        │
   ┌────┴────────────────────────┐
   │  NLP Stack                  │
   │  • tiktoken (tokenization)  │
   │  • prose (NER/entities)     │
   │  • cybertron (embeddings)   │
   └─────────────────────────────┘
```

### Data Flow

```
┌─────────┐  Extraction   ┌─────────────┐  Storage    ┌──────────┐
│  Input  │ ─────────────→│  Triple T   │ ───────────→│  SQLite  │
│  Text   │   T0→T1→T2    │  (3 levels) │  WAL Mode   │  + WAL   │
└─────────┘               └─────────────┘             └──────────┘
                                   │                         │
                                   │ Query                   │
                                   ↓                         ↓
┌─────────┐  Scoring       ┌─────────────┐  Allocation ┌──────────┐
│  Query  │ ←───────────── │  CBA Score  │ ←───────────│  Budget  │
│  Vector │   ρ×δ×η×τ×σ×χ  │  Composite  │  Greedy     │  4000tk  │
└─────────┘                └─────────────┘             └──────────┘
```

---

## Data Model

### Representation Space

Each memory `m ∈ ℳ` is a tuple:

```
m = (id, t₀, t₁, t₂, c, τ, ω, γ, δ, ν)
```

| Field | Type                  | Description                         |
| ----- | --------------------- | ----------------------------------- |
| `id`  | UUIDv4 (128 bits)     | Unique identifier                   |
| `t₀`  | Σ\* (UTF-8, max 64KB) | Verbatim - original text            |
| `t₁`  | Canonical JSON        | Structured fingerprint              |
| `t₂`  | ℝ³⁸⁴                  | Embedding vector (all-MiniLM-L6-v2) |
| `c`   | ℕ                     | Token count (tiktoken cl100k_base)  |
| `τ`   | ℝ⁺                    | UNIX timestamp (seconds)            |
| `ω`   | Ω                     | Memory type (enum)                  |
| `γ`   | Γ                     | Causal graph (node + edges)         |
| `δ`   | ℝ⁺                    | Decay rate λ_ω by type              |
| `ν`   | {0,1}³²               | Embedding model hash                |

### Memory Types and Decay Rates

| Type ω         | λ_ω (day⁻¹) | Half-life | Auto Archive | Usage                   |
| -------------- | ----------- | --------- | ------------ | ----------------------- |
| `decision`     | 0.001       | ~693 days | No           | Architectural decisions |
| `fact`         | 0.005       | ~139 days | No           | Facts, knowledge        |
| `preference`   | 0.01        | ~69 days  | No           | User preferences        |
| `session_note` | 0.1         | ~7 days   | 30 days      | Session notes           |
| `debug_log`    | 0.5         | ~1.4 days | 7 days       | Debug logs              |

### SQL Schema (SQLite)

```sql
-- Embedding model metadata (versioning)
CREATE TABLE embedding_models (
    model_hash TEXT PRIMARY KEY,  -- SHA256 truncated
    model_name TEXT NOT NULL,     -- "all-MiniLM-L6-v2"
    dimension INTEGER NOT NULL,
    created_at REAL NOT NULL,
    metadata BLOB                 -- JSON config
);

-- T0: Verbatim Store (append-only, WAL mode)
CREATE TABLE verbatim (
    id BLOB PRIMARY KEY,          -- 16 bytes UUID
    content TEXT NOT NULL,        -- UTF-8, max 64KB
    token_count INTEGER NOT NULL, -- tiktoken cl100k_base
    created_at REAL NOT NULL,     -- UNIX timestamp
    wing TEXT NOT NULL,           -- namespace/project
    room TEXT,                    -- sub-category
    metadata BLOB                 -- msgpack compressed
);

-- T1: Fingerprint Index (canonical JSON)
CREATE TABLE fingerprints (
    id BLOB PRIMARY KEY,
    verbatim_id BLOB NOT NULL REFERENCES verbatim(id),
    ftype TEXT NOT NULL,          -- decision|fact|preference|...
    extracted_at REAL NOT NULL,
    entities TEXT,                -- JSON array
    subjects TEXT,                -- JSON array
    decision TEXT,
    data BLOB NOT NULL,           -- minified JSON
    fact_count INTEGER DEFAULT 0,
    token_estimate INTEGER DEFAULT 0,
    model_hash TEXT
);

-- T2: Vector Index with versioning
CREATE TABLE embeddings (
    id BLOB PRIMARY KEY,
    model_hash TEXT NOT NULL,
    dim INTEGER NOT NULL,         -- 384
    vector BLOB NOT NULL,         -- 384 * 4 = 1536 bytes
    normalized BOOLEAN DEFAULT 1,
    created_at REAL NOT NULL
);

-- Causal Graph (DAG)
CREATE TABLE causal_nodes (
    id BLOB PRIMARY KEY,
    node_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    timestamp REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT
);

CREATE TABLE causal_edges (
    from_id BLOB NOT NULL,
    to_id BLOB NOT NULL,
    relation TEXT NOT NULL,       -- BECAUSE|TRIGGERED|CONTRADICTS|UPDATES|RESOLVES
    weight REAL DEFAULT 1.0,
    detected_at REAL NOT NULL,
    PRIMARY KEY (from_id, to_id, relation)
);

-- Overlap cache with TTL (30 days)
CREATE TABLE overlap_cache (
    id_a BLOB NOT NULL,
    id_b BLOB NOT NULL,
    similarity REAL NOT NULL,
    computed_at REAL NOT NULL,
    ttl REAL NOT NULL DEFAULT (unixepoch() + 2592000),
    PRIMARY KEY (id_a, id_b)
);
```

---

## Scoring Mathematics

### Composite Score Function

For a query `q` with embedding `e_q ∈ ℝ³⁸⁴`, the score of candidate `m` given already selected set `S`:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│   S(m|q,S,θ) = ρ(m,q) × δ_sig(m) × η(m) × τ_session(m,S)                │
│                × (1 - max_s∈S σ(m,s)) × χ_penalty(m,S)                  │
│                × 𝟙[ρ > θ_min]                                           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1. Semantic Relevance ρ

```
              e_m · e_q
ρ(m,q) = ───────────────── ∈ [0, 1]
          ‖e_m‖ ‖e_q‖

With L2 pre-normalization:
ρ(m,q) = (1 + cosim(e_m, e_q)) / 2
```

**Early pruning:** Only m with `ρ > θ_min = 0.6` are considered.

### 2. Informational Density δ_sig

```
               |facts(t₁(m))|
δ_raw(m) = ─────────────────
               √c(m)

δ_sig(m) = 2/(1 + e^(-2(δ_raw - 0.3))) - 1 ∈ [0, 1]
```

Sigmoid parameters:

- `μ = 0.3`: center (5 facts/100 tokens = neutral)
- `k = 2.0`: slope

**Goal:** Avoid over-valuing micro-memories.

### 3. Temporal Weight (Recency) η

```
η(m) = exp(-λ_ω · (t_now - τ(m)))

Where:
- λ_ω: decay rate by type
- t_now - τ(m): age in days
```

### 4. Temporal Coherence Boost τ_session

```
τ_session(m,S) = 1 + β · 𝟙[∃s ∈ S : |τ(m) - τ(s)| < θ_session]

Parameters:
- β = 0.2 (20% boost)
- θ_session = 7200s (2 hours)
```

**Goal:** Favor narrative coherence within a session.

### 5. Overlap Penalty σ

```
              e_m · e_s
σ(m,s) = ───────────────── ∈ [-1, 1]
          ‖e_m‖ ‖e_s‖

Applied penalty: (1 - max_s∈S σ(m,s))
```

### 6. Causal Penalty χ_penalty

```
χ_penalty(m,S) = exp(-α · |{s ∈ S : causal_link(s,m)}|)

Parameter:
- α = 0.15

Goal: Avoid over-selection of long causal chains.
```

---

## Extraction Pipeline

### T0 → T1: Structured Extraction

```go
// UTF-8 aware regex patterns
var decisionPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:decided|chose|selected|opted|recommended)\s+(?:to\s+)?(?:use|adopt|migrate to|take)...`),
    regexp.MustCompile(`(?i)(?:decision|choice)\s*:\s*(.+?)(?:\.|\n|$)`),
    regexp.MustCompile(`(?i)(?:we|I)\s+(?:will|shall)\s+(?:use|take|adopt)...`),
}

// NER extraction with prose
entities := extractEntities(doc)  // PERSON, ORG, GPE

// Type detection with strict priority
if matchDecision(content) → TypeDecision
else if matchPreference(content) → TypePreference
else if matchFact(content) → TypeFact
else → TypeSessionNote
```

### T1 Structure (Fingerprint)

```json
{
  "id": "uuid",
  "type": "decision|fact|preference|session_note|debug_log",
  "date": "2026-04-08T09:18:01Z",
  "entities": ["PostgreSQL", "API", "Auth"],
  "subject": ["database-migration"],
  "decision": "Use PostgreSQL",
  "rejected": ["MySQL", "MongoDB"],
  "reason": ["Better ACID", "Team expertise"],
  "validated_by": "CTO",
  "assignee": "John",
  "deadline": "Sprint 5",
  "causal_parent": null,
  "verbatim_ref": "T0:uuid"
}
```

### T0 → T2: Embedding Generation

```python
# Model: sentence-transformers/all-MiniLM-L6-v2
Dimension: 384
Normalization: L2 pre-normalization

vector = model.encode(text)  # ℝ³⁸⁴
vector = normalize_L2(vector)
```

---

## Allocation Algorithm

### Context Budget Allocator (CBA) v2

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CBA Algorithm                                   │
│                         O(n log n)                                      │
├─────────────────────────────────────────────────────────────────────────┤
│  Input: query q, budget B, wing w, room r                               │
│  Output: list of selected memories with render mode                     │
├─────────────────────────────────────────────────────────────────────────┤
│  1. Embedding with LRU cache                                            │
│     e_q ← Embed(q)                                                      │
│                                                                         │
│  2. Vector search                                                       │
│     C ← VectorSearch(e_q, N=100, w, r)                                  │
│                                                                         │
│  3. Early Pruning                                                       │
│     C' ← {c ∈ C : ρ(c,q) > 0.6}                                         │
│     if C' = ∅: C' ← top-5(C)                                           │
│                                                                         │
│  4. Initial scoring                                                     │
│     ∀c ∈ C':                                                            │
│       c.score ← ρ(c) × δ_sig(c) × η(c)                                  │
│                                                                         │
│  5. Greedy selection with dynamic renormalization                       │
│     S ← ∅, tokens_used ← 0                                             │
│     PQ ← MaxHeap(C')  # by initial score                                │
│                                                                         │
│     while PQ ≠ ∅ AND tokens_used < B:                                  │
│       c ← Pop(PQ)                                                       │
│                                                                         │
│       # Dynamic recalculation                                           │
│       c.max_overlap ← max_{s∈S} σ(c,s)                                  │
│       c.causal_penalty ← exp(-0.15 × |links(c,S)|)                      │
│       c.session_boost ← 1.2 if |τ(c)-τ(S)| < 2h else 1.0                │
│                                                                         │
│       adjusted_score ← c.score × (1-c.max_overlap)                      │
│                           × c.causal_penalty                            │
│                           × c.session_boost                             │
│                                                                         │
│       # Check if next has better score                                  │
│       if PQ[0].score × 0.8 > adjusted_score:                            │
│          Push(PQ, c) with adjusted_score                                │
│          continue                                                       │
│                                                                         │
│       # Determine render mode based on REMAINING budget                 │
│       remaining ← B - tokens_used                                       │
│       mode ← DetermineRenderMode(c, remaining)                          │
│       cost ← CalculateTokenCost(c, mode)                                │
│                                                                         │
│       # Downgrade if necessary                                          │
│       if tokens_used + cost > B:                                        │
│          mode ← downgrade(mode)                                         │
│          cost ← recalculate(mode)                                       │
│          if tokens_used + cost > B: continue                            │
│                                                                         │
│       S ← S ∪ {c}, tokens_used ← tokens_used + cost                     │
│                                                                         │
│  6. Return S                                                            │
└─────────────────────────────────────────────────────────────────────────┘
```

### Render Modes

Mode depends only on **remaining budget**, not overlap:

| Remaining Budget | Mode        | Tokens | Content            |
| ---------------- | ----------- | ------ | ------------------ | ---- | -------------- |
| < 100            | Header      | 2-5    | `[type             | date | wing] → T0:id` |
| < 1000           | Fingerprint | ~15%   | Essential facts T1 |
| ≥ 1000           | Verbatim    | 100%   | Original text T0   |

---

## Causal Graph

### Supported Relations

| Relation      | Direction | Semantics            |
| ------------- | --------- | -------------------- |
| `BECAUSE`     | A ← B     | B explains A         |
| `TRIGGERED`   | A ← B     | B triggered A        |
| `CONTRADICTS` | A ↔ B     | A and B contradict   |
| `UPDATES`     | A ← B     | B replaces/updates A |
| `RESOLVES`    | A ← B     | B resolves issue A   |

### Causal Relation Detection

```
Input: new_fp, recent_fps[50], verbatim_content
Output: list of causal edges

for each existing in recent_fps:
    if time_diff > 30 days: continue

    for each (relation, pattern) in causal_patterns:
        if pattern.match(verbatim_content):
            # Check implicit reference
            if hasOverlap(new_fp.entities, existing.entities) OR
               hasOverlap(new_fp.subjects, existing.subjects) OR
               content.contains(existing.id[:8]):

               edge ← CausalEdge{
                   from: existing.id,
                   to: new_fp.id,
                   relation: relation,
                   weight: 0.7
               }
               edges.add(edge)
```

### BFS Navigation

```go
// GetChain: trace back causes (parents)
func (g *Graph) GetChain(nodeID UUID, maxDepth int) []*CausalNode

// GetConsequences: follow effects (children)
func (g *Graph) GetConsequences(nodeID UUID, maxDepth int) []*CausalNode
```

---

## Performance & Complexity

### Algorithmic Complexities

| Operation          | Complexity | Notes                               |
| ------------------ | ---------- | ----------------------------------- |
| Storage (T0→T1,T2) | O(1)       | Amortized, single insertion         |
| Vector search      | O(n)       | SQLite linear scan (HNSW: O(log n)) |
| Scoring            | O(n)       | n = number of candidates            |
| Greedy Allocation  | O(n log n) | Max-heap operations                 |
| Causal Graph BFS   | O(V + E)   | V=nodes, E=edges                    |
| Total Recall       | O(n log n) | Bottleneck: heap operations         |

### Performance Constants

| Parameter         | Value        | Justification            |
| ----------------- | ------------ | ------------------------ |
| `MaxCandidates`   | 100          | Early pruning included   |
| `EmbeddingCache`  | 1000 entries | LRU for query embeddings |
| `CausalLookback`  | 50 latest FP | Time window: 30 days     |
| `OverlapCacheTTL` | 30 days      | Avoid O(n²) explosion    |
| `SessionWindow`   | 2 hours      | Conversational coherence |

### Benchmarks (estimated)

```
BenchmarkCosineSimilarity-384      50M ops/sec
BenchmarkNormalizeL2-384           20M ops/sec
BenchmarkAllocateWithCache-1000    ~5ms/query
BenchmarkAllocateNoCache-1000      ~50ms/query
```

---

## Configuration

### Configuration File

Copy the example configuration file:

```bash
cp config.example.yaml config.yaml
```

Then edit `config.yaml` to match your environment.

```yaml
system:
  version: "0.1.2"
  max_concurrent_queries: 10

storage:
  path: "./mira_data"
  sqlite:
    journal_mode: WAL # Write-Ahead Logging
    synchronous: NORMAL # Balance perf/safety
    cache_size: -64000 # 64MB
    mmap_size: 268435456 # 256MB
    temp_store: MEMORY

embeddings:
  current_model: "sentence-transformers/all-MiniLM-L6-v2"
  model_hash: "a2d8f3e9" # SHA256 truncated
  dimension: 384
  batch_size: 32
  cache_size: 1000 # LRU cache

allocator:
  default_budget: 4000 # tokens
  max_candidates: 100
  early_pruning_threshold: 0.6 # ρ_min
  session_window_seconds: 7200 # 2h
  session_boost_beta: 0.2 # 20%
  causal_penalty_alpha: 0.15
  density_sigmoid:
    k: 2.0
    mu: 0.3

decay_rates:
  decision: 0.001 # ~2 years half-life
  fact: 0.005 # ~5 months
  preference: 0.01 # ~2 months
  session_note: 0.1 # ~1 week
  debug_log: 0.5 # ~1.4 day

archive_thresholds:
  session_note: 30 # days
  debug_log: 7 # days

extraction:
  max_verbatim_size: 65536 # 64KB
  max_sentence_length: 500
  min_entity_length: 2
  causal_lookback: 50
  causal_max_days: 30
```

---

## MCP API

### Available Tools

#### `mira_store`

Store a memory with automatic T0→T1,T2 extraction.

```json
{
  "content": "We decided to use PostgreSQL for the database",
  "wing": "backend-service",
  "room": "database-migration"
}
```

**Response:**

```
Stored: 550e8400-e29b-41d4-a716-446655440000
Type: decision
Facts: 3
Tokens: 42
Model: a2d8f3e9
```

#### `mira_recall`

Retrieve optimal context with budget.

```json
{
  "query": "Which database should we use?",
  "budget": 4000,
  "wing": "backend-service"
}
```

**Response:**

```
=== MIRA CONTEXT ===
Query: Which database should we use? | Budget: 4000
Wing: backend-service

--- [1] VERBATIM (42 tokens) ---
We decided to use PostgreSQL for the database

=== Total: 42/4000 tokens (1.1%) ===
```

#### `mira_causal_chain`

Trace back causal chain.

```json
{
  "id": "550e8400...",
  "max_depth": 5,
  "include_consequences": true
}
```

**Response:**

```
=== CAUSAL CHAIN (Upstream) ===
 → [decision] Evaluate DB options (2026-04-01)
  → [fact] Benchmark PostgreSQL vs MySQL (2026-03-28)

=== CONSEQUENCES (Downstream) ===
→ [decision] Configure connection pool (2026-04-09)
```

---

## Development

### Code Structure

```
mira/
├── cmd/mira/           # Entry point
├── types/              # Domain models
├── store/              # SQLite persistence
├── extract/            # T0→T1,T2 pipeline
├── budget/             # CBA algorithm
├── causal/             # Graph operations
├── mcp/                # MCP server
├── config/             # Configuration
└── vector/             # Vector search adapter
```

### Make Commands

```bash
make build      # Compile
make test       # Unit tests
make bench      # Benchmarks
make migrate    # DB migrations
make clean      # Clean
```

### Tests

```bash
# Unit tests
go test -v ./...

# With race detector
go test -race ./...

# Benchmarks
go test -bench=. -benchmem ./budget
```

---

## References

### Key Libraries

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - OpenAI tokenization
- [prose](https://github.com/jdkato/prose) - NLP/NER in Go
- [cybertron](https://github.com/nlpodyssey/cybertron) - Transformer embeddings
- [mcp-go](https://github.com/mark3labs/mcp-go) - MCP Protocol

### Embedding Model

- **Model:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Size:** ~80MB
- **Performance:** ~1000 texts/sec on CPU

---

## Changelog

### v0.1.2 (2026-04-09)

- 🚀 New version 0.1.2

### v0.1.0 (2026-04-08)

- ✅ Initial release
- ✅ T0/T1/T2 memory architecture
- ✅ CBA algorithm (Context Budget Allocator)
- ✅ SQLite storage with WAL mode
- ✅ MCP server with 7 tools
- ✅ Causal graph with 5 relation types
- ✅ UTF-8 extraction with entity recognition
- ✅ Sigmoid density scoring
- ✅ Session boost and causal penalty
- ✅ Greedy allocation with dynamic renormalization
- ✅ Embedding model versioning

---

**MIRA** - _Memory with Information-theoretic Relevance Allocation_

_"Memory is the sap of artificial intelligence."_

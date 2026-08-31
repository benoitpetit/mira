<div align="center">
  <img src="./logo.png" alt="MIRA Logo" width="800">

  # MIRA
  ### Memory with Information-theoretic Relevance Allocation

  **Long-term Memory System for LLMs with Optimal Context Budget Allocation**

  [![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
  [![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-0.4.7-blue?style=flat-square)]()
  [![Tests](https://img.shields.io/badge/Tests-~70%25-yellow?style=flat-square)]()

  *100% Local • Deterministic (embedding variance < 1e-6) • Clean Architecture*

  [API Reference](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md) • [Skill](SKILL.md) • [Français](README_FR.md) • [SOUL Extension](https://github.com/benoitpetit/soul)

</div>

---

## Table of Contents

- [What is MIRA?](#what-is-mira)
- [How It Works](#how-it-works)
- [3-Level Architecture (T0/T1/T2)](#3-level-architecture-t0t1t2)
- [The CBA Algorithm](#the-cba-algorithm)
- [Enhanced Recall Pipeline](#enhanced-recall-pipeline)
- [Causal Graph](#causal-graph)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [MCP API](#mcp-api)
- [REST API](#rest-api)
- [Performance](#performance)
- [Technical Architecture](#technical-architecture)
- [Development](#development)
- [Changelog](#changelog)

---

## What is MIRA?

**MIRA** is a long-term memory system designed for **Large Language Models**. Instead of simple similarity retrieval, MIRA solves an optimization problem: maximize useful information within a fixed token budget.

Each memory is stored in three forms — full text (T0), structured facts (T1), and a 384-dimensional embedding (T2) — enabling adaptive rendering that adjusts to the available budget.

**Traditional approaches fall short:**

- Simple RAG retrieves by similarity and ignores information density
- Sliding window loses critical information from the beginning
- Static summarization does not adapt to the current query
- Basic vector DB has O(n) complexity with no budget management

**MIRA provides:**

- **Context Budget Allocation (CBA)** — maximizes information across 6 scoring dimensions
- **Triple representation (T0/T1/T2)** — adaptive rendering from full text down to a 5-token header
- **Hybrid search** — HNSW O(log n) + SQLite FTS5, fused with Reciprocal Rank Fusion
- **Causal graph** — automatic detection of cause-effect relationships between memories
- **Clean architecture** — hexagonal, fully tested, extensible

> **Need identity persistence?** The optional [SOUL](https://github.com/benoitpetit/soul) extension adds 8 MCP tools for capturing and recalling an agent's personality across model changes — activated with a single `--with-soul` flag.

---

## How It Works

### Storage

```
Text input  →  T1/T2 extraction  →  SQLite (T0 + T1) + HNSW index (T2)
```

When a memory is stored, the native extractor produces:

- **T1** — a structured JSON fingerprint (~15% of original tokens)
- **T2** — a 384-dimensional embedding for semantic search

Both are derived atomically and stored alongside the original verbatim (T0).

### Recall

```
Query  →  Embed  →  HNSW top-100 (+ FTS5)  →  RRF fusion  →  CBA scoring  →  Greedy selection
```

The CBA algorithm selects memories greedily against a token budget, adjusting each memory's render mode (Verbatim / Fingerprint / Header) based on remaining tokens.

### CBA Composite Score

**S(m) = ρ × δ × η × (1−σ) × τ × χ × 𝟙[ρ>θ]**

| Symbol | Dimension | Formula |
|--------|-----------|---------|
| ρ | Semantic relevance | cos(embedding_m, query) |
| δ | Information density | sigmoid(facts / √tokens) |
| η | Temporal weight | exp(−λ × age) |
| σ | Max overlap | max similarity with already-selected memories |
| τ | Session boost | +20% if within the same 2-hour window |
| χ | Causal penalty | exp(−0.15 × causal links to current selection) |
| 𝟙[ρ>θ] | Threshold gate | discard if ρ < 0.6 |

---

## 3-Level Architecture (T0/T1/T2)

The human brain does not record everything with the same fidelity. MIRA mimics this hierarchy.

### T0 — Verbatim (Episodic Memory)

The complete original text, stored as UTF-8 (max 64 KB). Used when the budget allows full context.

- **Storage:** full UTF-8 text
- **Cost:** ~200 tokens

### T1 — Fingerprint (Semantic Memory)

A structured canonical JSON with extracted facts, entities, decisions, and relationships.

```json
{
  "type": "decision",
  "decision": "PostgreSQL Migration",
  "rejected": ["MySQL", "MongoDB"],
  "reason": ["ACID robustness", "team expertise"],
  "assignee": "Jean",
  "deadline": "Sprint 5",
  "validated_by": "Sophie (PO)"
}
```

- **Storage:** canonical JSON
- **Cost:** ~30 tokens (15% of T0)

### T2 — Embedding (Search Index)

A 384-dimensional float32 vector used exclusively for HNSW similarity search. Never rendered into the context.

- **Storage:** float32[384]
- **Cost:** 0 tokens (search only)

### Memory Types and Decay

| Type | λ (day⁻¹) | Half-life | Auto-archive | Usage |
|------|-----------|-----------|--------------|-------|
| `decision` | 0.001 | ~693 days | No | Architectural decisions |
| `fact` | 0.005 | ~139 days | No | Knowledge, facts |
| `preference` | 0.01 | ~69 days | No | User preferences |
| `session_note` | 0.1 | ~7 days | 30 days | Session notes |
| `debug_log` | 0.5 | ~1.4 days | 7 days | Debug logs |

---

## The CBA Algorithm

### Algorithm (O(n²))

```
INPUT:  Query q, Budget B (tokens), Wing w, Room r
OUTPUT: List of memories with render mode

1. EMBEDDING
   e_q ← Embed(q)  — with LRU cache (1000 entries)

2. VECTOR SEARCH
   C ← HNSW_Search(e_q, N=100, w, r)          // O(log n)
   If HNSW not ready: C ← SQLite_Search(...)    // Fallback

3. EARLY PRUNING
   C' ← { c ∈ C : ρ(c,q) > 0.6 }
   If C' = ∅: C' ← top-5(C) by ρ

4. INITIAL SCORING
   For each c ∈ C':
      c.score ← ρ(c) × δ_sigmoid(c) × η_recency(c)

5. GREEDY SELECTION with dynamic renormalization
   S ← ∅, used ← 0
   PQ ← MaxHeap(C')

   While PQ ≠ ∅ and used < B:
      c ← Pop(PQ)
      c.σ ← max_{s∈S} sim(c, s)
      c.χ ← exp(−0.15 × |causal_links(c, S)|)
      c.τ ← 1.2 if |time(c) − time(S)| < 2h else 1.0
      adjusted ← c.score × (1−c.σ) × c.χ × c.τ

      If PQ[0].score × 0.8 > adjusted:
         Push(PQ, c) with adjusted score; continue

      mode ← ChooseMode(c, B − used)
      cost ← Cost(c, mode)
      If used + cost > B: Downgrade(mode); Recalculate; skip if still over

      S ← S ∪ {c}, used ← used + cost

6. RETURN S sorted by descending score
```

### Adaptive Render Modes

| Remaining budget | Mode | Tokens | Content |
|-----------------|------|--------|---------|
| < 100 | Header | 2–5 | `[type\|date\|wing]` |
| < 1000 | Fingerprint | ~15% | Essential T1 facts |
| ≥ 1000 | Verbatim | 100% | Full T0 text |

---

## Enhanced Recall Pipeline

```
Query → Expansion → Dense (HNSW) + Lexical (FTS5) → RRF Fusion → Clustering → Tag Boost → Adaptive Threshold → CBA Greedy Selection
```

### 1. Query Expansion

MIRA generates semantically close variants of the query (cleaned, without stopwords, top keywords) and **averages their embeddings**. This improves cross-lingual retrieval and robustness against vocabulary mismatch.

### 2. Hybrid Search (Dense + Lexical)

- **Dense:** HNSW O(log n) vector search
- **Lexical:** SQLite FTS5 full-text search (auto-enabled if available)
- **Fusion:** Reciprocal Rank Fusion (`k=60`) merges both rankings into a single candidate list

### 3. Search-Time Clustering

Candidates are grouped by cosine similarity ≥ 0.88. Near-duplicates collapse to their best representative, preventing budget waste on redundant memories.

### 4. Tag-Based Retrieval

The `memory_tags` table indexes extracted entities, subjects, and keywords. Candidates matching query tags receive a small additive relevance boost.

### 5. Adaptive Threshold

Instead of a fixed 0.6 floor, MIRA supports three dynamic methods:

| Method | Description | Default |
|--------|-------------|---------|
| `iqr` | First quartile of score distribution | Yes |
| `elbow` | Largest derivative drop | |
| `mean_stddev` | mean − stddev | |

Threshold is clamped between 0.15 (floor) and 0.75 (ceiling).

### 6. Heuristic Reranker (Optional)

A lightweight pure-Go reranker blends semantic and lexical signals:

- Jaccard-like token overlap
- Exact phrase presence bonus
- Length balance preference

Blend: `0.7 × semantic + 0.3 × rerank`

### 7. Fallback Vector Store

If HNSW is not ready (e.g., rebuilding from scratch), a transparent fallback wrapper routes searches to the SQLite vector store. Recall never fails.

### 8. Context Compression

Rule-based context compression for `session_note` verbatims. Summarised text is stored alongside the original and surfaced by the recall engine when the token budget is too tight for full verbatim.

- **No LLM required** — deterministic, instant compression
- **Auto-compress** at store time (async, non-fatal) or on-demand via `mira_compress`
- **Configurable** — set `min_tokens` threshold to skip short notes

```yaml
compression:
  auto_compress: false   # Auto-compress at store time
  min_tokens: 100        # Minimum token count to qualify
```

---

## Causal Graph

### Supported Relations

| Relation | Meaning | Triggered by |
|----------|---------|--------------|
| `BECAUSE` | B explains why A | "because", "since", "due to" |
| `TRIGGERED` | B triggered A | "following", "after", "in response to" |
| `CONTRADICTS` | A and B are incompatible | "contradicts", "however" |
| `UPDATES` | B replaces A | "updates", "replaces" |
| `RESOLVES` | B fixes problem A | "resolves", "solves", "fixes" |

### Automatic Detection

```go
causalPatterns := map[RelationType]*regexp.Regexp{
    RelTriggered:   regexp.MustCompile(`(?i)(?:following|after|in response to)`),
    RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to|in reason of)`),
    RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|in contradiction|however)`),
    RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces)`),
    RelResolves:    regexp.MustCompile(`(?i)(?:resolves|solves|fixes)`),
}
```

---

## Installation

### Prerequisites

- Go 1.25+ (if building from source)
- SQLite3 (included)
- ~100 MB disk space for the embedding model

### From Source

```bash
git clone https://github.com/benoitpetit/mira.git
cd mira
go build -o mira ./cmd/mira
./mira --version
```

### Via Go Install

```bash
go install github.com/benoitpetit/mira/cmd/mira@latest
```

### Binary Releases

Download pre-compiled binaries from the [Releases](https://github.com/benoitpetit/mira/releases) page:

```bash
# Linux/macOS
tar -xzf mira-linux-amd64.tar.gz
sudo mv mira /usr/local/bin/
mira --version

# Windows
unzip mira-windows-amd64.zip
.\mira.exe --version
```

---

## Quick Start

### 1. Initialize

```bash
cp config.example.yaml config.yaml
nano config.yaml
```

### 2. Start the MCP Server

```bash
# Stdio mode — for Claude Desktop, Cursor, etc.
./mira server

# With a custom config file
./mira --config ./config.yaml server

# With a custom storage path (also: MIRA_DATA_PATH env var)
./mira --storage-path /data/mira server

# MCP transport modes: stdio (default), sse
./mira server --transport sse --mcp-addr localhost:3001

# Enable the optional REST API
./mira server --with-api --api-addr :8080 --api-token my-secret

# Enable the SOUL identity extension
./mira server --with-soul

# Prometheus metrics (default: :9090)
./mira server --prometheus-addr :9091

# Disable Prometheus metrics
./mira server --no-metrics

# Ollama-backed extraction (requires a running Ollama instance)
./mira server --with-llm --llm-endpoint http://localhost:11434
```

### 3. Utility Commands

```bash
# Run database migrations and exit
./mira migrate

# System health check (human-readable or JSON)
./mira doctor
./mira doctor --json

# System status (for scripting/monitoring)
./mira status
./mira status --json

# One-shot recall from CLI
./mira query --query "Why did we choose PostgreSQL?" --wing backend-team
./mira query -q "API decisions" --json

# Store a single memory from CLI
./mira store --content "PostgreSQL chosen for primary DB" --wing backend-team --type decision

# Delete a memory by UUID
./mira delete 5a159ddf-bc11-46a6-8a0d-f39f25853cb4

# Export memories to JSON
./mira export --wing backend-team --output memories.json

# Import memories from JSON (with optional dry-run)
./mira import --file memories.json
./mira import --file memories.json --dry-run

# Optimize a chat history file to fit a token budget (no LLM calls)
./mira optimize --file history.json --budget 2000
./mira optimize --file history.json --stats-only

# Config validation and inspection
./mira config validate
./mira config show
./mira config show --json
```

### 4. Use MCP Tools

#### Store a Memory

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "We decided to migrate to PostgreSQL for v2. Rejected: MySQL (not ACID), MongoDB (not relational). Reason: ACID and team expertise. Approved by CTO. Assigned to Jean.",
    "wing": "backend-team",
    "room": "database-migration"
  }
}
```

#### Retrieve Context

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "Why did we choose PostgreSQL?",
    "budget": 2000,
    "wing": "backend-team"
  }
}
```

**Response:**

```
=== MIRA CONTEXT ===
Query: Why did we choose PostgreSQL? | Budget: 2000
Wing: backend-team

--- [1] FINGERPRINT (45 tokens) ---
Decision: PostgreSQL Migration
Rejected: MySQL, MongoDB
Reason: ACID, team expertise
Approved by: CTO
Assigned: Jean

--- [2] VERBATIM (120 tokens) ---
We decided to migrate to PostgreSQL for v2...

=== Total: 165/2000 tokens (8.3%) ===
```

#### Causal Chain

```json
{
  "tool": "mira_causal_chain",
  "arguments": {
    "id": "uuid-of-the-decision",
    "max_depth": 3,
    "include_consequences": true
  }
}
```

---

## Configuration

### config.yaml

```yaml
system:
  version: "0.4.7"

storage:
  path: ".mira"
  sqlite:
    journal_mode: WAL
    synchronous: NORMAL
    cache_size: -64000
    mmap_size: 268435456
    temp_store: MEMORY

embeddings:
  current_model: "sentence-transformers/all-MiniLM-L6-v2"
  model_hash: "a2d8f3e9"
  dimension: 384
  batch_size: 32
  cache_size: 1000

hnsw:
  M: 32
  Ml: 0.25
  ef_construction: 0   # inactive — not supported by underlying library
  ef_search: 100

allocator:
  default_budget: 4000
  max_candidates: 100
  early_pruning_threshold: 0.6
  session_window_seconds: 7200
  session_boost_beta: 0.2
  session_boost_max: 1.2
  causal_penalty_alpha: 0.15
  density_sigmoid:
    k: 2.0
    mu: 0.3

decay_rates:
  decision: 0.001
  fact: 0.005
  preference: 0.01
  session_note: 0.1
  debug_log: 0.5

archive_thresholds:
  session_note: 30
  debug_log: 7

overlap_cache:
  ttl_days: 30
  max_entries: 1000000

extraction:
  min_entity_length: 2
  causal_lookback: 50
  causal_max_days: 30

recall:
  adaptive_threshold_method: "iqr"
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

# SOUL identity extension (disabled by default)
soul:
  enabled: false

mcp:
  name: "mira"
  version: "0.4.7"
  transport: "stdio"   # "stdio" for Claude Desktop/Cursor, "sse" for HTTP SSE
  address: "localhost:3001"
  timeout_seconds: 30

# Optional REST HTTP API
api:
  enabled: false
  address: ":8080"
  auth_token: ""
  read_timeout_seconds: 30
  write_timeout_seconds: 30

# Prometheus metrics
metrics:
  enabled: true
  prometheus_addr: ":9090"
  report_interval_seconds: 60

# Webhook notifications
webhooks:
  enabled: false
  workers: 3
  queue_size: 1000
  timeout_seconds: 30
  endpoints: []
```

---

## MCP API

### Available Tools

| Tool | Description |
|------|-------------|
| `mira_store` | Store a memory with T0/T1/T2 extraction |
| `mira_recall` | Retrieve optimal context within a token budget |
| `mira_load` | Load the full verbatim by UUID |
| `mira_causal_chain` | Trace causal chain from a memory |
| `mira_status` | System statistics and health |
| `mira_health` | Quick JSON health check |
| `mira_timeline` | Chronological memory reconstruction |
| `mira_archive` | Archive and clean old memories |
| `mira_clear_memory` | Permanently delete memories (global or room-scoped) |
| `mira_compress` | Run rule-based context compression on session_notes |

### Fallback Wings

When recalling from a specific wing yields no results, `mira_recall` supports comma-separated fallback wings:

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "database migration strategy",
    "budget": 2000,
    "wing": "backend-team",
    "fallback_wings": "platform-team,dba-team"
  }
}
```

### Multilingual Search

`mira_recall` accepts queries in any language thanks to cross-lingual embeddings. When a query in one language searches memories stored in another, MIRA automatically broadens the search with relaxed thresholds.

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "règles de langue français anglais",
    "budget": 2000,
    "wing": "general"
  }
}
```

See [API_REFERENCES.md](docs/API_REFERENCES.md) for the full reference.

### Health Endpoints

When metrics are enabled, MIRA exposes health endpoints:

```bash
curl http://localhost:9090/health        # Full check (DB, Vector Store, Embedder)
curl http://localhost:9090/health/live   # Liveness probe (Kubernetes)
curl http://localhost:9090/health/ready  # Readiness probe (Kubernetes)
curl http://localhost:9090/metrics       # Prometheus metrics
```

---

## REST API

MIRA ships an optional REST HTTP API for scripting, dashboards, or non-MCP integrations. Disabled by default.

### Enable

```bash
# Via CLI flag
./mira server --with-api --api-addr :8080 --api-token my-secret

# Via config.yaml
api:
  enabled: true
  address: ":8080"
  auth_token: "my-secret"
```

### Authentication

When `auth_token` is set, every request must carry:

```
Authorization: Bearer my-secret
```

The `/openapi.json` endpoint is always public.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/memories` | Store a memory |
| `GET` | `/api/v1/memories/{id}` | Load full verbatim by UUID |
| `PUT` | `/api/v1/memories/{id}` | Update memory content |
| `DELETE` | `/api/v1/memories/{id}` | Delete a single memory |
| `POST` | `/api/v1/memories/recall` | Recall context (full CBA pipeline) |
| `POST` | `/api/v1/memories/search` | Semantic vector search |
| `POST` | `/api/v1/memories/consolidate` | Consolidate redundant memories |
| `DELETE` | `/api/v1/memories` | Clear memories (global or scoped) |
| `GET` | `/api/v1/timeline` | Chronological memory timeline |
| `POST` | `/api/v1/archive` | Archive old memories |
| `GET` | `/api/v1/causal/{id}` | Causal chain for a memory |
| `GET` | `/api/v1/status` | System status (JSON) |
| `GET` | `/openapi.json` | OpenAPI 3.1 specification |

### Quick Examples

```bash
# Store a memory
curl -s -X POST http://localhost:8080/api/v1/memories \
  -H "Authorization: Bearer my-secret" \
  -H "Content-Type: application/json" \
  -d '{"content":"We chose PostgreSQL for v2","wing":"backend","type":"decision"}'

# Recall context
curl -s -X POST http://localhost:8080/api/v1/memories/recall \
  -H "Authorization: Bearer my-secret" \
  -H "Content-Type: application/json" \
  -d '{"query":"Why PostgreSQL?","budget":2000,"wing":"backend"}'

# Get OpenAPI spec (no auth required)
curl -s http://localhost:8080/openapi.json | jq .info
```

See [docs/API_REFERENCES.md](docs/API_REFERENCES.md) for full request/response schemas.

---

## Performance

### Algorithmic Complexity

| Operation | Complexity | Notes |
|-----------|------------|-------|
| Store T0, T1, T2 | O(1) | Atomic insertion |
| Vector search | O(log n) | HNSW ANN |
| CBA scoring | O(n) | n = candidates |
| Greedy allocation | O(n²) | With dynamic renormalization |
| Causal graph BFS | O(V+E) | V = nodes, E = edges |

### Benchmarks

| Metric | Value |
|--------|-------|
| HNSW search | ~0.14 ms for 10K vectors (benchmarked) |
| SQLite fallback search | ~50 ms for 10K vectors (estimated) |
| Full allocation | ~35 ms for 100 candidates (estimated) |
| Cosine similarity | ~3.3M ops/sec |

### Optimizations in v0.3.3

- **Query Expansion** — multi-variant embedding averaging for robust cross-lingual retrieval
- **FTS5 Lexical Search** — SQLite full-text search with auto-triggers and backfill
- **RRF Hybrid Fusion** — Reciprocal Rank Fusion (`k=60`) combining HNSW and FTS5
- **Search-Time Clustering** — real-time deduplication at cosine similarity ≥ 0.88
- **Tag-Based Retrieval** — `memory_tags` table with automatic tag boosting in CBA
- **Heuristic Reranker** — optional lightweight lexical reranker
- **Adaptive Threshold Methods** — dynamic pruning with `iqr`, `elbow`, `mean_stddev`
- **Fallback Vector Store** — transparent HNSW → SQLite fallback when index not ready
- **Clear Memory Tool** — `mira_clear_memory` for global or room-scoped deletion
- **Causal Chain T0 Resolution** — `mira_causal_chain` resolves `T0:` verbatim references
- **ID Visibility in Outputs** — `mira_recall` and `mira_timeline` include memory IDs

### Optimizations in v0.3.1

- **Lazy Evaluation** — overlap calculation only for promising candidates
- **LRU Cache** — 1000 entries for query embeddings
- **HNSW Persistence** — fast index reload on restart
- **SQLite WAL Mode** — concurrent read/write performance
- **Adaptive Threshold** — lowered relevance floor for small corpora (< 10 memories)
- **Default Room Mapping** — auto-assigns standard rooms based on memory type

---

## Technical Architecture

### Hexagonal Architecture (Clean Architecture)

**Domain** — enterprise rules, no external dependencies
- `entities`: Verbatim, Fingerprint, Embedding, Candidate
- `valueobjects`: MemoryType, RenderMode, RelationType

**Use Cases** — application rules, depends only on Domain
- StoreMemory, RecallMemory (CBA), LoadMemory
- GetTimeline, GetStatus, GetCausalChain, Archive
- `ports`: Repository and service interfaces

**Interface Adapters** — implements ports
- `storage`: SQLiteRepository
- `vector`: HNSWStore, SQLiteVectorStore, overlap cache
- `extraction`: NativeExtractor, CybertronEmbedder
- `webhook`, `metrics`

**Frameworks & Drivers** — external technical details
- SQLite3, HNSW lib, Cybertron, MCP Server

### Project Structure

```
mira/
├── cmd/mira/              # Entry point (cobra CLI)
│   └── main.go            # Subcommands: server, migrate, doctor, query, export, import
├── internal/
│   ├── domain/
│   │   ├── entities/      # Business entities
│   │   └── valueobjects/  # Value objects
│   ├── usecases/
│   │   ├── ports/         # Interfaces (Repository, Services)
│   │   └── interactors/   # Use case implementations
│   ├── adapters/
│   │   ├── storage/       # SQLite repository
│   │   ├── vector/        # HNSW, SQLite vector store, overlap cache
│   │   ├── extraction/    # NLP, embeddings
│   │   ├── logging/       # Structured logging
│   │   ├── webhook/       # HTTP notifications
│   │   └── metrics/       # Prometheus metrics
│   ├── interfaces/
│   │   ├── mcp/           # MCP controller (stdio / SSE)
│   │   └── rest/          # Optional REST HTTP API (:8080)
│   ├── config/
│   └── app/               # Composition root (dependency injection)
├── docs/
│   ├── INDEX.md
│   ├── ARCHITECTURE.md
│   ├── FEATURES.md
│   └── API_REFERENCES.md
├── SKILL.md
├── config.example.yaml
└── README.md
```

---

## Development

### Testing

```bash
go test -v ./...            # Unit tests
go test -race ./...         # With race detector
go test -bench=. -benchmem ./...  # Benchmarks
go test -cover ./...        # Coverage
```

### Make Commands

```bash
make build        # Build
make test         # Tests (with race detector)
make test-short   # Quick tests
make bench        # Benchmarks
make bench-full   # Full benchmarks
make run          # Build and run with config.yaml
make clean        # Clean build artifacts and data
make lint         # Run linters
make fmt          # Format code
make install      # Install to GOPATH/bin
make prepublish VERSION=x.y.z  # Prepare a release
```

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the full release history.

---

## References

### Key Libraries

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) — OpenAI tokenization
- Native Go implementation — rule-based NLP/NER
- [cybertron](https://github.com/nlpodyssey/cybertron) — Transformer embeddings
- [hnsw](https://github.com/coder/hnsw) — HNSW graphs
- [mcp-go](https://github.com/mark3labs/mcp-go) — MCP protocol

### Embedding Model

- **Model:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Size:** ~80 MB
- **Performance:** ~1000 texts/sec on CPU

---

<div align="center">

**MIRA** — _Memory with Information-theoretic Relevance Allocation_

_"Memory is the sap of artificial intelligence."_

[API Reference](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md)

</div>

# MIRA Feature Catalog

Complete inventory of MIRA capabilities.

---

## Memory Model

| Feature | Description |
|---------|-------------|
| T0 Verbatim Storage | Full UTF-8 text up to 64KB |
| T1 Fingerprint Extraction | Structured JSON with entities, subjects, decisions, negation flag |
| T2 Embedding Generation | 384D vectors via Cybertron (local transformer) |
| Triple Representation Pipeline | Automatic T0→T1→T2 extraction on store |
| Render Mode Adaptation | Header / Fingerprint / Verbatim based on remaining budget |
| Embedding LRU Cache | 1000-entry thread-safe cache for query embeddings |
| Cross-Layer Validation | Soft T0/T1 coherence checks: entity presence, type/data alignment, token ratio sanity |
| Negation Detection | EN/FR regex patterns (`not`, `pas`, `jamais`…) set `negated=true` on fingerprints |
| Exact Deduplication | Content-hash exact-match via `SearchExact` prevents storing identical verbatims twice |

---

## Memory Types & Lifecycle

| Feature | Description |
|---------|-------------|
| Auto Type Detection | Detects decision, fact, preference, session_note, debug_log |
| Decay Rate System | Per-type exponential decay (λ from 0.001 to 0.5) |
| Auto-Archive | Session notes (>30d) and debug logs (>7d) automatically archived |
| Archive Tool (`mira_archive`) | Manual archive trigger with token-freed stats |
| Clear Memory Tool (`mira_clear_memory`) | Global or wing/room-scoped permanent deletion |
| Default Room Mapping | Auto-assigns rooms based on detected memory type |

---

## Retrieval & Ranking

| Feature | Description |
|---------|-------------|
| Context Budget Allocation (CBA) | Greedy token-budget optimization with diversity boost |
| 6-Dimensional Scoring | Relevance × Density × Recency × (1-Overlap) × Session × Causal |
| Diversity Boost | +10% per new subject covered during greedy selection |
| Session Boost | +20% boost for memories within 2-hour window |
| Session Memory Injection | `session_id` parameter boosts memories selected in previous turns (+30%) |
| Dynamic Budget Adjustment | Semantic budget scales ±20% based on query token count |
| Causal Penalty | Penalizes over-selection from long causal chains |
| Dynamic Renormalization | Overlap and causal penalty recalculated during greedy selection |
| Adaptive Threshold (Small Corpora) | Lowers relevance floor for databases with <10 memories |
| Cross-Language Recall | Cross-lingual embeddings + broad fallback for sparse queries |
| Fallback Wings | Comma-separated alternate wings searched if primary is empty |
| Query Expansion | Generates query variants and averages embeddings |
| FTS5 Lexical Search | SQLite full-text search with auto-triggers and backfill |
| Hybrid Search (RRF) | Reciprocal Rank Fusion (k=60) merging dense HNSW and lexical FTS5 results |
| Search-Time Clustering | Real-time deduplication by clustering candidates with cos-sim ≥ 0.88 |
| Tag-Based Retrieval | `memory_tags` table with automatic tag boost in scoring |
| Heuristic Reranker | Lightweight lexical reranker (Jaccard + phrase bonus + length balance) |
| Adaptive Threshold Methods | Three methods: `iqr` (default), `elbow`, `mean_stddev` |
| Fallback Vector Store | Transparent HNSW → SQLite fallback when index is not ready |
| Exact Vector Search | `SearchExact` content-hash matching for deduplication and precise lookups |

---

## Vector Search Infrastructure

| Feature | Description |
|---------|-------------|
| HNSW Approximate NN | O(log n) search with M=32, efSearch=100 |
| HNSW Validation | Runtime dimension + model-hash consistency check; auto-rebuild on mismatch |
| HNSW Persistence | Saves/loads `vectors.bin` across restarts |
| Background Index Build | Builds HNSW from SQLite embeddings on startup if needed |
| SQLite Vector Store | Exact brute-force cosine similarity fallback |
| Fallback Vector Store Wrapper | Automatic failover when HNSW reports "not ready" |
| Cross-Platform HNSW | Unix full support + Windows stub fallback |

---

## Causal Graph

| Feature | Description |
|---------|-------------|
| 5 Relation Types | BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES |
| Semantic Filtering | Relations require shared subjects/entities between source and target |
| Pattern-Based Detection | Regex detection from verbatim content (EN + FR) |
| Temporal Validation | Cause must precede effect in time |
| BFS Chain Traversal | `mira_causal_chain` upstream with configurable `max_depth` |
| Consequence Traversal | Optional downstream effect inclusion |
| T0 Reference Resolution | Resolves `T0:<uuid>` verbatim refs to fingerprint IDs |
| Cycle Detection | Visited-node tracking prevents infinite loops |

---

## MCP API Tools

| Tool | Description |
|------|-------------|
| `mira_store` | Store memory with auto-extraction and exact deduplication |
| `mira_recall` | Retrieve context via multi-stage pipeline (expansion, hybrid search, clustering, reranker). Supports `session_id` for multi-turn injection |
| `mira_load` | Load full verbatim by ID |
| `mira_causal_chain` | Trace causal relationships |
| `mira_timeline` | Chronological reconstruction with filters |
| `mira_status` | System statistics, health, version, and uptime |
| `mira_health` | Quick health check: JSON with `status`, `db_connected`, `memory_count` |
| `mira_archive` | Archive old memories |
| `mira_clear_memory` | Permanent deletion (global or room-scoped) |

---

## Observability & Operations

| Feature | Description |
|---------|-------------|
| Prometheus Metrics | Counters (`store_total`, `recall_total`, `search_total`, `errors_total`, `store_facts_total`, `recall_selected_total`), histograms (`*_duration_seconds`), gauges (`memory_count`, `vector_count`) exposed on `:9090/metrics` |
| Health Endpoints | `/health`, `/health/live`, `/health/ready` |
| Health Check Tool | `mira_health` MCP tool for quick liveness probes |
| Structured Metrics Output | Latency percentiles (p50/p90/p95/p99) in `mira_status` |
| Webhook Notifications | `store.completed`, `embedding.completed`, `causal.detected`, etc. |
| Circuit Breaker | Resilience pattern for webhook HTTP calls |
| Retry Logic | DLQ retry loop every 60s + exponential backoff |
| Structured Logging | `SimpleLogger` with prefix support integrated into StoreMemory and RecallMemory |
| Graceful Shutdown | 5-second timeout on SIGINT/SIGTERM; SSE server shutdown before DB close |

---

## Configuration & Deployment

| Feature | Description |
|---------|-------------|
| YAML Configuration | Full `config.yaml` with validation |
| Zero-Config Startup | Embedded defaults; no config file required |
| Multi-Platform Config Resolution | `-config` → `MIRA_CONFIG` → `./config.yaml` → OS config dir |
| `MIRA_DATA_PATH` Override | Environment variable to change storage path |
| Project-Scoped Storage | Default `.mira/` per project with auto-gitignore |
| Config Validation | Sensible defaults applied for all missing/invalid fields |
| SSE Transport | MCP server can run over Server-Sent Events (`transport: sse`) in addition to stdio |

---

## NLP & Extraction

| Feature | Description |
|---------|-------------|
| Native Go Extractor | Rule-based NER replacing archived `prose` library |
| Entity Extraction | Capitalized word detection + known entity gazetteers |
| Subject Inference | Infers subjects from sentence structure |
| Token Counting | `tiktoken-go` for accurate OpenAI-style token counts |
| Causal Pattern Detection | Regex-based relation detection (EN + FR) |
| Model Hash Tracking | Tracks which embedding model generated each vector |
| T0/T1 Validation Alerts | Inconsistencies stored in `fingerprint.data.custom.validation_alerts` |

---

## Testing & Quality

| Feature | Description |
|---------|-------------|
| Unit Tests | ~77% coverage across domain, usecases, adapters |
| Race Detector Tests | `go test -race ./...` |
| Benchmarks | Go benchmarks + HTML dashboard (`scripts/benchmark.sh`) |
| Benchmark Visualization | HTML dashboard with performance insights |
| Health Check Tests | Tests for liveness, readiness, component status |

---

*Version: 0.4.7*  
*Last updated: 2026-04-30*

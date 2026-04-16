# Changelog

All notable changes to MIRA will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.1] - 2026-04-17

### Performance & Scalability
- **HNSW Tuned Defaults**: Increased `M` from 16 → 32 and `EfSearch` from 50 → 100 for better recall quality and graph connectivity at scale
- **Concurrent Embedding Pool**: Replaced the global mutex in `CybertronEmbedder` with a model instance pool (default 2 instances)
  - Multiple `Encode` calls now run in parallel instead of being fully serialized
  - Significant latency reduction under concurrent load (e.g. multiple `mira_recall` or `mira_store` operations)

### Changed
- **Parallel Recall Pipeline**: Dense HNSW search and lexical FTS5 search now execute concurrently via `errgroup`
  - Both branches run simultaneously and merge via RRF once complete
  - Reduces end-to-end `mira_recall` latency when FTS5 is enabled

### Fixed
- **Removed Ineffective HNSW Parameter**: `EfConstruction` was configured but never applied by the underlying `coder/hnsw` library (v0.4.0)
  - Parameter is now documented as inactive and defaults to `0` to avoid confusion

## [0.4.0] - 2026-04-17

### Added
- **Enhanced Recall Pipeline**: Multi-stage retrieval system transforming `mira_recall` from simple vector search into a hybrid intelligence pipeline
  - **Query Expansion**: Generates semantic variants (cleaned, stop-word-free, keyword-focused) and averages their embeddings for robust cross-lingual retrieval
  - **FTS5 Lexical Search**: SQLite full-text search via `verbatim_fts` virtual table with auto-triggers and automatic backfill
  - **RRF Hybrid Fusion**: Reciprocal Rank Fusion (`k=60`) merges dense HNSW and lexical FTS5 rankings into a unified candidate list
  - **Search-Time Clustering**: Real-time deduplication by grouping candidates with cosine similarity ≥ 0.88 and selecting the best representative per cluster
  - **Tag-Based Retrieval**: New `memory_tags` table indexing extracted entities, subjects, and keywords; matching tags receive an additive relevance boost during CBA scoring
  - **Heuristic Reranker**: Optional lightweight pure-Go reranker using Jaccard overlap, exact-phrase bonus, and length balance; blended at `0.7*semantic + 0.3*rerank`
  - **Adaptive Threshold Methods**: Dynamic relevance pruning with three strategies (`iqr`, `elbow`, `mean_stddev`), clamped between configurable floor (0.15) and ceiling (0.75)
  - **Fallback Vector Store**: Transparent `FallbackVectorStore` wrapper that auto-routes HNSW searches to SQLite vector store when the index is not ready

- **New `recall` Configuration Section**: Centralized configuration for all recall enhancements
  - `adaptive_threshold_method`, `enable_fts5`, `rrf_k`, `query_expansion`, `search_time_clustering`, `reranker`

- **TagRepository Port & Implementation**: `StoreTags`, `GetVerbatimsByTags`, `GetTagsForVerbatim` implemented in `SQLiteRepository`

- **StoreMemory Tag Extraction**: Automatic tag extraction from entities, subjects, and content keywords on every store operation

### Changed
- **Documentation Overhaul**
  - Added `docs/INDEX.md`: documentation entry point and capability map
  - Added `docs/ARCHITECTURE.md`: comprehensive technical deep-dive into Clean Architecture, T0/T1/T2, CBA algorithm, recall pipeline, storage layer, and configuration model
  - Added `docs/FEATURES.md`: complete feature catalog covering all MIRA capabilities
  - Updated `README.md` and `README_FR.md` with the enhanced recall pipeline, updated configuration examples, and revised project structure
  - Updated `docs/API_REFERENCES.md` with recall pipeline internals and configuration reference

- **Config Example Updated**: `config.example.yaml` now includes the complete `recall` section

### Fixed
- **HNSW Reliability**: Recall no longer fails when HNSW is still building from scratch; fallback vector store ensures 100% recall availability

## [0.3.3] - 2026-04-14

### Added
- **Clear Memory Tool**: New `mira_clear_memory` MCP tool for permanent memory deletion
  - `global` mode: deletes all memories across all wings/rooms and resets the HNSW index
  - `room` mode: deletes only memories within a specific `wing` and optional `room`
  - Implemented `ClearAll` and `ClearByRoom` in SQLite repository and HNSW vector store

- **T0 Reference Resolution for Causal Chain**: `mira_causal_chain` now correctly resolves `T0:<uuid>` verbatim references to fingerprint IDs before tracing the causal graph
  - Added `GetFingerprintByVerbatimID` repository method
  - Enables LLMs to chain tools using verbatim references returned by recall

- **ID Exposure in Tool Outputs**: `mira_recall` and `mira_timeline` now include exact memory IDs in their text output
  - Allows reliable downstream chaining to `mira_causal_chain` and `mira_load`

### Fixed
- **LLM Self-Correction for Invalid IDs**: `mira_causal_chain` and `mira_load` return explicit, actionable error messages when invalid or invented IDs are passed
  - Error text instructs the caller to use exact IDs from `mira_recall` or `mira_timeline`
  - Reduces hallucinated identifiers like `T0:b0p-session-note-1`

- **Cross-Language Recall Robustness**: Adaptive relevance threshold lowered from 0.30 to 0.15 for small corpora (<100 memories)
  - Significantly improves retrieval when querying in one language against memories stored in another (e.g., English query → French memories)

## [0.3.2] - 2026-04-13

### Added
- **Fallback Wings for Recall**: `mira_recall` now supports `fallback_wings` parameter
  - Comma-separated list of alternate wings to search when primary wing yields no results
  - Enables cross-team/cross-project knowledge retrieval without changing the query

- **Default Room Mapping**: Automatic room assignment based on detected memory type
  - `decision` → `decisions`, `fact` → `facts`, `preference` → `preferences`
  - `session_note` → `session`, `debug_log` → `debug`
  - Reduces friction when storing memories without explicit room metadata

- **Adaptive Relevance Threshold**: Recall now adapts pruning threshold for small corpora
  - Databases with fewer than 10 memories use a lowered threshold (0.3) so queries still return useful results

### Fixed
- **Fingerprint UUID Alignment**: `GetCandidatesWithEmbeddings` now correctly returns fingerprint IDs instead of verbatim IDs
- **Fingerprint Data Population**: JSON fingerprint data is now properly unmarshalled from SQLite (was returning empty Data fields)
- **HNSW ID Consistency**: `AddCandidate` now maps HNSW nodes using `Verbatim.ID` instead of `Fingerprint.ID`, aligning the vector index with the embeddings table
- **CLI Version String**: Corrected version output from `v0.3.0` to `v0.3.2`

### Changed
- **Replaced archived prose library with native Go implementation**
  - Removed `github.com/jdkato/prose` dependency (library is archived/unmaintained)
  - Created `NativeExtractor`: Rule-based NLP/NER using regex patterns and gazetteers
  - Full interface compliance: `FingerprintExtractor`, `Embedder`, `CausalRelationDetector`
  - Tokenization with `tiktoken-go` for accurate token counting
  - Entity extraction via capitalized word detection + known entity lists
  - Causal relation detection using regex patterns (BECAUSE, TRIGGERED, CONTRADICTS, etc.)
  - Zero breaking changes: External MCP API unchanged

- **Documentation Cleanup**: Removed internal development docs (`docs/adr/`, `docs/dev/`, `docs/PLAN.md`) and temporary debug files from public repository

- **Project-Scoped Data Storage**: Default storage path changed from `./mira_data` to `.mira/`
  - Each project now gets its own isolated, hidden memory database
  - MIRA automatically appends `.mira/` to `.gitignore` when a `.gitignore` file exists in the project root
  - Prevents accidental commits of runtime data

- **Zero-Config Startup & Multi-Platform Config Resolution**
  - MIRA can now start without any `config.yaml` file (uses built-in defaults)
  - Added `config.ResolveConfigPath()` with cross-platform fallbacks:
    - explicit `-config` flag → `MIRA_CONFIG` env var → `./config.yaml` → OS user config dir (`~/.config/mira/`, `~/Library/Application Support/mira/`, `%APPDATA%/mira/`)
  - Added `MIRA_DATA_PATH` environment variable to override the storage path without editing config files
  - Improves MCP client integration (e.g. b0p) by removing the hard dependency on a present `config.yaml`

## [0.3.1] - 2026-04-12

### Added
- **Structured Logging Interface**: New `Logger` port with implementations
  - `SimpleLogger`: Production-ready logger with prefix support
  - `NoOpLogger`: No-op implementation for testing
  - Integrated into `StoreMemory` to fix silent error handling
  - Logs warnings for non-fatal errors (vector store, causal graph)

- **Complete Causal Graph BFS**: Full implementation of causal chain traversal
  - `GetChain()`: BFS traversal up to `maxDepth` levels for ancestors
  - `GetConsequences()`: BFS traversal up to `maxDepth` levels for descendants
  - Proper cycle detection with visited node tracking
  - Respects the `max_depth` parameter in `mira_causal_chain` tool

- **EmbeddingSource Interface**: Dependency inversion for vector stores
  - New `EmbeddingSource` port defining data access for vector stores
  - `GetCandidatesWithEmbeddings()`: Batch fetch candidates by ID
  - `GetAllEmbeddings()`: Fetch all embeddings for index building
  - `HNSWStore` now depends on `EmbeddingSource` interface (was concrete `SQLiteRepository`)
  - Enables swapping SQLite with other storage backends

- **Centralized Vector Utilities**: Eliminated cosine similarity duplication
  - `util.CosineSimilarity()`: Full cosine similarity calculation
  - `util.CosineSimilarityNormalized()`: Optimized for pre-normalized vectors
  - `util.CosineDistance()`: Returns distance (1 - similarity)
  - Removed 4 duplicate implementations across the codebase
  - All vector operations now use centralized, tested functions

- **Comprehensive Documentation**: Complete godoc for all ports
  - Package-level documentation for `ports` packages
  - Interface documentation with usage examples
  - Method documentation with parameter descriptions
  - Architecture notes and dependency explanations

### Changed
- **Configuration Completeness**: Added missing config fields
  - `embeddings.model_hash`: Model identifier for tracking
  - `mcp.timeout_seconds`: MCP operation timeout
  - `storage.sqlite.*`: Full SQLite configuration (journal_mode, synchronous, cache_size, mmap_size, temp_store)
  - `overlap_cache.*`: TTL and max entries for overlap cache
  - All fields validated with sensible defaults

- **Documentation Accuracy**: 100% consistency between README and code
  - Removed references to non-existent files (WHITEPAPER.md, API_EXAMPLES.md)
  - Fixed Prometheus metrics names to match implementation
  - Synchronized causal detection patterns with actual code
  - Documented all webhook headers including legacy ones
  - Corrected project structure diagrams

### Removed
- **Unused Configuration**: Removed `max_concurrent_queries`
  - Parameter was documented but never implemented
  - Removed from config struct, validation, and documentation

### Fixed
- **Silent Error Handling**: StoreMemory now logs all non-fatal errors
  - Vector store addition failures
  - Causal node creation failures
  - Causal relation detection failures
  - Edge addition failures

---

## [0.3.0] - 2026-04-10

### Changed
- **Clean Architecture Refactor**: Complete codebase restructure following Uncle Bob's Clean Architecture principles
  - Clear separation: Domain → Use Cases → Interface Adapters → Frameworks
  - Dependency Inversion: inner layers define interfaces (ports), outer layers implement
  - Domain Layer (`internal/domain/`): Pure business entities and value objects
    - Entities: Verbatim, Fingerprint, Embedding, CausalNode, Candidate
    - Value Objects: MemoryType, RenderMode, RelationType
  - Use Cases Layer (`internal/usecases/`): Application business rules
    - Interactors: StoreMemory, RecallMemory, LoadMemory, GetTimeline, GetStatus, GetCausalChain, ArchiveMemories
    - Ports: Repository interfaces, Service interfaces
  - Adapters Layer (`internal/adapters/`): Concrete implementations
    - SQLite repository implementing all repository ports
    - Native Go NLP extractor with rule-based NER and cybertron/simple embedders
    - SQLite vector store and overlap cache
    - HNSW vector store for approximate nearest neighbor search
  - Interfaces Layer (`internal/interfaces/`): External protocol adapters
    - MCP controller decoupled from business logic
  - App Layer (`internal/app/`): Composition root with dependency injection
  - Full logic preservation: All algorithms (CBA scoring, extraction, causal detection) remain identical
  - Zero breaking changes: External MCP API unchanged
  - Enhanced testability: All use cases testable with mocks

- **Enhanced Error Handling**: Improved error handling throughout vector stores
  - HNSW search now returns errors for diagnostics instead of silently failing
  - Thread-safe error logging in HNSW search operations

- **Configuration Validation**: Automatic defaults for invalid values
  - Comprehensive validation for all config sections (Storage, Embeddings, Allocator, HNSW, Metrics, Webhooks, MCP)
  - Automatic application of sensible defaults for missing or invalid values
  - Config validation tests

### Added
- HNSW Vector Index: Fully integrated Hierarchical Navigable Small World graph
  - O(log n) approximate nearest neighbor search (vs O(n) SQLite before)
  - 100x+ speedup at scale (>100k memories)
  - Background index building from SQLite on startup via `BuildFromStore()`
  - Automatic fallback to SQLite exact search during build or for filtered queries
  - Hybrid search with metadata filtering (wing/room)

- Cybertron Embeddings: Real transformer embeddings via Cybertron library
  - Integrated sentence-transformers/all-MiniLM-L6-v2 model (384D)
  - Automatic model download on first run (~80MB)
  - Fallback to SimpleEmbedder if Cybertron fails to load
  - Thread-safe encoding with mutex protection

- Performance Metrics System: Comprehensive metrics collection
  - Counters for operations (store, recall, search)
  - Latency tracking with percentiles (p50, p90, p95, p99)
  - Gauges for active searches, queue depth, cache hit rates
  - Integrated into `mira_status` tool output
  - Thread-safe atomic operations

- Prometheus Metrics Export: External monitoring support
  - HTTP endpoint at `/metrics` for Prometheus scraping
  - Health check endpoints at `/health`, `/health/live`, `/health/ready`
  - Configurable listen address (default: `:9090`)
  - Histograms with configurable buckets
  - Enabled via `metrics.enabled: true` in config

- Webhook Notifications: HTTP callbacks for events
  - Event types: `embedding.completed`, `embedding.failed`, `store.completed`, `causal.detected`
  - HMAC signature support for security
  - Configurable retry logic with circuit breaker pattern
  - Fixed event routing to subscribed endpoints
  - Enabled via `webhooks.enabled: true` in config

- Retry Logic: Exponential backoff for resilient operations
  - Configurable max attempts, initial delay, max delay, multiplier
  - Support for custom retryable error types
  - Context-aware cancellation

- Circuit Breaker: Resilience pattern for external calls
  - States: Closed, Open, HalfOpen
  - Configurable failure thresholds and timeout
  - Thread-safe implementation

- Health Checks: Kubernetes-ready probes
  - Liveness probe (`/health/live`) - is the process running
  - Readiness probe (`/health/ready`) - is the service ready to accept traffic
  - Full health check (`/health`) - detailed component status

- API Examples Documentation: Practical usage guide
  - `API_EXAMPLES.md` with 7 sections
  - JSON examples for all MCP tools
  - Integration patterns (sessions, code reviews, onboarding)
  - Best practices and error handling

### Fixed
- **Critical Bug Fixes**
  - HNSW `BuildFromStore()` now properly loads all embeddings from SQLite on startup (was empty)
  - `SQLiteVectorStore.Search()` now sorts results by cosine similarity (was sorted by date only)
  - `GetFingerprintByID()` now properly implemented (was returning "not implemented" error)
  - Webhook manager now correctly routes events to subscribed endpoints (was comparing wrong IDs)
  - Causal detection now respects temporal ordering (cause must precede effect)
  - Data race in Cybertron embedder fixed with proper mutex locking

### Configuration
```yaml
metrics:
  enabled: true
  prometheus_addr: ":9090"

webhooks:
  enabled: false
  workers: 3
  endpoints: []

hnsw:
  M: 16
  Ml: 0.25
  ef_construction: 200
  ef_search: 50
```

## [0.2.0] - 2026-04-09

### Added
- Technical Whitepaper: Comprehensive architecture documentation
- Extended VectorStore Interface: Added AddCandidate() and Delete() methods
- BuildFromStore(): Rebuild HNSW index from all existing embeddings in SQLite

### Changed
- Default Vector Search: Now uses HNSW by default (was SQLite)
- Configuration: HNSW config section now active by default
- Complexity: Vector search O(log n) instead of O(n)

## [0.1.0] - 2026-04-08

### Added
- Initial release of MIRA
- T0/T1/T2 memory architecture
- CBA (Context Budget Allocator) algorithm
- SQLite storage with WAL mode
- MCP server with 7 tools: mira_store, mira_recall, mira_load, mira_causal_chain, mira_status, mira_timeline, mira_archive
- Causal graph with 5 relation types: BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES
- UTF-8 extraction with entity recognition
- Sigmoid density scoring
- Session boost and causal penalty
- Greedy allocation with dynamic renormalization
- Embedding model versioning
- SimpleEmbedder (deterministic pseudo-random embeddings) for testing

---

[0.4.0]: https://github.com/benoitpetit/mira/releases/tag/v0.4.0
[0.3.3]: https://github.com/benoitpetit/mira/releases/tag/v0.3.3
[0.3.2]: https://github.com/benoitpetit/mira/releases/tag/v0.3.2
[0.3.1]: https://github.com/benoitpetit/mira/releases/tag/v0.3.1
[0.3.0]: https://github.com/benoitpetit/mira/releases/tag/v0.3.0
[0.2.0]: https://github.com/benoitpetit/mira/releases/tag/v0.2.0
[0.1.0]: https://github.com/benoitpetit/mira/releases/tag/v0.1.0

# Changelog

All notable changes to MIRA will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[0.3.1]: https://github.com/benoitpetit/mira/releases/tag/v0.3.1
[0.3.0]: https://github.com/benoitpetit/mira/releases/tag/v0.3.0
[0.2.0]: https://github.com/benoitpetit/mira/releases/tag/v0.2.0
[0.1.0]: https://github.com/benoitpetit/mira/releases/tag/v0.1.0

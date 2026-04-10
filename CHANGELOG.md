# Changelog

All notable changes to MIRA will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
    - Prose-based NLP extractor with cybertron/simple embedders
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

[0.3.0]: https://github.com/benoitpetit/mira/releases/tag/v0.3.0
[0.2.0]: https://github.com/benoitpetit/mira/releases/tag/v0.2.0
[0.1.0]: https://github.com/benoitpetit/mira/releases/tag/v0.1.0

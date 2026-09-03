# MIRA Documentation

Welcome to the comprehensive documentation for **MIRA** — *Memory with Information-theoretic Relevance Allocation*.

---

## What is MIRA?

MIRA is a **long-term memory system for Large Language Models (LLMs)** designed to optimize every token in a constrained context window. Unlike simple RAG systems that retrieve purely by similarity, MIRA solves an **optimization problem under constraint**: maximize useful information within a fixed token budget.

### Core Philosophy

- **Context is finite** — LLM windows range from 4K to 128K tokens, but projects span thousands of interactions.
- **Not all memories are equal** — information density, recency, causal links, and narrative coherence matter.
- **Local-first** — 100% local execution, deterministic, no external APIs required.
- **[Market benchmark references](MARKET_REFERENCES.md)** — published competitor figures with explicit non-comparability guidance.

---

## Documentation Map

| Document | Description |
|----------|-------------|
| [README.md](../README.md) | Quick start, installation, high-level overview |
| [README_FR.md](../README_FR.md) | Version française du README |
| [API_REFERENCES.md](API_REFERENCES.md) | MCP tool examples, REST HTTP API (13 endpoints), integration patterns |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Clean Architecture, T0/T1/T2 memory model, CBA algorithm, causal graph |
| [FEATURES.md](FEATURES.md) | Complete feature catalog |
| [MARKET_REFERENCES.md](MARKET_REFERENCES.md) | Published market context and fair-comparison protocol |
| [CHANGELOG.md](../CHANGELOG.md) | Full release history and version notes |
| [SKILL.md](../SKILL.md) | Agent guidelines for MCP memory loops |

---

## Key Capabilities

### Memory Storage
- **Triple Representation (T0/T1/T2)**: Verbatim text, structured fingerprint, semantic embedding
- **Automatic Type Detection**: decision, fact, preference, session_note, debug_log
- **Causal Graph Linking**: Auto-detects BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES relations
- **Project-Scoped Storage**: Isolated `.mira/` databases per project with auto-gitignore
- **Cross-Layer Validation**: T0/T1 coherence checks (entity presence, type/data alignment)
- **Negation Detection**: EN/FR patterns mark extracted fingerprints with `negated` flag
- **Exact Deduplication**: Content-hash exact-match prevents storing identical verbatims twice

### Memory Retrieval
- **Context Budget Allocation (CBA)**: Greedy token-budget optimizer with 6-dimensional scoring + diversity boost
- **HNSW Vector Search**: O(log n) approximate nearest neighbor over millions of memories
- **Hybrid Search**: Dense + FTS5 lexical fusion via Reciprocal Rank Fusion (RRF)
- **Query Expansion**: Multi-variant embedding averaging for cross-lingual robustness
- **Search-Time Clustering**: Real-time deduplication of near-duplicate results
- **Heuristic Reranker**: Lightweight lexical reranking for precision boost
- **Dynamic Budget Adjustment**: Semantic budget scales ±20% based on query complexity
- **Multi-Turn Session Injection**: `session_id` boost recalls memories selected in previous turns

### Operations & Observability
- **14 MCP Tools**: store, ingest, recall, load, update, search, consolidate,
  causal_chain, timeline, status, health, archive, clear_memory, and compress
- **REST HTTP API**: Optional HTTP server on `:8080` — 13 endpoints with OpenAPI 3.1 spec, Bearer token auth, and graceful shutdown
- **Prometheus Metrics**: `/metrics`, `/health`, `/health/live`, `/health/ready` — counters for store/recall/search/embed/errors, gauges for memory/vector counts
- **Webhook Notifications**: HMAC-signed HTTP callbacks with circuit breaker resilience and DLQ retry
- **Zero-Config Startup**: Runs without `config.yaml` using embedded defaults
- **Network MCP transports**: Server-Sent Events (`transport: sse`) and
  stateless JSON-RPC at `POST /mcp` (`transport: http`) are available in
  addition to stdio
- **cobra CLI**: `server`, `migrate`, `doctor`, `query`, `export`, `import` subcommands

---

## Quick Links

- **Installation**: See [README.md#installation](../README.md#installation)
- **MCP Tool Reference**: [API_REFERENCES.md#mcp-tools](API_REFERENCES.md#mcp-tools)
- **REST HTTP API Reference**: [API_REFERENCES.md#rest-http-api](API_REFERENCES.md#rest-http-api)
- **Architecture Deep-Dive**: [ARCHITECTURE.md](ARCHITECTURE.md)
- **Feature Matrix**: [FEATURES.md](FEATURES.md)
- **Changelog**: [CHANGELOG.md](../CHANGELOG.md)

---

*Version documented: 0.5.0*  
*Last updated: 2026-09-02*

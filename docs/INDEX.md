# MIRA Documentation

Welcome to the comprehensive documentation for **MIRA** — *Memory with Information-theoretic Relevance Allocation*.

---

## What is MIRA?

MIRA is a **long-term memory system for Large Language Models (LLMs)** designed to optimize every token in a constrained context window. Unlike simple RAG systems that retrieve purely by similarity, MIRA solves an **optimization problem under constraint**: maximize useful information within a fixed token budget.

### Core Philosophy

- **Context is finite** — LLM windows range from 4K to 128K tokens, but projects span thousands of interactions.
- **Not all memories are equal** — information density, recency, causal links, and narrative coherence matter.
- **Local-first** — 100% local execution, deterministic, no external APIs required.

---

## Documentation Map

| Document | Description |
|----------|-------------|
| [README.md](../README.md) | Quick start, installation, high-level overview |
| [README_FR.md](../README_FR.md) | Version française du README |
| [API_REFERENCES.md](API_REFERENCES.md) | Practical MCP tool examples and integration patterns |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Clean Architecture, T0/T1/T2 memory model, CBA algorithm, causal graph |
| [FEATURES.md](FEATURES.md) | Complete feature catalog |
| [CHANGELOG.md](../CHANGELOG.md) | Full release history and version notes |
| [SKILL.md](../SKILL.md) | Agent guidelines for MCP memory loops |

---

## Key Capabilities

### Memory Storage
- **Triple Representation (T0/T1/T2)**: Verbatim text, structured fingerprint, semantic embedding
- **Automatic Type Detection**: decision, fact, preference, session_note, debug_log
- **Causal Graph Linking**: Auto-detects BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES relations
- **Project-Scoped Storage**: Isolated `.mira/` databases per project with auto-gitignore

### Memory Retrieval
- **Context Budget Allocation (CBA)**: Greedy token-budget optimizer with 6-dimensional scoring
- **HNSW Vector Search**: O(log n) approximate nearest neighbor over millions of memories
- **Hybrid Search**: Dense + FTS5 lexical fusion via Reciprocal Rank Fusion (RRF)
- **Query Expansion**: Multi-variant embedding averaging for cross-lingual robustness
- **Search-Time Clustering**: Real-time deduplication of near-duplicate results
- **Heuristic Reranker**: Lightweight lexical reranking for precision boost

### Operations & Observability
- **8 MCP Tools**: store, recall, load, causal_chain, timeline, status, archive, clear_memory
- **Prometheus Metrics**: `/metrics`, `/health`, `/health/live`, `/health/ready`
- **Webhook Notifications**: HMAC-signed HTTP callbacks with circuit breaker resilience
- **Zero-Config Startup**: Runs without `config.yaml` using embedded defaults

---

## Quick Links

- **Installation**: See [README.md#installation](../README.md#installation)
- **MCP Tool Reference**: [API_REFERENCES.md](API_REFERENCES.md)
- **Architecture Deep-Dive**: [ARCHITECTURE.md](ARCHITECTURE.md)
- **Feature Matrix**: [FEATURES.md](FEATURES.md)
- **Changelog**: [CHANGELOG.md](../CHANGELOG.md)

---

*Version documented: 0.4.2*  
*Last updated: 2026-04-16*

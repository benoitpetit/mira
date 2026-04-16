# MIRA API References

Practical examples for using MIRA's MCP tools in real-world scenarios.

---

## Table of Contents

1. [Tool Reference](#tool-reference)
2. [Basic Operations](#basic-operations)
3. [Knowledge Management](#knowledge-management)
4. [Decision Tracking](#decision-tracking)
5. [Debugging & Troubleshooting](#debugging--troubleshooting)
6. [Advanced Queries](#advanced-queries)
7. [Integration Patterns](#integration-patterns)
8. [System Monitoring](#system-monitoring)
9. [HTTP API](#http-api)
10. [Best Practices](#best-practices)

---

## Tool Reference

### Available MCP Tools

| Tool | Description | Arguments |
|------|-------------|-----------|
| `mira_store` | Store a memory with T0/T1/T2 extraction | `content` (required), `wing` (required), `room` (optional), `type` (optional) |
| `mira_recall` | Retrieve optimal context with budget via multi-stage pipeline (expansion, hybrid search, clustering, reranker) | `query` (required), `budget` (optional), `wing` (optional), `room` (optional), `fallback_wings` (optional) |
| `mira_load` | Load full verbatim by ID | `id` (required) |
| `mira_causal_chain` | Trace causal chain | `id` (required), `max_depth` (optional), `include_consequences` (optional) |
| `mira_timeline` | Chronological reconstruction | `wing` (required), `room` (optional), `since` (optional), `until` (optional), `type` (optional) |
| `mira_status` | System statistics and health | none |
| `mira_archive` | Archive old memories | none |
| `mira_clear_memory` | Permanently delete all or room-scoped memories | `mode` (`global` or `room`), `wing` (required for room), `room` (optional for room) |

**Note on `room`**: If omitted, MIRA automatically assigns a standard room based on the detected memory type:
- `decision` → `decisions`
- `fact` → `facts`
- `preference` → `preferences`
- `session_note` → `session`
- `debug_log` → `debug`

### Memory Types

When storing with `mira_store`, you can specify a type or let MIRA auto-detect:

| Type | Description | Decay Rate | Auto-Archive |
|------|-------------|------------|--------------|
| `decision` | Architectural/design decisions | Very slow (693 days) | Never |
| `fact` | Objective information | Slow (139 days) | Never |
| `preference` | Subjective preferences | Medium (69 days) | Never |
| `session_note` | Temporary session context | Fast (7 days) | 30 days |
| `debug_log` | Debug/troubleshooting logs | Very fast (1.4 days) | 7 days |

---

## Basic Operations

### Store a Simple Fact

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "The authentication service runs on port 8080 and uses JWT tokens with a 24-hour expiration.",
    "wing": "auth-service",
    "room": "configuration"
  }
}
```

**Response:**
```
Stored: 550e8400-e29b-41d4-a716-446655440000
Type: fact
Facts: 2
Tokens: 18
Model: a1b2c3d4
```

### Store an Architectural Decision

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "The team decided to migrate from REST to GraphQL for the API layer. This choice was made because it reduces over-fetching and provides better type safety. The migration will be done incrementally, starting with the user service.",
    "wing": "api-gateway",
    "room": "architecture",
    "type": "decision"
  }
}
```

**Response:**
```
Stored: 550e8400-e29b-41d4-a716-446655440001
Type: decision
Facts: 4
Tokens: 45
Model: a1b2c3d4
```

### Store User Preferences

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "User prefers dark mode interface and keyboard shortcuts over mouse interactions. Uses Vim bindings in all editors.",
    "wing": "user-profile",
    "room": "preferences",
    "type": "preference"
  }
}
```

### Store Session Notes (Auto-Archived)

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "Working on the payment integration today. Need to test the webhook handling.",
    "wing": "payment-service",
    "room": "daily-notes",
    "type": "session_note"
  }
}
```

**Note:** Session notes are automatically archived after 30 days.

---

## Knowledge Management

### Recall Context for a Query

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "What authentication method should I use for the API?",
    "budget": 2000,
    "wing": "auth-service"
  }
}
```

**Parameters:**
- `query` (required): Search query text
- `budget` (optional): Token budget, default 4000
- `wing` (optional): Filter by wing/namespace
- `room` (optional): Filter by room/sub-category
- `fallback_wings` (optional): Comma-separated fallback wings to search if primary wing yields no results

**Internal Pipeline:**
1. **Query Expansion** — generates semantic variants and averages their embeddings
2. **Hybrid Search** — HNSW dense search + SQLite FTS5 lexical search
3. **RRF Fusion** — merges both result sets with Reciprocal Rank Fusion (k=60)
4. **Search-Time Clustering** — deduplicates near-duplicate candidates
5. **Tag Boost** — boosts candidates with matching extracted tags
6. **Adaptive Threshold** — dynamic relevance floor (IQR/elbow/mean-stddev)
7. **Greedy CBA Allocation** — budget-aware selection with render-mode downgrading

**Response:**
```
=== MIRA CONTEXT ===
Query: What authentication method should I use for the API? | Budget: 2000
Wing: auth-service

--- [1] VERBATIM (18 tokens) ---
The authentication service runs on port 8080 and uses JWT tokens with a 24-hour expiration.

--- [2] FINGERPRINT (12 tokens) ---
[Type: fact | Date: 2026-04-09 | Wing: auth-service]
- Subject: authentication service
- Configuration: port 8080, JWT tokens, 24h expiration
→ T0:550e8400-e29b-41d4-a716-446655440000

=== Total: 30/2000 tokens (1.5%) ===

INSTRUCTIONS:
- HEADER: Reference only, use mira_load(id) for full content
- FINGERPRINT: Essential extracted facts (informational density)
- VERBATIM: Complete original content
```

### Load Full Content by ID

```json
{
  "tool": "mira_load",
  "arguments": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Note:** IDs can be specified as full UUID or short form `T0:550e8400`.

**Response:**
```
[ID: 550e8400-e29b-41d4-a716-446655440000 | Wing: auth-service | Date: 2026-04-09T10:30:00Z]

The authentication service runs on port 8080 and uses JWT tokens with a 24-hour expiration.
```

### Get Project Timeline

```json
{
  "tool": "mira_timeline",
  "arguments": {
    "wing": "api-gateway",
    "since": "2026-04-01T00:00:00Z",
    "until": "2026-04-09T23:59:59Z",
    "type": "decision"
  }
}
```

**Parameters:**
- `wing` (required): Namespace/project
- `room` (optional): Filter by room
- `since` (optional): Start date (ISO 8601)
- `until` (optional): End date (ISO 8601)
- `type` (optional): Filter by memory type

**Response:**
```
=== TIMELINE: api-gateway ===

[2026-04-09 14:30] decision: GraphQL migration
[2026-04-08 11:15] decision: Adopt OpenTelemetry for tracing
[2026-04-05 09:00] decision: Use PostgreSQL over MySQL
```

---

## Decision Tracking

### Trace Causal Chain

```json
{
  "tool": "mira_causal_chain",
  "arguments": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "max_depth": 5,
    "include_consequences": true
  }
}
```

**Parameters:**
- `id` (required): Fingerprint UUID
- `max_depth` (optional): Maximum depth to traverse, default 5
- `include_consequences` (optional): Include downstream effects, default false

**Response:**
```
=== CAUSAL CHAIN (Upstream) ===

  → [decision] GraphQL migration (2026-04-09)
 → [decision] Adopt OpenTelemetry for tracing (2026-04-08)
→ [decision] Use PostgreSQL over MySQL (2026-04-05)

=== CONSEQUENCES (Downstream) ===
→ [decision] Implement Apollo Federation (2026-04-10)
  → [fact] Schema registry established
```

### Clear Memory

Delete all memories globally or scoped to a specific wing/room.

```json
// Clear everything
{
  "tool": "mira_clear_memory",
  "arguments": {
    "mode": "global"
  }
}

// Clear one room
{
  "tool": "mira_clear_memory",
  "arguments": {
    "mode": "room",
    "wing": "backend-team",
    "room": "database-migration"
  }
}
```

**Response:**
```
All memories have been permanently deleted.
```

### Store with Causal Relation

When you store related decisions, MIRA automatically detects causal relationships:

```json
// First decision
{
  "tool": "mira_store",
  "arguments": {
    "content": "We decided to use PostgreSQL as our primary database because of its JSON support and reliability.",
    "wing": "database",
    "room": "architecture",
    "type": "decision"
  }
}

// Second decision that references the first
{
  "tool": "mira_store",
  "arguments": {
    "content": "Following the PostgreSQL decision, we chose to use pgAdmin for database management and monitoring.",
    "wing": "database",
    "room": "tools",
    "type": "decision"
  }
}
```

MIRA will automatically create a causal edge: `PostgreSQL decision → pgAdmin decision`

**Causal Relations Detected:**
- `BECAUSE` - B explains why A happened
- `TRIGGERED` - B triggered/caused A
- `CONTRADICTS` - A and B contradict each other
- `UPDATES` - B replaces/updates A
- `RESOLVES` - B resolves the problem in A

---

## Debugging & Troubleshooting

### Store Debug Log

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "Connection timeout error on service 'payment-gateway' at 2026-04-09T15:30:00Z. Error: dial tcp 10.0.1.25:8080: i/o timeout. Retry count: 3.",
    "wing": "payment-service",
    "room": "debug",
    "type": "debug_log"
  }
}
```

**Note:** Debug logs are automatically archived after 7 days.

### Recall Error Context

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "connection timeout payment gateway",
    "budget": 1500,
    "wing": "payment-service"
  }
}
```

### Archive Old Memories

```json
{
  "tool": "mira_archive",
  "arguments": {}
}
```

**Response:**
```
Archiving complete:
- Session notes > 30d: 45
- Debug logs > 7d: 128
Total freed: 15420 tokens
```

---

## Advanced Queries

### Multi-Wing Search

Search across multiple wings by omitting the wing filter:

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "authentication JWT security best practices",
    "budget": 3000
  }
}
```

### Room-Specific Search

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "database schema migration",
    "wing": "user-service",
    "room": "migrations",
    "budget": 2000
  }
}
```

### Large Budget Query

For complex architectural decisions requiring full context:

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "Microservices decomposition strategy service boundaries",
    "budget": 8000
  }
}
```

### Multilingual Queries

MIRA uses cross-lingual embeddings (`all-MiniLM-L6-v2`), so you can query in any language regardless of the language used when storing the memory. If the initial search is too sparse, MIRA automatically performs a broad fallback search with relaxed thresholds.

```json
// French query against English memories
{
  "tool": "mira_recall",
  "arguments": {
    "query": "règles de langue français anglais",
    "wing": "general"
  }
}

// Spanish query
{
  "tool": "mira_recall",
  "arguments": {
    "query": "reglas de idioma español inglés",
    "wing": "general"
  }
}
```

---

## Integration Patterns

### Session-Based Knowledge Building

Build up knowledge during a working session:

```bash
# 1. Store initial context
mira_store(content="Starting work on the payment integration...", wing="payment-service")

# 2. Store discoveries as you work
mira_store(content="Found that Stripe API requires webhook signature verification...", wing="payment-service")
mira_store(content="Test API keys start with 'sk_test_'...", wing="payment-service", room="configuration")

# 3. Store the final decision
mira_store(content="Decided to implement idempotency keys for all payment requests...", 
           wing="payment-service", room="architecture", type="decision")

# 4. Later, recall all relevant context
mira_recall(query="How should I handle payment retries?", wing="payment-service")
```

### Code Review Workflow

```json
// Store review feedback
{
  "tool": "mira_store",
  "arguments": {
    "content": "PR #234: Consider using context.WithTimeout instead of hard-coded timeouts. The auth service should respect cancellation signals.",
    "wing": "auth-service",
    "room": "code-reviews",
    "type": "preference"
  }
}

// Later, recall relevant patterns
{
  "tool": "mira_recall",
  "arguments": {
    "query": "context timeout cancellation patterns golang",
    "wing": "auth-service",
    "room": "code-reviews",
    "budget": 2000
  }
}
```

### Onboarding Documentation

```json
// Store onboarding knowledge
{
  "tool": "mira_store",
  "arguments": {
    "content": "To set up the development environment: 1) Install Docker 2) Run ./scripts/setup.sh 3) Copy .env.example to .env 4) Run make dev",
    "wing": "developer-experience",
    "room": "onboarding",
    "type": "fact"
  }
}

// Query for setup instructions
{
  "tool": "mira_recall",
  "arguments": {
    "query": "How do I set up the development environment?",
    "wing": "developer-experience"
  }
}
```

---

## System Monitoring

### Check System Status

```json
{
  "tool": "mira_status",
  "arguments": {}
}
```

**Response:**
```
MIRA System Status:
═══════════════════════════════════════
Storage:
  Verbatims: 1250
  Fingerprints: 1250
  Embeddings: 1250 (models: [a1b2c3d4])
  Causal Nodes: 1250
  Causal Edges: 342
  Total Tokens: 456780

Memory Distribution:
  Decisions: 45
  Facts: 623
  Preferences: 89
  Session Notes: 412
  Debug Logs: 81

Active Wings: [auth-service, api-gateway, payment-service, user-service]
═══════════════════════════════════════
```

---

## HTTP API

When metrics are enabled in configuration, MIRA exposes HTTP endpoints for monitoring.

### Health Checks

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Full health check with component status |
| `GET /health/live` | Liveness probe (Kubernetes) |
| `GET /health/ready` | Readiness probe (Kubernetes) |
| `GET /metrics` | Prometheus metrics export |

**Example:**
```bash
curl http://localhost:9090/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2026-04-10T14:30:00Z",
  "version": "0.4.0",
  "checks": {
    "database": {"status": "pass", "message": "connected"},
    "vector_store": {"status": "pass", "message": "HNSW ready"},
    "embedder": {"status": "pass", "message": "model loaded"}
  }
}
```

### Prometheus Metrics

Available metrics at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `mira_store_total` | Counter | Total store operations |
| `mira_recall_total` | Counter | Total recall operations |
| `mira_search_total` | Counter | Total vector searches |
| `mira_errors_total` | Counter | Total errors |
| `mira_store_duration_seconds` | Histogram | Store latency |
| `mira_recall_duration_seconds` | Histogram | Recall latency |
| `mira_search_duration_seconds` | Histogram | Search latency |
| `mira_embed_duration_seconds` | Histogram | Embedding latency |
| `mira_memory_count` | Gauge | Current number of memories |
| `mira_vector_count` | Gauge | Current number of vectors in index |

---

## Best Practices

### 1. Wing Naming Convention

Use consistent wing names:
- `service-name` (e.g., `auth-service`, `payment-service`)
- `project-name` (e.g., `mobile-app`, `web-frontend`)
- `team-name` (e.g., `platform-team`, `security-team`)

### 2. Room Organization

Use rooms to categorize within wings:
- `configuration` - Settings, environment variables
- `architecture` - Design decisions, ADRs
- `migrations` - Database changes
- `incidents` - Post-mortems, debug logs
- `api` - API documentation, contracts
- `code-reviews` - Review feedback
- `onboarding` - Documentation for new team members

### 3. Memory Type Selection

Choose appropriate types for better retrieval:
- **decision** - Use for choices that impact architecture or process
- **fact** - Use for objective information, documentation
- **preference** - Use for subjective choices, style guides
- **session_note** - Use for temporary context (auto-archived after 30 days)
- **debug_log** - Use for troubleshooting (auto-archived after 7 days)

### 4. Budget Guidelines

- **Quick lookup**: 500-1000 tokens
- **Context building**: 2000-4000 tokens (default)
- **Deep analysis**: 6000-8000 tokens
- **Full recall**: 10000+ tokens

### 5. Query Quality

Write specific queries for better results:
- ❌ "Tell me about auth"
- ✅ "JWT token expiration configuration auth service"

### 6. ID References

Reference memories by ID:
- Full UUID: `550e8400-e29b-41d4-a716-446655440000`
- Short form: `T0:550e8400`

---

## Error Handling

### Common Errors

**Empty Result:**
```
No memories found matching query. Try:
- Broadening your query terms
- Checking the wing/room filters
- Storing relevant memories first
```

**Budget Exhausted:**
```
=== Total: 4000/4000 tokens (100.0%) ===
Consider increasing budget or refining query
```

**Invalid ID:**
```
Error: invalid UUID: invalid syntax
Use mira_recall to find valid IDs, then mira_load to retrieve full content
```

**Wing Required:**
```
Error: wing is required
```

---

## Tips & Tricks

1. **Use UUID short forms**: IDs can be referenced as `T0:550e8400`
2. **Chain tools**: Use `mira_recall` to find IDs, then `mira_load` for full content
3. **Filter by type**: Use `mira_timeline` with `type: decision` to see all decisions
4. **Cross-wing search**: Omit `wing` parameter to search across all wings
5. **Causal exploration**: Use `include_consequences: true` to see both causes and effects
6. **Session boost**: Memories from the same 2-hour window get a 20% relevance boost
7. **Auto-detection**: Omit `type` parameter to let MIRA auto-detect the memory type

---

---

## Configuration Reference

### `recall` Section Configuration

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

| Key | Default | Description |
|-----|---------|-------------|
| `adaptive_threshold_method` | `iqr` | Method for dynamic relevance pruning |
| `adaptive_threshold_floor` | `0.15` | Minimum relevance threshold |
| `enable_fts5` | `true` | Enable SQLite FTS5 lexical search |
| `rrf_k` | `60` | RRF constant for dense+lexical fusion |
| `query_expansion.enabled` | `true` | Expand queries into semantic variants |
| `search_time_clustering.enabled` | `true` | Deduplicate results at search time |
| `reranker.enabled` | `false` | Enable heuristic lexical reranking |

*Last updated: 2026-04-16*
*Version: 0.4.0*

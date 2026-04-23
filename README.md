<div align="center">
  <img src="./logo.png" alt="MIRA Logo" width="800">

  # MIRA
  ### Memory with Information-theoretic Relevance Allocation

  **Long-term Memory System for LLMs with Optimal Context Budget Allocation**

  [![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
  [![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-0.4.4-blue?style=flat-square)]()
  [![Tests](https://img.shields.io/badge/Tests-77%25-brightgreen?style=flat-square)]()

  *100% Local • Deterministic • O(n log n) • Clean Architecture*

  [API Reference](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md) • [Skill](SKILL.md) • [Francais](README_FR.md) • [SOUL Extension](https://github.com/benoitpetit/soul)

</div>

---

## Table of Contents

- [What is MIRA?](#what-is-mira)
- [The Memory Revolution for LLMs](#the-memory-revolution-for-llms)
- [How It Works](#how-it-works)
- [3-Level Architecture (T0/T1/T2)](#3-level-architecture-t0t1t2)
- [The CBA Algorithm](#the-cba-algorithm)
- [Enhanced Recall Pipeline](#enhanced-recall-pipeline)
- [Causal Graph](#causal-graph)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [MCP API](#mcp-api)
- [Performance](#performance)
- [Technical Architecture](#technical-architecture)
- [Development](#development)
- [Changelog](#changelog)

---

## What is MIRA?

**MIRA** is a sophisticated long-term memory system designed specifically for **Large Language Models (LLMs)**. Unlike traditional memory systems that simply store and retrieve, MIRA uses **information-theoretic allocation** to optimize every token in the context window.

### SOUL Extension: Identity Preservation

MIRA answers *"What does the agent know?"* But a complete agent needs more: it needs to know *"Who is it?"*

[**SOUL**](https://github.com/benoitpetit/soul) (System for Observed Unique Legacy) is an **optional identity extension** for MIRA that captures, stores, and recalls the personality, voice, and values of AI agents across sessions and model changes.

To embed SOUL in MIRA, start with `--with-soul` or set `soul.enabled: true` in config. When enabled, SOUL provides **8 additional MCP tools** (16 total) for:
- Capturing identity from conversations
- Recalling identity prompts for LLM context injection
- Detecting identity drift after model changes
- Generating reinforcement prompts after model swaps

SOUL is **opt-in and disabled by default**. MIRA works perfectly alone (8 tools). To activate SOUL, use `--with-soul` flag or set `soul.enabled: true` in `config.yaml`. When enabled, they share the same SQLite database.

| Configuration | Tools | What it answers |
|---------------|-------|-----------------|
| MIRA only | 8 `mira_*` | "What does the agent know?" |
| MIRA + SOUL | 16 `mira_*` + `soul_*` | "What does the agent know?" + "Who is the agent?" |

### The Problem MIRA Solves

Modern LLMs suffer from a fundamental problem: **the context window is limited** (4K-128K tokens), but conversations and projects span thousands of interactions. How do we decide what to keep in context?

**Traditional approaches fail:**

- [x] Simple RAG: Retrieval based only on similarity, ignores information density
- [x] Sliding window: Loses critical information from the beginning
- [x] Static summarization: Doesn't adapt to the current query
- [x] Basic Vector DB: O(n) complexity, no budget management

**MIRA provides the solution:**

- [+] **Context Budget Allocation**: Optimizes every token across 6 dimensions
- [+] **Information Density**: Prioritizes memory-rich facts
- [+] **Temporal Coherence**: Maintains narrative continuity
- [+] **Causal Graph**: Understands cause-effect relationships
- [+] **O(log n) Search**: HNSW for millions of memories
- [+] **Clean Architecture**: Maintainable, testable, extensible

---

## The Memory Revolution for LLMs

### What MIRA Brings New

#### 1. **Information Allocation (CBA)**

Instead of simply retrieving the "most similar", MIRA solves an **optimization problem under constraint**: maximize useful information within a fixed token budget.

```
Score(m) = Relevance × Density × Recency × (1-Overlap) × Coherence × CausalPenalty
```

#### 2. **Triple Representation (T0/T1/T2)**

Each memory exists in 3 forms for different uses:

- **T0 (Verbatim)**: Full original text
- **T1 (Fingerprint)**: Structured extracted facts (~15% of tokens)
- **T2 (Embedding)**: 384D semantic vector for search

#### 3. **Integrated Causal Graph**

Automatic detection of relations (BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES) to trace reasoning chains.

#### 4. **Adaptive Rendering**

Based on remaining budget, MIRA intelligently chooses the detail level:

- **Header** (5 tokens): Reference only
- **Fingerprint** (~15% tokens): Essential facts
- **Verbatim** (100% tokens): Full text

---

## How It Works

### Overview Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MEMORY STORAGE                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Input Text         T1,T2 Extraction        Atomic Storage             │
│   ┌─────────┐       ┌──────────────┐          ┌─────────────────┐       │
│   │"We      │──────→│  Fingerprint │─────────→│  SQLite + HNSW  │       │
│   │ decided │       │  + Embedding │          │  (WAL Mode)     │       │
│   │ to use  │       └──────────────┘          └─────────────────┘       │
│   │PostgreSQL"            │                       │                     │
│   └─────────┘             ↓                       ↓                     │
│                        T1: {                 Vector Index               │
│                          - decision: "PostgreSQL"  ℝ³⁸⁴                 │
│                          - rejected: ["MySQL",     HNSW O(log n)        │
│                                      "MongoDB"]                         │
│                          - reason: ["ACID", "Exp"]                      │
│                          - type: DECISION                               │
│                                                                         │
│                         T2: [0.23, -0.15, 0.89, ...] 384D               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         RETRIEVAL (RECALL)                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Query "Why PostgreSQL?"                                               │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    ┌─────────────────┐    ┌──────────────────────┐    │
│   │ Embedding   │───→│  HNSW Search    │───→│  Composite Scoring   │    │
│   │ Query       │    │  Top 100        │    │  CBA Algorithm       │    │
│   │ ℝ³⁸⁴        │    │  O(log n)       │    │  O(n log n)          │    │
│   └─────────────┘    └─────────────────┘    └──────────────────────┘    │
│                                                        │                │
│                                                        ▼                │
│                                              Greedy Selection           │
│                                              with 4000 token budget     │
│                                                        │                │
│       ┌────────────────────────────────────────────────┘                │
│       ▼                                                                 │
│   Optimized Result:                                                     │
│   ┌───────────────────────────────────────────────────────────────┐     │
│   │ [1] Fingerprint: "PostgreSQL Decision (ACID, expertise)" 45tk │     │
│   │ [2] Verbatim: "Meeting 04/15 - DB discussion..."         120tk│     │
│   │ [3] Header: "Sprint 5 deadline"                           5tk │     │
│   │ ...                                                           │     │
│   │ Total: 3987/4000 tokens (99.7% utilization)                   │     │
│   └───────────────────────────────────────────────────────────────┘     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### The CBA Composite Score

For each candidate memory, MIRA calculates a **multidimensional score**:

```
┌─────────────────────────────────────────────────────────────────────┐
│                     CBA SCORE FORMULA                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   S(m) = ρ × δ × η × (1-σ) × τ × χ × 𝟙[ρ>θ]                         │
│                                                                     │
│   where:                                                            │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━    │
│   ρ (rho)    = Semantic Relevance      cos(embedding_m, embedding_q)│
│   δ (delta)  = Information Density     sigmoid(facts/√tokens)       │
│   η (eta)    = Temporal Weight         exp(-λ × age)                │
│   σ (sigma)  = Max Overlap             sim(m, already_selected)     │
│   τ (tau)    = Session Boost           +20% if same session         │
│   χ (chi)    = Causal Penalty          avoids long chains           │
│   𝟙[ρ>θ]     = Relevance Threshold     eliminates if ρ < 0.6        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3-Level Architecture (T0/T1/T2)

### Why 3 Levels?

The human brain doesn't record everything with the same fidelity. MIRA mimics this hierarchy:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        T0/T1/T2 HIERARCHY                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   LEVEL T0 - VERBATIM (Episodic Memory)                             │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   "Meeting April 15, 2024 at 14:30.                                 │
│    Participants: Marie (Tech Lead), Jean (DevOps), Sophie (PO)      │
│    Marie: 'I propose we migrate to PostgreSQL for v2'               │
│    Jean: 'It requires training, but it's more robust'               │
│    Sophie: 'Client approves for Sprint 5'                           │
│    Final decision: PostgreSQL migration approved"                   │
│                                                                     │
│    • Storage: Full UTF-8 text (max 64KB)                            │
│    • Usage: Rich context when budget allows                         │
│    • Cost: ~200 tokens                                              │
│                                                                     │
│                              ↓ NLP Extraction                       │
│                                                                     │
│   LEVEL T1 - FINGERPRINT (Semantic Memory)                          │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   {                                                                 │
│     "type": "decision",                                             │
│     "decision": "PostgreSQL Migration",                             │
│     "rejected": ["MySQL", "MongoDB"],                               │
│     "reason": ["ACID Robustness", "Client validation"],             │
│     "assignee": "Jean",                                             │
│     "deadline": "Sprint 5",                                         │
│     "validated_by": "Sophie (PO)"                                   │
│   }                                                                 │
│                                                                     │
│    • Storage: Structured canonical JSON                             │
│    • Usage: Dense context when budget is medium                     │
│    • Cost: ~30 tokens (15% of T0)                                   │
│                                                                     │
│                              ↓ Embedding                            │
│                                                                     │
│   LEVEL T2 - EMBEDDING (Procedural Memory)                          │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   [0.23, -0.15, 0.89, -0.42, 0.67, ...]  // 384 dimensions          │
│                                                                     │
│    • Storage: float32[384] vector                                   │
│    • Usage: O(log n) vector search                                  │
│    • Cost: 0 tokens (search only)                                   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Memory Types and Decay

| Type           | λ (day⁻¹) | Half-life | Auto-Archive | Usage                   |
| -------------- | --------- | --------- | ------------ | ----------------------- |
| `decision`     | 0.001     | ~693 days | No           | Architectural decisions  |
| `fact`         | 0.005     | ~139 days | No           | Knowledge, facts         |
| `preference`   | 0.01      | ~69 days  | No           | User preferences         |
| `session_note` | 0.1       | ~7 days   | 30 days      | Session notes           |
| `debug_log`    | 0.5       | ~1.4 days | 7 days       | Debug logs              |

---

## The CBA Algorithm

### Context Budget Allocator v2

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CBA ALGORITHM - O(n log n)                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  INPUT:  Query q, Budget B (tokens), Wing w, Room r                 │
│  OUTPUT: List of memories with render mode                          │
│                                                                     │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│                                                                     │
│  1. EMBEDDING                                                       │
│     e_q ← Embed(q) with LRU cache (1000 entries)                    │
│                                                                     │
│  2. VECTOR SEARCH                                                   │
│     C ← HNSW_Search(e_q, N=100, w, r)        # O(log n)             │
│     If HNSW not ready: C ← SQLite_Search(e_q, N=100)  # Fallback    │
│                                                                     │
│  3. EARLY PRUNING                                                   │
│     C' ← {c ∈ C : ρ(c,q) > 0.6}                                     │
│     If C' = ∅: C' ← top-5(C) by ρ                                  │
│                                                                     │
│  4. INITIAL SCORING                                                 │
│     For each c ∈ C':                                                │
│        c.score ← ρ(c) × δ_sigmoid(c) × η_recency(c)                 │
│                                                                     │
│  5. GREEDY SELECTION WITH DYNAMIC RENORMALIZATION                   │
│     S ← ∅, tokens_used ← 0                                         │
│     PQ ← MaxHeap(C')  # by initial score                            │
│                                                                     │
│     WHILE PQ ≠ ∅ AND tokens_used < B:                              │
│        c ← Pop(PQ)                                                  │
│                                                                     │
│        # Dynamic recalculation (depends on already selected S)      │
│        c.σ_max ← max_{s∈S} similarity(c,s)                          │
│        c.χ ← exp(-0.15 × |causal_links(c,S)|)                       │
│        c.τ ← 1.2 if |time(c) - time(S)| < 2h else 1.0               │
│                                                                     │
│        adjusted_score ← c.score × (1-c.σ_max) × c.χ × c.τ           │
│                                                                     │
│        # Check if next has better score                             │
│        If PQ[0].score × 0.8 > adjusted_score:                       │
│           Push(PQ, c) with adjusted_score                           │
│           continue                                                  │
│                                                                     │
│        # Determine mode based on REMAINING BUDGET                   │
│        remaining ← B - tokens_used                                  │
│        mode ← ChooseMode(c, remaining)                              │
│        cost ← CalculateCost(c, mode)                                │
│                                                                     │
│        # Downgrade if necessary                                     │
│        If tokens_used + cost > B:                                   │
│           mode ← Downgrade(mode)  # Verbatim → Fingerprint → Header │
│           cost ← Recalculate(mode)                                  │
│           If tokens_used + cost > B: continue                       │
│                                                                     │
│        S ← S ∪ {c}, tokens_used ← tokens_used + cost                │
│                                                                     │
│  6. RETURN S sorted by descending score                             │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Adaptive Render Modes

| Remaining Budget | Mode        | Tokens | Content              |
| ---------------- | ----------- | ------ | -------------------- |
| < 100            | Header      | 2-5    | `[type\|date\|wing]` |
| < 1000           | Fingerprint | ~15%   | Essential facts T1   |
| ≥ 1000           | Verbatim    | 100%   | Original text T0     |

---

## Enhanced Recall Pipeline

MIRA's recall uses a **multi-stage retrieval pipeline** that goes far beyond simple vector similarity:

```
Query → Expansion → Dense (HNSW) + Lexical (FTS5) → RRF Fusion → Clustering → Tag Boost → Adaptive Threshold → CBA Greedy Selection
```

### 1. Query Expansion
Before embedding, MIRA generates semantically-close variants of the query (cleaned, without stop-words, top keywords) and **averages their embeddings**. This improves cross-lingual retrieval and robustness against vocabulary mismatch.

### 2. Hybrid Search (Dense + Lexical)
- **Dense**: HNSW O(log n) vector search
- **Lexical**: SQLite FTS5 full-text search (auto-enabled if available)
- **Fusion**: Reciprocal Rank Fusion (`k=60`) merges both rankings into a single candidate list

### 3. Search-Time Clustering
Candidates are grouped by cosine similarity ≥ 0.88. Near-duplicates are collapsed, and only the best representative per cluster proceeds to scoring. This prevents budget waste on redundant memories.

### 4. Tag-Based Retrieval
A new `memory_tags` table indexes extracted entities, subjects, and keywords. Candidates matching query tags receive a small additive relevance boost.

### 5. Adaptive Threshold Methods
Instead of a fixed 0.6 relevance floor, MIRA now supports three dynamic methods:

| Method | Description | Default |
|--------|-------------|---------|
| `iqr` | First quartile of score distribution | Yes |
| `elbow` | Largest derivative drop (elbow method) | |
| `mean_stddev` | mean - stddev | |

The threshold is clamped between `0.15` (floor) and `0.75` (ceiling).

### 6. Heuristic Reranker (Optional)
A lightweight pure-Go reranker scores top-k candidates using:
- Jaccard-like token overlap
- Exact phrase presence bonus
- Length balance preference

Blended with semantic relevance: `0.7*semantic + 0.3*rerank`.

### 7. Fallback Vector Store
If HNSW is not yet ready (e.g., building from scratch), a transparent fallback wrapper automatically routes searches to the SQLite vector store. Recall never fails.

---

## Causal Graph

### Supported Relations

```
┌─────────────────────────────────────────────────────────────────────┐
│                      CAUSAL RELATIONS                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   BECAUSE                    A ←────────── B                        │
│   "B explains why A"         Bug understood  Because we analyzed    │
│                              ───────────→   the logs                │
│                                                                     │
│   TRIGGERED                  A ←────────── B                        │
│   "B triggered A"            Migration    After the decision        │
│                              ───────────→  meeting                  │
│                                                                     │
│   CONTRADICTS                A ←────────→ B                         │
│   "A and B contradict"       Option A     Option B                  │
│                              ───────────→  incompatible             │
│                                                                     │
│   UPDATES                    A ←────────── B                        │
│   "B replaces/updates A"     Spec v1      Spec v2                   │
│                              ───────────→  (new version)            │
│                                                                     │
│   RESOLVES                   A ←────────── B                        │
│   "B resolves problem A"     Bug #123     Fix #124                  │
│                              ───────────→  (correction)             │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Automatic Detection

Relations are automatically detected via linguistic patterns:

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

- Go 1.23+ (if building from source)
- SQLite3 (included)
- ~100MB disk space for embedding model

### From Sources

```bash
# Clone the repository
git clone https://github.com/benoitpetit/mira.git
cd mira

# Build
go build -o mira ./cmd/mira

# Verify
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

### 1. Initialization

```bash
# Copy example configuration
cp config.example.yaml config.yaml

# Edit to your needs
nano config.yaml
```

### 2. Start the MCP Server

```bash
# stdio mode (for Claude Desktop, Cursor, etc.)
./mira

# With custom config file
./mira -config ./config.yaml

# Run database migrations only
./mira -migrate
```

### 3. Use MCP Tools

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
[full content]

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

### config.yaml File

```yaml
system:
  version: "0.4.4"

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

# HNSW Vector Index Configuration
hnsw:
  M: 32 # Max neighbors per node (tuned for better recall, see v0.4.2)
  Ml: 0.25 # Level generation factor
  ef_construction: 0 # Inactive — not supported by underlying hnsw library
  ef_search: 100 # Dynamic candidate list for search (tuned, see v0.4.2)

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

# Enhanced recall configuration
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
  version: "0.4.4"
  transport: "stdio"
  timeout_seconds: 30

# Prometheus metrics export
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

| Tool                | Description                           |
| ------------------- | ------------------------------------- |
| `mira_store`        | Store memory with T0→T1,T2 extraction |
| `mira_recall`       | Retrieve optimal context with budget  |
| `mira_load`         | Load full verbatim by ID              |
| `mira_causal_chain` | Trace causal chain                    |
| `mira_status`       | System statistics and health          |
| `mira_timeline`     | Filtered chronological reconstruction |
| `mira_archive`      | Archive and clean old memories        |
| `mira_clear_memory` | Permanently delete all or room-scoped memories |

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

If the primary wing has no matching memories, MIRA will automatically search the fallback wings in order.

### Multilingual & Broad Search

`mira_recall` supports queries in any language (English, French, Spanish, Italian, German, etc.) thanks to cross-lingual embeddings. If the initial semantic search yields sparse results — for example, when a query in one language searches against memories stored in another — MIRA automatically broadens the search with relaxed thresholds and merges the results. You do not need to translate queries or adjust parameters.

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

See [API_REFERENCES.md](docs/API_REFERENCES.md) for detailed API reference and usage examples.

### Health Check Endpoints

When metrics are enabled, MIRA exposes health endpoints:

```bash
# Full health check (includes DB, Vector Store, Embedder)
curl http://localhost:9090/health

# Liveness probe (Kubernetes)
curl http://localhost:9090/health/live

# Readiness probe (Kubernetes)
curl http://localhost:9090/health/ready

# Prometheus metrics
curl http://localhost:9090/metrics
```

---

## Performance

### Algorithmic Complexities

| Operation        | Complexity | Notes            |
| ---------------- | ---------- | ---------------- |
| Store T0,T1,T2   | O(1)       | Atomic insertion |
| Vector Search    | O(log n)   | HNSW ANN         |
| CBA Scoring      | O(n)       | n = candidates   |
| Allocation       | O(n log n) | Max-heap         |
| Causal Graph BFS | O(V+E)     | V=nodes, E=edges |

### Real-World Performance

| Metric            | Value                 |
| ----------------- | --------------------- |
| HNSW Search       | ~0.14 ms for 10K vectors (O(log n), ~0.5 ms est. at 100K) |
| SQLite Search     | ~50 ms for 10K vectors |
| Full Allocation   | ~35 ms for 100 candidates |
| Cosine Similarity | ~3.3M ops/sec         |

### Optimizations in v0.3.3

- **Query Expansion**: Multi-variant embedding averaging for robust cross-lingual retrieval
- **FTS5 Lexical Search**: SQLite full-text search integrated with auto-triggers and backfill
- **RRF Hybrid Fusion**: Reciprocal Rank Fusion (`k=60`) combining dense HNSW and lexical FTS5 results
- **Search-Time Clustering**: Real-time deduplication with cosine-similarity clustering (threshold 0.88)
- **Tag-Based Retrieval**: `memory_tags` table with automatic tag boosting in CBA scoring
- **Heuristic Reranker**: Optional lightweight lexical reranker for precision improvement
- **Adaptive Threshold Methods**: Dynamic relevance pruning with `iqr`, `elbow`, and `mean_stddev` strategies
- **Fallback Vector Store**: Transparent HNSW → SQLite fallback when the index is not ready
- **Clear Memory Tool**: New `mira_clear_memory` MCP tool for global or room-scoped memory deletion
- **Causal Chain T0 Resolution**: `mira_causal_chain` now correctly resolves `T0:` verbatim references to fingerprint IDs
- **ID Visibility in Outputs**: `mira_recall` and `mira_timeline` now include memory IDs for downstream tool chaining
- **LLM-Self-Correction Errors**: Invalid ID errors are now actionable, telling LLMs exactly where to get valid IDs

### Optimizations in v0.3.1

- **Lazy Evaluation**: Overlap calculation only for promising candidates
- **LRU Cache**: 1000 entries for query embeddings
- **HNSW Persistence**: Fast index reload on restart
- **SQLite WAL Mode**: Concurrent read/write performance
- **Adaptive Threshold**: Lowered relevance threshold for small corpora (<10 memories)
- **Default Room Mapping**: Auto-assigns standard rooms based on memory type

---

## Technical Architecture

### Clean Architecture (Uncle Bob)

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
│   │  • storage: SQLiteRepository                                │   │
│   │  • vector: HNSWStore, SQLiteVectorStore                     │   │
│   │  • extraction: NativeExtractor, CybertronEmbedder           │   │
│   │  • webhook, metrics                                         │   │
│   │  ✓ Implements ports                                         │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  FRAMEWORKS & DRIVERS                                       │   │
│   │  • SQLite3, HNSW lib, Cybertron, MCP Server                 │   │
│   │  ✓ External technical details                               │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Project Structure

```
mira/
├── cmd/mira/              # Entry point
├── internal/
│   ├── domain/            # Domain Layer
│   │   ├── entities/      # Business entities
│   │   └── valueobjects/  # Value objects
│   ├── usecases/          # Use Cases Layer
│   │   ├── ports/         # Interfaces (Repository, Services)
│   │   └── interactors/   # Use case implementations
│   ├── adapters/          # Adapters Layer
│   │   ├── storage/       # SQLite repository
│   │   ├── vector/        # HNSW, SQLite vector store, overlap cache
│   │   ├── extraction/    # NLP, embeddings
│   │   ├── logging/       # Structured logging
│   │   ├── webhook/       # HTTP notifications
│   │   └── metrics/       # Prometheus metrics
│   ├── interfaces/        # Interfaces Layer
│   │   └── mcp/           # MCP controller
│   ├── config/            # Configuration
│   └── app/               # Composition root (DI)
│       ├── main.go        # Dependency injection
│       ├── health.go      # Health checks
│       ├── health_test.go # Health check tests
│       └── main_test.go   # Application tests
├── docs/                  # Documentation
│   ├── INDEX.md           # Documentation entry point
│   ├── ARCHITECTURE.md    # Technical deep-dive
│   ├── FEATURES.md        # Complete feature catalog
│   └── API_REFERENCES.md  # API reference
├── SKILL.md               # Agent skill and memory loop guidelines
├── config.example.yaml    # Example configuration
└── README.md              # This file
```

---

## Development

### Testing

```bash
# Unit tests
go test -v ./...

# With race detector
go test -race ./...

# Benchmarks
go test -bench=. -benchmem ./...

# Coverage
go test -cover ./...
```

### Make Commands

```bash
make build       # Build
make test        # Tests (with race detector)
make test-short  # Quick tests
make bench       # Benchmarks
make bench-full  # Full benchmarks
make run         # Build and run with config.yaml
make clean       # Clean build artifacts and data
make lint        # Run linters
make fmt         # Format code
make install     # Install to GOPATH/bin
make prepublish VERSION=x.y.z  # Prepare a release
```

## Changelog

### v0.4.4 (2026-04-23)

- **SOUL opt-in integration**: MIRA now runs standalone (8 tools) by default. SOUL identity extension must be explicitly enabled via `--with-soul` CLI flag or `soul.enabled: true` in config.
- SOUL init failures are non-fatal — MIRA gracefully falls back to 8-tool mode.

### v0.4.3 (2026-04-23)

- **Fixed SOUL MCP parameter names**: `agent` → `agent_id`, `model` → `model_id`, `from` → `from_model`, `to` → `to_model`.

### v0.4.2 (2026-04-17)

- **HNSW tuned defaults**: `M` 16 → 32, `ef_search` 50 → 100 for better recall.
- **Concurrent embedding pool**: Replaced global mutex with model instance pool.
- **Parallel recall pipeline**: Dense HNSW + lexical FTS5 now execute concurrently.
- `ef_construction` documented as inactive (not supported by underlying library).

See [CHANGELOG.md](CHANGELOG.md) for the full release history.

---

## References

### Key Libraries

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - OpenAI tokenization
- Native Go implementation - Rule-based NLP/NER (replaces archived prose)
- [cybertron](https://github.com/nlpodyssey/cybertron) - Transformer embeddings
- [hnsw](https://github.com/coder/hnsw) - HNSW graphs
- [mcp-go](https://github.com/mark3labs/mcp-go) - MCP protocol

### Embedding Model

- **Model:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Size:** ~80MB
- **Performance:** ~1000 texts/sec on CPU

---

<div align="center">

**MIRA** - _Memory with Information-theoretic Relevance Allocation_

_"Memory is the sap of artificial intelligence."_

[API Reference](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md)

</div>

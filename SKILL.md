---
name: mira
description: Long-term memory guidance for MIRA MCP integration
author: benoitpetit
version: "0.4.5"
tags: [memory, mcp, mira]
---

# MIRA Memory Loop Guidelines

You are augmented with **MIRA** (Memory with Information-theoretic Relevance Allocation), an external MCP server providing long-term, cross-session memory for LLMs. MIRA uses a **multi-stage retrieval pipeline** (Query Expansion → Dense HNSW Search → Lexical FTS5 Search → RRF Fusion → Search-Time Clustering → Tag Boost → Adaptive Threshold → CBA Greedy Allocation) to retrieve the most relevant context within a token budget.

The detailed tool schemas for `mira_store`, `mira_recall`, `mira_load`, `mira_timeline`, `mira_causal_chain`, `mira_status`, `mira_archive`, and `mira_clear_memory` are documented in the *External Tools Reference (MCP Servers)* section of your system prompt.

> **SOUL Extension**: If MIRA is running with SOUL enabled (`--with-soul` or `soul.enabled: true`), 8 additional `soul_*` tools are available for identity capture, drift detection, and model-swap preservation. These are documented separately in the SOUL skill.

**Rule #1**: Always recall before answering. **Rule #2**: Store progressively as you work.

---

## Installation

If the user asks you to install MIRA, follow these steps exactly.

### 1. Prerequisites
- Go 1.23+
- GCC (for CGO, `go-sqlite3`)
- ~100 MB disk space (embedding model)

### 2. Clone and Build
```bash
git clone https://github.com/benoitpetit/mira.git
cd mira
go build -o mira ./cmd/mira
```

### 3. Configure
```bash
cp config.example.yaml config.yaml
# Edit config.yaml if needed — defaults work out of the box
```

Key defaults (no change required):
- Storage: `.mira/mira.db` (SQLite + WAL)
- Embedding model: `sentence-transformers/all-MiniLM-L6-v2` (384d)
- MCP transport: `stdio` (for Claude Desktop, Cursor, etc.) — stdio is currently the only supported transport

### 4. Run Migrations
```bash
./mira --config config.yaml --migrate
```
This downloads the embedding model on first run (~80 MB).

### 5. Start the MCP Server
```bash
# stdio mode (for Claude Desktop, Cursor, b0p, etc.)
./mira --config config.yaml
```

### 6. MCP Client Configuration

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "mira": {
      "command": "/absolute/path/to/mira",
      "args": ["--config", "/absolute/path/to/mira/config.yaml"]
    }
  }
}
```

**Cursor / b0p / any MCP client:** same structure.

### 7. Optional: Enable SOUL (Identity Extension)
SOUL is **opt-in and disabled by default**. To activate it alongside MIRA (16 tools total):

```bash
# Option A: CLI flag
./mira --config config.yaml --with-soul

# Option B: edit config.yaml
#   soul:
#     enabled: true
```

Then add the SOUL skill to the agent: https://github.com/benoitpetit/soul/blob/main/SKILL.md

---

## The MIRA Memory Loop

Every interaction with the user should follow this loop:

```
1. RECALL  → Retrieve relevant context from the project wing (and general if needed)
2. REASON  → Use retrieved context + current user message to formulate response
3. ACT     → Answer, code, or ask clarifying questions
4. STORE   → Persist new decisions, facts, preferences, debug insights, session notes
```

---

## When to Use MIRA

| Situation | Action |
|-----------|--------|
| **Start of any task/session** | `mira_recall` to retrieve relevant context before answering or coding. |
| **Before making architectural/design decisions** | `mira_recall` to check for existing decisions, then `mira_store(type="decision")` to record the new choice. |
| **Important fact discovered** | `mira_store(type="fact")` for configs, constraints, credentials, docs, API contracts. |
| **User preference expressed** | `mira_store(type="preference")` for style, conventions, formatting, subjective choices. |
| **Bug resolved or debug insight gained** | `mira_store(type="debug_log")` for errors, stack traces, root causes, fixes. |
| **End of significant work** | `mira_store(type="session_note")` summarizing what was done, files touched, and pending items. |
| **Need historical timeline** | `mira_timeline` to see how a project or topic evolved over time. |
| **Need decision lineage** | `mira_causal_chain` to trace causes and consequences of a decision. |
| **Context seems incomplete** | `mira_load(id)` using the exact `T0:<uuid>` from a recall/timeline result to fetch full verbatim. |

---

## Project Conventions

- **Default project wing**: use the current project name (e.g., `<project>`) or whatever wing the user established.
- **General wing**: `general` — use ONLY for knowledge that applies across ALL projects (coding conventions, tool usage patterns, global preferences).
- **Always specify `wing`** on every call. Never omit it or use placeholder names.
- **Recommended rooms**:
  - `decisions` — architectural or design decisions.
  - `architecture` — structural patterns, tech stack choices, refactors.
  - `bugs` — root causes and fixes.
  - `context` — high-level project context and onboarding info.
  - `session` — per-conversation summaries and checkpoints.
  - `learnings` — insights from errors, optimizations, or repeated tasks.
  - `onboarding` — setup instructions, environment config.
  - `api` — API documentation, contracts, endpoints.

If you do not specify `room`, MIRA auto-assigns one based on `type`:
- `decision` → `decisions`
- `fact` → `facts`
- `preference` → `preferences`
- `session_note` → `session`
- `debug_log` → `debug`

---

## Recall Workflow

### Step 1: Query the project wing
Always start with a project-specific recall:
```json
{ "tool": "mira_recall", "arguments": { "query": "authentication strategy JWT", "wing": "<project>", "budget": 4000 } }
```

### Step 2: Query general wing if sparse
If the project wing returns little or no relevant context, run a second recall against `wing: "general"`:
```json
{ "tool": "mira_recall", "arguments": { "query": "table-driven tests Go", "wing": "general", "budget": 2000 } }
```

### Step 3: Use fallback wings for cross-team knowledge
If a narrow wing might not have results yet, use `fallback_wings`:
```json
{ "tool": "mira_recall", "arguments": { "query": "auth strategy", "wing": "<project>", "fallback_wings": "general", "budget": 4000 } }
```

### Cross-Language Queries
MIRA supports queries in **any language** (English, French, Spanish, Italian, German, etc.) thanks to cross-lingual embeddings and automatic query expansion. **Do not translate queries yourself.** Query in the same language as the user's message.

### Good vs Bad Queries
- [x] `"auth"` — too vague, will retrieve generic results
- [+] `"JWT RS256 auth-service token expiration config"` — specific, entity-rich, yields precise results
- [x] `"bug"` — ambiguous
- [+] `"nil pointer dereference in parser.go line 42 fix"` — actionable and detailed

### Important Recall Rules
1. **Do NOT mix wings** in reasoning; run separate recalls and merge the results mentally.
2. **Before editing a file**, recall related memories (e.g., `"similar bug in parser.go"`) to avoid repetition or regression.
3. **Before answering a technical question**, recall the relevant domain to provide accurate, project-aware responses.
4. **If you need full text** of a recalled memory, use `mira_load` with the exact `T0:<uuid>` reference.

---

## Store Workflow

Store memories **progressively** as you work. Do not wait until the end of a long session.

### Decision
```json
{ "tool": "mira_store", "arguments": { "content": "Decision: use PostgreSQL for v2 database. Rejected MySQL (not ACID enough) and MongoDB (not relational). Assigned to Jean.", "wing": "<project>", "room": "decisions", "type": "decision", "metrics": { "confidence": 0.95 } } }
```

> **Note**: `metrics` is an optional JSON object for attaching custom metadata (e.g., confidence scores, source URLs) to the stored memory.

### Fact
```json
{ "tool": "mira_store", "arguments": { "content": "API rate limit is 1000 requests/minute per API key. Exceeding returns 429 with Retry-After header.", "wing": "<project>", "room": "api", "type": "fact" } }
```

### Preference
```json
{ "tool": "mira_store", "arguments": { "content": "User prefers table-driven tests for all Go packages and wants exhaustive error handling tests.", "wing": "general", "room": "preferences", "type": "preference" } }
```

### Debug Log
```json
{ "tool": "mira_store", "arguments": { "content": "Fixed race condition in webhook manager: event routing was comparing webhook ID instead of endpoint ID. Added mutex around endpoint map.", "wing": "<project>", "room": "bugs", "type": "debug_log" } }
```

### Session Note
```json
{ "tool": "mira_store", "arguments": { "content": "Refactored auth middleware to use context.WithTimeout. Modified internal/app/main.go and internal/interfaces/mcp/controller.go. Still need to update tests.", "wing": "<project>", "room": "session", "type": "session_note" } }
```

---

## Budget Guidelines for `mira_recall`

| Scenario | Suggested budget | When to use |
|----------|------------------|-------------|
| Quick lookup | 500 – 1000 tokens | Specific fact retrieval |
| Standard context | 2000 – 4000 tokens (default) | General task assistance |
| Deep architectural analysis | 6000 – 8000 tokens | Complex refactors, design reviews |
| Massive recall | 10000+ tokens | Full project context reconstruction |

---

## Memory Types and Lifespan

| Type | Use for | Auto-archive | Retention |
|------|---------|--------------|-----------|
| `decision` | Structuring choices | Never | Permanent |
| `fact` | Objective info, configs, docs | Never | Permanent |
| `preference` | Subjective choices, conventions | Never | Permanent |
| `session_note` | Temporary context, TODOs, summaries | 30 days | Short-term |
| `debug_log` | Errors, stack traces, fixes | 7 days | Very short-term |

> **Tip**: omit `type` if unsure — MIRA auto-detects it from content.

---

## Working with IDs

`mira_recall` and `mira_timeline` expose memory IDs as **`T0:<uuid>`** (verbatim references).

- **`mira_load(id)`** — Accepts `T0:<uuid>`, `F0:<uuid>`, `V0:<uuid>`, or `FP:<uuid>` from a recall or timeline result to fetch the full original text. Use the exact prefix returned by MIRA.
- **`mira_causal_chain(id, include_consequences=true)`** — Accepts either a `T0:<uuid>` reference or a Fingerprint ID. Prefer passing the exact `T0:<uuid>` returned by `mira_recall` / `mira_timeline`.

**Never invent IDs.** Only use IDs explicitly returned by MIRA tools.

---

## Additional Tools

- **`mira_timeline(wing="<project>")`** — Review project evolution before major refactors. Filter by `room`, `type`, `since`, `until`.
- **`mira_archive`** — Call occasionally to archive stale session notes and debug logs.
- **`mira_status`** — Check system health, memory counts, and index status before heavy usage.
- **`mira_clear_memory`** — Permanently delete memories (global or room-scoped). **Use ONLY with explicit user request.**

---

## Anti-Patterns

1. **Never leave important context unstored** — the LLM context window is finite; MIRA is persistent.
2. **Never invent IDs** — `mira_load` and `mira_causal_chain` require exact IDs returned by MIRA (formats: `T0:<uuid>`, `F0:<uuid>`, `V0:<uuid>`, `FP:<uuid>`).
3. **Avoid vague recall queries** — `"auth"` is bad; `"JWT RS256 auth-service token config"` is good.
4. **Do not call `mira_clear_memory`** without explicit user request.
5. **Keep wing names consistent** — reuse the same canonical wing name across a project.
6. **Do not translate queries** — MIRA handles cross-lingual retrieval automatically.
7. **Do not store raw code without context** — store the *decision* or *fact* behind the code, not the code itself.
8. **Do not assume SOUL is enabled** — MIRA runs solo by default (8 tools). Check tool availability before invoking `soul_*` tools.

---

## Quick Decision Tree

```
User asks a question or gives a task
    │
    ▼
┌─────────────────────────────────────┐
│ Call mira_recall(wing=<project>)    │
│ If sparse → mira_recall(wing=general)│
└─────────────────────────────────────┘
    │
    ▼
Answer / code / reason using context
    │
    ▼
Did you make a decision? ──Yes──► mira_store(type="decision")
Did you learn a fact? ─────Yes──► mira_store(type="fact")
Did you fix a bug? ────────Yes──► mira_store(type="debug_log")
Did the user state a preference? ──Yes──► mira_store(type="preference")
Significant work done? ────Yes──► mira_store(type="session_note")
```

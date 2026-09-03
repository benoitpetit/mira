---
name: mira
description: Long-term memory guidance for MIRA MCP integration
author: benoitpetit
version: "0.5.0"
tags: [memory, mcp, mira]
---

# MIRA Memory Loop Guidelines

You are augmented with **MIRA** (Memory with Information-theoretic Relevance Allocation), an external MCP server providing long-term, cross-session memory for LLMs. MIRA uses a **multi-stage retrieval pipeline** (Query Expansion → Dense HNSW Search → Lexical FTS5 Search → RRF Fusion → Search-Time Clustering → Tag Boost → Adaptive Threshold → CBA Greedy Allocation) to retrieve the most relevant context within a token budget.

The detailed tool schemas for `mira_store`, `mira_ingest`, `mira_recall`, `mira_load`, `mira_update`, `mira_search`, `mira_consolidate`, `mira_causal_chain`, `mira_status`, `mira_health`, `mira_archive`, `mira_compress`, `mira_timeline`, and `mira_clear_memory` are documented in the *External Tools Reference (MCP Servers)* section of your system prompt.

> **SOUL Extension**: If MIRA is running with SOUL enabled (`--with-soul` or `soul.enabled: true`), 8 additional `soul_*` tools are available for identity capture, drift detection, and model-swap preservation. These are documented separately in the SOUL skill. When SOUL is enabled, MIRA provides **14 + 8 = 22 tools total**.

**Rule #1**: Always recall before answering. **Rule #2**: Store progressively as you work.

MIRA provides **14 MCP tools** by default:
- `mira_store` — Store memories with T0/T1/T2 extraction
- `mira_recall` — Context-aware retrieval with token budget (CBA)
- `mira_load` — Load full verbatim by ID
- `mira_update` — Update memory content with re-extraction
- `mira_search` — Pure vector search without CBA
- `mira_consolidate` — Merge redundant session notes into synthesized facts
- `mira_causal_chain` — Trace causal chains
- `mira_timeline` — Chronological reconstruction
- `mira_status` — Full system statistics
- `mira_health` — Quick JSON health check
- `mira_archive` — Clean old memories
- `mira_clear_memory` — Delete memories
- `mira_compress` — Rule-based context compression
- `mira_ingest` — Extract history memories from structured conversation messages

---

## Installation

If the user asks you to install MIRA, follow these steps exactly.

### 1. Prerequisites
- Go 1.23+
- A CGO C toolchain and OpenSSL development headers (`libssl-dev` on Debian/Ubuntu)
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
- MCP transport: `stdio` (for Claude Desktop, Cursor, etc.), `sse` for
  Server-Sent Events, or stateless `http` at `POST /mcp`

### 4. Run Migrations
```bash
./mira migrate
```
This downloads the embedding model on first run (~80 MB).

### 5. Start the MCP Server
```bash
# stdio mode (for Claude Desktop, Cursor, b0p, etc.)
./mira --config config.yaml server

# sse mode (for remote clients or web UIs)
./mira --config config.yaml server
# With mcp.transport: "sse" and mcp.address: "localhost:3001" in config.yaml

# stateless JSON-RPC HTTP mode (POST requests to /mcp)
./mira --config config.yaml server
# With mcp.transport: "http" and mcp.address: "localhost:3001" in config.yaml
```

### 6. MCP Client Configuration

**Claude Code** (official CLI, private to the current project by default):
```bash
./mira setup --client claude-code --scope local
```

Use `--scope project` to share the server through the project `.mcp.json`, or
`--scope user` to make it available in every local project.

To capture substantive user prompts automatically through Claude Code's local
`UserPromptSubmit` hook, opt in explicitly:

```bash
./mira setup --client claude-code --automatic-memory --memory-wing my-project
```

To also store the final response supplied by Claude Code's `Stop` event, opt in
explicitly:

```bash
./mira setup --client claude-code --automatic-memory --include-assistant --memory-wing my-project
```

The hooks store `history` memories and never write successful output into the
Claude conversation.

**Codex** (`~/.codex/config.toml`, or `.codex/config.toml` for one trusted project):
```toml
[mcp_servers.mira]
command = "/absolute/path/to/mira"
args = ["--config", "/absolute/path/to/mira/config.yaml", "server"]
```

Or let MIRA call the official Codex CLI for you after `mira init`:

```bash
./mira setup --client codex
```

To capture substantive user prompts automatically, opt in to the Codex
`UserPromptSubmit` hook:

```bash
./mira setup --client codex --automatic-memory --memory-wing my-project
```

To additionally store the final response supplied by Codex's `Stop` event:

```bash
./mira setup --client codex --automatic-memory --include-assistant --memory-wing my-project
```

The hooks are merged into Codex's user hook file. Codex keeps its normal hook
trust boundary, so approve the newly detected hooks when prompted.

**Cursor** (project `.cursor/mcp.json`):
```bash
./mira setup --client cursor
```

Use `--dry-run` to inspect the JSON MIRA will merge. An existing different
`mira` entry is never replaced unless `--force` is supplied.

**Windsurf** (user `~/.codeium/windsurf/mcp_config.json`):
```bash
./mira setup --client windsurf
```

Use `--dry-run` to inspect the merged configuration first. Existing non-MIRA
servers are preserved and a different MIRA entry requires `--force`.

Windsurf also supports native Cascade hooks. Enable automatic user-prompt
capture with:

```bash
./mira setup --client windsurf --automatic-memory --memory-wing my-project
```

To additionally store completed Cascade responses, opt in explicitly:

```bash
./mira setup --client windsurf --automatic-memory --include-assistant --memory-wing my-project
```

**Claude Desktop** (macOS or Windows):
```bash
./mira setup --client claude-desktop
```

MIRA merges its entry into the documented Claude Desktop configuration location
and preserves other servers. On a platform without a documented default path,
pass its path explicitly: `--client-config /path/to/claude_desktop_config.json`.
Use `--dry-run` to preview and fully quit/restart Claude Desktop after setup.

### Conversation ingestion stream

When a local client hook or exporter emits one JSON object per line with
`role` and `content`, pipe it into MIRA for immediate, controlled extraction:

```bash
conversation-hook --jsonl | ./mira ingest --stream --wing my-project
```

MIRA stores substantive user messages by default; add `--include-assistant`
only when assistant replies should become `history` memories.

### Cursor CLI stream

Cursor does not expose a documented IDE lifecycle hook, but its CLI can emit
structured `stream-json` events that MIRA consumes directly:

```bash
cursor-agent --output-format stream-json "Summarize the current task" | \
  ./mira ingest --stream --wing my-project --include-assistant
```

Tool-call and partial assistant-delta events are ignored; MIRA stores only the
complete user event and terminal result.

### 7. Optional: Enable SOUL (Identity Extension)
SOUL is **opt-in and disabled by default**. To activate it alongside MIRA (22 tools total: 14 MIRA + 8 SOUL):

```bash
# Option A: CLI flag
./mira --config config.yaml server --with-soul

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
| **Before making architectural/design decisions** | `mira_recall` to check for existing decisions, then `mira_store(type="decision", kind="project")` to record the new choice. |
| **Important fact discovered** | `mira_store(type="fact", kind="knowledge")` for configs, constraints, credentials, docs, API contracts. |
| **User preference expressed** | `mira_store(type="preference", kind="user")` for style, conventions, formatting, subjective choices. |
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

For multi-turn conversations, use `session_id` to boost memories selected in previous turns:
```json
{ "tool": "mira_recall", "arguments": { "query": "auth strategy", "wing": "<project>", "session_id": "session-abc-123", "budget": 4000 } }
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

`kind` is separate from `type`: use `identity`, `user`, `project`, `task`,
`knowledge`, or `history` to describe the memory's role and filter recall. If
omitted, MIRA assigns a default from the detected type.

### Importing a Conversation

For a portable JSON chat export, run `mira ingest --file conversation.json
--wing <project> --dry-run` first. The command captures substantive user
messages as `history` memories and applies normal T0/T1/T2 extraction; add
`--include-assistant` only when assistant replies contain durable facts or
decisions worth retaining.

---

## Working with IDs

`mira_recall` and `mira_timeline` expose memory IDs as **`T0:<uuid>`** (verbatim references).

- **`mira_load(id)`** — Accepts `T0:<uuid>`, `F0:<uuid>`, `V0:<uuid>`, or `FP:<uuid>` from a recall or timeline result to fetch the full original text. Use the exact prefix returned by MIRA.
- **`mira_causal_chain(id, include_consequences=true)`** — Accepts either a `T0:<uuid>` reference or a Fingerprint ID. Prefer passing the exact `T0:<uuid>` returned by `mira_recall` / `mira_timeline`.

**Never invent IDs.** Only use IDs explicitly returned by MIRA tools.

---

## Additional Tools

- **`mira_update(id, content)`** — Update a memory's content and regenerate its fingerprint/embedding. Use for corrections and enrichments.
- **`mira_search(query, top_k, threshold)`** — Pure vector search without CBA. Useful for diagnostics and data exploration.
- **`mira_consolidate(wing, similarity_threshold)`** — Merge redundant session notes into synthesized facts.
- **`mira_timeline(wing="<project>")`** — Review project evolution before major refactors. Filter by `room`, `type`, `since`, `until`.
- **`mira_archive`** — Call occasionally to archive stale session notes and debug logs.
- **`mira_status`** — Check system health, memory counts, version, uptime, and index status before heavy usage.
- **`mira_health`** — Quick JSON health check (`status`, `db_connected`, `memory_count`). Use for lightweight liveness probes.
- **`mira_clear_memory`** — Permanently delete memories (global or room-scoped). **Use ONLY with explicit user request.**
- **`mira_compress`** — Run rule-based context compression over session_note verbatims. Optional, deterministic.

---

## Anti-Patterns

1. **Never leave important context unstored** — the LLM context window is finite; MIRA is persistent.
2. **Never invent IDs** — `mira_load` and `mira_causal_chain` require exact IDs returned by MIRA (formats: `T0:<uuid>`, `F0:<uuid>`, `V0:<uuid>`, `FP:<uuid>`).
3. **Avoid vague recall queries** — `"auth"` is bad; `"JWT RS256 auth-service token config"` is good.
4. **Do not call `mira_clear_memory`** without explicit user request.
5. **Keep wing names consistent** — reuse the same canonical wing name across a project.
6. **Do not translate queries** — MIRA handles cross-lingual retrieval automatically.
7. **Do not store raw code without context** — store the *decision* or *fact* behind the code, not the code itself.
8. **Do not assume SOUL is enabled** — MIRA runs solo by default (14 tools). Check tool availability before invoking `soul_*` tools.

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

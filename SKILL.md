---
name: mira
description: Long-term memory guidance for MIRA MCP integration
author: benoitpetit
version: "1.0"
tags: [memory, mcp, mira]
---

# MIRA Memory Loop Guidelines

You are augmented with **MIRA** (Memory with Information-theoretic Relevance Allocation), an external MCP server providing long-term, cross-session memory. The detailed tool schemas for `mira_store`, `mira_recall`, `mira_load`, `mira_timeline`, `mira_causal_chain`, `mira_status`, and `mira_archive` are already documented in the *External Tools Reference (MCP Servers)* section of your system prompt.

Use the workflow below to decide **when** and **how** to call these tools.

---

## When to Use MIRA

| Situation | Action |
|-----------|--------|
| **Start of a task/session** | `mira_recall` to retrieve relevant context before answering or coding. |
| **Architectural/design decision** | `mira_store(type="decision")` immediately after the choice is made. |
| **Important fact discovered** | `mira_store(type="fact")` for configs, constraints, credentials, docs. |
| **User preference expressed** | `mira_store(type="preference")` for style, conventions, subjective choices. |
| **Bug resolved or debug insight** | `mira_store(type="debug_log")` for errors, stack traces, root causes. |
| **End of significant work** | `mira_store(type="session_note")` summarizing what was done, files touched, and pending items. |
| **Need historical timeline** | `mira_timeline` to see how a topic evolved. |
| **Need decision lineage** | `mira_causal_chain` to trace causes and consequences. |

---

## Project Conventions for b0p

- **Default project wing**: `b0p`
- **General wing**: `general` — use ONLY for knowledge that applies across ALL projects (coding conventions, tool usage patterns, global preferences).
- **Always specify `wing`** on every call. Never omit it or use placeholder names.
- **Recommended rooms**:
  - `decisions` — architectural or design decisions.
  - `architecture` — structural patterns, tech stack choices, refactors.
  - `bugs` — root causes and fixes.
  - `context` — high-level project context and onboarding info.
  - `session` — per-conversation summaries and checkpoints.
  - `learnings` — insights from errors, optimizations, or repeated tasks.

If you do not specify `room`, MIRA auto-assigns one based on `type`:
- `decision` → `decisions`
- `fact` → `facts`
- `preference` → `preferences`
- `session_note` → `session`
- `debug_log` → `debug`

---

## Recall Workflow

1. **First**, query the project wing:
   ```json
   { "tool": "mira_recall", "arguments": { "query": "<topic>", "wing": "b0p", "budget": 4000 } }
   ```
2. **If results are sparse**, run a **second** query against `wing: "general"` for global context.
3. **Do NOT mix wings** in a single query; run two separate recalls and merge the results mentally.
4. Before editing a file, recall related memories (e.g., `"similar bug in parser.go"`) to avoid repetition.

---

## Store Workflow

Store memories **progressively** as you work:

```json
{ "tool": "mira_store", "arguments": { "content": "Decision: use gin for REST API routing", "wing": "b0p", "room": "architecture", "type": "decision" } }
```

```json
{ "tool": "mira_store", "arguments": { "content": "User prefers table-driven tests for all Go packages", "wing": "general", "room": "preferences", "type": "preference" } }
```

---

## Budget Guidelines for `mira_recall`

| Scenario | Suggested budget |
|----------|------------------|
| Quick lookup | 500 – 1000 tokens |
| Standard context | 2000 – 4000 tokens (default) |
| Deep architectural analysis | 6000 – 8000 tokens |
| Massive recall | 10000+ tokens |

---

## Memory Types and Lifespan

| Type | Use for | Auto-archive |
|------|---------|--------------|
| `decision` | Structuring choices | Never |
| `fact` | Objective info, configs, docs | Never |
| `preference` | Subjective choices, conventions | Never |
| `session_note` | Temporary context, TODOs, summaries | 30 days |
| `debug_log` | Errors, stack traces, fixes | 7 days |

> **Tip**: omit `type` if unsure — MIRA auto-detects it.

---

## Additional Tools

- **`mira_load(id)`** — Use when a recall result shows only a fingerprint (summary) and you need the full original text.
- **`mira_timeline(wing="b0p")`** — Review project evolution before major refactors.
- **`mira_causal_chain(id, include_consequences=true)`** — Trace how a decision or bug fix impacted other parts of the system.
- **`mira_archive`** — Call occasionally to archive stale session notes and debug logs.
- **`mira_status`** — Check index health before heavy usage.

---

## Anti-Patterns

1. **Never leave important context unstored** — the LLM context window is finite; MIRA is persistent.
2. **Never invent IDs** — `mira_load` and `mira_causal_chain` require exact IDs returned by MIRA.
3. **Avoid vague recall queries** — `"auth"` is bad; `"JWT RS256 auth-service token config"` is good.
4. **Do not call `mira_clear_memory`** without explicit user request.
5. **Keep wing names consistent** — `b0p` and `general` are the canonical names for this project.

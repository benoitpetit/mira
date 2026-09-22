# MIRA memory, recall, and soul corrections — implementation plan

**Goal:** Apply the approved business-logic corrections across the memory core, semantic recall, Soul profile, and PostgreSQL retention path, then verify and publish them from `main`.

**Architecture:** Keep the existing ports/adapters/use-case boundaries. Add a small shared derived-indexing helper in the interactor layer, make recall's ordering explicit (score → early threshold → cluster → prune), and evolve Soul snapshots through observation-aware merge helpers. Keep additive JSON compatibility.

**Tech stack:** Go, SQLite/PostgreSQL repositories, FTS5, HNSW/brute-force vector stores, JSON snapshots, Go test.

**Spec:** `docs/superpowers/specs/2026-09-22-mira-memory-soul-corrections-design.md` (approved in the conversation).

## Task 1 — Memory lifecycle indexing and encoding safety

**Files:** `internal/usecases/interactors/update_memory.go`, `store_memory.go`, `consolidate_memories.go`, new helper/tests, tag repository adapters.

- Add failing tests for update validation, tag rebuild, causal rebuild, and rune-safe consolidation.
- Implement a shared derived-indexing path for tags, causal node, and causal relations.
- Validate update input before mutation and rebuild all derived data after replacement.
- Normalize new tags and make existing tag lookup case-insensitive.
- Replace byte slicing in consolidation with rune-safe truncation.
- Run focused interactor/storage tests.

## Task 2 — Recall ordering and candidate validity

**Files:** `internal/usecases/interactors/recall_memory.go`, `search_semantic.go`, vector/store helpers, recall tests.

- Add failing tests for score-before-cluster, non-transitive clustering, active early threshold, nil logger, invalid/future candidates, deterministic RRF, and future recency.
- Score and tag-boost before clustering, apply early threshold, then cluster and prune.
- Select representatives by actual candidate score and use stable representative comparison.
- Guard optional logging, filter invalid candidates at the semantic boundary, clamp recency, and stabilize RRF tie-breaking.
- Run focused recall/search tests.

## Task 3 — Soul observation evolution and safe recall context

**Files:** `internal/agentmemory/runtime.go`, `tools/mcp_behavioral_metrics.go`, Soul tests.

- Add failing tests for one-observation counting, negation-aware traits, cumulative blended profile fields, negative directives, patch validation, persisted reason, and budget allocation.
- Make extraction boundary/negation aware and count captures rather than keyword hits.
- Blend profile fields cumulatively and support decreases in directives.
- Reject unknown or invalid patch fields and persist `change_reason`.
- Allocate recall budget dynamically and delimit memory evidence as non-normative.
- Run focused Soul tests.

## Task 4 — PostgreSQL retention parity and provider validity

**Files:** PostgreSQL repository, provider fallback, tests.

- Add failing SQL/mock or repository-level tests for PostgreSQL archive selection, dependent deletion, token count, and validity filtering in fallback search.
- Implement PostgreSQL archive parity with SQLite and filter invalid fallback memories.
- Run focused PostgreSQL/provider tests.

## Task 5 — Full verification and publication

- Run `gofmt`, `go test -tags fts5 ./...`, `go vet ./...`, and build checks as applicable.
- Inspect diff and ensure `.codex-gocache/` and `.codex-tmp/` remain untracked and untouched.
- Delete non-`main` local branches; inspect remote branches and remove only explicitly in-scope stale feature branches if safe.
- Commit the implementation on `main` and push `main` to `origin`.
- Re-run final status/log verification after push.

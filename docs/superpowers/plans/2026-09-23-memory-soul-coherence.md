# MIRA Memory and Soul Coherence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the approved memory/soul audit corrections, add the autonomous belief and quality mechanisms, and synchronize all public documentation and the algorithm landing page.

**Architecture:** Preserve T0/T1/T2, hybrid dense/lexical retrieval, CBA, and immutable soul lineage. Add provenance and lifecycle metadata as orthogonal fields; derive reversible consolidation, causal confidence, belief views, and quality feedback from the authoritative SQL records. Keep clients as callers of existing MCP/REST operations while MIRA owns scoring, lifecycle, and consistency rules.

**Tech Stack:** Go 1.25+, SQLite/PostgreSQL, SQL migrations, HNSW/vector adapters, MCP, Next.js/React landing page, Go tests with `-tags fts5`.

**Spec:** `docs/superpowers/specs/2026-09-23-memory-soul-coherence-design.md`

## Global Constraints

- Preserve T0/T1/T2 and existing MCP tool names; extend inputs compatibly.
- Treat SQL as authoritative; derived vector, tag, causal, and belief indexes must be repairable.
- Never attribute `user`, `system`, or `tool` text to the agent identity extractor.
- Keep consolidated sources recoverable and mark their lifecycle instead of deleting them.
- Do not add cloud services or external runtime dependencies.
- Keep SQLite and PostgreSQL migrations and repository behavior equivalent.
- Run `go test -tags fts5 ./...` before claiming completion.
- Update `README.md`, `README_FR.md`, `SKILL.md`, `docs/ARCHITECTURE.md`, `docs/FEATURES.md`, `config.example.yaml`, and `mira-landing` when public behavior changes.

## Review Focus

- Mixed-role soul capture must not learn user language as agent traits — covered by Task 1 MCP/runtime tests.
- Snapshot compaction must preserve current/jalons and lineage under repeated writes — covered by Task 2 retention tests.
- Consolidation failure or revocation must not strand or delete source memories — covered by Task 3 transaction tests.
- Generic subjects and out-of-window memories must not create confirmed causal edges — covered by Task 4 detector tests.
- A small budget must include identity invariants and never exceed the reported token count — covered by Task 6 budget tests.

### Task 1: Role-aware soul capture and bounded evidence

**Files:**
- Modify: `internal/agentmemory/runtime.go` (`CaptureRequest`, `Snapshot`, extraction and persistence helpers)
- Modify: `internal/agentmemory/mcp.go` (capture schema and argument decoding)
- Modify: `internal/agentmemory/runtime_test.go`
- Modify: `internal/agentmemory/runtime_test.go` test provider fixtures
- Create: `internal/adapters/storage/migrations/012_agent_memory_evidence.up.sql`
- Modify: `internal/adapters/storage/migrations.go` and PostgreSQL migration registration

**Interfaces:**
- Produces `ConversationObservation{Role, Content, Timestamp, SessionID}` and bounded `TraitEvidence` persisted by snapshot ID.
- Keeps legacy `CaptureRequest.Conversation` as unattributed input and adds `Messages []ConversationObservation`.

- [ ] **Step 1: Write failing tests** for mixed user/assistant messages, legacy unattributed capture, bounded evidence length, and JSON round-trip of evidence.
- [ ] **Step 2: Run** `go test ./internal/agentmemory -run 'Test.*Capture|Test.*Evidence'`; expect failures because the role-aware request and evidence fields do not exist.
- [ ] **Step 3: Implement** role filtering in `extractSnapshot`: concatenate only assistant observations for trait/voice/style/value/emotion extraction; retain unattributed text only for a low-confidence diagnostic marker.
- [ ] **Step 4: Add** `trait_evidence` storage keyed by identity snapshot and trait name, with source role, bounded excerpt, session, timestamp, and confidence; write it in the same transaction as the snapshot.
- [ ] **Step 5: Extend** `soul_capture` input schema with a messages array while preserving `conversation`; reject malformed roles and normalize unknown roles to unattributed rather than assistant.
- [ ] **Step 6: Run** the focused tests and `go test ./internal/agentmemory ./internal/config`; expect PASS.
- [ ] **Step 7: Commit** `feat: make soul capture role-aware and evidence-backed`.

### Task 2: Real soul snapshot retention and compaction

**Files:**
- Modify: `internal/agentmemory/runtime.go` (`storeSnapshotWith`, `Capture`, `Update`, `Patch`, `HandleSwap`)
- Modify: `internal/agentmemory/runtime_test.go`
- Modify: `internal/config/config.go` and `config.example.yaml`
- Create: `internal/adapters/storage/migrations/013_agent_memory_retention.up.sql`

**Interfaces:**
- Adds `Snapshot.RetentionClass` (`current`, `milestone`, `intermediate`, `compacted`) and an internal retention operation.
- Adds `Runtime.CompactHistory(ctx, agentID string) (int, error)`; it is idempotent and keeps `derived_from_id` resolvable.

- [ ] **Step 1: Write failing tests** that create more than `MaxHistoryVersions` snapshots and assert current, model-swap, drift, and explicit-change milestones remain.
- [ ] **Step 2: Run** `go test ./internal/agentmemory -run 'Test.*Retention|Test.*Compact'`; expect failures.
- [ ] **Step 3: Add** retention metadata and a transactional compaction query that preserves milestone rows and marks intermediate rows compacted into a retained snapshot.
- [ ] **Step 4: Invoke** compaction after successful capture/update/patch/swap, never before the new snapshot transaction commits.
- [ ] **Step 5: Ensure** `History` follows lineage and excludes compacted intermediates by default while accepting an explicit include flag internally.
- [ ] **Step 6: Run** SQLite and PostgreSQL-compatible unit tests; verify repeated compaction produces no additional rows or broken parents.
- [ ] **Step 7: Commit** `feat: enforce bounded soul snapshot retention`.

### Task 3: Lifecycle metadata and reversible consolidation

**Files:**
- Modify: `internal/domain/entities/verbatim.go`, `internal/domain/entities/candidate.go`
- Modify: `internal/adapters/storage/sqlite_repository.go` and `postgres_repository.go`
- Create: `internal/adapters/storage/migrations/014_memory_lifecycle.up.sql`
- Modify: `internal/usecases/interactors/consolidate_memories.go`
- Create: `internal/usecases/interactors/revoke_consolidation.go`
- Modify: `internal/usecases/interactors/recall_memory.go`, `memory_derived_index.go`
- Modify: `internal/usecases/interactors/consolidate_memories_test.go`, `recall_memory_test.go`

**Interfaces:**
- Adds `LifecycleState` and `SupersededBy` to `Verbatim`/storage.
- Adds `RevokeConsolidation(ctx, synthesizedID uuid.UUID) error` and repository lifecycle methods.

- [ ] **Step 1: Write failing tests** asserting consolidation marks source notes `superseded`, retains T0/T1/T2, excludes them from normal recall, and revocation reactivates them while marking the synthesis `contested`.
- [ ] **Step 2: Run** the focused consolidation tests; expect failures.
- [ ] **Step 3: Add** lifecycle columns and indexes to both SQL dialect migrations, with existing rows defaulting to `active`.
- [ ] **Step 4: Replace** `ClearByIDs` in consolidation with one transaction that inserts the synthesis, links source IDs, and updates source lifecycle states.
- [ ] **Step 5: Add** repository queries so candidates include lifecycle and recall filters non-active sources unless explicitly requested for diagnostics.
- [ ] **Step 6: Implement** revocation as an idempotent transaction; rebuild tags/causal data and repair vector state before returning success.
- [ ] **Step 7: Run** focused tests plus storage/vector tests and commit `feat: make consolidation reversible`.

### Task 4: Evidence-backed causal relations and configured windows

**Files:**
- Modify: `internal/domain/entities/causal.go`, `internal/domain/valueobjects/relation_type.go`
- Create: `internal/adapters/storage/migrations/015_causal_evidence.up.sql`
- Modify: `internal/adapters/extraction/native_extractor.go`
- Modify: `internal/usecases/interactors/memory_derived_index.go`
- Modify: `internal/usecases/interactors/store_memory.go`, `update_memory.go`
- Modify: `internal/app/main.go` to pass `CausalLookback` and `CausalMaxDays`
- Modify: causal and storage tests

**Interfaces:**
- Extends `CausalEdge` with `Confidence`, `Status`, `Evidence`, `Detector`, and `DetectedAt`.
- Adds detector options for lookback count, max age, and generic-subject filtering.

- [ ] **Step 1: Write failing tests** for configured lookback/max age, generic `Note` rejection, and proposed/confirmed status transitions.
- [ ] **Step 2: Run** `go test ./internal/adapters/extraction ./internal/usecases/interactors -run 'Causal|causal'`; expect failures.
- [ ] **Step 3: Implement** detector options and pass them from config; remove the hardcoded lookback of 50.
- [ ] **Step 4: Emit** proposed edges with bounded evidence and confidence; only confirm when a non-generic subject/entity or semantic threshold is present.
- [ ] **Step 5: Persist** the new fields in SQLite/PostgreSQL and keep old edges as confirmed with a conservative legacy confidence.
- [ ] **Step 6: Update** causal graph reads to expose status and make `HasEdge` return confirmed edges by default.
- [ ] **Step 7: Run** focused tests and commit `feat: add confidence and lifecycle to causal edges`.

### Task 5: Quality-aware and relation-aware CBA scoring

**Files:**
- Modify: `internal/usecases/interactors/recall_memory.go`
- Modify: `internal/usecases/interactors/search_time_clusterer.go`
- Modify: `internal/adapters/storage/sqlite_repository.go`, `postgres_repository.go` candidate decoding
- Modify: `internal/usecases/interactors/recall_memory_test.go`, `recall_memory_extra_test.go`

**Interfaces:**
- Adds candidate quality fields: extraction confidence, validation freshness, lifecycle factor, and confirmed relation metadata.
- Keeps `scoreCandidates` and `selectGreedy` signatures stable.

- [ ] **Step 1: Write failing tests** for low extraction confidence, stale validation, active-vs-superseded selection, and each relation type.
- [ ] **Step 2: Run** focused recall tests; expect failures.
- [ ] **Step 3: Decode** extraction quality from `FingerprintData.Custom` and lifecycle metadata into `Candidate`.
- [ ] **Step 4: Multiply** the base score by bounded quality and apply relation-specific rules: update replacement, contradiction pair marker, causal packet bonus, proposed-edge weak signal.
- [ ] **Step 5: Modify** greedy selection to reserve atomic causal packets and degrade packet rendering together when budget is insufficient.
- [ ] **Step 6: Verify** adaptive thresholds and reranking still operate on the revised score and no score exceeds `[0,1]`.
- [ ] **Step 7: Run** recall/vector tests and commit `feat: make CBA quality and relation aware`.

### Task 6: Shared soul/memory budget, identity classification, and index repair

**Files:**
- Modify: `internal/agentmemory/runtime.go`, `internal/agentmemory/provider.go`
- Modify: `internal/agentmemory/mcp.go`
- Modify: `internal/usecases/interactors/recall_memory.go`
- Modify: `internal/adapters/vector/fallback_vector_store.go`, HNSW adapters
- Modify: `internal/usecases/interactors/archive_memories.go` and app wiring
- Modify: `internal/agentmemory/runtime_test.go`, recall/archive/vector tests

**Interfaces:**
- Adds explicit soul budget partition configuration with defaults preserving the current total budget.
- Adds a repair-capable archive path that deletes/rebuilds derived vectors before success.

- [ ] **Step 1: Write failing tests** for budget partitioning, header accounting, `kind=identity` identity events, archived IDs absent from vector recall, and repair fallback.
- [ ] **Step 2: Implement** deterministic partitioning: identity invariants first, preferences second, work evidence last; clamp all partitions and account for delimiters.
- [ ] **Step 3: Change** `NotifyMiraOfIdentityChange` to pass `KindIdentity` while preserving extraction `TypeFact` where applicable.
- [ ] **Step 4: Make** archive/clear paths synchronize vector state or rebuild from SQL before returning an error/success result.
- [ ] **Step 5: Run** all soul, recall, storage, and vector tests; commit `fix: synchronize identity and archived memory indexes`.

### Task 7: Versioned belief view and local quality feedback

**Files:**
- Create: `internal/domain/entities/belief.go`
- Create: `internal/usecases/interactors/belief_registry.go`
- Create: `internal/adapters/storage/migrations/016_beliefs_quality_feedback.up.sql`
- Modify: repository ports and SQLite/PostgreSQL repositories
- Modify: recall MCP/controller wiring and tests

**Interfaces:**
- Adds `Belief{Subject, Predicate, Value, ValidFrom, ValidUntil, Confidence, Sources, Status}` and registry methods for derive/query/retract.
- Adds bounded feedback values `useful`, `stale`, `contradictory`, `irrelevant` and an aggregation method used as a small CBA calibration factor.

- [ ] **Step 1: Write failing tests** for active/superseded/contested belief resolution, temporal validity, source retention, and feedback aggregation bounds.
- [ ] **Step 2: Implement** derivation from structured fingerprints and lifecycle/causal metadata without replacing T0/T1/T2.
- [ ] **Step 3: Persist** beliefs and feedback with unique source links and idempotent upserts.
- [ ] **Step 4: Expose** read/retract/feedback operations through existing controller boundaries without allowing clients to bypass resolution rules.
- [ ] **Step 5: Apply** only a bounded calibration factor to CBA, with cold-start defaults and no external model calls.
- [ ] **Step 6: Run** package tests and commit `feat: add autonomous belief and recall feedback views`.

### Task 8: Documentation and algorithm landing page

**Files:**
- Modify: `README.md`, `README_FR.md`, `SKILL.md`
- Modify: `docs/ARCHITECTURE.md`, `docs/FEATURES.md`, `docs/API_REFERENCES.md`, `config.example.yaml`
- Modify: `mira-landing/components/landing/memory-system-section.tsx`, related product sections, and landing copy
- Modify: `mira-landing/app/page.tsx` only if section ordering or metadata changes

- [ ] **Step 1: Update** the architecture docs with role-aware soul flow, lifecycle states, causal confidence, quality-aware CBA, belief view, and budget partition diagram.
- [ ] **Step 2: Update** French/English readmes, skill tool contracts, API references, and example configuration with compatibility notes and new operations.
- [ ] **Step 3: Update** the landing algorithm section to show the actual pipeline: capture → provenance → T0/T1/T2 → belief/lifecycle derivation → hybrid search → relation-aware CBA → bounded render.
- [ ] **Step 4: Run** `gofmt`, `git diff --check`, `go test -tags fts5 ./...`, and `npm run build` in `mira-landing`.
- [ ] **Step 5: Commit** `docs: document memory soul coherence and algorithm updates`.

## Plan Self-Review

- Spec coverage: all eight spec sections map to Tasks 1–8; documentation requirements map to Task 8.
- Placeholder scan: no TODO/TBD steps; every task names files, interfaces, tests, and expected behavior.
- Type consistency: lifecycle state is shared by `Verbatim`/candidate/repositories; causal status is shared by entities, detector, storage, and CBA; belief and feedback APIs are introduced before recall calibration.
- Dependency order: Tasks 1–2 precede soul budget work; Task 3 precedes lifecycle-aware CBA; Task 4 precedes relation-aware CBA; Task 7 follows stable lifecycle/quality primitives; Task 8 follows all public behavior changes.
- Review focus: each of the five focus cases has an owning test task.

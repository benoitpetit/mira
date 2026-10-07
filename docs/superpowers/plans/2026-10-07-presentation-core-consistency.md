# Presentation and Core Consistency Implementation Plan

> **For agentic workers:** Native implementation in the current task. No tests will be added or run, per workspace instruction.

**Goal:** Make public MIRA claims match the shipped core while improving measurable behavior where the source currently falls short.

**Architecture:** Keep the stable release boundary explicit. Correct marketing language where backend/platform behavior differs, expose existing richer agent-install flows accurately, and make one-sample benchmark setup timings visibly distinct from repeated latency percentiles.

**Tech Stack:** Go core, Next.js/TypeScript site, versioned JSON benchmark snapshots.

**Spec:** Findings from the read-only source audit in this conversation.

## Global Constraints

- Do not claim a feature is in v0.8.5 if it only exists on the later `main` commits.
- Preserve and explain SQLite, PostgreSQL, and Windows fallback behavior.
- Never label whitespace-counted context budgets as model-token counts.
- Keep benchmark provenance and raw samples visible.
- Do not add or run tests in this task.

## Review Focus

- Release-versus-main agent-install support — make the version boundary explicit.
- One-sample setup timings — do not present them as stable p95 latency measurements.
- Cross-language recall — qualify by configured embedding model.
- PostgreSQL and Windows search paths — state the backend/platform-specific implementation.
- Context budget — distinguish estimated token metadata from whitespace-unit selection budget.

---

### Task 1: Benchmark evidence and presentation

**Files:** `mira-landing/public/data/benchmarks/current.json`, `mira-landing/components/benchmarks/benchmark-report.tsx`, `mira-landing/app/benchmarks/page.tsx`.

- [x] Keep the source snapshot and provenance intact.
- [x] Render one-sample setup cases as a single observation, not p50/p95 percentiles.
- [x] Limit the five-sample statement to repeated latency cases and state that setup observations are single-run values.

### Task 2: Align technical claims and integration paths

**Files:** `mira-landing/components/landing/product-sections.tsx`, `allocator-section.tsx`, `memory-system-section.tsx`, `integration-section.tsx`, core `README_FR.md`/integration command documentation if required.

- [x] Describe FTS5 as SQLite-specific and PostgreSQL lexical search separately; disclose Windows brute-force fallback.
- [x] Qualify cross-language recall by model and avoid guarantees.
- [x] Label budget measurements as whitespace units; preserve token estimate descriptions separately.
- [x] Distinguish v0.8.5's five `mira setup` clients from the later `mira agent install` support for Hermes, OpenCode, and Pi.
- [x] Keep the fuller integration path discoverable without claiming it ships in v0.8.5.

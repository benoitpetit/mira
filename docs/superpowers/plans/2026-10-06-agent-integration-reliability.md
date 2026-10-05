# Agent Integration Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make MIRA's agent installation truthful, project-scoped, secure, and usable across Codex, Claude Code, Windsurf, Cursor, Claude Desktop, Hermes, OpenCode, and Pi Agent.

**Architecture:** Keep memory policy inside `internal/agentbridge`; add client-specific capability declarations and output renderers at the command boundary. The installer writes each client’s native MCP, instruction, skill, and hook artifacts and records their actual recall/capture modes in the manifest.

**Tech Stack:** Go 1.23, Cobra, YAML v3, JSON, TOML parser, Next.js landing page.

**Spec:** `docs/superpowers/specs/2026-10-06-agent-integration-design.md`

## Global Constraints

- Project scope must never write a user-level client configuration or hook file.
- All automatic capture must pass through `agentbridge.Bridge.Handle` before storage.
- Recalled content must stay redacted and wrapped as reference-only context.
- Only claim hook automation for a client/event with a documented compatible output contract.
- Generated skills may list only tools exposed by the active MIRA MCP surface.
- Existing third-party configuration and user-authored instruction content must be preserved.
- Landing-page edits must preserve the user’s existing uncommitted work.

## Review Focus

- Existing Codex project config containing unrelated TOML tables keeps those tables after MIRA installation.
- A secret-shaped prompt captured through legacy `setup --automatic-memory` is redacted before storage.
- A project installation made from a subdirectory resolves paths from the selected project root.
- A missing optional client executable yields an actionable `doctor` result instead of a false healthy status.
- Reinstalling every client is idempotent and does not duplicate hooks, rules, skills, or MCP entries.

---

### Task 1: Client capability contract and manifest modes

**Files:**
- Create: `internal/agentinstall/client.go`
- Modify: `internal/agentinstall/manifest.go`
- Modify: `cmd/mira/agent.go`
- Test: `internal/agentinstall/client_test.go`
- Test: `internal/agentinstall/manifest_test.go`

**Interfaces:**
- Produces: `agentinstall.Client`, `agentinstall.LookupClient(string)`, `Client.RecallMode`, `Client.CaptureMode`, and persisted manifest mode fields.
- Consumes: existing `agentinstall.Manifest` and policy methods.

- [ ] **Step 1: Write failing capability and manifest tests**

Cover all eight clients, their supported scope/mode declarations, and save/load of explicit recall and capture modes.

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/agentinstall -run 'Client|Manifest'`

Expected: FAIL because client declarations and persisted modes do not exist.

- [ ] **Step 3: Implement the client declaration and manifest mode fields**

Use canonical IDs `codex`, `claude-code`, `windsurf`, `cursor`, `claude-desktop`, `hermes`, `opencode`, and `pi`. Derive mode values from policy plus client capabilities during installation.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `go test ./internal/agentinstall -run 'Client|Manifest'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentinstall/client.go internal/agentinstall/client_test.go internal/agentinstall/manifest.go internal/agentinstall/manifest_test.go cmd/mira/agent.go
git commit -m "feat: model agent client capabilities"
```

### Task 2: Bridge output adapters and unified capture protection

**Files:**
- Create: `internal/agentbridge/output.go`
- Modify: `internal/agentbridge/event.go`
- Modify: `cmd/mira/agent.go`
- Modify: `cmd/mira/hook.go`
- Test: `internal/agentbridge/output_test.go`
- Test: `cmd/mira/hook_test.go`

**Interfaces:**
- Consumes: `agentinstall.Client` from Task 1 and `agentbridge.Result`.
- Produces: `agentbridge.RenderHookOutput(client, event, result) ([]byte, error)` and a single redacted capture path.

- [ ] **Step 1: Write failing output and redaction tests**

Assert Codex and Claude Code serialize `hookSpecificOutput.hookEventName` and `additionalContext`; assert capture through the low-level hook path removes a bearer token before `StoreMemory` receives content.

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/agentbridge ./cmd/mira -run 'HookOutput|Hook.*Redact'`

Expected: FAIL because generic bridge JSON is emitted and legacy hooks bypass redaction.

- [ ] **Step 3: Implement client-specific output and route legacy hooks through the bridge policy**

Keep `Result` neutral. Render only hook-compatible events for Codex and Claude Code; produce no misleading recall output for capture-only clients. Move legacy content through redaction and substantive filtering before storage.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `go test ./internal/agentbridge ./cmd/mira -run 'HookOutput|Hook.*Redact'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentbridge/output.go internal/agentbridge/output_test.go internal/agentbridge/event.go cmd/mira/agent.go cmd/mira/hook.go cmd/mira/hook_test.go
git commit -m "fix: render agent hook context per client"
```

### Task 3: Project-safe Codex and Claude Code lifecycle installation

**Files:**
- Modify: `cmd/mira/main.go`
- Modify: `cmd/mira/agent.go`
- Modify: `internal/agentinstall/client.go`
- Test: `cmd/mira/main_test.go`
- Test: `cmd/mira/agent_test.go`

**Interfaces:**
- Consumes: client scopes/modes from Task 1 and rendered output from Task 2.
- Produces: project-local Codex MCP/hook artifacts and installed `SessionStart` hooks for Codex and Claude Code.

- [ ] **Step 1: Write failing installation tests**

Assert `--scope project` targets `.codex/config.toml` and `.codex/hooks.json`, retains unrelated TOML content, and installs `SessionStart` plus `UserPromptSubmit`. Assert Claude Code installs matching `SessionStart` and prompt hooks in the correct settings file.

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./cmd/mira -run 'Codex.*Project|Agent.*SessionStart|Claude.*SessionStart'`

Expected: FAIL because Codex uses user paths and session-start hooks are absent.

- [ ] **Step 3: Implement scoped MCP/config writers and session-start hook installation**

Use a TOML parser that preserves unrelated tables. Use the Codex CLI only for explicit user scope; write project TOML directly for project scope. Keep hook commands absolute and manifest-bound.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `go test ./cmd/mira -run 'Codex.*Project|Agent.*SessionStart|Claude.*SessionStart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum cmd/mira/main.go cmd/mira/main_test.go cmd/mira/agent.go cmd/mira/agent_test.go internal/agentinstall/client.go
git commit -m "fix: scope Codex and Claude lifecycle integration"
```

### Task 4: Native skills and install adapters for every client

**Files:**
- Create: `internal/agentinstall/skill.go`
- Modify: `cmd/mira/agent.go`
- Modify: `cmd/mira/main.go`
- Test: `internal/agentinstall/skill_test.go`
- Test: `cmd/mira/agent_test.go`

**Interfaces:**
- Consumes: client declarations from Task 1.
- Produces: `InstallManagedSkill` and client adapter configuration for Hermes, OpenCode, and Pi.

- [ ] **Step 1: Write failing adapter tests**

Cover Cursor MDC frontmatter, OpenCode project `opencode.json` MCP entry with direct tools, Pi `.pi/mcp.json` and `.pi/skills/mira/SKILL.md`, Hermes YAML MCP merge, and generated skills whose tool list matches `Controller.ToolDefinitions`.

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/agentinstall ./cmd/mira -run 'Skill|Hermes|OpenCode|Pi|CursorRule'`

Expected: FAIL because these installers and generated skills do not exist.

- [ ] **Step 3: Implement managed portable skills and client-specific config adapters**

Install skills only in native supported locations, preserve configuration on merge, and set honest `skill-guided` modes for clients without compatible automatic context hooks. Add `hermes`, `opencode`, and `pi` to CLI validation, auto-detection where a marker exists, doctor, and uninstall.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `go test ./internal/agentinstall ./cmd/mira -run 'Skill|Hermes|OpenCode|Pi|CursorRule'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentinstall/skill.go internal/agentinstall/skill_test.go cmd/mira/agent.go cmd/mira/agent_test.go cmd/mira/main.go
git commit -m "feat: install MIRA skills for supported agents"
```

### Task 5: Accurate diagnostics and lifecycle removal

**Files:**
- Modify: `cmd/mira/agent.go`
- Modify: `internal/agentinstall/manifest.go`
- Test: `cmd/mira/agent_test.go`

**Interfaces:**
- Consumes: manifest artifact paths and mode fields from Tasks 1–4.
- Produces: `agent doctor` verification of actual client artifacts and mode reporting; idempotent uninstall for all supported clients.

- [ ] **Step 1: Write failing doctor and uninstall tests**

Assert a missing hook, skill, or MCP configuration fails doctor with the affected mode; assert fallback clients report `skill-guided`; assert uninstall removes MIRA-owned artifacts without deleting unrelated configuration.

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./cmd/mira -run 'AgentDoctor|AgentUninstall|AgentStatus'`

Expected: FAIL because doctor only checks a subset of artifacts and does not report actual modes.

- [ ] **Step 3: Implement artifact checks and managed removal**

Validate configuration syntax and MIRA entries where parsers are available. Make missing optional CLI executables warnings for declarative configurations, not false positives. Preserve unrelated configuration blocks and instruction text.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `go test ./cmd/mira -run 'AgentDoctor|AgentUninstall|AgentStatus'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/mira/agent.go cmd/mira/agent_test.go internal/agentinstall/manifest.go
git commit -m "feat: verify installed agent integration modes"
```

### Task 6: Documentation and landing-page contract alignment

**Files:**
- Modify: `README.md`
- Modify: `README_FR.md`
- Modify: `SKILL.md`
- Modify: `docs/agent-integration.md`
- Modify: `docs/FEATURES.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/API_REFERENCES.md`
- Modify: `../mira-landing/components/landing/integration-section.tsx`
- Modify: `../mira-landing/app/versions/page.tsx`
- Test: `cmd/mira/agent_test.go`

**Interfaces:**
- Consumes: supported clients and mode strings from Tasks 1–5.
- Produces: consistent user-facing support table, commands, and landing-page integration messaging.

- [ ] **Step 1: Write failing documentation-contract tests**

Add a table-driven test for the public client IDs and expected mode labels used by generated documentation data. Ensure the root skill names only the MCP tools actually registered.

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./cmd/mira -run 'AgentDocumentationContract'`

Expected: FAIL because public integration metadata is not centralized.

- [ ] **Step 3: Implement shared documentation metadata and update all docs/site copy**

Describe automatic recall, automatic capture, and skill-guided behavior separately. Add Hermes, OpenCode, and Pi commands. Update the landing page without overwriting unrelated uncommitted design work.

- [ ] **Step 4: Run the focused test and build the landing page**

Run: `go test ./cmd/mira -run 'AgentDocumentationContract' && npm run build`

Expected: Go test passes and the landing build succeeds.

- [ ] **Step 5: Commit**

```bash
git add README.md README_FR.md SKILL.md docs cmd/mira/agent_test.go
git commit -m "docs: align agent integration guidance"
git -C ../mira-landing add components/landing/integration-section.tsx app/versions/page.tsx
git -C ../mira-landing commit -m "docs: describe MIRA agent integrations"
```

### Task 7: Full verification and release-readiness review

**Files:**
- Modify: tests only if a review finding requires a regression case.

**Interfaces:**
- Consumes: all preceding client integration contracts.
- Produces: verified core and landing build artifacts.

- [ ] **Step 1: Run the full core test suite**

Run: `CGO_CFLAGS=-fPIC go test -tags fts5 ./...`

Expected: PASS.

- [ ] **Step 2: Run static checks and both repository diff checks**

Run: `go vet ./... && git diff --check && git -C ../mira-landing diff --check`

Expected: PASS with no whitespace errors.

- [ ] **Step 3: Run the landing build**

Run: `npm run build`

Expected: PASS.

- [ ] **Step 4: Commit any review fixes**

```bash
git add -A && git commit -m "test: verify agent integration matrix"
```

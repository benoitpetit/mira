# Autonomous Agent Installation and Transparent Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install MIRA into supported AI agents with client-native instructions, deterministic hooks where available, bounded recall injection, and prudent automatic memory capture.

**Architecture:** Add a small `internal/agentinstall` domain package for manifest, policy, project identity, managed instruction blocks, redaction and doctor results. Add a short-lived `internal/agentbridge` runtime that receives normalized client events, performs bounded recall/capture/soul observation, and returns a client-neutral event result. Keep client-specific MCP/config/hook merging in adapter files and preserve the existing `mira setup` path as a compatibility surface.

**Tech Stack:** Go 1.25, Cobra, YAML, existing MIRA `app.Application`, SQLite/PostgreSQL repositories, existing `agentmemory` runtime, JSON client configuration files, table-driven Go tests.

**Spec:** `docs/superpowers/specs/2026-09-24-autonomous-agent-installation-design.md`

## Global Constraints

- Default policy is `standard`: substantive user prompts are captured; assistant responses are disabled by default.
- `minimal` captures nothing automatically, `standard` captures substantive user prompts, and `complete` additionally captures assistant responses when the client exposes them.
- `mira agent install` must be idempotent and must preserve unrelated client configuration.
- Managed instructions use `<!-- MIRA:BEGIN managed instructions -->` and `<!-- MIRA:END managed instructions -->` markers.
- Retrieved memory is emitted only inside `<MIRA_CONTEXT trust="reference-only">` delimiters and never becomes an authorization or higher-priority instruction.
- Secrets, credential-shaped values, session tokens and private keys are redacted before storage or injection.
- Hook processes are short-lived, local, non-blocking, and must not start REST, metrics, webhook, or long-running MCP services.
- `mira setup` and existing hidden `mira hook <client>` commands remain backward compatible.
- No database migration is required.
- Go verification uses `go test -tags fts5`.

## Review Focus

- A second installation must replace one managed block rather than duplicate instructions; Task 2 tests exact preservation of surrounding content.
- A malformed existing client config must fail without overwriting it; Task 4 tests atomic file writes and unchanged bytes on error.
- A secret embedded in a prompt, JSON field, or Markdown code block must not reach storage or injected context; Task 3 tests redaction across supported forms.
- A client without a deterministic hook must be reported as instruction-guided rather than falsely advertised as automatic; Task 4 tests capability status.
- A recall or storage failure during a hook must not block the agent process; Task 3 tests fail-open event behavior and metadata-only diagnostics.

---

### Task 1: Manifest, policy, project identity and configuration contract

**Files:**
- Create: `internal/agentinstall/manifest.go`
- Create: `internal/agentinstall/manifest_test.go`
- Create: `internal/agentinstall/policy.go`
- Create: `internal/agentinstall/policy_test.go`
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

**Interfaces:**
- Produces `agentinstall.Manifest`, `agentinstall.Policy`, `agentinstall.LoadManifest(path)`, `agentinstall.SaveManifest(path, manifest)`, and `agentinstall.ResolveProjectWing(projectRoot)`.
- `Manifest` contains client, scope, wing, policy, capture settings, recall budget/settings, and soul observation settings matching the spec YAML.
- `Policy` exposes `CaptureUserPrompts()`, `CaptureAssistantResponses()`, `InstructionsEnabled()`, and `InjectionMode()` so later tasks do not inspect string policy names.

- [ ] **Step 1: Write failing tests** for default `standard` behavior, YAML round trips, invalid policy names, `wing: auto` stability for the same absolute project root, and safe rejection of empty/invalid wings.

```go
func TestManifestDefaultsToPrudentStandardPolicy(t *testing.T) {
	manifest := agentinstall.DefaultManifest("codex", "/repo")
	if manifest.Policy != agentinstall.PolicyStandard || !manifest.Capture.UserPrompts {
		t.Fatalf("unexpected defaults: %+v", manifest)
	}
	if manifest.Capture.AssistantResponses || !manifest.Capture.RedactSecrets {
		t.Fatalf("prudent defaults leaked assistant capture or disabled redaction")
	}
}
```

- [ ] **Step 2: Run** `go test -tags fts5 ./internal/agentinstall -run 'TestManifest|TestPolicy|TestResolveProjectWing' -count=1` and verify it fails because the package and contract are absent.
- [ ] **Step 3: Implement** typed manifest defaults, YAML tags, validation, policy methods, atomic save via temporary file plus rename, and a deterministic wing derived from the normalized absolute project path. Do not add agent-install state to the main MIRA database.
- [ ] **Step 4: Extend** `config.example.yaml` with the agent integration section only where it is part of the project config; keep `.mira/agent.yaml` authoritative for per-agent policy and ensure partial YAML loading preserves defaults.
- [ ] **Step 5: Run** `gofmt -w internal/agentinstall/*.go internal/config/config.go`, then `go test -tags fts5 ./internal/agentinstall ./internal/config`.
- [ ] **Step 6: Commit** `feat: add autonomous agent manifest and policies`.

### Task 2: Managed instructions, redaction and substantive-content filters

**Files:**
- Create: `internal/agentinstall/managed_block.go`
- Create: `internal/agentinstall/managed_block_test.go`
- Create: `internal/agentinstall/redaction.go`
- Create: `internal/agentinstall/redaction_test.go`
- Create: `internal/agentinstall/substantive.go`
- Create: `internal/agentinstall/substantive_test.go`

**Interfaces:**
- Produces `MergeManagedBlock(existing, body string) (string, error)`, `RemoveManagedBlock(existing string) (string, error)`, `RedactSecrets(input string) (output string, changed bool)`, and `IsSubstantivePrompt(input string, minChars int) bool`.
- The managed block parser rejects duplicate or unclosed MIRA markers instead of silently damaging host instructions.
- Redaction returns deterministic placeholders such as `[REDACTED_API_KEY]` and never logs the original value.

- [ ] **Step 1: Write failing table tests** for insertion into empty text, replacement of an existing block, preservation of prefix/suffix bytes, removal, duplicate markers, unclosed markers, API keys, bearer tokens, passwords, PEM private keys, cookies, and short/transient prompts.
- [ ] **Step 2: Run** `go test -tags fts5 ./internal/agentinstall -run 'TestManaged|TestRedact|TestSubstantive' -count=1` and verify the new tests fail.
- [ ] **Step 3: Implement** marker-aware merge/remove functions that preserve line endings and return an error for ambiguous marker state. Implement compiled regular expressions for common secrets plus key/value patterns; redact before any call to `StoreMemory` or recall rendering.
- [ ] **Step 4: Implement** substantive filtering with Unicode rune count, whitespace normalization, minimum length, and a small transient-command/list heuristic. Keep the heuristic conservative so explicit decisions, questions and constraints pass.
- [ ] **Step 5: Run** `gofmt -w internal/agentinstall/*.go` and `go test -tags fts5 ./internal/agentinstall`.
- [ ] **Step 6: Commit** `feat: add managed agent instructions and local redaction`.

### Task 3: Short-lived agent event bridge and bounded injection

**Files:**
- Create: `internal/agentbridge/event.go`
- Create: `internal/agentbridge/event_test.go`
- Modify: `cmd/mira/hook.go`
- Modify: `internal/app/main.go` only if a narrow constructor/accessor is required by the bridge

**Interfaces:**
- Produces `agentbridge.Event{Client, EventName, Role, Content, SessionID, ThreadID, Wing, Room, ManifestPath}` and `agentbridge.Result{Continue, Context, Captured, Diagnostics}`.
- Produces `agentbridge.Handle(ctx, event) (Result, error)`; operational recall/capture failures are represented as diagnostics and fail open unless the event is malformed.
- Reuses `RecallMemoryUC`, `StoreMemoryUC`, and the existing agentmemory runtime instead of duplicating extraction, soul budgets, or lifecycle logic.

- [ ] **Step 1: Write failing tests** for session-start recall, prompt-submit capture, response-complete capture only under `complete`, `<MIRA_CONTEXT trust="reference-only">` delimiters, separate identity/evidence budgets, secret-free output, and fail-open recall/storage errors.
- [ ] **Step 2: Run** `go test -tags fts5 ./internal/agentbridge ./cmd/mira -run 'TestAgentEvent|TestHook' -count=1` and verify failure.
- [ ] **Step 3: Implement** manifest loading and event normalization. For `session-start` and `prompt-submit`, call recall with the configured wing and bounded budget; for capture events, apply `RedactSecrets`, `IsSubstantivePrompt`, role rules, and session/content deduplication before `StoreMemoryUC.Execute`.
- [ ] **Step 4: Implement** optional assistant observation through the existing soul runtime as evidence only; pass `auto_apply_traits=false` through the bridge and never call an identity patch from an event.
- [ ] **Step 5: Add** the machine-readable command `mira agent event --client <name> --event <session-start|prompt-submit|response-complete> --manifest <path>`; it reads stdin JSON, writes only the client-neutral result required by the adapter, disables background services using the existing `prepareHookConfig`, and never writes captured content to stderr.
- [ ] **Step 6: Run** focused tests plus `go test -tags fts5 ./internal/agentmemory ./internal/usecases/interactors ./cmd/mira`.
- [ ] **Step 7: Commit** `feat: add fail-open autonomous agent event bridge`.

### Task 4: Client adapters and safe configuration merges

**Files:**
- Create: `internal/agentinstall/adapters.go`
- Create: `internal/agentinstall/adapters_test.go`
- Modify: `cmd/mira/main.go` setup helpers or extract them into `internal/agentinstall` without changing existing command behavior
- Modify: `cmd/mira/hook.go` client event wiring

**Interfaces:**
- Produces `InstallRequest{ProjectRoot, Scope, Manifest, BinaryPath, MiraConfigPath, ClientConfigPath, DryRun, Force}`, `DoctorRequest{ProjectRoot, Scope, ManifestPath, ClientConfigPath}`, `UninstallRequest{ProjectRoot, Scope, ManifestPath, ClientConfigPath, DryRun}`, and `Adapter` with `Name()`, `Capabilities()`, `Install(ctx, InstallRequest) (AdapterResult, error)`, `Doctor(ctx, DoctorRequest) (AdapterResult, error)`, and `Uninstall(ctx, UninstallRequest) (AdapterResult, error)`.
- `AdapterResult` contains `ChangedFiles`, `Warnings`, `Errors`, and `Capabilities`; file and command details are represented as redacted, human-readable operations so dry-run and doctor output share one result shape.
- Each adapter returns capability state: MCP registration, managed instruction path, deterministic capture, deterministic injection, and human-readable limitations.

- [ ] **Step 1: Write failing tests** for Codex `AGENTS.md`, Claude Code `CLAUDE.md`, Windsurf MCP/hooks, Cursor `.cursor/mcp.json` plus `.cursor/rules/mira.mdc`, and Claude Desktop platform MCP config. Assert unrelated settings remain byte-for-byte semantically intact and repeated installation does not duplicate entries.
- [ ] **Step 2: Run** `go test -tags fts5 ./internal/agentinstall ./cmd/mira -run 'Test.*Adapter|Test.*Setup' -count=1` and verify failure.
- [ ] **Step 3: Implement** adapter capability descriptors and configuration merges. Reuse the existing official Codex/Claude CLI registration helpers and JSON merge logic; add managed instruction merge/removal through Task 2. Resolve project-scoped instruction files from the project root (`AGENTS.md`, `CLAUDE.md`, Cascade rules, or `.cursor/rules/mira.mdc`) and user-scoped files from the platform config directory. Use temp-file plus rename for files written by MIRA.
- [ ] **Step 4: Implement** only the deterministic hooks already supported by each client. Cursor and Claude Desktop must report instruction-guided fallback rather than claim event interception.
- [ ] **Step 5: Add** `--dry-run` adapter output that redacts credential-shaped values and states exactly which files and commands would change.
- [ ] **Step 6: Run** `gofmt -w internal/agentinstall/*.go cmd/mira/*.go`, then `go test -tags fts5 ./internal/agentinstall ./cmd/mira`.
- [ ] **Step 7: Commit** `feat: add native agent client adapters`.

### Task 5: `mira agent` CLI lifecycle and backward compatibility

**Files:**
- Create: `cmd/mira/agent.go`
- Create: `cmd/mira/agent_test.go`
- Modify: `cmd/mira/main.go`
- Modify: `cmd/mira/main_test.go`

**Interfaces:**
- Adds `mira agent install`, `mira agent doctor`, `mira agent status`, and `mira agent uninstall` without removing `mira setup` or hidden hook commands.
- `install` accepts `--client auto|codex|claude-code|windsurf|cursor|claude-desktop`, `--scope project|user`, `--policy minimal|standard|complete`, `--wing auto|<name>`, `--mira-config`, `--client-config`, `--dry-run`, and `--force`.
- `doctor` exits non-zero on malformed manifest, missing required client/MCP configuration, or unavailable local MIRA binary; it reports instruction-guided fallback as a warning, not an error.

- [ ] **Step 1: Write failing CLI tests** using temporary project/home directories for auto client detection, install, repeated install, status, doctor, dry-run, and uninstall. Assert no write occurs in dry-run and unrelated settings survive.
- [ ] **Step 2: Run** `go test -tags fts5 ./cmd/mira -run 'TestAgent' -count=1` and verify failure.
- [ ] **Step 3: Implement** `newAgentCmd` and subcommands, call the manifest and adapter packages, and register the command in `main()` alongside `newSetupCmd()`.
- [ ] **Step 4: Preserve** the old `setup` semantics and make its new functionality opt-in; do not silently turn existing `mira setup` calls into full automatic capture for users who did not request it.
- [ ] **Step 5: Add** clear output listing policy, wing, config paths, MCP status, hook capability, recall injection capability, and next action. Keep diagnostics metadata-only.
- [ ] **Step 6: Run** `gofmt -w cmd/mira/*.go`, `go test -tags fts5 ./cmd/mira ./internal/agentinstall ./internal/agentbridge`, and existing install-script tests.
- [ ] **Step 7: Commit** `feat: add autonomous agent install lifecycle`.

### Task 6: End-to-end verification, documentation and migration guidance

**Files:**
- Modify: `README.md`
- Modify: `README_FR.md`
- Modify: `SKILL.md`
- Modify: `docs/FEATURES.md`
- Modify: `docs/API_REFERENCES.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `config.example.yaml`
- Create: `docs/agent-integration.md`
- Test: `tests/install_scripts.test.mjs` if installer documentation or assertions change

- [ ] **Step 1: Write documentation examples** for the recommended one-command installation, policies, `agent doctor`, client limitations, uninstall, and secret/redaction behavior. Link to the design contract and do not claim deterministic hooks for fallback clients.
- [ ] **Step 2: Add** an architecture diagram showing installation → MCP → managed instructions → event bridge → recall/capture/soul evidence.
- [ ] **Step 3: Run** `go test -tags fts5 ./...` and `node --test tests/install_scripts.test.mjs` from the MIRA repository.
- [ ] **Step 4: Run** `git diff --check`, inspect both the generated client configs in temp fixtures and the full CLI help output for `mira agent`.
- [ ] **Step 5: Commit** `docs: document autonomous agent integration`.
- [ ] **Step 6: Perform final verification** with `git status --short`, `go test -tags fts5 ./...`, and a dry-run install for each supported client. Do not create a release as part of this feature.

## Final review checklist

- Every task preserves the existing MIRA memory/soul contracts and avoids a database migration.
- The standard policy is genuinely automatic where a client hook exists and honestly reports instruction-guided fallback otherwise.
- Repeated install/uninstall operations are reversible and cannot delete user-authored instruction content.
- Secret redaction is tested before any storage or injection path.
- Full Go tests and install-script tests pass before the feature is described as complete.

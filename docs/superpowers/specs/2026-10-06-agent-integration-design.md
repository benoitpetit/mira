# Agent Integration Reliability Design

**Date:** 2026-10-06  
**Status:** Approved in conversation; implementation plan pending review

## Goal

Make MIRA a reliable, project-scoped memory layer for every supported agent.
An installation must expose the real MCP tools, give the agent durable usage
guidance, and automate recall or capture only where the client protocol can
actually support it.

## Problems to solve

- The generic agent bridge serializes a `Result` JSON object, while Codex and
  Claude Code require client-specific `hookSpecificOutput.additionalContext`
  to inject recalled context.
- Codex project installations currently register MCP and hooks in user-level
  locations.
- `mira setup --automatic-memory` bypasses the bridge's secret redaction and
  substantive-content filtering.
- `session_start` is enabled in the manifest but no installed hook invokes it.
- The root `SKILL.md` is documentation, not an installed native skill, and
  describes tools that are not consistently available through the MCP server.
- Cursor, Windsurf, and Claude Desktop are described as if native event
  interception were equivalent across them. It is not.

## Architecture

### Neutral bridge

`internal/agentbridge` remains the sole owner of memory policy: redaction,
substantive-content filtering, deduplication, capture, recall budget, and
reference-only rendering. It returns a typed, client-neutral result.

### Client adapters

Each supported client has an adapter declaration defining:

- MCP configuration location and scope;
- managed instruction location and native skill location;
- supported lifecycle events;
- recall delivery mode: hook, extension, or skill-guided;
- capture delivery mode: hook, extension, or none;
- configuration parser/serializer and uninstall ownership marker.

The command layer uses the declaration to install and inspect a client. The
bridge result is rendered by a dedicated output adapter rather than emitted as
generic JSON.

### Integration modes

| Mode | Meaning |
| --- | --- |
| `hook` | MIRA injects reference-only recall or captures content through a verified client hook. |
| `extension` | A project-local extension invokes MIRA around the client lifecycle. |
| `skill-guided` | The installed skill and instructions tell the agent to call `mira_recall` and `mira_store`; no automatic interception is claimed. |

`mira agent status` and `mira agent doctor` report the actual recall and
capture modes independently.

## Client contracts

### Codex

- Project scope writes `.codex/config.toml` and `.codex/hooks.json`.
- User scope writes `~/.codex/config.toml` and `~/.codex/hooks.json`.
- `SessionStart` and `UserPromptSubmit` use the Codex `additionalContext`
  output schema. `Stop` captures assistant output only for `complete`.
- Doctor reports the trust requirement for non-managed hooks and verifies the
  project or user files selected by the manifest.

### Claude Code

- Project scope uses `.mcp.json` and `.claude/settings.json`; local scope uses
  `.claude/settings.local.json`; user scope uses the user config.
- `SessionStart` and `UserPromptSubmit` return Claude's
  `hookSpecificOutput.additionalContext` shape.
- `Stop` captures assistant output only for `complete`.

### Windsurf

- Native `pre_user_prompt` and `post_cascade_response` hooks are capture-only.
- Recall is `skill-guided`; the rule and native skill direct the agent to use
  `mira_recall` before substantive work.
- Complete policy warns that Cascade response payloads can contain tool
  trajectories and remains opt-in.

### Cursor and Claude Desktop

- They receive project MCP configuration plus correctly activated instructions
  or native skills where supported.
- Their mode is `skill-guided`; no hook-based automatic recall/capture claim.
- Cursor rules include valid MDC frontmatter with `alwaysApply: true`.

### Hermes

- Register a local stdio server in `~/.hermes/config.yaml` for user scope.
- Install project instructions and a portable MIRA skill for agent behavior.
- Start with `skill-guided` recall and capture: do not install shell hooks
  until Hermes has a stable, documented event-output contract that can add
  context without risking content loss.

### OpenCode

- Project scope writes `opencode.json` and `.opencode/skills/mira/SKILL.md`.
- Use `AGENTS.md` for persistent project instruction.
- Register MIRA as a local server under `mcp.servers.mira` with `codemode:
  false`, so its tools remain directly callable.
- Start `skill-guided`; no custom plugin is introduced in this release.

### Pi Agent

- Project scope writes `.pi/mcp.json`, `.pi/APPEND_SYSTEM.md`, and
  `.pi/skills/mira/SKILL.md`.
- Register MIRA through the Pi project MCP configuration.
- Start `skill-guided`; a Pi extension is deliberately deferred because it
  executes arbitrary TypeScript in the agent process and needs its own
  security review and versioned distribution model.

## Security and data rules

- All automatic capture paths call the same bridge code before storage.
- Capture redacts secrets before extraction and storage.
- Recalled content is redacted again and wrapped in
  `<MIRA_CONTEXT trust="reference-only">`.
- A project installation must not reference a configuration, manifest, hook,
  or database path belonging to another project.
- Destructive MCP tools retain client approval behavior; MIRA's tool
  descriptions clearly identify permanent deletion.

## Native skill

The portable source skill is `SKILL.md`. Installation copies a generated,
managed version into the client-native skill directory when that directory is
supported. The generated skill only names tools registered by the active MCP
surface and states when to recall, store, load, and avoid secrets. The root
skill remains the distributable source of truth.

## Verification

- Unit tests cover every client path, scope, generated instruction, generated
  skill, MCP configuration, and hook configuration.
- Bridge tests cover redaction and every client hook output shape.
- Command tests verify `doctor` reports the actual installed modes and missing
  artifacts.
- Documentation tests or snapshots keep client support tables aligned with the
  declaration.
- Run focused Go tests during each task, then the core test suite before
  release. Build the landing page after its copy is changed.

## Documentation and landing page

Update README, French README when the integration section is mirrored,
`SKILL.md`, `docs/agent-integration.md`, feature and architecture references,
API command references, and the landing-page installation section. They must
show supported clients, installation scope, actual automation modes, and the
recommended lifecycle command.

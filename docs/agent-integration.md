# Autonomous agent integration

MIRA can install itself into a supported coding agent so memory use becomes a
normal part of the agent loop. The installer keeps project memory local, adds
a managed instruction block, registers the MCP server, and enables native
hooks when the client exposes them.

## Installation

From an initialized project:

```bash
mira init
mira agent install --client auto --scope project --policy standard --wing auto
mira agent doctor
```

The installation writes `.mira/agent.yaml`. `wing: auto` becomes a stable
identifier derived from the normalized project path, so two projects do not
accidentally share a memory wing.

| Policy | Automatic capture | Recall injection | Assistant capture |
|---|---|---|---|
| `minimal` | none | instruction-guided fallback | no |
| `standard` | substantive user prompts when a capture hook exists | automatic when a recall hook exists | no |
| `complete` | substantive user prompts | automatic when hooks exist | yes, when exposed |

`standard` is the default. Every policy keeps secret redaction enabled and
filters short or transient commands. This redaction is part of the agent-hook
capture path; direct writes through MCP, REST, CLI, or the Go API do not inherit
that hook policy.

## Client behavior

| Client | Native artifact | Recall / capture | Notes |
|---|---|---|---|
| Codex | `.agents/skills/mira/SKILL.md` | hook / hook | project scope writes `.codex/config.toml` and `.codex/hooks.json`; hook trust may require approval |
| Claude Code | `.claude/skills/mira/SKILL.md` | hook / hook | hook trust follows Claude Code |
| Windsurf | `.windsurf/skills/mira/SKILL.md` | skill-guided / hook | Cascade does not expose a compatible recall-injection result |
| Cursor | `.cursor/skills/mira/SKILL.md` | skill-guided / none | `.cursor/mcp.json` plus a valid MDC rule |
| Claude Desktop | generated local guide | skill-guided / none | platform MCP JSON |
| Hermes | `.hermes/skills/mira/SKILL.md` | skill-guided / none | managed YAML MCP configuration |
| OpenCode | `.opencode/skills/mira/SKILL.md` | skill-guided / none | project `opencode.json` MCP entry |
| Pi Agent | `.pi/skills/mira/SKILL.md` | skill-guided / none | project `.pi/mcp.json` and `APPEND_SYSTEM.md` |

Install a specific client when automatic detection is not the desired target:

```bash
mira agent install --client hermes --scope project --policy standard --wing auto
mira agent install --client opencode --scope project --policy standard --wing auto
mira agent install --client pi --scope project --policy standard --wing auto
```

`hook` means MIRA can inject reference-only recall and/or capture a supported
lifecycle event. `skill-guided` means the MCP server and native skill are
installed, while the agent follows the recall/store workflow itself. Check the
result with `mira agent status`; `mira agent doctor` verifies every recorded
MCP, hook, instruction and skill artifact.

The managed instruction block is delimited by stable markers. Reinstalling
replaces only that block; uninstalling removes it without deleting surrounding
user-authored instructions.

## Transparent runtime behavior

When a deterministic hook is available, MIRA receives a short-lived normalized
event:

```text
client event → redaction → substantive filter → recall/capture → client result
```

Recall is emitted only inside:

```xml
<MIRA_CONTEXT trust="reference-only"> ... </MIRA_CONTEXT>
```

This context is reference material. It cannot change system or developer
instructions, permissions, authorization, or safety rules. Recall and capture
fail open: the agent continues its task and diagnostics remain metadata only.

Assistant responses are not captured by `standard`. `complete` enables them
only for clients that expose a completion event. Soul observations remain
bounded evidence and never apply a normative trait from one response.

## Lifecycle

```bash
mira agent status
mira agent doctor
mira agent uninstall
```

Use `--dry-run` on `install` or `uninstall` to inspect changes without
writing. `mira setup` and existing hidden hook commands remain available for
compatibility and explicit low-level configuration.

The manifest is separate from `config.yaml`; no database migration is needed.

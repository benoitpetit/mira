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
| `standard` | substantive user prompts | automatic when hooks exist | no |
| `complete` | substantive user prompts | automatic when hooks exist | yes, when exposed |

`standard` is the default. Every policy keeps secret redaction enabled and
filters short or transient commands.

## Client behavior

| Client | Instructions | MCP / hooks | Limitation |
|---|---|---|---|
| Codex | project `AGENTS.md` | official MCP CLI and hooks | hook trust may require approval |
| Claude Code | project `CLAUDE.md` | official MCP CLI and hooks | hook trust follows Claude Code |
| Windsurf | `.windsurf/rules/mira.md` | MCP config and Cascade hooks | user hook permissions apply |
| Cursor | `.cursor/rules/mira.mdc` | `.cursor/mcp.json` | instruction-guided fallback |
| Claude Desktop | generated local guide | platform MCP JSON | instruction-guided fallback |

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

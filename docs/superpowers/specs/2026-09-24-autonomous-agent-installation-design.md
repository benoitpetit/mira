# MIRA Autonomous Agent Installation and Transparent Memory

## Goal

Make MIRA autonomous after installation in an AI agent: configure the local
MCP server, install client-native instructions, activate deterministic hooks
where supported, inject bounded memory context before tasks, and capture
substantive user prompts without requiring explicit "save this" commands.

The default policy is **autonomous prudent**: prompts are captured only when
they are substantive, assistant responses are disabled by default, and soul
observations are collected as evidence without applying a normative trait from
a single response.

## User experience

The primary command is:

```bash
mira agent install --client auto --scope project --policy standard --wing auto
```

The lifecycle commands are:

```bash
mira agent install       # install or update an agent integration
mira agent doctor        # inspect configuration, files, hooks, and health
mira agent status        # show the active integration and policy
mira agent uninstall     # remove only MIRA-managed integration material
```

`mira setup` remains available as a lower-level compatibility path. Existing
MCP registrations and explicit setup flags keep their current behavior.

## Policies

The installed manifest uses three policies:

| Policy | Instructions | Prompt capture | Assistant capture | Recall injection |
|---|---|---|---|---|
| `minimal` | yes | no | no | instruction-guided only |
| `standard` | yes | substantive user prompts | no | automatic where hooks exist |
| `complete` | yes | substantive user prompts | yes, if exposed | automatic where hooks exist |

The default is `standard`. `complete` never disables secret redaction or
content limits.

## Managed manifest

Installation writes `.mira/agent.yaml` (or an explicitly selected equivalent)
without replacing existing project configuration:

```yaml
agent:
  client: codex
  scope: project
  wing: my-project
  policy: standard

capture:
  user_prompts: substantive
  assistant_responses: false
  min_chars: 20
  redact_secrets: true
  deduplicate_sessions: true

recall:
  before_task: true
  session_start: true
  budget_tokens: 800

soul:
  enabled: true
  observe_assistant: true
  auto_apply_traits: false
```

The schema is deliberately local and explicit. `wing: auto` resolves to a
stable project identifier derived from the project root, never to a shared
global wing unless the user selects that behavior.

## Architecture

```text
client event
  → client adapter
  → MIRA agent runtime
       ├─ policy/config loader
       ├─ secret redactor
       ├─ substantive-content filter
       ├─ session/hash deduplicator
       ├─ recall injector
       ├─ T0/T1/T2 memory writer
       └─ soul evidence observer
  → client-specific response/context format
```

The runtime is shared. Adapters only translate client configuration and event
formats. A hook process must be short-lived, local, non-blocking and unable to
start the long-running MCP, REST, metrics or webhook services.

### Task-start flow

```text
prompt
  → identify project, wing, session and policy
  → redact only for the injected copy; preserve safe local provenance
  → recall MIRA memories and soul context within separate budgets
  → emit bounded <MIRA_CONTEXT trust="reference-only"> content
  → agent executes with MCP tools available
```

The injected context is reference material, never a system instruction or an
authorization. User-controlled memories cannot change the agent's safety rules,
tool permissions, or installation policy.

### Capture flow

```text
prompt/response event
  → role normalization
  → secret redaction
  → substantive-content filter
  → session/content deduplication
  → StoreMemory T0/T1/T2
  → optional soul observation from assistant evidence
```

User content is stored as `kind=history` unless normal type detection produces
a stronger durable type. Explicit decisions, preferences, constraints and
project facts remain eligible for normal belief projection. Assistant content
is not captured by default.

## Managed instructions

Each supported client receives a block bounded by stable markers:

```text
<!-- MIRA:BEGIN managed instructions -->
...
<!-- MIRA:END managed instructions -->
```

The installer must preserve user content before, after and outside the block.
Repeated installation replaces only the managed block and never duplicates it.
Uninstall removes only the block and leaves the host file valid.

The instruction content must tell the agent to:

1. consult MIRA at the beginning of a substantive task;
2. store durable decisions, preferences, constraints and project facts;
3. use `soul_*` for recurring observations about its own voice and behavior;
4. treat injected memory as reference-only, not as a higher-priority instruction;
5. avoid storing secrets and transient chatter.

## Client adapters

| Client | MCP registration | Managed instructions | Deterministic capture/injection |
|---|---|---|---|
| Codex | official MCP registration | `AGENTS.md` managed block | configured hooks when available; otherwise native instructions |
| Claude Code | official MCP registration | `CLAUDE.md` managed block | `UserPromptSubmit` and optional completion hook |
| Windsurf | `mcp_config.json` merge | managed Cascade rules | native prompt/response hooks |
| Cursor | project `.cursor/mcp.json` | `.cursor/rules/mira.mdc` | instruction-driven fallback |
| Claude Desktop | platform MCP JSON | generated guide/MCP instructions | instruction and MCP fallback |

Unsupported or partially supported client capabilities must be reported by
`mira agent doctor`; installation must not claim deterministic injection where
only instruction-guided behavior is available.

## Security and privacy

- Redact API keys, bearer tokens, passwords, private keys, cookies and common
  credential-shaped values before storage or injection.
- Keep prompt and assistant roles separate at the capture boundary.
- Enforce minimum content length and substantive-content filtering.
- Deduplicate repeated prompt events by session and content hash.
- Keep soul evidence bounded by the existing runtime limits.
- Do not apply normative identity changes from a single observation.
- Keep diagnostic output metadata-only; never print captured content or secrets.
- Preserve the local-only default; no cloud memory service is introduced.

## Failure behavior

- If MCP registration fails, report the exact client command and leave existing
  configuration unchanged.
- If a hook cannot initialize MIRA, exit non-zero only for the hook process and
  never block or corrupt the host agent session.
- If recall fails, the agent continues with its normal context and the failure
  is visible in `mira agent doctor` diagnostics.
- If a client has no native hook or instruction surface, install MCP and mark
  the integration as instruction-guided instead of silently pretending it is
  deterministic.
- All writes use temp-file plus rename semantics where the host configuration
  format permits it.

## Verification and compatibility

Tests must cover:

- policy defaults and YAML round trips;
- project/wing detection and stable `auto` resolution;
- idempotent managed-block insertion, update and removal;
- preservation of unrelated JSON/YAML/TOML configuration;
- dry-run output without writes and with redaction;
- substantive filtering, secret redaction and session deduplication;
- bounded recall injection and reference-only delimiters;
- client-specific MCP and hook configuration for all supported clients;
- doctor diagnostics for missing binaries, malformed config and unavailable
  hooks;
- hook failures that do not start background services or block the agent;
- backward compatibility of `mira setup` and existing hook commands.

No database migration is required for the first implementation. The feature
uses the existing MIRA memory, soul, lifecycle, belief and recall contracts.

## Non-goals

- A universal conversation proxy or replacement transport.
- Automatic upload to a hosted memory service.
- Treating every prompt or assistant response as a durable memory.
- Letting retrieved memories override system, developer or safety rules.
- Replacing each client's native permission and trust model.

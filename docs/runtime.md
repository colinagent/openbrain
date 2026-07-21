# OpenBrain Runtime

This document is the public design source for the OpenBrain runtime. Private
deployment and release operations do not belong here.

## Role

The runtime is the local Go process that scans node packages, assembles agent
prompts, starts endpoint agents and tool servers, stores sessions, manages
thread state, and dispatches agent and tool calls.

The runtime code lives at the repository top level:

```text
openbrain/
  opagent-runtime/
    cmd/opagent-runtime/
    cmd/opagent-bootstrap/
    internal/
    packages/
  agents/
    coder/
    simple-memory/
    gbrain/
  skills/
    openbrain-cloud-sync/
  tools/
    gbrain-cloud/
```

The old `runtime/opagent-runtime` layout is retired. Runtime build and test
commands should use `opagent-runtime` directly.

## Local Workspace Binding Index

The runtime keeps a local binding index at
`~/.openbrain/index/workspaces.json`. This file maps OpenBrain workspace
records to local or remote workspace paths used by runtime APIs, storage sync,
and source recovery.

The index is account-scoped. Version 2 stores entries under the signed-in
OpenBrain user id:

```json
{
  "version": 2,
  "accounts": {
    "user-example": {
      "workspaces": [],
      "hiddenWorkspaces": []
    }
  }
}
```

Runtime APIs read only the current account bucket from local auth state. When no
user is signed in, source/cache views and background sync bindings return an
empty view instead of exposing bindings from another account. If a folder path
is already bound in a different account bucket, the runtime reports an explicit
takeover conflict before moving the binding.

Related public docs:

- `docs/skills.md`: skill packages, mounting, and prompt-time behavior
- `docs/tools.md`: tool-server manifests, mounting, and MCP refresh behavior
- `docs/subagent.md`: subagent mounting and `agent_task` delegation
- `docs/opagent-protocol.md`: shared protocol types, transports, and opcodes

## Node Scanning

### Mental Model

OpenBrain packages are Markdown manifests. The runtime scanner turns those
files into protocol `OpNode` records that the rest of the system caches,
starts, and dispatches against.

Each node has:

- a stable or generated `id`;
- a manifest `uri` pointing at the source file;
- a working directory `cwd` used to resolve relative paths;
- kind-specific metadata parsed from YAML front matter;
- optional `run` and `opcodes` fields for endpoint agents and tool servers.

The scanner does not rewrite manifests on disk. It only reads them.

### Refresh Order

On refresh, runtime scans the configured base directory in a fixed order:

```text
1. tools/   -> tool servers must exist before agents resolve tool refs
2. skills/  -> global skills must exist before agents resolve @skills/... refs
3. agents/  -> agent manifests resolve tools, skills, and subagents last
```

Implementation: `opagent-runtime/internal/core/node_refresh.go`.

This ordering matters because agent manifests often reference nodes discovered
earlier in the same refresh cycle.

### Node Kinds

| Manifest file | Node kind | Typical role |
| --- | --- | --- |
| `AGENT.md` | `agent` | Prompt, opcodes, mounted tools/skills/subagents, optional endpoint run config |
| `SKILL.md` | `skill` | Reusable workflow instructions the model reads on demand |
| `TOOL.md` | `tools` | MCP or process-backed tool server |

### Base Directory Layout

The scanner expects top-level package directories under the runtime base dir:

```text
<baseDir>/
  agents/
  skills/
  tools/
  configs/
```

Built-in OpenBrain packages ship under the repository's managed runtime bundle.
User-installed packages also live under the local OpenBrain data directory with
the same shape.

### Agent Package Layout

Prefer one agent package per directory:

```text
agents/<name>/
  .agent/
    AGENT.md
    bin/                 # optional endpoint binary
    skills/              # optional agent-local skills
    tools/               # optional agent-local tool servers
    subagents/           # optional nested agent packages
```

Legacy `AGENTS.md` is still discovered, but new packages should use
`.agent/AGENT.md`.

For this layout:

- `uri` points at the manifest file.
- `cwd` is `agents/<name>`.
- `agentHome` prompt variable resolves to `agents/<name>/.agent`.
- `agentRoot` prompt variable resolves to `agents/<name>`.
- Relative `tools:`, `skills:`, `subagents:`, and `run.command` entries resolve
  inside `agents/<name>/.agent`.

Workspace binding can expose the same logical agent at a workspace path using
path-aware dedup during directory scans. Shared built-in agents should still use
explicit ids so references stay stable across workspaces.

### AGENT.md Manifest

Common front matter fields:

| Field | Purpose |
| --- | --- |
| `id` | Stable runtime node id such as `agent-coder` |
| `name` | Display name |
| `description` | Short summary for UI and prompt appendices |
| `tags` | Free-form labels |
| `opcodes` | Supported runtime operations such as `thread/submit` and `prompt/get` |
| `tools` | System tools (`read`, `shell`, ...) and/or mounted tool servers |
| `skills` | Mounted skill nodes |
| `subagents` | Mounted agent nodes eligible for runtime delegation |
| `model` | Optional local `models.json` model key override for this agent |
| `run` | Endpoint connection config: local command or remote URL, optional daemon mode |
| `bind` | Alias one agent manifest to another scanned agent node |

Example:

```yaml
---
id: agent-coder
name: coder
description: Expert coding assistant for OpenBrain development and debugging.
tags: builtin
opcodes:
  - thread/submit
  - prompt/get
subagents:
  - "@agent-gbrain"
run:
  command: ["bin/coder"]
  daemon: true
---
```

### Run Endpoint Modes

`run` describes how runtime connects to an endpoint agent or tool server.
It has two mutually exclusive endpoint forms:

```yaml
run:
  command: ["bin/coder"]
  daemon: true
```

```yaml
run:
  url: "https://api.op-agent.com/brain/mcp"
  daemon: true
  header:
    Authorization: "Bearer {openbrain_session}"
```

Rules:

- Use `run.command` for local stdio processes.
- Use `run.url` for remote MCP streamable HTTP endpoints.
- Do not set `run.command` and `run.url` together.
- `run.header` is only valid with `run.url`; omit it when no HTTP headers are
  needed.
- Header values are plain strings. Runtime replaces `{openbrain_session}` with
  the logged-in OpenBrain session token from local user auth state.
- `run.daemon: true` means runtime keeps the endpoint connection alive between
  calls. `false` or omitted means the endpoint is not cached as a long-lived
  daemon.
- Do not put schedules in `run`. Scheduled work lives in the dedicated task
  system, not in node manifests.

Subagent behavior is documented in `docs/subagent.md`.

### Model Selection

Runtime model selection is explicit and meta-driven:

- Top-level callers must pass the resolved chat model as `meta.modelKey`.
- Runtime does not read Default Chat Model and does not choose a first enabled
  model for agent turns.
- Callers that use a user default must resolve it before calling runtime, then
  send the concrete `modelKey`.
- Runtime validates `meta.modelKey` against local `models.json` before
  dispatch. Disabled, missing, or unusable entries fail the turn with a clear
  error and a configuration message when a thread is available.
- New callers must not emit legacy `meta.model`; runtime strips it from active
  turn meta.
- Runtime does not write the selected model back to `AGENT.md` or `models.json`.
- Enabled models are user/config choices; runtime availability still depends on
  a usable local provider/API configuration for the concrete `modelKey`.

When a parent delegates through `agent_task`, the child uses its own
`AGENT.md model:` when present. Otherwise runtime inherits the parent loop's
explicit `modelKey`. Endpoint agents receive only the resolved model key in
call meta, usually as `_meta.modelKey`, so daemon agents can forward it when
they delegate back into runtime.

### Thread Snapshot Reads

Thread JSONL is the durable source of truth. Runtime keeps full-context readers
for execution, replay, compaction, and continuation, but `thread/snapshot/get`
is a render-facing read path and returns only a bounded `entries[]` window. The
default snapshot window is `tail(limit=400)`, and callers can request older or
newer ranges with `before(anchorId)` or `after(anchorId)`. Snapshot metadata
still aggregates queue state, message records, channel summaries, tail state,
and the full-file `revision` while only returning the requested entry window.

### Streaming Turn Finalization

For streaming model APIs, the provider's accumulated stream partial is the
source of truth for terminal assistant content. A provider may use upstream
completed payloads to fill block metadata, usage, stop reason, response ids, or
completed tool-call arguments, but it must not let an empty or partial completed
payload erase text or reasoning already emitted as stream deltas.

Runtime treats the terminal provider response as the durable assistant message.
If that terminal response contains semantic blocks such as thinking or tool
calls but is missing visible text that was already streamed, runtime merges the
missing partial text before the first tool call. If the stream errors or is
cancelled after semantic partial content was emitted, runtime can persist a
terminal assistant message derived from the partial instead of falling back to
a text-only error placeholder. Visible text is retained; reasoning blocks are
retained for aborted or failed turns so replay metadata remains available.
Stream error details live in the canonical message raw payload, not in a new
wire field.

Markdown chat files remain projections. They are updated from terminal turn
results, not from live deltas. When a failed or aborted turn has reasoning but
no visible assistant text, the projection writes a fixed interruption/failure
notice while the durable canonical message keeps the reasoning content.

### Reference Resolution

Manifest list fields such as `tools:`, `skills:`, and `subagents:` accept three
reference shapes:

| Shape | Example | Meaning |
| --- | --- | --- |
| Bare name | `read`, `shell` | Built-in systool name; only valid in `tools:` |
| Base-dir ref | `@tools/gbrain-cloud`, `@skills/search`, `@agent-gbrain` | Resolve under `<baseDir>/{tools,skills,agents}/...` or by explicit node id |
| Path ref | `./skills`, `./tools/research-tools`, `~/packages/foo/SKILL.md` | Resolve relative to the agent resource dir, or as an absolute/`~` path |

Path refs to directories scan that directory for the requested node kind:

- `./skills` scans child folders containing `SKILL.md`
- `./tools/research-tools` resolves `TOOL.md` in that folder or scans tool dirs

Path refs to a manifest file mount that single node.

Prefer directory refs for large agent-local skill packs. See `docs/skills.md`.
Tool-server refs follow the same model. See `docs/tools.md`.

Built-in systool is not a tool-server package. When an agent omits `tools:`,
runtime exposes built-in systool tools allowed by the agent's capabilities:
OS tools, `agent_task` when valid subagents are mounted, and `message_*` for
thread-capable agents. Message tools persist their durable effect as
thread/channel state rather than a feature-owned conversation store. Once an
agent declares a `tools:` field, systool becomes an
allowlist and only bare systool names listed there are exposed. To expose no
systool tools, set `"@systool": null` in `AGENT.md`. MCP servers must always be
declared explicitly in `tools:` and are never loaded just because they exist
under `tools/`.

### Skill And Tool Scan Rules

Global skill scanning walks only immediate child directories of `skills/` and
loads `skills/<slug>/SKILL.md`. Nested `references/**/SKILL.md` files are ignored.

Required skill front matter:

- `name`
- `description`

Optional but recommended for shared packages:

- `id: skill-<slug>`

Tool scanning follows the same "one manifest per package directory" rule with
`TOOL.md`.

### Explicit Skills From Runtime Calls

Agent manifests can mount skills permanently with `skills:`. Runtime calls can
also select skills for one turn by putting these values in call meta:

- `selectedSkillIDs`: skill node ids such as `skill-openbrain-cloud-sync`
- `selectedSkillContext`: non-secret structured context for the selected skill

At loop start, runtime resolves `selectedSkillIDs` from the node cache and adds
them to the `Selected Skills` prompt appendix. The model must still read each
selected skill's `SKILL.md` with its normal `read` tool before acting.

Cron tasks are normal runtime callers. When a cron task payload contains
`payload.data.selectedSkillIDs` or `payload.data.selectedSkillContext`,
`buildCronTaskCall` promotes those fields into agent-call meta. The full cron
payload remains available as `payloadJSON`; selected skill context is only for
prompt routing and must not contain secrets.

Scheduled agent jobs must also set `payload.data.modelKey`; runtime promotes it
to call meta before dispatch. If a cron job has no usable chat model, runtime
skips that run and publishes a message asking the user to configure the job.
Managed task creators, including OpenBrain Cloud Sync, must materialize any
default choice into `payload.data.modelKey` before the job reaches runtime.

Each cron task also keeps a local run history in `cron/history/<taskID>.json`.
The runtime exposes it through `cron/history` so the desktop UI can show the
latest execution records without reading task configuration files.

### OpenBrain Cloud Source Cache

Cloud source listing has two local persistence layers with separate roles:

- `.openbrain/cache/cloud-sources.json` is the last successful full Cloud source
  snapshot for the signed-in user and active/default org. Desktop may use it for
  cold-start first paint while it refreshes remote state.
- `.openbrain/index/workspaces.json` is the local binding index. It records
  runtime-local paths, repository identity, hidden sources, and binding
  verification state. It is not a complete Cloud source cache.

Runtime must not use the binding index as a replacement for the latest Cloud
source snapshot. When a fresh Cloud list succeeds, runtime rewrites the
snapshot. Local source mutations update or invalidate the snapshot entries they
affect so the next cold start does not show stale graph state.

### IDs And Conflicts

Manifests may declare stable explicit ids:

```yaml
id: agent-coder
```

Explicit ids must match the node kind prefix: `agent-`, `skill-`, or `tools-`.

When no explicit id is present, runtime falls back to deterministic id
generation from owner, host, kind, URI, and environment. Moving a package can
change fallback ids, so built-in and marketplace packages should always declare
explicit ids.

If two scanned files claim the same explicit id with different URIs, refresh
fails with an id conflict error.

### Org Namespaces

Directories named like `@org-acme/` under `agents/` or `skills/` are treated as
organization namespaces. The scanner walks one level deeper inside them to find
agent or skill packages shared by an org without flattening everything into the
global root.

### What The Scanner Does Not Do

- It does not execute agent prompts or skill bodies during scan.
- It does not inline skill bodies into `AGENT.md`.
- It does not infer skills from Markdown links inside the prompt body.
- It does not treat arbitrary YAML keys in `SKILL.md` as runtime mount config
  unless documented product code reads them. See `docs/skills.md`.

Tool servers use the same scan-and-resolve model with `TOOL.md`. See
`docs/tools.md`. Transport and opcode details live in `docs/opagent-protocol.md`.

## Prompt Assembly

For normal local agents, runtime loads the Markdown body from `AGENT.md`. For
endpoint agents, runtime may call `prompt/get`; when the endpoint returns a
final prompt, runtime uses that result as the base prompt.

Runtime then performs a final global variable expansion pass. This applies to
both local manifest prompts and endpoint `prompt/get` results. Endpoint agents
may pre-expand variables themselves; the runtime pass is intentionally safe and
idempotent for already-expanded text.

Supported prompt variables:

```text
${platform}   Go runtime.GOOS value, such as darwin, linux, or windows
${agentRoot}  agent package root, such as ~/.openbrain/agents/simple-memory
${agentHome}  agent resource dir, such as ~/.openbrain/agents/simple-memory/.agent
```

Runtime never rewrites prompt files on disk during expansion.

After the base prompt is loaded, runtime may append host-owned guidance for
available skills, selected skills, message tools, and mounted subagents.
These appendices are runtime behavior, not content stored in the manifest.

## Managed Assets

Runtime bundles are expected to include the managed files required by
`opagent-bootstrap`, runtime self-update, and desktop local release install:

```text
bin/opagent-runtime[.exe]
bin/opagent-bootstrap[.exe]
bin/gbrain[.exe]
agents/coder/.agent/AGENT.md
agents/coder/.agent/bin/coder[.exe]
agents/simple-memory/.agent/AGENT.md
agents/gbrain/...
agents/opagent-server/.agent/AGENT.md
agents/opagent-server/.agent/bin/openbrain-server[.exe]
skills/openbrain-cloud-sync/SKILL.md
skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper[.exe]
tools/gbrain-cloud/TOOL.md
tools/rg-search/TOOL.md
tools/rg-search/bin/rg[.exe]
configs/config.json
```

The `bin/gbrain[.exe]` asset is built from or downloaded from the external
`colinagent/gbrain` fork; the GBrain source tree is not vendored in this
repository.

`agents/simple-memory` is a normal agent package, not a daemon. It ships only
its manifest and prompt; its runtime state file is created locally when used.

After bundle install/update, runtime projects built-in system tool binaries:
for any top-level `tools/*/TOOL.md` with `tags: system`, every top-level regular
file in that package's `bin/` directory is copied to `<baseDir>/bin`. The same
step persistently appends `<baseDir>/bin` to the current user's PATH. Existing
system commands keep priority in normal terminals because the user PATH entry is
appended; OpenBrain processes can still prefer `<baseDir>/bin` internally.

## Built-In Agents

- `agents/coder`: endpoint coding agent. Runtime-visible ID is `agent-coder`.
  It starts `agents/coder/.agent/bin/coder` and delegates the parent loop back
  through `agent/loop/create`.
- `agents/simple-memory`: reusable memory subagent. Runtime-visible ID is
  `agent-simple-memory`.
- `agents/gbrain`: GBrain-backed knowledge agent.

## Built-In Skills

- `skills/openbrain-cloud-sync`: workflow used by the managed
  `OpenBrain Cloud Sync` cron task. The task targets `agent-coder` and selects
  this skill for the turn. The skill includes a platform-specific helper binary
  for OpenBrain API auth, short-lived workspace git token exchange, temporary
  `GIT_ASKPASS`, standard safe git sync steps, and Cloud Brain sync triggering.

The old `agents/opagent` package and nested memory subagent are retired.

## Build And Test

Go modules are linked by the repository `go.work`. Run tests from module
directories, not from the repository root:

```bash
(cd opagent-runtime && go test ./...)
(cd server && go test ./...)
(cd agents/coder && go test ./...)
(cd skills/openbrain-cloud-sync && go test ./...)
(cd opagent-protocol/go-sdk && go test ./...)
```

Runtime release helpers:

```bash
scripts/openbrain/build-runtime-release.sh
scripts/openbrain/openbrain-run-dev.sh
```

Public-boundary validation:

```bash
scripts/check-openbrain-public-boundary.sh
git diff --check
```

# OpenBrain Tool Servers

This document is the public design source for OpenBrain tool packages and how
the runtime discovers, connects to, and exposes them to agents.

Implementation references:

- Scanner: `opagent-runtime/internal/scan/tools_scan.go`,
  `opagent-runtime/internal/scan/scanner.go`
- Refresh and connection: `opagent-runtime/internal/core/node_refresh.go`,
  `opagent-runtime/internal/core/connection.go`
- Agent loop assembly: `opagent-runtime/internal/core/agent.go`

Related docs: `docs/runtime.md`, `docs/skills.md`, `docs/opagent-protocol.md`.

## Role

A tool server is a `kind=tools` node backed by a `TOOL.md` manifest. At scan
time runtime reads the manifest. At refresh time runtime may connect to the
server, call MCP `tools/list`, and cache the returned tool specs on the node.

Agents do not embed tool schemas in `AGENT.md`. They mount tool servers or name
individual built-in tools in front matter. Runtime assembles the final tool
surface for each agent loop.

## Mental Model

```text
TOOL.md front matter  -> scanner metadata + run endpoint
Runtime refresh        -> connect (when applicable) + ListToolSpecs()
AgentMeta.ToolServers  -> mounted MCP tool-server node IDs
AgentMeta.SysToolMode  -> default | allowlist | disabled
AgentMeta.SysTools     -> allowlisted built-in systool names when mode=allowlist
Agent loop             -> merged ToolSpec map passed to the model
```

Tool servers speak OpAgent Protocol over MCP transports. See
`docs/opagent-protocol.md`.

## Package Layout

Each tool server lives in its own directory:

```text
tools/<name>/
  TOOL.md
  bin/                    # optional packaged command
```

Agent-local tool servers use the same shape:

```text
agents/<name>/.agent/
  tools/
    research-tools/
      TOOL.md
      bin/research-tools
```

Global tool servers live under `<baseDir>/tools/`. Runtime scans them during
the first phase of node refresh, before skills and agents.

`ScanTools` is recursive: it loads `TOOL.md` in the current directory, then
walks child directories. See `Scanner.ScanTools` in
`opagent-runtime/internal/scan/scanner.go`.

## TOOL.md Manifest

Every tool server uses YAML front matter followed by an optional Markdown body.
The scanner requires valid YAML front matter delimited by `---`.

Fields parsed today in `parseRawToolsConfig`:

| Field | Purpose |
| --- | --- |
| `id` | Optional stable node id such as `tools-rg-search`. Must use the `tools-` prefix when present. |
| `name` | Tool-server name stored in `ToolsMeta.Name` |
| `description` | Short summary. `bio` is accepted as an alias and merged with `description`. |
| `tags` | Free-form node tags |
| `run` | Connection endpoint and daemon mode |

### Run Endpoints

`run` is parsed by `ParseRun` in `opagent-runtime/internal/scan/run_parse.go`
and validated by `op.Run.Validate()` in `opagent-protocol/go-sdk/op/agent.go`.

Rules enforced in code:

- `run.command` and `run.url` are mutually exclusive.
- `run.daemon` is an optional boolean. When true, runtime keeps the endpoint
  connection alive between calls.
- `run.header` is only valid with `run.url`.
- Omit `run.header` when the URL endpoint does not need custom HTTP headers.

Relative paths inside `run.command` are resolved against the directory that
contains `TOOL.md` via `ResolveRunPaths`.

At connection time:

- non-empty `run.command` uses stdio transport (`op.CommandTransport`)
- non-empty `run.url` uses HTTP streamable transport
  (`op.StreamableClientTransport`)

See `CreateConnection` in `opagent-runtime/internal/core/connection.go`.

### Headers On URL Endpoints

`run.header` is parsed as a string map and sent on every HTTP streamable
request for `run.url` endpoints. It is not supported for `run.command`.
Use it for bearer tokens, API keys, or other per-request HTTP headers. Missing
or empty `run.header` means no extra headers are sent.

Built-in OpenBrain session headers can use `{openbrain_session}` inside header
values. Runtime resolves it from `<baseDir>/configs/user/auth.json`.

Built-in example:

```yaml
run:
  daemon: true
  url: "https://api.op-agent.com/gbrain/mcp"
  header:
    Authorization: "Bearer {openbrain_session}"
```

### System Tool Bins

Some tool packages ship command-line binaries that OpenBrain should expose to
the user-level OpenBrain bin directory. Mark those packages with `tags: system`
and put the platform binary files directly under the package's top-level `bin/`
directory:

```yaml
---
name: rg-search
description: Packaged local CLI available on the OpenBrain runtime PATH.
tags: system
---
```

```text
tools/rg-search/
  TOOL.md
  bin/rg       # macOS/Linux
  bin/rg.exe   # Windows
```

Install/update code copies every top-level regular file in `tools/<name>/bin/`
to `<baseDir>/bin` when the tool manifest has `tags: system`. Subdirectories
and non-regular files are ignored. The file names are owned by the package
itself and documented in the `TOOL.md` body, not declared in front matter.

Top-level tool packages under `tools/*` that include both `builtin` and
`system` tags are bundled with OpenBrain releases. During install/update, their
`bin/*` files are copied to `~/.openbrain/bin` on macOS/Linux and
`%USERPROFILE%\.openbrain\bin` on Windows. OpenBrain processes can prefer that
directory internally for deterministic product behavior. The persistent user
PATH entry is appended, so an existing system `rg` or other same-named command
keeps priority in the user's normal terminal.

`tags: system` does not control MCP connection behavior. A tool node is
connected only when `run.command` or `run.url` is present. A system tool package
without `run` is cached as a metadata-only node; a system tool package with
`run` is connected like any other MCP tool server.

## Mounting Tools On An Agent

Agents declare tools in `AGENT.md` front matter. Relative paths resolve inside
`agents/<name>/.agent`.

### Supported Reference Forms

| Form | Example | Runtime effect |
| --- | --- | --- |
| Bare name | `read`, `shell`, `write`, `edit`, `agent_task`, `message_publish` | Adds one built-in systool name to `AgentMeta.SysTools` if it is in `op.SystoolNames` |
| Base-dir ref | `@tools/gbrain-cloud` | Resolves under `<baseDir>/tools/<name>/TOOL.md`, or by explicit node id |
| Path ref | `./tools/research-tools`, `~/packages/foo/TOOL.md` | Resolves relative to `.agent`, or as an absolute/`~` path |

Reference classification lives in `classifyRef` and `resolveRefs` in
`opagent-runtime/internal/scan/scanner.go`.

Built-in systool names are not MCP servers and are not resolved through
`@tools/...`. The scanner recognizes these bare names:

```go
var SystoolNames = []string{
  "shell", "read", "write", "edit",
  "agent_task",
  "message_publish", "message_update", "message_read", "message_subscribe", "message_ack",
}
```

Defined in `opagent-protocol/go-sdk/op/agent.go`.

### Built-In systool Selection

`systool` is built into the runtime process. It does not have a `TOOL.md`
manifest, does not spawn a daemon, and is not mounted through a `@tools/...`
base-dir reference.

System tool exposure is controlled by `AGENT.md` front matter:

| Manifest shape | Built-in systool result |
| --- | --- |
| No `tools:` field and no `"@systool": null` | Default mode: expose built-in systool tools allowed by agent capabilities |
| `tools:` field present | Allowlist mode: expose only bare systool names listed in `tools:` |
| `"@systool": null` | Disabled mode: expose no systool tools |

MCP tool servers are always explicit. A server is exposed only when the agent
lists the server in `tools:` with `@tools/<name>`, an explicit node id, or a
path ref. Listing an MCP server does not grant extra systool tools.

If `"@systool": null` is combined with explicit bare systool names, scanner
validation fails. It may be combined with MCP server refs when an agent should
use those servers but no local systool tools.

### Example Manifests

Built-in coding agent using default local tools:

```yaml
# no tools: field
```

GBrain agent mixing a cloud MCP server and explicit local tools:

```yaml
tools:
  - "@tools/gbrain-cloud"
  - shell
  - read
```

This exposes `gbrain-cloud`, `shell`, and `read`. It does not expose `write` or
`edit`, and it does not expose `agent_task` or `message_*`.

MCP-only agent:

```yaml
tools:
  - "@tools/gbrain-cloud"
```

This exposes only the cloud MCP server and no systool tools.

Research agent with a local tool server directory:

```yaml
tools:
  - read
  - shell
  - ./tools/research-tools
```

See `TestScanAgents_ResolvesRelativeToolServerRefs` in
`opagent-runtime/internal/scan/scanner_test.go`.

## Built-In systool

`systool` is the runtime-owned in-process tool family with server id
`__systool__`. Implementation lives in
`opagent-runtime/internal/builtintools/`.

It registers these OS tools:

- `shell`
- `read`
- `write`
- `edit`

The same runtime-owned server id is also used for host tools injected by the
agent loop, such as `agent_task` for mounted subagents and `message_*` tools
for thread-capable agents. In default mode these host tools are injected when
their normal capability conditions are met. In allowlist mode they must also be
listed explicitly in `tools:`.

Agents that need a user decision should use `message_publish` with
`kind=request` and structured `questions`. Use the message `title` as the
business heading. Request questions only carry `questions[].id`,
`questions[].question`, and `questions[].options[].id/label`; the client adds
the free-form `Other...` reply affordance.

`assembleTools` in `opagent-runtime/internal/core/agent.go` reads
`AgentMeta.SysToolMode` and `AgentMeta.SysTools`, then copies specs from the
built-in registry. It no longer reads a systool MCP cache entry.

When an agent mounts a tool server id in `ToolServers`, runtime loads that
node's cached `ToolsMeta.Tools` into the loop tool map. The `system` tag does
not filter mounted MCP tool servers.

## Refresh Behavior Summary

From `RefreshNodeCache`:

1. Scan all tool nodes.
2. For each tool node:
   - if `run.command` or `run.url` is present: connect, call `ListToolSpecs()`,
     store specs on the node
   - if no endpoint is present: cache the scanned node without MCP tool specs
3. Scan skills, then agents.

If connection or `ListToolSpecs()` fails for an endpoint-backed tool server,
runtime logs the error and skips that node for the refresh cycle. This is
best-effort behavior tested in `opagent-runtime/internal/node/scan_test.go`.

## Authoring Checklist

- Put one tool server in `tools/<name>/TOOL.md` or
  `agents/<agent>/.agent/tools/<name>/TOOL.md`.
- Use `run.command` for local stdio servers or `run.url` for remote MCP servers.
- Do not set both command and url.
- For URL endpoints that need authentication, use `run.header`:
  `Authorization: "Bearer {openbrain_session}"` for the logged-in OpenBrain
  session, or a normal bearer/API-key value for user-managed servers.
- Use `tags: system` only when install/update should copy top-level files from
  the package's `bin/` directory into the user-level OpenBrain bin directory.
- Use `tags: builtin` on top-level packages that should be included in
  OpenBrain release bundles and the public marketplace catalog.
- Mount the server from `AGENT.md` with `@tools/<name>`, a path ref, or explicit
  systool names.
- Give shared packages `id: tools-<name>` so references stay stable.

## Validation And Tests

Useful scanner tests:

- `TestScanAgents_ResolvesRelativeToolServerRefs`
- `TestScanToolsParsesRunHeader`
- `TestScanTools_PreservesSystemTagAndRunEndpoint`
- `TestScanTools_AcceptsSystemToolWithoutRun`
- `TestScanTools_RunScheduleFieldIsRejected`

Run:

```bash
(cd opagent-runtime && go test ./internal/scan/... ./internal/node/...)
```

Tool-server authors implementing MCP handlers should also test against the Go
SDK in `opagent-protocol/go-sdk`.

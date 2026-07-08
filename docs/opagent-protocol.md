# OpAgent Protocol

This document is the public design source for the OpAgent Protocol SDKs shipped
in this repository.

The Go SDK is adapted from the Model Context Protocol Go SDK. The upstream
credit and module layout are documented in `opagent-protocol/README.md`.

OpenBrain runtime, desktop, built-in agents, and mounted MCP tool servers use
this protocol surface to exchange MCP messages and OpenBrain-specific opcodes.

Related docs: `docs/runtime.md`, `docs/tools.md`, `docs/skills.md`.

## Repository Layout

```text
opagent-protocol/
  README.md
  go-sdk/
    op/          # primary Go API
    auth/        # SDK auth helpers; not used as a Run manifest field
    jsonrpc/     # JSON-RPC helpers
  ts-sdk/        # minimal TypeScript SDK for daemon agents
```

Go module path:

```text
github.com/colinagent/openbrain/opagent-protocol/go-sdk
```

## Design Goals

OpAgent Protocol keeps three layers explicit:

1. MCP-compatible transport and tool/resource/prompt methods.
2. Shared node identity types for agents, skills, and tool servers.
3. OpenBrain-specific opcodes carried in `OpAgentParams` and `OpNodeParams`.

Runtime scans Markdown manifests into `OpNode` values defined here, then uses
the Go SDK client to talk to endpoint processes and tool servers.

## Core Node Model

Primary type: `op.OpNode` in `opagent-protocol/go-sdk/op/agent.go`.

| Field | Meaning |
| --- | --- |
| `ID` | Node id such as `agent-coder`, `skill-query`, or generated `tools-<uuid>` |
| `Kind` | `agent`, `skill`, or `tools` |
| `URI` | Manifest URI, usually `file://.../AGENT.md`, `SKILL.md`, or `TOOL.md` |
| `Cwd` | Working directory for relative path resolution |
| `Tags` | Free-form labels copied from manifest front matter |
| `Run` | Endpoint and daemon config for local processes or remote URLs |
| `OpCodes` | Supported OpenBrain operations |
| `Meta` | `AgentMeta`, `SkillMeta`, or `ToolsMeta` |

### Node Kinds

```go
const (
    NodeKindAgent NodeKind = "agent"
    NodeKindSkill NodeKind = "skill"
    NodeKindTools NodeKind = "tools"
)
```

### Stable IDs

Manifests may declare explicit ids with kind prefixes:

- `agent-*`
- `skill-*`
- `tools-*`

When `id` is omitted, `BuildNodeID` generates a deterministic
`kind-<uuidv5>` suffix from `(uid, hostID, kind, uri, env)`.

Explicit-id validation and prefix rules live in the scanner and in
`NodeKindFromID`.

### Meta Types Loaded From Manifests

Runtime loads prompt bodies separately. Meta types intentionally exclude prompt
text.

`AgentMeta`:

- `Name`, `Description`, `Avatar`, `MaxToken`, `Model`
- `ToolServers []string`
- `SysToolMode string`
- `SysTools []string`
- `Skills []string`
- `SubAgents []string`

`SkillMeta`:

- `Slug`, `Name`, `Description`, `Tags`

`ToolsMeta`:

- `Name`, `Description`
- `Tools []*ToolSpec` populated after MCP `tools/list`

Built-in systool names recognized by runtime scanner reference resolution:

```go
var SystoolNames = []string{
    "shell", "read", "write", "edit",
    "agent_task",
    "message_publish", "message_update", "message_read", "message_subscribe", "message_ack",
}
```

## Run

`op.Run` describes how runtime connects to an endpoint agent or tool server.

| Field | Meaning |
| --- | --- |
| `Command []string` | Local process started for stdio transport |
| `URL string` | Remote MCP streamable HTTP endpoint |
| `Header map[string]string` | Optional HTTP headers for URL endpoints |
| `Daemon bool` | Keep the endpoint connection alive between calls |

Validation rules in `Run.Validate()`:

- `command` and `url` cannot both be set
- `header` is only valid with `url`

`Run.HasEndpoint()` returns true when either `command` or `url` is non-empty.

Manifest field names are lowercase YAML/JSON keys:

```yaml
run:
  command: ["bin/coder"]
  daemon: true
```

```yaml
run:
  url: "https://api.op-agent.com/gbrain/mcp"
  daemon: true
  header:
    Authorization: "Bearer {openbrain_session}"
```

`header` defaults to absent. It is for HTTP endpoints only; local command
endpoints do not receive or inherit these headers.

OpenBrain runtime maps endpoints to transports in `CreateConnection`:

- command -> `op.CommandTransport` / stdio
- url -> `op.StreamableClientTransport` / HTTP streamable
- `daemon: true` -> long-lived connection cached until runtime shutdown

Transport type constants:

```go
const (
    Stdio          TransportType = "stdio"
    HttpStreamable TransportType = "http_streamable"
)
```

## MCP Compatibility

The Go SDK implements MCP server and client sessions on top of JSON-RPC.

Supported protocol versions declared in `op/shared.go`:

- `2025-06-18` (latest default)
- `2025-03-26`
- `2024-11-05`

Common MCP surface used by OpenBrain today:

- initialize / initialized
- `tools/list`
- `tools/call`
- logging notifications
- SDK support for sampling and elicitation hooks on the client side

Runtime-owned built-in systool tools are not separate MCP servers. They are
assembled directly in process by OpenBrain runtime and use `ToolSpec.ServerID =
"__systool__"` when exposed to the model.

The SDK also includes prompts, resources, roots, and completion handlers on the
server type, but OpenBrain built-in packages primarily rely on tools plus the
OpenBrain opcode extensions below.

## OpenBrain Opcode Extensions

OpenBrain adds typed requests beyond stock MCP:

```go
type OpAgentParams struct {
    OpCode  OpCode
    Meta    `json:"_meta,omitempty"`
    Content Content `json:"content,omitempty"`
}

type OpNodeParams struct {
    OpCode  OpCode
    Meta    `json:"_meta,omitempty"`
    Content Content `json:"content,omitempty"`
}
```

Active opcode constants are defined in `opagent-protocol/go-sdk/op/op_code.go`.
Representative groups:

| Area | Opcodes |
| --- | --- |
| Agent loop | `agent/loop/create`, `prompt/get`, `thread/submit`, `thread/compact`, `thread/interrupted`, `thread/steer`, `thread/follow_up`, `thread/follow_up/promote`, `thread/queue/get`, `thread/queue/remove`, `thread/active/list` |
| Thread management | `thread/create`, `thread/fork`, `thread/meta/get`, `thread/meta/update`, `thread/snapshot/get`, `thread/review/list`, `thread/review/resolve`, `thread/review/rollback` |
| Editor | `editor/completion`, `editor/completion/cancel` |
| Node discovery | `node/list`, `agent/scan` |
| Host/config | `system/started`, `notify/message`, `config/get`, `config/system/get` |

Some older opcodes remain in the file as deprecated aliases, for example
`agent/call` and `agent/continue`. New code should prefer `thread/submit`.

Thread state is the public read/write boundary: `thread/snapshot/get` is the
surface read path for rendered thread state, while `thread/submit` and queue
opcodes are the execution path. Message host tools such as `message_publish`,
`message_update`, `message_read`, and `message_ack` persist conversation state
through thread/channel records rather than feature-specific history stores.

Thread snapshots return durable `entries[]`, `entryWindow`, and `revision`.
`entries[]` is a bounded window of raw thread entry objects after the thread
header, preserving each entry's original wire shape while exposing common
`type`, `id`, `parentId`, and `timestamp` metadata in SDKs. `entryWindow`
records the returned range as zero-based `[start,end)` offsets plus `total`,
`hasBefore`, and `hasAfter`. If callers omit a window query, snapshot defaults
to `tail(limit=400)`; runtimes clamp oversized limits. `revision` is still the
last durable entry id in the full JSONL, or the thread id for an empty thread.
Surfaces should render from the returned `entries[]` window and page with
`tail`, `before(anchorId)`, or `after(anchorId)`; the snapshot wire does not
include a separate canonical `messages` reshaping.

Thread JSONL durable entry wire types live in the protocol package when they
are shared across runtime, server, and surfaces. Canonical model history uses
`op.ConversationMessage` inside `op.ThreadCanonicalMessageEntry` with wire
`type: "canonical_message"`. Thread metadata changes use
`op.ThreadMetaUpdateEntry` with wire `type: "thread_meta_update"` for fields
such as title, chat path, file id, and plan paths. Runtime provider code may
keep helper aliases and streaming-only types, but persisted thread entry shapes
are protocol owned. Product-level user questions and approvals are message records
(`message_publish` with `kind=request` and `questions[]`); durable thread entries
do not define separate `elicitation_request` or `elicitation_result` records.
Streaming deltas are presentation events, not durable history. A completed turn
persists as a canonical assistant message in thread JSONL; failed or aborted
streams may include provider error detail inside the canonical message raw
payload, but the protocol does not define a separate stream-error field for
thread entries.
Message request questions use the message `title` as the display/business
heading and keep question payloads minimal: `questions[].id`,
`questions[].question`, and `questions[].options[].id/label`. Clients add the
free-form `Other...` answer affordance instead of agents publishing an `other`
option.
When a user answers a message-system request, `message/reply` returns the user
reply record and, when applicable, the original request record after it has been
marked `resolved`. Follow-up turns receive structured answer context, including
the original request `title` as `requestTitle`, so agents can key business logic
from `requestTitle`, `questionID`, and `optionID` instead of parsing display
text or relying on generated message ids.
Runtime system config includes `threadStorage.maxThreads`, which bounds flat
thread JSONL storage globally and defaults to 10000.

Runtime dispatches incoming host operations through handlers installed on SDK
client sessions, including `OpAgentHandler` and `OpNodeHandler`. Endpoint
agents implement the server side with `Server.AddAgent` and optional
`ServerOptions.OpNodeHandler`.

For endpoint agent calls, runtime may include the resolved active model in
`CallAgentParams.Meta`, typically as `_meta.modelKey`. This lets daemon agents
observe the caller-selected model or forward it back into runtime when they
delegate through thread operations or subagents.

## Go SDK Usage Patterns

### Tool Server Over Stdio

Custom MCP tool servers use the standard SDK server APIs:

```go
server := op.NewServer(&op.Implementation{Name: "research-tools", Version: "v1.0.0"}, nil)
op.AddTool(server, &op.Tool{Name: "read", Description: "..."}, handler.HandleRead)
err := server.Run(ctx, &op.StdioTransport{})
```

See mounted tool-server manifests under `tools/` or `agents/<name>/.agent/tools/`.

### Runtime Client Connection

OpenBrain runtime creates SDK clients when refreshing tool servers and endpoint
agents:

```go
client := op.NewClient(&op.Implementation{Name: "client", Version: "v1.0.0"}, cliOpts)
session, err := client.Connect(ctx, op.Transport(transport), nil)
toolResult, err := session.ListTools(ctx, &op.ListToolsParams{})
```

Source: `opagent-runtime/internal/core/connection.go`.

After `ListTools`, runtime converts MCP tool definitions into `op.ToolSpec`
records and stores them on the tool node.

### Prompt Loading From Endpoint Agents

Endpoint agents answer `prompt/get` through `OpNodeParams`:

```go
result, err := conn.OpNode(ctx, &op.OpNodeParams{
    OpCode: op.OpPromptGet,
    Meta:   requestMeta,
})
```

Source: `opagent-runtime/internal/core/agent.go` (`loadPromptViaEndpoint`).

The built-in `agents/coder` binary implements the server side of this flow.

## TypeScript SDK

`opagent-protocol/ts-sdk` provides a minimal server for daemon agents that speak
newline-delimited JSON-RPC over stdio, matching the Go stdio transport.

Public exports:

- `OpServer`
- `StdioTransport`
- content helpers such as `textContent`

Example from `opagent-protocol/ts-sdk/README.md`:

```ts
import { OpServer, StdioTransport, textContent } from "@op-agent/opagent-protocol";

const server = new OpServer({ name: "demo", version: "0.1.0" });
server.addAgent({ name: "demo" }, async (req) => ({
  agentID: req.params.agentID,
  content: textContent("hello"),
}));
await server.run(new StdioTransport());
```

The TypeScript SDK exposes `OpNodeHandler` for opcode-based server methods such
as `prompt/get`. It is intentionally smaller than the Go SDK.

## How Runtime Uses The Protocol

End-to-end flow:

1. Scanner reads Markdown manifests into `OpNode` values.
2. `RefreshNodeCache` connects to tool servers and endpoint agents that declare
   run endpoints.
3. Tool specs are fetched with MCP `tools/list`.
4. Agent loops merge runtime-owned built-in systool specs plus mounted
   tool-server specs.
5. Model tool calls go back through runtime, which invokes MCP `tools/call` on
   the owning connection.

Skills and subagents do not add MCP tools by themselves. Skills are instruction
files the model reads with built-in `read`. Subagents are mounted agent nodes
delegated through runtime-owned host tools.

## Tests

Go SDK:

```bash
(cd opagent-protocol/go-sdk && go test ./...)
```

TypeScript SDK:

```bash
(cd opagent-protocol/ts-sdk && npm test)
```

Runtime integration tests that exercise protocol connections live under
`opagent-runtime/internal/core/` and `opagent-runtime/internal/node/`.

## Further Reading

- `docs/runtime.md`: scan order, manifest layout, prompt assembly
- `docs/tools.md`: `TOOL.md` manifests and tool-server refresh behavior
- `docs/skills.md`: `SKILL.md` packages and agent mounting
- `docs/subagent.md`: subagent delegation contract

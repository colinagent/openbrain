# OpAgent Protocol Specification

This document is the wire-level source of truth for OpAgent SDKs. The current
Go SDK remains the reference implementation, but new SDKs must match the
contract below instead of inferring behavior from implementation details.

## Transport

OpAgent uses JSON-RPC 2.0 messages over persistent transports.

For stdio, messages are newline-delimited JSON:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{...}}
{"jsonrpc":"2.0","id":1,"result":{...}}
```

The stdio transport does not use `Content-Length` framing. Each message is one
complete JSON value followed by `\n`. SDKs must write compact single-line JSON
and must ignore empty lines while reading.

JSON-RPC request IDs may be strings, numbers, or null. Responses use the same ID
as the request. Notifications omit `id` and do not receive a response.

## Lifecycle

The client initializes a server session before using agent methods.

### `initialize`

Request params:

```ts
{
  protocolVersion: string
  clientInfo: { name: string, version: string }
  capabilities: Record<string, unknown>
  _meta?: Meta
}
```

Result:

```ts
{
  protocolVersion: string
  serverInfo: { name: string, version: string }
  capabilities: Record<string, unknown>
  instructions?: string
  _meta?: Meta
}
```

The current latest protocol version is `2025-06-18`. Implementations should
accept `2025-06-18`, `2025-03-26`, and `2024-11-05`; when the requested version
is unsupported, servers should negotiate to `2025-06-18`.

After receiving the initialize result, the client sends
`notifications/initialized` with optional params `{ _meta?: Meta }`.

### `ping`

Either side may send `ping` with omitted or empty params. The result is `{}`.

## Common Shapes

### `Meta`

`Meta` is an open JSON object carried as `_meta`. Known stable keys include:

- `agentID`: effective OpNode id for the active agent
- `threadID`: active chat thread id
- `turnID` or `turnRequestID`: active turn id
- `fileID`: active chat file id
- `cwd`: workspace directory
- `path` / `chatPath`: active chat transcript path
- `title`: chat title
- `type`: notification event type

Unknown keys must be preserved when forwarding.

### `Content`

Content is a discriminated union by `type`.

```ts
type TextContent = {
  type: "text"
  text: string
  annotations?: Record<string, unknown>
}

type JsonContent = {
  type: "json"
  payload: unknown
}

type ImageContent = {
  type: "image"
  mimeType: string
  data: string
  annotations?: Record<string, unknown>
}

type AudioContent = {
  type: "audio"
  mimeType: string
  data: string
  annotations?: Record<string, unknown>
}

type ResourceLink = {
  type: "resource_link"
  uri: string
  name: string
  title?: string
  description?: string
  mimeType?: string
  size?: number
  annotations?: Record<string, unknown>
}

type EmbeddedResource = {
  type: "resource"
  resource: {
    uri: string
    mimeType?: string
    text?: string
    blob?: string
  }
  annotations?: Record<string, unknown>
}
```

`json` content must use `payload`; legacy `message` inside json content is not
valid.

## Agent Methods

### `agents/call`

The host calls a daemon agent by its registered agent name. Endpoint agents
currently register the `AgentMeta.name` value as the call target.

Request params:

```ts
{
  agentID: string
  content?: Content
  _meta?: Meta
}
```

Result:

```ts
{
  agentID: string
  content: Content
  _meta?: Meta
}
```

Unknown `agentID` is a JSON-RPC invalid params error (`-32602`).

### `node/operation`

Generic node operation channel used for prompt lookup and host/client extension
operations.

Request params:

```ts
{
  opCode: OpCode
  content?: Content
  _meta?: Meta
}
```

Result:

```ts
{
  opCode: OpCode
  content: Content
  _meta?: Meta
}
```

For agent prompt loading, callers send `opCode: "prompt/get"` and expect text
content containing the prompt body.

## Notifications

### `notifications/info`

General protocol notification. The OpAgent chat pipeline uses this for progress
and activity events.

Params:

```ts
{
  opcode: "notify/message"
  content: Content
  _meta?: Meta
}
```

Stable chat event `Meta.type` values include:

- `start`
- `text_start`
- `text_delta`
- `text_end`
- `thinking_start`
- `thinking_delta`
- `thinking_end`
- `toolcall_start`
- `toolcall_delta`
- `toolcall_end`
- `tool_result_step`
- `turn_result`
- `tokenUsage`
- `done`
- `elicit`

`turn_result` content is JSON with the `TurnResultPayload` shape:

```ts
{
  threadID: string
  fileID?: string
  turnID: string
  agentID: string
  path?: string
  chatPath?: string
  title: string
  parentThreadID?: string
  planTurn?: boolean
  userMessage: Message
  assistantText?: string
  reasoningText?: string
  toolResults?: {
    toolName: string
    argumentsObject?: Record<string, unknown>
    resultText: string
    isError?: boolean
  }[]
  canonicalMessages?: unknown
}
```

## Op Codes

Current op codes are strings. SDKs should expose constants and preserve unknown
strings.

Agent and node:

- `agent/call` (deprecated legacy edge adapter)
- `agent/continue` (deprecated legacy edge adapter)
- `agent/loop/create`
- `prompt/get`
- `agent/scan`
- `node/list`

System and config:

- `system/started`
- `notify/message`
- `config/get`
- `config/system/get`

Chat and thread:

- `thread/create`
- `thread/fork`
- `thread/meta/get`
- `thread/meta/update`
- `thread/snapshot/get`
- `thread/review/list`
- `thread/review/resolve`
- `thread/review/rollback`
- `editor/completion`
- `editor/completion/cancel`
- `thread/submit`
- `thread/compact`
- `thread/interrupted`
- `thread/steer`
- `thread/follow_up`
- `thread/follow_up/promote`
- `thread/queue/get`
- `thread/queue/remove`
- `thread/active/list`

Thread JSONL durable entries use thread-neutral wire `type` strings. Shared
protocol-owned entries include `canonical_message`, `thread_meta_update`,
`queue_enqueue`, `queue_dequeue`, `queue_remove`, `queue_promote`,
`message_append`, `message_update`, and `message_ack`. The JSONL header uses
`type: "thread"`.

`thread/snapshot/get` is a render-facing read path. Its query may include
`entryWindow: { mode: "tail" | "before" | "after", anchorId?: string,
limit?: number }`. The result includes `entries[]` for only that bounded
window plus `entryWindow: { start, end, total, hasBefore, hasAfter }`. Omitting
the query means `tail(limit=400)`; `revision` remains the full JSONL's latest
durable entry id, not the last id in the returned window.

## Error Responses

Use standard JSON-RPC 2.0 error codes unless a method documents otherwise:

- `-32700` parse error
- `-32600` invalid request
- `-32601` method not found
- `-32602` invalid params
- `-32603` internal error

Error data is optional and must be JSON-serializable.

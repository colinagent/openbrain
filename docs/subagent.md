# OpenBrain Subagents

This document is the public design source for OpenBrain subagent behavior.

Related docs: `docs/runtime.md`, `docs/skills.md`, `docs/tools.md`, `docs/opagent-protocol.md`.

## Model

A subagent is not a separate node kind. A subagent is a normal `kind=agent`
node mounted by a parent agent through `subagents:`:

```yaml
subagents:
  - "@agent-gbrain"
```

The scanner resolves these references to `AgentMeta.SubAgents`. Runtime then
filters them before exposing them to the parent model.

## Delegation Contract

An agent is runtime-delegatable only when it is:

- a scanned `kind=agent` node;
- mounted by the parent through `subagents:`;
- capable of `thread/submit`.

`prompt/get` alone means the runtime can load a prompt. It does not make an
agent eligible for delegation.

When at least one mounted subagent is valid, runtime injects the host-owned
`agent_task` tool into the parent agent. The parent calls it with:

```json
{
  "subagent_id": "agent-gbrain",
  "task": "Find the durable project knowledge relevant to this change.",
  "context": "Optional parent context"
}
```

Runtime validates that the target subagent is mounted and `thread/submit`
capable. It then creates a child thread, records parent metadata, inherits the
parent working directory when possible, dispatches through the normal agent call
path, and returns a structured tool result.

Model selection follows the same runtime rule for local and endpoint agents:

- If the child `AGENT.md` declares `model: <modelKey>`, that explicit child
  model is used by default.
- Otherwise the child call inherits the parent loop's explicit `modelKey`.
- Runtime validates explicit child model keys against local `models.json`
  before dispatch. If an explicit child model is unavailable, runtime returns an
  error and publishes a configuration message when the child thread exists.
- If neither child nor parent supplies a model key, delegation fails before the
  child turn starts.
- Endpoint child agents receive the resolved model in call meta, normally as
  `_meta.modelKey`.

Subagent delegation is runtime-owned. Do not add a separate delegation tool in
built-in systool or in an MCP server for this path.

## Discovery Surface

Runtime appends an `Available Subagents` section to the parent prompt. Each
entry includes enough information for the parent to discover the child contract:

- subagent ID;
- name;
- description when present;
- `AGENT.md` path;
- `agentRoot`;
- `agentHome`.

The parent does not need baked-in knowledge of a child implementation. When the
details matter, the parent should read the listed child `AGENT.md`.

## Prompt Contract

The subagent prompt is the contract. It should describe:

- what task input the subagent expects;
- what durable state it reads or writes;
- what it should return to the parent;
- what it must not do.

Subagents should use runtime prompt variables for package-local paths:

```text
${agentRoot}
${agentHome}
```

These variables are expanded by runtime for both local agents and endpoint
agents after `prompt/get`.

## Simple Memory

`agents/simple-memory` is an optional reusable built-in memory subagent:

```text
agents/simple-memory/
  .agent/
    AGENT.md
  .gitignore
```

Manifest contract:

```yaml
id: agent-simple-memory
name: simple-memory
opcodes:
  - thread/submit
  - prompt/get
tools:
  - read
  - write
  - edit
```

It has no `run.command` and no standalone daemon.

Its durable state file is:

```text
${agentRoot}/memory.md
```

`memory.md` is runtime/user state and is not committed. The memory agent should
read the existing file before changing it, store only durable facts and stable
preferences, avoid secrets or transient logs, and leave the file unchanged when
there is nothing durable to remember.

Endpoint agents remain responsible for their own `prompt/get` implementation.
They can mount `agent-simple-memory` like any other subagent when they need
durable memory; memory is not injected through special coding-agent logic. The
built-in `agents/coder` default mounts only `agent-gbrain`.

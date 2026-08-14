# OpenBrain

[中文 README](README.zh-CN.md)

> **Build your second brain. Connect with other minds.**

OpenBrain helps you turn your knowledge into a persistent second brain, then
connect it with other people, teams, and public brains. Your agents can query
the brains you permit, while every connection stays explicit and under your
control.

[Download](https://download.op-agent.com) · [Website](https://openbrain.chat) · [Agent OS](https://openbrain.chat/agent-os)

## Build your second brain

Bring your notes, local workspaces, and knowledge sources into a brain your
agents can query when they need context. Source material and durable knowledge
remain visible instead of disappearing with one chat.

## Connect with other minds

Connect personal, team, and public brains without flattening them into one
store. The runtime queries connected brains on demand, within explicit account,
organization, workspace, sharing, and permission boundaries.

## The runtime your agents live on

The runtime brings your second brain to life. It gives agents the context and
capabilities to use knowledge, act through tools, collaborate, and remember:

- **Agents** — custom, coding, memory, and supported knowledge agents.
- **Skills** — reusable `SKILL.md` workflow packages loaded when relevant.
- **Tools and MCP** — built-in tools plus `TOOL.md` MCP tool servers.
- **Subagents and memory** — delegated work with persistent thread context.
- **Trust boundaries** — permission profiles, sandboxed execution, and
  pre-action approvals.

## Powered by OpAgent

OpenBrain runs on [OpAgent](https://github.com/colinagent/opagent), the
Apache-2.0 Go Agent framework and multi-agent OS Runtime. OpAgent provides the
portable Runtime, Protocol SDKs, sandbox, permissions, Agent/Tool/Skill model,
subagents, and file-native thread engine. This repository contains OpenBrain's
product documentation and product-specific integrations—not a second copy of
the Runtime or Protocol core.

OpenBrain adds product policy and experiences around that foundation:

- GBrain-backed knowledge Agents and their focused Skill pack.
- OpenBrain Cloud GBrain and Cloud Sync integrations.
- Product session, approval, prompt, schedule, and managed-service policy.
- A public Remote Control wire contract for product clients and relays.
- Local and remote workspaces presented through one Desktop experience.

GBrain is an optional knowledge-agent integration, not OpenBrain's sole
foundation. Mount it when you want its focused knowledge workflow.

Read about the [Runtime integration](docs/runtime.md), [product sandbox policy](docs/sandbox.md),
[Skills](docs/skills.md), [Tools](docs/tools.md), [Subagents](docs/subagent.md),
and [product protocols](docs/opagent-protocol.md).

## Repository layout

- `agents/gbrain/`: OpenBrain's GBrain knowledge Agent and product Skill pack.
- `skills/openbrain-cloud-sync/`: product Cloud workspace synchronization.
- `tools/gbrain-cloud/`: session-authenticated product MCP endpoint manifest.
- `integrations/opagent/`: OpenBrain-owned host hooks for public OpAgent Runtime.
- `protocol/remotecontrol/`: versioned OpenBrain Remote Control wire contract.
- `docs/`: public product behavior and integration documentation.

OpenBrain Desktop, iOS, cloud services, and the cooperating local
`openbrain-server` are proprietary. Their source, build/signing pipelines,
deployment topology, and operations documentation are intentionally absent.

## Development

The repository root is a Go workspace, not a Go module. OpAgent dependencies
come from `github.com/colinagent/opagent` and are never vendored here.

```bash
(cd integrations/opagent && go test ./...)
(cd protocol/remotecontrol && go test ./...)
(cd skills/openbrain-cloud-sync && go test ./...)
node --test skills/openbrain-cloud-sync/SKILL.source.test.mjs
scripts/check-openbrain-public-boundary.sh
```

## License

OpenBrain source in this repository is licensed under AGPL-3.0; see
[LICENSE](LICENSE) and [NOTICE](NOTICE). OpAgent is a separate Apache-2.0
dependency. GBrain is maintained separately under MIT.

Copyright © 2026 OpAgent Inc.

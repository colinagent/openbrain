# OpenBrain

[中文 README](./README.zh-CN.md)

> **Build your second brain. Connect with other minds.**

OpenBrain helps you turn your knowledge into a persistent second brain, then
connect it with other people, teams, and public brains. Your agents can query
the brains you permit, while every connection stays explicit and under your
control.

[Download](https://openbrain.chat/download) · [Website](https://openbrain.chat) · [Explore the Runtime](https://openbrain.chat/agent-os)

## Build your second brain

Bring your notes, local workspaces, and knowledge sources into a brain your
agents can query when they need context.

## Connect with other minds

Connect personal, team, and public brains without flattening them into one
store. The runtime queries connected brains on demand, within explicit access
and permission boundaries.

## The runtime your agents live on

The runtime brings your second brain to life. It gives agents the context and
capabilities to use knowledge, act through tools, collaborate, and remember:

- **Agents** — Coder, custom agents, memory agents, and supported knowledge
  agents.
- **Skills** — reusable `SKILL.md` workflow packages loaded when relevant.
- **Tools and MCP** — built-in tools plus `TOOL.md` MCP tool servers.
- **Subagents and memory** — delegated work with persistent thread context.
- **OpAgent Protocol** — one contract for local and remote runtimes.
- **Trust boundary** — permission profiles, sandboxed execution, and
  pre-action approvals.

Read the public design docs for the [runtime](docs/runtime.md),
[skills](docs/skills.md), [tools and MCP](docs/tools.md),
[subagents](docs/subagent.md), and [protocol](docs/opagent-protocol.md).

## GBrain support

GBrain is a supported knowledge-agent and brain integration. It can be mounted
when you want its knowledge workflow, but it is optional—not OpenBrain's core
identity or sole foundation.

## Ready to run

Download OpenBrain to build your second brain. The agent runtime can run locally
or against a remote workspace.

[Download OpenBrain](https://openbrain.chat/download)

## Repository layout

- `agents/`: built-in product agents. `agents/coder` is the default coding
  agent; `agents/gbrain` is the GBrain-backed knowledge agent.
- `tools/`: MCP tool packages. `tools/gbrain-cloud` exposes OpenBrain Cloud
  GBrain MCP to agents; shell/read/write/edit are runtime built-ins.
- `opagent-runtime/`: public OpAgent runtime packages and runtime entrypoints.
- `opagent-protocol/`: public OpAgent protocol SDKs.
- `scripts/openbrain/`: generic runtime/package build helpers.
- `docs/runtime.md`: runtime design.
- `docs/subagent.md`: subagent design.

OpenBrain Desktop and its cooperating local server are distributed products,
but their implementation is proprietary and is not part of this source
repository.

## Development

Go modules are linked locally by the root `go.work`; public module files should
not contain local-path `replace` directives.

```bash
(cd opagent-runtime && go test ./...)
(cd opagent-protocol/go-sdk && go test ./...)
(cd agents/coder && go test ./...)
```

The repository root is a `go.work` workspace and is not itself a Go module, so
run Go tests from the module directories above.

For runtime or subagent changes, read [docs/runtime.md](docs/runtime.md) and
[docs/subagent.md](docs/subagent.md) first.

## Releases

OpenBrain desktop installers remain available from GitHub Releases even though
their source is proprietary. The uploaded assets for each product release are
limited to the three Desktop downloads for macOS, Linux, and Windows. Runtime
self-update manifests, runtime bundles, bootstrap binaries, and desktop update
metadata are served from the public download endpoint:
`https://download.op-agent.com`.

## License

This repository uses multiple licenses:

- **AGPL-3.0** — Source in this repository: `agents/`, `tools/`,
  `opagent-runtime/`, `opagent-protocol/`, `skills/`, `docs/`, and generic
  helpers under `scripts/openbrain/`. See [LICENSE](LICENSE) and
  [NOTICE](NOTICE).
- **Proprietary** — OpenBrain Desktop and its local server are distributed
  separately; their source is not licensed by this repository.
- **MIT** — GBrain itself is the external `garrytan/gbrain` project. OpenBrain
  release helpers package upstream source directly or use an upstream-aligned
  `colinagent/gbrain` binary mirror.

If you modify or distribute OpenBrain code, you must comply with AGPL-3.0
(including source availability when required) and retain attribution to
OpenBrain as described in NOTICE. Copyright is held by OpAgent Inc.

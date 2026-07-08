# OpenBrain

[中文 README](./README.zh-CN.md)

> **GBrain, ready to use.**

OpenBrain gives GBrain a desktop experience backed by an open agent runtime.

[Download](https://openbrain.chat/download) · [Website](https://openbrain.chat) · [GitHub](https://github.com/colinagent/openbrain)

## Connect other GBrains

Connect sources and peer brains. In practice, connected brains are queried on demand as subagents.

![Personal and team brain connections](docs/assets/大脑互联图.png)

## GBrain Agent

**Use GBrain as a subagent**

Keep your main agent focused.

## Ready to use. Zero setup.

Download OpenBrain and start with GBrain — no extra wiring.

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

OpenBrain desktop installers are available from GitHub Releases. Runtime
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

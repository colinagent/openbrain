# OpenBrain Server

Local desktop server for OpenBrain.

This app is OpenBrain-branded at the desktop/server layer, while its AgentOS
protocol, runtime packages, and marketplace integration use the public OpAgent
project. Marketplace belongs to OpAgent and intentionally uses OpAgent's public
marketplace defaults.

## Build

```bash
scripts/build.sh darwin-arm64
```

Supported platforms:

- `darwin-arm64`
- `darwin-amd64`
- `linux-amd64`
- `windows-amd64`

The build output matches the desktop runtime bundle layout:

```text
dist/<platform>/agents/openbrain-server/bin/openbrain-server[.exe]
```

## Boundaries

- Default OpAgent base dir: `~/.openbrain`
- Server binary and implementation name: `openbrain-server`
- Public OpAgent dependencies are expected and allowed.
- Keep this app limited to public desktop/server code and public dependencies.

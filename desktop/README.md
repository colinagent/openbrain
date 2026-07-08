# OpenBrain Desktop

Electron desktop shell for OpenBrain.

## Identity

- Product name: `OpenBrain`
- macOS bundle id: `io.openbrain.app`
- Deep link scheme: `openbrain://`
- User data/config root: `~/.openbrain`
- Public product domain: `https://openbrain.chat`
- Default API gateway: `https://api.op-agent.com`

## Release Packaging

Desktop artifacts are packaged with electron-builder. Public release artifacts
are associated with `openbrain-v*` tags.

This public repository must not contain manager URLs, object-storage
credentials, object-storage origins, or internal deployment hosts.

Build local macOS artifacts:

```bash
npm run dist:mac
```

For signed macOS releases, provide a valid Developer ID Application identity
and Apple notarization credentials through the standard electron-builder/macOS
environment variables. `OPENBRAIN_DESKTOP_UPDATE_URL` can override the updater
feed for local testing.

## Runtime Bundle Input

Desktop packaging expects OpenBrain runtime assets to be staged before running
`dist:*`.

Default stage layout:

```text
.tmp/openbrain-release/<version>/<platform>/bundle.tar.gz
.tmp/openbrain-release/<version>/<platform>/runtime-version.txt
.tmp/openbrain-release/<version>/<platform>/stage/bin/openbrain-bootstrap
```

You can also point at explicit files:

```bash
export OPENBRAIN_RUNTIME_BUNDLE_PATH=/path/to/bundle.tar.gz
export OPENBRAIN_BOOTSTRAP_PATH=/path/to/openbrain-bootstrap
export OPENBRAIN_RUNTIME_VERSION=0.0.0
```

The staged `openbrain/` packaging directory is generated and ignored by Git.

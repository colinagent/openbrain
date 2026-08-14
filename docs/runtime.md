# OpenBrain Runtime integration

OpenBrain is powered by the public
[OpAgent Runtime](https://github.com/colinagent/opagent/tree/main/opagent-runtime).
The Runtime core, Protocol SDKs, generic sandbox, and generic Agent/Tool/Skill
implementations are maintained in that repository. This document describes
only OpenBrain's product integration and public behavior.

## Local and remote are one model

Each OpenBrain workspace tab talks to the Runtime server that owns that
workspace, similar to remote development in an editor:

- a local tab calls the local product server and local OpAgent Runtime;
- a remote tab uses a forwarded loopback port to the remote product server and
  remote OpAgent Runtime.

Normal resources, source access, sync, threads, and tools execute at the active
workspace Runtime. The Desktop renderer does not add separate SSH branches for
those operations.

## Product data directory

OpenBrain always starts Runtime and its cooperating product server with an
explicit product directory:

```text
macOS/Linux: ~/.opagent/openbrain
Windows:     %USERPROFILE%\.opagent\openbrain
```

`OPAGENT_BASE_DIR` is the only environment override used by the product
processes. Token, PATH, threads, logs, backups, avatars, GBrain state, and local
indexes all derive from the normalized `baseDir`. OpenBrain does not read,
move, delete, or fall back to a deprecated product directory.

The standalone OpAgent default remains `~/.opagent`; the nested OpenBrain
directory prevents standalone and product state from colliding.

## Product-owned host policy

[`integrations/opagent`](../integrations/opagent) builds `runtime.Options` from
trusted product code. It owns:

- product configuration loading and approval review;
- OpenBrain session header resolution;
- exact authorization for the managed GBrain Cloud tool;
- a fail-closed callback for proprietary system services;
- product prompt and Tool scope additions;
- Cloud Sync schedule policy and background services.

The GBrain Cloud authorizer requires the exact node ID, HTTPS URL, session
Header shape, daemon mode, and manifest path below the product base directory.
A copied workspace manifest cannot obtain the product session token. Unknown
remote nodes and system services remain denied.

## Private components

Desktop and `openbrain-server` are cooperating product components but are not
open source. This repository intentionally contains no server implementation,
Desktop main/renderer code, signing assets, or private build and deployment
instructions.

For portable Runtime APIs, lifecycle, permissions, and storage invariants, use
the [OpAgent documentation](https://github.com/colinagent/opagent/tree/main/docs).

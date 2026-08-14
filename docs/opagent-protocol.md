# Protocols used by OpenBrain

OpenBrain uses two deliberately separate protocol layers.

## OpAgent Protocol

Agent, Tool, Skill, thread, permission, and Runtime operations use
[OpAgent Protocol](https://github.com/colinagent/opagent/tree/main/opagent-protocol).
Its specification and Go/TypeScript SDKs are maintained with the portable
Runtime under Apache-2.0. OpenBrain does not fork or duplicate those SDKs.

Product-specific configuration is supplied through namespaced extension data
and host hooks. It does not add product account, Cloud, or Desktop types to the
reusable Protocol core.

## OpenBrain Remote Control Protocol

[`protocol/remotecontrol`](../protocol/remotecontrol) defines the public v1 wire
contract used by OpenBrain remote-control clients, relays, and host connectors.
It includes bounded envelopes, capabilities, operations, request replay,
chunking, acknowledgement, heartbeat, and resynchronization rules.

Remote Control envelopes are product transport frames, not Runtime opcodes.
Keeping them separate prevents a product relay contract from becoming a
privileged capability in every OpAgent host.

The Desktop host, relay, cloud control plane, principal verification, and
operation handlers remain proprietary. Publishing the wire types does not
publish those implementations or grant a client any capability by itself.

```bash
(cd protocol/remotecontrol && go test ./...)
```

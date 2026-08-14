# OpenBrain Remote Control Protocol

This product-specific module defines the versioned public wire contract between
OpenBrain remote-control clients, relays, and host connectors. It is not part of
OpAgent Protocol: relay envelopes and product capabilities are transport frames,
not reusable Runtime opcodes.

```text
github.com/colinagent/openbrain/protocol/remotecontrol
```

The implementation of the Desktop host, local product server, relay, and cloud
control plane is proprietary and is not included here.

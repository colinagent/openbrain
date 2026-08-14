# OpenBrain integration for OpAgent

This AGPL module contains OpenBrain's product-owned policy for the public
OpAgent Runtime. It compiles the product data directory, session-header
resolution, exact managed GBrain endpoint identity, private system-service
authorization callback, approval reviewer, prompt/tool policy, scheduled-work
policy, and background services into `runtime.Options`.

Runtime and Protocol mechanisms are dependencies from
[`colinagent/opagent`](https://github.com/colinagent/opagent); they are not
copied into this repository.

The product host must construct `Dependencies` from trusted compiled code. A
workspace manifest or user configuration cannot install a system-service
authorizer, broaden the managed remote endpoint allowlist, or claim the
product-managed Cloud Sync task identity.

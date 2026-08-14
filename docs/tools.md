# OpenBrain Tools

Generic Tool and MCP authoring belongs to
[OpAgent](https://github.com/colinagent/opagent/blob/main/docs/components.md).
OpenBrain keeps only its product-specific Tool manifest here.

## GBrain Cloud

[`tools/gbrain-cloud/TOOL.md`](../tools/gbrain-cloud/TOOL.md) describes the
managed OpenBrain Cloud GBrain MCP endpoint. It uses the current product
session through the `{openbrain_session}` placeholder; users and Agents are not
asked to copy a GBrain or bearer token into manifests.

The product integration authorizes this endpoint only when all of these match:

- node ID `tools-gbrain-cloud`;
- managed manifest path under `~/.opagent/openbrain/tools/gbrain-cloud`;
- exact HTTPS product API URL;
- exact Authorization Header placeholder;
- remote daemon shape with no local command.

Header resolution happens only after authorization. A workspace package that
copies the manifest text fails the managed-path check and cannot receive the
OpenBrain session.

Generic `rg-search`, built-in filesystem/shell tools, sandbox process routing,
and MCP transports live in the OpAgent project.

---
name: gbrain-cloud
description: OpenBrain Cloud GBrain MCP tool server. Uses the current OpenBrain session.
tags: builtin
run:
  daemon: true
  url: "https://api.op-agent.com/brain/mcp"
  header:
    Authorization: "Bearer {openbrain_session}"
---

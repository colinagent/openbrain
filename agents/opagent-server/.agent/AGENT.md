---
id: agent-openbrain-server
name: openbrain-server
description: OpenBrain local server for workspace access, websocket events, chat, and resources.
tags:
  - builtin
  - server
opcodes:
  - system/started
  - notify/message
  - system/config/get
run:
  command: ["./bin/openbrain-server", "--host", "127.0.0.1", "--port", "19530"]
  daemon: true
---

OpenBrain server is a managed local daemon used by the desktop client and
runtime. It exposes workspace, chat, transfer, resource, dashboard, and
notification endpoints for the local OpenBrain session.

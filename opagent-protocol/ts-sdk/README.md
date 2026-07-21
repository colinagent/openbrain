# OpAgent Protocol TypeScript SDK

Minimal TypeScript SDK for writing OpAgent daemon agents. In OpenBrain
manifests, long-lived endpoint agents are declared with `run.daemon: true`;
the SDK itself only implements the JSON-RPC server side.

Typical manifest for a local stdio endpoint:

```yaml
run:
  command: ["node", "dist/server.js"]
  daemon: true
```

Remote MCP HTTP endpoints do not use this stdio transport. Declare them with
`run.url` and optional HTTP-only `run.header` instead:

```yaml
run:
  url: "https://api.op-agent.com/brain/mcp"
  daemon: true
  header:
    Authorization: "Bearer {openbrain_session}"
```

```ts
import { OpServer, StdioTransport, textContent } from "@op-agent/opagent-protocol";

const server = new OpServer({ name: "demo", version: "0.1.0" });

server.addAgent({ name: "demo" }, async (req) => ({
  agentID: req.params.agentID,
  content: textContent("hello")
}));

await server.run(new StdioTransport());
```

The stdio transport is newline-delimited JSON-RPC 2.0 to match the Go SDK.

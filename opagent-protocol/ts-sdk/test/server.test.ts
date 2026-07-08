import { PassThrough } from "node:stream";
import { afterEach, describe, expect, it } from "vitest";
import { createMessageConnection, type MessageConnection } from "vscode-jsonrpc/node.js";
import {
  contentText,
  NdjsonMessageReader,
  NdjsonMessageWriter,
  OpCodes,
  OpServer,
  StdioTransport,
  textContent,
} from "../src/index.js";

const connections: MessageConnection[] = [];
const servers: OpServer[] = [];

afterEach(() => {
  for (const connection of connections.splice(0)) {
    connection.dispose();
  }
  for (const server of servers.splice(0)) {
    server.close();
  }
});

function createPair() {
  const clientToServer = new PassThrough();
  const serverToClient = new PassThrough();
  const serverTransport = new StdioTransport({
    stdin: clientToServer,
    stdout: serverToClient,
  });
  const client = createMessageConnection(
    new NdjsonMessageReader(serverToClient),
    new NdjsonMessageWriter(clientToServer),
  );
  connections.push(client);
  client.listen();
  return { client, serverTransport };
}

describe("OpServer", () => {
  it("handles initialize, agent call, node operation, and notifications", async () => {
    const { client, serverTransport } = createPair();
    const server = new OpServer({ name: "test-agent", version: "0.1.0" });
    servers.push(server);

    server.addAgent({ name: "demo" }, async (req) => {
      await req.session.notifyText("stream", {
        type: "text_delta",
        chatPath: "/tmp/chat.md",
      });
      return {
        agentID: req.params.agentID,
        content: textContent(`echo:${contentText(req.params.content)}`),
      };
    });
    server.onOpNode(async (req) => {
      expect(req.params.opCode).toBe(OpCodes.PromptGet);
      return {
        opCode: req.params.opCode,
        content: textContent("prompt body"),
      };
    });
    server.connect(serverTransport);

    const notifications: unknown[] = [];
    client.onNotification("notifications/info", (params) => {
      notifications.push(params);
    });

    const init = await client.sendRequest<Record<string, unknown>>("initialize", {
      protocolVersion: "2025-06-18",
      clientInfo: { name: "client", version: "0.1.0" },
      capabilities: {},
    });
    expect(init).toMatchObject({
      protocolVersion: "2025-06-18",
      serverInfo: { name: "test-agent", version: "0.1.0" },
    });

    await client.sendNotification("notifications/initialized", {});

    const agentResult = await client.sendRequest<Record<string, unknown>>("agents/call", {
      agentID: "demo",
      content: { type: "text", text: "hello" },
      _meta: { chatPath: "/tmp/chat.md" },
    });
    expect(agentResult).toMatchObject({
      agentID: "demo",
      content: { type: "text", text: "echo:hello" },
    });
    expect(notifications).toHaveLength(1);
    expect(notifications[0]).toMatchObject({
      opcode: "notify/message",
      content: { type: "text", text: "stream" },
      _meta: { type: "text_delta" },
    });

    const nodeResult = await client.sendRequest<Record<string, unknown>>("node/operation", {
      opCode: "prompt/get",
      _meta: {},
    });
    expect(nodeResult).toMatchObject({
      opCode: "prompt/get",
      content: { type: "text", text: "prompt body" },
    });
  });

  it("returns invalid params for unknown agents", async () => {
    const { client, serverTransport } = createPair();
    const server = new OpServer({ name: "test-agent", version: "0.1.0" });
    servers.push(server);
    server.addAgent({ name: "demo" }, () => ({
      agentID: "demo",
      content: textContent("ok"),
    }));
    server.connect(serverTransport);

    await expect(
      client.sendRequest("agents/call", { agentID: "missing" }),
    ).rejects.toMatchObject({
      code: -32602,
    });
  });
});

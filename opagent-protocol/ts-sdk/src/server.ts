import {
  ErrorCodes,
  ResponseError,
  type MessageConnection,
} from "vscode-jsonrpc/node.js";
import { jsonContent, mergeMeta, textContent } from "./content.js";
import { StdioTransport } from "./transport.js";
import {
  Methods,
  negotiateProtocolVersion,
  OpCodes,
  type AgentMeta,
  type CallAgentParams,
  type CallAgentResult,
  type Content,
  type Implementation,
  type InfoNotificationParams,
  type InitializeParams,
  type InitializeResult,
  type Meta,
  type OpNodeParams,
  type OpNodeResult,
} from "./types.js";

export interface ServerOptions {
  instructions?: string;
  capabilities?: Record<string, unknown>;
  initializedHandler?: (req: ServerRequest<Record<string, unknown>>) => void | Promise<void>;
}

export interface AddAgentOptions {
  id?: string;
  aliases?: string[];
}

export interface ServerRequest<P> {
  params: P;
  session: ServerSession;
}

export type CallAgentHandler = (
  req: ServerRequest<CallAgentParams>,
) => CallAgentResult | Promise<CallAgentResult>;

export type OpNodeHandler = (
  req: ServerRequest<OpNodeParams>,
) => OpNodeResult | Promise<OpNodeResult>;

interface RegisteredAgent {
  meta: AgentMeta;
  handler: CallAgentHandler;
}

export class ServerSession {
  constructor(private readonly connection: MessageConnection) {}

  notifyInfo(params: InfoNotificationParams): Promise<void> {
    return this.connection.sendNotification(Methods.NotificationsInfo, params);
  }

  notifyMessage(content: Content, meta?: Meta): Promise<void> {
    return this.notifyInfo({
      opcode: OpCodes.NotifyMessage,
      content,
      _meta: meta,
    });
  }

  notifyText(text: string, meta?: Meta): Promise<void> {
    return this.notifyMessage(textContent(text), meta);
  }

  notifyJson(payload: unknown, meta?: Meta): Promise<void> {
    return this.notifyMessage(jsonContent(payload), meta);
  }
}

export class OpServer {
  private readonly agents = new Map<string, RegisteredAgent>();
  private opNodeHandler: OpNodeHandler | undefined;
  private connection: MessageConnection | undefined;
  private session: ServerSession | undefined;

  constructor(
    private readonly implementation: Implementation,
    private readonly options: ServerOptions = {},
  ) {
    if (!implementation?.name?.trim()) {
      throw new Error("OpServer implementation.name is required");
    }
    if (!implementation?.version?.trim()) {
      throw new Error("OpServer implementation.version is required");
    }
  }

  addAgent(meta: AgentMeta, handler: CallAgentHandler, options: AddAgentOptions = {}): void {
    if (!meta?.name?.trim()) {
      throw new Error("agent meta.name is required");
    }
    const keys = new Set<string>([
      meta.name.trim(),
      options.id?.trim() ?? "",
      ...(options.aliases ?? []).map((alias) => alias.trim()),
    ]);
    for (const key of keys) {
      if (key) {
        this.agents.set(key, { meta, handler });
      }
    }
  }

  onOpNode(handler: OpNodeHandler): void {
    this.opNodeHandler = handler;
  }

  connect(transport: StdioTransport = new StdioTransport()): ServerSession {
    if (this.connection) {
      throw new Error("OpServer is already connected");
    }
    const connection = transport.createConnection();
    const session = new ServerSession(connection);
    this.connection = connection;
    this.session = session;

    connection.onRequest(Methods.Initialize, async (params: InitializeParams) => {
      return this.handleInitialize(params);
    });
    connection.onNotification(Methods.Initialized, async (params: Record<string, unknown> = {}) => {
      await this.options.initializedHandler?.({ params, session });
    });
    connection.onRequest(Methods.Ping, async () => ({}));
    connection.onRequest(Methods.AgentsCall, async (params: CallAgentParams) => {
      return this.handleCallAgent(params, session);
    });
    connection.onRequest(Methods.NodeOperation, async (params: OpNodeParams) => {
      return this.handleOpNode(params, session);
    });
    connection.listen();
    return session;
  }

  async run(transport: StdioTransport = new StdioTransport()): Promise<void> {
    this.connect(transport);
    await new Promise<void>((resolve) => {
      this.connection?.onClose(() => resolve());
    });
  }

  close(): void {
    this.connection?.dispose();
    this.connection = undefined;
    this.session = undefined;
  }

  get currentSession(): ServerSession | undefined {
    return this.session;
  }

  private handleInitialize(params: InitializeParams): InitializeResult {
    if (!params || typeof params !== "object") {
      throw new ResponseError(ErrorCodes.InvalidParams, "initialize params are required");
    }
    return {
      protocolVersion: negotiateProtocolVersion(params.protocolVersion),
      serverInfo: this.implementation,
      capabilities: {
        logging: {},
        experimental: {
          opagent: {
            agents: this.agents.size > 0,
            nodeOperation: Boolean(this.opNodeHandler),
          },
        },
        ...this.options.capabilities,
      },
      instructions: this.options.instructions,
    };
  }

  private async handleCallAgent(
    params: CallAgentParams,
    session: ServerSession,
  ): Promise<CallAgentResult> {
    const agentID = params?.agentID?.trim();
    if (!agentID) {
      throw new ResponseError(ErrorCodes.InvalidParams, "agentID is required");
    }
    const agent = this.agents.get(agentID);
    if (!agent) {
      throw new ResponseError(ErrorCodes.InvalidParams, `unknown agent ${JSON.stringify(agentID)}`);
    }
    const result = await agent.handler({ params, session });
    if (!result?.content) {
      throw new ResponseError(ErrorCodes.InternalError, "agent handler returned no content");
    }
    return {
      agentID: result.agentID || params.agentID,
      content: result.content,
      _meta: mergeMeta(params._meta, result._meta),
    };
  }

  private async handleOpNode(params: OpNodeParams, session: ServerSession): Promise<OpNodeResult> {
    if (!params?.opCode?.trim()) {
      throw new ResponseError(ErrorCodes.InvalidParams, "opCode is required");
    }
    if (!this.opNodeHandler) {
      throw new ResponseError(ErrorCodes.MethodNotFound, "server does not support node/operation");
    }
    const result = await this.opNodeHandler({ params, session });
    if (!result?.content) {
      throw new ResponseError(ErrorCodes.InternalError, "node handler returned no content");
    }
    return {
      opCode: result.opCode || params.opCode,
      content: result.content,
      _meta: mergeMeta(params._meta, result._meta),
    };
  }
}

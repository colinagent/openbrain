export const JSON_RPC_VERSION = "2.0" as const;

export const PROTOCOL_VERSION_LATEST = "2025-06-18" as const;
export const SUPPORTED_PROTOCOL_VERSIONS = [
  "2025-06-18",
  "2025-03-26",
  "2024-11-05",
] as const;

export const Methods = {
  Initialize: "initialize",
  Initialized: "notifications/initialized",
  Ping: "ping",
  AgentsCall: "agents/call",
  NodeOperation: "node/operation",
  NotificationsInfo: "notifications/info",
} as const;

export const OpCodes = {
  AgentCall: "agent/call",
  AgentContinue: "agent/continue",
  AgentLoopCreate: "agent/loop/create",
  PromptGet: "prompt/get",
  AgentScan: "agent/scan",
  NodeList: "node/list",
  SystemStarted: "system/started",
  NotifyMessage: "notify/message",
  ConfigGet: "config/get",
  ConfigSystemGet: "config/system/get",
  ThreadCreate: "thread/create",
  ThreadFork: "thread/fork",
  ThreadMetaGet: "thread/meta/get",
  ThreadMetaUpdate: "thread/meta/update",
  ThreadSnapshotGet: "thread/snapshot/get",
  ThreadReviewList: "thread/review/list",
  ThreadReviewResolve: "thread/review/resolve",
  ThreadReviewRollback: "thread/review/rollback",
  EditorCompletion: "editor/completion",
  EditorCompletionCancel: "editor/completion/cancel",
  ThreadSubmit: "thread/submit",
  ThreadCompact: "thread/compact",
  ThreadInterrupted: "thread/interrupted",
  ThreadSteer: "thread/steer",
  ThreadFollowUp: "thread/follow_up",
  ThreadFollowUpPromote: "thread/follow_up/promote",
  ThreadQueueGet: "thread/queue/get",
  ThreadQueueRemove: "thread/queue/remove",
  ThreadActiveList: "thread/active/list",
} as const;

export type OpCode = (typeof OpCodes)[keyof typeof OpCodes] | (string & {});

export type Meta = Record<string, unknown>;

export interface Implementation {
  name: string;
  version: string;
}

export interface InitializeParams {
  protocolVersion: string;
  clientInfo: Implementation;
  capabilities: Record<string, unknown>;
  _meta?: Meta;
}

export interface InitializeResult {
  protocolVersion: string;
  serverInfo: Implementation;
  capabilities: Record<string, unknown>;
  instructions?: string;
  _meta?: Meta;
}

export interface TextContent {
  type: "text";
  text: string;
  annotations?: Record<string, unknown>;
}

export interface JsonContent {
  type: "json";
  payload: unknown;
}

export interface ImageContent {
  type: "image";
  mimeType: string;
  data: string;
  annotations?: Record<string, unknown>;
}

export interface AudioContent {
  type: "audio";
  mimeType: string;
  data: string;
  annotations?: Record<string, unknown>;
}

export interface ResourceLink {
  type: "resource_link";
  uri: string;
  name: string;
  title?: string;
  description?: string;
  mimeType?: string;
  size?: number;
  annotations?: Record<string, unknown>;
}

export interface EmbeddedResource {
  type: "resource";
  resource: {
    uri: string;
    mimeType?: string;
    text?: string;
    blob?: string;
  };
  annotations?: Record<string, unknown>;
}

export type Content =
  | TextContent
  | JsonContent
  | ImageContent
  | AudioContent
  | ResourceLink
  | EmbeddedResource;

export interface AgentMeta {
  name: string;
  description?: string;
  avatar?: string;
  maxToken?: number;
  bindAgentID?: string;
  toolServers?: string[];
  sysTools?: string[];
  skills?: string[];
  subAgents?: string[];
}

export interface CallAgentParams {
  agentID: string;
  content?: Content;
  _meta?: Meta;
}

export interface CallAgentResult {
  agentID: string;
  content: Content;
  _meta?: Meta;
}

export interface OpNodeParams {
  opCode: OpCode;
  content?: Content;
  _meta?: Meta;
}

export interface OpNodeResult {
  opCode: OpCode;
  content: Content;
  _meta?: Meta;
}

export interface InfoNotificationParams {
  opcode: OpCode;
  content: Content;
  _meta?: Meta;
}

export type MessageRole =
  | "system"
  | "developer"
  | "user"
  | "assistant"
  | "tool"
  | "function";

export interface MessageToolCall {
  id: string;
  name: string;
  arguments?: Record<string, unknown>;
  type?: string;
}

export interface Message {
  role: MessageRole;
  content?: string;
  content_parts?: unknown[];
  reasoning_content?: string;
  reasoning_replay_field?: string;
  reasoning_signature?: string;
  tool_calls?: MessageToolCall[];
  tool_call_id?: string;
  name?: string;
  timestamp?: number;
  usage?: Record<string, unknown>;
  stop_reason?: string;
  response_id?: string;
}

export interface TurnResultToolResult {
  toolName: string;
  argumentsObject?: Record<string, unknown>;
  resultText: string;
  isError?: boolean;
}

export interface TurnResultPayload {
  threadID: string;
  fileID?: string;
  turnID: string;
  agentID: string;
  path?: string;
  chatPath?: string;
  title: string;
  parentThreadID?: string;
  planTurn?: boolean;
  userMessage: Message;
  assistantText?: string;
  reasoningText?: string;
  toolResults?: TurnResultToolResult[];
  canonicalMessages?: unknown;
}

export function negotiateProtocolVersion(version: string | undefined): string {
  if (version && (SUPPORTED_PROTOCOL_VERSIONS as readonly string[]).includes(version)) {
    return version;
  }
  return PROTOCOL_VERSION_LATEST;
}

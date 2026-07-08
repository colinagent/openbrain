import { useAppStore } from '../store/appStore';
import { getChatWorkspaceStore } from '../store/chatWorkspaceStore';
import { useTabManagerStore } from '../store/tabManagerStore';

export type ChatCreateResult = {
  threadID: string;
  fileID: string;
  cwd?: string;
  title: string;
  path: string;
  chatPath?: string;
  initialContent?: string;
};

export type ThreadMeta = {
  threadID: string;
  fileID?: string;
  agentID: string;
  cwd: string;
  path?: string;
  chatPath: string;
  threadFilePath?: string;
  title: string;
  parentThreadID?: string;
  planPath?: string;
  executionPlanPath?: string;
};

export type ThreadRunStatus = 'idle' | 'running';
export type ThreadTailStatus = 'empty' | 'complete' | 'needs_continuation';
export type ThreadContinuationReason = '' | 'user_tail' | 'tool_result_tail' | 'assistant_tool_use' | 'assistant_error' | 'assistant_aborted';

export type ThreadEntry = {
  type?: string;
  id?: string;
  parentId?: string | null;
  timestamp?: string;
  [key: string]: unknown;
};

export type ThreadSnapshotEntryWindowQuery = {
  mode: 'tail' | 'before' | 'after';
  anchorId?: string | null;
  limit?: number;
};

export type ThreadSnapshotEntryWindow = {
  mode?: 'tail' | 'before' | 'after';
  anchorId?: string;
  limit?: number;
  start: number;
  end: number;
  total: number;
  hasBefore: boolean;
  hasAfter: boolean;
};

export type ChatControlQueueItem = {
  id: string;
  agentID?: string;
  agentName?: string;
  cwd?: string;
  message: {
    role: string;
    content?: string;
    content_parts?: Array<{
      type: 'text';
      text?: string;
    }>;
  };
  selectedSkillIDs?: string[];
  selectedSkillContext?: Record<string, unknown> | null;
  planTurn?: boolean;
};

export type ChatControlQueuedMessages = {
  steering?: ChatControlQueueItem[];
  followUp?: ChatControlQueueItem[];
};

export type ThreadContextUsage = {
  tokens?: number;
  contextWindow?: number;
  percentMilli?: number;
  known?: boolean;
};

export type ChatMessageRecord = {
  id: string;
  channelID: string;
  threadID: string;
  agentID: string;
  sender: 'user' | 'agent' | 'system';
  kind: 'message' | 'request' | 'status';
  status: 'open' | 'resolved' | 'archived';
  title?: string;
  body: string;
  actions?: Array<{ id: string; label: string; tone?: 'primary' | 'danger' }>;
  questions?: Array<{
    id: string;
    question: string;
    options?: Array<{ id: string; label: string }>;
  }>;
  replyToMessageID?: string;
  actionID?: string;
  answers?: Array<{ questionID: string; optionID?: string; label?: string; other?: boolean; text?: string }>;
  createdAt: string;
  updatedAt: string;
  meta?: Record<string, unknown>;
};

export type ChatMessageChannelSummary = {
  channelID: string;
  threadID: string;
  agentID: string;
  title?: string;
  lastMessage?: ChatMessageRecord;
  openCount?: number;
  unreadUserCount?: number;
  updatedAt?: string;
};

export type ThreadSnapshot = {
  meta: ThreadMeta;
  entries?: ThreadEntry[];
  entryWindow?: ThreadSnapshotEntryWindow;
  revision?: string;
  runStatus?: ThreadRunStatus;
  tailStatus?: ThreadTailStatus;
  continuationReason?: ThreadContinuationReason;
  queuedMessages?: ChatControlQueuedMessages;
  messageRecords?: ChatMessageRecord[];
  channelSummaries?: ChatMessageChannelSummary[];
  contextUsage?: ThreadContextUsage;
};

const inflightThreadMetaRequests = new Map<string, Promise<ThreadMeta>>();
const missingRemoteThreadMeta = new Set<string>();

function getActiveWorkspaceTabId(): string {
  return useTabManagerStore.getState().activeTabId;
}

function cacheThreadMeta(workspaceTabId: string, meta: ThreadMeta | null | undefined): void {
  if (!meta) {
    return;
  }
  const chatPath = normalizeChatPath(meta.path || meta.chatPath);
  const threadID = (meta.threadID || '').trim();
  if (!threadID) {
    return;
  }
  getChatWorkspaceStore(workspaceTabId).getState().upsertThreadMeta({
    threadID,
    fileID: (meta.fileID || '').trim(),
    agentID: (meta.agentID || '').trim(),
    cwd: (meta.cwd || '').trim(),
    chatPath,
    ...(meta.threadFilePath ? { threadFilePath: meta.threadFilePath.trim() } : {}),
    title: (meta.title || '').trim(),
    ...(meta.parentThreadID ? { parentThreadID: meta.parentThreadID.trim() } : {}),
    planPath: (meta.planPath || '').trim(),
    executionPlanPath: (meta.executionPlanPath || '').trim(),
  });
}

function resolveChatBaseUrl(workspaceTabId: string): string {
  const ws = useAppStore.getStoreByTabId(workspaceTabId).getState();
  const port = ws.remoteSession?.localPort;
  return port ? `http://127.0.0.1:${port}` : 'http://127.0.0.1:19530';
}

function normalizeChatPath(value: string | null | undefined): string {
  return (value || '').trim();
}

function buildThreadMetaRequestKey(
  workspaceTabId: string,
  query: { chatPath?: string | null; threadID?: string | null; fileID?: string | null; agentID?: string | null },
): string {
  return [
    workspaceTabId,
    normalizeChatPath(query.chatPath),
    (query.threadID || '').trim(),
    (query.fileID || '').trim(),
    (query.agentID || '').trim(),
  ].join('|');
}

function buildMissingRemoteMetaKey(workspaceTabId: string, chatPath: string): string {
  return `${workspaceTabId}|${normalizeChatPath(chatPath)}`;
}

function getCachedThreadMeta(chatPath: string, workspaceTabId: string): ThreadMeta | null {
  const normalizedPath = normalizeChatPath(chatPath);
  if (!normalizedPath) {
    return null;
  }
  return getChatWorkspaceStore(workspaceTabId).getState().getThreadMeta(normalizedPath);
}

export function buildThreadLookupByPath(
  chatPath: string,
  workspaceTabId = getActiveWorkspaceTabId(),
  overrides?: { threadID?: string | null; fileID?: string | null; agentID?: string | null },
): { chatPath: string; threadID?: string; fileID?: string; agentID?: string } {
  const normalizedPath = normalizeChatPath(chatPath);
  const cached = normalizedPath ? getCachedThreadMeta(normalizedPath, workspaceTabId) : null;
  const threadID = (overrides?.threadID || cached?.threadID || '').trim();
  const fileID = (overrides?.fileID || cached?.fileID || '').trim();
  const agentID = (overrides?.agentID || '').trim();
  return {
    chatPath: normalizedPath,
    ...(threadID ? { threadID } : {}),
    ...(fileID ? { fileID } : {}),
    ...(agentID ? { agentID } : {}),
  };
}

function retargetResolvedChatPath(
  workspaceTabId: string,
  previousPath: string,
  nextPath: string,
  title?: string
): void {
  const from = normalizeChatPath(previousPath);
  const to = normalizeChatPath(nextPath);
  if (!from || !to || from === to) {
    return;
  }
  useAppStore.getStoreByTabId(workspaceTabId).getState().retargetTabPath(from, to, title);
  getChatWorkspaceStore(workspaceTabId).getState().retargetChatPath(from, to);
}

function applyReturnedThreadMeta(
  workspaceTabId: string,
  requestedPath: string | null | undefined,
  meta: ThreadMeta,
): ThreadMeta {
  const requested = normalizeChatPath(requestedPath);
  const resolvedPath = normalizeChatPath(meta.path || meta.chatPath);
  if (requested && resolvedPath) {
    retargetResolvedChatPath(workspaceTabId, requested, resolvedPath, meta.title);
    missingRemoteThreadMeta.delete(buildMissingRemoteMetaKey(workspaceTabId, requested));
  }
  if (resolvedPath) {
    missingRemoteThreadMeta.delete(buildMissingRemoteMetaKey(workspaceTabId, resolvedPath));
  }
  cacheThreadMeta(workspaceTabId, meta);
  return {
    ...meta,
    chatPath: resolvedPath || meta.chatPath || '',
  };
}

type ThreadHTTPError = Error & {
  status?: number;
  serverMessage?: string;
};

function parseErrorResponseMessage(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) {
    return '';
  }
  try {
    const payload = JSON.parse(trimmed) as { error?: unknown; message?: unknown };
    const message = typeof payload.error === 'string'
      ? payload.error
      : (typeof payload.message === 'string' ? payload.message : '');
    return message.trim() || trimmed;
  } catch {
    return trimmed;
  }
}

function isMissingThreadMessage(message: string): boolean {
  const normalized = message.trim().toLowerCase();
  return normalized === 'file does not exist'
    || normalized.endsWith(': file does not exist')
    || normalized.includes('thread not found');
}

function buildErrorResponseMessage(fallbackMessage: string, status: number, serverMessage: string): string {
  if (fallbackMessage === 'Get thread meta failed' && isMissingThreadMessage(serverMessage)) {
    return `${fallbackMessage}: thread not found`;
  }
  return serverMessage
    ? `${fallbackMessage}: ${serverMessage}`
    : `${fallbackMessage}: HTTP ${status}`;
}

async function parseJSONResponse<T>(res: Response, fallbackMessage: string): Promise<T> {
  if (!res.ok) {
    const errText = (await res.text().catch(() => '')).trim();
    const serverMessage = parseErrorResponseMessage(errText);
    const error = new Error(buildErrorResponseMessage(fallbackMessage, res.status, serverMessage)) as ThreadHTTPError;
    error.status = res.status;
    error.serverMessage = serverMessage;
    throw error;
  }
  return res.json() as Promise<T>;
}

export async function createThread(
  cwd: string,
  userInput: string,
  agentID: string,
  workspaceTabId = getActiveWorkspaceTabId(),
  signal?: AbortSignal,
): Promise<ChatCreateResult> {
  const normalizedCwd = (cwd || '').trim();
  const res = await fetch(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/create`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    ...(signal ? { signal } : {}),
    body: JSON.stringify({
      ...(normalizedCwd ? { cwd: normalizedCwd } : {}),
      userInput,
      agentID,
    }),
  });
  return parseJSONResponse<ChatCreateResult>(res, 'Create chat failed');
}

export async function getThreadMeta(
  query: { chatPath?: string | null; threadID?: string | null; fileID?: string | null; agentID?: string | null },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const url = new URL(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/meta`);
  if (query.chatPath) {
    url.searchParams.set('chatPath', query.chatPath);
  }
  if (query.threadID) {
    url.searchParams.set('threadID', query.threadID);
  }
  if (query.fileID) {
    url.searchParams.set('fileID', query.fileID);
  }
  if (query.agentID) {
    url.searchParams.set('agentID', query.agentID);
  }
  const res = await fetch(url.toString());
  const meta = await parseJSONResponse<ThreadMeta>(res, 'Get thread meta failed');
  cacheThreadMeta(workspaceTabId, meta);
  return meta;
}

export async function getThreadSnapshot(
  query: {
    chatPath?: string | null;
    threadID?: string | null;
    fileID?: string | null;
    agentID?: string | null;
    modelKey?: string | null;
    entryWindow?: ThreadSnapshotEntryWindowQuery | null;
  },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadSnapshot> {
  const url = new URL(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/snapshot`);
  if (query.chatPath) {
    url.searchParams.set('chatPath', query.chatPath);
  }
  if (query.threadID) {
    url.searchParams.set('threadID', query.threadID);
  }
  if (query.fileID) {
    url.searchParams.set('fileID', query.fileID);
  }
  if (query.agentID) {
    url.searchParams.set('agentID', query.agentID);
  }
  if (query.modelKey) {
    url.searchParams.set('modelKey', query.modelKey);
  }
  if (query.entryWindow) {
    url.searchParams.set('entryWindow', query.entryWindow.mode);
    if (query.entryWindow.anchorId) {
      url.searchParams.set('entryAnchorId', query.entryWindow.anchorId);
    }
    if (typeof query.entryWindow.limit === 'number' && Number.isFinite(query.entryWindow.limit)) {
      url.searchParams.set('entryLimit', String(Math.max(0, Math.floor(query.entryWindow.limit))));
    }
  }
  const res = await fetch(url.toString());
  const snapshot = await parseJSONResponse<ThreadSnapshot>(res, 'Get chat thread snapshot failed');
  const requestedPath = normalizeChatPath(query.chatPath);
  return {
    ...snapshot,
    meta: applyReturnedThreadMeta(workspaceTabId, requestedPath, snapshot.meta),
  };
}

function cacheResolvedRemoteThreadMeta(
  workspaceTabId: string,
  requestedPath: string,
  meta: ThreadMeta,
): ThreadMeta {
  return applyReturnedThreadMeta(workspaceTabId, requestedPath, meta);
}

export async function getRuntimeThreadMeta(
  query: { chatPath?: string | null; threadID?: string | null; fileID?: string | null; agentID?: string | null },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const requestedPath = normalizeChatPath(query.chatPath);
  const meta = await getThreadMeta(query, workspaceTabId);
  return cacheResolvedRemoteThreadMeta(workspaceTabId, requestedPath, meta);
}

export async function getResolvedThreadMeta(
  query: { chatPath?: string | null; threadID?: string | null; fileID?: string | null; agentID?: string | null },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const requestedPath = normalizeChatPath(query.chatPath);
  const canUseMissingRemoteCache = requestedPath
    && !(query.threadID || '').trim()
    && !(query.fileID || '').trim()
    && !(query.agentID || '').trim();
  const requestKey = buildThreadMetaRequestKey(workspaceTabId, query);
  const existingRequest = inflightThreadMetaRequests.get(requestKey);
  if (existingRequest) {
    return existingRequest;
  }

  if (canUseMissingRemoteCache) {
    const missingKey = buildMissingRemoteMetaKey(workspaceTabId, requestedPath);
    const fallback = getCachedThreadMeta(requestedPath, workspaceTabId);
    if (fallback && missingRemoteThreadMeta.has(missingKey)) {
      cacheThreadMeta(workspaceTabId, fallback);
      return fallback;
    }
  }

  const request = (async () => {
    let lastError: unknown = null;
    try {
      const meta = await getThreadMeta(query, workspaceTabId);
      return cacheResolvedRemoteThreadMeta(workspaceTabId, requestedPath, meta);
    } catch (error) {
      lastError = error;
    }

    if (!requestedPath) {
      throw lastError;
    }

    const fallback = getCachedThreadMeta(requestedPath, workspaceTabId);
    if (fallback) {
      if (canUseMissingRemoteCache) {
        missingRemoteThreadMeta.add(buildMissingRemoteMetaKey(workspaceTabId, requestedPath));
      }
      cacheThreadMeta(workspaceTabId, fallback);
      return fallback;
    }

    throw lastError;
  })().finally(() => {
    inflightThreadMetaRequests.delete(requestKey);
  });

  inflightThreadMetaRequests.set(requestKey, request);
  return request;
}

export async function resolveCurrentChatPath(
  chatPath: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  const normalizedPath = normalizeChatPath(chatPath);
  if (!normalizedPath) {
    return '';
  }
  const meta = await getRuntimeThreadMeta(buildThreadLookupByPath(normalizedPath, workspaceTabId), workspaceTabId);
  return normalizeChatPath(meta.path || meta.chatPath) || normalizedPath;
}

export function primeLocalThreadMeta(
  chatPath: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): ThreadMeta | null {
  return getCachedThreadMeta(chatPath, workspaceTabId);
}

export async function updateThreadMeta(
  payload: {
    chatPath?: string | null;
    threadID?: string | null;
    fileID?: string | null;
    title?: string | null;
    planPath?: string | null;
    executionPlanPath?: string | null;
  },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const normalizedChatPath = normalizeChatPath(payload.chatPath);
  const normalizedThreadID = (payload.threadID || '').trim();
  const normalizedFileID = (payload.fileID || '').trim();
  let resolvedPayload = {
    ...payload,
    ...(normalizedChatPath ? { chatPath: normalizedChatPath } : {}),
    ...(normalizedThreadID ? { threadID: normalizedThreadID } : {}),
    ...(normalizedFileID ? { fileID: normalizedFileID } : {}),
  };

  if (normalizedChatPath && (!normalizedThreadID || !normalizedFileID)) {
    const cached = getCachedThreadMeta(normalizedChatPath, workspaceTabId);
    let resolvedMeta = cached;
    if ((!resolvedMeta?.threadID || !resolvedMeta?.fileID)) {
      resolvedMeta = await getResolvedThreadMeta(
        buildThreadLookupByPath(normalizedChatPath, workspaceTabId, {
          threadID: normalizedThreadID || null,
          fileID: normalizedFileID || null,
        }),
        workspaceTabId,
      );
    }
    if (resolvedMeta) {
      if (!normalizedThreadID && (resolvedMeta.threadID || '').trim()) {
        resolvedPayload = { ...resolvedPayload, threadID: resolvedMeta.threadID.trim() };
      }
      const resolvedFileID = (resolvedMeta.fileID || '').trim();
      if (!normalizedFileID && resolvedFileID) {
        resolvedPayload = { ...resolvedPayload, fileID: resolvedFileID };
      }
    }
  }

  const res = await fetch(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/meta`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(resolvedPayload),
  });
  const meta = await parseJSONResponse<ThreadMeta>(res, 'Update thread meta failed');
  return applyReturnedThreadMeta(workspaceTabId, resolvedPayload.chatPath, meta);
}

export async function retitleThread(
  payload: {
    chatPath?: string | null;
    threadID?: string | null;
    fileID?: string | null;
    title: string;
  },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const res = await fetch(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/retitle`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const meta = await parseJSONResponse<ThreadMeta>(res, 'Retitle thread failed');
  return applyReturnedThreadMeta(workspaceTabId, payload.chatPath, meta);
}

export async function forkThread(
  payload: {
    sourceThreadID?: string | null;
    sourceFileID?: string | null;
    sourceChatPath?: string | null;
    cwd: string;
    agentID: string;
    title: string;
    chatBaseDir?: string | null;
    chatFileName?: string | null;
    planPath?: string | null;
    executionPlanPath?: string | null;
  },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ThreadMeta> {
  const res = await fetch(`${resolveChatBaseUrl(workspaceTabId)}/v1/thread/fork`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const meta = await parseJSONResponse<ThreadMeta>(res, 'Fork thread failed');
  cacheThreadMeta(workspaceTabId, meta);
  return meta;
}

import { useStore } from 'zustand';
import { createStore, type StoreApi } from 'zustand/vanilla';
import { rendererI18n } from '../../main/i18n/renderer';
import { getChatWorkspaceStore } from './chatWorkspaceStore';
import {
  createFileService,
  type FileEntry,
  type FileChange,
  type ReaddirResult,
  type SearchFileResult,
  type StatResult,
} from '../services/fileService';
import {
  createGitService,
  type GitBranch,
  type GitDirtySummary,
} from '../services/gitService';
import {
  createReviewService,
  type ThreadReviewState,
} from '../services/reviewService';
import {
  createAgentService,
  type OpNode,
  type SystemConfigResult,
} from '../services/agentService';
import { createMarketplaceService } from '../services/marketplaceService';
import {
  createMessengerService,
  type MessengerReplyInput,
} from '../services/messengerService';
import { buildMarketplaceUsageReport } from '../services/marketplaceUsage';
import {
  createStorageService,
  type WorkspaceStorageModelParams,
  type WorkspaceStorageStatusResult,
} from '../services/storageService';
import { useMessengerStore } from './messengerStore';
import { useModelsStore } from './modelsStore';
import {
  CRON_TASK_HISTORY_LIMIT,
  createCronService,
  type CronRunResult,
  type CronTaskHistoryEntry,
  type CronTask,
  type CronTaskRecord,
} from '../services/cronService';
import {
  WSConnection,
  type ConnectionCallbacks,
  type ConnectionState,
} from '../services/wsConnection';
import {
  useTabManagerStore,
  type WorkspaceChatTabSession,
} from './tabManagerStore';
import { upsertSingletonEditorTab } from './singletonEditorTab';
import { NEW_TAB_TITLE, retargetActiveBlankNewTab } from './newEditorTab';
import { dirnamePosix, normalizePosixPath } from '../utils/markdownMedia';
import { base64ToFile, importFile } from '../services/resourceService';
import { canonicalFileURI, type CanonicalFileURI } from '../core/resource/uri';
import {
  buildCustomAgentTemplate,
  buildReferenceAgentMarkdown,
  normalizeAgentNodeID,
} from '../utils/agentFrontmatter';
import { findChatCapableAgentOpcode } from '../utils/agentSwitch';
import {
  getChatWorkdir,
  isThreadChatPath as isThreadConversationPath,
  type ChatAgentTarget,
} from '../utils/chatAgentTarget';
import {
  isConversationMarkdownContent,
  parseCanonicalChatFrontmatter,
} from '../utils/frontmatterParser';
import type {
  WorkspaceStorageBinding,
  WorkspaceSyncPolicy,
} from '../types/electron';
import { resolveDefaultChatModelSelection } from '../utils/chatModelSelection';
import { resolveChatModelPreference } from '../utils/chatModelPreferences';

const EMPTY_ENTRIES: FileEntry[] = [];

type RestoreChatTabsResult = {
  restoredPaths: string[];
  selectedPath: string | null;
};

export type EditorRevealTarget = {
  line: number;
  column?: number;
};

export type BookHighlightRectTarget = {
  top: number;
  left: number;
  width: number;
  height: number;
};

export type BookOpenTarget =
  | { format: 'epub'; cfi: string; text?: string | null }
  | {
      format: 'pdf';
      page: number;
      rects?: BookHighlightRectTarget[];
      text?: string | null;
    };

export type EditorReviewLineRange = {
  startLine: number;
  endLine: number;
};

export type EditorReviewHunk = {
  oldStartLine: number;
  oldLineCount: number;
  newStartLine: number;
  newLineCount: number;
  removedLines?: string[];
  addedLines?: string[];
};

export type EditorReviewOverlay = {
  filePath: string;
  threadID: string;
  turnID: string;
  chatPath: string;
  changedRanges: EditorReviewLineRange[];
  hunks: EditorReviewHunk[];
};

export type EditorCompletionBlock = {
  text: string;
  start?: number;
  end?: number;
  kind?: string;
  language?: string;
};

export type EditorCompletionRequest = {
  requestID: string;
  agentID?: string | null;
  modelKey: string;
  thinkingLevel?: string | null;
  editorKind: string;
  languageId?: string | null;
  documentPath?: string | null;
  cursorOffset: number;
  prefix?: string;
  suffix?: string;
  currentBlock?: EditorCompletionBlock | null;
  previousBlock?: EditorCompletionBlock | null;
  nextBlock?: EditorCompletionBlock | null;
  maxOutputTokens?: number;
};

export type EditorCompletionResult = {
  requestID: string;
  insertText: string;
  replaceFrom: number;
  replaceTo: number;
  stopReason?: string;
  modelKey?: string;
};

type EditorRandomIDResult = {
  id?: string;
};

export type SidebarSearchFlags = {
  matchCase: boolean;
  wholeWord: boolean;
  regex: boolean;
};

export type EditorTab = {
  id: string;
  title: string;
  uri?: CanonicalFileURI;
  filePath?: string;
  editorId: string;
  content: string;
  isDirty: boolean;
  missing?: boolean;
  resourceVersion?: number;
  pendingScrollHeading: string | null;
  pendingRevealTarget?: EditorRevealTarget | null;
  pendingBookTarget?: BookOpenTarget | null;
  documentRole?: 'editor' | 'conversation';
  threadID?: string;
};

export type DocumentTab = EditorTab;

export type PendingDirtyTabClose = {
  tabId: string;
  title: string;
};

export type EditorFocusRequest = {
  tabId: string;
  seq: number;
};

export type RemoteSessionInfo = {
  hostLabel: string;
  localPort: number;
  remotePort: number;
  wsUrl: string;
  httpUrl: string;
  remoteHome: string;
  workspaceDir: string;
  installDir: string;
};

type DerivedDirs = {
  baseDir: string;
  workspaceDir: string;
  agentsDir: string;
  instanceID: string;
};

type VisibleTreeSnapshot = {
  remoteIdentity: string;
  currentDir: string | null;
  expandedDirs: string[];
};

export type AgentBinding = {
  cwd: string;
  localNodeID: string | null;
  effectiveAgentID: string;
  source: 'local' | 'bind';
};

export type AgentSubagentInfo = {
  id: string;
  name: string | null;
  uri: string | null;
  path: string | null;
};

function createId(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function getFileTitle(path: string) {
  return path.split('/').pop() || path;
}

function authorityIdFromState(
  state: Pick<AppState, 'instanceID' | 'remoteSession'>,
): string {
  const instanceID = (state.instanceID || '').trim();
  if (instanceID) {
    return instanceID;
  }
  const session = state.remoteSession;
  if (session) {
    const hostLabel = (session.hostLabel || '').trim();
    const remoteHome = normalizePosixPath(session.remoteHome || '');
    const installDir = normalizePosixPath(session.installDir || '');
    if (hostLabel && remoteHome && installDir) {
      return `remote:${hostLabel}|${remoteHome}|${installDir}`;
    }
  }
  return 'local:default';
}

function pathToDocumentURI(
  path: string | null | undefined,
  state: Pick<AppState, 'instanceID' | 'remoteSession'>,
): CanonicalFileURI | null {
  const normalized = normalizePosixPath((path || '').trim());
  if (!normalized || !normalized.startsWith('/')) {
    return null;
  }
  return canonicalFileURI(authorityIdFromState(state), normalized);
}

function matchesDocumentTab(
  tab: Pick<DocumentTab, 'uri' | 'filePath'>,
  _path: string,
  uri: CanonicalFileURI | null | undefined,
): boolean {
  return Boolean(uri && tab.uri && tab.uri === uri);
}

function resolveDocumentRole(
  content: string,
  editorId: string,
): 'editor' | 'conversation' {
  if (editorId === 'markdown' && isConversationMarkdownContent(content)) {
    return 'conversation';
  }
  return 'editor';
}

function normalizeDocumentTab(tab: DocumentTab): DocumentTab {
  const documentRole = resolveDocumentRole(tab.content, tab.editorId);
  const chatFrontmatter =
    documentRole === 'conversation'
      ? parseCanonicalChatFrontmatter(tab.content)
      : null;
  return {
    ...tab,
    documentRole,
    ...(chatFrontmatter?.threadID
      ? { threadID: chatFrontmatter.threadID }
      : { threadID: undefined }),
  };
}

function ensureDocumentTabURI(
  tab: DocumentTab,
  state: Pick<AppState, 'instanceID' | 'remoteSession'>,
): DocumentTab {
  if (tab.uri || !(tab.filePath || '').trim()) {
    return tab;
  }
  const uri = pathToDocumentURI(tab.filePath, state);
  return uri ? { ...tab, uri } : tab;
}

function isConversationDocument(
  tab: DocumentTab | null | undefined,
): tab is DocumentTab & { documentRole: 'conversation' } {
  return tab?.documentRole === 'conversation';
}

function isBinaryPreviewDocumentTab(
  tab: Pick<DocumentTab, 'editorId'> | null | undefined,
): boolean {
  return (
    tab?.editorId === 'image' ||
    tab?.editorId === 'pdf' ||
    tab?.editorId === 'book'
  );
}

export function getEditorDocuments(documents: DocumentTab[]): DocumentTab[] {
  return documents.filter((tab) => !isConversationDocument(tab));
}

function getOpenDocuments(state: Pick<AppState, 'documents'>): DocumentTab[] {
  return state.documents;
}

function getDocumentChatPath(
  tab: DocumentTab | null | undefined,
): string | null {
  if (!isConversationDocument(tab)) {
    return null;
  }
  const path = normalizePosixPath((tab.filePath || '').trim());
  return path || null;
}

function isSameOrChildPath(
  path: string | null | undefined,
  rootPath: string,
): boolean {
  const candidate = (path || '').trim();
  return candidate === rootPath || candidate.startsWith(`${rootPath}/`);
}

function replaceMovedPath(
  path: string | null | undefined,
  ops: Array<{ oldPath: string; newPath: string }>,
): string | null {
  const candidate = (path || '').trim();
  if (!candidate) {
    return null;
  }
  for (const { oldPath, newPath } of ops) {
    if (candidate === oldPath) {
      return newPath;
    }
    if (candidate.startsWith(`${oldPath}/`)) {
      return `${newPath}${candidate.slice(oldPath.length)}`;
    }
  }
  return candidate;
}

function replaceMovedEntries(
  dir: string,
  entries: FileEntry[],
  ops: Array<{ oldPath: string; newPath: string }>,
): FileEntry[] {
  let changed = false;
  const nextEntries = entries.map((entry) => {
    const entryPath = normalizePosixPath(
      `${dir.replace(/\/+$/, '')}/${entry.name}`,
    );
    const nextPath = replaceMovedPath(entryPath, ops);
    if (!nextPath || nextPath === entryPath) {
      return entry;
    }
    changed = true;
    return {
      ...entry,
      name: getFileTitle(nextPath),
    };
  });
  return changed ? nextEntries : entries;
}

function formatImageTimestamp(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');
  const millis = String(date.getMilliseconds()).padStart(3, '0');
  return `${year}${month}${day}${hours}${minutes}${seconds}${millis}`;
}

function buildPastedImageMarkdown(relativePath: string, altText?: string) {
  const rawPath = (relativePath || '').trim();
  const normalizedPath = normalizePosixPath(rawPath);
  const trimmedPath = normalizedPath.startsWith('/')
    ? normalizedPath
    : `./${normalizedPath.replace(/^\.?\//, '')}`;
  const fallback =
    trimmedPath
      .split('/')
      .pop()
      ?.replace(/\.[^.]+$/i, '') || 'image';
  const alt = (altText || '').trim() || fallback;
  return `![${alt}](${trimmedPath})`;
}

function normalizeImageExtension(extension: string) {
  const trimmed = extension.trim().toLowerCase();
  if (!trimmed) {
    return '.png';
  }
  return trimmed.startsWith('.') ? trimmed : `.${trimmed}`;
}

const THREAD_MARKER_LINE_RE = /^@[^\s]+$/;

function normalizeWorkspaceChatPath(path: string | null | undefined): string {
  return normalizePosixPath((path || '').trim());
}

function isThreadChatPath(path: string | null | undefined): boolean {
  return isThreadConversationPath(normalizeWorkspaceChatPath(path));
}

function resolveChatHttpBaseUrl(
  remoteSession: RemoteSessionInfo | null | undefined,
): string {
  const port = remoteSession?.localPort;
  return port ? `http://127.0.0.1:${port}` : 'http://127.0.0.1:19530';
}

async function syncRenamedThreadProjection(
  remoteSession: RemoteSessionInfo | null,
  oldPath: string,
  newPath: string,
) {
  const from = normalizeWorkspaceChatPath(oldPath);
  const to = normalizeWorkspaceChatPath(newPath);
  if (!isThreadChatPath(from) || !isThreadChatPath(to)) {
    return;
  }
  const baseUrl = resolveChatHttpBaseUrl(remoteSession);
  const query = new URL(`${baseUrl}/v1/thread/meta`);
  query.searchParams.set('chatPath', from);
  const metaRes = await fetch(query.toString());
  if (!metaRes.ok) {
    return;
  }
  const meta = (await metaRes.json()) as { threadID?: string };
  const threadID = (meta.threadID || '').trim();
  if (!threadID) {
    return;
  }
  await fetch(`${baseUrl}/v1/thread/meta`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      threadID,
      chatPath: to,
      title: getFileTitle(to),
    }),
  });
}

function isThreadRoundHeaderChunk(text: string): boolean {
  const normalized = text.replace(/\r\n/g, '\n');
  if (!normalized.startsWith('@')) {
    return false;
  }
  const lines = normalized.split('\n');
  if (lines.length < 2) {
    return false;
  }
  const markerLines = lines
    .map((line) => line.trim())
    .filter((line) => THREAD_MARKER_LINE_RE.test(line));
  if (markerLines.length < 2) {
    return false;
  }
  const firstLine = lines[0].trim();
  if (!THREAD_MARKER_LINE_RE.test(firstLine)) {
    return false;
  }
  let lastNonEmptyLine = '';
  for (let i = lines.length - 1; i >= 0; i -= 1) {
    const line = lines[i].trim();
    if (line) {
      lastNonEmptyLine = line;
      break;
    }
  }
  return THREAD_MARKER_LINE_RE.test(lastNonEmptyLine);
}

function getThreadRoundSeparatorPrefix(currentContent: string): string {
  if (!currentContent) {
    return '';
  }
  if (currentContent.endsWith('\n\n')) {
    return '';
  }
  if (currentContent.endsWith('\n')) {
    return '\n';
  }
  return '\n\n';
}

function toActiveState(tab: EditorTab) {
  return {
    currentFileURI: tab.uri || null,
    currentFilePath: tab.filePath || null,
    fileContent: tab.content,
    isDirty: tab.isDirty,
    pendingScrollHeading: tab.pendingScrollHeading,
    pendingRevealTarget: tab.pendingRevealTarget || null,
    pendingBookTarget: tab.pendingBookTarget || null,
    currentReviewOverlay: null,
    editorId: tab.editorId,
    editorFocusRequest: null,
  };
}

function nextEditorFocusRequest(
  state: { editorFocusRequest: EditorFocusRequest | null },
  tabId: string | null | undefined,
  shouldFocus: boolean,
): EditorFocusRequest | null {
  const id = (tabId || '').trim();
  if (!shouldFocus || !id) {
    return null;
  }
  return {
    tabId: id,
    seq: (state.editorFocusRequest?.seq ?? 0) + 1,
  };
}

function isPinnableEditorTab(tab: EditorTab | null | undefined): boolean {
  return Boolean((tab?.filePath || '').trim());
}

function resolvePinnedTabId(
  pinnedTabId: string | undefined,
  tabs: EditorTab[],
): string | undefined {
  if (!pinnedTabId) {
    return undefined;
  }
  return tabs.some((tab) => tab.id === pinnedTabId && isPinnableEditorTab(tab))
    ? pinnedTabId
    : undefined;
}

function patchDocumentsWithPinnedState(
  state: Pick<AppState, 'pinnedTabId' | 'instanceID' | 'remoteSession'>,
  tabs: DocumentTab[],
): Pick<AppState, 'documents' | 'pinnedTabId'> {
  const documents = tabs.map((tab) =>
    normalizeDocumentTab(ensureDocumentTabURI(tab, state)),
  );
  return {
    documents,
    pinnedTabId: resolvePinnedTabId(state.pinnedTabId, documents),
  };
}

export type GitInfo = {
  status: 'idle' | 'loading' | 'ready';
  error: string | null;
  isRepo: boolean;
  repoRoot: string | null;
  currentBranch: string | null;
  detached: boolean;
  detachedLabel: string | null;
  branches: GitBranch[];
  dirty: GitDirtySummary;
  loadedPath: string | null;
};

const EMPTY_GIT_DIRTY: GitDirtySummary = {
  changedFiles: 0,
  addedLines: 0,
  deletedLines: 0,
  hasChanges: false,
};

export type WorkspaceStorageInfo = {
  status: 'idle' | 'loading' | 'syncing' | 'synced' | 'error' | 'local';
  error: string | null;
  workspaceID: string | null;
  path: string | null;
  storage: WorkspaceStorageBinding | null;
  policy: WorkspaceSyncPolicy;
  lastSyncAt: string | null;
  message: string | null;
};

function defaultWorkspaceSyncPolicy(enabled = false): WorkspaceSyncPolicy {
  return {
    autoSync: enabled,
    onOpen: false,
    onLocalChange: false,
    intervalSec: enabled ? 300 : 0,
    conflict: 'keep-both',
    deleteMode: 'keep',
  };
}

function normalizeStorageStatus(
  status: string | null | undefined,
): WorkspaceStorageInfo['status'] {
  switch ((status || '').trim()) {
    case 'loading':
    case 'syncing':
    case 'synced':
    case 'error':
    case 'local':
      return status as WorkspaceStorageInfo['status'];
    default:
      return 'idle';
  }
}

function createEmptyWorkspaceStorageInfo(): WorkspaceStorageInfo {
  return {
    status: 'idle',
    error: null,
    workspaceID: null,
    path: null,
    storage: null,
    policy: defaultWorkspaceSyncPolicy(false),
    lastSyncAt: null,
    message: null,
  };
}

function workspaceStorageErrorMessage(value: string | null): string | null {
  const raw = (value || '').trim();
  if (!raw) {
    return null;
  }
  const lower = raw.toLowerCase();
  if (
    lower.includes('write access to repository not granted') ||
    lower.includes('the requested url returned error: 403') ||
    lower.includes('github app cannot write this workspace repository')
  ) {
    return 'GitHub App does not have write access to this repository. Update the OpenBrain GitHub App installation to grant Contents read/write and include this repository, then try Sync now.';
  }
  if (
    lower.includes('contents read/write') ||
    lower.includes('contents: write')
  ) {
    return raw;
  }
  return raw;
}

function storageInfoFromResult(
  result: WorkspaceStorageStatusResult,
  fallbackPath: string,
  pendingStatus?: WorkspaceStorageInfo['status'],
): WorkspaceStorageInfo {
  const storage = result.storage || null;
  const policy =
    result.policy ||
    storage?.syncPolicy ||
    defaultWorkspaceSyncPolicy(Boolean(storage?.enabled));
  const error = workspaceStorageErrorMessage(
    result.error || result.lastError || null,
  );
  return {
    status: error
      ? 'error'
      : pendingStatus || normalizeStorageStatus(result.status),
    error,
    workspaceID: (result.workspaceID || '').trim() || null,
    path: (result.path || fallbackPath || '').trim() || null,
    storage,
    policy,
    lastSyncAt: (result.lastSyncAt || '').trim() || null,
    message: (result.message || '').trim() || null,
  };
}

async function ensureModelsLoadedForExecutionParams(): Promise<void> {
  const modelsStore = useModelsStore.getState();
  if (!modelsStore.config.models.some((model) => model.enabled)) {
    await modelsStore.load().catch(() => undefined);
  }
}

function modelParamsForModelKey(modelKey: string | null | undefined): WorkspaceStorageModelParams | null {
  const config = useModelsStore.getState().config;
  const normalizedModelKey = (modelKey || '').trim();
  if (!normalizedModelKey) {
    return null;
  }
  const model = config.models.find((candidate) => candidate.enabled && candidate.key === normalizedModelKey);
  if (!model) {
    return null;
  }
  const preference = resolveChatModelPreference(config, model);
  return {
    modelKey: normalizedModelKey,
    ...(preference.thinkingLevel ? { thinkingLevel: preference.thinkingLevel } : {}),
    ...(preference.contextWindow ? { contextWindow: preference.contextWindow } : {}),
    ...(preference.serviceTier ? { serviceTier: preference.serviceTier } : {}),
  };
}

async function resolveDefaultChatModelParams(): Promise<WorkspaceStorageModelParams> {
  await ensureModelsLoadedForExecutionParams();
  const config = useModelsStore.getState().config;
  const selection = resolveDefaultChatModelSelection(config);
  if (!selection.modelKey) {
    return {};
  }
  return modelParamsForModelKey(selection.modelKey) || {};
}

async function resolveOpenBrainCloudSyncModelParams(): Promise<WorkspaceStorageModelParams> {
  return resolveDefaultChatModelParams();
}

async function resolveMessengerReplyInput(input: MessengerReplyInput, workspaceTabId: string): Promise<MessengerReplyInput> {
  await ensureModelsLoadedForExecutionParams();
  const chatState = getChatWorkspaceStore(workspaceTabId).getState();
  const selectedModelKey = chatState.getModelKeyForTarget(chatState.selectedConversationTarget);
  const modelParams = modelParamsForModelKey(input.modelKey)
    || modelParamsForModelKey(selectedModelKey)
    || await resolveDefaultChatModelParams();
  return {
    ...input,
    ...modelParams,
  };
}

function createEmptyGitInfo(): GitInfo {
  return {
    status: 'idle',
    error: null,
    isRepo: false,
    repoRoot: null,
    currentBranch: null,
    detached: false,
    detachedLabel: null,
    branches: [],
    dirty: { ...EMPTY_GIT_DIRTY },
    loadedPath: null,
  };
}

interface AppState {
  // Connection state
  connectionState: ConnectionState;
  serverUrl: string;
  remoteSession: RemoteSessionInfo | null;
  remoteConnecting: boolean;
  remoteError: string | null;
  baseDir: string | null;
  workspaceRootDir: string | null;
  agentsRootDir: string | null;
  instanceID: string | null;

  // File system state
  currentDir: string | null;
  entries: FileEntry[];
  dirEntries: Map<string, FileEntry[]>;
  dirLoading: Set<string>;
  dirErrors: Map<string, string>;
  dirWatchIds: Map<string, string>;
  expandedDirs: Set<string>;
  requestRootAction: 'new-file' | 'new-folder' | null;
  sidebarSearchQuery: string;
  sidebarSearchIncludes: string;
  sidebarSearchExcludes: string;
  sidebarSearchFlags: SidebarSearchFlags;
  sidebarSearchLoading: boolean;
  sidebarSearchError: string | null;
  sidebarSearchResults: SearchFileResult[];
  sidebarSearchTotalCount: number;
  sidebarSearchTruncated: boolean;
  sidebarSearchRequestSeq: number;

  // Node records and workspace-directory agent bindings.
  // Bump nodeGraphRevision whenever the graph is replaced so UI selectors can
  // subscribe to graph changes without depending on a specific Map reference.
  nodesByID: Map<string, OpNode>;
  nodeGraphRevision: number;
  agentBindingByCwd: Map<string, AgentBinding>;
  agentNodes: OpNode[];
  skillNodes: OpNode[];
  agentNodesLoading: boolean;
  gitInfo: GitInfo;
  storageInfo: WorkspaceStorageInfo;

  // Editor state
  currentFileURI: CanonicalFileURI | null;
  currentFilePath: string | null;
  fileContent: string;
  isDirty: boolean;
  pendingScrollHeading: string | null;
  pendingRevealTarget: EditorRevealTarget | null;
  pendingBookTarget: BookOpenTarget | null;
  currentReviewOverlay: EditorReviewOverlay | null;
  editorId: string | null; // 'markdown' | 'text' | etc.
  editorFocused: boolean;
  editorBlurRequestSeq: number;
  editorFocusRequest: EditorFocusRequest | null;
  documents: DocumentTab[];
  activeTabId?: string;
  pinnedTabId?: string;
  pendingDirtyTabClose: PendingDirtyTabClose | null;

  // Actions
  connect: () => void;
  setServerUrl: (url: string) => void;
  setRemoteSession: (session: RemoteSessionInfo | null) => void;
  setRemoteConnecting: (next: boolean) => void;
  setRemoteError: (next: string | null) => void;
  disconnect: () => void;
  reconnectNow: () => void;
  suspend: () => void;
  resume: () => void;
  dispose: () => void;
  setActive: (active: boolean) => void;
  setCurrentDir: (dir: string) => void;
  setRequestRootAction: (action: 'new-file' | 'new-folder' | null) => void;
  setSidebarSearchQuery: (value: string) => void;
  setSidebarSearchIncludes: (value: string) => void;
  setSidebarSearchExcludes: (value: string) => void;
  setSidebarSearchFlag: (
    flag: keyof SidebarSearchFlags,
    value: boolean,
  ) => void;
  clearSidebarSearchState: () => void;
  runSidebarSearch: (options?: { installRetry?: boolean }) => Promise<void>;
  refreshGitInfo: (path?: string) => Promise<void>;
  refreshStorageStatus: (path?: string) => Promise<WorkspaceStorageInfo | null>;
  updateWorkspaceSyncPolicy: (policy: WorkspaceSyncPolicy) => Promise<WorkspaceStorageInfo | null>;
  syncWorkspaceNow: (options?: {
    reason?: 'manual' | 'local-change';
  }) => Promise<WorkspaceStorageInfo | null>;
  listCronTasks: () => Promise<CronTaskRecord[]>;
  getCronTask: (id: string) => Promise<CronTaskRecord | null>;
  updateCronTask: (task: CronTask) => Promise<CronTaskRecord>;
  runCronTask: (id: string) => Promise<CronRunResult>;
  listCronTaskHistory: (id: string, limit?: number) => Promise<CronTaskHistoryEntry[]>;
  listThreadReviews: (threadID: string) => Promise<ThreadReviewState[]>;
  resolveThreadReview: (params: {
    chatPath?: string;
    threadID?: string;
    turnID: string;
    decision: 'approve' | 'reject' | 'approveAll' | 'rejectAll';
    path?: string;
  }) => Promise<ThreadReviewState | null>;
  rollbackThreadReview: (params: {
    chatPath?: string;
    threadID?: string;
    turnID: string;
    scope: 'file' | 'turn';
    path?: string;
  }) => Promise<ThreadReviewState | null>;
  execCommand: (params: {
    command: string;
    workspaceRoot: string;
    targetPath?: string | null;
  }) => Promise<{
    commandID: string;
    filePath: string;
    workspaceRoot: string;
    created: boolean;
  }>;
  stopCommand: (commandID: string) => Promise<void>;
  ensureDirectory: (dir: string) => Promise<void>;
  loadDirectory: (dir: string) => Promise<void>;
  fetchDirAgentsInfo: (dir: string) => Promise<void>;
  readDirectory: (dir: string) => Promise<FileEntry[]>;
  listDirectory: (dir: string) => Promise<ReaddirResult>;
  statPath: (path: string) => Promise<StatResult & { error?: string }>;
  toggleDir: (dir: string) => void;
  openFile: (
    path: string,
    options?: {
      heading?: string;
      reveal?: EditorRevealTarget;
      bookTarget?: BookOpenTarget;
      reviewOverlay?: EditorReviewOverlay | null;
      focusEditor?: boolean;
    },
  ) => Promise<void>;
  openWelcomeTab: () => void;
  openModelsTab: () => void;
  openOpenBrainSettingsTab: () => void;
  openDesktopSettingsTab: () => void;
  openDashboardTab: () => void;
  openCronTaskTab: (id: string, title?: string) => void;
  openMarketplaceTab: () => void;
  openOrgMarketplaceTab: (orgID: string, orgName: string) => void;
  listMarketplaceItems: (options?: {
    force?: boolean;
    orgID?: string | null;
  }) => Promise<import('../types/electron').MarketplaceListResult>;
  refreshMarketplaceItems: (options?: {
    orgID?: string | null;
  }) => Promise<import('../types/electron').MarketplaceListResult>;
  listMarketplaceOrgs: () => Promise<
    import('../types/electron').MarketplaceOrgListResult
  >;
  getMarketplaceState: () => Promise<{
    state: import('../types/electron').MarketplaceStateFile;
    catalogVersion: string | null;
    generatedAt: string | null;
  }>;
  installMarketplaceItem: (
    kind: 'agent' | 'skill' | 'tool',
    id: string,
    orgID?: string | null,
  ) => Promise<import('../types/electron').MarketplaceActionResult>;
  updateMarketplaceItem: (
    kind: 'agent' | 'skill' | 'tool',
    id: string,
    orgID?: string | null,
  ) => Promise<import('../types/electron').MarketplaceActionResult>;
  openUntitledTab: () => void;
  openThreadTab: (
    path: string,
    title?: string,
    content?: string,
  ) => Promise<string | null>;
  ensureThreadTab: (
    path: string,
    title?: string,
    content?: string,
  ) => string | null;
  restoreChatTabsSession: (
    entries: WorkspaceChatTabSession[],
    selectedPath?: string | null,
  ) => Promise<RestoreChatTabsResult>;
  moveChatTabToLast: (path: string) => void;
  appendToTab: (path: string, text: string) => void;
  retargetActiveBlankTab: (
    newPath: string,
    title?: string,
    content?: string,
  ) => boolean;
  retargetTabPath: (oldPath: string, newPath: string, title?: string) => void;
  retargetMovedTabs: (ops: Array<{ oldPath: string; newPath: string }>) => void;
  setActiveTab: (tabId: string) => void;
  setActiveConversationTab: (tabId: string) => void;
  activateConversationPath: (path: string) => void;
  activateLastPrimaryTab: (options?: {
    excludeTabId?: string | null;
  }) => boolean;
  activateLastNonConversationTab: () => boolean;
  setPinnedTab: (tabId: string | null) => void;
  togglePinnedTab: (tabId: string) => void;
  clearPinnedTab: () => void;
  closeTab: (tabId: string) => void;
  dismissPendingDirtyTabClose: () => void;
  confirmPendingDirtyTabClose: () => void;
  closeOtherTabs: (tabId: string) => void;
  closeAllTabs: () => void;
  saveFile: () => Promise<void>;
  saveTabByPath: (path: string) => Promise<boolean>;
  flushDirtyTabs: () => Promise<{ saved: number; failed: string[] }>;
  setFileContent: (content: string) => void;
  setTabContent: (tabId: string, content: string) => void;
  setPendingScrollHeading: (heading: string | null) => void;
  setPendingRevealTarget: (reveal: EditorRevealTarget | null) => void;
  setPendingBookTarget: (target: BookOpenTarget | null) => void;
  setCurrentReviewOverlay: (overlay: EditorReviewOverlay | null) => void;
  setEditorFocused: (focused: boolean) => void;
  consumeEditorFocusRequest: (tabId: string) => boolean;
  requestEditorCompletion: (
    request: EditorCompletionRequest,
  ) => Promise<EditorCompletionResult | null>;
  cancelEditorCompletion: (requestID: string) => Promise<void>;
  requestEditorRandomID: () => Promise<string | null>;
  requestEditorBlur: () => void;
  refreshMessenger: () => Promise<void>;
  loadMessengerChannel: (channelID: string) => Promise<void>;
  replyMessenger: (input: MessengerReplyInput) => Promise<void>;
  markMessengerRead: (channelIDs?: string[]) => Promise<void>;
  archiveMessengerAgentPendingRequests: (agentID: string) => Promise<number>;
  archiveMessengerAgentMessages: (agentID: string) => Promise<number>;
  archiveMessengerChannel: (channelID: string) => Promise<void>;
  createFile: (path: string) => Promise<{ success: boolean; error?: string }>;
  createFolder: (path: string) => Promise<{ success: boolean; error?: string }>;
  deleteEntry: (
    path: string,
    isDir: boolean,
    options?: { useTrash?: boolean; recursive?: boolean },
  ) => Promise<{ success: boolean; error?: string }>;
  moveEntries: (
    ops: Array<{ oldPath: string; newPath: string }>,
  ) => Promise<{ success: boolean; error?: string }>;
  copyEntries: (
    ops: Array<{ sourcePath: string; targetPath: string }>,
  ) => Promise<{ success: boolean; error?: string }>;
  renameEntry: (
    oldPath: string,
    newPath: string,
  ) => Promise<{ success: boolean; error?: string }>;
  persistPastedImage: (
    base64: string,
    documentPath?: string | null,
  ) => Promise<{ markdown?: string; documentRef?: string; error?: string }>;

  // Agent helpers
  readTextFile: (path: string) => Promise<string | null>;
  writeTextFile: (path: string, content: string) => Promise<boolean>;
  writeBase64File: (
    path: string,
    base64: string,
    options?: { overwrite?: boolean },
  ) => Promise<{ success: boolean; error?: string }>;
  ensureAgentRecord: (agentID: string) => Promise<OpNode | null>;
  resolveAgentByID: (agentID: string) => {
    id: string;
    name: string | null;
    avatar: string | null;
    model: string | null;
    uri: string | null;
    path: string | null;
  } | null;
  resolveAgentIDByPath: (path: string) => string | null;
  resolveAgentIDByUri: (uri: string) => string | null;
  hasAgentBinding: (cwd: string) => boolean;
  getEffectiveAgentForCwd: (cwd: string) => ChatAgentTarget | null;
  getChatAgentForCwd: (cwd: string) => ChatAgentTarget | null;
  getDefaultOpenBrainForCwd: (cwd: string) => ChatAgentTarget | null;
  getAgentOpCode: (agentID: string) => string | null;
  getAgentSubagents: (agentID: string) => AgentSubagentInfo[];
  getMountableAgentSubagents: (agentID: string) => AgentSubagentInfo[];
  mountAgentSubagent: (agentID: string, subagentID: string) => Promise<boolean>;
  unmountAgentSubagent: (
    agentID: string,
    subagentID: string,
  ) => Promise<boolean>;
  addAgentReference: (targetDir: string, agentID: string) => Promise<void>;
  switchAgentReference: (
    targetDir: string,
    agentID: string,
  ) => Promise<boolean>;
  addCustomAgent: (targetDir: string) => Promise<void>;
  ensureDerivedDirs: (opts?: {
    force?: boolean;
  }) => Promise<DerivedDirs | null>;
  refreshAgentNodes: (opts?: { force?: boolean }) => Promise<void>;
  invalidateAgentScanCache: () => void;

  // Sidebar helpers
  refreshVisibleWorkspaceTree: () => Promise<void>;
  revealInSidebar: (path: string) => Promise<void>;

  // Backup helpers
  restoreBackups: () => Promise<void>;
  reloadOpenTabsByPaths: (
    paths: string[],
    options?: { skipDirty?: boolean },
  ) => Promise<void>;
  reloadOpenTabsFromDisk: () => Promise<void>;
  reloadVisibleWorkspaceAfterGitChange: () => Promise<void>;
  setStreamingChatPath: (path: string | null) => void;
  getStreamingChatPath: () => string | null;
  checkoutGitBranch: (
    branch: string,
    options?: { create?: boolean },
  ) => Promise<{
    success: boolean;
    error?: string;
    currentBranch?: string;
  }>;
  pushConfigSyncFiles: (
    files: Array<{ name: string; content: string }>,
  ) => Promise<void>;
}

type WorkspaceStore = StoreApi<AppState>;
type ConfigSyncPushPayload = {
  files: Array<{ name: string; content: string }>;
};

type AppStoreHook = {
  <T = AppState>(selector?: (state: AppState) => T): T;
  getState: () => AppState;
  getStateByTabId: (tabId: string) => AppState;
  getStoreByTabId: (tabId: string) => WorkspaceStore;
};

// UI copy should follow the active workspace transition instead of a stale socket
// state left over from the previously connected workspace.
export function getDisplayConnectionState(
  state: Pick<AppState, 'connectionState' | 'remoteConnecting'>,
): ConnectionState {
  return state.remoteConnecting ? 'connecting' : state.connectionState;
}

export function getConnectionStateText(state: ConnectionState): string {
  const key =
    state === 'connected'
      ? 'shell:connection.connected'
      : state === 'connecting'
        ? 'shell:connection.connecting'
        : state === 'reconnecting'
          ? 'shell:connection.reconnecting'
          : 'shell:connection.disconnected';
  return rendererI18n.t(key);
}

const workspaceStores = new Map<string, WorkspaceStore>();
const systemConfigByInstanceID = new Map<string, SystemConfigResult>();
const instanceIDByServerUrl = new Map<string, string>();
const systemConfigInflightByServerUrl = new Map<
  string,
  Promise<DerivedDirs | null>
>();
let configSyncListenerBound = false;

function normalizeDirPath(input: string): string {
  const value = input.trim();
  if (!value) return '';
  if (value === '/') return '/';
  return value.replace(/\/+$/, '');
}

function joinBaseDir(baseDir: string, leaf: string): string {
  const root = normalizeDirPath(baseDir);
  const name = leaf.trim().replace(/^\/+/, '');
  if (!root) {
    return `/${name}`;
  }
  if (root === '/') {
    return `/${name}`;
  }
  return `${root}/${name}`;
}

function deriveDirsFromConfig(
  cfg: SystemConfigResult | null | undefined,
): DerivedDirs | null {
  const baseDirRaw = typeof cfg?.baseDir === 'string' ? cfg.baseDir : '';
  const defaultWorkspaceRaw =
    typeof cfg?.defaultWorkspace === 'string' ? cfg.defaultWorkspace : '';
  const hostIDRaw =
    typeof cfg?.hostID === 'string'
      ? cfg.hostID
      : typeof cfg?.instanceID === 'string'
        ? cfg.instanceID
        : '';
  const baseDir = normalizeDirPath(baseDirRaw);
  const defaultWorkspace = normalizeDirPath(defaultWorkspaceRaw);
  const instanceID = hostIDRaw.trim();
  if (!baseDir || !defaultWorkspace || !instanceID) {
    return null;
  }
  return {
    baseDir,
    workspaceDir: defaultWorkspace,
    agentsDir: joinBaseDir(baseDir, 'agents'),
    instanceID,
  };
}

function buildRemoteSessionIdentity(
  session: RemoteSessionInfo | null | undefined,
): string | null {
  if (!session) {
    return null;
  }
  const remoteHome = normalizeDirPath(session.remoteHome || '');
  const workspaceDir = normalizeDirPath(session.workspaceDir || '');
  const installDir = normalizeDirPath(session.installDir || '');
  const hostLabel = (session.hostLabel || '').trim();
  if (!remoteHome || !workspaceDir || !installDir || !hostLabel) {
    return null;
  }
  return [hostLabel, remoteHome, workspaceDir, installDir].join('|');
}

function isPathInsideRoot(
  path: string | null | undefined,
  root: string | null | undefined,
): boolean {
  const target = normalizeDirPath(path || '');
  const base = normalizeDirPath(root || '');
  if (!target || !base) {
    return false;
  }
  return target === base || target.startsWith(`${base}/`);
}

function relativeDirPath(
  fromDir: string | null | undefined,
  toDir: string | null | undefined,
): string {
  const from = normalizeDirPath(fromDir || '');
  const to = normalizeDirPath(toDir || '');
  if (!from || !to || from === to) {
    return '';
  }
  const fromParts = from.split('/').filter(Boolean);
  const toParts = to.split('/').filter(Boolean);
  let common = 0;
  while (
    common < fromParts.length &&
    common < toParts.length &&
    fromParts[common] === toParts[common]
  ) {
    common += 1;
  }
  const up = fromParts.slice(common).map(() => '..');
  const down = toParts.slice(common);
  const rel = [...up, ...down].join('/');
  return rel ? (rel.startsWith('.') ? rel : `./${rel}`) : '';
}

function agentResourceRootFromWorkdir(
  workdir: string | null | undefined,
): string {
  const normalized = normalizeDirPath(workdir || '');
  return normalized ? `${normalized}/.agent` : '';
}

function isPathInVisibleRoots(
  path: string | null | undefined,
  derived: DerivedDirs | null,
): boolean {
  if (!path || !derived) {
    return false;
  }
  return (
    isPathInsideRoot(path, derived.workspaceDir) ||
    isPathInsideRoot(path, derived.agentsDir)
  );
}

function isAbsoluteDirPath(path: string | null | undefined): boolean {
  const normalized = normalizeDirPath(path || '');
  if (!normalized) {
    return false;
  }
  return normalized === '/' || normalized.startsWith('/');
}

function uniqueDirs(input: Array<string | null | undefined>): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const value of input) {
    const dir = normalizeDirPath(value || '');
    if (!dir || seen.has(dir)) {
      continue;
    }
    seen.add(dir);
    result.push(dir);
  }
  return result;
}

function isTransientDirectoryLoadError(message: string): boolean {
  const normalized = message.trim().toLowerCase();
  return normalized.includes('timeout')
    || normalized.includes('not connected')
    || normalized.includes('connection disposed')
    || normalized.includes('connection timed out');
}

function getVisibleWorkspaceDirs(
  state: Pick<AppState, 'currentDir' | 'expandedDirs'>,
): string[] {
  const currentDir = normalizeDirPath(state.currentDir || '');
  if (!currentDir) {
    return [];
  }
  return uniqueDirs([currentDir, ...Array.from(state.expandedDirs)]).filter(
    (dir) => isPathInsideRoot(dir, currentDir),
  );
}

function parseSidebarSearchGlobs(input: string): string[] {
  return input
    .split(/[\n,]+/)
    .map((value) => value.trim())
    .filter(Boolean);
}

function bindConfigSyncPushListenerOnce() {
  if (configSyncListenerBound) {
    return;
  }
  if (!window.electronAPI?.onConfigSyncPush) {
    return;
  }
  configSyncListenerBound = true;
  window.electronAPI.onConfigSyncPush((payload: ConfigSyncPushPayload) => {
    const files = Array.isArray(payload?.files) ? payload.files : [];
    if (!files.length) {
      return;
    }
    for (const store of workspaceStores.values()) {
      void store.getState().pushConfigSyncFiles(files);
    }
  });
}

function createWorkspaceStore(_tabId: string): WorkspaceStore {
  const connection = new WSConnection();
  const fileService = createFileService(connection);
  const gitService = createGitService(connection);
  const reviewService = createReviewService(connection);
  const agentService = createAgentService(connection);
  const storageService = createStorageService(connection);
  const cronService = createCronService(connection);
  const messengerService = createMessengerService(connection);
  const marketplaceService = createMarketplaceService(connection, () => {
    const state = getState?.();
    if (!state) {
      return { agents: [], skills: [], tools: [] };
    }
    const currentDir = (state.currentDir || '').trim();
    const boundAgentID = currentDir
      ? state.agentBindingByCwd.get(currentDir)?.effectiveAgentID || null
      : null;
    const chatState = getChatWorkspaceStore(_tabId).getState();
    const effectiveAgentID =
      (chatState.agentID || '').trim() || boundAgentID || null;
    return buildMarketplaceUsageReport({
      remote: Boolean(state.remoteSession),
      baseDir: state.baseDir || null,
      agentsRootDir: state.agentsRootDir || null,
      effectiveAgentID,
      selectedSkillID: chatState.selectedSkill?.id || null,
      nodes: Array.from(state.nodesByID.values()),
    });
  });
  const FILE_CHANGE_DEBOUNCE_MS = 300;
  const WORKSPACE_SYNC_CHANGE_DEBOUNCE_MS = 30_000;
  const AUTO_SAVE_DELAY_MS = 1000;
  const BACKUP_DELAY_MS = 1000;
  const SEARCH_BINARY_MISSING_TEXT = 'ripgrep binary not found';
  const pendingDirs = new Set<string>();
  const autoSaveTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const backupTimers = new Map<string, ReturnType<typeof setTimeout>>();
  let fileChangeTimer: ReturnType<typeof setTimeout> | null = null;
  let workspaceSyncWatchPath = '';
  let workspaceSyncWatchId = '';
  let workspaceSyncChangeTimer: ReturnType<typeof setTimeout> | null = null;
  let getState: (() => AppState) | null = null;
  let setState: StoreApi<AppState>['setState'] | null = null;
  let pendingEnsureDir: string | null = null;
  const AGENT_RECORD_TTL_MS = 10 * 60 * 1000;
  const AGENT_RECORD_NEGATIVE_TTL_MS = 5000;
  /** TTL for agent/scan (per-dir) cache; invalidated on Folder tab click via invalidateAgentScanCache */
  const AGENT_SCAN_TTL_MS = 1000;
  const agentRecordCache = new Map<
    string,
    { data: OpNode | null; expireAt: number }
  >();
  const agentRecordInflight = new Map<string, Promise<OpNode | null>>();
  const agentScanCache = new Map<
    string,
    { data: OpNode[]; expireAt: number }
  >();
  const agentScanInflight = new Map<string, Promise<OpNode[]>>();
  let agentScanRevision = 0;
  let nodesRefreshInflight: Promise<void> | null = null;
  let visibleTreeSnapshot: VisibleTreeSnapshot | null = null;
  let gitInfoRequestSeq = 0;
  let storageStatusRequestSeq = 0;
  let storageSyncInFlight: Promise<WorkspaceStorageInfo | null> | null = null;

  let streamingChatPath = '';

  const nodeKind = (node: OpNode | null | undefined): string | null => {
    const kind = (node?.kind || '').trim().toLowerCase();
    return kind || null;
  };

  const nodeURI = (node: OpNode | null | undefined): string | null => {
    const uri = (node?.uri || '').trim();
    return uri || null;
  };

  const uriToPath = (uri: string): string | null => {
    if (!uri.startsWith('file://')) return null;
    const raw = uri.slice('file://'.length);
    if (!raw) return null;
    try {
      return decodeURIComponent(raw);
    } catch {
      return raw;
    }
  };

  const workdirFromURI = (uri: string): string | null => {
    if (!uri) return null;
    const path = uriToPath(uri);
    if (!path) return null;
    return workdirFromAgentConfigPath(path);
  };

  const workdirFromAgentConfigPath = (path: string): string | null => {
    if (!path) return null;
    if (path.endsWith('/.agent/AGENT.md')) {
      return path.slice(0, -'/.agent/AGENT.md'.length);
    }
    if (path.endsWith('/.agents/AGENTS.md')) {
      return path.slice(0, -'/.agents/AGENTS.md'.length);
    }
    return null;
  };

  const workdirFromNode = (node: OpNode | null | undefined): string | null => {
    const cwd = (node?.cwd || '').trim();
    if (cwd) return cwd;
    const uri = nodeURI(node);
    if (!uri) return null;
    return workdirFromURI(uri);
  };

  const nodeMeta = (node: OpNode | null | undefined): Record<string, any> =>
    (node?.meta as Record<string, any> | undefined) || {};

  const nodeID = (node: OpNode | null | undefined): string =>
    (node?.id || '').trim();

  const boundIDFromMeta = (meta: Record<string, any>): string => {
    const raw = typeof meta.bind === 'string' ? meta.bind : '';
    return normalizeAgentNodeID(raw);
  };

  const stringArrayFromMeta = (value: unknown): string[] => {
    if (!Array.isArray(value)) {
      return [];
    }
    return value
      .map((item) => (typeof item === 'string' ? item.trim() : ''))
      .filter(Boolean);
  };

  const subagentIDsFromMeta = (meta: Record<string, any>): string[] => {
    const direct = stringArrayFromMeta(meta.subAgents);
    if (direct.length > 0) {
      return direct;
    }
    const legacy = stringArrayFromMeta(meta.SubAgents);
    if (legacy.length > 0) {
      return legacy;
    }
    return stringArrayFromMeta(meta.subagents);
  };

  const agentInfoFromNode = (
    id: string,
    node: OpNode | null | undefined,
  ): AgentSubagentInfo => {
    const meta = nodeMeta(node);
    const name =
      typeof meta.name === 'string' ? meta.name.trim() || null : null;
    return {
      id,
      name,
      uri: nodeURI(node),
      path: agentConfigWorkdirFromNode(node),
    };
  };

  const isBindAgentNode = (node: OpNode | null | undefined): boolean => {
    const bind =
      typeof nodeMeta(node).bind === 'string' ? nodeMeta(node).bind.trim() : '';
    return Boolean(bind);
  };

  const isMountableAgentUnderRoot = (
    node: OpNode,
    root: string | null | undefined,
  ): boolean => {
    const path = normalizeDirPath(agentConfigWorkdirFromNode(node) || '');
    const normalizedRoot = normalizeDirPath(root || '');
    if (!path || !normalizedRoot || !isPathInsideRoot(path, normalizedRoot)) {
      return false;
    }
    const rel =
      path === normalizedRoot ? '' : path.slice(normalizedRoot.length + 1);
    const parts = rel.split('/').filter(Boolean);
    if (parts.length === 1) {
      return true;
    }
    return parts.length === 2 && parts[0].startsWith('@org-');
  };

  const isLocalSubagentOfParent = (
    node: OpNode | null | undefined,
    parentWorkdir: string | null | undefined,
  ): boolean => {
    const path = normalizeDirPath(agentConfigWorkdirFromNode(node) || '');
    const parent = normalizeDirPath(parentWorkdir || '');
    if (!path || !parent) {
      return false;
    }
    return isPathInsideRoot(path, `${parent}/.agent/subagents`);
  };

  const inferAgentsRootFromAgentPath = (
    path: string | null | undefined,
  ): string | null => {
    const normalized = normalizeDirPath(path || '');
    if (!normalized) {
      return null;
    }
    const parts = normalized.split('/').filter(Boolean);
    const agentIndex = parts.lastIndexOf('agents');
    if (agentIndex < 0 || agentIndex >= parts.length - 1) {
      return null;
    }
    const rootParts = parts.slice(0, agentIndex + 1);
    return `${normalized.startsWith('/') ? '/' : ''}${rootParts.join('/')}`;
  };

  const resolveAgentsRootForSubagentCandidates = (
    state: AppState,
    parentNode: OpNode,
  ): string | null => {
    const configuredRoot = normalizeDirPath(state.agentsRootDir || '');
    if (configuredRoot) {
      return configuredRoot;
    }
    const baseDir = normalizeDirPath(state.baseDir || '');
    if (baseDir) {
      return joinBaseDir(baseDir, 'agents');
    }
    return inferAgentsRootFromAgentPath(agentConfigWorkdirFromNode(parentNode));
  };

  const unquoteYamlScalar = (value: string): string => {
    let raw = (value || '').trim();
    if (!raw) {
      return '';
    }
    if (!(raw.startsWith('"') || raw.startsWith("'"))) {
      raw = raw.replace(/\s+#.*$/, '').trim();
    }
    if (raw.length >= 2) {
      const first = raw[0];
      const last = raw[raw.length - 1];
      if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
        return raw.slice(1, -1).trim();
      }
    }
    return raw;
  };

  const quoteYamlString = (value: string): string => {
    const escaped = value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
    return `"${escaped}"`;
  };

  const leadingSpaces = (line: string): number => {
    const match = line.match(/^ */);
    return match ? match[0].length : 0;
  };

  const agentConfigPathFromNode = (
    node: OpNode | null | undefined,
  ): string | null => {
    const uri = nodeURI(node);
    return uri ? uriToPath(uri) : null;
  };

  const agentConfigWorkdirFromNode = (
    node: OpNode | null | undefined,
  ): string | null => {
    const configPath = agentConfigPathFromNode(node);
    if (configPath) {
      const configWorkdir = workdirFromAgentConfigPath(configPath);
      if (configWorkdir) {
        return configWorkdir;
      }
    }
    return workdirFromNode(node);
  };

  const agentNodeMatchesDir = (node: OpNode, dir: string): boolean => {
    const normalizedDir = normalizePosixPath(dir);
    if (!normalizedDir) {
      return false;
    }
    const configWorkdir = agentConfigWorkdirFromNode(node);
    if (configWorkdir && normalizePosixPath(configWorkdir) === normalizedDir) {
      return true;
    }
    const cwd = workdirFromNode(node);
    return Boolean(cwd && normalizePosixPath(cwd) === normalizedDir);
  };

  const agentNodeLookupID = (agentID: string, state: AppState): string => {
    const raw = (agentID || '').trim();
    const normalized = normalizeAgentNodeID(raw);
    if (normalized && state.nodesByID.has(normalized)) {
      return normalized;
    }
    if (raw && state.nodesByID.has(raw)) {
      return raw;
    }
    return normalized;
  };

  const resolveSubagentEntryToID = (
    entry: string,
    parentWorkdir: string,
    state: AppState,
  ): string => {
    const ref = unquoteYamlScalar(entry);
    if (!ref) {
      return '';
    }
    const direct = normalizeAgentNodeID(ref);
    if (direct) {
      return direct;
    }

    let targetDir = '';
    if (ref.startsWith('@agents/')) {
      const baseDir = normalizePosixPath(state.baseDir || '');
      const suffix = ref.slice('@agents/'.length).replace(/^\/+/, '');
      if (baseDir && suffix) {
        targetDir = normalizePosixPath(`${baseDir}/agents/${suffix}`);
      }
    } else if (ref.startsWith('/')) {
      targetDir = normalizePosixPath(workdirFromAgentConfigPath(ref) || ref);
    } else if (ref.startsWith('.')) {
      const resourceRoot = agentResourceRootFromWorkdir(parentWorkdir);
      const resolvedPath = normalizePosixPath(
        `${resourceRoot || parentWorkdir}/${ref}`,
      );
      targetDir = normalizePosixPath(
        workdirFromAgentConfigPath(resolvedPath) || resolvedPath,
      );
    }
    if (!targetDir) {
      return '';
    }

    for (const node of state.nodesByID.values()) {
      if (nodeKind(node) !== 'agent') {
        continue;
      }
      if (agentNodeMatchesDir(node, targetDir)) {
        return nodeID(node);
      }
    }
    return '';
  };

  const removeSubagentFromAgentMarkdown = (
    markdown: string,
    parentWorkdir: string,
    targetSubagentID: string,
    state: AppState,
  ): { content: string; removed: boolean } => {
    const newline = markdown.includes('\r\n') ? '\r\n' : '\n';
    const text = markdown.replace(/\r\n/g, '\n');
    const lines = text.split('\n');
    if (lines.length < 3 || lines[0].trim() !== '---') {
      return { content: markdown, removed: false };
    }

    let closeIndex = -1;
    for (let i = 1; i < lines.length; i += 1) {
      const trimmed = lines[i].trim();
      if (trimmed === '---' || trimmed === '...') {
        closeIndex = i;
        break;
      }
    }
    if (closeIndex < 0) {
      return { content: markdown, removed: false };
    }

    const normalizedTarget = normalizeAgentNodeID(targetSubagentID);
    if (!normalizedTarget) {
      return { content: markdown, removed: false };
    }

    const nextLines = [...lines];
    for (let i = 1; i < closeIndex; i += 1) {
      const match = lines[i].match(/^(\s*)subagents\s*:\s*(.*)$/i);
      if (!match) {
        continue;
      }
      const keyIndent = match[1].length;
      const inlineValue = match[2].trim();
      if (inlineValue) {
        const resolved = resolveSubagentEntryToID(
          inlineValue,
          parentWorkdir,
          state,
        );
        if (resolved === normalizedTarget) {
          nextLines.splice(i, 1);
          return {
            content: nextLines.join('\n').replace(/\n/g, newline),
            removed: true,
          };
        }
        return { content: markdown, removed: false };
      }

      let blockEnd = i + 1;
      const listItemIndexes: number[] = [];
      while (blockEnd < closeIndex) {
        const line = lines[blockEnd];
        const trimmed = line.trim();
        if (!trimmed) {
          blockEnd += 1;
          continue;
        }
        if (leadingSpaces(line) <= keyIndent) {
          break;
        }
        if (/^\s*-\s+/.test(line)) {
          listItemIndexes.push(blockEnd);
        }
        blockEnd += 1;
      }

      const removeIndexes = new Set<number>();
      for (const itemIndex of listItemIndexes) {
        const itemMatch = lines[itemIndex].match(/^\s*-\s*(.*)$/);
        const rawEntry = itemMatch ? itemMatch[1] : '';
        const resolved = resolveSubagentEntryToID(
          rawEntry,
          parentWorkdir,
          state,
        );
        if (resolved === normalizedTarget) {
          removeIndexes.add(itemIndex);
        }
      }
      if (removeIndexes.size === 0) {
        return { content: markdown, removed: false };
      }

      const remainingItemCount = listItemIndexes.filter(
        (index) => !removeIndexes.has(index),
      ).length;
      if (remainingItemCount === 0) {
        nextLines.splice(i, blockEnd - i);
      } else {
        for (const index of Array.from(removeIndexes).sort((a, b) => b - a)) {
          nextLines.splice(index, 1);
        }
      }
      return {
        content: nextLines.join('\n').replace(/\n/g, newline),
        removed: true,
      };
    }

    return { content: markdown, removed: false };
  };

  const addSubagentToAgentMarkdown = (
    markdown: string,
    parentWorkdir: string,
    targetSubagentID: string,
    state: AppState,
  ): { content: string; added: boolean } => {
    const newline = markdown.includes('\r\n') ? '\r\n' : '\n';
    const text = markdown.replace(/\r\n/g, '\n');
    const lines = text.split('\n');
    if (lines.length < 3 || lines[0].trim() !== '---') {
      return { content: markdown, added: false };
    }

    let closeIndex = -1;
    for (let i = 1; i < lines.length; i += 1) {
      const trimmed = lines[i].trim();
      if (trimmed === '---' || trimmed === '...') {
        closeIndex = i;
        break;
      }
    }
    if (closeIndex < 0) {
      return { content: markdown, added: false };
    }

    const normalizedTarget = normalizeAgentNodeID(targetSubagentID);
    if (!normalizedTarget) {
      return { content: markdown, added: false };
    }
    const targetNode = state.nodesByID.get(normalizedTarget) || null;
    const targetWorkdir = agentConfigWorkdirFromNode(targetNode);
    const resourceRoot = agentResourceRootFromWorkdir(parentWorkdir);
    const localRef = isLocalSubagentOfParent(targetNode, parentWorkdir)
      ? relativeDirPath(resourceRoot, targetWorkdir)
      : '';
    const ref = localRef || quoteYamlString(`@${normalizedTarget}`);

    const nextLines = [...lines];
    for (let i = 1; i < closeIndex; i += 1) {
      const match = lines[i].match(/^(\s*)subagents\s*:\s*(.*)$/i);
      if (!match) {
        continue;
      }
      const keyIndentText = match[1];
      const keyIndent = keyIndentText.length;
      const inlineValue = match[2].trim();
      if (inlineValue) {
        if (inlineValue.startsWith('[') || inlineValue.startsWith('{')) {
          return { content: markdown, added: false };
        }
        const resolved = resolveSubagentEntryToID(
          inlineValue,
          parentWorkdir,
          state,
        );
        if (resolved === normalizedTarget) {
          return { content: markdown, added: false };
        }
        const itemIndent = `${keyIndentText}  `;
        nextLines.splice(
          i,
          1,
          `${keyIndentText}subagents:`,
          `${itemIndent}- ${inlineValue}`,
          `${itemIndent}- ${ref}`,
        );
        return {
          content: nextLines.join('\n').replace(/\n/g, newline),
          added: true,
        };
      }

      let blockEnd = i + 1;
      const listItemIndexes: number[] = [];
      while (blockEnd < closeIndex) {
        const line = lines[blockEnd];
        const trimmed = line.trim();
        if (!trimmed) {
          blockEnd += 1;
          continue;
        }
        if (leadingSpaces(line) <= keyIndent) {
          break;
        }
        if (/^\s*-\s+/.test(line)) {
          listItemIndexes.push(blockEnd);
        }
        blockEnd += 1;
      }

      let itemIndent = `${keyIndentText}  `;
      for (const itemIndex of listItemIndexes) {
        const itemMatch = lines[itemIndex].match(/^(\s*)-\s*(.*)$/);
        if (!itemMatch) {
          continue;
        }
        itemIndent = itemMatch[1];
        const resolved = resolveSubagentEntryToID(
          itemMatch[2],
          parentWorkdir,
          state,
        );
        if (resolved === normalizedTarget) {
          return { content: markdown, added: false };
        }
      }

      nextLines.splice(blockEnd, 0, `${itemIndent}- ${ref}`);
      return {
        content: nextLines.join('\n').replace(/\n/g, newline),
        added: true,
      };
    }

    nextLines.splice(closeIndex, 0, 'subagents:', `  - ${ref}`);
    return {
      content: nextLines.join('\n').replace(/\n/g, newline),
      added: true,
    };
  };

  const resolveMutableAgentNode = (
    agentID: string,
    state: AppState,
  ): OpNode | null => {
    const key = agentNodeLookupID(agentID, state);
    if (!key) {
      return null;
    }
    const node = state.nodesByID.get(key) || null;
    const boundID = boundIDFromMeta(nodeMeta(node));
    if (boundID && boundID !== key) {
      return state.nodesByID.get(boundID) || node;
    }
    return node;
  };

  const clearAgentRecordCache = () => {
    agentRecordCache.clear();
    agentRecordInflight.clear();
  };

  const clearAgentObjectIndex = () => {
    agentScanRevision += 1;
    agentScanCache.clear();
    agentScanInflight.clear();
  };

  const clearVisibleTreeSnapshot = () => {
    visibleTreeSnapshot = null;
  };

  const writeAgentReferenceFile = async (
    dir: string,
    agentID: string,
  ): Promise<boolean> => {
    const nodeID = normalizeAgentNodeID(agentID);
    if (!nodeID) {
      return false;
    }
    const agentDir = `${dir}/.agent`;
    const chatDir = `${agentDir}/chat`;
    const agentMd = `${agentDir}/AGENT.md`;

    const ensureAgentDir = await fileService.mkdir(agentDir, true);
    if (ensureAgentDir.error) {
      return false;
    }
    const ensureChatDir = await fileService.mkdir(chatDir, true);
    if (ensureChatDir.error) {
      return false;
    }
    const writeResult = await fileService.writeFile(
      agentMd,
      buildReferenceAgentMarkdown(nodeID),
      {
        create: true,
        overwrite: true,
        atomic: true,
      },
    );
    return !writeResult.error;
  };

  const captureVisibleTreeSnapshot = (state: AppState) => {
    const remoteIdentity = buildRemoteSessionIdentity(state.remoteSession);
    if (!remoteIdentity) {
      visibleTreeSnapshot = null;
      return;
    }
    visibleTreeSnapshot = {
      remoteIdentity,
      currentDir: normalizeDirPath(state.currentDir || '') || null,
      expandedDirs: uniqueDirs(Array.from(state.expandedDirs)),
    };
  };

  const toPersistableNode = (
    node: OpNode,
    hostID: string,
  ): Record<string, unknown> => ({
    id: (node.id || '').trim(),
    hostID,
    uid: node.uid,
    kind: node.kind,
    uri: node.uri,
    cwd: node.cwd,
    tags: node.tags,
    opCodes: node.opCodes,
    run: node.run,
    meta: node.meta,
  });

  const persistNodesAndCacheAvatars = async (
    nodes: OpNode[],
  ): Promise<void> => {
    const electronAPI = window.electronAPI;
    if (!electronAPI?.nodes?.upsert || !electronAPI?.avatar?.cacheNode) {
      return;
    }
    const hostID = (getState?.().instanceID || '').trim();
    if (!hostID) {
      return;
    }
    const persistableNodes = nodes.filter(
      (node) => node && (node.id || '').trim(),
    );
    if (persistableNodes.length === 0) {
      return;
    }

    try {
      await electronAPI.nodes.upsert(
        hostID,
        persistableNodes.map((node) => toPersistableNode(node, hostID)),
      );
    } catch (err) {
      console.warn(
        '[appStore] nodes.upsert failed:',
        (err as Error)?.message || err,
      );
      return;
    }

    const agentNodes = persistableNodes.filter(
      (node) => nodeKind(node) === 'agent',
    );
    if (agentNodes.length === 0) {
      return;
    }

    await Promise.allSettled(
      agentNodes.map((node) =>
        electronAPI.avatar.cacheNode(hostID, toPersistableNode(node, hostID)),
      ),
    );
  };

  const getCachedDerivedDirsByServerUrl = (
    serverUrl: string,
  ): DerivedDirs | null => {
    const url = (serverUrl || '').trim();
    if (!url) {
      return null;
    }
    const instanceID = instanceIDByServerUrl.get(url);
    if (!instanceID) {
      return null;
    }
    const cachedConfig = systemConfigByInstanceID.get(instanceID);
    return deriveDirsFromConfig(cachedConfig);
  };

  const ensureSystemConfig = async (opts?: {
    force?: boolean;
    attempts?: number;
  }): Promise<DerivedDirs | null> => {
    const snapshot = getState?.();
    if (!snapshot || snapshot.connectionState !== 'connected') {
      return null;
    }
    const serverUrl = (snapshot.serverUrl || '').trim();
    if (!serverUrl) {
      return null;
    }
    const force = opts?.force === true;
    const cached = getCachedDerivedDirsByServerUrl(serverUrl);
    if (!force && cached) {
      return cached;
    }
    const pending = systemConfigInflightByServerUrl.get(serverUrl);
    if (pending) {
      return pending;
    }
    const request = agentService
      .getSystemConfig({ attempts: opts?.attempts })
      .then((cfg) => {
        const derived = deriveDirsFromConfig(cfg);
        if (derived) {
          instanceIDByServerUrl.set(serverUrl, derived.instanceID);
          systemConfigByInstanceID.set(
            derived.instanceID,
            cfg as SystemConfigResult,
          );
          return derived;
        }
        instanceIDByServerUrl.delete(serverUrl);
        return null;
      })
      .finally(() => {
        systemConfigInflightByServerUrl.delete(serverUrl);
      });
    systemConfigInflightByServerUrl.set(serverUrl, request);
    return request;
  };

  const invalidateAgentScanCache = () => {
    clearAgentObjectIndex();
  };

  const buildAgentScanKey = (dir: string) => dir;

  const normalizeScopeDir = (dir: string): string => {
    const trimmed = (dir || '').trim();
    if (!trimmed) {
      return '';
    }
    if (trimmed === '/') {
      return '/';
    }
    return trimmed.replace(/\/+$/, '');
  };

  const isWithinScope = (path: string, scopeRoot: string): boolean => {
    const normalizedPath = normalizeScopeDir(path);
    const normalizedRoot = normalizeScopeDir(scopeRoot);
    if (!normalizedPath || !normalizedRoot) {
      return false;
    }
    if (normalizedRoot === '/') {
      return normalizedPath.startsWith('/');
    }
    if (normalizedPath === normalizedRoot) {
      return true;
    }
    return normalizedPath.startsWith(`${normalizedRoot}/`);
  };

  const shouldScanWorkspaceDir = (dir: string, state: AppState): boolean => {
    const root = normalizeScopeDir(
      state.currentDir || state.workspaceRootDir || '',
    );
    return isWithinScope(dir, root);
  };

  const isDirectChildPath = (parent: string, path: string): boolean => {
    const prefix = `${parent}/`;
    if (!path.startsWith(prefix)) {
      return false;
    }
    const rest = path.slice(prefix.length);
    return rest.length > 0 && !rest.includes('/');
  };

  const mergeNodesByID = (
    current: Map<string, OpNode>,
    nodes: OpNode[],
  ): Map<string, OpNode> => {
    const next = new Map(current);
    for (const node of nodes) {
      const id = (node?.id || '').trim();
      if (!id) {
        continue;
      }
      next.set(id, node);
    }
    return next;
  };

  const buildNodesByID = (nodes: OpNode[]): Map<string, OpNode> => {
    const next = new Map<string, OpNode>();
    for (const node of nodes) {
      const id = nodeID(node);
      if (id) {
        next.set(id, node);
      }
    }
    return next;
  };

  const resolveAgentBindingsFromNodes = (nodes: OpNode[]): AgentBinding[] => {
    const agentNodes = nodes.filter((n) => n?.id && nodeKind(n) === 'agent');
    const results: AgentBinding[] = [];
    for (const node of agentNodes) {
      const meta = nodeMeta(node);
      const boundID = boundIDFromMeta(meta);
      const localNodeID = (node.id || '').trim();
      const effectiveAgentID = boundID || localNodeID;
      if (!effectiveAgentID) continue;
      const cwd = workdirFromNode(node);
      if (!cwd) continue;
      results.push({
        cwd,
        localNodeID,
        effectiveAgentID,
        source: boundID ? 'bind' : 'local',
      });
    }
    return results;
  };

  const resolveCurrentAgentNodeID = async (
    agentID: string,
  ): Promise<string> => {
    const requestedID = normalizeAgentNodeID(agentID);
    if (!requestedID) {
      return '';
    }

    const before = getState?.().nodesByID.get(requestedID) || null;
    const beforeURI = nodeURI(before);
    const beforeCwd = workdirFromNode(before);

    await refreshNodesCache(true);

    const state = getState?.();
    if (!state) {
      return '';
    }
    if (state.nodesByID.has(requestedID)) {
      return requestedID;
    }

    const matchingNode = state.agentNodes.find((node) => {
      const id = nodeID(node);
      if (!id) {
        return false;
      }
      if (beforeURI && nodeURI(node) === beforeURI) {
        return true;
      }
      if (beforeCwd && workdirFromNode(node) === beforeCwd) {
        return true;
      }
      return false;
    });
    return nodeID(matchingNode);
  };

  const mergeDirAgentBindings = (
    current: Map<string, AgentBinding>,
    dir: string,
    items: AgentBinding[],
    replaceScope: boolean = true,
  ) => {
    const next = new Map(current);

    if (replaceScope) {
      for (const cwd of Array.from(next.keys())) {
        if (cwd === dir || isDirectChildPath(dir, cwd)) {
          next.delete(cwd);
        }
      }
    }

    for (const item of items) {
      const { cwd, effectiveAgentID } = item;
      if (!cwd || !effectiveAgentID) continue;
      next.set(cwd, item);
    }

    return next;
  };

  const requestDirAgentNodes = async (dir: string): Promise<OpNode[]> => {
    const requestKey = buildAgentScanKey(dir);
    const now = Date.now();

    const cached = agentScanCache.get(requestKey);
    if (cached && cached.expireAt > now) {
      return cached.data;
    }

    const pending = agentScanInflight.get(requestKey);
    if (pending) {
      return pending;
    }

    const requestRevision = agentScanRevision;
    const promise = agentService
      .agentScan(dir)
      .then((nodes) => {
        if (requestRevision === agentScanRevision) {
          agentScanCache.set(requestKey, {
            data: nodes,
            expireAt: Date.now() + AGENT_SCAN_TTL_MS,
          });
          void persistNodesAndCacheAvatars(nodes);
        }
        return nodes;
      })
      .finally(() => {
        if (agentScanInflight.get(requestKey) === promise) {
          agentScanInflight.delete(requestKey);
        }
      });

    agentScanInflight.set(requestKey, promise);
    return promise;
  };

  const refreshNodesCache = async (refresh: boolean = false) => {
    const snapshot = getState?.();
    if (!snapshot || snapshot.connectionState !== 'connected') {
      return;
    }
    if (nodesRefreshInflight) {
      return nodesRefreshInflight;
    }
    const task = (async () => {
      setState?.({ agentNodesLoading: true });
      try {
        const nodes = await agentService.listNodes(
          refresh ? { refresh: true } : undefined,
        );
        void persistNodesAndCacheAvatars(nodes);
        const agentNodes = nodes.filter(
          (n) => n?.id && nodeKind(n) === 'agent',
        );
        const globalNodesByID = buildNodesByID(nodes);
        const globalBindings = resolveAgentBindingsFromNodes(nodes);
        const skillNodes = nodes.filter((n) => nodeKind(n) === 'skill');
        setState?.((state) => {
          const nextNodesByID = new Map(globalNodesByID);
          const nextAgentBindingByCwd = new Map<string, AgentBinding>();
          const currentDir = normalizeDirPath(state.currentDir || '');

          // node/list is the global runtime cache (baseDir agents/skills/tools). Directory
          // agent scans are local to the visible workspace tree, so a late node/list refresh
          // must not erase already-scanned bubbles for the current workspace root.
          if (currentDir) {
            for (const [cwd, binding] of state.agentBindingByCwd) {
              if (!isPathInsideRoot(cwd, currentDir)) {
                continue;
              }
              nextAgentBindingByCwd.set(cwd, binding);
              for (const id of [
                binding.localNodeID,
                binding.effectiveAgentID,
              ]) {
                if (!id || nextNodesByID.has(id)) {
                  continue;
                }
                const existingNode = state.nodesByID.get(id);
                if (existingNode) {
                  nextNodesByID.set(id, existingNode);
                }
              }
            }
          }

          for (const binding of globalBindings) {
            nextAgentBindingByCwd.set(binding.cwd, binding);
          }

          return {
            nodesByID: nextNodesByID,
            nodeGraphRevision: state.nodeGraphRevision + 1,
            agentBindingByCwd: nextAgentBindingByCwd,
            agentNodes,
            skillNodes,
          };
        });
      } finally {
        setState?.({ agentNodesLoading: false });
      }
    })();
    nodesRefreshInflight = task.finally(() => {
      nodesRefreshInflight = null;
    });
    return nodesRefreshInflight;
  };

  const refreshAgentBindingsForDirs = (dirs: Iterable<string>) => {
    const targets = uniqueDirs(Array.from(dirs));
    if (targets.length === 0) {
      return;
    }
    clearAgentRecordCache();
    invalidateAgentScanCache();
    for (const dir of targets) {
      void getState?.().fetchDirAgentsInfo(dir);
    }
  };

  const flushFileChanges = () => {
    fileChangeTimer = null;
    if (pendingDirs.size === 0) {
      return;
    }
    const dirs = Array.from(pendingDirs);
    pendingDirs.clear();
    dirs.forEach((dir) => {
      getState?.().loadDirectory(dir);
    });
  };

  const scheduleFileChangeRefresh = (dirs: Iterable<string>) => {
    for (const dir of dirs) {
      pendingDirs.add(dir);
    }
    if (fileChangeTimer) {
      clearTimeout(fileChangeTimer);
    }
    fileChangeTimer = setTimeout(flushFileChanges, FILE_CHANGE_DEBOUNCE_MS);
  };

  const upsertWorkspaceSyncMessage = (info: WorkspaceStorageInfo) => {
    const error = (info.error || '').trim();
    if (!error) {
      return;
    }
    useMessengerStore.getState().upsertMessage({
      id: `workspace-sync:${info.workspaceID || info.path || 'current'}`,
      severity: 'error',
      source: 'Workspace Sync',
      title: 'Workspace sync failed',
      body: error,
      workspaceID: info.workspaceID,
      workspacePath: info.path,
      action: 'open-sync',
    });
  };

  const isIgnoredWorkspaceSyncPath = (path: string): boolean => {
    const normalized = normalizePosixPath(path);
    if (!normalized) {
      return true;
    }
    const parts = normalized.split('/').filter(Boolean);
    return parts.includes('.git') ||
      parts.includes('node_modules') ||
      parts.includes('.openbrain') ||
      parts.includes('dist') ||
      parts.includes('build') ||
      parts.includes('.cache');
  };

  const isPathInsideDir = (path: string, dir: string): boolean => {
    const normalizedPath = normalizePosixPath(path);
    const normalizedDir = normalizePosixPath(dir).replace(/\/+$/, '');
    return normalizedPath === normalizedDir || normalizedPath.startsWith(`${normalizedDir}/`);
  };

  const scheduleWorkspaceSyncForChanges = (changes: FileChange[]) => {
    const state = getState?.();
    if (!state || state.connectionState !== 'connected') {
      return;
    }
    const currentDir = normalizePosixPath(state.currentDir || '');
    if (!currentDir || !state.storageInfo.storage?.enabled || !state.storageInfo.policy.onLocalChange) {
      return;
    }
    const relevant = changes.some((change) => (
      isPathInsideDir(change.path, currentDir) && !isIgnoredWorkspaceSyncPath(change.path)
    ));
    if (!relevant) {
      return;
    }
    if (workspaceSyncChangeTimer) {
      clearTimeout(workspaceSyncChangeTimer);
    }
    workspaceSyncChangeTimer = setTimeout(() => {
      workspaceSyncChangeTimer = null;
      const latest = getState?.();
      if (!latest || latest.connectionState !== 'connected') {
        return;
      }
      if (!latest.storageInfo.storage?.enabled || !latest.storageInfo.policy.onLocalChange) {
        return;
      }
      void latest.syncWorkspaceNow({ reason: 'local-change' });
    }, WORKSPACE_SYNC_CHANGE_DEBOUNCE_MS);
  };

  const persistBase64ImageAsset = async (input: {
    parentDir: string;
    relativeAssetsDir: string;
    base64: string;
    extension: string;
  }): Promise<{ relativePath?: string; error?: string }> => {
    const parentDir = normalizePosixPath(input.parentDir);
    const relativeAssetsDir = normalizePosixPath(
      input.relativeAssetsDir,
    ).replace(/^\/+/, '');
    if (!parentDir || !relativeAssetsDir) {
      return { error: '无法确定图片保存目录' };
    }

    const assetsDir = normalizePosixPath(`${parentDir}/${relativeAssetsDir}`);
    const extension = normalizeImageExtension(input.extension);

    try {
      const mkdirResult = await fileService.mkdir(assetsDir, true);
      if (mkdirResult.error) {
        console.error(
          'Failed to create image assets directory:',
          mkdirResult.error,
        );
        return { error: '创建图片目录失败' };
      }

      const timestamp = formatImageTimestamp(new Date());
      for (let index = 0; index < 100; index += 1) {
        const fileName =
          index === 0
            ? `image-${timestamp}`
            : `image-${timestamp}-${index + 1}`;
        const targetPath = `${assetsDir}/${fileName}${extension}`;
        const writeResult = await fileService.writeFile(
          targetPath,
          input.base64,
          {
            encoding: 'base64',
            create: true,
            overwrite: false,
            atomic: true,
          },
        );

        if (!writeResult.error) {
          return {
            relativePath: `./${relativeAssetsDir}/${fileName}${extension}`,
          };
        }
        if (!/exist/i.test(writeResult.error)) {
          console.error('Failed to write image asset:', writeResult.error);
          return { error: '写入图片文件失败' };
        }
      }
      return { error: '生成图片文件名失败，请重试' };
    } catch (error) {
      console.error('Error persisting image asset:', error);
      return { error: '写入图片文件失败' };
    }
  };

  return createStore<AppState>((set, get) => {
    getState = get;
    setState = set;

    const buildConnectionCallbacks = (): ConnectionCallbacks => ({
      onStateChange: (state: ConnectionState) => {
        set({ connectionState: state });
      },
      onConnect: () => {
        void (async () => {
          const derived = await ensureSystemConfig({ force: true, attempts: 12 });
          if (derived) {
            set({
              baseDir: derived.baseDir,
              workspaceRootDir: derived.workspaceDir,
              agentsRootDir: derived.agentsDir,
              instanceID: derived.instanceID,
            });
          } else {
            set({
              baseDir: null,
              workspaceRootDir: null,
              agentsRootDir: null,
              instanceID: null,
            });
          }
          invalidateAgentScanCache();
          await restoreVisibleTreeAfterConnect(derived);
          await get().refreshGitInfo();
          await get().refreshStorageStatus();
          await get().refreshMessenger();
        })();
      },
      onDisconnect: () => {
        captureVisibleTreeSnapshot(get());
        streamingChatPath = '';
        void clearWorkspaceSyncWatch();
        set({
          dirWatchIds: new Map(),
          gitInfo: createEmptyGitInfo(),
          storageInfo: createEmptyWorkspaceStorageInfo(),
        });
        const chatState = getChatWorkspaceStore(_tabId).getState();
        chatState.clearAllQueuedMessages();
        chatState.setActiveCommand(null);
        chatState.clearAllInProgress();
        chatState.clearAllAwaitingUsers();
      },
      onCommandState: (event) => {
        const chatState = getChatWorkspaceStore(_tabId).getState();
        if (event.state === 'started') {
          chatState.setActiveCommand({
            commandID: event.commandID,
            filePath: event.filePath,
          });
          chatState.setChatPathInProgress(event.filePath, true);
          return;
        }
        chatState.setActiveCommand(null);
        chatState.setChatPathInProgress(event.filePath, false);
        if (event.state === 'finished') {
          chatState.setErrorForChatPath(event.filePath, null);
          return;
        }
        chatState.setErrorForChatPath(
          event.filePath,
          event.error || event.state.replace(/_/g, ' '),
        );
      },
      onMessengerMessage: (message) => {
        useMessengerStore.getState().upsertRecord(message);
        getChatWorkspaceStore(_tabId).getState().upsertThreadMessageRecords([message]);
      },
      onFileChange: (changes: FileChange[]) => {
        scheduleWorkspaceSyncForChanges(changes);
        const { currentDir, expandedDirs } = get();
        const dirsToReload = new Set<string>();
        const deletedPaths = new Set<string>();
        const existingPaths = new Set<string>();

        for (const change of changes) {
          const dir = change.path.substring(0, change.path.lastIndexOf('/'));
          if (dir === currentDir || expandedDirs.has(dir)) {
            dirsToReload.add(dir);
          }
          if (change.type === 'deleted') {
            deletedPaths.add(change.path);
          } else if (change.type === 'created' || change.type === 'changed') {
            existingPaths.add(change.path);
          }
        }

        const agentBindingDirsToRefresh = new Set<string>();
        for (const change of changes) {
          const agentCwd = workdirFromAgentConfigPath(change.path);
          if (agentCwd) {
            agentBindingDirsToRefresh.add(agentCwd);
          }
        }

        if (deletedPaths.size > 0 || existingPaths.size > 0) {
          set((state) => {
            let tabsChanged = false;
            const nextTabs = getOpenDocuments(state).map((tab) => {
              const tabPath = tab.filePath || '';
              if (!tabPath) return tab;
              if (deletedPaths.has(tabPath) && !tab.missing) {
                tabsChanged = true;
                return { ...tab, missing: true };
              }
              if (existingPaths.has(tabPath) && tab.missing) {
                tabsChanged = true;
                return { ...tab, missing: false };
              }
              return tab;
            });

            if (!tabsChanged) {
              return {};
            }
            return patchDocumentsWithPinnedState(state, nextTabs);
          });
        }

        if (dirsToReload.size > 0) {
          scheduleFileChangeRefresh(dirsToReload);
        }

        const changedOpenPaths = Array.from(existingPaths).filter((path) =>
          Boolean(normalizePosixPath(path)),
        );
        if (changedOpenPaths.length > 0) {
          void reloadOpenTabsByPaths(changedOpenPaths, { skipDirty: true });
        }
        if (agentBindingDirsToRefresh.size > 0) {
          refreshAgentBindingsForDirs(agentBindingDirsToRefresh);
        }
      },
    });

    const discardAutoSave = (path: string | null | undefined) => {
      if (!path) {
        return;
      }
      const handle = autoSaveTimers.get(path);
      if (handle) {
        clearTimeout(handle);
        autoSaveTimers.delete(path);
      }
    };

    const discardBackup = (tabId: string) => {
      const handle = backupTimers.get(tabId);
      if (handle) {
        clearTimeout(handle);
        backupTimers.delete(tabId);
      }
    };

    const closeTabNow = (tabId: string) => {
      const state = get();
      const tab = getOpenDocuments(state).find((item) => item.id === tabId);
      if (!tab) {
        set({ pendingDirtyTabClose: null });
        return;
      }

      discardAutoSave(tab.filePath);
      discardBackup(tab.id);
      if (!tab.filePath && window.electronAPI?.backup) {
        window.electronAPI.backup.delete(tab.id).catch(() => {});
      }

      set((current) => {
        const index = getOpenDocuments(current).findIndex(
          (item) => item.id === tabId,
        );
        if (index === -1) {
          return { pendingDirtyTabClose: null };
        }
        const nextTabs = getOpenDocuments(current).filter(
          (item) => item.id !== tabId,
        );
        const shouldPickNext = current.activeTabId === tabId;
        const nextActiveTab = shouldPickNext
          ? nextTabs[index] || nextTabs[index - 1]
          : undefined;
        if (!shouldPickNext) {
          return {
            ...patchDocumentsWithPinnedState(current, nextTabs),
            pendingDirtyTabClose: null,
          };
        }
        if (!nextActiveTab) {
          return {
            ...patchDocumentsWithPinnedState(current, nextTabs),
            activeTabId: undefined,
            currentFileURI: null,
            currentFilePath: null,
            fileContent: '',
            isDirty: false,
            pendingScrollHeading: null,
            pendingRevealTarget: null,
            pendingBookTarget: null,
            editorId: null,
            pendingDirtyTabClose: null,
          };
        }
        return {
          ...patchDocumentsWithPinnedState(current, nextTabs),
          activeTabId: nextActiveTab.id,
          pendingDirtyTabClose: null,
          ...toActiveState(nextActiveTab),
        };
      });
      void syncWatches();
    };

    const scheduleBackup = (tab: EditorTab) => {
      if (tab.filePath) {
        // Has path = not untitled, use auto-save instead
        return;
      }
      if (!window.electronAPI?.backup) {
        return;
      }

      discardBackup(tab.id);

      const handle = setTimeout(async () => {
        backupTimers.delete(tab.id);
        if (!window.electronAPI?.backup) return;
        try {
          await window.electronAPI.backup.save({
            id: tab.id,
            title: tab.title,
            content: tab.content,
            editorId: tab.editorId,
          });
        } catch (e) {
          console.error('Error backing up untitled tab:', e);
        }
      }, BACKUP_DELAY_MS);

      backupTimers.set(tab.id, handle);
    };

    const scheduleAutoSave = (
      path: string | null | undefined,
      uri: CanonicalFileURI | null | undefined,
      content: string,
    ) => {
      if (!path) {
        return;
      }

      // Debounce per file path: keep pushing out while user edits.
      discardAutoSave(path);

      const handle = setTimeout(async () => {
        autoSaveTimers.delete(path);

        const state = get();
        if (state.connectionState !== 'connected') {
          return;
        }

        const tab = getOpenDocuments(state).find((t) => t.uri === uri);
        if (!tab || !tab.isDirty) {
          return;
        }

        // Ignore stale timers (a newer edit should have rescheduled).
        if (tab.content !== content) {
          return;
        }

        try {
          const result = await fileService.writeFile(path, content, {
            create: true,
            overwrite: true,
            atomic: true,
          });

          if (result.error) {
            console.error('Failed to auto save file:', result.error);
            return;
          }

          set((prev) => {
            const nextTabs = getOpenDocuments(prev).map((t) => {
              if (
                matchesDocumentTab(t, path, uri) &&
                t.isDirty &&
                t.content === content
              ) {
                return { ...t, isDirty: false };
              }
              return t;
            });

            const patch: Partial<AppState> = patchDocumentsWithPinnedState(
              prev,
              nextTabs,
            );
            if (
              uri &&
              prev.currentFileURI === uri &&
              prev.isDirty &&
              prev.fileContent === content
            ) {
              patch.isDirty = false;
            }
            return patch;
          });
        } catch (e) {
          console.error('Error auto saving file:', e);
        }
      }, AUTO_SAVE_DELAY_MS);

      autoSaveTimers.set(path, handle);
    };

    const watchDirectory = async (dir: string) => {
      if (!dir) {
        return;
      }
      if (get().connectionState !== 'connected') {
        return;
      }
      if (get().dirWatchIds.has(dir)) {
        return;
      }
      const result = await fileService.watch(dir, false);
      if (result.error || !result.watchId) {
        if (result.error && result.error !== 'Not connected') {
          console.error('Failed to watch directory:', dir, result.error);
        }
        return;
      }
      set((state) => {
        const next = new Map(state.dirWatchIds);
        next.set(dir, result.watchId!);
        return { dirWatchIds: next };
      });
    };

    const unwatchDirectory = async (dir: string) => {
      const watchId = get().dirWatchIds.get(dir);
      if (!watchId) {
        return;
      }
      try {
        await fileService.unwatch(watchId);
      } catch {
        // ignore
      }
      set((state) => {
        const next = new Map(state.dirWatchIds);
        next.delete(dir);
        return { dirWatchIds: next };
      });
    };

    const clearAllWatches = async () => {
      const watchIds = Array.from(get().dirWatchIds.values());
      if (watchIds.length === 0) {
        return;
      }
      await Promise.all(
        watchIds.map(async (id) => {
          try {
            await fileService.unwatch(id);
          } catch {
            // ignore
          }
        }),
      );
      set({ dirWatchIds: new Map() });
    };

    const clearWorkspaceSyncWatch = async () => {
      if (workspaceSyncChangeTimer) {
        clearTimeout(workspaceSyncChangeTimer);
        workspaceSyncChangeTimer = null;
      }
      const watchId = workspaceSyncWatchId;
      workspaceSyncWatchId = '';
      workspaceSyncWatchPath = '';
      if (!watchId) {
        return;
      }
      try {
        await fileService.unwatch(watchId);
      } catch {
        // ignore
      }
    };

    const syncWorkspaceSyncWatch = async () => {
      const state = get();
      const targetDir = normalizePosixPath(state.currentDir || '');
      const enabled = state.connectionState === 'connected' &&
        targetDir &&
        state.storageInfo.storage?.enabled &&
        state.storageInfo.policy.onLocalChange;
      if (!enabled) {
        await clearWorkspaceSyncWatch();
        return;
      }
      if (workspaceSyncWatchPath === targetDir && workspaceSyncWatchId) {
        return;
      }
      await clearWorkspaceSyncWatch();
      const result = await fileService.watch(targetDir, true, [
        '.git',
        '.openbrain',
        'node_modules',
        'dist',
        'build',
        '.cache',
      ]);
      if (result.error || !result.watchId) {
        if (result.error && result.error !== 'Not connected') {
          useMessengerStore.getState().upsertMessage({
            id: `workspace-sync-watch:${targetDir}`,
            severity: 'error',
            source: 'Workspace Sync',
            title: 'Local change sync is not watching',
            body: result.error,
            workspacePath: targetDir,
            action: 'open-sync',
          });
        }
        return;
      }
      workspaceSyncWatchPath = targetDir;
      workspaceSyncWatchId = result.watchId;
    };

    const getOpenFileWatchDirs = (
      state: Pick<AppState, 'documents'>,
    ): string[] =>
      uniqueDirs(
        getOpenDocuments(state).map((tab) => {
          const path = normalizePosixPath((tab.filePath || '').trim());
          if (!path) {
            return null;
          }
          const slash = path.lastIndexOf('/');
          if (slash <= 0) {
            return null;
          }
          return path.slice(0, slash);
        }),
      );

    const getDesiredWatchDirs = (
      state: Pick<AppState, 'currentDir' | 'expandedDirs' | 'documents'>,
    ): string[] =>
      uniqueDirs([
        state.currentDir,
        ...Array.from(state.expandedDirs),
        ...getOpenFileWatchDirs(state),
      ]);

    const reloadOpenTabsByPaths = async (
      paths: string[],
      options?: { skipDirty?: boolean },
    ) => {
      if (get().connectionState !== 'connected') {
        return;
      }

      const pathSet = new Set(
        paths
          .map((path) => normalizePosixPath((path || '').trim()))
          .filter(Boolean),
      );
      if (pathSet.size === 0) {
        return;
      }

      const skipDirty = options?.skipDirty === true;
      const snapshot = get();
      const tabsToReload = getOpenDocuments(snapshot).filter((tab) => {
        const path = normalizePosixPath((tab.filePath || '').trim());
        if (!path || !pathSet.has(path)) {
          return false;
        }
        if (skipDirty && tab.isDirty) {
          return false;
        }
        return true;
      });
      if (tabsToReload.length === 0) {
        return;
      }

      const uniquePaths = Array.from(
        new Set(
          tabsToReload
            .map((tab) => normalizePosixPath(tab.filePath || ''))
            .filter(Boolean),
        ),
      );
      const binaryPreviewPaths = new Set(
        tabsToReload
          .filter(isBinaryPreviewDocumentTab)
          .map((tab) => normalizePosixPath(tab.filePath || ''))
          .filter(Boolean),
      );
      const textPaths = uniquePaths.filter(
        (path) => !binaryPreviewPaths.has(path),
      );
      const results = await Promise.all(
        textPaths.map(async (path) => ({
          path,
          result: await fileService.readFile(path),
        })),
      );
      const resultByPath = new Map(
        results.map((item) => [item.path, item.result]),
      );
      const binaryPreviewResults = await Promise.all(
        Array.from(binaryPreviewPaths).map(async (path) => ({
          path,
          result: await fileService.stat(path),
        })),
      );
      const binaryPreviewResultByPath = new Map(
        binaryPreviewResults.map((item) => [item.path, item.result]),
      );

      set((state) => {
        let tabsChanged = false;
        const nextTabs = getOpenDocuments(state).map((tab) => {
          const path = normalizePosixPath((tab.filePath || '').trim());
          if (!path || !pathSet.has(path)) {
            return tab;
          }
          if (skipDirty && tab.isDirty) {
            return tab;
          }

          if (isBinaryPreviewDocumentTab(tab)) {
            const result = binaryPreviewResultByPath.get(path);
            if (!result) {
              return tab;
            }
            if (result.error) {
              if (/file not found/i.test(result.error)) {
                if (tab.missing && tab.isDirty === false) {
                  return tab;
                }
                tabsChanged = true;
                return { ...tab, isDirty: false, missing: true };
              }
              return tab;
            }

            tabsChanged = true;
            return {
              ...tab,
              isDirty: false,
              missing: false,
              resourceVersion: (tab.resourceVersion ?? 0) + 1,
            };
          }

          const result = resultByPath.get(path);
          if (!result) {
            return tab;
          }
          if (result.error) {
            if (/file not found/i.test(result.error)) {
              if (tab.missing) {
                return tab;
              }
              tabsChanged = true;
              return { ...tab, isDirty: false, missing: true };
            }
            return tab;
          }
          if (result.tooLarge) {
            return tab;
          }

          const nextContent = result.content || '';
          if (
            tab.content === nextContent &&
            tab.isDirty === false &&
            tab.missing !== true
          ) {
            return tab;
          }
          tabsChanged = true;
          return {
            ...tab,
            content: nextContent,
            isDirty: false,
            missing: false,
          };
        });

        if (!tabsChanged) {
          return {};
        }

        const activeTab = state.activeTabId
          ? nextTabs.find((tab) => tab.id === state.activeTabId) || null
          : null;
        if (!activeTab) {
          return patchDocumentsWithPinnedState(state, nextTabs);
        }
        return {
          ...patchDocumentsWithPinnedState(state, nextTabs),
          ...toActiveState(activeTab),
        };
      });
    };

    const persistTabContent = async (
      tabId: string,
      targetPath: string,
      content: string,
    ): Promise<boolean> => {
      const normalizedTargetPath = normalizePosixPath(
        (targetPath || '').trim(),
      );
      if (!normalizedTargetPath) {
        return false;
      }

      discardAutoSave(normalizedTargetPath);

      try {
        const result = await fileService.writeFile(
          normalizedTargetPath,
          content,
          {
            create: true,
            overwrite: true,
            atomic: true,
          },
        );
        if (result.error) {
          console.error('Failed to save file:', result.error);
          return false;
        }

        set((state) => {
          const nextTabs = getOpenDocuments(state).map((tab) => {
            if (tab.id !== tabId) {
              return tab;
            }

            const nextPath = tab.filePath || normalizedTargetPath;
            return {
              ...tab,
              uri: pathToDocumentURI(nextPath, state) || tab.uri,
              filePath: nextPath,
              title: tab.filePath
                ? tab.title
                : getFileTitle(normalizedTargetPath),
              content,
              isDirty: false,
              missing: false,
            };
          });

          const activeTab =
            state.activeTabId === tabId
              ? nextTabs.find((tab) => tab.id === tabId) || null
              : null;
          if (!activeTab) {
            return patchDocumentsWithPinnedState(state, nextTabs);
          }
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            currentFileURI: activeTab.uri || null,
            currentFilePath: activeTab.filePath || null,
            fileContent: activeTab.content,
            isDirty: false,
            pendingScrollHeading: activeTab.pendingScrollHeading,
            pendingRevealTarget: activeTab.pendingRevealTarget || null,
            editorId: activeTab.editorId,
          };
        });

        void syncWatches();
        const agentCwd = workdirFromAgentConfigPath(normalizedTargetPath);
        if (agentCwd) {
          refreshAgentBindingsForDirs([agentCwd]);
        }
        return true;
      } catch (error) {
        console.error('Error saving file:', error);
        return false;
      }
    };

    const syncWatches = async () => {
      if (get().connectionState !== 'connected') {
        return;
      }
      const { currentDir, expandedDirs, dirWatchIds } = get();
      const desired = new Set(
        getDesiredWatchDirs({
          currentDir,
          expandedDirs,
          documents: get().documents,
        }),
      );

      // Unwatch removed dirs.
      for (const dir of dirWatchIds.keys()) {
        if (!desired.has(dir)) {
          void unwatchDirectory(dir);
        }
      }

      // Watch new dirs.
      for (const dir of desired) {
        void watchDirectory(dir);
      }
      void syncWorkspaceSyncWatch();
    };

    const restoreVisibleTreeAfterConnect = async (
      derived: DerivedDirs | null,
    ) => {
      const state = get();
      const remoteIdentity = buildRemoteSessionIdentity(state.remoteSession);
      const snapshot =
        remoteIdentity && visibleTreeSnapshot?.remoteIdentity === remoteIdentity
          ? visibleTreeSnapshot
          : null;
      const pendingDir = normalizeDirPath(pendingEnsureDir || '') || null;
      const currentDir = normalizeDirPath(state.currentDir || '') || null;
      const snapshotCurrentDir =
        normalizeDirPath(snapshot?.currentDir || '') || null;

      const restoredCurrentDir =
        pendingDir ||
        (snapshotCurrentDir && isAbsoluteDirPath(snapshotCurrentDir)
          ? snapshotCurrentDir
          : null) ||
        (currentDir && isAbsoluteDirPath(currentDir) ? currentDir : null) ||
        derived?.workspaceDir ||
        currentDir;

      const restoredExpandedDirs = derived
        ? uniqueDirs(
            (snapshot?.expandedDirs ?? Array.from(state.expandedDirs)).filter(
              (dir) => {
                if (isPathInVisibleRoots(dir, derived)) {
                  return true;
                }
                if (
                  restoredCurrentDir &&
                  isPathInsideRoot(dir, restoredCurrentDir)
                ) {
                  return true;
                }
                return false;
              },
            ),
          )
        : uniqueDirs(snapshot?.expandedDirs ?? Array.from(state.expandedDirs));

      set((store) => ({
        currentDir: restoredCurrentDir,
        expandedDirs: new Set(restoredExpandedDirs),
        entries: restoredCurrentDir
          ? (store.dirEntries.get(restoredCurrentDir) ?? [])
          : [],
      }));

      if (pendingDir && restoredCurrentDir === pendingDir) {
        pendingEnsureDir = null;
        await get().ensureDirectory(restoredCurrentDir);
      } else {
        pendingEnsureDir = null;
        const dirsToReload = uniqueDirs([
          restoredCurrentDir,
          derived?.agentsDir,
          ...restoredExpandedDirs,
        ]);
        if (dirsToReload.length > 0) {
          await Promise.all(
            dirsToReload.map((dir) => get().loadDirectory(dir)),
          );
        }
      }

      await refreshNodesCache();

      const workspaceDirs = getVisibleWorkspaceDirs({
        currentDir: restoredCurrentDir,
        expandedDirs: new Set(restoredExpandedDirs),
      });
      if (workspaceDirs.length > 0) {
        await Promise.all(
          workspaceDirs.map((dir) => get().fetchDirAgentsInfo(dir)),
        );
      }

      void syncWatches();
    };

    const disposeStore = () => {
      pendingDirs.clear();
      pendingEnsureDir = null;
      streamingChatPath = '';

      if (fileChangeTimer) {
        clearTimeout(fileChangeTimer);
        fileChangeTimer = null;
      }

      for (const handle of autoSaveTimers.values()) {
        clearTimeout(handle);
      }
      autoSaveTimers.clear();

      for (const handle of backupTimers.values()) {
        clearTimeout(handle);
      }
      backupTimers.clear();

      void clearAllWatches();
      void clearWorkspaceSyncWatch();
      connection.dispose();
      clearAgentRecordCache();
      clearAgentObjectIndex();
      clearVisibleTreeSnapshot();

      set((state) => ({
        connectionState: 'disconnected',
        remoteSession: null,
        remoteConnecting: false,
        remoteError: null,
        baseDir: null,
        workspaceRootDir: null,
        agentsRootDir: null,
        instanceID: null,
        currentDir: null,
        entries: [],
        dirEntries: new Map(),
        dirLoading: new Set(),
        dirErrors: new Map(),
        dirWatchIds: new Map(),
        expandedDirs: new Set(),
        requestRootAction: null,
        sidebarSearchQuery: '',
        sidebarSearchIncludes: '',
        sidebarSearchExcludes: '',
        sidebarSearchFlags: {
          matchCase: false,
          wholeWord: false,
          regex: false,
        },
        sidebarSearchLoading: false,
        sidebarSearchError: null,
        sidebarSearchResults: [],
        sidebarSearchTotalCount: 0,
        sidebarSearchTruncated: false,
        sidebarSearchRequestSeq: 0,
        nodesByID: new Map(),
        nodeGraphRevision: state.nodeGraphRevision + 1,
        agentBindingByCwd: new Map(),
        agentNodes: [],
        skillNodes: [],
        agentNodesLoading: false,
        gitInfo: createEmptyGitInfo(),
        storageInfo: createEmptyWorkspaceStorageInfo(),
        currentFileURI: null,
        currentFilePath: null,
        fileContent: '',
        isDirty: false,
        pendingScrollHeading: null,
        pendingRevealTarget: null,
        pendingBookTarget: null,
        currentReviewOverlay: null,
        editorId: null,
        editorFocused: false,
        editorBlurRequestSeq: 0,
        editorFocusRequest: null,
        documents: [],
        activeTabId: undefined,
        pinnedTabId: undefined,
        pendingDirtyTabClose: null,
      }));
    };

    return {
      // Initial state
      connectionState: 'disconnected',
      serverUrl: 'ws://127.0.0.1:19530/ws',
      remoteSession: null,
      remoteConnecting: false,
      remoteError: null,
      baseDir: null,
      workspaceRootDir: null,
      agentsRootDir: null,
      instanceID: null,
      currentDir: null,
      entries: [],
      dirEntries: new Map<string, FileEntry[]>(),
      dirLoading: new Set<string>(),
      dirErrors: new Map<string, string>(),
      dirWatchIds: new Map<string, string>(),
      expandedDirs: new Set<string>(),
      requestRootAction: null as 'new-file' | 'new-folder' | null,
      sidebarSearchQuery: '',
      sidebarSearchIncludes: '',
      sidebarSearchExcludes: '',
      sidebarSearchFlags: { matchCase: false, wholeWord: false, regex: false },
      sidebarSearchLoading: false,
      sidebarSearchError: null,
      sidebarSearchResults: [],
      sidebarSearchTotalCount: 0,
      sidebarSearchTruncated: false,
      sidebarSearchRequestSeq: 0,
      nodesByID: new Map<string, OpNode>(),
      nodeGraphRevision: 0,
      agentBindingByCwd: new Map<string, AgentBinding>(),
      agentNodes: [],
      skillNodes: [],
      agentNodesLoading: false,
      gitInfo: createEmptyGitInfo(),
      storageInfo: createEmptyWorkspaceStorageInfo(),
      currentFileURI: null,
      currentFilePath: null,
      fileContent: '',
      isDirty: false,
      pendingScrollHeading: null,
      pendingRevealTarget: null,
      pendingBookTarget: null,
      currentReviewOverlay: null,
      editorId: null,
      editorFocused: false,
      editorBlurRequestSeq: 0,
      editorFocusRequest: null,
      documents: [],
      activeTabId: undefined,
      pinnedTabId: undefined,
      pendingDirtyTabClose: null,

      // Actions
      connect: () => {
        const { serverUrl } = get();
        if (!serverUrl) {
          set({ connectionState: 'disconnected' });
          return;
        }
        connection.connect(serverUrl, buildConnectionCallbacks());
      },

      setServerUrl: (url: string) => {
        set({ serverUrl: url });
      },

      setRemoteSession: (session: RemoteSessionInfo | null) => {
        if (session) {
          const state = get();
          const previousRemoteSession = get().remoteSession;
          const cachedDerived = getCachedDerivedDirsByServerUrl(session.wsUrl);
          const existingCurrentDir = state.currentDir;
          const nextIdentity = buildRemoteSessionIdentity(session);
          const previousIdentity = buildRemoteSessionIdentity(
            previousRemoteSession,
          );
          const keepVisibleTree =
            !!nextIdentity && nextIdentity === previousIdentity;
          void clearAllWatches();
          void clearWorkspaceSyncWatch();
          agentService.clearCache();
          clearAgentRecordCache();
          if (!keepVisibleTree) {
            clearAgentObjectIndex();
            clearVisibleTreeSnapshot();
          }
          set({
            remoteSession: session,
            serverUrl: session.wsUrl,
            baseDir: cachedDerived?.baseDir || null,
            workspaceRootDir: cachedDerived?.workspaceDir || null,
            agentsRootDir: cachedDerived?.agentsDir || null,
            instanceID: cachedDerived?.instanceID || null,
            currentDir: keepVisibleTree
              ? existingCurrentDir ||
                cachedDerived?.workspaceDir ||
                session.workspaceDir ||
                null
              : cachedDerived?.workspaceDir || session.workspaceDir || null,
            entries: keepVisibleTree ? state.entries : [],
            dirEntries: keepVisibleTree ? new Map(state.dirEntries) : new Map(),
            dirLoading: new Set(),
            dirErrors: keepVisibleTree ? new Map(state.dirErrors) : new Map(),
            dirWatchIds: new Map(),
            expandedDirs: keepVisibleTree
              ? new Set(state.expandedDirs)
              : new Set(),
            sidebarSearchLoading: false,
            sidebarSearchError: null,
            sidebarSearchResults: [],
            sidebarSearchTotalCount: 0,
            sidebarSearchTruncated: false,
            sidebarSearchRequestSeq: 0,
            nodesByID: keepVisibleTree ? new Map(state.nodesByID) : new Map(),
            nodeGraphRevision: keepVisibleTree
              ? state.nodeGraphRevision
              : state.nodeGraphRevision + 1,
            agentBindingByCwd: keepVisibleTree
              ? new Map(state.agentBindingByCwd)
              : new Map(),
            agentNodes: keepVisibleTree ? state.agentNodes : [],
            skillNodes: keepVisibleTree ? state.skillNodes : [],
            agentNodesLoading: false,
            storageInfo: keepVisibleTree
              ? state.storageInfo
              : createEmptyWorkspaceStorageInfo(),
          });
        } else {
          void clearAllWatches();
          void clearWorkspaceSyncWatch();
          agentService.clearCache();
          clearAgentRecordCache();
          clearAgentObjectIndex();
          clearVisibleTreeSnapshot();
          set((state) => ({
            remoteSession: null,
            serverUrl: 'ws://127.0.0.1:19530/ws',
            baseDir: null,
            workspaceRootDir: null,
            agentsRootDir: null,
            instanceID: null,
            currentDir: null,
            entries: [],
            dirEntries: new Map(),
            dirLoading: new Set(),
            dirErrors: new Map(),
            dirWatchIds: new Map(),
            expandedDirs: new Set(),
            sidebarSearchLoading: false,
            sidebarSearchError: null,
            sidebarSearchResults: [],
            sidebarSearchTotalCount: 0,
            sidebarSearchTruncated: false,
            sidebarSearchRequestSeq: 0,
            nodesByID: new Map(),
            nodeGraphRevision: state.nodeGraphRevision + 1,
            agentBindingByCwd: new Map(),
            agentNodes: [],
            skillNodes: [],
            agentNodesLoading: false,
            storageInfo: createEmptyWorkspaceStorageInfo(),
          }));
        }
      },

      setRemoteConnecting: (next: boolean) => {
        set({ remoteConnecting: next });
      },

      setRemoteError: (next: string | null) => {
        set({ remoteError: next });
      },

      disconnect: () => {
        void clearAllWatches();
        void clearWorkspaceSyncWatch();
        connection.disconnect();
      },

      reconnectNow: () => {
        const { serverUrl } = get();
        if (!serverUrl) {
          set({ connectionState: 'disconnected' });
          return;
        }
        connection.connect(serverUrl, buildConnectionCallbacks());
        connection.forceReconnect('workspace-activated');
      },

      suspend: () => {
        void clearAllWatches();
        void clearWorkspaceSyncWatch();
        connection.suspend();
      },

      resume: () => {
        connection.resume();
      },

      dispose: () => {
        disposeStore();
      },

      setActive: (active: boolean) => {
        if (active) {
          // Reconnect if needed (e.g. connection was lost while inactive)
          if (get().connectionState !== 'connected') {
            get().reconnectNow();
          }
          const { currentDir, dirEntries } = get();
          if (currentDir) {
            const entries = dirEntries.get(currentDir);
            if (!entries || entries.length === 0) {
              get().loadDirectory(currentDir);
            }
          }
          void syncWatches();
          void syncWorkspaceSyncWatch();
        } else {
          // When window loses focus, only pause file change processing
          // but keep the WebSocket connection alive
          pendingDirs.clear();
          if (fileChangeTimer) {
            clearTimeout(fileChangeTimer);
            fileChangeTimer = null;
          }
          // Don't call connection.suspend() - keep WebSocket connected
          // Just clear watches to reduce server load
          void clearAllWatches();
          void clearWorkspaceSyncWatch();
        }
      },

      setCurrentDir: (dir: string) => {
        void clearAllWatches();
        void clearWorkspaceSyncWatch();
        agentService.clearCache();
        clearAgentRecordCache();
        clearAgentObjectIndex();
        set((state) => ({
          currentDir: dir,
          entries: [],
          dirEntries: new Map(),
          dirLoading: new Set(),
          dirErrors: new Map(),
          dirWatchIds: new Map(),
          expandedDirs: new Set(),
          nodesByID: new Map(),
          nodeGraphRevision: state.nodeGraphRevision + 1,
          agentBindingByCwd: new Map(),
          agentNodes: [],
          skillNodes: [],
          agentNodesLoading: false,
          gitInfo: createEmptyGitInfo(),
          storageInfo: createEmptyWorkspaceStorageInfo(),
        }));
        if (get().connectionState === 'connected') {
          void syncWatches();
          void (async () => {
            await refreshNodesCache();
            if (
              normalizeDirPath(get().currentDir || '') !== normalizeDirPath(dir)
            ) {
              return;
            }
            await get().fetchDirAgentsInfo(dir);
          })();
          void get().refreshGitInfo(dir);
          void get().refreshStorageStatus(dir);
          get().loadDirectory(dir);
        }
      },

      setRequestRootAction: (action: 'new-file' | 'new-folder' | null) => {
        set({ requestRootAction: action });
      },

      setSidebarSearchQuery: (value: string) => {
        set({ sidebarSearchQuery: value });
      },

      setSidebarSearchIncludes: (value: string) => {
        set({ sidebarSearchIncludes: value });
      },

      setSidebarSearchExcludes: (value: string) => {
        set({ sidebarSearchExcludes: value });
      },

      setSidebarSearchFlag: (
        flag: keyof SidebarSearchFlags,
        value: boolean,
      ) => {
        set((state) => ({
          sidebarSearchFlags: {
            ...state.sidebarSearchFlags,
            [flag]: value,
          },
        }));
      },

      clearSidebarSearchState: () => {
        set({
          sidebarSearchLoading: false,
          sidebarSearchError: null,
          sidebarSearchResults: [],
          sidebarSearchTotalCount: 0,
          sidebarSearchTruncated: false,
        });
      },

      runSidebarSearch: async (options?: { installRetry?: boolean }) => {
        const state = get();
        const root = normalizeDirPath(state.currentDir || '');
        const query = state.sidebarSearchQuery.trim();
        if (!root || state.connectionState !== 'connected') {
          set({
            sidebarSearchLoading: false,
            sidebarSearchError:
              state.connectionState === 'connected' ? null : 'Not connected',
            sidebarSearchResults: [],
            sidebarSearchTotalCount: 0,
            sidebarSearchTruncated: false,
          });
          return;
        }
        if (!query) {
          set({
            sidebarSearchLoading: false,
            sidebarSearchError: null,
            sidebarSearchResults: [],
            sidebarSearchTotalCount: 0,
            sidebarSearchTruncated: false,
          });
          return;
        }

        const requestSeq = state.sidebarSearchRequestSeq + 1;
        set({
          sidebarSearchLoading: true,
          sidebarSearchError: null,
          sidebarSearchRequestSeq: requestSeq,
        });

        const result = await fileService.search({
          root,
          query,
          regex: state.sidebarSearchFlags.regex,
          matchCase: state.sidebarSearchFlags.matchCase,
          wholeWord: state.sidebarSearchFlags.wholeWord,
          includes: parseSidebarSearchGlobs(state.sidebarSearchIncludes),
          excludes: parseSidebarSearchGlobs(state.sidebarSearchExcludes),
        });

        if (get().sidebarSearchRequestSeq !== requestSeq) {
          return;
        }

        if (result.error) {
          const canInstallRetry =
            options?.installRetry !== false &&
            result.error.toLowerCase().includes(SEARCH_BINARY_MISSING_TEXT);
          if (canInstallRetry) {
            const install = await get().installMarketplaceItem(
              'tool',
              'rg-search',
            );
            if (!install.success) {
              set({
                sidebarSearchLoading: false,
                sidebarSearchError: install.error || result.error,
                sidebarSearchResults: [],
                sidebarSearchTotalCount: 0,
                sidebarSearchTruncated: false,
              });
              return;
            }
            await get().runSidebarSearch({ installRetry: false });
            return;
          }

          set({
            sidebarSearchLoading: false,
            sidebarSearchError: result.error,
            sidebarSearchResults: [],
            sidebarSearchTotalCount: 0,
            sidebarSearchTruncated: false,
          });
          return;
        }

        set({
          sidebarSearchLoading: false,
          sidebarSearchError: null,
          sidebarSearchResults: result.files || [],
          sidebarSearchTotalCount: result.totalCount || 0,
          sidebarSearchTruncated: result.truncated === true,
        });
      },

      listThreadReviews: async (threadID: string) => {
        return reviewService.listReviews(threadID);
      },

      resolveThreadReview: async (params) => {
        return reviewService.resolveReview(params);
      },

      rollbackThreadReview: async (params) => {
        return reviewService.rollbackReview(params);
      },

      execCommand: async ({ command, workspaceRoot, targetPath }) => {
        return connection.request<{
          commandID: string;
          filePath: string;
          workspaceRoot: string;
          created: boolean;
        }>('command/exec', {
          command,
          workspaceRoot,
          ...(targetPath ? { targetPath } : {}),
        });
      },

      stopCommand: async (commandID) => {
        await connection.request('command/stop', { commandID });
      },

      refreshGitInfo: async (path?: string) => {
        const targetDir = normalizeDirPath(path || get().currentDir || '');
        if (!targetDir || get().connectionState !== 'connected') {
          set({ gitInfo: createEmptyGitInfo() });
          return;
        }

        const requestSeq = ++gitInfoRequestSeq;
        set((state) => ({
          gitInfo: {
            ...state.gitInfo,
            status: 'loading',
            error: null,
            loadedPath: targetDir,
          },
        }));

        const result = await gitService.getBranches(targetDir);
        if (requestSeq !== gitInfoRequestSeq) {
          return;
        }

        if (result.error) {
          console.warn('[git] failed to load branches:', result.error);
          set({
            gitInfo: {
              ...createEmptyGitInfo(),
              status: 'ready',
              error: result.error,
              loadedPath: targetDir,
            },
          });
          return;
        }

        if (!result.isRepo) {
          set({
            gitInfo: {
              ...createEmptyGitInfo(),
              status: 'ready',
              loadedPath: targetDir,
            },
          });
          return;
        }

        set({
          gitInfo: {
            status: 'ready',
            error: null,
            isRepo: true,
            repoRoot: result.repoRoot || null,
            currentBranch: result.currentBranch || null,
            detached: result.detached === true,
            detachedLabel: result.detachedLabel || null,
            branches: result.branches || [],
            dirty: result.dirty || { ...EMPTY_GIT_DIRTY },
            loadedPath: targetDir,
          },
        });
      },

      refreshStorageStatus: async (path?: string) => {
        const targetDir = normalizeDirPath(path || get().currentDir || '');
        if (!targetDir || get().connectionState !== 'connected') {
          const empty = createEmptyWorkspaceStorageInfo();
          set({ storageInfo: empty });
          return null;
        }

        const requestSeq = ++storageStatusRequestSeq;
        set((state) => ({
          storageInfo: {
            ...state.storageInfo,
            status: 'loading',
            error: null,
            path: targetDir,
          },
        }));

        const modelParams = await resolveOpenBrainCloudSyncModelParams();
        const result = await storageService.status({ path: targetDir, ...modelParams });
        if (requestSeq !== storageStatusRequestSeq) {
          return null;
        }

        const info = storageInfoFromResult(result, targetDir);
        set({ storageInfo: info });
        upsertWorkspaceSyncMessage(info);
        void syncWorkspaceSyncWatch();
        return info;
      },

      updateWorkspaceSyncPolicy: async (policy: WorkspaceSyncPolicy) => {
        const targetDir = normalizeDirPath(get().currentDir || '');
        if (!targetDir || get().connectionState !== 'connected') {
          return null;
        }
        const modelParams = await resolveOpenBrainCloudSyncModelParams();
        const result = await storageService.updatePolicy({ path: targetDir, policy, ...modelParams });
        const info = storageInfoFromResult(result, targetDir);
        set({ storageInfo: info });
        upsertWorkspaceSyncMessage(info);
        void syncWorkspaceSyncWatch();
        return info;
      },

      syncWorkspaceNow: async (_options?: {
        reason?: 'manual' | 'local-change';
      }) => {
        const targetDir = normalizeDirPath(get().currentDir || '');
        if (!targetDir || get().connectionState !== 'connected') {
          return null;
        }
        if (storageSyncInFlight) {
          return storageSyncInFlight;
        }

        set((state) => ({
          storageInfo: {
            ...state.storageInfo,
            status: 'syncing',
            error: null,
            path: targetDir,
          },
        }));

        storageSyncInFlight = (async () => {
          const modelParams = await resolveOpenBrainCloudSyncModelParams();
          const result = await storageService.syncNow({ path: targetDir, ...modelParams });
          const info = storageInfoFromResult(result, targetDir);
          set({ storageInfo: info });
          upsertWorkspaceSyncMessage(info);
          void syncWorkspaceSyncWatch();
          return info;
        })().finally(() => {
          storageSyncInFlight = null;
        });

        return storageSyncInFlight;
      },

      listCronTasks: async () => {
        if (get().connectionState !== 'connected') {
          return [];
        }
        return cronService.list();
      },

      getCronTask: async (id: string) => {
        if (!id || get().connectionState !== 'connected') {
          return null;
        }
        return cronService.get(id);
      },

      updateCronTask: async (task: CronTask) => {
        if (!task?.id || get().connectionState !== 'connected') {
          throw new Error('Cron requires an active runtime connection.');
        }
        const result = await cronService.update(task);
        if (!result?.task) {
          throw new Error('Cron update returned no task.');
        }
        const title = result.task.name?.trim();
        if (result.task.id && title) {
          get().openCronTaskTab(result.task.id, title);
        }
        return result;
      },

      runCronTask: async (id: string) => {
        if (!id || get().connectionState !== 'connected') {
          throw new Error('Cron requires an active runtime connection.');
        }
        return cronService.run(id);
      },

      listCronTaskHistory: async (id: string, limit = CRON_TASK_HISTORY_LIMIT) => {
        if (!id || get().connectionState !== 'connected') {
          return [];
        }
        return cronService.history(id, limit);
      },

      ensureDirectory: async (dir: string) => {
        if (!dir) {
          return;
        }
        if (get().connectionState !== 'connected') {
          pendingEnsureDir = dir;
          return;
        }
        try {
          void watchDirectory(dir);
          const result = await fileService.mkdir(dir, true);
          if (result.error && result.error !== 'Not connected') {
            console.error('Failed to ensure directory:', result.error);
          }
        } catch (e) {
          console.error('Error ensuring directory:', e);
        }
        await get().loadDirectory(dir);
      },

      loadDirectory: async (dir: string) => {
        if (get().connectionState !== 'connected') {
          return;
        }
        const hadSnapshot = get().dirEntries.has(dir);
        set((state) => {
          const nextLoading = new Set(state.dirLoading);
          const nextErrors = new Map(state.dirErrors);
          if (!state.dirEntries.has(dir)) {
            nextLoading.add(dir);
          } else {
            nextLoading.delete(dir);
          }
          nextErrors.delete(dir);
          return { dirLoading: nextLoading, dirErrors: nextErrors };
        });
        try {
          const result = await fileService.readdir(dir);
          if (result.error) {
            if (result.error !== 'Not connected') {
              console.error('Failed to load directory:', result.error);
            }
            set((state) => {
              const nextMap = new Map(state.dirEntries);
              const nextLoading = new Set(state.dirLoading);
              const nextErrors = new Map(state.dirErrors);
              const message = result.error || 'Failed to load directory';
              nextLoading.delete(dir);
              if (state.dirEntries.has(dir) && isTransientDirectoryLoadError(message)) {
                nextErrors.delete(dir);
              } else {
                nextMap.set(dir, []);
                nextErrors.set(dir, message);
              }
              const patch: Partial<AppState> = {
                dirEntries: nextMap,
                dirLoading: nextLoading,
                dirErrors: nextErrors,
              };
              if (dir === state.currentDir && (!state.dirEntries.has(dir) || !isTransientDirectoryLoadError(message))) {
                patch.entries = [];
              }
              return patch;
            });
            return;
          }

          const entries = result.entries || [];
          set((state) => {
            const nextMap = new Map(state.dirEntries);
            const nextLoading = new Set(state.dirLoading);
            const nextErrors = new Map(state.dirErrors);
            nextMap.set(dir, entries);
            nextLoading.delete(dir);
            nextErrors.delete(dir);
            const patch: Partial<AppState> = {
              dirEntries: nextMap,
              dirLoading: nextLoading,
              dirErrors: nextErrors,
            };
            if (dir === state.currentDir) {
              patch.entries = entries;
            }
            return patch;
          });

          const snapshot = get();
          if (shouldScanWorkspaceDir(dir, snapshot)) {
            // Lazy load: do not block readdir/UI on workspace agent bindings.
            const scanRevision = agentScanRevision;
            void requestDirAgentNodes(dir)
              .then((nodes) => {
                if (scanRevision !== agentScanRevision) {
                  return;
                }
                set((state) => {
                  const nextNodesByID = mergeNodesByID(state.nodesByID, nodes);
                  const bindings = resolveAgentBindingsFromNodes(nodes);
                  return {
                    nodesByID: nextNodesByID,
                    nodeGraphRevision: state.nodeGraphRevision + 1,
                    agentBindingByCwd: mergeDirAgentBindings(
                      state.agentBindingByCwd,
                      dir,
                      bindings,
                      true,
                    ),
                  };
                });
              })
              .catch(() => {
                // ignore
              });
          }
        } catch (e) {
          console.error('Error loading directory:', e);
          set((state) => {
            const nextMap = new Map(state.dirEntries);
            const nextLoading = new Set(state.dirLoading);
            const nextErrors = new Map(state.dirErrors);
            const message =
              e instanceof Error ? e.message : 'Failed to load directory';
            nextLoading.delete(dir);
            if ((state.dirEntries.has(dir) || hadSnapshot) && isTransientDirectoryLoadError(message)) {
              nextErrors.delete(dir);
            } else {
              nextMap.set(dir, []);
              nextErrors.set(dir, message);
            }
            const patch: Partial<AppState> = {
              dirEntries: nextMap,
              dirLoading: nextLoading,
              dirErrors: nextErrors,
            };
            if (dir === state.currentDir && ((!state.dirEntries.has(dir) && !hadSnapshot) || !isTransientDirectoryLoadError(message))) {
              patch.entries = [];
            }
            return patch;
          });
        }
      },

      fetchDirAgentsInfo: async (dir: string) => {
        const target = (dir || '').trim();
        if (!target || get().connectionState !== 'connected') {
          return;
        }
        if (!shouldScanWorkspaceDir(target, get())) {
          return;
        }

        if (!get().dirEntries.get(target)) {
          await get().loadDirectory(target);
        }
        try {
          const scanRevision = agentScanRevision;
          const nodes = await requestDirAgentNodes(target);
          if (scanRevision !== agentScanRevision) {
            return;
          }
          set((state) => {
            const nextNodesByID = mergeNodesByID(state.nodesByID, nodes);
            const bindings = resolveAgentBindingsFromNodes(nodes);
            return {
              nodesByID: nextNodesByID,
              nodeGraphRevision: state.nodeGraphRevision + 1,
              agentBindingByCwd: mergeDirAgentBindings(
                state.agentBindingByCwd,
                target,
                bindings,
                true,
              ),
            };
          });
        } catch {
          // ignore
        }
      },

      readDirectory: async (dir: string) => {
        try {
          await get().loadDirectory(dir);
          return get().dirEntries.get(dir) ?? EMPTY_ENTRIES;
        } catch {
          return EMPTY_ENTRIES;
        }
      },

      listDirectory: async (dir: string) => {
        if (get().connectionState !== 'connected') {
          return { error: 'Not connected' };
        }
        return fileService.readdir(dir);
      },

      statPath: async (path: string) => {
        if (get().connectionState !== 'connected') {
          return { error: 'Not connected' } as StatResult & { error: string };
        }
        return fileService.stat(path);
      },

      toggleDir: (dir: string) => {
        const { expandedDirs } = get();
        const newExpanded = new Set(expandedDirs);

        if (newExpanded.has(dir)) {
          newExpanded.delete(dir);
        } else {
          newExpanded.add(dir);
          get().loadDirectory(dir);
        }

        set({ expandedDirs: newExpanded });
        if (get().connectionState === 'connected') {
          void syncWatches();
        }
      },

      openFile: async (
        path: string,
        options?: {
          heading?: string;
          reveal?: EditorRevealTarget;
          bookTarget?: BookOpenTarget;
          reviewOverlay?: EditorReviewOverlay | null;
          focusEditor?: boolean;
        },
      ) => {
        const { isDirty, currentFilePath, currentFileURI } = get();
        if (isDirty && currentFilePath) {
          console.warn('Unsaved changes in', currentFilePath);
        }

        try {
          const nextURI = pathToDocumentURI(path, get());
          const existing = get().documents.find((tab) =>
            matchesDocumentTab(tab, path, nextURI),
          );
          if (existing) {
            const pendingScrollHeading = options?.heading || null;
            const pendingRevealTarget = options?.reveal || null;
            const pendingBookTarget = options?.bookTarget || null;
            const currentReviewOverlay = options?.reviewOverlay || null;
            set((state) => {
              const nextDocuments = state.documents.map((tab) =>
                tab.id === existing.id
                  ? {
                      ...tab,
                      pendingScrollHeading,
                      pendingRevealTarget,
                      pendingBookTarget,
                    }
                  : tab,
              );
              const nextActive = nextDocuments.find(
                (tab) => tab.id === existing.id,
              );
              if (!nextActive) {
                return {};
              }
              return {
                ...patchDocumentsWithPinnedState(state, nextDocuments),
                activeTabId: existing.id,
                ...toActiveState(nextActive),
                currentReviewOverlay,
                editorFocusRequest: nextEditorFocusRequest(
                  state,
                  existing.id,
                  options?.focusEditor === true,
                ),
              };
            });
            return;
          }

          const settings = await window.electronAPI?.settings.get();
          const { editorRegistry } = await import('../services/editorRegistry');
          const editorId = editorRegistry.resolveEditorId(
            path,
            settings?.editor?.workbenchEditorAssociations || {},
          );

          const openableExtensions = settings?.editor?.openableExtensions || [];
          if (openableExtensions.length > 0) {
            const extension = path.substring(path.lastIndexOf('.'));
            if (!openableExtensions.includes(extension)) {
              console.warn('File extension not in openable list:', extension);
              return;
            }
          }

          let nextContent = '';
          const isBinaryPreview =
            editorId === 'image' || editorId === 'pdf' || editorId === 'book';
          if (!isBinaryPreview) {
            const result = await fileService.readFile(path);
            if (result.error) {
              console.error('Failed to read file:', result.error);
              return;
            }

            if (result.tooLarge) {
              console.warn('File too large to open');
              return;
            }

            nextContent = result.content || '';
          }

          const nextTab: DocumentTab = normalizeDocumentTab({
            id: createId('tab'),
            title: getFileTitle(path),
            uri: nextURI || undefined,
            filePath: path,
            editorId,
            content: nextContent,
            isDirty: false,
            resourceVersion: isBinaryPreview ? 0 : undefined,
            pendingScrollHeading: options?.heading || null,
            pendingRevealTarget: options?.reveal || null,
            pendingBookTarget: options?.bookTarget || null,
          });
          const nextChatPath = getDocumentChatPath(nextTab);

          set((state) => {
            // Auto-close Welcome tab when opening other tabs
            const filteredTabs = state.documents.filter(
              (tab) => tab.editorId !== 'welcome',
            );
            const nextTabs = [...filteredTabs, nextTab];
            return {
              ...patchDocumentsWithPinnedState(state, nextTabs),
              activeTabId: nextTab.id,
              ...toActiveState(nextTab),
              currentReviewOverlay: options?.reviewOverlay || null,
              editorFocusRequest: nextEditorFocusRequest(
                state,
                nextTab.id,
                options?.focusEditor === true,
              ),
            };
          });
          if (nextChatPath) {
            const chatState = getChatWorkspaceStore(_tabId).getState();
            chatState.showComposer();
            chatState.selectChatConversation(nextChatPath);
          }
          void syncWatches();
        } catch (e) {
          console.error('Error opening file:', e);
        }
      },

      openWelcomeTab: () => {
        set((state) => {
          const existing = state.documents.find(
            (tab) => tab.editorId === 'welcome',
          );
          const tab: DocumentTab = existing || {
            id: createId('tab'),
            title: rendererI18n.t('shell:tab.welcome'),
            editorId: 'welcome',
            content: '',
            isDirty: false,
            pendingScrollHeading: null,
          };
          const nextTabs = existing
            ? state.documents
            : [tab, ...state.documents];
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            activeTabId: tab.id,
            ...toActiveState(tab),
          };
        });
      },

      openModelsTab: () => {
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            'models',
            () => ({
              id: createId('tab'),
              title: rendererI18n.t('shell:tab.models'),
              editorId: 'models',
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openOpenBrainSettingsTab: () => {
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            'openbrain-settings',
            () => ({
              id: createId('tab'),
              title: rendererI18n.t('shell:tab.openBrainSettings'),
              editorId: 'openbrain-settings',
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openDesktopSettingsTab: () => {
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            'desktop-settings',
            () => ({
              id: createId('tab'),
              title: rendererI18n.t('shell:tab.desktopSettings'),
              editorId: 'desktop-settings',
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openDashboardTab: () => {
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            'dashboard',
            () => ({
              id: createId('tab'),
              title: rendererI18n.t('shell:tab.dashboard'),
              editorId: 'dashboard',
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openCronTaskTab: (id: string, title?: string) => {
        const taskID = (id || '').trim();
        if (!taskID) {
          return;
        }
        const editorId = `cron-task:${taskID}`;
        const tabTitle = (title || '').trim() || 'Cron Task';
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            editorId,
            () => ({
              id: createId('tab'),
              title: tabTitle,
              editorId,
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            const nextDocuments = state.documents.map((tab) =>
              tab.id === result.tab.id && tab.title !== tabTitle
                ? { ...tab, title: tabTitle }
                : tab,
            );
            const nextTab = nextDocuments.find((tab) => tab.id === result.tab.id) || result.tab;
            return {
              ...patchDocumentsWithPinnedState(state, nextDocuments),
              activeTabId: nextTab.id,
              ...toActiveState(nextTab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openMarketplaceTab: () => {
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            'marketplace',
            () => ({
              id: createId('tab'),
              title: rendererI18n.t('shell:tab.marketplace'),
              editorId: 'marketplace',
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      openOrgMarketplaceTab: (orgID: string, orgName: string) => {
        const normalizedOrgID = (orgID || '').trim();
        const title = (orgName || normalizedOrgID || 'Organization').trim();
        if (!normalizedOrgID) {
          return;
        }
        set((state) => {
          const result = upsertSingletonEditorTab(
            getOpenDocuments(state),
            `marketplace:${normalizedOrgID}`,
            () => ({
              id: createId('tab'),
              title,
              editorId: `marketplace:${normalizedOrgID}`,
              content: '',
              isDirty: false,
              pendingScrollHeading: null,
            }),
            { removeWelcome: true },
          );
          if (result.existed) {
            return {
              activeTabId: result.tab.id,
              ...toActiveState(result.tab),
            };
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
      },

      listMarketplaceItems: async (options?: {
        force?: boolean;
        orgID?: string | null;
      }) => {
        return marketplaceService.listItems(options);
      },

      refreshMarketplaceItems: async (options?: { orgID?: string | null }) => {
        return marketplaceService.refreshForOrg(options?.orgID || null);
      },

      listMarketplaceOrgs: async () => {
        return marketplaceService.listOrgs();
      },

      getMarketplaceState: async () => {
        return marketplaceService.getState();
      },

      installMarketplaceItem: async (
        kind: 'agent' | 'skill' | 'tool',
        id: string,
        orgID?: string | null,
      ) => {
        const result = await marketplaceService.installItem(kind, id, orgID);
        if (result.success) {
          await refreshNodesCache(true);
          const state = get();
          const baseDir = (state.baseDir || '').trim();
          const rootDir =
            kind === 'agent'
              ? (state.agentsRootDir || '').trim()
              : baseDir
                ? `${baseDir}/${kind === 'skill' ? 'skills' : 'tools'}`
                : '';
          if (rootDir) {
            await get().loadDirectory(rootDir);
          }
        }
        return result;
      },

      updateMarketplaceItem: async (
        kind: 'agent' | 'skill' | 'tool',
        id: string,
        orgID?: string | null,
      ) => {
        const result = await marketplaceService.updateItem(kind, id, orgID);
        if (result.success) {
          await refreshNodesCache(true);
          const state = get();
          const baseDir = (state.baseDir || '').trim();
          const rootDir =
            kind === 'agent'
              ? (state.agentsRootDir || '').trim()
              : baseDir
                ? `${baseDir}/${kind === 'skill' ? 'skills' : 'tools'}`
                : '';
          if (rootDir) {
            await get().loadDirectory(rootDir);
          }
        }
        return result;
      },

      openUntitledTab: () => {
        set((state) => {
          const tab: DocumentTab = {
            id: createId('tab'),
            title: NEW_TAB_TITLE,
            editorId: 'markdown',
            content: '',
            isDirty: true, // Mark as dirty since it's unsaved
            pendingScrollHeading: null,
          };
          // Auto-close Welcome tab when opening other tabs
          const filteredTabs = state.documents.filter(
            (t) => t.editorId !== 'welcome',
          );
          const nextTabs = [...filteredTabs, tab];
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            activeTabId: tab.id,
            ...toActiveState(tab),
            editorFocusRequest: nextEditorFocusRequest(state, tab.id, true),
          };
        });
      },

      openThreadTab: async (path: string, title?: string, content?: string) => {
        const normalizedPath = normalizePosixPath((path || '').trim());
        if (!normalizedPath) {
          return null;
        }
        const uri = pathToDocumentURI(normalizedPath, get());
        const existing = get().documents.find((tab) =>
          matchesDocumentTab(tab, normalizedPath, uri),
        );
        let resolvedContent = existing?.content;
        if (typeof content === 'string') {
          resolvedContent = content;
        } else if (!existing) {
          const result = await fileService.readFile(normalizedPath);
          if (result.error || result.tooLarge) {
            console.error(
              'Failed to read conversation file:',
              result.error || 'file too large',
            );
            return null;
          }
          resolvedContent = result.content || '';
        }
        const tabId = get().ensureThreadTab(
          normalizedPath,
          title,
          resolvedContent || '',
        );
        if (tabId) {
          const state = get();
          const pinnedTab = state.pinnedTabId
            ? state.documents.find((tab) => tab.id === state.pinnedTabId) ||
              null
            : null;
          if (isConversationDocument(pinnedTab)) {
            if (state.pinnedTabId !== tabId) {
              state.setPinnedTab(tabId);
            }
            state.activateLastNonConversationTab();
          } else {
            state.setActiveConversationTab(tabId);
          }
        }
        return tabId;
      },

      ensureThreadTab: (path: string, title?: string, content = '') => {
        if (!path) {
          return null;
        }
        let ensuredTabId: string | null = null;
        set((state) => {
          const nextURI = pathToDocumentURI(path, state);
          const existing = state.documents.find((tab) =>
            matchesDocumentTab(tab, path, nextURI),
          );
          if (existing) {
            const shouldUpdateTitle = Boolean(
              title && title !== existing.title,
            );
            const shouldUpdateContent =
              typeof content === 'string' && content !== existing.content;
            const nextDocuments =
              shouldUpdateTitle || shouldUpdateContent
                ? state.documents.map((tab) =>
                    tab.id === existing.id
                      ? {
                          ...tab,
                          ...(shouldUpdateTitle ? { title: title! } : {}),
                          ...(shouldUpdateContent
                            ? { content, isDirty: false, missing: false }
                            : {}),
                        }
                      : tab,
                  )
                : state.documents;
            ensuredTabId = existing.id;
            return patchDocumentsWithPinnedState(state, nextDocuments);
          }
          const nextTab = normalizeDocumentTab({
            id: createId('tab'),
            title: title || getFileTitle(path),
            uri: nextURI || undefined,
            filePath: path,
            editorId: 'markdown',
            content,
            isDirty: false,
            pendingScrollHeading: null,
          });
          ensuredTabId = nextTab.id;
          return patchDocumentsWithPinnedState(state, [
            ...state.documents,
            nextTab,
          ]);
        });
        void syncWatches();
        return ensuredTabId;
      },

      restoreChatTabsSession: async (entries, selectedPath) => {
        const normalizedEntries: Array<{
          path: string;
          title: string;
          content: string;
        }> = [];
        for (const entry of entries) {
          const path = normalizeWorkspaceChatPath(entry?.path);
          if (!path || normalizedEntries.some((item) => item.path === path)) {
            continue;
          }
          const result = await fileService.readFile(path);
          if (result.error || result.tooLarge) {
            continue;
          }
          const content =
            typeof result.content === 'string' ? result.content : '';
          if (!isConversationMarkdownContent(content)) {
            continue;
          }
          normalizedEntries.push({
            path,
            title: (entry?.title || '').trim() || getFileTitle(path),
            content,
          });
        }

        if (normalizedEntries.length === 0) {
          return {
            restoredPaths: [],
            selectedPath: null,
          };
        }

        const preferredPath = normalizeWorkspaceChatPath(selectedPath);
        let nextSelectedPath: string | null = null;

        set((state) => {
          const restoredPaths = normalizedEntries.map((entry) => entry.path);
          const nextDocs = state.documents.filter(
            (tab) =>
              !restoredPaths.includes(normalizeWorkspaceChatPath(tab.filePath)),
          );

          const restoredTabs = normalizedEntries.map((entry) => {
            const nextURI = pathToDocumentURI(entry.path, state);
            const existing = state.documents.find((tab) =>
              matchesDocumentTab(tab, entry.path, nextURI),
            );
            if (existing) {
              return normalizeDocumentTab({
                ...existing,
                uri: nextURI || existing.uri,
                filePath: entry.path,
                title: entry.title,
                editorId: 'markdown' as const,
                content: entry.content,
                isDirty: false,
                missing: false,
              });
            }
            const restoredTab = normalizeDocumentTab({
              id: createId('tab'),
              title: entry.title,
              uri: nextURI || undefined,
              filePath: entry.path,
              editorId: 'markdown',
              content: entry.content,
              isDirty: false,
              missing: false,
              pendingScrollHeading: null,
            });
            return restoredTab;
          });

          const mergedTabs = [...nextDocs, ...restoredTabs];
          nextSelectedPath = restoredPaths.includes(preferredPath)
            ? preferredPath
            : restoredPaths[0] || null;

          return {
            ...patchDocumentsWithPinnedState(state, mergedTabs),
          };
        });

        void syncWatches();
        return {
          restoredPaths: normalizedEntries.map((entry) => entry.path),
          selectedPath: nextSelectedPath,
        };
      },

      moveChatTabToLast: (path: string) => {
        if (!path) return;
        set((state) => {
          const targetURI = pathToDocumentURI(path, state);
          const targetTab = state.documents.find((t) =>
            matchesDocumentTab(t, path, targetURI),
          );
          if (!isConversationDocument(targetTab)) return {};
          const rest = state.documents.filter(
            (t) => !matchesDocumentTab(t, path, targetURI),
          );
          let lastChatIdx = -1;
          for (let i = rest.length - 1; i >= 0; i -= 1) {
            if (isConversationDocument(rest[i])) {
              lastChatIdx = i;
              break;
            }
          }
          if (lastChatIdx === -1) return {};
          const insertAt = lastChatIdx + 1;
          const nextTabs = [
            ...rest.slice(0, insertAt),
            targetTab,
            ...rest.slice(insertAt),
          ];
          return patchDocumentsWithPinnedState(state, nextTabs);
        });
      },

      appendToTab: (path: string, text: string) => {
        if (!path || !text) {
          return;
        }
        set((state) => {
          const targetURI = pathToDocumentURI(path, state);
          const target = state.documents.find((tab) =>
            matchesDocumentTab(tab, path, targetURI),
          );
          if (!target) {
            return {};
          }
          let nextText = text;
          if (
            isConversationDocument(target) &&
            isThreadRoundHeaderChunk(text)
          ) {
            const current = target.content;
            const separatorPrefix = getThreadRoundSeparatorPrefix(current);
            if (separatorPrefix) {
              nextText = `${separatorPrefix}${text}`;
            }
          }
          const nextContent = `${target.content}${nextText}`;
          const nextTabs = state.documents.map((tab) =>
            tab.id === target.id
              ? normalizeDocumentTab({
                  ...tab,
                  content: nextContent,
                  isDirty: false,
                })
              : tab,
          );
          if (state.activeTabId !== target.id) {
            return patchDocumentsWithPinnedState(state, nextTabs);
          }
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            currentFileURI: target.uri || targetURI || null,
            currentFilePath: path,
            fileContent: nextContent,
            isDirty: false,
            editorId: target.editorId,
            pendingScrollHeading: target.pendingScrollHeading,
            pendingRevealTarget: target.pendingRevealTarget || null,
          };
        });
      },

      retargetActiveBlankTab: (
        newPath: string,
        title?: string,
        content?: string,
      ) => {
        const to = (newPath || '').trim();
        if (!to) {
          return false;
        }
        const nextContent = typeof content === 'string' ? content : '';

        let retargeted = false;
        set((state) => {
          const toURI = pathToDocumentURI(to, state);
          const result = retargetActiveBlankNewTab(
            getOpenDocuments(state),
            state.activeTabId,
            (tab) => ({
              ...tab,
              uri: toURI || undefined,
              filePath: to,
              title: title || getFileTitle(to),
              content: nextContent,
              isDirty: false,
              missing: false,
            }),
          );
          retargeted = result.retargeted;
          if (!result.retargeted || !result.tab) {
            return {};
          }
          return {
            ...patchDocumentsWithPinnedState(state, result.tabs),
            activeTabId: result.tab.id,
            ...toActiveState(result.tab),
          };
        });
        if (retargeted) {
          void syncWatches();
        }
        return retargeted;
      },

      retargetTabPath: (oldPath: string, newPath: string, title?: string) => {
        const from = (oldPath || '').trim();
        const to = (newPath || '').trim();
        if (!from || !to || from === to) {
          return;
        }
        get().retargetMovedTabs([{ oldPath: from, newPath: to }]);
        if (title) {
          set((state) => {
            const toURI = pathToDocumentURI(to, state);
            const target = getOpenDocuments(state).find((tab) =>
              matchesDocumentTab(tab, to, toURI),
            );
            if (!target || target.title === title) {
              return {};
            }
            const nextTabs = getOpenDocuments(state).map((tab) =>
              tab.id === target.id ? { ...tab, title } : tab,
            );
            const nextActive =
              state.activeTabId === target.id
                ? nextTabs.find((tab) => tab.id === target.id)
                : undefined;
            return nextActive
              ? {
                  ...patchDocumentsWithPinnedState(state, nextTabs),
                  ...toActiveState(nextActive),
                }
              : patchDocumentsWithPinnedState(state, nextTabs);
          });
        }
      },

      retargetMovedTabs: (ops) => {
        const normalizedOps = ops
          .map((op) => ({
            oldPath: (op.oldPath || '').trim(),
            newPath: (op.newPath || '').trim(),
          }))
          .filter(
            (op) => !!op.oldPath && !!op.newPath && op.oldPath !== op.newPath,
          )
          .sort((a, b) => b.oldPath.length - a.oldPath.length);

        if (normalizedOps.length === 0) {
          return;
        }

        set((state) => {
          let changed = false;
          const nextTabs = getOpenDocuments(state).map((tab) => {
            const nextPath = replaceMovedPath(tab.filePath, normalizedOps);
            if (!nextPath || nextPath === tab.filePath) {
              return tab;
            }
            changed = true;
            return {
              ...tab,
              uri: pathToDocumentURI(nextPath, state) || undefined,
              filePath: nextPath,
              title: getFileTitle(nextPath),
              missing: false,
            };
          });

          if (!changed) {
            return {};
          }

          const activeTab = state.activeTabId
            ? nextTabs.find((tab) => tab.id === state.activeTabId)
            : undefined;
          if (!activeTab) {
            return patchDocumentsWithPinnedState(state, nextTabs);
          }
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            ...toActiveState(activeTab),
          };
        });
        void syncWatches();
      },

      setActiveTab: (tabId: string) => {
        const nextTab = get().documents.find((tab) => tab.id === tabId);
        if (!nextTab) {
          return;
        }
        set({
          activeTabId: tabId,
          ...toActiveState(nextTab),
        });
      },

      setActiveConversationTab: (tabId: string) => {
        const nextTab = get().documents.find((tab) => tab.id === tabId);
        if (!nextTab || !isConversationDocument(nextTab)) {
          return;
        }
        const state = get();
        const pinnedTab = state.pinnedTabId
          ? state.documents.find((tab) => tab.id === state.pinnedTabId) || null
          : null;
        if (isConversationDocument(pinnedTab) && state.pinnedTabId !== tabId) {
          state.setPinnedTab(tabId);
          return;
        }
        set({
          activeTabId: tabId,
          ...toActiveState(nextTab),
        });
      },

      activateConversationPath: (path: string) => {
        void get().openThreadTab(path);
      },

      activateLastPrimaryTab: (options) => {
        const state = get();
        const excludeTabId = (options?.excludeTabId || '').trim();
        const visibleTabs = getEditorDocuments(state.documents).filter(
          (tab) => {
            if (excludeTabId && tab.id === excludeTabId) {
              return false;
            }
            return true;
          },
        );
        const fallback =
          visibleTabs.length > 0 ? visibleTabs[visibleTabs.length - 1] : null;
        if (!fallback) {
          set({
            activeTabId: undefined,
            currentFileURI: null,
            currentFilePath: null,
            fileContent: '',
            isDirty: false,
            pendingScrollHeading: null,
            pendingRevealTarget: null,
            pendingBookTarget: null,
            currentReviewOverlay: null,
            editorId: null,
          });
          return false;
        }
        set({
          activeTabId: fallback.id,
          ...toActiveState(fallback),
        });
        return true;
      },

      activateLastNonConversationTab: () => {
        return get().activateLastPrimaryTab();
      },

      setPinnedTab: (tabId) => {
        const normalizedId = (tabId || '').trim();
        if (!normalizedId) {
          set((state) =>
            state.pinnedTabId == null ? {} : { pinnedTabId: undefined },
          );
          return;
        }
        const target =
          get().documents.find((tab) => tab.id === normalizedId) || null;
        if (!isPinnableEditorTab(target)) {
          return;
        }
        set((state) =>
          state.pinnedTabId === normalizedId
            ? {}
            : { pinnedTabId: normalizedId },
        );
      },

      togglePinnedTab: (tabId) => {
        const normalizedId = (tabId || '').trim();
        if (!normalizedId) {
          return;
        }
        const state = get();
        if (state.pinnedTabId === normalizedId) {
          set({ pinnedTabId: undefined });
          return;
        }
        const target =
          state.documents.find((tab) => tab.id === normalizedId) || null;
        if (!isPinnableEditorTab(target)) {
          return;
        }
        set({ pinnedTabId: normalizedId });
        if (target) {
          state.activateLastPrimaryTab({
            excludeTabId: normalizedId,
          });
        }
      },

      clearPinnedTab: () => {
        set((state) =>
          state.pinnedTabId == null ? {} : { pinnedTabId: undefined },
        );
      },

      closeTab: (tabId: string) => {
        const state = get();
        const tab = getOpenDocuments(state).find((item) => item.id === tabId);
        if (!tab) {
          return;
        }
        if (tab.isDirty) {
          set({
            pendingDirtyTabClose: {
              tabId: tab.id,
              title: tab.title,
            },
          });
          return;
        }
        closeTabNow(tabId);
      },

      dismissPendingDirtyTabClose: () => {
        set({ pendingDirtyTabClose: null });
      },

      confirmPendingDirtyTabClose: () => {
        const pending = get().pendingDirtyTabClose;
        if (!pending) {
          return;
        }
        closeTabNow(pending.tabId);
      },

      closeOtherTabs: (tabId: string) => {
        for (const tab of getOpenDocuments(get())) {
          if (tab.id !== tabId) {
            discardAutoSave(tab.filePath);
            discardBackup(tab.id);
            if (!tab.filePath && window.electronAPI?.backup) {
              window.electronAPI.backup.delete(tab.id).catch(() => {});
            }
          }
        }
        set((state) => {
          const keep = getOpenDocuments(state).find((tab) => tab.id === tabId);
          if (!keep) {
            return {};
          }
          return {
            ...patchDocumentsWithPinnedState(state, [keep]),
            activeTabId: keep.id,
            pendingDirtyTabClose: null,
            ...toActiveState(keep),
          };
        });
        void syncWatches();
      },

      closeAllTabs: () => {
        for (const tab of getOpenDocuments(get())) {
          discardAutoSave(tab.filePath);
          discardBackup(tab.id);
          if (!tab.filePath && window.electronAPI?.backup) {
            window.electronAPI.backup.delete(tab.id).catch(() => {});
          }
        }
        set({
          documents: [],
          activeTabId: undefined,
          pinnedTabId: undefined,
          currentFileURI: null,
          currentFilePath: null,
          fileContent: '',
          isDirty: false,
          pendingScrollHeading: null,
          pendingRevealTarget: null,
          pendingBookTarget: null,
          editorId: null,
          pendingDirtyTabClose: null,
        });
        void syncWatches();
      },

      saveFile: async () => {
        const snapshot = get();
        const { activeTabId, fileContent, currentDir } = snapshot;
        const activeTab = getOpenDocuments(snapshot).find(
          (t) => t.id === activeTabId,
        );
        if (!activeTab) return;

        let targetPath = activeTab.filePath;

        // Untitled tab: show save dialog to get file path
        if (!targetPath) {
          const { requestSaveFileDialog } =
            await import('../components/FileDialog/saveFileDialogBridge');
          const defaultName = activeTab.title.endsWith('.md')
            ? activeTab.title
            : `${activeTab.title}.md`;
          const result = await requestSaveFileDialog({
            defaultDir: currentDir,
            defaultFileName: defaultName,
          });
          if (!result) return;
          targetPath = result;
        }

        // At this point targetPath is guaranteed to be a string
        if (!targetPath) return;

        const saved = await persistTabContent(
          activeTab.id,
          targetPath,
          fileContent,
        );
        if (!saved) {
          return;
        }

        if (!activeTab.filePath && window.electronAPI?.backup) {
          window.electronAPI.backup.delete(activeTab.id).catch(() => {});
        }
      },

      saveTabByPath: async (path: string) => {
        const normalizedPath = normalizePosixPath((path || '').trim());
        if (!normalizedPath) {
          return false;
        }
        const targetTab = getOpenDocuments(get()).find(
          (tab) =>
            normalizePosixPath((tab.filePath || '').trim()) === normalizedPath,
        );
        if (!targetTab || !targetTab.filePath || !targetTab.isDirty) {
          return true;
        }
        return persistTabContent(
          targetTab.id,
          targetTab.filePath,
          targetTab.content,
        );
      },

      flushDirtyTabs: async () => {
        const state = get();
        const dirtyTabs = getOpenDocuments(state).filter((tab) => tab.isDirty);
        let saved = 0;
        const failed: string[] = [];

        for (const tab of dirtyTabs) {
          if (tab.filePath) {
            const persisted = await persistTabContent(
              tab.id,
              tab.filePath,
              tab.content,
            );
            if (persisted) {
              saved += 1;
            } else {
              failed.push(tab.filePath);
            }
            continue;
          }

          if (!window.electronAPI?.backup) {
            continue;
          }

          discardBackup(tab.id);
          try {
            await window.electronAPI.backup.save({
              id: tab.id,
              title: tab.title,
              content: tab.content,
              editorId: tab.editorId,
            });
          } catch (error) {
            console.error(
              'Failed to back up untitled tab before window close:',
              error,
            );
          }
        }

        return { saved, failed };
      },

      setFileContent: (content: string) => {
        const snapshot = get();
        const { fileContent, activeTabId } = snapshot;
        if (content !== fileContent) {
          set((state) => {
            if (!state.activeTabId) {
              return { fileContent: content, isDirty: true };
            }
            const nextTabs = getOpenDocuments(state).map((tab) =>
              tab.id === state.activeTabId
                ? normalizeDocumentTab({ ...tab, content, isDirty: true })
                : tab,
            );
            return {
              fileContent: content,
              isDirty: true,
              ...patchDocumentsWithPinnedState(state, nextTabs),
            };
          });

          const activeTab = getOpenDocuments(snapshot).find(
            (t) => t.id === activeTabId,
          );
          if (activeTab?.filePath) {
            // Has path: auto-save to file
            scheduleAutoSave(
              activeTab.filePath,
              activeTab.uri || pathToDocumentURI(activeTab.filePath, get()),
              content,
            );
          } else if (activeTab) {
            // Untitled: backup to local storage
            scheduleBackup({ ...activeTab, content });
          }
        }
      },

      setTabContent: (tabId: string, content: string) => {
        const target = getOpenDocuments(get()).find((tab) => tab.id === tabId);
        if (!target || target.content === content) {
          return;
        }

        set((state) => {
          const nextTabs = getOpenDocuments(state).map((tab) =>
            tab.id === tabId
              ? normalizeDocumentTab({ ...tab, content, isDirty: true })
              : tab,
          );
          if (state.activeTabId !== tabId) {
            return patchDocumentsWithPinnedState(state, nextTabs);
          }
          return {
            ...patchDocumentsWithPinnedState(state, nextTabs),
            fileContent: content,
            isDirty: true,
          };
        });

        if (target.filePath) {
          scheduleAutoSave(
            target.filePath,
            target.uri || pathToDocumentURI(target.filePath, get()),
            content,
          );
        } else {
          scheduleBackup({ ...target, content });
        }
      },

      setPendingScrollHeading: (heading: string | null) => {
        set((state) => {
          if (!state.activeTabId) {
            return { pendingScrollHeading: heading };
          }
          const nextTabs = getOpenDocuments(state).map((tab) =>
            tab.id === state.activeTabId
              ? { ...tab, pendingScrollHeading: heading }
              : tab,
          );
          return {
            pendingScrollHeading: heading,
            ...patchDocumentsWithPinnedState(state, nextTabs),
          };
        });
      },

      setPendingRevealTarget: (reveal: EditorRevealTarget | null) => {
        set((state) => {
          if (!state.activeTabId) {
            return { pendingRevealTarget: reveal };
          }
          const nextTabs = getOpenDocuments(state).map((tab) =>
            tab.id === state.activeTabId
              ? { ...tab, pendingRevealTarget: reveal }
              : tab,
          );
          return {
            pendingRevealTarget: reveal,
            ...patchDocumentsWithPinnedState(state, nextTabs),
          };
        });
      },

      setPendingBookTarget: (target: BookOpenTarget | null) => {
        set((state) => {
          if (!state.activeTabId) {
            return { pendingBookTarget: target };
          }
          const nextTabs = getOpenDocuments(state).map((tab) =>
            tab.id === state.activeTabId
              ? { ...tab, pendingBookTarget: target }
              : tab,
          );
          return {
            pendingBookTarget: target,
            ...patchDocumentsWithPinnedState(state, nextTabs),
          };
        });
      },

      setCurrentReviewOverlay: (overlay: EditorReviewOverlay | null) => {
        set({ currentReviewOverlay: overlay });
      },

      setEditorFocused: (focused: boolean) => {
        set((state) =>
          state.editorFocused === focused ? {} : { editorFocused: focused },
        );
      },

      consumeEditorFocusRequest: (tabId: string) => {
        const id = (tabId || '').trim();
        const request = get().editorFocusRequest;
        if (!id || request?.tabId !== id) {
          return false;
        }
        set({ editorFocusRequest: null });
        return true;
      },

      requestEditorCompletion: async (request: EditorCompletionRequest) => {
        if (get().connectionState !== 'connected') {
          return null;
        }
        const normalizedRequest: EditorCompletionRequest = {
          ...request,
          requestID: (request.requestID || '').trim(),
          agentID: (request.agentID || '').trim() || null,
          modelKey: (request.modelKey || '').trim(),
        };
        if (!normalizedRequest.requestID) {
          return null;
        }
        try {
          return await connection.request<EditorCompletionResult>(
            'editor/completion',
            normalizedRequest,
          );
        } catch (error) {
          const message = (error as Error).message || '';
          if (
            !message.includes('context canceled') &&
            !message.includes('Request timeout') &&
            !message.includes('websocket')
          ) {
            console.warn('[editorCompletion] request failed:', message);
          }
          return null;
        }
      },

      cancelEditorCompletion: async (requestID: string) => {
        const normalizedRequestID = (requestID || '').trim();
        if (!normalizedRequestID || get().connectionState !== 'connected') {
          return;
        }
        try {
          await connection.request('editor/completion/cancel', {
            requestID: normalizedRequestID,
          });
        } catch {
          // Best-effort only.
        }
      },

      requestEditorRandomID: async () => {
        if (get().connectionState !== 'connected') {
          return null;
        }
        try {
          const result = await connection.request<EditorRandomIDResult>(
            'editor/randomID',
            {},
          );
          return (result?.id || '').trim() || null;
        } catch (error) {
          console.warn(
            '[editorRandomID] request failed:',
            (error as Error).message || error,
          );
          return null;
        }
      },

      requestEditorBlur: () => {
        set((state) => ({
          editorBlurRequestSeq: state.editorBlurRequestSeq + 1,
        }));
      },

      refreshMessenger: async () => {
        if (get().connectionState !== 'connected') {
          return;
        }
        try {
          const result = await messengerService.list({ limit: 100 });
          useMessengerStore.getState().setList(
            result.channels || [],
            result.messages || [],
          );
        } catch (error) {
          console.warn('[messenger] refresh failed:', (error as Error).message || error);
        }
      },

      loadMessengerChannel: async (channelID: string) => {
        const normalizedChannelID = (channelID || '').trim();
        if (!normalizedChannelID || get().connectionState !== 'connected') {
          return;
        }
        try {
          const result = await messengerService.channel({
            channelID: normalizedChannelID,
            limit: 500,
          });
          const returnedChannelID = (result.channelID || normalizedChannelID).trim();
          if (returnedChannelID) {
            useMessengerStore.getState().setChannelMessages(
              returnedChannelID,
              result.messages || [],
            );
            await messengerService.markRead({ channelID: returnedChannelID });
          }
        } catch (error) {
          console.warn('[messenger] load channel failed:', (error as Error).message || error);
          throw error;
        }
      },

      replyMessenger: async (input: MessengerReplyInput) => {
        if (get().connectionState !== 'connected') {
          throw new Error('Not connected');
        }
        const result = await messengerService.reply(await resolveMessengerReplyInput(input, _tabId));
        const records = result.resolved
          ? [result.resolved, result.record]
          : [result.record];
        const messengerState = useMessengerStore.getState();
        for (const record of records) {
          messengerState.upsertRecord(record);
        }
        getChatWorkspaceStore(_tabId).getState().upsertThreadMessageRecords(records);
      },

      markMessengerRead: async (channelIDs?: string[]) => {
        if (get().connectionState !== 'connected') {
          return;
        }
        const ids = Array.from(new Set((channelIDs || [])
          .map((channelID) => (channelID || '').trim())
          .filter(Boolean)));
        if (ids.length === 0) {
          return;
        }
        await Promise.all(ids.map((channelID) => messengerService.markRead({ channelID })));
      },

      archiveMessengerAgentPendingRequests: async (agentID: string) => {
        const normalizedAgentID = (agentID || '').trim();
        if (!normalizedAgentID || get().connectionState !== 'connected') {
          return 0;
        }
        const result = await messengerService.archive({
          agentID: normalizedAgentID,
          pendingRequestsOnly: true,
        });
        useMessengerStore.getState().archiveAgentPendingRequests(normalizedAgentID);
        await get().refreshMessenger().catch((error) => {
          console.warn('Failed to refresh messenger after clearing pending requests:', error);
        });
        return result?.archived ?? 0;
      },

      archiveMessengerAgentMessages: async (agentID: string) => {
        const normalizedAgentID = (agentID || '').trim();
        if (!normalizedAgentID || get().connectionState !== 'connected') {
          return 0;
        }
        const result = await messengerService.archive({
          agentID: normalizedAgentID,
        });
        useMessengerStore.getState().archiveAgentMessages(normalizedAgentID);
        await get().refreshMessenger().catch((error) => {
          console.warn('Failed to refresh messenger after clearing agent messages:', error);
        });
        return result?.archived ?? 0;
      },

      archiveMessengerChannel: async (channelID: string) => {
        const normalizedChannelID = (channelID || '').trim();
        if (!normalizedChannelID || get().connectionState !== 'connected') {
          return;
        }
        await messengerService.archive({ channelID: normalizedChannelID });
        useMessengerStore.getState().archiveChannel(normalizedChannelID);
      },

      createFile: async (path: string) => {
        try {
          const result = await fileService.writeFile(path, '', {
            create: true,
            overwrite: false,
          });

          if (result.error) {
            console.error('Failed to create file:', result.error);
            return { success: false, error: result.error };
          }

          const dir = path.substring(0, path.lastIndexOf('/'));
          await get().loadDirectory(dir);
          return { success: true };
        } catch (e) {
          console.error('Error creating file:', e);
          return { success: false, error: (e as Error).message };
        }
      },

      createFolder: async (path: string) => {
        try {
          const result = await fileService.mkdir(path);
          if (result.error) {
            console.error('Failed to create folder:', result.error);
            return { success: false, error: result.error };
          }

          const dir = path.substring(0, path.lastIndexOf('/'));
          await get().loadDirectory(dir);
          return { success: true };
        } catch (e) {
          console.error('Error creating folder:', e);
          return { success: false, error: (e as Error).message };
        }
      },

      deleteEntry: async (path: string, isDir: boolean, options) => {
        try {
          const useTrash = options?.useTrash ?? false;
          const recursive = options?.recursive ?? isDir;
          const result = await fileService.delete(path, recursive, useTrash);
          if (result.error) {
            console.error('Failed to delete entry:', result.error);
            return { success: false, error: result.error };
          }

          // If we are about to close tabs due to deletion, discard any pending auto-saves.
          const isUnderDeletedDir = (tabPath?: string) =>
            !!tabPath &&
            (tabPath === path || (isDir && tabPath.startsWith(`${path}/`)));
          for (const tab of getOpenDocuments(get())) {
            if (isUnderDeletedDir(tab.filePath)) {
              discardAutoSave(tab.filePath);
            }
          }
          const parentDir = path.substring(0, path.lastIndexOf('/'));
          const deletedName = path.substring(path.lastIndexOf('/') + 1);

          set((state) => {
            const affectedTabs = getOpenDocuments(state).filter((tab) =>
              isUnderDeletedDir(tab.filePath),
            );
            if (affectedTabs.length === 0) {
              // Still update expanded/cached dirs if needed.
              const nextExpanded = new Set(state.expandedDirs);
              if (isDir) {
                for (const dir of nextExpanded) {
                  if (dir === path || dir.startsWith(`${path}/`)) {
                    nextExpanded.delete(dir);
                  }
                }
              } else {
                nextExpanded.delete(path);
              }

              const nextDirEntries = new Map(state.dirEntries);
              if (isDir) {
                for (const dir of nextDirEntries.keys()) {
                  if (dir === path || dir.startsWith(`${path}/`)) {
                    nextDirEntries.delete(dir);
                  }
                }
              }
              const parentEntries = nextDirEntries.get(parentDir);
              if (parentEntries) {
                nextDirEntries.set(
                  parentDir,
                  parentEntries.filter((entry) => entry.name !== deletedName),
                );
              }

              return { expandedDirs: nextExpanded, dirEntries: nextDirEntries };
            }
            const nextTabs = getOpenDocuments(state).filter(
              (tab) => !isUnderDeletedDir(tab.filePath),
            );
            const activeTabRemoved =
              state.activeTabId &&
              affectedTabs.some((tab) => tab.id === state.activeTabId);

            const nextExpanded = new Set(state.expandedDirs);
            if (isDir) {
              for (const dir of nextExpanded) {
                if (dir === path || dir.startsWith(`${path}/`)) {
                  nextExpanded.delete(dir);
                }
              }
            } else {
              nextExpanded.delete(path);
            }

            const nextDirEntries = new Map(state.dirEntries);
            if (isDir) {
              for (const dir of nextDirEntries.keys()) {
                if (dir === path || dir.startsWith(`${path}/`)) {
                  nextDirEntries.delete(dir);
                }
              }
            }
            const parentEntries = nextDirEntries.get(parentDir);
            if (parentEntries) {
              nextDirEntries.set(
                parentDir,
                parentEntries.filter((entry) => entry.name !== deletedName),
              );
            }

            if (!activeTabRemoved) {
              return {
                ...patchDocumentsWithPinnedState(state, nextTabs),
                expandedDirs: nextExpanded,
                dirEntries: nextDirEntries,
              };
            }
            const nextActiveTab = nextTabs[0];
            if (!nextActiveTab) {
              return {
                tabs: [],
                activeTabId: undefined,
                pinnedTabId: undefined,
                currentFileURI: null,
                currentFilePath: null,
                fileContent: '',
                isDirty: false,
                pendingScrollHeading: null,
                pendingRevealTarget: null,
                pendingBookTarget: null,
                editorId: null,
                expandedDirs: nextExpanded,
                dirEntries: nextDirEntries,
              };
            }
            return {
              ...patchDocumentsWithPinnedState(state, nextTabs),
              activeTabId: nextActiveTab.id,
              ...toActiveState(nextActiveTab),
              expandedDirs: nextExpanded,
              dirEntries: nextDirEntries,
            };
          });

          await get().loadDirectory(parentDir);
          return { success: true };
        } catch (e) {
          console.error('Error deleting entry:', e);
          return { success: false, error: (e as Error).message };
        }
      },

      moveEntries: async (ops) => {
        const normalizedOps = ops
          .map((op) => ({
            oldPath: (op.oldPath || '').trim(),
            newPath: (op.newPath || '').trim(),
          }))
          .filter(
            (op) => !!op.oldPath && !!op.newPath && op.oldPath !== op.newPath,
          )
          .sort((a, b) => b.oldPath.length - a.oldPath.length);

        if (normalizedOps.length === 0) {
          return { success: true };
        }

        try {
          const remoteSession = get().remoteSession;
          for (const { oldPath, newPath } of normalizedOps) {
            await fileService.rename(oldPath, newPath);
          }

          set((state) => {
            for (const tab of getOpenDocuments(state)) {
              const tabPath = tab.filePath || '';
              for (const { oldPath } of normalizedOps) {
                if (isSameOrChildPath(tabPath, oldPath)) {
                  discardAutoSave(tabPath);
                  break;
                }
              }
            }

            const nextExpanded = new Set<string>();
            for (const dir of state.expandedDirs) {
              nextExpanded.add(replaceMovedPath(dir, normalizedOps) || dir);
            }

            const nextDirEntries = new Map<string, FileEntry[]>();
            for (const [dir, entries] of state.dirEntries.entries()) {
              nextDirEntries.set(
                replaceMovedPath(dir, normalizedOps) || dir,
                replaceMovedEntries(dir, entries, normalizedOps),
              );
            }

            return {
              expandedDirs: nextExpanded,
              dirEntries: nextDirEntries,
            };
          });

          get().retargetMovedTabs(normalizedOps);

          const dirsToReload = new Set<string>();
          for (const { oldPath, newPath } of normalizedOps) {
            const oldDir = oldPath.substring(0, oldPath.lastIndexOf('/'));
            const newDir = newPath.substring(0, newPath.lastIndexOf('/'));
            if (oldDir) dirsToReload.add(oldDir);
            if (newDir) dirsToReload.add(newDir);
          }
          await Promise.all(
            Array.from(dirsToReload).map((dir) => get().loadDirectory(dir)),
          );

          await Promise.all(
            normalizedOps.map(async ({ oldPath, newPath }) => {
              await syncRenamedThreadProjection(
                remoteSession,
                oldPath,
                newPath,
              ).catch((error) => {
                console.error('Error syncing renamed chat session:', error);
              });
            }),
          );

          return { success: true };
        } catch (e) {
          console.error('Error moving entries:', e);
          return { success: false, error: (e as Error).message };
        }
      },

      copyEntries: async (ops) => {
        const normalizedOps = ops
          .map((op) => ({
            sourcePath: (op.sourcePath || '').trim(),
            targetPath: (op.targetPath || '').trim(),
          }))
          .filter(
            (op) =>
              !!op.sourcePath &&
              !!op.targetPath &&
              op.sourcePath !== op.targetPath,
          );

        if (normalizedOps.length === 0) {
          return { success: true };
        }

        try {
          for (const { sourcePath, targetPath } of normalizedOps) {
            const result = await fileService.copy(
              sourcePath,
              targetPath,
              false,
            );
            if (result.error) {
              throw new Error(result.error);
            }
          }

          const dirsToReload = new Set<string>();
          for (const { targetPath } of normalizedOps) {
            const dir = targetPath.substring(0, targetPath.lastIndexOf('/'));
            if (dir) dirsToReload.add(dir);
          }
          await Promise.all(
            Array.from(dirsToReload).map((dir) => get().loadDirectory(dir)),
          );
          return { success: true };
        } catch (e) {
          console.error('Error copying entries:', e);
          return { success: false, error: (e as Error).message };
        }
      },

      renameEntry: async (oldPath: string, newPath: string) => {
        return get().moveEntries([{ oldPath, newPath }]);
      },

      persistPastedImage: async (
        base64: string,
        documentPath?: string | null,
      ) => {
        const currentFilePath =
          (documentPath || '').trim() || (get().currentFilePath || '').trim();
        if (!currentFilePath) {
          return { error: '请先保存 Markdown 文件后再粘贴图片' };
        }
        try {
          const file = base64ToFile(
            base64,
            `image-${formatImageTimestamp(new Date())}.png`,
            'image/png',
          );
          const result = await importFile({
            purpose: 'markdown-image',
            targetDocumentPath: currentFilePath,
            file,
          });
          const relativePath = (result.documentRef || '').trim();
          const fileName =
            relativePath
              .split('/')
              .pop()
              ?.replace(/\.[^.]+$/i, '') || '';
          if (!fileName) {
            return { error: '写入图片文件失败' };
          }
          return {
            markdown: buildPastedImageMarkdown(relativePath, fileName),
            documentRef: relativePath,
          };
        } catch (error) {
          return {
            error: error instanceof Error ? error.message : '写入图片文件失败',
          };
        }
      },

      readTextFile: async (path: string) => {
        try {
          const result = await fileService.readFile(path);
          if (result.error || result.tooLarge) {
            return null;
          }
          return typeof result.content === 'string' ? result.content : '';
        } catch {
          return null;
        }
      },

      writeTextFile: async (path: string, content: string) => {
        try {
          const result = await fileService.writeFile(path, content, {
            create: true,
            overwrite: true,
            atomic: true,
          });
          return !result.error;
        } catch {
          return false;
        }
      },

      writeBase64File: async (
        path: string,
        base64: string,
        options?: { overwrite?: boolean },
      ) => {
        try {
          const result = await fileService.writeFile(path, base64, {
            encoding: 'base64',
            create: true,
            overwrite: options?.overwrite ?? true,
            atomic: true,
          });
          if (result.error) {
            return { success: false, error: result.error };
          }
          return { success: true };
        } catch (error) {
          return {
            success: false,
            error: error instanceof Error ? error.message : '写入文件失败',
          };
        }
      },

      ensureAgentRecord: async (agentID: string) => {
        const key = (agentID || '').trim();
        if (!key || get().connectionState !== 'connected') {
          return null;
        }

        const now = Date.now();
        const cached = agentRecordCache.get(key);
        if (cached && cached.expireAt > now) {
          return cached.data;
        }

        const pending = agentRecordInflight.get(key);
        if (pending) {
          return pending;
        }

        const promise = (async () => {
          let record = get().nodesByID.get(key) || null;
          if (!record) {
            await refreshNodesCache(true);
            record = get().nodesByID.get(key) || null;
          }
          return record;
        })()
          .then((record) => {
            const ttl = record
              ? AGENT_RECORD_TTL_MS
              : AGENT_RECORD_NEGATIVE_TTL_MS;
            agentRecordCache.set(key, {
              data: record,
              expireAt: Date.now() + ttl,
            });
            return record;
          })
          .catch(() => {
            agentRecordCache.set(key, {
              data: null,
              expireAt: Date.now() + AGENT_RECORD_NEGATIVE_TTL_MS,
            });
            return null;
          })
          .finally(() => {
            agentRecordInflight.delete(key);
          });

        agentRecordInflight.set(key, promise);
        return promise;
      },

      resolveAgentByID: (agentID: string) => {
        const id = (agentID || '').trim();
        if (!id) {
          return null;
        }
        const node = get().nodesByID.get(id);
        if (!node) {
          return null;
        }
        const meta = nodeMeta(node);
        return {
          id: node.id,
          name: typeof meta.name === 'string' ? meta.name : null,
          avatar: typeof meta.avatar === 'string' ? meta.avatar : null,
          model: typeof meta.model === 'string' ? meta.model.trim() || null : null,
          uri: nodeURI(node),
          path: workdirFromNode(node),
        };
      },

      resolveAgentIDByPath: (path: string) => {
        const p = (path || '').trim();
        if (!p) {
          return null;
        }
        return get().agentBindingByCwd.get(p)?.effectiveAgentID || null;
      },

      resolveAgentIDByUri: (uri: string) => {
        const uriKey = (uri || '').trim();
        if (!uriKey) {
          return null;
        }
        for (const node of get().nodesByID.values()) {
          if (nodeURI(node) === uriKey) {
            return (node.id || '').trim() || null;
          }
        }
        return null;
      },

      hasAgentBinding: (cwd: string) => {
        const key = (cwd || '').trim();
        return Boolean(key && get().agentBindingByCwd.has(key));
      },

      getEffectiveAgentForCwd: (cwd: string) => {
        const key = (cwd || '').trim();
        if (!key) {
          return null;
        }
        const binding = get().agentBindingByCwd.get(key);
        if (!binding?.effectiveAgentID) {
          return null;
        }
        const resolved = get().resolveAgentByID(binding.effectiveAgentID);
        return {
          agentID: binding.effectiveAgentID,
          agentName: (resolved?.name || '').trim() || binding.effectiveAgentID,
          agentCwd: key,
        };
      },

      getChatAgentForCwd: (cwd: string) => {
        const target = get().getEffectiveAgentForCwd(cwd);
        if (!target) {
          return null;
        }
        const opcode = get().getAgentOpCode(target.agentID);
        return opcode ? target : null;
      },

      getDefaultOpenBrainForCwd: (cwd: string) => {
        const agentCwd = normalizeDirPath(cwd);
        if (!agentCwd) {
          return null;
        }
        const openbrainNode = get().agentNodes.find((node) => {
          if (nodeKind(node) !== 'agent' || !findChatCapableAgentOpcode(node)) {
            return false;
          }
          const id = nodeID(node).toLowerCase();
          const name = String(nodeMeta(node).name || '')
            .trim()
            .toLowerCase();
          const nodePath = normalizeDirPath(workdirFromNode(node) || '');
          const pathName =
            nodePath.split('/').filter(Boolean).pop()?.toLowerCase() || '';
          return (
            id === 'agent-openbrain' ||
            id === 'openbrain' ||
            name === 'openbrain' ||
            pathName === 'openbrain'
          );
        });
        if (!openbrainNode) {
          return null;
        }
        const agentID = nodeID(openbrainNode);
        if (!agentID) {
          return null;
        }
        const name =
          String(nodeMeta(openbrainNode).name || '').trim() || 'openbrain';
        return {
          agentID,
          agentName: name,
          agentCwd,
        };
      },

      getAgentOpCode: (agentID: string) => {
        const key = (agentID || '').trim();
        if (!key) return null;
        const node = get().nodesByID.get(key);
        return findChatCapableAgentOpcode(node);
      },

      getAgentSubagents: (agentID: string) => {
        const state = get();
        const key = agentNodeLookupID(agentID, state);
        if (!key) {
          return [];
        }
        const node = resolveMutableAgentNode(key, state);
        if (!node) {
          return [];
        }
        const ids = subagentIDsFromMeta(nodeMeta(node));
        const seen = new Set<string>();
        const items: AgentSubagentInfo[] = [];
        for (const id of ids) {
          const subagentID = normalizeAgentNodeID(id);
          if (!subagentID || seen.has(subagentID)) {
            continue;
          }
          seen.add(subagentID);
          const subagent = state.nodesByID.get(subagentID) || null;
          if (!findChatCapableAgentOpcode(subagent)) {
            continue;
          }
          items.push(agentInfoFromNode(subagentID, subagent));
        }
        return items;
      },

      getMountableAgentSubagents: (agentID: string) => {
        const state = get();
        const parentID = agentNodeLookupID(agentID, state);
        if (!parentID) {
          return [];
        }
        const parentNode = resolveMutableAgentNode(parentID, state);
        if (!parentNode) {
          return [];
        }
        const mutableParentID = normalizeAgentNodeID(nodeID(parentNode));
        const mountedIDs = new Set(
          subagentIDsFromMeta(nodeMeta(parentNode))
            .map((id) => normalizeAgentNodeID(id))
            .filter(Boolean),
        );
        const root = resolveAgentsRootForSubagentCandidates(state, parentNode);
        const parentWorkdir = agentConfigWorkdirFromNode(parentNode);
        const seen = new Set<string>();
        const items: AgentSubagentInfo[] = [];

        for (const node of state.nodesByID.values()) {
          const id = normalizeAgentNodeID(nodeID(node));
          if (
            !id ||
            id === parentID ||
            id === mutableParentID ||
            seen.has(id) ||
            mountedIDs.has(id) ||
            isBindAgentNode(node) ||
            !findChatCapableAgentOpcode(node) ||
            (!isMountableAgentUnderRoot(node, root) &&
              !isLocalSubagentOfParent(node, parentWorkdir))
          ) {
            continue;
          }
          seen.add(id);
          items.push(agentInfoFromNode(id, node));
        }

        items.sort((a, b) => {
          const aName = (a.name || a.id).trim();
          const bName = (b.name || b.id).trim();
          const nameOrder = aName.localeCompare(bName, undefined, {
            sensitivity: 'base',
          });
          if (nameOrder !== 0) {
            return nameOrder;
          }
          const pathOrder = (a.path || '').localeCompare(
            b.path || '',
            undefined,
            { sensitivity: 'base' },
          );
          if (pathOrder !== 0) {
            return pathOrder;
          }
          return a.id.localeCompare(b.id, undefined, { sensitivity: 'base' });
        });

        return items;
      },

      mountAgentSubagent: async (agentID: string, subagentID: string) => {
        const targetID = normalizeAgentNodeID(subagentID);
        if (!targetID) {
          return false;
        }

        await refreshNodesCache(true);
        const state = get();
        const parentID = agentNodeLookupID(agentID, state);
        if (!parentID || parentID === targetID) {
          return false;
        }
        const parentNode = resolveMutableAgentNode(parentID, state);
        const targetNode = state.nodesByID.get(targetID) || null;
        if (!parentNode || !targetNode || nodeKind(targetNode) !== 'agent') {
          return false;
        }
        const agentMd = agentConfigPathFromNode(parentNode);
        const parentWorkdir = agentConfigWorkdirFromNode(parentNode);
        if (!agentMd || !parentWorkdir) {
          return false;
        }

        const readResult = await fileService.readFile(agentMd);
        if (readResult.error || typeof readResult.content !== 'string') {
          return false;
        }
        const next = addSubagentToAgentMarkdown(
          readResult.content,
          parentWorkdir,
          targetID,
          get(),
        );
        if (!next.added || next.content === readResult.content) {
          return false;
        }
        const writeResult = await fileService.writeFile(agentMd, next.content, {
          create: true,
          overwrite: true,
          atomic: true,
        });
        if (writeResult.error) {
          return false;
        }

        clearAgentRecordCache();
        clearAgentObjectIndex();
        await refreshNodesCache(true);
        await get().loadDirectory(parentWorkdir);
        await get().loadDirectory(`${parentWorkdir}/.agent`);
        await get().fetchDirAgentsInfo(parentWorkdir);
        return true;
      },

      unmountAgentSubagent: async (agentID: string, subagentID: string) => {
        const targetID = normalizeAgentNodeID(subagentID);
        if (!targetID) {
          return false;
        }

        await refreshNodesCache(true);
        const state = get();
        const parentID = agentNodeLookupID(agentID, state);
        if (!parentID) {
          return false;
        }
        const parentNode = resolveMutableAgentNode(parentID, state);
        const agentMd = agentConfigPathFromNode(parentNode);
        const parentWorkdir = agentConfigWorkdirFromNode(parentNode);
        if (!agentMd || !parentWorkdir) {
          return false;
        }

        const readResult = await fileService.readFile(agentMd);
        if (readResult.error || typeof readResult.content !== 'string') {
          return false;
        }
        const next = removeSubagentFromAgentMarkdown(
          readResult.content,
          parentWorkdir,
          targetID,
          get(),
        );
        if (!next.removed || next.content === readResult.content) {
          return false;
        }
        const writeResult = await fileService.writeFile(agentMd, next.content, {
          create: true,
          overwrite: true,
          atomic: true,
        });
        if (writeResult.error) {
          return false;
        }

        clearAgentRecordCache();
        clearAgentObjectIndex();
        await refreshNodesCache(true);
        await get().loadDirectory(parentWorkdir);
        await get().loadDirectory(`${parentWorkdir}/.agent`);
        await get().fetchDirAgentsInfo(parentWorkdir);
        return true;
      },

      addAgentReference: async (targetDir: string, agentID: string) => {
        const dir = (targetDir || '').trim();
        if (!dir) {
          return;
        }
        const resolvedNodeID = await resolveCurrentAgentNodeID(agentID);
        if (!resolvedNodeID) {
          return;
        }

        const agentMd = `${dir}/.agent/AGENT.md`;

        try {
          const written = await writeAgentReferenceFile(dir, resolvedNodeID);
          if (!written) {
            return;
          }
          clearAgentRecordCache();
          clearAgentObjectIndex();
          set((state) => {
            const nextBindings = new Map(state.agentBindingByCwd);
            nextBindings.set(dir, {
              cwd: dir,
              localNodeID: null,
              effectiveAgentID: resolvedNodeID,
              source: 'bind',
            });
            return { agentBindingByCwd: nextBindings };
          });
          await get().loadDirectory(dir);
          await get().loadDirectory(`${dir}/.agent`);
          await get().fetchDirAgentsInfo(dir);
          await get().openFile(agentMd);
        } catch (e) {
          console.error('Error adding agent reference:', e);
        }
      },

      switchAgentReference: async (targetDir: string, agentID: string) => {
        const dir = (targetDir || '').trim();
        if (!dir) {
          return false;
        }
        const resolvedNodeID = await resolveCurrentAgentNodeID(agentID);
        if (!resolvedNodeID) {
          return false;
        }

        try {
          const written = await writeAgentReferenceFile(dir, resolvedNodeID);
          if (!written) {
            return false;
          }
          set((state) => {
            const nextBindings = new Map(state.agentBindingByCwd);
            nextBindings.set(dir, {
              cwd: dir,
              localNodeID: null,
              effectiveAgentID: resolvedNodeID,
              source: 'bind',
            });
            return { agentBindingByCwd: nextBindings };
          });
          clearAgentRecordCache();
          clearAgentObjectIndex();
          await get().loadDirectory(dir);
          await get().loadDirectory(`${dir}/.agent`);
          await get().fetchDirAgentsInfo(dir);
          return true;
        } catch (e) {
          console.error('Error switching agent reference:', e);
          return false;
        }
      },

      addCustomAgent: async (targetDir: string) => {
        const dir = (targetDir || '').trim();
        if (!dir) {
          return;
        }
        const dirName = dir.split('/').filter(Boolean).pop() || 'workspace';

        const agentDir = `${dir}/.agent`;
        const chatDir = `${agentDir}/chat`;
        const agentMd = `${agentDir}/AGENT.md`;

        try {
          await fileService.mkdir(agentDir, true);
          await fileService.mkdir(chatDir, true);
          await fileService.writeFile(
            agentMd,
            buildCustomAgentTemplate({ dirName }),
            {
              create: true,
              overwrite: true,
              atomic: true,
            },
          );
          clearAgentObjectIndex();
          await get().loadDirectory(dir);
          await get().loadDirectory(agentDir);
          await get().fetchDirAgentsInfo(dir);
          await get().openFile(agentMd, { focusEditor: true });
        } catch (e) {
          console.error('Error adding custom agent:', e);
        }
      },

      ensureDerivedDirs: async (opts?: { force?: boolean }) => {
        const derived = await ensureSystemConfig(opts);
        if (derived) {
          set({
            baseDir: derived.baseDir,
            workspaceRootDir: derived.workspaceDir,
            agentsRootDir: derived.agentsDir,
            instanceID: derived.instanceID,
          });
        }
        return derived;
      },

      refreshAgentNodes: async (opts?: { force?: boolean }) => {
        await refreshNodesCache(Boolean(opts?.force));
      },

      invalidateAgentScanCache,

      refreshVisibleWorkspaceTree: async () => {
        const snapshot = get();
        const currentDir = normalizeDirPath(snapshot.currentDir || '');
        if (snapshot.connectionState !== 'connected' || !currentDir) {
          return;
        }

        const visibleDirs = uniqueDirs([
          currentDir,
          ...Array.from(snapshot.expandedDirs),
        ]);
        const workspaceDirs = visibleDirs.filter((dir) =>
          isPathInsideRoot(dir, currentDir),
        );

        invalidateAgentScanCache();
        await refreshNodesCache(true);

        if (visibleDirs.length > 0) {
          await Promise.all(visibleDirs.map((dir) => get().loadDirectory(dir)));
        }

        if (workspaceDirs.length > 0) {
          await Promise.all(
            workspaceDirs.map((dir) => get().fetchDirAgentsInfo(dir)),
          );
        }
      },

      revealInSidebar: async (path: string) => {
        const filePath = (path || '').trim();
        if (!filePath) {
          return;
        }

        const { currentDir, expandedDirs } = get();
        if (!currentDir || !filePath.startsWith(currentDir)) {
          return;
        }

        // Get the relative path from currentDir
        const relativePath = filePath.slice(currentDir.length + 1);
        const parts = relativePath.split('/');

        // Expand all parent directories
        const dirsToExpand: string[] = [];
        let accumulated = currentDir;
        for (let i = 0; i < parts.length - 1; i++) {
          accumulated = `${accumulated}/${parts[i]}`;
          if (!expandedDirs.has(accumulated)) {
            dirsToExpand.push(accumulated);
          }
        }

        // Expand directories one by one to ensure content is loaded
        for (const dir of dirsToExpand) {
          get().toggleDir(dir);
        }

        // Wait for directories to load and DOM to update, then scroll to the file
        const scrollToFile = () => {
          const element = document.querySelector(
            `[data-file-path="${filePath}"]`,
          );
          if (element) {
            element.scrollIntoView({ block: 'center', behavior: 'smooth' });
          }
        };

        if (dirsToExpand.length === 0) {
          // No directories to expand, scroll immediately
          requestAnimationFrame(scrollToFile);
        } else {
          // Wait for directories to load (give some time for async loadDirectory)
          setTimeout(scrollToFile, 150);
        }
      },

      restoreBackups: async () => {
        if (!window.electronAPI?.backup) {
          return;
        }
        try {
          const backups = await window.electronAPI.backup.load();
          if (backups.length === 0) {
            return;
          }

          set((state) => {
            // Filter out welcome tab if we're restoring backups
            const filteredTabs = getOpenDocuments(state).filter(
              (tab) => tab.editorId !== 'welcome',
            );
            const restoredTabs: EditorTab[] = backups.map((backup) => ({
              id: backup.id,
              title: backup.title,
              editorId: backup.editorId,
              content: backup.content,
              isDirty: true, // Mark as dirty since it's not saved
              pendingScrollHeading: null,
            }));

            // Merge restored tabs (avoid duplicates by id)
            const existingIds = new Set(filteredTabs.map((t) => t.id));
            const newTabs = restoredTabs.filter((t) => !existingIds.has(t.id));
            const allTabs = [...filteredTabs, ...newTabs];

            if (allTabs.length === 0) {
              return {};
            }

            // If no active tab, set the first restored tab as active
            const activeTab = state.activeTabId
              ? allTabs.find((t) => t.id === state.activeTabId)
              : allTabs[0];

            if (!activeTab) {
              return patchDocumentsWithPinnedState(state, allTabs);
            }

            return {
              ...patchDocumentsWithPinnedState(state, allTabs),
              activeTabId: activeTab.id,
              ...toActiveState(activeTab),
            };
          });
        } catch (e) {
          console.error('Error restoring backups:', e);
        }
      },

      reloadOpenTabsByPaths,

      reloadOpenTabsFromDisk: async () => {
        if (get().connectionState !== 'connected') {
          return;
        }
        const snapshot = get();
        const pathTabs = getOpenDocuments(snapshot).filter(
          (tab) => tab.filePath && tab.uri,
        );
        if (pathTabs.length === 0) {
          return;
        }

        await reloadOpenTabsByPaths(
          pathTabs.map((tab) => tab.filePath!),
          { skipDirty: false },
        );
      },

      setStreamingChatPath: (path: string | null) => {
        streamingChatPath = normalizePosixPath((path || '').trim());
      },

      getStreamingChatPath: () => streamingChatPath || null,

      reloadVisibleWorkspaceAfterGitChange: async () => {
        if (get().connectionState !== 'connected') {
          return;
        }
        const snapshot = get();
        const dirsToReload = uniqueDirs([
          snapshot.currentDir,
          ...Array.from(snapshot.expandedDirs),
        ]);

        clearAgentRecordCache();
        clearAgentObjectIndex();
        invalidateAgentScanCache();
        set((state) => ({
          nodesByID: new Map<string, OpNode>(),
          nodeGraphRevision: state.nodeGraphRevision + 1,
          agentBindingByCwd: new Map<string, AgentBinding>(),
          agentNodes: [],
          skillNodes: [],
        }));

        if (dirsToReload.length > 0) {
          await Promise.all(
            dirsToReload.map((dir) => get().loadDirectory(dir)),
          );
        }

        await refreshNodesCache();

        await ensureSystemConfig();

        const workspaceDirs = getVisibleWorkspaceDirs(snapshot);
        if (workspaceDirs.length > 0) {
          await Promise.all(
            workspaceDirs.map((dir) => get().fetchDirAgentsInfo(dir)),
          );
        }
      },

      checkoutGitBranch: async (
        branch: string,
        options?: { create?: boolean },
      ) => {
        const targetBranch = (branch || '').trim();
        const currentDir = normalizeDirPath(get().currentDir || '');
        if (!targetBranch || !currentDir) {
          return { success: false, error: 'No workspace directory selected' };
        }
        if (get().connectionState !== 'connected') {
          return { success: false, error: 'Not connected' };
        }
        if (getOpenDocuments(get()).some((tab) => tab.isDirty)) {
          return {
            success: false,
            error:
              'Please save all modified editor tabs before switching branches',
          };
        }
        if (get().gitInfo.dirty.hasChanges) {
          return {
            success: false,
            error:
              'Please commit or discard Git changes before switching branches',
          };
        }

        const result = await gitService.checkout({
          path: currentDir,
          branch: targetBranch,
          create: options?.create === true,
        });
        if (result.error) {
          return { success: false, error: result.error };
        }

        await get().reloadVisibleWorkspaceAfterGitChange();
        await get().reloadOpenTabsFromDisk();
        await get().refreshGitInfo(currentDir);
        return {
          success: true,
          currentBranch: result.currentBranch || targetBranch,
        };
      },

      pushConfigSyncFiles: async (
        files: Array<{ name: string; content: string }>,
      ) => {
        if (!Array.isArray(files) || files.length === 0) {
          return;
        }
        const state = get();
        if (!state.remoteSession || !connection.isConnected()) {
          return;
        }
        const validFiles = files.filter((file) =>
          Boolean(
            file &&
            typeof file.name === 'string' &&
            file.name &&
            typeof file.content === 'string',
          ),
        );
        if (!validFiles.length) {
          return;
        }
        try {
          await connection.request('config/push', { files: validFiles });
        } catch (err) {
          console.warn('[configSync] push failed:', err);
        }
      },
    };
  });
}

export function getWorkspaceStore(tabId: string): WorkspaceStore {
  const existing = workspaceStores.get(tabId);
  if (existing) {
    bindConfigSyncPushListenerOnce();
    return existing;
  }
  const store = createWorkspaceStore(tabId);
  workspaceStores.set(tabId, store);
  bindConfigSyncPushListenerOnce();
  return store;
}

export function getActiveWorkspaceStore(): WorkspaceStore {
  const { activeTabId } = useTabManagerStore.getState();
  return getWorkspaceStore(activeTabId);
}

export function setWorkspaceActive(tabId: string, active: boolean) {
  const store = getWorkspaceStore(tabId);
  store.getState().setActive(active);
}

export function removeWorkspaceStore(tabId: string) {
  const store = workspaceStores.get(tabId);
  if (!store) {
    return;
  }
  store.getState().dispose();
  workspaceStores.delete(tabId);
}

export const useAppStore = (<T = AppState>(
  selector?: (state: AppState) => T,
): T => {
  const activeTabId = useTabManagerStore((state) => state.activeTabId);
  const store = getWorkspaceStore(activeTabId);
  const select = selector ?? ((state) => state as unknown as T);
  return useStore(store, select);
}) as AppStoreHook;

useAppStore.getState = () => getActiveWorkspaceStore().getState();
useAppStore.getStateByTabId = (tabId: string) =>
  getWorkspaceStore(tabId).getState();
useAppStore.getStoreByTabId = (tabId: string) => getWorkspaceStore(tabId);

export type { AppState };
